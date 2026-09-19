# CASA evidence

**Specification:** App Defense Alliance CASA v2.1.1 (2026-06-03)
**Assurance level:** AL1
**Application:** Warmbly
**Google OAuth client:** `warmbly-mailboxes` (project 1010273043313)
**Repository state:** branch `chore/casa-al1-security-assessment`, base commit `b31c5d52`
**Prepared:** 2026-09-19

Scope is defined in `scope.md`. Cryptographic detail for 4.1.3 is in
`crypto-inventory.md`. OAuth detail for 3.2.1 and 3.2.2 is in `oauth.md`.

This document states the controls as they stand. The change log for the
assessment, which necessarily describes what each control replaced, is held
outside this repository and supplied to the lab directly: Warmbly is
self-hostable, so a public account of what a given control fixed is a map of
every instance that has not updated yet.

Every file reference below is `path:line` at the commit above.

---

## 1 Authentication

### 1.1.1 Authentication is resistant to brute force attacks

**Status: meets the requirement.** CASA asks for at least one of five controls.
Warmbly implements four of them.

**External authentication services.** Google Sign-In, Apple Sign-In and, for
self-hosted instances, a generic enterprise OIDC provider. Each is listed in
`oauth.md`. A deployment may also use none of them, so the controls below stand
on their own.

**Control 2.1, rate limiting under 100 failed attempts per account per hour.**
Failed passwords are counted per account, in Redis, keyed on a hash of the
address:

- `internal/app/auth/config.go` — `LoginFailureLimit = 10`, `LoginFailureTTL = 1 hour`
- `internal/app/auth/cache.go` — `getLoginFailureKey`, `loginFailureExceeded`, `recordLoginFailure`, `clearLoginFailures`
- `internal/app/auth/login.go:36` — the budget is checked before the hash comparison, so a caller past the limit cannot even measure Argon2's timing
- `internal/app/auth/login.go:43` — a wrong password is charged; `:47` clears the count on success

Ten per hour is well inside the hundred CASA allows. The counter is keyed on the
address rather than the resolved user id, so a guesser learns nothing from the
difference between an account that exists and one that does not.

Per-source limiting sits alongside it: `internal/api/middleware/ratelimit_ip.go`
bounds every unauthenticated `/auth` request to 60 per 15 minutes per IP
(`AUTH_IP_RATE_LIMIT`), and the remaining public routes to 600 per 15 minutes
(`PUBLIC_IP_RATE_LIMIT`).

**Control 2.2, CAPTCHA.** Cloudflare Turnstile on login, registration, password
reset request and password reset confirm:

- `internal/pkg/captcha/turnstile.go` — verifier, including remote-IP check, optional hostname pin and a five-minute challenge-freshness window
- `internal/app/auth/login.go:26`, `internal/app/auth/registration.go:31`, `internal/app/auth/reset_password.go:20` and `:138` — enforcement

Enabled by setting `TURNSTILE_SECRET`. The hosted deployment sets it. A
self-hosted instance may not, which is why control 2.1 above is unconditional.

**Control 2.4, minimum length with breached-password prohibition.** Both halves,
server-side:

- `internal/pkg/crypt/validation.go` — `CheckPassword`: 8 to 128 characters, then a denylist lookup
- `internal/pkg/crypt/passwords/breached.txt` — the UK NCSC list of the 100,000 most commonly breached passwords, reduced to the 46,528 entries long enough to pass the length rule. Sourced from the Have I Been Pwned corpus
- `internal/pkg/crypt/validation.go` — `IsBreachedPassword` lowercases the candidate, so recasing a known password does not get past it
- `internal/pkg/crypt/validation_test.go` — regression tests, including that the list is actually loaded

Applied at every entry point: `internal/app/auth/registration.go`,
`internal/app/auth/reset_password.go` (both reset and change),
`internal/app/bootstrap/bootstrap.go`, `cmd/warmblyctl/password.go`.

This follows NIST SP 800-63B 5.1.1.2, which asks for a breach check and
explicitly discourages composition rules.

**Control 2.5, additional check from an unfamiliar device or location.**

- `internal/app/auth/login.go:120-149` — `loginCodeRequired`; under `AUTH_LOGIN_CODE=new_device` an emailed code is demanded for a user-agent not seen before
- `internal/app/authrisk/authrisk.go:33-70` — impossible-travel check; two sign-ins that could not be the same person travelling (faster than 1000 km/h) force the code even from a known device
- `internal/app/auth/cache.go` — known-device memory, 90 days

**Control 2.3, MFA enforced by default,** is the one Warmbly does not claim for
all users. TOTP and passkeys are available to everyone and are enforced for
administrative accounts (see 3.3.1).

**Artifacts:** `artifacts/screenshots/` (rate limit refusal, CAPTCHA challenge,
breached-password refusal, emailed-code challenge from a new device).

### 1.1.2 System-generated initial passwords or activation codes

**Status: meets the requirement.**

Warmbly generates no initial passwords. An account gets a password one of three
ways: the person chooses it at registration, the operator supplies one when
creating the first owner, or the person sets one through a reset. None of them
is a system-generated password that could become a long-term one.

Codes and one-time tokens:

| Verifier | Generation | Length / entropy | Expiry | Single use |
|---|---|---|---|---|
| Registration code | `internal/pkg/crypt/gen.go` `VerificationCode`, `crypto/rand.Int` | 6 digits, ~19.9 bits | 10 min | 3 attempts, session deleted |
| Login code | same | 6 digits | 10 min | 3 attempts |
| First-run setup token | `internal/app/bootstrap/bootstrap.go:170` | 32 bytes, 256 bits | 24 h | `GETDEL`, and refused once any account exists |
| Team invitation | `internal/app/organization/service.go:1201` | 32 bytes, 256 bits | 7 days default | Deleted on acceptance |
| Password reset | `internal/app/auth/reset_password.go:83` | JWT plus a 128-bit nonce | 1 h | Nonce deleted before use |

