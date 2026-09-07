package webhook

import "context"

// AutomationDepthKey is the internal event-data key carrying how many automation
// hops led to an event. Underscore-prefixed so it is stripped from customer
// deliveries and cannot be set by an inbound webhook body.
const AutomationDepthKey = "_automation_depth"

type automationDepthCtxKey struct{}

// WithAutomationDepth marks a context as running inside an automation action,
// depth hops down from the original event. Events dispatched under it carry the
// depth so a flow that creates a contact cannot re-trigger itself forever.
func WithAutomationDepth(ctx context.Context, depth int) context.Context {
	if depth <= 0 {
		return ctx
	}
	return context.WithValue(ctx, automationDepthCtxKey{}, depth)
}

// AutomationDepth reports the automation depth carried by ctx, 0 outside one.
func AutomationDepth(ctx context.Context) int {
	if d, ok := ctx.Value(automationDepthCtxKey{}).(int); ok && d > 0 {
		return d
	}
	return 0
}

// stampAutomationDepth returns a copy of a map payload for the sink, carrying
// the context's automation depth when there is one. Always a copy: the sink
// runs automations on a goroutine that writes into the map while this call
// still marshals the same payload for endpoint delivery.
func stampAutomationDepth(ctx context.Context, data any) any {
	m, ok := data.(map[string]any)
	if !ok {
		return data
	}
	out := make(map[string]any, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	if depth := AutomationDepth(ctx); depth > 0 {
		out[AutomationDepthKey] = float64(depth)
	}
	return out
}
