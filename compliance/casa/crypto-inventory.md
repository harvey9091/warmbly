# Cryptographic inventory

For CASA test case 4.1.3, which asks for every encryption, hashing and MAC
operation with its algorithm, key size, key and IV generation, and key
management.

## Encryption at rest

Warmbly uses envelope encryption. AWS KMS, or a local master key on a
self-hosted instance, is the root of trust; each organization gets its own data
encryption key; the plaintext key is never stored.

| Operation | Algorithm | Key size | Key generation | IV / nonce | Storage and rotation | Evidence |
|---|---|---|---|---|---|---|
| Organization data key, wrapped | AWS KMS `GenerateDataKey` | 256-bit | KMS | KMS | Wrapped key base64 in `organization_encrypted_keys`; the CMK rotates on AWS's schedule and old ciphertexts keep decrypting | `internal/infrastructure/kms/encryption.go`, `decryption.go` |
| Organization data key, self-host | AES-256-GCM | 256-bit master | Master from `KMS_LOCAL_MASTER_KEY`, length-checked at boot; data key from `crypto/rand` | 12 bytes, `crypto/rand` | Master in environment or file, memory only | `internal/infrastructure/kms/local.go` |
| Organization data key, on a node | None held | n/a | n/a | n/a | A node holds no key material; it posts sealed keys to the control plane and gets plaintext back | `internal/infrastructure/kms/brokered.go` |
| Message bodies, integration and MCP tokens | AES-256-GCM, nonce prefixed, base64 | 256-bit data key | Per organization, above | 12 bytes, `crypto/rand` | Plaintext key cached in Redis for 15 minutes | `internal/app/cipher/encrypt.go`, `decrypt.go`, `cache.go` |
| Mailbox SMTP and IMAP credentials, mailbox OAuth tokens, webhook signing secrets | AES-256-GCM, nonce prefixed, hex | 256-bit `CREDENTIALS_ENCRYPTION_KEY` | Operator-supplied, 64 hex characters, validated at boot | 12 bytes, `crypto/rand` | Environment variable; writes fail closed without it | `internal/pkg/encrypt/encrypter.go`, `internal/repository/pg_email.go`, `pg_webhook.go` |
| TOTP secrets | AES-256-GCM | 256-bit, SHA-256 of `TWOFA_SECRET` | Operator-supplied, falls back to `AUTH_SECRET` | 12 bytes, `crypto/rand` | Rotating it invalidates enrolled authenticators, which is documented | `internal/app/twofa/seal.go` |
| Workspace export archives | Argon2id then AES-256-GCM | 256-bit derived | Argon2id t=4, m=256 MiB, p=4 from the passphrase | 16-byte salt, 12-byte nonce, `crypto/rand` | The passphrase is never stored; parameters are clamped on import | `internal/app/orgtransfer/seal.go` |

## Password and code hashing

| Operation | Algorithm | Parameters | Salt | Evidence |
|---|---|---|---|---|
| Account passwords | Argon2id | m=64 MiB, t=3, p=2, 32-byte output | 16 bytes, `crypto/rand` | `internal/pkg/argon2/config.go`, `hash.go` |
| Emailed login and registration codes | Argon2id | same | same | `internal/app/auth/login.go`, `registration.go` |
| 2FA recovery codes | Argon2id | same | same | `internal/app/twofa/recovery.go` |

Argon2id is in the approved list in NIST SP 800-63B 5.1.1.2, and these
parameters exceed the OWASP minimum. Verification is constant-time
(`internal/pkg/argon2/verify.go`).

## Token hashing

These are high-entropy random values, so an unsalted SHA-256 is the correct
construction: there is nothing to brute-force and the lookup has to be by exact
value.

| Token | Bits of entropy | Stored as | Evidence |
|---|---|---|---|
| API key | 256 | SHA-256 hex | `internal/app/apikey/service.go` |
| OAuth authorization code, access token, refresh token | 256 each | SHA-256 | `internal/app/oauth/service.go` |
| CLI device code | 256 | SHA-256 | `internal/app/cliauth/service.go` |
| Fleet join token | 256 | SHA-256, constant-time compare | `internal/app/fleetnode/service.go` |
| First-run setup token | 256 | SHA-256 | `internal/app/bootstrap/bootstrap.go` |

## Message authentication