Codes are stored Argon2id-hashed, never in the clear. The 24-hour setup token is
inside the 48-hour maximum; the 7-day invitation is a capability to join a
workspace, not an account credential, and is the value CASA allows for a
link-style verifier.

### 1.1.3 Passwords stored in a form resistant to offline attacks

**Status: meets the requirement.**

Argon2id, which is in the NIST SP 800-63B 5.1.1.2 approved list:

- `internal/pkg/argon2/config.go` — memory 64 MiB, iterations 3, parallelism 2, 16-byte salt, 32-byte output
- `internal/pkg/argon2/hash.go` — `argon2.IDKey`, PHC-encoded, salt from `crypto/rand`
- `internal/pkg/argon2/verify.go:30` — `subtle.ConstantTimeCompare`

The parameters exceed the OWASP minimum of 19 MiB / t=2 / p=1. Comparison
happens at `internal/repository/pg_auth.go:67` and
`internal/app/auth/reset_password.go:203`.

Argon2id also protects the emailed login and registration codes and the 2FA
recovery codes. Full inventory in `crypto-inventory.md`.

### 1.2.1 Default credentials on publicly exposed interfaces

**Status: meets the requirement.**

No account exists on a fresh instance. The first one is created either from an
operator-supplied password or by claiming a one-time setup link printed at boot,
and the claim is refused once any account exists
(`internal/app/bootstrap/bootstrap.go:270-294`).

Fixture accounts with published passwords exist for local development only
(`cmd/seed`, `internal/seed`). Three things keep them out of a deployment:

- the compose seed service is behind a profile and is never published (`docker-compose.yml`)
- the installer has no seed step
- `cmd/seed/main.go` refuses to run when `APP_ENV` is set to anything other than a development value, which closes the case where the binary inside the release image is pointed at a production database

`internal/app/instancecheck/checks_security.go` reports a published default
secret still in use as an instance finding, continuously rather than once at
boot.

### 1.3.1 Out of band verifier expires in a reasonable timeframe

**Status: meets the requirement.** Password reset expires in 1 hour, inside the
7 days CASA allows. MFA-related verifiers expire in 10 minutes (emailed code) and
5 minutes (2FA pending challenge), inside the 30 minutes allowed. See the table
under 1.1.2.

### 1.3.2 Out of band verifier used only once

**Status: meets the requirement.**

- Emailed login and registration codes: the session is deleted on success (`internal/app/auth/login.go:186`)
- Password reset: the nonce is deleted before the password is written (`internal/app/auth/cache.go`)
- 2FA pending challenge: deleted before the session is minted (`internal/app/twofa/login.go:74-77`)
- Recovery codes: consumed by compare-and-swap on `used_at` (`internal/repository/pg_totp.go`)
- TOTP: the accepted time step is retired, so the same six digits cannot be presented twice inside their validity window (`internal/app/twofa/totp.go` `ValidateCodeStep`, `internal/repository/pg_totp.go` `ConsumeTOTPStep`, migration `000183`). The update is compare-and-swap, so two requests racing with one code cannot both win
- Setup token and SSO state: `GETDEL`
- Mailbox OAuth state: `GETDEL` (`internal/app/email/cache.go`)

### 1.3.3 Out of band verifier is securely random

**Status: meets the requirement.** Every verifier comes from `crypto/rand`:
`internal/pkg/crypt/gen.go` (`Nonce`, `VerificationCode` via `rand.Int`, so no
modulo bias), `internal/app/bootstrap/bootstrap.go`,
`internal/app/organization/service.go`, `internal/app/oauth/service.go`. No use
of `math/rand` for any secret; its only uses are scheduling jitter and warmup
behaviour sampling.

### 1.3.4 Out of band verifier resists brute force

**Status: meets the requirement.** The six-digit codes carry ~19.9 bits, above
the 20-bit guidance for a six-digit number, and being under 64 bits they are
rate limited as CASA requires:

- 3 attempts per login or registration session (`internal/app/auth/config.go` `AuthAttempts`)
- 5 sends per address per 30 minutes (`AuthEmailLimit`, `AuthEmailTTL`)
- 2 password-reset requests per address per 4 hours (`PasswordResetLimit`)
- 5 attempts per 2FA challenge, plus 20 verifications per IP per 15 minutes (`internal/app/twofa/service.go`)
- 60 requests per IP per 15 minutes across the whole `/auth` group

Every link-style token is 128 bits or more and needs no rate limit by the same
rule.

---

## 2 Session Management

### 2.1.1 No passwords or session tokens in URL parameters

**Status: meets the requirement.** Evidence: Burp scan (see `README.md`), plus:

Credentials are read from the `Authorization` header only:
`internal/api/middleware/auth.go:21-28`, `internal/api/middleware/apikey.go:70-88`.
A grep for query-parameter credential reads across `internal/` and `cmd/` finds
none. Passwords travel in JSON bodies on POST; there is no GET login form.

Tokens that do appear in a URL are single-purpose, single-use and short-lived,
never session tokens: the password-reset link (1 h), the invitation link, the
first-run setup link (24 h), the SSO handoff code (60 s), OAuth callback codes,
and the WebSocket ticket (10 min, minted per connect by `POST /v1/getaway`).

The WebSocket takes its credential as a query parameter because that is what the
Phoenix transport supports. That credential is now a purpose-scoped ticket and
nothing else: see 2.3.4.

### 2.2.1 Logout invalidates stateful session tokens

**Status: meets the requirement.**

Sessions are stateful. The access and refresh tokens are JWTs, but each carries a
nonce that must match the `sessions` row, so both are revocable:

