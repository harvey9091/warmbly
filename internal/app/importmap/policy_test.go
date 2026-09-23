package importmap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/typesafe"
)

type fakeAsker struct {
	answers   map[string]typesafe.Answer
	err       error
	state     any
	questions map[string]typesafe.Question
	calls     int
}

func (f *fakeAsker) Ask(_ context.Context, state any, q map[string]typesafe.Question) (*typesafe.Response, error) {
	f.calls++
	f.state, f.questions = state, q
	if f.err != nil {
		return nil, f.err
	}
	return &typesafe.Response{Answers: f.answers}, nil
}

func ignored(n int) []models.ContactImportColumnMapping {
	out := make([]models.ContactImportColumnMapping, n)
	for i := range out {
		out[i] = models.ContactImportColumnMapping{Index: i, Target: models.ContactImportTargetIgnore}
	}
	return out
}

// optionFor finds the option id a question offers for a description fragment.
func optionFor(t *testing.T, q typesafe.Question, fragment string) string {
	t.Helper()
	for id, desc := range q.Criteria.(map[string]string) {
		if strings.Contains(desc, fragment) {
			return id
		}
	}
	t.Fatalf("no option mentions %q in %v", fragment, q.Criteria)
	return ""
}

func TestInferAsksOnlyAboutUnmappedColumnsAndSendsNoValues(t *testing.T) {
	headers := []string{"Email", "Job Title", "Firmenname", "Blank", "dana@acme.com"}
	sample := [][]string{
		{"dana@acme.com", "Head of Sales", "Acme BV", "", "lee@beta.io"},
		{"lee@beta.io", "CTO", "Beta GmbH", "", "kim@gamma.io"},
	}
	mapping := ignored(len(headers))
	mapping[0].Target = models.ContactImportTargetEmail

	f := &fakeAsker{answers: map[string]typesafe.Answer{}}
	_, _ = Infer(context.Background(), f, mapping, headers, Shapes(len(headers), sample), []string{"Title"})

	if f.calls != 1 {
		t.Fatalf("want one call per file, got %d", f.calls)
	}
	for _, id := range []string{"column_2", "column_3", "column_5"} {
		if _, ok := f.questions[id]; !ok {
			t.Errorf("missing question %s", id)
		}
	}
	for _, id := range []string{"column_1", "column_4"} {
		if _, ok := f.questions[id]; ok {
			t.Errorf("asked about %s, which is mapped or empty", id)
		}
	}

	raw, _ := json.Marshal(f.state)
	for _, cell := range []string{"Head of Sales", "Acme BV", "dana@acme.com", "lee@beta.io", "CTO"} {
		if strings.Contains(string(raw), cell) {
			t.Errorf("state carries the cell value %q: %s", cell, raw)
		}
	}
	if !strings.Contains(string(raw), `"Column 5"`) {
		t.Errorf("a header that is really data must go by its number: %s", raw)
	}
	if strings.Contains(f.questions["column_5"].Instructions, "@") {
		t.Errorf("the question repeats a data-shaped header: %s", f.questions["column_5"].Instructions)
	}
}

func TestInferAppliesConfidentAnswersAndCodeDecidesTheRest(t *testing.T) {
	headers := []string{"Email", "Job Title", "Firmenname", "Org", "Opt-in", "Notes"}
	sample := [][]string{{"a@x.com", "CEO", "Acme", "Acme", "yes", "met at a conference"}}
	mapping := ignored(len(headers))
	mapping[0].Target = models.ContactImportTargetEmail

	// A first call only to learn the option ids this file is offered.
	probe := &fakeAsker{answers: map[string]typesafe.Answer{}}
	Infer(context.Background(), probe, mapping, headers, Shapes(len(headers), sample), []string{"Title"})
	title := optionFor(t, probe.questions["column_2"], `"Title"`)
	if _, offered := probe.questions["column_2"].Criteria.(map[string]string)[string(models.ContactImportTargetEmail)]; offered {
		t.Errorf("Email is already filled and must not be offered")
	}
	if _, offered := probe.questions["column_2"].Criteria.(map[string]string)[string(models.ContactImportTargetSubscribed)]; offered {
		t.Errorf("Subscribed is offered for a column that is not yes or no")
	}
	if _, offered := probe.questions["column_5"].Criteria.(map[string]string)[string(models.ContactImportTargetSubscribed)]; !offered {
		t.Errorf("Subscribed is not offered for a yes or no column")
	}

	f := &fakeAsker{answers: map[string]typesafe.Answer{
		"column_2": {Choice: title, Confidence: 0.91},
		// Two columns want Company: the more confident one gets it.
		"column_3": {Choice: string(models.ContactImportTargetCompany), Confidence: 0.95},
		"column_4": {Choice: string(models.ContactImportTargetCompany), Confidence: 0.80},
		// Under the floor: left for the user.
		"column_5": {Choice: string(models.ContactImportTargetSubscribed), Confidence: 0.55},
		"column_6": {Choice: optionNone, Confidence: 0.99},
		// Never asked, and an option nobody offered: both ignored.
		"column_1": {Choice: string(models.ContactImportTargetPhone), Confidence: 0.99},
		"column_9": {Choice: "made_up", Confidence: 0.99},
	}}
	got, inferred := Infer(context.Background(), f, mapping, headers, Shapes(len(headers), sample), []string{"Title"})

	want := []models.ContactImportColumnMapping{
		{Index: 0, Target: models.ContactImportTargetEmail},
		{Index: 1, Target: models.ContactImportTargetCustom, CustomKey: "Title"},
		{Index: 2, Target: models.ContactImportTargetCompany},
		{Index: 3, Target: models.ContactImportTargetIgnore},
		{Index: 4, Target: models.ContactImportTargetIgnore},
		{Index: 5, Target: models.ContactImportTargetIgnore},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d: got %+v, want %+v", i+1, got[i], want[i])
		}
	}
	if len(inferred) != 2 || inferred[0] != 1 || inferred[1] != 2 {
		t.Errorf("inferred = %v, want [1 2]", inferred)
	}
	if mapping[1].Target != models.ContactImportTargetIgnore {
		t.Errorf("Infer changed the caller's mapping in place")
	}
}