| Operation | Algorithm | Key | Comparison | Evidence |
|---|---|---|---|---|
| Outbound webhook signature | HMAC-SHA256 over `<timestamp>.<body>` | 256-bit per endpoint, encrypted at rest | Receiver's | `internal/app/webhook/service.go` |
| Unsubscribe link token | HMAC-SHA256, 128-bit truncated tag | Derived from `AUTH_SECRET` with a purpose string | `hmac.Equal` | `internal/app/unsublink/signer.go` |
| Forms render token | HMAC-SHA256 | SHA-256 of the internal token plus a purpose string | `hmac.Equal` | `internal/formserver/token.go` |
| Inbound GitHub release webhook | HMAC-SHA256 | Operator secret | `hmac.Equal` | `internal/app/releases/service.go` |
| Stripe webhook | Stripe's signature scheme | Stripe secret | Library | `internal/app/stripe/service.go` |

Every key here is domain-separated: a key derived for one purpose cannot verify
another purpose's tag.

## Digital signatures

| Operation | Algorithm | Key | Validation | Evidence |
|---|---|---|---|---|
| Session, refresh, websocket, challenge and reset tokens | HS256 | `AUTH_SECRET`, at least 32 bytes, enforced at boot | Algorithm pinned to HS256, expiry required, purpose claim required | `internal/app/token/gen.go`, `internal/config/config_auth.go` |
| Same tokens, verified by the realtime service | HS256 | The same value as `JWT_SECRET`, same floor | `verify_strict` with an exact algorithm list, purpose required | `realtime/lib/realtime/auth.ex` |
| Google and Apple ID tokens | RS256 | Provider JWKS | Algorithm pinned, issuer and audience checked, expiry required | `internal/pkg/idtoken/idtoken.go` |
| Cloud Tasks caller | RS256 | Google JWKS | Algorithm, issuer, subject and audience checked | `internal/api/middleware/oidc.go` |
| APNs authentication | ES256 (P-256) | Apple `.p8` | Apple's | `internal/infrastructure/apns/client.go` |

## Randomness

Every secret, nonce, salt and token comes from `crypto/rand`:
`internal/pkg/crypt/gen.go` is the shared source. The six-digit verification
code uses `rand.Int` with a bound rather than a modulo, so it is unbiased.

`math/rand` is used only for scheduling jitter, warmup behaviour sampling and
spintax selection, none of which is a security decision.

## Transport

| Connection | Minimum version | Verification |
|---|---|---|
| Inbound HTTPS | 1.2, edge-terminated | Let's Encrypt or platform certificate |
| Outbound SMTP | 1.2 explicit | Full verification; cleartext only to a loopback peer on a self-host, checked on the socket |
| Outbound IMAP | 1.2 (Go default) | Full verification, same loopback exception |
| Outbound HTTP | 1.2 (Go default) | Full verification, through the SSRF-guarded client |
| Postgres | Operator-set `sslmode` | `verify-full` with the RDS bundle in the production template |
| Redis, NATS, Kafka | TLS schemes supported and used across untrusted networks | System roots |

## Algorithms deliberately absent

No MD5, DES, 3DES, RC4, or any CBC mode. SHA-1 appears only inside RFC 6238
TOTP, where the specification requires it and where it is used as an HMAC key
derivation rather than for collision resistance.

## Key management summary

| Key | Where it lives | Rotation |
|---|---|---|
| AWS KMS CMK | KMS, never leaves it | AWS-managed annual rotation; old ciphertexts continue to decrypt |
| `KMS_LOCAL_MASTER_KEY` | Environment or file, memory only | Manual; requires re-wrapping every organization key |
| Organization data key | Wrapped in Postgres, plaintext cached in Redis for 15 minutes | Per organization, on creation |
| `CREDENTIALS_ENCRYPTION_KEY` | Environment | Manual; requires re-sealing stored credentials |
| `AUTH_SECRET` | Environment | Manual; rotating it invalidates every session, which is the intended effect |
| `TWOFA_SECRET` | Environment | Manual; rotating it invalidates enrolled authenticators, documented |
| Webhook signing secret | Encrypted in Postgres | Customer-initiated, per endpoint |
| API key | Hashed in Postgres | Customer-initiated: create new, revoke old |

Rotating the two instance-wide keys currently requires a re-encryption pass that
is not yet automated. Recorded here as a known gap rather than claimed as done.
