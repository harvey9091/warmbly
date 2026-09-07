package aitools

import (
	"testing"

	"github.com/warmbly/warmbly/internal/app/form"
	"github.com/warmbly/warmbly/internal/app/segment"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/generation"
)

// Registration only closes over these values, it never calls them, so
// embedding the interface beats stubbing thirty methods. An accidental call
// panics rather than passing quietly.
type stubSegments struct{ segment.Service }
type stubForms struct{ form.Service }
type stubSuppressions struct{ SuppressionManager }

func leadSurfaceRegistry() *Registry {
	return BuildRegistry(Deps{
		Segments:     stubSegments{},
		Forms:        stubForms{},
		Suppressions: stubSuppressions{},
	})
}

// Every tool these three surfaces add, with the gate it must carry. A wrong
// permission here is a tool that reads or writes more than the caller may.
var leadSurfaceTools = map[string]struct {
	risk    generation.RiskClass
	orgPerm models.OrganizationPermission
	apiPerm uint64
}{
	// Linking an audience is a campaign write; the rest are contact-gated.
	"list_segments":           {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"get_segment":             {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"list_segment_fields":     {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"preview_segment":         {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"create_segment":          {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"update_segment":          {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"delete_segment":          {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"set_segment_members":     {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"list_campaign_segments":  {generation.RiskRead, models.PermViewCampaigns, models.APIPermReadCampaigns},
	"set_campaign_segments":   {generation.RiskWrite, models.PermManageCampaigns, models.APIPermWriteCampaigns},
	"add_segment_to_campaign": {generation.RiskWrite, models.PermManageCampaigns, models.APIPermWriteCampaigns},

	// mint_form_link writes a ticket row, so it keeps the write permission.
	"list_forms":            {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"get_form":              {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"create_form":           {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"update_form":           {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"delete_form":           {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"list_form_submissions": {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"get_form_stats":        {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"get_campaign_forms":    {generation.RiskRead, models.PermViewCampaigns, models.APIPermReadCampaigns},
	"mint_form_link":        {generation.RiskRead, models.PermManageContacts, models.APIPermWriteContacts},

	// Changing who is unreachable decides who gets mail.
	"list_suppressions":  {generation.RiskRead, models.PermViewContacts, models.APIPermReadContacts},
	"add_suppressions":   {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
	"remove_suppression": {generation.RiskWrite, models.PermManageContacts, models.APIPermWriteContacts},
}

func TestLeadSurfaceToolsRegisterWithTheirGates(t *testing.T) {
	r := leadSurfaceRegistry()
	for name, want := range leadSurfaceTools {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("%s: not registered", name)
			continue
		}
		if tool.Risk != want.risk {
			t.Errorf("%s: risk = %v, want %v", name, tool.Risk, want.risk)
		}
		if tool.RequiredOrgPerm != want.orgPerm {
			t.Errorf("%s: org perm = %v, want %v", name, tool.RequiredOrgPerm, want.orgPerm)
		}
		if tool.RequiredAPIPerm != want.apiPerm {
			t.Errorf("%s: api perm = %v, want %v", name, tool.RequiredAPIPerm, want.apiPerm)
		}
		if tool.Handler == nil {
			t.Errorf("%s: nil handler", name)
		}
		if tool.InputSchema == nil {
			t.Errorf("%s: nil input schema", name)
		}
	}
}

// A send-class tool is hidden from both /ai/tools and MCP, which would make
// adding these pointless. None of them transmit mail.
func TestLeadSurfaceToolsAreNeverSendClass(t *testing.T) {
	r := leadSurfaceRegistry()
	for name := range leadSurfaceTools {
		if tool, ok := r.Get(name); ok && tool.Risk == generation.RiskSend {
			t.Errorf("%s is send-class, so no agent surface will ever expose it", name)
		}
	}
}

// No service wired means no tool registered, rather than one that panics.
func TestLeadSurfaceToolsSkippedWithoutServices(t *testing.T) {
	r := BuildRegistry(Deps{})
	for name := range leadSurfaceTools {
		if _, ok := r.Get(name); ok {
			t.Errorf("%s registered with no backing service", name)
		}
	}
}

// A contacts-read key sees the reads and none of the writes.
func TestLeadSurfaceToolsRespectAReadOnlyKey(t *testing.T) {
	r := leadSurfaceRegistry()
	inv := Invocation{IsAPIKey: true, APIPerms: models.APIPermReadContacts}

	permitted := map[string]bool{}
	for _, tool := range r.PermittedTools(inv) {
		permitted[tool.Name] = true
	}

	if !permitted["list_segments"] {
		t.Error("a contacts-read key should see list_segments")
	}
	if !permitted["list_suppressions"] {
		t.Error("a contacts-read key should see list_suppressions")
	}
	for _, name := range []string{"create_segment", "delete_form", "remove_suppression", "mint_form_link"} {
		if permitted[name] {
			t.Errorf("%s reached a read-only key", name)
		}
	}
	// A contacts key is not a campaigns key.
	if permitted["set_campaign_segments"] {
		t.Error("set_campaign_segments reached a key with no campaign write bit")
	}
}

func TestSegmentMatchDefaultsToAllAndRejectsJunk(t *testing.T) {
	got, err := segmentMatch("")
	if err != nil || got != models.SegmentMatchAll {
		t.Errorf(`segmentMatch("") = %q, %v; want "all", nil`, got, err)
	}
	if got, err := segmentMatch("any"); err != nil || got != models.SegmentMatchAny {
		t.Errorf(`segmentMatch("any") = %q, %v; want "any", nil`, got, err)
	}
	// Reading an unknown mode as "all" could widen an audience to everyone.
	if _, err := segmentMatch("either"); err == nil {
		t.Error("segmentMatch accepted an unknown match mode")
	}
}

func TestToSegmentConditionsCarriesBothValueShapes(t *testing.T) {
	got := toSegmentConditions([]toolCondition{
		{Field: "email_domain", Operator: "ends_with", Value: "acme.com"},
		{Field: "esp_provider", Operator: "in", Values: []string{"gmail", "outlook"}},
	})
	if len(got) != 2 {
		t.Fatalf("got %d conditions, want 2", len(got))
	}
	if got[0].Value != "acme.com" || got[0].Values != nil {
		t.Errorf("scalar condition mangled: %+v", got[0])
	}
	if len(got[1].Values) != 2 || got[1].Value != "" {
		t.Errorf("list condition mangled: %+v", got[1])
	}
}

func TestToFormFieldsRejectsAnUnknownBlockType(t *testing.T) {
	if _, err := toFormFields([]toolFormField{{ID: "a", Type: "email"}}); err != nil {
		t.Fatalf("valid block rejected: %v", err)
	}
	if _, err := toFormFields([]toolFormField{{ID: "a", Type: "signature_pad"}}); err == nil {
		t.Error("toFormFields accepted a block type the builder cannot render")
	}
}