func TestInferFallsBackToTheSuggestion(t *testing.T) {
	mapping := ignored(2)
	mapping[0].Target = models.ContactImportTargetEmail
	headers := []string{"Email", "Sector"}
	shapes := []Shape{ShapeEmail, ShapeText}

	got, inferred := Infer(context.Background(), &fakeAsker{err: errors.New("timeout")}, mapping, headers, shapes, nil)
	if got[1].Target != models.ContactImportTargetIgnore || inferred != nil {
		t.Fatalf("a failed call changed the mapping: %+v %v", got, inferred)
	}
	got, inferred = Infer(context.Background(), nil, mapping, headers, shapes, nil)
	if got[1].Target != models.ContactImportTargetIgnore || inferred != nil {
		t.Fatalf("no judge changed the mapping: %+v %v", got, inferred)
	}
	f := &fakeAsker{}
	full := []models.ContactImportColumnMapping{{Index: 0, Target: models.ContactImportTargetEmail}, {Index: 1, Target: models.ContactImportTargetCompany}}
	Infer(context.Background(), f, full, headers, shapes, nil)
	if f.calls != 0 {
		t.Fatalf("called the judge with nothing left to ask")
	}
}

func TestInferKeepsAnExistingFieldToOneColumn(t *testing.T) {
	mapping := ignored(3)
	mapping[0].Target = models.ContactImportTargetEmail
	mapping[1] = models.ContactImportColumnMapping{Index: 1, Target: models.ContactImportTargetCustom, CustomKey: "Industry"}
	headers := []string{"Email", "Industry", "Sector"}
	probe := &fakeAsker{}
	Infer(context.Background(), probe, mapping, headers, []Shape{ShapeEmail, ShapeText, ShapeText}, []string{"Industry", "Plan"})
	for _, desc := range probe.questions["column_3"].Criteria.(map[string]string) {
		if strings.Contains(desc, `"Industry"`) {
			t.Fatalf("offered a field another column already fills")
		}
	}
}

func TestQuestionsStayWithinTheAPILimits(t *testing.T) {
	keys := make([]string, 400)
	for i := range keys {
		keys[i] = "field " + strings.Repeat("x", i%7)
	}
	headers := make([]string, 90)
	shapes := make([]Shape, 90)
	for i := range headers {
		headers[i] = "Header"
		shapes[i] = ShapeText
	}
	f := &fakeAsker{}
	Infer(context.Background(), f, ignored(90), headers, shapes, keys)
	if len(f.questions) != MaxQuestions {
		t.Fatalf("asked %d questions, want %d", len(f.questions), MaxQuestions)
	}
	for id, q := range f.questions {
		if n := len(q.Criteria.(map[string]string)); n > 255 {
			t.Fatalf("%s offers %d options, over the API's 255", id, n)
		}
	}
}

func TestShapeOf(t *testing.T) {
	for want, values := range map[Shape][]string{
		ShapeEmail:    {"dana@acme.com", "Lee <lee@beta.io>", ""},
		ShapeURL:      {"https://www.linkedin.com/in/dana", "https://cbre.nl"},
		ShapePhone:    {"+31 6 1234 5678", "(555) 123-4567"},
		ShapeNumber:   {"2.30", "1,200", "15%"},
		ShapeDate:     {"Sep 05, 2026 9:03 PM", "2026-09-05"},
		ShapeYesNo:    {"yes", "No", "TRUE"},
		ShapeText:     {"Real Estate", "CBRE Nederland"},
		ShapeLongText: {"Strategic Window: Just hired a new VP of Sales and is expanding into the Benelux market"},
		ShapeEmpty:    {"", "  "},
		ShapeMixed:    {"dana@acme.com", "Real Estate", "2.30"},
	} {
		if got := ShapeOf(values); got != want {
			t.Errorf("ShapeOf(%q) = %q, want %q", values, got, want)
		}
	}
}
