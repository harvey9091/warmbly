# Assessment scope

## The application

Warmbly is an email warmup and cold outreach platform. A customer connects their
own mailboxes, and Warmbly sends, syncs and tracks mail through them. The Google
OAuth client under assessment, `warmbly-mailboxes`, is what connects a Gmail or
Google Workspace mailbox.

## First-party components in scope

Everything below is one system behind one authentication and authorization
model, so all of it is in scope.

| Component | Path | Role |
|---|---|---|
| Backend API | `cmd/backend`, `internal/api`, `internal/app` | The control plane. Authentication, authorization, every customer-facing endpoint |
| Consumer | `cmd/consumer` | Processes bus events and updates platform state |
| Worker | `cmd/worker` | Sends and syncs mail. Holds no database credential and no cloud credential |
| Forms service | `cmd/forms`, `internal/formserver`, `forms/` | Public lead-capture form pages on their own origin |
| Tracking service | `tracking/` (Rust) | Open and click tracking, unsubscribe forwarding |
| Realtime service | `realtime/` (Elixir) | WebSocket fan-out to signed-in dashboards |
| Dashboard | `web/` | The customer-facing single-page app |
| Admin panel | `admin/` | The operator surface. Platform-admin bit plus MFA on every route |
| Marketing site | `site/` | Static. Also serves `install.sh` and `cli.sh` |

## Hostnames

The Qualys SSL Labs scans and the DNS review for test cases 4.1.1, 4.1.2 and
6.4.1 cover:

| Hostname | Serves |
|---|---|
| `warmbly.com` | Marketing site, `install.sh`, `cli.sh` |
| `app.warmbly.com` | Dashboard |
| `api.warmbly.com` | Backend API, `/public` objects, OAuth callbacks |
| `admin.warmbly.com` | Admin panel |
| `docs.warmbly.com` | Documentation |
| `forms.warmbly.com` | Public form pages |
| The tracking host | Open and click tracking |
| The realtime host | WebSocket gateway |

Customer-owned custom tracking and forms domains point at Warmbly by CNAME.
Certificates for those are issued on demand only after the instance has verified
the name, gated by `GET /tls/authorize`
(`internal/api/handler/tls_authorize.go`).

## Third-party services in scope

CASA puts a third-party API in scope when it performs authentication, or reads
or mutates user data. These qualify, and are covered under sections 1, 2 and 3
only:

| Service | What it does | Flow |
|---|---|---|
| Google Sign-In | Authenticates a person into Warmbly | Authorization code with PKCE and nonce |
| Apple Sign-In | Authenticates a person into Warmbly | Authorization code with state and nonce |
| Google (Gmail API) | Reads and sends a customer's mail | Authorization code with PKCE. The client under assessment |
| Microsoft Graph | Reads and sends a customer's mail | Authorization code with PKCE |
| Enterprise OIDC | Authenticates a person into a self-hosted instance | Discovery, authorization code with PKCE and nonce |

Warmbly is also an OAuth 2.1 authorization server for third-party apps and MCP
clients (`internal/app/oauth`). That is first-party code and is assessed as part
of the backend.

## Out of scope

- **Stripe.** Billing only. Warmbly never sees a card number: Checkout and the
  billing portal are hosted by Stripe, and no PAN, CVV or expiry field exists
  anywhere in this repository.
- **PostHog and Sentry.** Product analytics and error reporting. Neither
  authenticates anyone nor holds customer mail.
- **AWS KMS, S3, SES; Cloudflare; Railway.** Infrastructure the application runs
  on. In scope for how Warmbly configures and uses them, which sections 4 and 6
  cover, not as separately assessed products.
- **Recipients' mail servers.** Warmbly authenticates to a customer's own
  mailbox provider; it never connects to a recipient's MX.

## What the Burp scan has to reach

The DAST test cases require an authenticated scan. A scan that only sees the
signed-out surface proves nothing about them. The scan should be run against a
staging instance with a seeded workspace, authenticated as a workspace owner,
and should reach at least:

- the full dashboard under `/app`, including campaigns, contacts, the unified
  inbox, forms, automations and settings
- the REST API under `/v1` with a bearer token, including the list and detail
  endpoints for every resource in `docs/content/docs/api/endpoints.mdx`
- the public form pages on the forms origin
- the unsubscribe and tracking endpoints on the tracking origin

Authentication for the scan: sign in with a password, complete the emailed code,
and attach the resulting bearer token to scan requests. The token lives 12 hours,
which is long enough for a full crawl and audit.