- `internal/app/token/verify.go:41-68` — rejects a revoked session and a stale nonce
- `internal/app/token/logout.go` — `RevokeSession` stamps `revoked_at` and deletes the Redis cache entry, so revocation is immediate rather than waiting out the cache TTL
- `internal/app/token/logout.go` — `RevokeAllSession` for "sign out everywhere"
- `internal/repository/pg_token.go` — refresh rotates both nonces by compare-and-swap, so a replayed refresh token fails
- `internal/api/handler/session.go` — self-service session list and revocation

A ban now revokes live sessions too, through the token service so the cache is
cleared as well (`internal/app/admin/service.go`, `cmd/backend/main.go`).

### 2.2.2 Password change terminates other sessions

**Status: meets the requirement.**

- `internal/app/auth/reset_password.go:203-237` — changing a password requires the current one and then revokes every other session
- `internal/app/auth/reset_password.go:178-186` — a forgotten-password reset revokes all sessions
- `internal/repository/pg_token.go` — `RevokeOtherSessions` has no provider filter, so federated and passkey sessions are covered

### 2.2.3 Non-revocable stateless tokens expire within 24 hours

**Status: meets the requirement.** Every stateless token Warmbly mints is either
inside 24 hours or bound to a revocable server-side record:

| Token | TTL | Revocable |
|---|---|---|
| Access JWT | 12 h | Yes, session nonce |
| Refresh JWT | 180 d | Yes, session nonce, rotated on every use |
| Login-code challenge | 10 min | Yes, Redis nonce |
| Password reset | 1 h | Yes, Redis nonce |
| 2FA pending | 5 min | Yes, Redis record |
| WebSocket ticket | 10 min | n/a, inside 24 h |
| OAuth access token | 1 h | Yes, hashed row |
| OAuth refresh token | 90 d | Yes, hashed row, rotated |

Constants: `internal/app/token/config.go`, `internal/app/auth/config.go`,
`internal/app/socket/config.go`, `internal/models/oauth_app.go`.

### 2.3.1 and 2.3.2 Cookie Secure and HttpOnly attributes

**Status: not applicable, and confirmed by construction.** Warmbly sets no
cookies anywhere. A grep for `SetCookie`, `http.Cookie` and `Set-Cookie` across
`internal/` and `cmd/` returns nothing; no frontend uses `credentials: include`.
Authentication is a bearer token in the `Authorization` header on every surface:
dashboard, admin panel, iOS app and API.

Because the token is in `localStorage` rather than a cookie, the relevant client
risk is XSS rather than CSRF. The compensating controls are the content security
policy and framing headers added for 5.1.7, React's default escaping, the
parser-based sanitizer on all rendered HTML, the 12-hour access token, and
immediate server-side revocation.

Evidence: Burp scan should report no cookie findings, because there are no
cookies.

### 2.3.3 Session tokens rather than static API secrets

**Status: meets the requirement.**

A session is minted only after authentication, through one path for every
provider (`internal/app/auth/login.go` `finishLoginAsWith`). The session id is a
fresh UUID and both nonces are 128 bits from `crypto/rand`
(`internal/app/token/gen.go`).

API keys exist as a deliberate developer feature, not as the primary
authentication: 256-bit random, SHA-256 at rest, per-key permission bitmask, IP
allowlist, optional expiry, per-key rate limit, mailbox allowlist, revocation
with reason (`internal/app/apikey/service.go`,
`internal/api/middleware/apikey.go`). Third-party applications are steered to
OAuth 2.1 with one-hour access tokens instead.

Sensitive routes refuse API keys entirely and require a session
(`internal/api/routes.go`, the `jwtOnly` group), including creating an API key.

### 2.3.4 Stateless tokens signed, protected against substitution

**Status: meets the requirement.**

- `internal/app/token/gen.go` — HS256, and the verifier pins exactly that algorithm with `jwt.WithValidMethods([]string{"HS256"})` and `jwt.WithExpirationRequired()`. `alg=none` is refused
- `internal/config/config_auth.go` — `AUTH_SECRET` must be at least 32 bytes, enforced at boot. `realtime/config/runtime.exs` applies the same floor to the same value
- `realtime/lib/realtime/auth.ex` — `JOSE.JWT.verify_strict(..., ["HS256"], ...)`
- `internal/pkg/idtoken/idtoken.go` — third-party ID tokens are RS256-pinned with JWKS, issuer and audience checks
- `internal/api/middleware/oidc.go` — the Cloud Tasks caller check pins RS256 and now also the audience

**Token substitution between flows.** One signing key issues the access,
refresh, websocket, login-challenge, 2FA-pending and password-reset tokens.
Every token now carries a `purpose` claim and every verifier requires the one it
expects:

- `internal/app/token/config.go` — the `Purpose*` constants
- `internal/app/token/gen.go` — `GenerateTokenFor`
- `internal/app/token/verify.go` — `VerifyTokenFor`
- `realtime/lib/realtime/auth.ex` — the socket accepts `purpose: "ws"` and nothing else
- `internal/app/token/purpose_test.go` — a token minted for any one purpose is refused for every other

### 2.4.1 Sensitive account modifications require re-authentication

**Status: meets the requirement.**

Two-stage sign-in exists and the intermediate token is not a session: the
challenge token issued after a password has no `sessions` row, so it is refused
by `ValidateAccessToken` (`internal/app/token/verify.go:51-66`) and can only be
spent at `LoginConfirm`. The same holds for the 2FA pending token.

Already re-verified before the change:

| Action | What it asks for | Evidence |
|---|---|---|
| Change password | Current password | `internal/app/auth/reset_password.go:203` |
| Disable 2FA | Current TOTP or recovery code | `internal/app/twofa/service.go:74` |

Now gated by a fresh confirmation (`RequireFreshAuth`, five-minute window):

