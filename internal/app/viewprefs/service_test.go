package viewprefs

import (
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func TestValidateNormalizesAndRejects(t *testing.T) {
	ok := &models.ViewPreferences{
		View:    models.ViewContacts,
		Columns: []string{"company", "custom:Company   Mobile", "phone"},
		Sort:    &models.ViewSort{By: " custom:Industry ", Reverse: true},
	}
	if xerr := validate(ok); xerr != nil {
		t.Fatalf("valid layout rejected: %v", xerr)
	}
	if ok.Columns[1] != "custom:Company Mobile" {
		t.Fatalf("custom column not normalized: %q", ok.Columns[1])
	}
	if ok.Sort.By != "custom:Industry" {
		t.Fatalf("sort not normalized: %q", ok.Sort.By)
	}

	blank := &models.ViewPreferences{View: models.ViewContacts, Sort: &models.ViewSort{By: "  "}}
	if xerr := validate(blank); xerr != nil || blank.Sort != nil {
		t.Fatalf("a blank sort means the default: %v %+v", xerr, blank.Sort)
	}

	for name, prefs := range map[string]*models.ViewPreferences{
		"duplicate":      {Columns: []string{"company", "custom:A", "custom:A"}},
		"bad builtin":    {Columns: []string{"Company"}},
		"bad custom key": {Columns: []string{"custom:Revenue ($)"}},
		"too long":       {Columns: []string{"custom:" + strings.Repeat("a", 300)}},
		"bad sort":       {Sort: &models.ViewSort{By: "drop table"}},
		"too many":       {Columns: manyColumns(models.ViewPreferencesMaxColumns + 1)},
	} {
		prefs.View = models.ViewContacts
		if xerr := validate(prefs); xerr == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func manyColumns(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "custom:f" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
	}
	return out
}
