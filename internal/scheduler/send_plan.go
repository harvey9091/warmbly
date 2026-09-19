package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/app/behavior"
	"github.com/warmbly/warmbly/internal/app/warmupramp"
	"github.com/warmbly/warmbly/internal/models"
)

// CampaignSendPlanner is satisfied by the scheduler service.
type CampaignSendPlanner interface {
	// PlanCampaignDay is today's sending plan for one campaign: what will go
	// out and every limit that decided it. orgDailyLimit is the workspace's
	// plan-level daily campaign limit, negative when unlimited. Read-only.
	PlanCampaignDay(ctx context.Context, campaignID uuid.UUID, orgDailyLimit int) (*models.CampaignSendPlan, error)
	// PoolCapacityToday is what a mailbox pool can send today under a
	// campaign's clamps, for a campaign that may not exist yet (the wizard's
	// estimate) or for the whole workspace (the dashboard meter). A campaign
	// with no daily limit clamps nothing at the campaign level.
	PoolCapacityToday(ctx context.Context, campaign *models.Campaign, accounts []models.Email) (*models.WorkspaceSendCapacity, error)
}

// mailboxDay is one mailbox's day on a campaign with the working shown: every
// clamp in the order the send path applies it, and how many sends it took.
// The deltas add up: room minus the deltas is remaining.
type mailboxDay struct {
	acct   models.Email
	cap    capClamp
	health healthRead
	gate   mailboxGate

	configured          int
	sentThis, sentOther int
	remaining           int
	byCampaignLimit     int
	byRamp              int
	byGraduation        int
	byRisk              int
	byGate              int
	byOther             int
	byHealthPace        int
	byHours             int
	byBehavior          int
	bySpacing           int
	state               string
	reopensAt           time.Time
	minGap              int
	graduation          *models.ColdRampInfo
}

// stagedCap is explainCap with every intermediate cap kept, so a plan can say
// how many sends each clamp took. The final value is explainCap's; the test
// holds the two together.
func stagedCap(p *campaignPass, acct models.Email) (stages [5]int, limitedBy string) {
	c := p.campaign
	stages[0] = acct.CampaignLimit
	limitedBy = capByMailbox
	cur := stages[0]
	clamp := func(i int, v int, why string) {
		if v < cur {
			cur, limitedBy = v, why
		}
		stages[i] = cur
	}
	if c.DailyLimit > 0 {
		clamp(1, c.DailyLimit, capByCampaign)
	} else {
		stages[1] = cur
	}
	if c.RampEnabled {
		clamp(2, campaignRampCeiling(true, c.RampStart, c.RampIncrement, c.RampCeiling, c.RampLevel), capByRamp)
	} else {
		stages[2] = cur
	}
	clamp(3, coldCeilingFor(p.coldRamp[acct.ID], cur), capByGraduation)
	if m := p.risk.CapMultiplier(); m < 1 {
		risked := int(float64(cur)*m + 0.5)
		if risked < 1 {
			risked = 1
		}
		clamp(4, risked, capByRisk)
	} else {
		stages[4] = cur
	}
	return stages, limitedBy
}

// room is what a cap leaves after this campaign's own sends.
func room(capv, sentThis int) int {
	return max(0, capv-sentThis)
}

