package form

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

func demoFields() []models.FormField {
	return []models.FormField{
		{ID: "first", Type: models.FormFieldText, Label: "First name", MapTo: "first_name"},
		{ID: "email", Type: models.FormFieldEmail, Label: "Email", Required: true, MapTo: "email"},
		{ID: "size", Type: models.FormFieldSelect, Label: "Company size", Options: []string{"1-10", "11-50"}},
		{ID: "topics", Type: models.FormFieldCheckboxes, Label: "Topics", Options: []string{"Warmup", "Campaigns"}},
		{ID: "agree", Type: models.FormFieldCheckbox, Label: "Consent", Required: true},
		{ID: "utm", Type: models.FormFieldHidden, Label: "Source", Value: "landing"},
		{ID: "h", Type: models.FormFieldHeading, Label: "Heading"},
	}
}

func TestBuildSubmissionHappyPath(t *testing.T) {
	data, lead, xerr := buildSubmission(demoFields(), map[string][]string{
		"first":  {" Ada "},
		"email":  {"Ada@Example.COM"},
		"size":   {"1-10"},
		"topics": {"Warmup", "Campaigns"},
		"agree":  {"on"},
	})
	if xerr != nil {
		t.Fatalf("unexpected error: %v", xerr)
	}
	if lead == nil {
		t.Fatal("expected a lead")
	}
	if lead.Email != "ada@example.com" || lead.FirstName != "Ada" {
		t.Fatalf("bad mapping: %+v", lead)
	}
	if data["utm"] != "landing" {
		t.Fatalf("hidden constant missing: %v", data["utm"])
	}
	if got := lead.CustomFields["Company size"]; got != "1-10" {
		t.Fatalf("custom field company_size = %q", got)
	}
	if got, ok := data["topics"].([]string); !ok || len(got) != 2 {
		t.Fatalf("topics = %#v", data["topics"])
	}
}

func TestBuildSubmissionValidation(t *testing.T) {
	cases := []struct {
		name    string
		answers map[string][]string
	}{
		{"missing required email", map[string][]string{"agree": {"on"}}},
		{"bad email", map[string][]string{"email": {"not-an-email"}, "agree": {"on"}}},
		{"unknown option", map[string][]string{"email": {"a@b.co"}, "size": {"evil"}, "agree": {"on"}}},
		{"unchecked required consent", map[string][]string{"email": {"a@b.co"}}},
		{"unknown checkbox value", map[string][]string{"email": {"a@b.co"}, "agree": {"on"}, "topics": {"nope"}}},
	}
	for _, tc := range cases {
		if _, _, xerr := buildSubmission(demoFields(), tc.answers); xerr == nil {
			t.Fatalf("%s: expected an error", tc.name)
		}
	}
}

func TestBuildSubmissionWithoutEmailFieldStoresDataOnly(t *testing.T) {
	fields := []models.FormField{{ID: "msg", Type: models.FormFieldTextarea, Label: "Message"}}
	data, lead, xerr := buildSubmission(fields, map[string][]string{"msg": {"hi"}})
	if xerr != nil {
		t.Fatalf("unexpected error: %v", xerr)
	}
	if lead != nil {
		t.Fatal("no email answer must mean no contact")
	}
	if data["msg"] != "hi" {
		t.Fatalf("data = %#v", data)
	}
}

func TestValidateFormFieldsRejectsDuplicateMapTo(t *testing.T) {
	fields := []models.FormField{
		{ID: "a", Type: models.FormFieldText, Label: "A", MapTo: "first_name"},
		{ID: "b", Type: models.FormFieldText, Label: "B", MapTo: "first_name"},
	}
	if xerr := models.ValidateFormFields(fields); xerr == nil {
		t.Fatal("expected duplicate map_to to fail")
	}
}

func TestPublicIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newPublicID()
		if len(id) != 21 {
			t.Fatalf("bad length: %q", id)
		}
		if seen[id] {
			t.Fatal("duplicate public id")
		}
		seen[id] = true
	}
}

func TestPageBreakIsStructureOnly(t *testing.T) {
	fields := append(demoFields(), models.FormField{ID: "pb1", Type: models.FormFieldPageBreak, Label: "About you"})
	if xerr := models.ValidateFormFields(fields); xerr != nil {
		t.Fatalf("page_break rejected: %v", xerr)
	}
	// A titleless break is fine too.
	fields = append(fields, models.FormField{ID: "pb2", Type: models.FormFieldPageBreak})
	if xerr := models.ValidateFormFields(fields); xerr != nil {
		t.Fatalf("untitled page_break rejected: %v", xerr)
	}
	data, _, xerr := buildSubmission(fields, map[string][]string{
		"email": {"a@b.co"}, "agree": {"yes"},
	})
	if xerr != nil {
		t.Fatalf("submit with page breaks: %v", xerr)
	}
	if _, ok := data["pb1"]; ok {
		t.Fatal("page_break leaked into submission data")
	}
	// A form of only layout blocks stays unpublishable even with breaks.
	if xerr := publishable([]models.FormField{{ID: "pb", Type: models.FormFieldPageBreak}}); xerr == nil {
		t.Fatal("page-break-only form must not publish")
	}
	if got := formPageCount(fields); got != 3 {
		t.Fatalf("page count = %d, want 3", got)
	}
}

func TestValidateFormDesignV2(t *testing.T) {
	good := models.FormDesign{
		Layout: "split", Mode: "focus", Align: "center", Theme: "midnight",
		PageBackgroundEnd: "#0f172a", FontFamily: "fraunces",
	}
	if xerr := models.ValidateFormDesign(&good); xerr != nil {
		t.Fatalf("valid v2 design rejected: %v", xerr)
	}
	for _, bad := range []models.FormDesign{
		{Layout: "sideways"},
		{Mode: "carousel"},
		{Align: "justify"},
		{Theme: "Not A Slug"},
		{PageBackgroundEnd: "blue"},
		{FontFamily: "comic-sans"},
	} {
		b := bad
		if xerr := models.ValidateFormDesign(&b); xerr == nil {
			t.Fatalf("invalid design accepted: %+v", bad)
		}
	}
}

func TestReferrerDomain(t *testing.T) {
	cases := map[string]string{
		"https://www.example.com/pricing?x=1": "example.com",
		"https://sub.shop.io/":                "sub.shop.io",
		"not a url":                           "",
		"":                                    "",
	}
	for in, want := range cases {
		if got := referrerDomain(in); got != want {
			t.Fatalf("referrerDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