| Action | Route |
|---|---|
| Create an API key | `POST /api-keys` |
| Add a passkey | `POST /auth/passkey/register/begin`, `/finish` |
| Remove a passkey | `DELETE /auth/passkey/credentials/:id` |
| Transfer a workspace | `POST /organizations/transfer-ownership` |
| Schedule workspace deletion | `POST /organizations/current/danger-zone/delete` |
| Schedule account deletion | `POST /me/danger-zone/delete` |

- `internal/api/middleware/fresh_auth.go` — the gate. It applies to session callers. An API key and an OAuth token have no session and no second factor to present; each was itself minted from a confirmed session, its use is audited, and the route's permission gate governs it, so they pass through. The threat this addresses is a browser token lifted from an unattended machine, and refusing automation would exceed what the control asks for
- `internal/api/handler/reauth.go` — `POST /v1/auth/reauth`, accepting a password or a current TOTP or recovery code
- `internal/app/token/reauth.go` — `ReauthWindow`, the stamp, and the cache bust
- `internal/app/auth/cache.go` — a per-account budget on confirmations, because this endpoint checks a password and would otherwise be a second, unthrottled place to guess one
- migration `000184` — `sessions.reauth_at`
- `web/src/components/app/modals/ReauthModal.tsx` and `web/src/lib/api/client/Request.ts` — the client prompts and retries automatically

Workspace and account deletion additionally require typing the exact name, and
are delayed and cancellable (`internal/api/handler/danger_zone.go`).

An account created through Google, Apple or enterprise SSO may hold neither a
password nor an enrolled authenticator. There is nothing for it to confirm with,
and accepting the live session instead would turn a stolen token into a
permanent API key, so the endpoint refuses with `reauth_no_factor` and names the
remedy: enrol two-factor authentication, which is not itself gated. This is a
deliberate choice of friction over a silent hole, and it leaves such accounts
with a recovery factor they did not have before.

---

## 3 Access Control

### 3.1.1, 3.1.2, 3.1.3 Least privilege on a trusted service layer

**Status: meets the requirement.** One written description covers all three, as
CASA allows.

**Where access control is decided.** Entirely server-side, in middleware ahead of
every handler (`internal/api/routes.go`, `internal/api/middleware/`). The
frontend hides what a role cannot use, but hiding is not the control: every
route re-derives the caller's identity, workspace and permissions from the
database on each request.

**Identity.** `AuthMiddleware` (`auth.go`) validates the bearer token against the
`sessions` row. `CombinedAuthMiddleware` (`apikey.go`) additionally accepts an
API key or an OAuth access token, each resolved to its organization and
permission mask.

**Workspace context.** Taken from the session's `current_organization_id`, the
API key's organization, or the OAuth grant. It is never taken from a request
body. Where a route carries an organization id in its path, membership is
verified before the handler runs (`internal/api/middleware/organization.go`
`RequireMembership`, and `requireMember` in
`internal/app/organization/service.go` for the routes whose path parameter the
middleware does not match).

**Roles and permissions.** A uint16 bitmask per member
(`internal/models/organization_permission.go`), with seeded Admin, Manager and
Viewer roles plus custom roles. The owner is a flag on the organization, not a
role, and always holds every permission.

**No self-elevation.** `UpdateMemberRole`
(`internal/app/organization/service.go`) refuses to re-role yourself, refuses to
touch the owner, and requires the actor to already hold every permission being
granted. Ownership transfer now additionally requires the actor to be the owner.
On the platform-admin side, `GrantAdminPermissions`
(`internal/app/admin/service.go`) refuses to grant a bit the granter does not
hold, and `RevokeAdminPermissions` refuses to remove the last super admin.

**Fail closed.** Every middleware aborts on error rather than continuing: a
database or cache failure produces 401, 403 or 500 and the handler never runs.
The membership helper is the explicit form of this, because
`GetMembership` answers `(nil, nil)` for a non-member and a caller that only
tests the error would let everyone through.

**Least privilege in the architecture, not just the API.** A worker holds no
database credential and no cloud credential: it reaches relational data through
the internal API and gets key and blob operations brokered
(`internal/api/handler/internal_dek.go`, `internal_blobs.go`), with the blob
presigner restricted to three key prefixes.

### 3.1.4 Insecure Direct Object Reference

**Status: meets the requirement.**

**The pattern.** Every repository method that reads or writes a tenant-owned row
takes the organization id as a parameter and filters on it in SQL. The
identifier from the request is never the only predicate. For example
`internal/repository/pg_contact.go` — `WHERE id = $1 AND organization_id = $2`
on get, update and delete; bulk operations use `id = ANY($1) AND organization_id = $2`.

**APIs that accept a caller-supplied identifier.** Path parameters on every
resource (campaigns, contacts, mailboxes, sequences, automations, templates, API
keys, webhooks, members, threads, forms, CRM records), plus body identifiers on
create and update. The full route list is
`docs/content/docs/api/endpoints.mdx`.

**How they are protected.**

1. Path identifiers are scoped in the query, as above.
2. Body identifiers that reference another row are verified to belong to the
   caller's workspace before being stored. `internal/repository/pg_crm.go`
   `verifyRefs` does this for every reference a deal, task or note can carry.
3. Read-side joins carry the tenant predicate too, so a row that somehow holds a
   foreign reference still discloses nothing (`pg_crm.go`, the `SearchDeals`
   joins).
4. A foreign identifier answers 404, never 403, so a prober cannot tell an id
   that exists elsewhere from one that does not exist.
5. Public object references are unguessable capabilities rather than sequential
   ids: form public ids are 105 bits, unsubscribe tokens are HMAC-signed or
   128-bit random, tracked links and tracking tickets are UUIDv4.

A review of every handler and repository method against this pattern was
carried out for this assessment, and the findings were remediated. The change
log is supplied to the lab separately, for the reason given at the top of this
document.

### 3.1.5 Anti-CSRF

**Status: meets the requirement.** Evidence: Burp scan.