// planMailbox walks one mailbox through the day. windowSecondsLeft is the
// campaign's sending time still ahead today; zero means the window is closed
// for the rest of the day and the campaign-level clamp reports it instead.
func (s *schedulerService) planMailbox(ctx context.Context, pass *campaignPass, acct models.Email, sentThis, sentAll int, now time.Time, windowSecondsLeft int, windowClosesAt time.Time) mailboxDay {
	// A cap lowered after sends went out counts what went out, so the
	// waterfall's arithmetic holds on the very day the cap moved.
	d := mailboxDay{acct: acct, configured: max(acct.CampaignLimit, sentThis), sentThis: sentThis, sentOther: max(0, sentAll-sentThis), minGap: acct.MinWaitTime}
	stages, limitedBy := stagedCap(pass, acct)
	d.cap = capClamp{Cap: stages[4], LimitedBy: limitedBy}
	r := room(d.configured, sentThis)
	step := func(capv int) int {
		next := room(capv, sentThis)
		delta := r - next
		r = next
		return delta
	}
	d.byCampaignLimit = step(stages[1])
	d.byRamp = step(stages[2])
	d.byGraduation = step(stages[3])
	d.byRisk = step(stages[4])
	if st, ok := pass.coldRamp[acct.ID]; ok && stages[3] < stages[2] {
		d.graduation = warmupramp.Notice(st.WarmupStartedAt, st.ColdRampStartedAt, st.Placements, stages[2], now)
	}

	// Standing gates: authentication, cold rotation, warmup health. Asked with
	// a nominal budget so the answer is the gate alone; the budget is applied
	// below where its delta can be named.
	gate, _ := s.gateFor(ctx, pass, acct, 1)
	d.health = s.healthFor(ctx, pass, acct.ID)
	if !gate.open() && gate.reason != gateBudget {
		d.gate = gate
		d.byGate = r
		r = 0
		switch gate.reason {
		case gateAuth:
			d.state = models.MailboxPlanDomainAuth
		case gateResting:
			d.state = models.MailboxPlanResting
		default:
			d.state = models.MailboxPlanHealthHold
		}
		return d
	}
	// Other campaigns share this mailbox's cap; what they sent is gone.
	next := max(0, r-d.sentOther)
	d.byOther = r - next
	r = next
	if r == 0 {
		d.state = models.MailboxPlanBudgetSpent
		d.gate = mailboxGate{reason: gateBudget, paced: true}
		return d
	}
	// A watch or throttled band spaces sends wider; the pass reads the
	// dampened budget, so the day lands near this.
	if d.health.known {
		if m := adjustmentFor(d.health.state).volumeMultiplier; m < 1 {
			next = int(float64(r) * m)
			d.byHealthPace = r - next
			r = next
		}
	}

	// The mailbox's own clock: a behaviour profile's rolled workday, or the
	// 8am-8pm band in its own timezone when that differs from the campaign's.
	secondsLeft := windowSecondsLeft
	bhv := pass.behaviors[acct.ID]
	if bhv.Enabled {
		// Today's rolled budget first: placeWithinBehavior walks to tomorrow
		// when it is spent, and that is the plan binding, not the hours.
		today := bhv.PlanOn(behavior.PlanDateFor(now, bhv.Loc))
		if today.IsWorkingDay && behavior.MinuteOfDay(now, bhv.Loc) < today.WorkEndMinute {
			if s.behaviorDailyCap(ctx, bhv, r, now) == 0 {
				d.state = models.MailboxPlanBudgetSpent
				d.gate = mailboxGate{reason: gateBudget, paced: true}
				d.byBehavior = r
				return d
			}
		}
		openAt, ok := s.placeWithinBehavior(ctx, bhv, now)
		if !ok {
			d.state = models.MailboxPlanNoWorkingDay
			d.gate = mailboxGate{reason: gateNoWorkday}
			d.byHours = r
			return d
		}
		if !sameLocalDay(openAt, now, bhv.Loc) {
			d.state = models.MailboxPlanHoursClosed
			d.reopensAt = openAt
			d.gate = mailboxGate{reason: gateHours, paced: true, reopensAt: openAt}
			d.byHours = r
			return d
		}
		if openAt.After(now) {
			d.reopensAt = openAt
			secondsLeft = min(secondsLeft, int(windowClosesAt.Sub(openAt).Seconds()))
		}
		next = min(r, s.behaviorDailyCap(ctx, bhv, r, openAt))
		d.byBehavior += r - next
		r = next
		if r == 0 {
			d.state = models.MailboxPlanBudgetSpent
			d.gate = mailboxGate{reason: gateBudget, paced: true}
			return d
		}
		d.minGap = s.behaviorGapFloor(bhv, openAt, acct.MinWaitTime)
		plan := bhv.PlanOn(behavior.PlanDateFor(openAt, bhv.Loc))
		local := openAt.In(bhv.Loc)
		workEnd := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, bhv.Loc).Add(time.Duration(plan.WorkEndMinute) * time.Minute)
		secondsLeft = min(secondsLeft, max(0, int(workEnd.Sub(openAt).Seconds())))
		// The hourly ceiling is the plan's own clamp, so it is charged to it.
		if plan.HourlyLimit > 0 {
			hours := (secondsLeft + 3599) / 3600
			if byHour := plan.HourlyLimit * hours; byHour < r {
				d.byBehavior += r - byHour
				r = byHour
			}
		}
	} else if acct.Timezone != "" && acct.Timezone != pass.campaign.Timezone {
		// The 8am-8pm band in the mailbox's own timezone, both ends: the
		// placer moves any send past 8pm to the next morning.
		loc := loadLocation(acct.Timezone)
		local := now.In(loc)
		if h := local.Hour(); h < 8 || h >= 20 {
			open := businessHoursReopen(now, loc)
			d.reopensAt = open
			if !sameLocalDay(open, now, loc) || !open.Before(windowClosesAt) {
				d.state = models.MailboxPlanHoursClosed
				d.gate = mailboxGate{reason: gateHours, paced: true, reopensAt: open}
				d.byHours = r
				return d
			}
			secondsLeft = min(secondsLeft, int(windowClosesAt.Sub(open).Seconds()))
		}
		bandEnd := time.Date(local.Year(), local.Month(), local.Day(), 20, 0, 0, 0, loc)
		if bandEnd.Before(windowClosesAt) {
			secondsLeft = min(secondsLeft, max(0, int(bandEnd.Sub(now).Seconds())))
		}
	}

	if windowSecondsLeft <= 0 {
		// The campaign's window is closed for the day; that clamp is reported
		// once for the whole pool rather than on every mailbox.
		d.state = models.MailboxPlanWindowClosed
		d.remaining = r
		return d
	}

	// Spacing: one send per min gap, from the later of now and the mailbox's
	// last send plus its gap. Warmup mail sits on the same clock.
	var earliest time.Time
	if last, err := s.taskRepo.GetLastEmailTime(ctx, acct.ID); err == nil && last != nil && d.minGap > 0 {
		if at := last.Add(time.Duration(d.minGap) * time.Second); at.After(now) {
			earliest = at
			secondsLeft -= int(at.Sub(now).Seconds())
		}
	}
	paceMax := 0
	if secondsLeft >= 0 {
		paceMax = 1
		if d.minGap > 0 {
			paceMax += secondsLeft / d.minGap
		} else {
			paceMax = r
		}
	}
	next = min(r, paceMax)
	d.bySpacing = r - next
	r = next
	d.remaining = r
	if r > 0 {
		d.state = models.MailboxPlanSending
		return d
	}
	// Its next allowed send falls after the day's sending time ends.
	d.state = models.MailboxPlanHoursClosed
	if !earliest.IsZero() {
		d.reopensAt = earliest
	}
	return d
}

