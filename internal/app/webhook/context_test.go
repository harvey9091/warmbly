package webhook

import (
	"context"
	"testing"
)

func TestAutomationDepth_RoundTrip(t *testing.T) {
	if got := AutomationDepth(context.Background()); got != 0 {
		t.Fatalf("depth outside an automation = %d", got)
	}
	ctx := WithAutomationDepth(context.Background(), 2)
	if got := AutomationDepth(ctx); got != 2 {
		t.Fatalf("depth = %d, want 2", got)
	}
	if WithAutomationDepth(context.Background(), 0) != context.Background() {
		t.Fatal("a zero depth must not wrap the context")
	}
}

func TestStampAutomationDepth_CopiesMapPayloadsOnly(t *testing.T) {
	data := map[string]any{"contact_email": "a@b.co"}
	plain := stampAutomationDepth(context.Background(), data).(map[string]any)
	if plain[AutomationDepthKey] != nil {
		t.Fatal("no depth outside an automation")
	}
	plain["written_by_sink"] = true
	if _, shared := data["written_by_sink"]; shared {
		t.Fatal("the sink must get its own copy even without a depth")
	}
	ctx := WithAutomationDepth(context.Background(), 3)
	out, ok := stampAutomationDepth(ctx, data).(map[string]any)
	if !ok || out[AutomationDepthKey] != float64(3) {
		t.Fatalf("stamped = %v", out)
	}
	if _, leaked := data[AutomationDepthKey]; leaked {
		t.Fatal("the caller's map must stay untouched")
	}
	type typed struct{ A string }
	if got := stampAutomationDepth(ctx, typed{A: "x"}); got != (typed{A: "x"}) {
		t.Fatalf("struct payloads pass through unchanged, got %v", got)
	}
}