Warmbly sets no cookies and uses no ambient browser credential, so a
cross-site request carries no authority: the bearer token has to be attached by
JavaScript that the attacker's origin cannot run. That is the primary control
and it is structural.

Supporting controls:

- CORS is an explicit origin allowlist with credentials, or wildcard without credentials, never both (`internal/api/routes.go`, `internal/api/cors.go`)
- `X-Frame-Options: DENY` and `frame-ancestors 'none'` on the API, and on the dashboard and admin panel (`internal/api/middleware/security_headers.go`, `web/nginx-security-headers.conf`, `web/public/_headers`)
- unauthenticated state-changing endpoints carry anti-automation: Turnstile on registration, login and password reset; honeypot, timing trap and per-IP budget on public form submission

### 3.1.6 Directory browsing disabled

**Status: meets the requirement.** Evidence: Burp scan.

No component serves a directory index. The forms service uses gin's `Static`,
which wraps the filesystem so `Readdir` returns nothing
(`internal/formserver/server.go`). The nginx images set no `autoindex`, so the
default off applies (`web/nginx.conf`, `admin/nginx.conf`,
`deploy/nginx/warmbly.conf`). The Rust and Elixir services register fixed routes
and mount no static tree. The backend's `/public` route serves a single object
per request, restricted to four key prefixes, and now refuses a key naming a
directory (`internal/infrastructure/storage/filesystem.go`).

### 3.2.1 and 3.2.2 OAuth

**Status: meets the requirement.** Full detail in `oauth.md`, including the
exact Google scopes.

Summary: every integration uses the authorization code flow. The implicit and
resource-owner-password grants are not implemented anywhere, as a client or as a
server. PKCE is used on every flow that supports it, including the Gmail and
Microsoft mailbox flows. `redirect_uri` is a fixed server-side value derived
from configuration, never taken from the request. `state` is 128 to 256 bits
from `crypto/rand`, stored server-side, bound to the user who started the flow,
and consumed atomically with `GETDEL`.

### 3.3.1 Administrative interfaces use multi-factor authentication

**Status: meets the requirement.**

The admin panel is the only application-exposed administrative interface. It is
limited to application-layer functions and exposes no cloud infrastructure:
there is no install, restart, shell, logs or reboot action anywhere in it.

MFA is enforced, not merely available:

- `internal/api/middleware/admin.go` — `AdminMiddleware` refuses any session that did not present a second factor, with code `admin_mfa_required`
- migration `000182` — `sessions.mfa_verified`
- `internal/app/token/gen.go` — `GenerateMFASession`, the only way the flag is set
- `internal/app/twofa/login.go` and `internal/app/passkey/login.go` — the only two callers: a TOTP or recovery code, or a passkey
- `admin/src/components/layout/RequireAdmin.tsx` — explains the refusal and points at where to enrol

The check is on the session rather than on enrolment, so an admin who turns 2FA
on does not silently keep a single-factor session. Existing sessions default to
not verified, which is the safe direction. It is not configurable.

A passkey counts as multi-factor: the credential never leaves the device and the
platform unlocks it with a biometric or PIN.

`warmblyctl` is the operator's other surface. It talks to the database directly,
serves no HTTP, and is not internet-exposed; access to it is access to the
database.

---

## 4 Communications

### 4.1.1 TLS enforced, 1.2 or above, strong ciphers

**Status: meets the requirement.** Evidence: Qualys SSL Labs report per hostname
in `artifacts/`.

TLS is terminated at the edge on every deployment shape: Railway and Cloudflare
for the hosted service, bundled Caddy 2 with automatic HTTPS for the
one-command self-host, nginx with Let's Encrypt for bare metal. All three
default to TLS 1.2 and 1.3 with modern cipher suites.

HSTS is now emitted by every layer: `internal/api/middleware/security_headers.go`
(on a request the edge reports as HTTPS), the rendered Caddyfile in
`site/public/install.sh`, `deploy/nginx/warmbly.conf`,
`web/nginx-security-headers.conf`, and the Cloudflare Pages `_headers` files.

Outbound TLS is enforced rather than assumed. Connections to a customer's
mailbox provider require TLS or STARTTLS
(`internal/client/smtpimap/smtp/client.go`, `internal/client/smtpimap/imap/client.go`,
both with `MinVersion: tls.VersionTLS12`); cleartext is possible only to a
loopback peer on a self-hosted instance, checked twice, once on the hostname and
once on the actual socket (`internal/client/netbind/netbind.go`). Outbound
webhooks require HTTPS and a public address unless the operator explicitly opts
out (`internal/app/webhook/service.go`).

### 4.1.2 Trusted TLS certificates

**Status: meets the requirement.** Evidence: the same Qualys reports.

Certificates are issued by Let's Encrypt or the platform edge. No self-signed
certificate is trusted anywhere in the default configuration.
`InsecureSkipVerify` appears only behind an operator-set development flag
(`MAIL_TLS_INSECURE`, `internal/client/netbind/netbind.go`) and in the local
sandbox package, and the installer pins it to false.

### 4.1.3 No weak cryptography

**Status: meets the requirement.** Full inventory in `crypto-inventory.md`,
covering every encryption, hashing and MAC operation with algorithm, key size,
key and IV generation, and key management.

Summary: AES-256-GCM for everything encrypted at rest, with 12-byte nonces from
`crypto/rand`; Argon2id for passwords and codes; SHA-256 for token lookup;
HMAC-SHA256 for webhook and link signatures; HS256 with a key of at least 32
bytes for sessions; RS256 with JWKS for third-party ID tokens; ES256 for APNs.

No MD5, DES, RC4 or CBC anywhere. SHA-1 appears only inside RFC 6238 TOTP, where
the specification requires it.

### 4.1.4 Cryptographic modules fail securely

**Status: meets the requirement.**

