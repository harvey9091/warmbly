package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// An instant branch's target must be sendable through the whole scheduler, not
// only through routing: the placer re-derives the step wait and would put the
// delay back (issue #583).
func TestLiveInstantBranchTargetSendsWithoutItsStepWait(t *testing.T) {
	handle, pool := liveDB(t)
	ctx := context.Background()
	f := newEntryDelayFixture(t, pool, 0)

	target := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO sequences (id, campaign_id, organization_id, name, subject,
	        body_plain, body_html, wait_after, position, kind)
	      VALUES ($1, $2, $3, 'Step 2', 'Again', 'Hello again', '<p>Hello again</p>', 10, 1, 'email')`,
		target, f.campaign, f.org); err != nil {
		t.Fatalf("target step: %v", err)
	}
	setBranch := func(instant string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE sequences SET conditions = $2 WHERE id = $1`, f.step,
			`{"branches":[{"branch_id":"b1","target_step_id":"`+target.String()+`",`+instant+
				`"conditions":[{"field":"opened","operator":"within_days","value":3}]}]}`); err != nil {
			t.Fatalf("set branch: %v", err)
		}
	}
	// No instant field: the default the canvas draws as an instant edge.
	setBranch("")
	sent := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := pool.Exec(ctx, `INSERT INTO campaign_contact_progress (campaign_id, contact_id, sequence_id, sent_at, dispatched_at, opened_at)
	      VALUES ($1, $2, $3, $4, $4, $5)`, f.campaign, f.contact, f.step, sent, sent.Add(30*time.Minute)); err != nil {
		t.Fatalf("progress: %v", err)
	}

	s := liveScheduler(t, handle, pool)
	_, pair, _, err := s.CalculateNextCampaignTime(ctx, f.campaign)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if pair == nil || pair.SequenceID != target || !pair.Instant {
		t.Fatalf("want the instant target sendable now, got %+v", pair)
	}
	pv, perr := s.(ContactSendPreviewer).PreviewContactSend(ctx, f.campaign, f.contact)
	if perr != nil {
		t.Fatalf("preview: %v", perr)
	}
	if pv.NotBefore != nil && pv.NotBefore.After(time.Now().Add(time.Hour)) {
		t.Fatalf("preview holds the instant target until %s", pv.NotBefore.UTC())
	}

	// Opting out puts the target's ten-day wait back.
	setBranch(`"instant":false,`)
	at, pair, _, err := s.CalculateNextCampaignTime(ctx, f.campaign)
	if !errors.Is(err, ErrCampaignDeferred) || pair != nil {
		t.Fatalf("want the opted-out target deferred, got pair=%+v err=%v", pair, err)
	}
	if want := sent.Add(10 * 24 * time.Hour); at.Before(want.Add(-time.Minute)) {
		t.Fatalf("re-check at %s, want the step wait's end %s or later", at.UTC(), want)
	}
}