// dayWindow is the campaign's calendar for today, in its own timezone.
func dayWindow(campaign *models.Campaign, now time.Time) (models.CampaignSendWindow, int, time.Time) {
	tz := loadLocation(campaign.Timezone)
	windows := effectiveWindows(campaign)
	local := now.In(tz)
	y, m, d := local.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, tz)
	out := models.CampaignSendWindow{}
	if campaign.EndDate != nil {
		t := *campaign.EndDate
		out.EndsAt = &t
	}
	if campaign.StartDate != nil && campaign.StartDate.After(now) {
		t := *campaign.StartDate
		out.StartsAt = &t
	}
	if campaign.EndDate != nil && campaign.EndDate.Before(now) {
		return out, 0, midnight
	}
	if windows.IsEmpty() {
		out.SendingDay = true
		out.OpenNow = out.StartsAt == nil || !campaign.StartDate.After(now)
		closes := midnight.AddDate(0, 0, 1)
		out.ClosesAt = &closes
		from := now
		if out.StartsAt != nil {
			from = *out.StartsAt
		}
		left := 0
		if from.Before(closes) {
			left = int(closes.Sub(from).Seconds())
		}
		if !out.OpenNow {
			out.OpensAt = out.StartsAt
		}
		out.MinutesLeft = left / 60
		return out, left, closes
	}
	nowMin := local.Hour()*60 + local.Minute()
	closes := midnight
	left := 0
	for _, iv := range windows[int(local.Weekday())] {
		out.SendingDay = true
		end := midnight.Add(time.Duration(iv.End) * time.Minute)
		if end.After(closes) {
			closes = end
		}
		if nowMin >= iv.Start && nowMin < iv.End {
			out.OpenNow = true
		}
		if iv.End > nowMin {
			left += (iv.End - max(iv.Start, nowMin)) * 60
		}
	}
	if out.SendingDay {
		out.ClosesAt = &closes
	}
	// A start date still ahead holds the whole window, whatever the clock says.
	if out.StartsAt != nil {
		out.OpenNow = false
		if out.StartsAt.Before(closes) {
			from := max(nowMin, out.StartsAt.In(tz).Hour()*60+out.StartsAt.In(tz).Minute())
			left = 0
			for _, iv := range windows[int(local.Weekday())] {
				if iv.End > from {
					left += (iv.End - max(iv.Start, from)) * 60
				}
			}
		} else {
			left = 0
		}
	}
	if !out.OpenNow {
		from := now
		if out.StartsAt != nil && out.StartsAt.After(from) {
			from = *out.StartsAt
		}
		if open := nextScheduleSlot(from, windows, tz); open.After(now) {
			out.OpensAt = &open
		}
	}
	out.MinutesLeft = left / 60
	return out, left, closes
}