Every symmetric operation is AES-GCM. There is no CBC and no padded mode
anywhere, so a padding oracle has nothing to attack: an authentication failure
is indistinguishable from any other decryption failure.

Failures are opaque to the caller. `internal/app/cipher/decrypt.go` returns a
generic error; callers report it to the error tracker and answer with a generic
message. The data-key broker returns one fixed sentence regardless of cause
(`internal/api/handler/internal_dek.go`), deliberately, so a prober cannot tell
"not one of our keys" from "malformed". As of this assessment, every
internal-class error answers with one fixed sentence and logs the detail against
the request id (`internal/errx/errx.go` `clientMessage`).

Comparisons on attacker-supplied secrets are constant-time throughout:
`subtle.ConstantTimeCompare` for internal tokens, join tokens and Argon2 output;
`hmac.Equal` for every HMAC.

---

## 5 Data Validation and Sanitization

Test cases 5.1.1 through 5.1.10 are validated by the authenticated Burp scan.
The written controls below describe what the scan is expected to confirm.

### 5.1.1 HTTP parameter pollution

Query parameters are parsed first-value-wins by gin, matching Go's `net/url` and
the proxies in front of it, so there is no front-end/back-end disagreement to
exploit. Only one route reads a multi-value parameter and it allowlists each
element (`internal/api/handler/advisor.go`). No authentication material is read
from the query string.

### 5.1.2 URL redirects and forwards

Redirects are allowlisted or server-derived in every case:

- OAuth callbacks return to a fixed URL built from configuration
- the login `next` parameter must be a same-origin relative path (`web/src/app/auth/login/page.tsx`)
- OAuth client `redirect_uri` is matched by exact string against the registered list, with dangerous schemes rejected at registration (`internal/app/oauth/flow.go`, `internal/app/oauth/dcr.go`)
- Stripe checkout and portal return URLs are now pinned to this instance's dashboard origin rather than taken from the request (`internal/api/handler/billing_return_url.go`)
- the pool-link return URL is pinned to the registered instance's own host

The click-tracking redirect is an intentional redirector: the destination is read
from a database row minted when the email was sent, never from the request, and
the scheme is validated to http or https at mint time on the decoded href
(`internal/tasks/links.go`). The URL carries a ticket id and no destination, so
there is no open-redirect parameter to manipulate.

### 5.1.3 and 5.1.4 Dynamic code execution and template injection

No scripting engine is embedded: no `eval`, no `new Function`, no JavaScript VM,
no Lua, no Starlark. The frontend contains no `eval` or `new Function`.

The expression and merge-field engines use Go `text/template` over a data object
that is a `map[string]string` of contact fields, with a 20-function allowlist of
pure string and arithmetic helpers (`internal/pkg/tmplfuncs/tmplfuncs.go`). No
struct pointer, service handle or `io` value is ever placed in the data, so
there is nothing to pivot to. Spintax is a bounded regex expander with a
20-iteration cap (`internal/tasks/spintax.go`).

All transactional email templates use `html/template`, which escapes
contextually.

### 5.1.5 Server-Side Request Forgery

Every outbound request whose URL a user can influence goes through
`internal/pkg/safehttp`. The guard runs at the dialer, not on the hostname
string, which is what defeats DNS rebinding:

- resolution happens inside the dialer, and the validated address is the one dialled
- a mixed result set fails closed: one private answer blocks the request
- blocked ranges include loopback, RFC1918, IPv6 unique-local, link-local including 169.254.169.254, CGNAT, multicast, IPv4-mapped IPv6 and the reserved ranges
- a pre-resolution hostname denylist covers `localhost` and the cloud metadata names, closing split-horizon DNS
- ports are restricted to 443 and 8443
- redirects are re-validated per hop, capped at five; the webhook worker refuses redirects entirely

Covered surfaces: customer webhooks, webhook verification, user-entered MCP
server URLs, integration actions, AI page fetching, operator notification
channels, and OAuth app webhook URLs. As of this assessment the email
verification MX probe also refuses a non-public address
(`internal/pkg/emailverify/emailverify.go`).

User-entered IMAP and SMTP hosts are an arbitrary connection by product
necessity. The compensating control is socket-level: after connecting, the peer
address is checked, so a rebinding answer does not help
(`internal/client/netbind/netbind.go`).

### 5.1.6 XPath and XML injection

Not applicable, and confirmed structurally: no Go file imports `encoding/xml`.
There is no XPath library, no SAML, and no feed parsing. XXE is impossible.

XLSX import is the only XML-derived input. It is handled by `excelize`, which
processes OOXML internally without DTD support, and is now bounded against
decompression bombs (512 MiB total, 64 MiB per part) with streaming row reads
and a recover around the parser (`internal/app/contact/import.go`).

### 5.1.7 Cross-site scripting

React escapes by default and there are exactly two
`dangerouslySetInnerHTML` uses in the dashboard, both rendering a compile-time
SVG path map.

Inbound email HTML is the largest untrusted surface. It is sanitized with
`bluemonday` on an allowlist policy that drops `script`, `style`, `iframe`,
`object`, `embed` and `applet` with their content, permits only http, https,
mailto and tel URL schemes, and permits data URIs only for raster images, so
neither `data:text/html` nor SVG survives (`internal/pkg/mailhtml/mailhtml.go`).
It is then rendered in an iframe without `allow-scripts`
(`web/src/components/app/unibox/EmailBody.tsx`).

The mailbox signature editor now sanitizes with DOMPurify on assignment rather
than inspecting with a regex (`web/src/components/app/EmailEditor.tsx`).

Security headers are set on every surface:
`internal/api/middleware/security_headers.go` for the API,
`web/nginx-security-headers.conf` and `web/public/_headers` for the dashboard,
the equivalents for the admin panel and marketing site, and per-form
`frame-ancestors` for the forms service.

