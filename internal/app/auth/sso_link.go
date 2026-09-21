package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/warmbly/warmbly/internal/app/token"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/pkg/crypt"
)

// A federated sign-in on an existing password account is parked here until
// that password arrives; the provider verified the address, not the account.

// ssoLinkTTL is how long the password form has: the login-code window.
const ssoLinkTTL = AuthSessionTTL

func getSSOLinkKey(sessionID uuid.UUID) string {
	return "sso_link:" + sessionID.String()
}

func getSSOLinkTriesKey(sessionID uuid.UUID) string {
	return "sso_link_tries:" + sessionID.String()
}

// createLinkChallenge parks the verified identity behind a single-use pending
// token; the client gets the address and provider to display, nothing else.
func (s *authService) createLinkChallenge(ctx context.Context, userID uuid.UUID, identity models.UserIdentity) (*models.LoginResult, *errx.Error) {
	sid := uuid.New()
	nonce, err := crypt.Nonce()
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	now := time.Now()
	pendTok, xerr := s.tokenService.GenerateTokenFor(token.PurposeSSOLink, userID, sid, "", nonce, now, now.Add(ssoLinkTTL))
	if xerr != nil {
		errs.CaptureException(xerr)
		return nil, errx.InternalError()
	}
	pending := &models.SSOLinkPending{
		UserID:   userID,
		Nonce:    nonce,
		Provider: identity.Provider,
		Issuer:   identity.Issuer,
		Subject:  identity.Subject,
		Email:    identity.Email,
	}
	if xerr := s.saveSSOLinkPending(ctx, sid, pending, ssoLinkTTL); xerr != nil {
		return nil, xerr
	}
	return &models.LoginResult{
		LinkRequired: true,
		PendingToken: pendTok,
		ExpiresIn:    int(ssoLinkTTL.Seconds()),
		LinkEmail:    identity.Email,
		LinkProvider: identity.Provider,
	}, nil
}

