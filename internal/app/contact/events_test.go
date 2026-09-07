package contact

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/models"
)

type recordingDispatcher struct {
	events []models.WebhookEventType
	data   []map[string]any
}

func (r *recordingDispatcher) Dispatch(_ context.Context, _ uuid.UUID, t models.WebhookEventType, data any) (uuid.UUID, error) {
	r.events = append(r.events, t)
	m, _ := data.(map[string]any)
	r.data = append(r.data, m)
	return uuid.New(), nil
}

func TestEmitCreated_OnlyNewRowsAndNotWhenSuppressed(t *testing.T) {
	rec := &recordingDispatcher{}
	svc := &contactService{webhooks: rec}
	in := []models.AddContact{
		{Email: "new@example.com", Source: models.ContactSourceForm, SourceDetail: "Demo request"},
		{Email: "old@example.com", Source: models.ContactSourceForm},
	}
	out := []models.Contact{
		{ID: uuid.New(), Email: "new@example.com", IsNew: true, Campaigns: []models.MiniCampaign{{ID: "c1", Name: "Q3"}}},
		{ID: uuid.New(), Email: "old@example.com", IsNew: false},
	}
	svc.emitCreated(context.Background(), uuid.New(), in, out)
	if len(rec.events) != 1 || rec.events[0] != models.WebhookEventContactCreated {
		t.Fatalf("events = %v", rec.events)
	}
	got := rec.data[0]
	if got["contact_email"] != "new@example.com" || got["source"] != "form" || got["source_detail"] != "Demo request" {
		t.Fatalf("payload = %v", got)
	}
	if ids, _ := got["campaign_ids"].([]string); len(ids) != 1 || ids[0] != "c1" {
		t.Fatalf("campaign_ids = %v", got["campaign_ids"])
	}

	rec.events = nil
	svc.emitCreated(WithoutCreatedEvents(context.Background()), uuid.New(), in, out)
	if len(rec.events) != 0 {
		t.Fatalf("suppressed context still emitted %v", rec.events)
	}

	quiet := &contactService{}
	quiet.emitCreated(context.Background(), uuid.New(), in, out) // nil dispatcher is a no-op

	bigIn := make([]models.AddContact, contactCreatedEventMaxBatch+1)
	bigOut := make([]models.Contact, len(bigIn))
	for i := range bigIn {
		bigIn[i] = models.AddContact{Email: "x@example.com"}
		bigOut[i] = models.Contact{ID: uuid.New(), IsNew: true}
	}
	rec.events = nil
	svc.emitCreated(context.Background(), uuid.New(), bigIn, bigOut)
	if len(rec.events) != 0 {
		t.Fatalf("a batch past the cap must stay silent, emitted %d", len(rec.events))
	}
}

func TestContactCreatedPayload_DefaultsUnknownSourceAndEmptyMaps(t *testing.T) {
	p := ContactCreatedPayload(models.Contact{ID: uuid.New(), Email: "x@y.z"}, models.AddContact{})
	if p["source"] != "unknown" {
		t.Fatalf("source = %v", p["source"])
	}
	if m, ok := p["custom_fields"].(map[string]string); !ok || m == nil {
		t.Fatalf("custom_fields = %v", p["custom_fields"])
	}
}