The forms default is deliberately unchanged. A form with no configured embed
allowlist may be framed anywhere, which is the documented contract and what
every embed installed without configuring the list depends on; narrowing it
would have taken those forms off their owners' websites with no error anywhere.
What changed is that the permissive case now states `frame-ancestors *`
explicitly instead of sending no header, so the policy is legible to a scanner
and to a reader rather than being an absence. Restricting framing is offered as
a per-form setting, and the documentation now recommends using it.

### 5.1.8 Database injection

The repository layer uses pgx with `$n` placeholders throughout, and
`internal/repository/query_prepare_live_test.go` prepares every query in the
package against a live server.

Where SQL is built dynamically, identifiers come from static allowlists, never
from the request. The segment filter engine, which compiles customer-authored
JSON into SQL, resolves every field through a static catalog and four literal
column maps, matches operators against constants with a `FALSE` default, binds
every value, binds JSONB keys rather than interpolating them, and escapes LIKE
metacharacters (`internal/repository/pg_segment_sql.go`). All eleven dynamic
`ORDER BY` builders allowlist the column.

Sort keys that reach an `ORDER BY` are bound as parameters rather than
interpolated, and validated at write time against the same rule every other
custom-field key answers to.

Org export and import triple-guard their identifiers: table names come from a
compiled registry, column names are filtered against the destination's live
catalog, and every identifier passes `pgx.Identifier.Sanitize`.

### 5.1.9 OS command injection

No production HTTP service invokes a subprocess. `cmd/backend`, `cmd/worker`,
`cmd/consumer`, `internal/api`, `internal/app` and `internal/formserver` do not
import `os/exec`. The Rust and Elixir services invoke nothing.

The only request-reachable path is the operator update action, which is behind
the admin bit and MFA, validates the tag against a strict character allowlist
before use, and never invokes a shell (`internal/updater/image.go`).

### 5.1.10 Local and remote file inclusion

No `http.ServeFile`, `c.File` or raw `http.Dir` in any HTTP surface. Blob keys
are cleaned and `..` is rejected as a literal path component before
normalization (`internal/infrastructure/storage/filesystem.go`); the public
object route and the node presigner both reject `..` before testing the prefix,
which is the ordering that makes the check correct
(`internal/api/handler/public_object.go`, `internal_blobs.go`).

### 5.2.1 Untrusted file uploads

**Status: meets the requirement.**

| Upload | Limit | Type validation | Stored as |
|---|---|---|---|
| Campaign attachment | 15 MiB | Extension denylist, sniffed type | `attachments/<campaign>/<uuid>-<name>`, presigned download only |
| Email image | 5 MiB | Magic-byte allowlist, real image decode, dimension cap | `email-images/<org>/<uuid>.<ext>` |
| Avatar | 2 MiB | Magic-byte allowlist, real decode, dimension cap | `avatars/<kind>/<id>-<epoch>-<nonce>.<ext>` |
| Form asset | 1 to 4 MiB | Magic-byte allowlist, real decode | `form-assets/<org>/<id>-<kind>-<ts>.<ext>` |
| Contact import | 50 MiB | Extension, then bounded parse | Parsed in memory, never stored |
| Workspace archive | 8 GiB | Zip structure and manifest | Parsed, then discarded |

The controls that matter for execution:

- every image path forces the extension from a server-side allowlist, so the stored key cannot end in `.svg` or `.html`
- `/public` derives Content-Type from the key's extension, serves only that image allowlist inline, hands anything else over as `application/octet-stream` with `Content-Disposition: attachment`, and sets `X-Content-Type-Options: nosniff` plus a sandboxing content security policy (`internal/api/handler/public_object.go`)
- attachments are never served from a Warmbly origin; they are fetched by a 15-minute presigned URL
- the workspace-import path validates every object key against the shapes this product mints, forces the image extension under public prefixes, and rewrites the workspace segment of a public key to the importing workspace rather than taking it from the archive, so an archive cannot address another workspace's prefix (`internal/app/orgtransfer/blobkey.go`)

There is no antivirus scanning. Campaign attachments are relayed to external
recipients as the customer supplied them, which is the same posture as any mail
client; recipient-side scanning is the control. This is stated rather than
claimed otherwise.

---

## 6 Configuration

### 6.1.1 No components with known exploitable vulnerabilities

**Status: meets the requirement.** Evidence: `artifacts/` scan output.

Scanners run in CI (`.github/workflows/security.yml`): `govulncheck` for Go
including the standard library, Trivy for the filesystem, `pnpm audit --prod`
per frontend tree, and `cargo audit` for the Rust service.

At the commit above:

| Tree | Result |
|---|---|
| Go | No advisory at CVSS 7.0 or above with a fix available. Four remain with no upstream fix; each is justified below |
| web, admin, site, forms | No high or critical advisory in production dependencies |
| docs | No high or critical advisory in production dependencies |
| tracking (Rust) | Clean |
| realtime (Elixir) | Two cowlib advisories, both below CVSS 7.0 |

**Justified, no upstream fix available.** CASA permits this where the library
has a regular patch process:

| Advisory | Component | Why it does not apply |
|---|---|---|
| GO-2026-6452 | `xuri/excelize` | A panic on a crafted shared-string index. Reachable only through contact import, which requires authentication and the `manage_contacts` permission. The parser now runs under a recover and answers 400, and decompression is bounded, so the worst case is a rejected upload |
| GO-2026-5046, 5047, 5048 | `hamba/avro` | Decoder advisories. The decoder is compiled only into the `-kafka` image variant, behind a build tag, and decodes messages from Warmbly's own internal bus, never attacker input. The default images contain no Avro decoder at all |

Both projects patch regularly; these will be picked up when they do.

### 6.2.1 Debug modes disabled in production

**Status: meets the requirement.** Evidence: Burp scan, plus:

- gin runs in release mode in every shipped configuration (`docker-compose.yml`, `deploy/config/env.example`, `site/public/install.sh`), set explicitly by every service rather than inherited
- no Go file imports `net/http/pprof`; there is no `/debug` route
- panics are reported to the error tracker with their stack and answered with a bare 500 carrying no body (`internal/api/middleware/reporting.go`)
- a 500 response now carries one fixed sentence; the detail is logged against the request id (`internal/errx/errx.go`)
- the Phoenix production config does not print connection details on a failure (`realtime/config/runtime.exs`)
- source maps are generated only when they are being uploaded to the error tracker, and deleted after upload (`web/vite.config.ts`, `web/package.json`)
- health endpoints return a status literal and nothing else

### 6.3.1 The Origin header is not used for access control

**Status: meets the requirement.** Evidence: Burp scan.

Nothing authenticates or authorizes on `Origin`, `Referer` or `Host`. A grep for
those headers finds only CORS configuration and the per-form embed policy.

CORS never reflects an arbitrary origin with credentials: the wildcard branch
sets `AllowCredentials: false` explicitly, and the allowlist branch enumerates
origins from configuration (`internal/api/routes.go`). Private-network origin
reflection exists only outside release mode, for local development.

The per-form `frame-ancestors` allowlist is a browser containment directive in a
response header, not a server-side grant derived from a request header. It
grants nobody anything, and the form is equally reachable by direct navigation.

### 6.4.1 Subdomain takeover

**Status: meets the requirement.** Evidence: DNS export in `artifacts/`, to be
attached at submission.

The hostname inventory is in `scope.md`. Every record points at infrastructure
Warmbly controls or at a platform Warmbly holds the account for.

Customer-owned custom domains are the interesting case: a customer points a
CNAME at Warmbly for tracking or forms. A certificate is issued for such a name
only after the instance has verified it, checked on every request by
`GET /tls/authorize` (`internal/api/handler/tls_authorize.go`), which requires a
verified row, fails closed on a database error, and does not cache the failure.
Background sweeps re-verify and clear a record that stops resolving.

### 6.5.1 No credentials or payment details in logs

**Status: meets the requirement.** Evidence: `artifacts/log-sample-login.txt`.

- the request logger records method, path, status, latency and client IP, and not the query string, which on some routes carries a single-use token placed there by a provider or a mail client (`internal/api/middleware/request_log.go`; the forms service uses the same logger)
- no password, token or secret is logged in any auth code path
- API keys are stored and looked up as SHA-256; the usage log records the key id, never the key
- Warmbly never receives payment details: Stripe Checkout and the billing portal are hosted by Stripe, and no card field exists in this repository
- Sentry's `SendDefaultPII` is off in all three initializations, so request headers, cookies and bodies are not attached
- session replay masks password inputs and every one-time code entry field; console capture is off, and no token is written to the console

A login request produces one line of the shape recorded in
`artifacts/log-sample-login.txt`.

### 6.6.1 Browser storage cleared at logout

**Status: meets the requirement.**

The dashboard and admin panel store the token pair, a persisted workspace
selection and UI preferences, and reply drafts. There is no IndexedDB, no
service worker, and no persisted query cache.

One teardown is used by logout and by every path that discovers the session is
gone, so being signed out clears the same things as signing out
(`web/src/lib/session.ts` `clearClientSession`, called from
`web/src/lib/api/hooks/auth/useLogout.ts` and `web/src/hooks/UserProvider.tsx`).
It clears the tokens, the reply drafts, the SSO binding, the persisted store and
the query cache.

Reply drafts hold the body, subject and recipients of an unsent email, so they
are cleared alongside the tokens (`web/src/lib/auth.ts`, the prefix sweep in
`clearTokens`).

### 6.7.1 Server-side secrets stored securely

**Status: meets the requirement.**

**How secrets reach the application.** Environment variables, from the platform's
secret store on the hosted deployment and from a `.env` file on a self-host. The
installer creates that file with `umask 077` before writing to it, so it is 0600
from the moment it exists and never briefly world-readable
(`site/public/install.sh`). AWS Secrets Manager and SSM Parameter Store are
supported as alternative sources (`internal/config`).

**Refusal of published defaults.** The backend will not start when one of the
five published development secrets is still in use
(`cmd/backend/boot.go`), and `AUTH_SECRET` must be at least 32 bytes.

**Customer secrets at rest.**

| Secret | Protection |
|---|---|
| Mailbox SMTP and IMAP credentials | AES-256-GCM under the instance credential key |
| Mailbox OAuth tokens | AES-256-GCM under the instance credential key |
| Integration and MCP tokens | AES-256-GCM under the per-organization data key |
| API keys | SHA-256, irreversible, shown once |
| Webhook signing secrets | AES-256-GCM under the instance credential key. A workspace archive carries the secret only when exported with credentials, and the export and import guide says to rotate after a move |
| TOTP secrets | AES-256-GCM under a dedicated key |

**Access control.** Workers hold no cloud credential: key decryption and blob
signing are brokered by the control plane, on a separate token from the one the
internet-facing services carry, and the presigner is restricted to three key
prefixes (`internal/api/handler/internal_dek.go`, `internal_blobs.go`,
`internal/api/middleware/internal_auth.go`). The admin panel cannot reveal a
secret: the configuration view redacts by name marker and returns a four-character
fingerprint instead of a value (`internal/app/instanceconfig/redact.go`).

**Monitoring.** Every mutation that touches a secret is written to the audit log
with actor, action and target. As of this assessment the two endpoints that hand
out key material also log every access and every refusal, which is the clearest
probe signal the instance produces (`internal/api/handler/broker_access_log.go`).

**No secrets in the repository.** A scan for AWS keys, Stripe live keys, private
key blocks and platform tokens across the tree and its history returns nothing.
The only committed `.env` files are examples with placeholders.