// PlanCampaignDay implements CampaignSendPlanner.
func (s *schedulerService) PlanCampaignDay(ctx context.Context, campaignID uuid.UUID, orgDailyLimit int) (*models.CampaignSendPlan, error) {
	campaign, err := s.campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	projectRampLevel(campaign, now)
	tz := loadLocation(campaign.Timezone)
	window, windowSecondsLeft, closesAt := dayWindow(campaign, now)

	plan := &models.CampaignSendPlan{
		CampaignID: campaign.ID,
		Status:     campaign.Status,
		Day:        now.UTC().Format("2006-01-02"),
		Timezone:   tz.String(),
		ComputedAt: now,
		Window:     window,
		Limits:     []models.CampaignSendLimit{},
		Mailboxes:  []models.CampaignMailboxPlan{},
	}

	accounts, _, err := s.campaignSenders(ctx, campaign)
	if err != nil {
		return nil, err
	}
	pass := s.newCampaignPass(ctx, campaign, accounts)
	if err := s.prefillSentToday(ctx, pass, accounts); err != nil {
		return nil, err
	}
	sentBySender, err := s.taskRepo.CountCampaignSendsTodayBySender(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	days := make([]mailboxDay, 0, len(accounts))
	for _, acct := range accounts {
		sentAll, err := s.sentTodayFor(ctx, pass, acct.ID)
		if err != nil {
			return nil, err
		}
		days = append(days, s.planMailbox(ctx, pass, acct, sentBySender[acct.ID], sentAll, now, windowSecondsLeft, closesAt))
	}

	// The pool's waterfall: each clamp's deltas summed over the mailboxes.
	type tally struct {
		kind      string
		emails    int
		mailboxes int
	}
	tallies := []tally{
		{kind: models.SendLimitCampaignDailyLimit}, {kind: models.SendLimitCampaignRamp},
		{kind: models.SendLimitWarmupGraduation}, {kind: models.SendLimitWorkspaceRisk},
		{kind: models.SendLimitDomainAuth}, {kind: models.SendLimitResting}, {kind: models.SendLimitHealthHold},
		{kind: models.SendLimitOtherCampaigns}, {kind: models.SendLimitHealthPace},
		{kind: models.SendLimitMailboxHours}, {kind: models.SendLimitSendingBehavior}, {kind: models.SendLimitSpacing},
	}
	add := func(kind string, n int) {
		if n <= 0 {
			return
		}
		for i := range tallies {
			if tallies[i].kind == kind {
				tallies[i].emails += n
				tallies[i].mailboxes++
				return
			}
		}
	}
	poolRemaining := 0
	for _, d := range days {
		plan.ConfiguredCeiling += d.configured
		plan.SentToday += d.sentThis
		poolRemaining += d.remaining
		add(models.SendLimitCampaignDailyLimit, d.byCampaignLimit)
		add(models.SendLimitCampaignRamp, d.byRamp)
		add(models.SendLimitWarmupGraduation, d.byGraduation)
		add(models.SendLimitWorkspaceRisk, d.byRisk)
		switch d.gate.reason {
		case gateAuth:
			add(models.SendLimitDomainAuth, d.byGate)
		case gateResting:
			add(models.SendLimitResting, d.byGate)
		case gateHealth:
			add(models.SendLimitHealthHold, d.byGate)
		}
		add(models.SendLimitOtherCampaigns, d.byOther)
		add(models.SendLimitHealthPace, d.byHealthPace)
		add(models.SendLimitMailboxHours, d.byHours)
		add(models.SendLimitSendingBehavior, d.byBehavior)
		add(models.SendLimitSpacing, d.bySpacing)

		mp := models.CampaignMailboxPlan{
			ID: d.acct.ID, Email: d.acct.Email, Provider: d.acct.Provider,
			ConfiguredCap: d.configured, CapToday: d.cap.Cap, LimitedBy: d.cap.LimitedBy,
			SentToday: d.sentThis, SentByOtherCampaigns: d.sentOther,
			ExpectedRemaining: d.remaining, State: d.state, MinGapSeconds: d.minGap, Graduation: d.graduation,
		}
		if !d.reopensAt.IsZero() {
			t := d.reopensAt
			mp.ReopensAt = &t
		}
		if d.health.known && d.health.state != "" && d.health.state != models.WarmupHealthHealthy {
			mp.Health = string(d.health.state)
		}
		plan.Mailboxes = append(plan.Mailboxes, mp)
	}
	for _, t := range tallies {
		if t.emails > 0 {
			plan.Limits = append(plan.Limits, models.CampaignSendLimit{Kind: t.kind, Emails: t.emails, Mailboxes: t.mailboxes})
		}
	}
	// The biggest mailbox-level clamp is the story when nothing below binds.
	bottleneck := ""
	biggest := 0
	for _, t := range tallies {
		if t.emails > biggest {
			biggest, bottleneck = t.emails, t.kind
		}
	}
	campaignLimit := func(kind string, n int) {
		if n <= 0 {
			return
		}
		plan.Limits = append(plan.Limits, models.CampaignSendLimit{Kind: kind, Emails: n})
		bottleneck = kind
	}

	// Campaign-level clamps, in the order the send path meets them.
	remaining := poolRemaining
	if windowSecondsLeft <= 0 {
		campaignLimit(models.SendLimitSendingWindow, remaining)
		remaining = 0
	}
	if campaign.Status != "active" || pass.risk.BlocksSending() {
		campaignLimit(models.SendLimitNotRunning, remaining)
		remaining = 0
	}
	if orgDailyLimit >= 0 && campaign.OrganizationID != nil {
		sent, err := s.campaignProgressRepo.CountEmailsSentTodayByOrganization(ctx, *campaign.OrganizationID)
		if err != nil {
			return nil, err
		}
		left := max(0, orgDailyLimit-sent)
		plan.Organization = &models.CampaignOrgAllowance{DailyLimit: orgDailyLimit, SentToday: sent, Remaining: left}
		if left < remaining {
			campaignLimit(models.SendLimitOrgDailyLimit, remaining-left)
			remaining = left
		}
	}

	// The leads: mailboxes can only send to a step that is due today, and a
	// lead bound to a mailbox with nothing left waits for it.
	unavailable := map[uuid.UUID]bool{}
	for _, d := range days {
		if d.remaining == 0 {
			unavailable[d.acct.ID] = true
		}
	}
	supply, err := s.campaignProgressRepo.LeadSupply(ctx, campaignID, closesAt, unavailable)
	if err != nil {
		return nil, err
	}
	newLeadsToday := 0
	if campaign.MaxNewLeadsPerDay > 0 {
		if n, err := s.campaignRepo.CountNewLeadsStartedToday(ctx, campaignID); err == nil {
			newLeadsToday = n
		}
	}
	plan.Leads = models.CampaignLeadSupply{
		DueNow: supply.DueNow, DueLaterToday: supply.DueLaterToday,
		NewLeadsDueToday: supply.DueNowNewLeads + supply.DueLaterTodayNewLeads,
		WaitingOnStep:    supply.WaitingOnStep, WaitingOnCondition: supply.WaitingOnCondition, Held: supply.Held,
		WaitingOnSender:      supply.WaitingOnSender,
		NewLeadsStartedToday: newLeadsToday, MaxNewLeadsPerDay: campaign.MaxNewLeadsPerDay, NextDueAt: supply.NextDueAt,
	}
	followUps := supply.DueNow + supply.DueLaterToday - plan.Leads.NewLeadsDueToday
	newDue := plan.Leads.NewLeadsDueToday
	if avail := followUps + newDue; avail < remaining {
		campaignLimit(models.SendLimitLeads, remaining-avail)
		remaining = avail
	}
	if campaign.MaxNewLeadsPerDay > 0 {
		roomForNew := max(0, campaign.MaxNewLeadsPerDay-newLeadsToday)
		if capped := followUps + min(newDue, roomForNew); capped < remaining {
			campaignLimit(models.SendLimitNewLeadCap, remaining-capped)
			remaining = capped
		}
	}

	plan.ExpectedRemaining = remaining
	plan.Projected = plan.SentToday + remaining
	if remaining == 0 && bottleneck == "" && plan.SentToday > 0 {
		bottleneck = models.SendBottleneckBudgetSpent
	}
	plan.Bottleneck = bottleneck
	if campaign.Status == "active" {
		plan.NextWakeAt = s.campaignWakeup(ctx, campaignID)
	}
	return plan, nil
}

// PoolCapacityToday implements CampaignSendPlanner.
func (s *schedulerService) PoolCapacityToday(ctx context.Context, campaign *models.Campaign, accounts []models.Email) (*models.WorkspaceSendCapacity, error) {
	if campaign == nil {
		campaign = &models.Campaign{}
	}
	pass := s.newCampaignPass(ctx, campaign, accounts)
	// One read for the whole pool. A miss is not an error here: a mailbox
	// whose sends are unknown counts toward capacity and nothing toward
	// remaining, which is the conservative side.
	_ = s.prefillSentToday(ctx, pass, accounts)
	out := &models.WorkspaceSendCapacity{}
	for _, acct := range accounts {
		out.ConfiguredCeiling += acct.CampaignLimit
		stages, _ := stagedCap(pass, acct)
		if gate, _ := s.gateFor(ctx, pass, acct, 1); !gate.open() && gate.reason != gateBudget {
			out.Held++
			continue
		}
		if bhv := pass.behaviors[acct.ID]; bhv.Enabled {
			if _, ok := s.placeWithinBehavior(ctx, bhv, time.Now()); !ok {
				out.Held++
				continue
			}
		}
		capv := stages[4]
		if capv <= 0 {
			out.Held++
			continue
		}
		out.Mailboxes++
		out.Capacity += capv
		sent, err := s.sentTodayFor(ctx, pass, acct.ID)
		if err != nil {
			continue
		}
		out.Remaining += max(0, capv-sent)
	}
	return out, nil
}

// prefillSentToday loads the pool's sends today in one query into the pass's
// memo, so the per-mailbox reads that follow cost nothing.
func (s *schedulerService) prefillSentToday(ctx context.Context, pass *campaignPass, accounts []models.Email) error {
	ids := make([]uuid.UUID, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	counts, err := s.taskRepo.CountCampaignEmailsSentTodayByAccounts(ctx, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		pass.sentToday[id] = counts[id]
	}
	return nil
}
