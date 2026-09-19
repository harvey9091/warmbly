package viewprefs

import (
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func cols(v ...string) *[]string { return &v }

func TestValidateNormalizesAndRejects(t *testing.T) {
	ok := models.ViewPreferencesUpdate{
		Columns: cols("name", "company", "custom:Company   Mobile", "phone"),
		Sort:    &models.ViewSort{By: " custom:Industry ", Reverse: true},
	}
	if xerr := validate(models.ViewContacts, &ok); xerr != nil {
		t.Fatalf("valid layout rejected: %v", xerr)
	}
	if (*ok.Columns)[2] != "custom:Company Mobile" {
		t.Fatalf("custom column not normalized: %q", (*ok.Columns)[2])
	}
	if ok.Sort.By != "custom:Industry" {
		t.Fatalf("sort not normalized: %q", ok.Sort.By)
	}

	blank := models.ViewPreferencesUpdate{Sort: &models.ViewSort{By: "  ", Reverse: true}}
	if xerr := validate(models.ViewContacts, &blank); xerr != nil || blank.Sort == nil || blank.Sort.By != "" || blank.Sort.Reverse {
		t.Fatalf("a blank sort means the default: %v %+v", xerr, blank.Sort)
	}

	partial := models.ViewPreferencesUpdate{}
	if xerr := validate(models.ViewContacts, &partial); xerr != nil {
		t.Fatalf("an empty update keeps everything: %v", xerr)
	}

	leads := models.ViewPreferencesUpdate{Columns: cols("progress", "opened", "sender")}
	if xerr := validate(models.ViewCampaignLeads, &leads); xerr != nil {
		t.Fatalf("leads columns rejected: %v", xerr)
	}

	for name, tc := range map[string]struct {
		view string
		upd  models.ViewPreferencesUpdate
		code string
	}{
		"duplicate":           {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("company", "custom:A", "custom:A")}, "duplicate_column"},
		"unknown builtin":     {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("nonexistent_col")}, "invalid_column"},
		"other view's column": {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("opened")}, "invalid_column"},
		"bad case":            {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("Company")}, "invalid_column"},
		"bad custom key":      {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("custom:Revenue ($)")}, "invalid_column"},
		"too long":            {models.ViewContacts, models.ViewPreferencesUpdate{Columns: cols("custom:" + strings.Repeat("a", 300))}, "invalid_column"},
		"bad sort":            {models.ViewContacts, models.ViewPreferencesUpdate{Sort: &models.ViewSort{By: "drop table"}}, "invalid_sort"},
		"column as sort":      {models.ViewContacts, models.ViewPreferencesUpdate{Sort: &models.ViewSort{By: "status"}}, "invalid_sort"},
		"too many":            {models.ViewContacts, models.ViewPreferencesUpdate{Columns: manyColumns(models.ViewPreferencesMaxColumns + 1)}, "too_many_columns"},
	} {
		xerr := validate(tc.view, &tc.upd)
		if xerr == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if xerr.Identifier != tc.code {
			t.Errorf("%s: code %q, want %q", name, xerr.Identifier, tc.code)
		}
	}
}

func manyColumns(n int) *[]string {
	out := make([]string, n)
	for i := range out {
		out[i] = "custom:f" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
	}
	return &out
}