// SSOLinkConfirm takes the password for a parked federated sign-in. The
// password is checked against the provider-asserted address on the sign-in
// failure budget; the identity is attached only after the ban check and,
// on a 2FA account, only once the second factor passes too.
func (s *authService) SSOLinkConfirm(ctx context.Context, data *SSOLinkData, ipaddr, userAgent string) (*models.LoginResult, *errx.Error) {
	if data == nil || data.PendingToken == "" || data.Password == "" {
		return nil, errx.ErrCredentials
	}
	claims, xerr := s.tokenService.VerifyTokenFor(token.PurposeSSOLink, data.PendingToken)
	if xerr != nil {
		return nil, errx.ErrSSOLinkExpired
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Before(time.Now()) {
		return nil, errx.ErrSSOLinkExpired
	}
	pend, xerr := s.getSSOLinkPending(ctx, claims.SessionID)
	if xerr != nil {
		return nil, xerr
	}
	// Bound to its single-use record: a token of another purpose has no
	// sso_link record, and a replay after success finds none either.
	if pend == nil || pend.Nonce != claims.Nonce || pend.UserID != claims.UserID {
		return nil, errx.ErrSSOLinkExpired
	}
	// Charged before the check, so concurrent guesses cannot share one try.
	if !s.reserveSSOLinkAttempt(ctx, claims.SessionID, time.Until(claims.ExpiresAt.Time)) {
		s.deleteSSOLinkPending(ctx, claims.SessionID)
		return nil, errx.ErrSSOLinkExpired
	}
	if s.loginFailureExceeded(ctx, pend.Email) {
		return nil, errx.ErrAuthLimit
	}

	uid, cerr := s.authRepository.IsValidCredentials(ctx, pend.Email, data.Password)
	if cerr != nil || uid != pend.UserID {
		if cerr == nil || errors.Is(cerr, errx.ErrCredentials) {
			s.recordLoginFailure(ctx, pend.Email)
			return nil, errx.ErrCredentials
		}
		return nil, cerr
	}
	s.clearLoginFailures(ctx, pend.Email)

	// Consumed before anything is linked, so one password proves exactly one
	// link and a retried request cannot mint a second session.
	s.deleteSSOLinkPending(ctx, claims.SessionID)

	if xerr := s.refuseSuspended(ctx, pend.UserID); xerr != nil {
		return nil, xerr
	}

	identity := models.UserIdentity{
		Provider: pend.Provider,
		Issuer:   pend.Issuer,
		Subject:  pend.Subject,
		Email:    pend.Email,
	}
	needsLink, xerr := s.identityUnclaimed(ctx, pend.UserID, identity)
	if xerr != nil {
		return nil, xerr
	}
	provider := sessionProvider(pend.Provider)
	if needsLink && s.twofa != nil {
		if enabled, _ := s.twofa.IsEnabled(ctx, pend.UserID); enabled {
			pendTok, expiresIn, perr := s.twofa.CreateLinkingChallenge(ctx, pend.UserID, identity, provider)
			if perr != nil {
				return nil, perr
			}
			return &models.LoginResult{TwoFARequired: true, PendingToken: pendTok, ExpiresIn: expiresIn}, nil
		}
	}
	if needsLink {
		if xerr := s.linkIdentity(ctx, pend.UserID, identity); xerr != nil {
			return nil, xerr
		}
	}
	if s.identities != nil && identity.Issuer != "" && identity.Subject != "" {
		_ = s.identities.TouchLogin(ctx, identity.Issuer, identity.Subject)
	}

	return s.finishLoginAs(ctx, pend.UserID, ipaddr, userAgent, provider)
}

// identityUnclaimed reports whether the identity still has to be linked to
// this account: false when a parallel challenge already linked it (a plain
// re-login), refused when it belongs to someone else or the account already
// holds another subject from the issuer.
func (s *authService) identityUnclaimed(ctx context.Context, userID uuid.UUID, identity models.UserIdentity) (bool, *errx.Error) {
	if s.identities == nil || identity.Issuer == "" || identity.Subject == "" {
		return false, nil
	}
	owner, ierr := s.identities.FindUserByIdentity(ctx, identity.Issuer, identity.Subject)
	if ierr != nil {
		errs.CaptureException(ierr)
		return false, errx.InternalError()
	}
	if owner == userID {
		return false, nil
	}
	if owner != uuid.Nil {
		return false, errx.New(errx.Forbidden, "that identity is already linked to another account")
	}
	if xerr := s.refuseSecondIdentity(ctx, userID, identity.Issuer); xerr != nil {
		return false, xerr
	}
	return true, nil
}

// reserveSSOLinkAttempt charges one password attempt to the challenge and
// reports whether it is still within AuthAttempts. Fails open on a cache
// error, like the other budgets: it is a brake, not the lock.
func (s *authService) reserveSSOLinkAttempt(ctx context.Context, sessionID uuid.UUID, ttl time.Duration) bool {
	key := getSSOLinkTriesKey(sessionID)
	count, err := s.cache.Incr(ctx, key).Result()
	if err != nil {
		errs.CaptureException(err)
		return true
	}
	if count == 1 && ttl > 0 {
		if err := s.cache.Expire(ctx, key, ttl).Err(); err != nil {
			errs.CaptureException(err)
		}
	}
	return count <= AuthAttempts
}

func (s *authService) saveSSOLinkPending(ctx context.Context, sessionID uuid.UUID, pending *models.SSOLinkPending, ttl time.Duration) *errx.Error {
	if ttl <= 0 {
		return errx.ErrSession
	}
	data, err := json.Marshal(pending)
	if err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}
	if err := s.cache.Set(ctx, getSSOLinkKey(sessionID), data, ttl).Err(); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}
	return nil
}

func (s *authService) getSSOLinkPending(ctx context.Context, sessionID uuid.UUID) (*models.SSOLinkPending, *errx.Error) {
	data, err := s.cache.Get(ctx, getSSOLinkKey(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	var pending models.SSOLinkPending
	if err := json.Unmarshal(data, &pending); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return &pending, nil
}

func (s *authService) deleteSSOLinkPending(ctx context.Context, sessionID uuid.UUID) {
	if err := s.cache.Del(ctx, getSSOLinkKey(sessionID), getSSOLinkTriesKey(sessionID)).Err(); err != nil {
		errs.CaptureException(err)
	}
}
