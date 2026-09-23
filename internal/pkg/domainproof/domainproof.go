// Package domainproof derives the DNS TXT value a workspace publishes to prove
// it controls a domain. The value is keyed by the instance secret and bound to
// one workspace and one domain, so it cannot be copied from someone else's
// public DNS or carried to another workspace in an archive.
package domainproof

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// Label is where the record lives, under the domain.
const Label = "_warmbly"

// Prover derives proof values from the instance secret.
type Prover struct{ key []byte }

// New keys a prover from the instance secret; the secret is never used directly.
func New(secret string) *Prover {
	sum := sha256.Sum256([]byte("warmbly-domain-proof\x00" + secret))
	return &Prover{key: sum[:]}
}

// Name is the TXT record's host name for a domain.
func Name(domain string) string {
	return Label + "." + strings.ToLower(strings.TrimSuffix(domain, "."))
}

// Value is the TXT record's content for one workspace and domain.
func (p *Prover) Value(orgID uuid.UUID, domain string) string {
	mac := hmac.New(sha256.New, p.key)
	mac.Write([]byte(orgID.String() + "|" + strings.ToLower(strings.TrimSuffix(domain, "."))))
	return "warmbly-verify=" + hex.EncodeToString(mac.Sum(nil)[:16])
}

// Matches reports whether any published TXT string is this workspace's proof for the domain.
func (p *Prover) Matches(txts []string, orgID uuid.UUID, domain string) bool {
	want := p.Value(orgID, domain)
	for _, t := range txts {
		if hmac.Equal([]byte(strings.TrimSpace(t)), []byte(want)) {
			return true
		}
	}
	return false
}
