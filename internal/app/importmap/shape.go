package importmap

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/warmbly/warmbly/internal/email"
)

// Shape is the kind of value a column holds, worked out here from the sample
// so the model can tell a column apart without reading anyone's data.
type Shape string

const (
	ShapeEmpty    Shape = "empty"
	ShapeEmail    Shape = "email addresses"
	ShapeURL      Shape = "web addresses"
	ShapePhone    Shape = "phone numbers"
	ShapeNumber   Shape = "numbers"
	ShapeDate     Shape = "dates or times"
	ShapeYesNo    Shape = "yes or no values"
	ShapeText     Shape = "short text"
	ShapeLongText Shape = "long text"
	ShapeMixed    Shape = "a mix of kinds"
)

// dominantShare is how much of a column one kind must cover to name it.
const dominantShare = 0.8

// longTextRunes is where a cell stops being a name or a label.
const longTextRunes = 60

var (
	phoneRe  = regexp.MustCompile(`^\+?\(?[0-9][0-9 ().\-]{5,}[0-9]$`)
	numberRe = regexp.MustCompile(`^[-+]?[$€£]?[0-9][0-9,. ]*%?$`)
)

var dateLayouts = []string{
	time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", "2006-01-02 15:04", "01/02/2006", "02/01/2006",
	"1/2/2006", "2/1/2006", "02.01.2006", "Jan 02, 2006 3:04 PM", "Jan 2, 2006", "2 Jan 2006", "January 2, 2006",
}

var yesNo = map[string]bool{
	"yes": true, "no": true, "true": true, "false": true, "y": true, "n": true,
	"subscribed": true, "unsubscribed": true, "opted in": true, "opted out": true,
}

// ShapeOf names the kind of the non-empty values, or ShapeMixed when no kind
// covers most of them.
func ShapeOf(values []string) Shape {
	counts := map[Shape]int{}
	n := 0
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		counts[valueShape(v)]++
		n++
	}
	if n == 0 {
		return ShapeEmpty
	}
	best, bestN := ShapeMixed, 0
	for s, c := range counts {
		if c > bestN || (c == bestN && s < best) {
			best, bestN = s, c
		}
	}
	if float64(bestN) < dominantShare*float64(n) {
		return ShapeMixed
	}
	return best
}

// Shapes is ShapeOf for every column of a sample.
func Shapes(width int, sample [][]string) []Shape {
	out := make([]Shape, width)
	col := make([]string, 0, len(sample))
	for i := range out {
		col = col[:0]
		for _, row := range sample {
			if i < len(row) {
				col = append(col, row[i])
			}
		}
		out[i] = ShapeOf(col)
	}
	return out
}

func valueShape(v string) Shape {
	lower := strings.ToLower(v)
	switch {
	case yesNo[lower]:
		return ShapeYesNo
	case strings.Contains(v, "@") && email.IsValid(extractAddress(v)):
		return ShapeEmail
	case isURL(lower):
		return ShapeURL
	case isDate(v):
		return ShapeDate
	case isPhone(v):
		return ShapePhone
	case numberRe.MatchString(v):
		if _, err := strconv.ParseFloat(strings.NewReplacer(",", "", " ", "", "$", "", "€", "", "£", "", "%", "").Replace(v), 64); err == nil {
			return ShapeNumber
		}
	}
	if utf8.RuneCountInString(v) > longTextRunes {
		return ShapeLongText
	}
	return ShapeText
}

// extractAddress reads "Dana Reyes <dana@acme.com>" the way the importer does.
func extractAddress(v string) string {
	if a, ok := email.Normalize(v); ok {
		return a
	}
	return v
}

func isURL(v string) bool {
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "www.") {
		u, err := url.Parse(v)
		return err == nil && (u.Host != "" || strings.HasPrefix(v, "www."))
	}
	return false
}

// isPhone wants the punctuation people write numbers with (a leading +,
// brackets, spaces or dashes), so a bare 10-digit count stays a number.
func isPhone(v string) bool {
	if !phoneRe.MatchString(v) || digits(v) < 7 {
		return false
	}
	return strings.HasPrefix(v, "+") || strings.ContainsAny(v, " ()-")
}

func isDate(v string) bool {
	for _, l := range dateLayouts {
		if _, err := time.Parse(l, v); err == nil {
			return true
		}
	}
	return false
}

func digits(v string) int {
	n := 0
	for _, r := range v {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
