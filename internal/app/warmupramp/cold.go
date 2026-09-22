package warmupramp

import (
	"time"

	"github.com/warmbly/warmbly/internal/models"
)

const (
	// ColdRampIncrement is how much a graduating mailbox may add per clean day.
	ColdRampIncrement = 5

	// Starting volumes by warmup maturity, following the documented cold
	// posture: a recently connected mailbox belongs near 10-20/day.
	coldStartUnproven = 5
	coldStartWarmed   = 10
	coldStartMature   = 20

	coldWarmedDays = 7
	coldMatureDays = 14
)

// ColdStart is the cold volume a mailbox graduates at, from how long it warmed.
func ColdStart(warmupDays int) int {
	switch {
	case warmupDays >= coldMatureDays:
		return coldStartMature
	case warmupDays >= coldWarmedDays:
		return coldStartWarmed
	default:
		return coldStartUnproven
	}
}

// ColdCeiling is a graduating mailbox's cold cap for today: its starting volume
// plus ColdRampIncrement per clean day since its first cold send, clamped to
// the mailbox's own cap. Placements freeze the climb through the same Days()
// union the warmup ramp uses. A zero rampStart means it has not sent cold mail
// yet, so it gets its starting volume.
func ColdCeiling(warmupDays int, rampStart time.Time, placements []time.Time, now time.Time, mailboxCap int) int {
	ceiling := ColdStart(warmupDays)
	if !rampStart.IsZero() {
		ceiling += Days(rampStart, placements, now, FreezeWindow) * ColdRampIncrement
	}
	if ceiling > mailboxCap {
		return mailboxCap
	}
	return ceiling
}

// Notice is the graduation notice the mailbox drawer and a campaign's send
// plan both show: today's ceiling, the configured cap, the clean days left
// before the two meet, and whether a placement is pausing the climb. One
// builder, so the two surfaces cannot round differently. Nil when the
// mailbox never warmed.
func Notice(warmupStartedAt, coldRampStartedAt *time.Time, placements []time.Time, mailboxCap int, now time.Time) *models.ColdRampInfo {
	if warmupStartedAt == nil {
		return nil
	}
	warmupDays := int(now.Sub(*warmupStartedAt).Hours() / 24)
	if warmupDays < 0 {
		warmupDays = 0
	}
	var rampStart time.Time
	if coldRampStartedAt != nil {
		rampStart = *coldRampStartedAt
	}
	ceiling := ColdCeiling(warmupDays, rampStart, placements, now, mailboxCap)
	left := max(0, mailboxCap-ceiling)
	days := left / ColdRampIncrement
	if left%ColdRampIncrement != 0 {
		days++
	}
	return &models.ColdRampInfo{
		Ceiling:       ceiling,
		MailboxCap:    mailboxCap,
		DaysToFullCap: days,
		Held:          ColdHeldUntil(rampStart, placements, now, FreezeWindow) != nil,
	}
}

// ColdHeldUntil is when the cold ramp resumes climbing, or nil when it already
// is. It filters placements the same way ColdCeiling does, so the number the
// dashboard shows and the reason it gives cannot disagree.
func ColdHeldUntil(rampStart time.Time, placements []time.Time, now time.Time, freeze time.Duration) *time.Time {
	if rampStart.IsZero() {
		return nil
	}
	counted := make([]time.Time, 0, len(placements))
	for _, p := range placements {
		if !p.Before(rampStart) {
			counted = append(counted, p)
		}
	}
	return FrozenUntil(counted, now, freeze)
}
