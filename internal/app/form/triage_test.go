package form

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/typesafe"
)

// fakeAsker records the state it was asked about and answers from a script.
type fakeAsker struct {
	calls int
	state any
	resp  *typesafe.Response
	err   error
}

func (f *fakeAsker) Ask(_ context.Context, state any, _ map[string]typesafe.Question) (*typesafe.Response, error) {
	f.calls++
	f.state = state
	return f.resp, f.err
}

func answers(junk float64, choice string, confidence float64) *typesafe.Response {
	return &typesafe.Response{Answers: map[string]typesafe.Answer{
		"junk":      {Type: typesafe.QuestionNoul, Noul: junk},
		"submitter": {Type: typesafe.QuestionChoice, Choice: choice, Confidence: confidence},
	}}
}

func triageFields() []models.FormField {
	return []models.FormField{
		{ID: "email", Type: models.FormFieldEmail, Label: "Email", MapTo: "email"},
		{ID: "msg", Type: models.FormFieldTextarea, Label: "Message"},
		{ID: "topics", Type: models.FormFieldCheckboxes, Label: "Topics", Options: []string{"Warmup", "Campaigns"}},
		{ID: "utm", Type: models.FormFieldHidden, Label: "Source", Value: "landing"},
		{ID: "h", Type: models.FormFieldHeading, Label: "Heading"},
	}
}

func longData() map[string]any {
	return map[string]any{
		"email":  "ada@example.com",
		"msg":    "We are looking at replacing our outreach tool for a team of twelve.",
		"topics": []string{"Warmup", "Campaigns"},
		"utm":    "landing",
	}
}

func TestTriageJunkWinsOverChoice(t *testing.T) {
	a := &fakeAsker{resp: answers(0.91, models.FormTriageBuyer, 0.95)}
	got, conf, ok := triage(context.Background(), a, triageFields(), longData())
	if !ok || got != models.FormTriageJunk || conf != 0.91 {
		t.Fatalf("got %q %.2f %v", got, conf, ok)
	}
}

func TestTriageChoiceAboveFloor(t *testing.T) {
	for _, want := range []string{models.FormTriageBuyer, models.FormTriageVendor, models.FormTriageJobSeeker, models.FormTriageOther} {
		a := &fakeAsker{resp: answers(0.1, want, 0.84)}
		got, conf, ok := triage(context.Background(), a, triageFields(), longData())
		if !ok || got != want || conf != 0.84 {
			t.Fatalf("%s: got %q %.2f %v", want, got, conf, ok)
		}
	}
}

func TestTriageLowConfidenceFilesAsOther(t *testing.T) {
	a := &fakeAsker{resp: answers(0.2, models.FormTriageBuyer, 0.41)}
	got, conf, ok := triage(context.Background(), a, triageFields(), longData())
	if !ok || got != models.FormTriageOther || conf != 0.41 {
		t.Fatalf("got %q %.2f %v", got, conf, ok)
	}
}

func TestTriageUnknownChoiceFilesAsOther(t *testing.T) {
	a := &fakeAsker{resp: answers(0.2, "alien", 0.99)}
	got, _, ok := triage(context.Background(), a, triageFields(), longData())
	if !ok || got != models.FormTriageOther {
		t.Fatalf("got %q %v", got, ok)
	}
}

func TestTriageSkipsShortSubmissions(t *testing.T) {
	a := &fakeAsker{resp: answers(0.99, models.FormTriageBuyer, 0.99)}
	got, _, ok := triage(context.Background(), a, triageFields(), map[string]any{"email": "a@b.co"})
	if ok || got != "" {
		t.Fatalf("expected untriaged, got %q %v", got, ok)
	}
	if a.calls != 0 {
		t.Fatalf("expected no call, got %d", a.calls)
	}
}

func TestTriageErrorLeavesUntriaged(t *testing.T) {
	a := &fakeAsker{err: errors.New("boom")}
	got, _, ok := triage(context.Background(), a, triageFields(), longData())
	if ok || got != "" {
		t.Fatalf("expected untriaged, got %q %v", got, ok)
	}
	if _, _, ok := triage(context.Background(), nil, triageFields(), longData()); ok {
		t.Fatal("nil asker must not triage")
	}
}

func TestTriageStateCarriesOnlyLabelledAnswers(t *testing.T) {
	a := &fakeAsker{resp: answers(0.1, models.FormTriageBuyer, 0.9)}
	triage(context.Background(), a, triageFields(), longData())
	st, ok := a.state.(triageState)
	if !ok {
		t.Fatalf("state = %T", a.state)
	}
	if len(st.Answers) != 3 {
		t.Fatalf("answers = %+v", st.Answers)
	}
	for _, ans := range st.Answers {
		if ans.Label == "Source" {
			t.Fatal("hidden field must not reach the model")
		}
	}
	if st.Answers[2].Label != "Topics" || st.Answers[2].Value != "Warmup, Campaigns" {
		t.Fatalf("checkbox group = %+v", st.Answers[2])
	}
}

func TestTriageStateCapsValues(t *testing.T) {
	fields := []models.FormField{
		{ID: "a", Type: models.FormFieldTextarea, Label: "A"},
		{ID: "b", Type: models.FormFieldTextarea, Label: "B"},
	}
	data := map[string]any{"a": strings.Repeat("é", 900), "b": strings.Repeat("x", 900)}
	st, total := buildTriageState(fields, data)
	if len(st.Answers) != 2 {
		t.Fatalf("answers = %d", len(st.Answers))
	}
	if n := len([]rune(st.Answers[0].Value)); n != triageValueRunes {
		t.Fatalf("value cap: %d runes", n)
	}
	if total != 2*triageValueRunes {
		t.Fatalf("total = %d", total)
	}

	// The state cap: nine 500-rune answers fit 4000 runes only up to the
	// eighth, and nothing after the cap is sent.
	many := make([]models.FormField, 0, 10)
	big := map[string]any{}
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		many = append(many, models.FormField{ID: id, Type: models.FormFieldTextarea, Label: id})
		big[id] = strings.Repeat("y", 600)
	}
	st, total = buildTriageState(many, big)
	if total != triageStateRunes {
		t.Fatalf("total = %d", total)
	}
	if len(st.Answers) != 8 {
		t.Fatalf("answers = %d", len(st.Answers))
	}
}
