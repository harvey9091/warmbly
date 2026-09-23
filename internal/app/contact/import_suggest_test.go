package contact

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func TestSuggestMappingMatchesExistingCustomFields(t *testing.T) {
	headers := []string{"Email", "industry", "Company URL", "Total Score", "Company", "INDUSTRY", "Notes"}
	existing := []string{"Industry", "company_url", "industry", "Company"}

	got := SuggestMapping(headers, nil, existing)

	want := []models.ContactImportColumnMapping{
		{Index: 0, Target: models.ContactImportTargetEmail},
		// The stored spelling wins, and the most used of two spellings.
		{Index: 1, Target: models.ContactImportTargetCustom, CustomKey: "Industry"},
		{Index: 2, Target: models.ContactImportTargetCustom, CustomKey: "company_url"},
		// No field by that name: left for the user, never invented.
		{Index: 3, Target: models.ContactImportTargetIgnore},
		// A standard field outranks a custom field of the same name.
		{Index: 4, Target: models.ContactImportTargetCompany},
		// One column per field; a second spelling of it stays ignored.
		{Index: 5, Target: models.ContactImportTargetIgnore},
		{Index: 6, Target: models.ContactImportTargetIgnore},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d mappings, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d (%q): got %+v, want %+v", i, headers[i], got[i], want[i])
		}
	}
}

func TestSuggestMappingKeepsVerificationAheadOfCustomFields(t *testing.T) {
	headers := []string{"email", "ZeroBounce Status"}
	got := SuggestMapping(headers, [][]string{{"a@x.com", "valid"}}, []string{"ZeroBounce Status"})
	if got[1].Target != models.ContactImportTargetVerificationStatus {
		t.Fatalf("verdict column mapped to %q, want verification_status", got[1].Target)
	}
}

func TestSuggestMappingWithoutExistingFields(t *testing.T) {
	got := SuggestMapping([]string{"email", "Industry"}, nil, nil)
	if got[1].Target != models.ContactImportTargetIgnore {
		t.Fatalf("unknown header mapped to %q, want ignore", got[1].Target)
	}
}

func TestFoldCustomFieldKey(t *testing.T) {
	for in, want := range map[string]string{
		"Company URL":    "companyurl",
		"company_url":    "companyurl",
		"company-url":    "companyurl",
		" Company.URL ":  "companyurl",
		"Revenue ($)":    "revenue",
		"###":            "",
		"Straße Nummer2": "straßenummer2",
	} {
		if got := FoldCustomFieldKey(in); got != want {
			t.Errorf("FoldCustomFieldKey(%q) = %q, want %q", in, got, want)
		}
	}
}
