// Package importmap asks TypeSafe which field an import column holds when its
// header matched nothing, so "Job Title", "Firmenname" or "Sector" lands on the
// right field before anyone opens a dropdown.
//
// THIS FILE IS THE POLICY: every question, option and threshold is here. It
// follows the rules the inbox tagger learned the hard way:
//
//  1. One call per file. Every unmatched column is its own question over one
//     state, so the headers are ingested once.
//  2. The model answers, code decides. Resolve applies the confidence floor
//     and gives each field to one column at most.
//  3. Never ask what is already known. Columns the header aliases, the
//     verification vocabulary, an existing custom field or the values
//     themselves already decided are facts, and are never sent as questions.
//
// No cell value leaves the instance. The state is the headers, the workspace's
// custom field names, and the kind of value each column holds (Shape).
package importmap

import (
	"context"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/typesafe"
)

// Thresholds. Tuning these is tuning the product.
const (
	// ConfFloor is the confidence below which a choice is not applied. A
	// mapping the user has to undo costs more than one they have to pick.
	ConfFloor = 0.70

	// MaxQuestions bounds one call. A file wider than this is a CRM dump, and
	// the columns past it stay for the user or "Keep N more as custom fields".
	MaxQuestions = 40

	// MaxCustomOptions bounds the custom fields offered, most used first, well
	// inside the API's 255 options per choice.
	MaxCustomOptions = 150

	// headerRunes caps one header in the state.
	headerRunes = 80

	// Timeout bounds the call so a slow judge costs the suggestion, never the
	// preview.
	Timeout = 4 * time.Second
)

// optionNone is the answer for a column that holds none of the fields offered.
const optionNone = "none"

// standardOptions are the fields the model may pick, with what each holds.
// Verification status is never offered: it is recognised from the values'
// vocabulary, which the model is not shown.
var standardOptions = []struct {
	target models.ContactImportColumnTarget
	desc   string
}{
	{models.ContactImportTargetEmail, "The contact's email address"},
	{models.ContactImportTargetFirstName, "The contact's first name, or given name"},
	{models.ContactImportTargetLastName, "The contact's last name, surname, or family name"},
	{models.ContactImportTargetCompany, "The name of the company or organization the contact works for"},
	{models.ContactImportTargetPhone, "The contact's phone or mobile number"},
	{models.ContactImportTargetSubscribed, "Whether the contact agreed to receive email, as yes or no"},
	{models.ContactImportTargetCategories, "Tags, labels, lists, or segments the contact belongs to"},
}

// Column is one column as the model sees it.
type Column struct {
	Header string `json:"header"`
	Holds  Shape  `json:"holds"`
}

type state struct {
	Columns      []Column `json:"columns"`
	CustomFields []string `json:"existing_custom_fields"`
}

// option is one answer a column question may give.
type option struct {
	target    models.ContactImportColumnTarget
	customKey string
}

// Infer fills the ignored columns of mapping that the model places with
// confidence, and returns the indexes it filled. headers and shapes are per
// column; existingKeys is most used first. Any failure returns mapping as it
// was: the deterministic suggestion always stands on its own.
func Infer(
	ctx context.Context,
	asker typesafe.Asker,
	mapping []models.ContactImportColumnMapping,
	headers []string,
	shapes []Shape,
	existingKeys []string,
) ([]models.ContactImportColumnMapping, []int) {
	if asker == nil {
		return mapping, nil
	}
	taken := takenTargets(mapping)
	options, criteria := buildOptions(taken, existingKeys)

	st := state{Columns: make([]Column, len(headers)), CustomFields: capKeys(existingKeys)}
	questions := map[string]typesafe.Question{}
	for i := range headers {
		col := Column{Header: safeHeader(i, headers[i]), Holds: shapeAt(shapes, i)}
		st.Columns[i] = col
		if len(questions) >= MaxQuestions || i >= len(mapping) || mapping[i].Target != models.ContactImportTargetIgnore || col.Holds == ShapeEmpty {
			continue
		}
		questions[questionID(i)] = typesafe.Choice(
			"Which contact field does the column headed \""+col.Header+"\" hold? Its values are "+string(col.Holds)+".",
			columnCriteria(criteria, col.Holds),
		)
	}
	if len(questions) == 0 {
		return mapping, nil
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	resp, err := asker.Ask(ctx, st, questions)
	if err != nil || resp == nil {
		return mapping, nil
	}
	return Resolve(mapping, resp.Answers, options, taken)
}

// Resolve applies the answers: a confident choice of a field nobody fills yet
// maps its column, and when two columns want one field the more confident
// wins. It never touches a column the suggester already mapped.
func Resolve(
	mapping []models.ContactImportColumnMapping,
	answers map[string]typesafe.Answer,
	options map[string]option,
	taken map[string]bool,
) ([]models.ContactImportColumnMapping, []int) {
	type candidate struct {
		col  int
		opt  option
		id   string
		conf float64
	}
	var cands []candidate
	for id, a := range answers {
		col, ok := columnOf(id)
		if !ok || col >= len(mapping) || mapping[col].Target != models.ContactImportTargetIgnore {
			continue
		}
		if a.Confidence < ConfFloor || a.Choice == optionNone {
			continue
		}
		opt, ok := options[a.Choice]
		if !ok {
			continue
		}
		cands = append(cands, candidate{col: col, opt: opt, id: identity(opt), conf: a.Confidence})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].conf != cands[j].conf {
			return cands[i].conf > cands[j].conf
		}
		return cands[i].col < cands[j].col
	})

	out := append([]models.ContactImportColumnMapping(nil), mapping...)
	claimed := make(map[string]bool, len(taken))
	for k := range taken {
		claimed[k] = true
	}
	var inferred []int
	for _, c := range cands {
		if c.id != "" && claimed[c.id] {
			continue
		}
		if c.id != "" {
			claimed[c.id] = true
		}
		out[c.col] = models.ContactImportColumnMapping{Index: c.col, Target: c.opt.target, CustomKey: c.opt.customKey}
		inferred = append(inferred, c.col)
	}
	sort.Ints(inferred)
	return out, inferred
}

