package repository

import "testing"

// resolveContactSort is the one place sort_by becomes SQL, for the list and for
// "select all matching" alike.
func TestResolveContactSort(t *testing.T) {
	t.Run("built-in column", func(t *testing.T) {
		args := []any{"org"}
		idx := 2
		name, spec := resolveContactSort("company", &args, &idx)
		if name != "company" || spec.expr != "c.company" || spec.nullable {
			t.Fatalf("got %q %+v", name, spec)
		}
		if len(args) != 1 || idx != 2 {
			t.Fatalf("a built-in sort must bind nothing: args=%v idx=%d", args, idx)
		}
	})
	t.Run("custom field binds its key", func(t *testing.T) {
		args := []any{"org"}
		idx := 2
		name, spec := resolveContactSort("custom:Company  Mobile", &args, &idx)
		if name != "custom:Company Mobile" {
			t.Fatalf("name %q: the key must be normalized so the cursor's sort key is stable", name)
		}
		if !spec.nullable || spec.kind != sortText {
			t.Fatalf("a custom field is nullable text: %+v", spec)
		}
		if spec.expr != "NULLIF(c.custom_fields ->> $2::text, '')" {
			t.Fatalf("expr %q", spec.expr)
		}
		if len(args) != 2 || args[1] != "Company Mobile" || idx != 3 {
			t.Fatalf("the key must be bound once: args=%v idx=%d", args, idx)
		}
	})
	t.Run("malformed custom key falls back", func(t *testing.T) {
		args := []any{}
		idx := 1
		name, _ := resolveContactSort("custom:Revenue ($)", &args, &idx)
		if name != "created_at" || len(args) != 0 || idx != 1 {
			t.Fatalf("got %q args=%v idx=%d", name, args, idx)
		}
	})
	t.Run("unknown column falls back", func(t *testing.T) {
		args := []any{}
		idx := 1
		name, _ := resolveContactSort("nope", &args, &idx)
		if name != "created_at" {
			t.Fatalf("got %q", name)
		}
	})
}
