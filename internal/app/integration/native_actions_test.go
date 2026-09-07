package integration

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

func TestValidateNativeActionConfig_UpsertContact(t *testing.T) {
	campaign := uuid.New().String()
	cases := []struct {
		name    string
		cfg     string
		wantErr bool
	}{
		{"needs an email", `{}`, true},
		{"email alone is enough", `{"email":"{{.email}}"}`, false},
		{"unknown if_exists", `{"email":"{{.email}}","if_exists":"merge"}`, true},
		{"skip is allowed", `{"email":"{{.email}}","if_exists":"skip"}`, false},
		{"bad campaign id", `{"email":"{{.email}}","campaign_id":"nope"}`, true},
		{"good campaign id", `{"email":"{{.email}}","campaign_id":"` + campaign + `"}`, false},
		{"bad tag id", `{"email":"{{.email}}","category_ids":["x"]}`, true},
		{"blank tag id is ignored", `{"email":"{{.email}}","category_ids":[""]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNativeActionConfig(models.IntegrationActionUpsertContact, json.RawMessage(tc.cfg))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateNativeActionConfig_AddToCampaign(t *testing.T) {
	if err := validateNativeActionConfig(models.IntegrationActionAddToCampaign, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an error without a campaign")
	}
	if err := validateNativeActionConfig(models.IntegrationActionAddToCampaign, json.RawMessage(`{"campaign_id":"`+uuid.New().String()+`"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildUpsertContact_RendersTemplatesAgainstEvent(t *testing.T) {
	campaign := uuid.New().String()
	tag := uuid.New().String()
	cfg := parseNativeConfig(json.RawMessage(`{
		"email": "{{.data.email}}",
		"first_name": "{{.data.first_name}}",
		"company": "{{.data.company}}",
		"custom_fields": [{"key":"team_size","value":"{{.data.team_size}}"},{"key":"empty","value":"{{.data.missing}}"},{"key":"","value":"x"}],
		"category_ids": ["` + tag + `", "junk"],
		"campaign_id": "` + campaign + `"
	}`))
	data := map[string]any{
		"data": map[string]any{"email": "  Jane@Example.com ", "first_name": "Jane", "company": "Example Inc", "team_size": "10-50"},
	}
	in, err := buildUpsertContact(models.Automation{Name: "Lead ads intake"}, cfg, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Email != "jane@example.com" {
		t.Fatalf("email = %q", in.Email)
	}
	if in.FirstName != "Jane" || in.Company != "Example Inc" || in.LastName != "" {
		t.Fatalf("fields = %+v", in)
	}
	if in.CustomFields["team_size"] != "10-50" {
		t.Fatalf("custom fields = %v", in.CustomFields)
	}
	if _, ok := in.CustomFields["empty"]; ok {
		t.Fatal("a blank rendered custom field must be dropped, not written as empty")
	}
	if len(in.Categories) != 1 || in.Categories[0] != tag {
		t.Fatalf("categories = %v", in.Categories)
	}
	if len(in.Campaigns) != 1 || in.Campaigns[0] != campaign {
		t.Fatalf("campaigns = %v", in.Campaigns)
	}
	if in.Source != models.ContactSourceAutomation || in.SourceDetail != "Lead ads intake" {
		t.Fatalf("source = %s / %s", in.Source, in.SourceDetail)
	}
}

func TestBuildUpsertContact_RejectsEmptyOrInvalidEmail(t *testing.T) {
	cfg := parseNativeConfig(json.RawMessage(`{"email":"{{.email}}"}`))
	if _, err := buildUpsertContact(models.Automation{}, cfg, map[string]any{}); err == nil {
		t.Fatal("expected an error when the email renders empty")
	}
	if _, err := buildUpsertContact(models.Automation{}, cfg, map[string]any{"email": "not-an-email"}); err == nil {
		t.Fatal("expected an error for an address without @")
	}
}

func TestSampleEventData_NewTriggersCarryTheirFields(t *testing.T) {
	created := sampleEventData("contact.created")
	if created["source"] != "form" || created["contact_email"] == "" {
		t.Fatalf("contact.created sample = %v", created)
	}
	if _, ok := created["campaign_id"]; ok {
		t.Fatal("a new contact carries no single campaign_id")
	}
	form := sampleEventData("form.submitted")
	if form["form_name"] == "" {
		t.Fatalf("form.submitted sample = %v", form)
	}
	answers, ok := form["data"].(map[string]any)
	if !ok || answers["email"] == "" {
		t.Fatalf("form answers = %v", form["data"])
	}
}

func TestActionPreview_UpsertContactRendersFields(t *testing.T) {
	n := models.AutomationNode{
		Type:   models.AutomationNodeAction,
		Action: models.IntegrationActionUpsertContact,
		Config: json.RawMessage(`{"email":"{{.email}}","company":"{{.company}}","custom_fields":[{"key":"plan","value":"{{.plan}}"}]}`),
	}
	p := actionPreview(n, map[string]any{"email": "a@b.co", "company": "Acme", "plan": "pro"})
	if p["email"] != "a@b.co" || p["company"] != "Acme" || p["custom:plan"] != "pro" {
		t.Fatalf("preview = %v", p)
	}
}
