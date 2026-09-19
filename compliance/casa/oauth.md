# OAuth integrations

For CASA test cases 3.2.1 (no deprecated flows) and 3.2.2 (`redirect_uri` and
`state` validation).

## Summary

Every integration uses the **authorization code flow**. The **implicit grant**
and the **resource owner password credentials grant** are not implemented
anywhere in this codebase, as a client or as a server. Warmbly's own
authorization server rejects any `grant_type` other than `authorization_code`
and `refresh_token`.

## Warmbly as a client

### The client under assessment: Gmail mailbox access

This is the `warmbly-mailboxes` client.

| Property | Value |
|---|---|
| Flow | Authorization code with PKCE (S256) |
| Authorization endpoint | `https://accounts.google.com/o/oauth2/auth` |
| `redirect_uri` | Fixed server-side: the API's public base plus `/addresses/google/callback`. Never read from the request |
| `state` | 128-bit from `crypto/rand`, stored in Redis with a 10-minute TTL, consumed with `GETDEL`, and bound to the user who started the flow |
| PKCE verifier | Generated at start, stored in the server-side state, never sent to the browser |
| Extra parameters | `access_type=offline`, `prompt=consent` |
| Token storage | AES-256-GCM under the instance credential key |
| Revocation | On mailbox deletion, the refresh token is posted to Google's revoke endpoint |

**Scopes requested, and nothing else:**

| Scope | Why |
|---|---|
| `https://www.googleapis.com/auth/gmail.modify` | Send campaign and warmup mail, read replies, move messages between folders |
| `https://www.googleapis.com/auth/gmail.settings.basic` | Read the send-as addresses Google has verified, and the signature already configured |

Google's Gmail scopes nest, and `gmail.modify` already confers `gmail.readonly`,
`gmail.send`, `gmail.compose` and `gmail.metadata`, so naming those as well would
widen the consent screen and the declared restricted-scope set without granting
anything additional. Mailboxes connected under a broader consent continue to
work: the nesting is resolved in both directions by `scopeSatisfiedBy`
(`internal/app/email/onboarding.go`).

`https://mail.google.com/` is **not** requested. It appears only in the
satisfaction table, as a scope that would confer the ones above if a user had
granted it previously.

**Partial consent is refused.** A consent screen lets a person untick individual
permissions and still returns a token. `checkGrantedScopes`
(`internal/app/email/onboarding.go`) compares what was granted against what was
asked and refuses the connection, naming the missing permission, rather than
storing a mailbox that looks connected and fails days later.

Evidence: `internal/config/inbox.go`, `internal/app/email/onboarding.go`,
`internal/app/email/oauth_params.go`, `internal/app/email/cache.go`.

### Microsoft Graph mailbox access

| Property | Value |
|---|---|
| Flow | Authorization code with PKCE (S256) |
| `redirect_uri` | Fixed server-side: API base plus `/addresses/outlook/callback` |
| `state` | As above: 128-bit, Redis, `GETDEL`, user-bound |
| Scopes | `Mail.Send`, `Mail.ReadWrite`, `User.Read`, `offline_access` |
| Extra parameters | `prompt=select_account`. Deliberately not `prompt=consent`, which causes Entra ID to re-run consent eligibility and refuse non-admin users |

### Google Sign-In (authenticating a person into Warmbly)

| Property | Value |
|---|---|
| Flow | Authorization code with PKCE (S256) and a nonce |
| `redirect_uri` | Fixed server-side, must be absolute, validated at boot |
| `state` | 256-bit from `crypto/rand`, Redis, 10-minute TTL, `GETDEL`, provider-bound |
| ID token | Verified against Google's JWKS: RS256 pinned, issuer allowlist, audience equal to the client id, expiry required, nonce compared |
| Tokens stored | None. Warmbly reads the identity and discards the tokens |

After the callback, a single-use 60-second handoff code is exchanged over POST
with a binding secret the browser held throughout, which defends against login
CSRF (RFC 9700 4.7.1).

Evidence: `internal/app/socialauth/socialauth.go`, `internal/app/auth/sso.go`,
`internal/pkg/idtoken/idtoken.go`.

### Apple Sign-In

Authorization code with `response_mode=form_post`, state and nonce. Apple does
not support PKCE on the web, so the nonce inside the ID token plus a single-use
state are the binding. The ID token is verified against Apple's JWKS with the
issuer and audience pinned. `redirect_uri` is fixed server-side and required to
be HTTPS at boot.

### Enterprise OIDC (self-hosted)

Discovery at boot with the issuer pinned to the configured value, RS256
required, authorization code with PKCE and a nonce, and the same Redis state
machinery. Optional domain allowlist.

Evidence: `internal/app/oidcauth/oidcauth.go`.

### Third-party integrations

HubSpot, Slack, Google Sheets, Pipedrive and Salesforce, each authorization code
with a fixed server-side redirect URI and a 192-bit state stored in Postgres,
consumed atomically by a conditional update, and bound to the user who started
it. Google Sheets and Salesforce use PKCE. Tokens are encrypted under the
per-organization data key.

Evidence: `internal/app/integration/oauth.go`, `service.go`.

**Callback origin.** The bouncer pages that hand an authorization code back to
the dashboard address `postMessage` to this instance's dashboard origin, and
deliver nothing rather than falling back to a wildcard when that origin is
unconfigured (`internal/api/handler/integration.go`,
`internal/api/handler/email_oauth_callback.go`).

## Warmbly as an authorization server

Warmbly implements OAuth 2.1 for third-party apps and MCP clients.

| Property | Value |
|---|---|
| Grants | `authorization_code` and `refresh_token` only. Anything else returns `unsupported_grant_type` |
| PKCE | S256 only; `plain` is rejected. Mandatory for public clients, checked at authorize and again at token exchange |
| `redirect_uri` | Exact string match against the registered list, with no prefix matching. Re-checked at token exchange against the value the code was issued for |
| Registration | HTTPS, loopback HTTP, or a private-use scheme. `javascript:`, `data:`, `vbscript:`, `file:`, `blob:` and `about:` are rejected; opaque URIs are rejected; 12 maximum, 2048 characters each |
| Authorization code | 256-bit, stored SHA-256, 10-minute TTL, consumed by conditional update, bound to the client |
| Access token | 1 hour, hashed at rest |
| Refresh token | 90 days, hashed at rest, rotated on every use |
| Revocation | RFC 7009, and never confirms whether a token existed |
| Dynamic registration | RFC 7591, open but rate limited per IP, and a self-registered client can never request `SEND_CAMPAIGNS` or `API_KEYS` |

Evidence: `internal/app/oauth/flow.go`, `service.go`, `dcr.go`,
`internal/api/handler/oauth.go`.
