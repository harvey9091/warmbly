package scheduler

import (
	"context"
	"testing"

	"github.com/warmbly/warmbly/internal/models"
)

// The plan is read through the real repositories: the sender ledger, the
// routing over every lead, the organization's day. What it says has to add
// up, and has to agree with what the fixture put on the books. Same harness
// and env var as live_integration_test.go.
func TestLivePlanCampaignDayAddsUp(t *testing.T) {
	_, pool := liveDB(t)
	f := newLiveFixture(t, pool, "UTC")
	f.addSentLead(t)
	planner, ok := loggedScheduler(t, f).(CampaignSendPlanner)
	if !ok {
		t.Fatal("the scheduler service does not plan")
	}

	plan, err := planner.PlanCampaignDay(context.Background(), f.campaign, 5)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ConfiguredCeiling != 50 || plan.SentToday != 1 {
		t.Fatalf("ceiling %d sent %d, want 50 and the one send on the books", plan.ConfiguredCeiling, plan.SentToday)
	}
	removed := 0
	for _, l := range plan.Limits {
		if l.Emails <= 0 {
			t.Fatalf("limit %q listed with nothing removed", l.Kind)
		}
		removed += l.Emails
	}
	if plan.ConfiguredCeiling-removed-plan.SentToday != plan.ExpectedRemaining {
		t.Fatalf("the waterfall does not add up: %d - %d - %d != %d (%+v)",
			plan.ConfiguredCeiling, removed, plan.SentToday, plan.ExpectedRemaining, plan.Limits)
	}
	if plan.Projected != plan.SentToday+plan.ExpectedRemaining {
		t.Fatalf("projected %d != sent %d + remaining %d", plan.Projected, plan.SentToday, plan.ExpectedRemaining)
	}
	// The fixture's own lead is the only step due: the sent lead's flow is
	// over, so however much the mailbox could send, one is all there is.
	if plan.Leads.DueNow != 1 || plan.ExpectedRemaining > 1 {
		t.Fatalf("leads %+v remaining %d, want exactly one due and at most one to go", plan.Leads, plan.ExpectedRemaining)
	}
	if plan.ExpectedRemaining == 1 && plan.Bottleneck != models.SendLimitLeads {
		t.Fatalf("bottleneck %q, want the leads to be what binds", plan.Bottleneck)
	}
	if plan.Organization == nil || plan.Organization.DailyLimit != 5 || plan.Organization.SentToday != 1 || plan.Organization.Remaining != 4 {
		t.Fatalf("organization %+v, want the plan's allowance of 5 with one used", plan.Organization)
	}
	if len(plan.Mailboxes) != 1 {
		t.Fatalf("got %d mailboxes, want the fixture's one", len(plan.Mailboxes))
	}
	mb := plan.Mailboxes[0]
	if mb.ID != f.mailbox || mb.CapToday != 50 || mb.LimitedBy != capByMailbox || mb.SentToday != 1 || mb.SentByOtherCampaigns != 0 || mb.MinGapSeconds != 600 {
		t.Fatalf("mailbox row %+v", mb)
	}
	if !plan.Window.SendingDay {
		t.Fatalf("an always-open campaign reported no sending day: %+v", plan.Window)
	}
}

// A campaign that is not running still says what it would send, and says
// that the campaign not running is what stops it.
func TestLivePlanCampaignDayForAPausedCampaign(t *testing.T) {
	_, pool := liveDB(t)
	f := newLiveFixture(t, pool, "UTC")
	if _, err := pool.Exec(context.Background(), `UPDATE campaigns SET status = 'paused' WHERE id = $1`, f.campaign); err != nil {
		t.Fatal(err)
	}
	planner := loggedScheduler(t, f).(CampaignSendPlanner)
	plan, err := planner.PlanCampaignDay(context.Background(), f.campaign, -1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExpectedRemaining != 0 || plan.Bottleneck != models.SendLimitNotRunning || plan.Organization != nil || plan.NextWakeAt != nil {
		t.Fatalf("paused campaign planned %+v", plan)
	}
	held := 0
	for _, l := range plan.Limits {
		if l.Kind == models.SendLimitNotRunning {
			held = l.Emails
		}
	}
	if held < 1 {
		t.Fatalf("the not-running clamp removed %d, want what the pool would otherwise send (%+v)", held, plan.Limits)
	}
}
