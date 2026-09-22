package advisor

import (
	"testing"

	"github.com/google/uuid"
)

// The keep list has to spell fingerprints exactly as Finding.Fingerprint does,
// or ResolveMissing closes the findings it was meant to preserve.
func TestKeepUnjudgedMatchesFingerprints(t *testing.T) {
	id := uuid.New()
	want := map[string]bool{}
	for _, key := range judgmentFindingKeys {
		want[Finding{Key: key, EntityType: "step", EntityID: &id}.Fingerprint()] = true
	}
	for _, fp := range keepUnjudged([]uuid.UUID{id}) {
		if !want[fp] {
			t.Fatalf("keepUnjudged produced %q, which no judgment finding would carry", fp)
		}
		delete(want, fp)
	}
	if len(want) != 0 {
		t.Fatalf("keepUnjudged missed %v", want)
	}
}