// buildOptions lists the fields still free, under short ids so no field name
// has to survive as an option key. criteria holds each id's description.
func buildOptions(taken map[string]bool, existingKeys []string) (map[string]option, map[string]string) {
	options := map[string]option{}
	criteria := map[string]string{}
	for _, s := range standardOptions {
		o := option{target: s.target}
		if taken[identity(o)] {
			continue
		}
		id := string(s.target)
		options[id] = o
		criteria[id] = s.desc
	}
	for i, k := range capKeys(existingKeys) {
		o := option{target: models.ContactImportTargetCustom, customKey: k}
		if taken[identity(o)] {
			continue
		}
		id := "field_" + strconv.Itoa(i+1)
		options[id] = o
		criteria[id] = "The workspace's existing custom field named \"" + k + "\""
	}
	criteria[optionNone] = "None of these: the column holds something else"
	return options, criteria
}

// columnCriteria drops the options a column's values rule out, so a column of
// URLs is never offered Subscribed.
func columnCriteria(all map[string]string, holds Shape) map[string]string {
	out := make(map[string]string, len(all))
	for id, desc := range all {
		if id == string(models.ContactImportTargetSubscribed) && holds != ShapeYesNo {
			continue
		}
		if id == string(models.ContactImportTargetEmail) && holds != ShapeEmail {
			continue
		}
		out[id] = desc
	}
	return out
}

// takenTargets is every destination the suggestion already fills.
func takenTargets(mapping []models.ContactImportColumnMapping) map[string]bool {
	taken := map[string]bool{}
	for _, m := range mapping {
		o := option{target: m.Target, customKey: m.CustomKey}
		if id := identity(o); id != "" {
			taken[id] = true
		}
	}
	return taken
}

// identity names where an option writes; "" for targets that take any number
// of columns (categories) or none (ignore).
func identity(o option) string {
	switch o.target {
	case models.ContactImportTargetIgnore, models.ContactImportTargetCategories, "":
		return ""
	case models.ContactImportTargetCustom:
		return "custom:" + o.customKey
	}
	return string(o.target)
}

func capKeys(keys []string) []string {
	if len(keys) > MaxCustomOptions {
		return keys[:MaxCustomOptions]
	}
	if keys == nil {
		return []string{}
	}
	return keys
}

// safeHeader is the header as sent. A header row that is really a data row
// ("dana@acme.com") is never sent: the column goes by its number instead.
func safeHeader(i int, h string) string {
	if ShapeOf([]string{h}) != ShapeText {
		return "Column " + strconv.Itoa(i+1)
	}
	if utf8.RuneCountInString(h) > headerRunes {
		h = string([]rune(h)[:headerRunes])
	}
	return h
}

func shapeAt(shapes []Shape, i int) Shape {
	if i < len(shapes) {
		return shapes[i]
	}
	return ShapeEmpty
}

func questionID(col int) string { return "column_" + strconv.Itoa(col+1) }

func columnOf(id string) (int, bool) {
	const prefix = "column_"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return 0, false
	}
	n, err := strconv.Atoi(id[len(prefix):])
	if err != nil || n < 1 {
		return 0, false
	}
	return n - 1, true
}
