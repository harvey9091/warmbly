package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/pkg/crypt"
)

// Login and registration keep separate send budgets. They used to share one
// key, which let anyone lock a known user out of login for the whole window by
// POSTing /auth/register with that user's address.
func getEmailVerificationKey(flow, email string) string {
	return "email_verification:" + flow + ":" + crypt.SHA256(email)
}

// getKnownDeviceKey remembers a device that has already completed a full login,
// so AUTH_LOGIN_CODE=new_device only challenges genuinely new ones.
func getKnownDeviceKey(userID uuid.UUID, fingerprint string) string {
	return "known_device:" + userID.String() + ":" + fingerprint
}

func getPasswordResetLimitKey(email string) string {
	return "password_reset_limit:" + crypt.SHA256(email)
}

// getLoginFailureKey counts wrong passwords for one address. Keyed on the
// address rather than the user id because the lookup that would resolve the id
// is the thing being throttled, and a miss must cost the guesser the same as a
// hit.
func getLoginFailureKey(email string) string {
	return "login_fail:" + crypt.SHA256(email)
}

// getReauthFailureKey counts failed confirmations for one account. The
// re-authentication endpoint checks a password, so without its own budget it is
// a second, unthrottled place to guess one: the per-IP limiter allows a few
// hundred an hour and the per-account login counter does not see this path.
func getReauthFailureKey(userID uuid.UUID) string {
	return "reauth_fail:" + userID.String()
}

func getLoginSessionKey(sessionID uuid.UUID) string {
	return "login_sess:" + sessionID.String()
}

func getRegistrationSessionKey(sessionID uuid.UUID) string {
	return "registration_sess:" + sessionID.String()
}

func getResetPasswordSessionKey(sessionID uuid.UUID) string {
	return "reset_password:" + sessionID.String()
}

func (s *authService) saveLoginSession(ctx context.Context, sessionID uuid.UUID, session *models.LoginSession, expiresAt time.Time) *errx.Error {
	data, err := json.Marshal(session)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	if err := s.cache.Set(ctx, getLoginSessionKey(sessionID), data, time.Until(expiresAt)).Err(); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	return nil
}

func (s *authService) getLoginSession(ctx context.Context, sessionID uuid.UUID) (*models.LoginSession, *errx.Error) {
	data, err := s.cache.Get(ctx, getLoginSessionKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	var session models.LoginSession
	if err := json.Unmarshal(data, &session); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	return &session, nil
}

func (s *authService) saveRegistrationSession(ctx context.Context, sessionID uuid.UUID, session *models.RegistrationSession, expiresAt time.Time) *errx.Error {
	data, err := json.Marshal(session)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	if err := s.cache.Set(ctx, getRegistrationSessionKey(sessionID), data, time.Until(expiresAt)).Err(); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	return nil
}

func (s *authService) getRegistrationSession(ctx context.Context, sessionID uuid.UUID) (*models.RegistrationSession, *errx.Error) {
	data, err := s.cache.Get(ctx, getRegistrationSessionKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	var session models.RegistrationSession
	if err := json.Unmarshal(data, &session); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}

	return &session, nil
}

func (s *authService) canSendEmail(ctx context.Context, flow, email string) *errx.Error {
	key := getEmailVerificationKey(flow, email)

	count, err := s.cache.Incr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	if count == 1 {
		if err := s.cache.Expire(ctx, key, AuthEmailTTL).Err(); err != nil {
			errs.CaptureException(err)
			return errx.InternalError()
		}
	}

	if count > AuthEmailLimit {
		return errx.ErrAuthLimit
	}

	return nil
}

func (s *authService) passwordResetLimit(ctx context.Context, email string) *errx.Error {
	key := getPasswordResetLimitKey(email)

	count, err := s.cache.Incr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	if count == 1 {
		if err := s.cache.Expire(ctx, key, PasswordResetLimitTTL).Err(); err != nil {
			errs.CaptureException(err)
			return errx.InternalError()
		}
	}

	if count > PasswordResetLimit {
		return errx.ErrAuthLimit
	}

	return nil
}

// refundPasswordResetLimit gives back an attempt that produced no mail through
// no fault of the person asking. The budget is only two requests per four
// hours, so without this a transient SES rejection or a cache blip spent half
// of someone's allowance and a second one locked them out of the flow for the
// rest of the afternoon, with nothing in their inbox to explain why. Best
// effort: failing to refund must never turn into a failed request on top of
// the one that already failed.
//
// Only OUR failures are refunded. An address with no account still pays, or
// the counter stops costing an attacker anything to probe with.
func (s *authService) refundPasswordResetLimit(ctx context.Context, email string) {
	key := getPasswordResetLimitKey(email)

	// DECR cannot take the counter below what this request added: the key is
	// only ever incremented by a request that reaches here to undo it, and a
	// key that expired in between comes back at -1 with no TTL, which would
	// hand out unlimited attempts. Refuse that case rather than create it.
	count, err := s.cache.Decr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return
	}
	if count < 0 {
		if err := s.cache.Del(ctx, key).Err(); err != nil {
			errs.CaptureException(err)
		}
	}
}

