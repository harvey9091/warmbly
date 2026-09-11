package integration

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func TestVerificationClientRefusesANonVerificationProvider(t *testing.T) {
	for _, p := range models.VerificationProviders {
		if c, err := verificationClient(p, "key"); err != nil || c == nil {
			t.Fatalf("%s = %v, %v", p, c, err)
		}
	}
	if _, err := verificationClient(models.IntegrationSlack, "key"); err == nil {
		t.Fatal("slack was accepted as a verifier")
	}
}

func TestProviderLabelNamesEveryVerifier(t *testing.T) {
	for _, p := range models.VerificationProviders {
		if label := ProviderLabel(p); label == string(p) {
			t.Fatalf("%s has no catalog display name", p)
		}
	}
}
