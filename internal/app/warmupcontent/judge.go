package warmupcontent

import (
	"context"
	"fmt"
	"strings"

	"github.com/warmbly/warmbly/internal/pkg/typesafe"
)

// The judgment thresholds. A generated thread that reads as marketing teaches
// the recipient's provider exactly the wrong thing about the sender, so a
// pitch is refused at the ordinary "yes" level while filler needs a strong
// answer: the deterministic lints already catch the obvious cases and the
// bank must not be emptied by a model being merely unsure.
const (
	// judgePitchYes is the noul above which the thread carries a pitch.
	judgePitchYes = 0.60
	// judgeTemplateStrong is the noul above which the thread reads as filler.
	judgeTemplateStrong = 0.80
	// judgeNaturalnessFloor is the normalized score below which the thread is
	// refused, but only when the model is confident in the reading.
	judgeNaturalnessFloor = 0.5
	// judgeConfFloor is the confidence below which the naturalness score is
	// not acted on at all.
	judgeConfFloor = 0.70
	// judgeStateLimit caps the runes sent as state; a thread is short, and a
	// runaway generation is not worth the tokens.
	judgeStateLimit = 6000
)

// The questions. Links and signup words are not asked about: warmlint checks
// those deterministically before this runs.
const (
	judgeQuestionPitch       = "contains_pitch"
	judgeQuestionTemplate    = "reads_as_template"
	judgeQuestionNaturalness = "naturalness"
)

var judgeNaturalnessLevels = []string{
	"Reads as generated filler",
	"Somewhat natural",
	"Reads as a real exchange between colleagues",
}

func judgeQuestions() map[string]typesafe.Question {
	return map[string]typesafe.Question{
		judgeQuestionPitch:       typesafe.Noul("The conversation contains a sales pitch, a promotion, a discount, or a call to action."),
		judgeQuestionTemplate:    typesafe.Noul("The conversation reads as generated filler rather than something two colleagues wrote."),
		judgeQuestionNaturalness: typesafe.Score("How natural is this exchange?", judgeNaturalnessLevels),
	}
}

// threadState is what the model sees: the thread and nothing else.
type threadState struct {
	Subject string   `json:"subject"`
	Opening string   `json:"opening"`
	Replies []string `json:"replies"`
}

// ThreadJudgment is the model's reading of one generated thread. Reject is
// the only thing that turns it into a decision.
type ThreadJudgment struct {
	Pitch       float64
	Template    float64
	Naturalness float64
	Confidence  float64
	Model       string
	InputTokens int
}

// Reject reports whether the thread stays out of the bank, and why.
func (j ThreadJudgment) Reject() (bool, string) {
	switch {
	case j.Pitch >= judgePitchYes:
		return true, fmt.Sprintf("contains a pitch (%.2f)", j.Pitch)
	case j.Template >= judgeTemplateStrong:
		return true, fmt.Sprintf("reads as generated filler (%.2f)", j.Template)
	case j.Naturalness < judgeNaturalnessFloor && j.Confidence >= judgeConfFloor:
		return true, fmt.Sprintf("unnatural exchange (%.2f at %.2f confidence)", j.Naturalness, j.Confidence)
	}
	return false, ""
}

// judgeThread asks all three questions in one call over the humanized thread.
func judgeThread(ctx context.Context, asker typesafe.Asker, subject, description string, messages []string) (*ThreadJudgment, error) {
	if asker == nil {
		return nil, fmt.Errorf("warmup judge: no asker")
	}
	state := boundThreadState(subject, description, messages, judgeStateLimit)
	resp, err := asker.Ask(ctx, state, judgeQuestions())
	if err != nil {
		return nil, err
	}
	pitch, ok := resp.Answers[judgeQuestionPitch]
	if !ok {
		return nil, fmt.Errorf("warmup judge: no %s answer", judgeQuestionPitch)
	}
	template, ok := resp.Answers[judgeQuestionTemplate]
	if !ok {
		return nil, fmt.Errorf("warmup judge: no %s answer", judgeQuestionTemplate)
	}
	natural, ok := resp.Answers[judgeQuestionNaturalness]
	if !ok {
		return nil, fmt.Errorf("warmup judge: no %s answer", judgeQuestionNaturalness)
	}
	return &ThreadJudgment{
		Pitch:       pitch.Noul,
		Template:    template.Noul,
		Naturalness: natural.Normalized(len(judgeNaturalnessLevels)),
		Confidence:  natural.Confidence,
		Model:       resp.Model,
		InputTokens: resp.Usage.InputTokens,
	}, nil
}

// boundThreadState builds the state with the total text capped at limit
// runes: the subject and opening first, then replies until the budget runs
// out, the last one cut rather than dropped.
func boundThreadState(subject, description string, messages []string, limit int) threadState {
	remaining := limit
	take := func(s string) string {
		if remaining <= 0 {
			return ""
		}
		if n := len([]rune(s)); n > remaining {
			s = string([]rune(s)[:remaining])
		}
		remaining -= len([]rune(s))
		return s
	}
	state := threadState{
		Subject: take(strings.TrimSpace(subject)),
		Opening: take(strings.TrimSpace(description)),
		Replies: make([]string, 0, len(messages)),
	}
	for _, m := range messages {
		if remaining <= 0 {
			break
		}
		if m = take(strings.TrimSpace(m)); m != "" {
			state.Replies = append(state.Replies, m)
		}
	}
	return state
}