// saveResetPasswordSession binds the emailed reset JWT to a server-side nonce.
// The TTL is PasswordResetTTL, the same lifetime the JWT carries and the same
// one the email quotes: it used to be SessionTTL (10 minutes) against a 1-hour
// token and a mail that promised 4 hours.
func (s *authService) saveResetPasswordSession(ctx context.Context, sessionID uuid.UUID, nonce string) *errx.Error {
	if err := s.cache.SetEx(ctx, getResetPasswordSessionKey(sessionID), nonce, PasswordResetTTL).Err(); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	return nil
}

func (s *authService) getResetPasswordSession(ctx context.Context, sessionID uuid.UUID) (string, *errx.Error) {
	val, err := s.cache.Get(ctx, getResetPasswordSessionKey(sessionID)).Result()
	if err != nil {
		// An expired or already-used link is ordinary, not an internal fault.
		if errors.Is(err, redis.Nil) {
			return "", errx.ErrToken
		}
		errs.CaptureException(err)
		return "", errx.InternalError()
	}

	return val, nil
}

// rememberDevice marks this device as having completed a full login, so
// AUTH_LOGIN_CODE=new_device stops challenging it.
func (s *authService) rememberDevice(ctx context.Context, userID uuid.UUID, userAgent string) {
	fp := deviceFingerprint(userAgent)
	if fp == "" {
		return
	}
	_ = s.cache.SetEx(ctx, getKnownDeviceKey(userID, fp), "1", KnownDeviceTTL).Err()
}

// isKnownDevice reports whether this device has completed a login inside the
// retention window.
func (s *authService) isKnownDevice(ctx context.Context, userID uuid.UUID, userAgent string) bool {
	fp := deviceFingerprint(userAgent)
	if fp == "" {
		return false
	}
	n, err := s.cache.Exists(ctx, getKnownDeviceKey(userID, fp)).Result()
	return err == nil && n > 0
}

// deviceFingerprint is deliberately weak: a user agent is trivially spoofable,
// so this is a convenience signal for skipping a code on a device the user has
// already used, never an authentication factor. An empty agent yields no
// fingerprint, which fails closed into "new device".
func deviceFingerprint(userAgent string) string {
	if strings.TrimSpace(userAgent) == "" {
		return ""
	}
	return crypt.SHA256(userAgent)
}

func (s *authService) deletePasswordResetSession(ctx context.Context, sessionID uuid.UUID) *errx.Error {
	val, err := s.cache.Del(ctx, getResetPasswordSessionKey(sessionID)).Result()
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}

	if val == 0 {
		return errx.ErrToken
	}

	return nil
}

// loginFailureExceeded reports whether this address has spent its hourly budget
// of wrong passwords. It fails OPEN on a cache error: the limiter is a brake on
// guessing, and a Redis outage must not lock every customer out of their own
// account.
func (s *authService) loginFailureExceeded(ctx context.Context, email string) bool {
	count, err := s.cache.Get(ctx, getLoginFailureKey(email)).Int64()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			errs.CaptureException(err)
		}
		return false
	}
	return count >= LoginFailureLimit
}

// recordLoginFailure charges one wrong password to the address.
func (s *authService) recordLoginFailure(ctx context.Context, email string) {
	key := getLoginFailureKey(email)
	count, err := s.cache.Incr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return
	}
	if count == 1 {
		if err := s.cache.Expire(ctx, key, LoginFailureTTL).Err(); err != nil {
			errs.CaptureException(err)
		}
	}
}

// clearLoginFailures forgives the count once the right password arrives, so a
// person who mistypes a few times and then gets it right starts clean.
func (s *authService) clearLoginFailures(ctx context.Context, email string) {
	if err := s.cache.Del(ctx, getLoginFailureKey(email)).Err(); err != nil {
		errs.CaptureException(err)
	}
}

// ReserveReauthAttempt charges one attempt before the proof is checked, so
// concurrent guesses cannot all pass a read-only check. Fails open on a cache
// error, like the login counter: the budget is a brake on guessing, and a
// Redis outage must not stop someone confirming their own change.
func (s *authService) ReserveReauthAttempt(ctx context.Context, userID uuid.UUID) bool {
	key := getReauthFailureKey(userID)
	count, err := s.cache.Incr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return true
	}
	if count == 1 {
		if err := s.cache.Expire(ctx, key, LoginFailureTTL).Err(); err != nil {
			errs.CaptureException(err)
		}
	}
	return count <= LoginFailureLimit
}

// ReleaseReauthAttempt refunds a reserved attempt that ended before any
// credential was compared.
func (s *authService) ReleaseReauthAttempt(ctx context.Context, userID uuid.UUID) {
	key := getReauthFailureKey(userID)
	n, err := s.cache.Decr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return
	}
	// A key that expired in between comes back from DECR with no TTL; drop it
	// so the budget cannot be left without an expiry.
	if n <= 0 {
		if err := s.cache.Del(ctx, key).Err(); err != nil {
			errs.CaptureException(err)
		}
	}
}

// ClearReauthFailures forgives the count once a confirmation succeeds.
func (s *authService) ClearReauthFailures(ctx context.Context, userID uuid.UUID) {
	if err := s.cache.Del(ctx, getReauthFailureKey(userID)).Err(); err != nil {
		errs.CaptureException(err)
	}
}
