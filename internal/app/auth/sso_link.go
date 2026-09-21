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

// A federated sign-in whose address belongs to an existing password account
// is parked here until that password arrives. The provider verified the
// address; the password is what proves the person at the provider is the
// person who owns the account, so the identity is attached only after it.

// ssoLinkTTL is how long the password form has. The same window a login code
// gets: long enough to fetch a password manager, short enough that a parked
// challenge is not worth stealing.
const ssoLinkTTL = AuthSessionTTL

func getSSOLinkKey(sessionID uuid.UUID) string {
	return "sso_link:" + sessionID.String()
}

// createLinkChallenge parks the verified identity behind a single-use pending
// token. Nothing about the identity travels to the client: the form gets the
// address and the provider name to display, and hands back only the token.
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

// SSOLinkConfirm takes the account's password for a parked federated sign-in,
// attaches the identity and issues the session through the same gate every
// other login uses, so a ban or an enrolled second factor holds here too.
//
// The password is checked against the address the provider asserted, never
// one the request supplies, and a wrong one is charged to the same per-address
// budget as a wrong password at sign-in: this form is not a second, unmetered
// place to guess one.
func (s *authService) SSOLinkConfirm(ctx context.Context, data *SSOLinkData, ipaddr, userAgent string) (*models.LoginResult, *errx.Error) {
	if data == nil || data.PendingToken == "" || data.Password == "" {
		return nil, errx.ErrCredentials
	}
	claims, xerr := s.tokenService.VerifyTokenFor(token.PurposeSSOLink, data.PendingToken)
	if xerr != nil {
		return nil, errx.ErrSession
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Before(time.Now()) {
		return nil, errx.ErrSession
	}
	pend, xerr := s.getSSOLinkPending(ctx, claims.SessionID)
	if xerr != nil {
		return nil, xerr
	}
	// Bound to its single-use record: a token of another purpose has no
	// sso_link record, and a replay after success finds none either.
	if pend == nil || pend.Nonce != claims.Nonce || pend.UserID != claims.UserID {
		return nil, errx.ErrSession
	}
	if pend.Tries >= AuthAttempts {
		s.deleteSSOLinkPending(ctx, claims.SessionID)
		return nil, errx.ErrCodeLimit
	}
	if s.loginFailureExceeded(ctx, pend.Email) {
		return nil, errx.ErrAuthLimit
	}

	uid, cerr := s.authRepository.IsValidCredentials(ctx, pend.Email, data.Password)
	if cerr != nil || uid != pend.UserID {
		if cerr == nil || errors.Is(cerr, errx.ErrCredentials) {
			pend.Tries++
			_ = s.saveSSOLinkPending(ctx, claims.SessionID, pend, time.Until(claims.ExpiresAt.Time))
			s.recordLoginFailure(ctx, pend.Email)
			return nil, errx.ErrCredentials
		}
		return nil, cerr
	}
	s.clearLoginFailures(ctx, pend.Email)

	// Consumed before anything is linked, so one password proves exactly one
	// link and a retried request cannot mint a second session.
	s.deleteSSOLinkPending(ctx, claims.SessionID)

	identity := models.UserIdentity{
		Provider: pend.Provider,
		Issuer:   pend.Issuer,
		Subject:  pend.Subject,
		Email:    pend.Email,
	}
	// Re-checked at link time: the account may have gained an identity from
	// this issuer while the form sat open.
	if xerr := s.refuseSecondIdentity(ctx, pend.UserID, identity.Issuer); xerr != nil {
		return nil, xerr
	}
	if xerr := s.linkIdentity(ctx, pend.UserID, identity); xerr != nil {
		return nil, xerr
	}
	if s.identities != nil && identity.Issuer != "" && identity.Subject != "" {
		_ = s.identities.TouchLogin(ctx, identity.Issuer, identity.Subject)
	}

	return s.finishLoginAs(ctx, pend.UserID, ipaddr, userAgent, sessionProvider(pend.Provider))
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
	if err := s.cache.Del(ctx, getSSOLinkKey(sessionID)).Err(); err != nil {
		errs.CaptureException(err)
	}
}
