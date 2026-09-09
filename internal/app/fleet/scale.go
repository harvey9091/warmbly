package fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/jobrun"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// Scaler watches how full the fleet is and says so. It no longer buys
// machines: there is no cloud account to buy them with, and a node joins by
// running one command on a box you already have. What it still does is notice
// that capacity is running out before sending starts backing up, and write
// that to decision_log where an operator will see it.
type Scaler struct {
	WorkerRepo     repository.WorkerRepository
	Decisions      repository.DecisionLogRepository
	Interval       time.Duration // default 1h
	CriticalThresh float64       // default 0.85
	WarningThresh  float64       // default 0.70
}

func (s *Scaler) defaults() {
	if s.Interval == 0 {
		s.Interval = time.Hour
	}
	if s.CriticalThresh == 0 {
		s.CriticalThresh = 0.85
	}
	if s.WarningThresh == 0 {
		s.WarningThresh = 0.70
	}
}

func (s *Scaler) Run(ctx context.Context) {
	s.defaults()
	// Runs once on boot so an admin does not wait an hour for the first signal.
	jobrun.Loop(ctx, "fleet_scale", s.Interval, true, s.tick)
}

func (s *Scaler) tick(ctx context.Context) error {
	rows, err := s.WorkerRepo.ListCapacityCandidates(ctx, []models.WorkerHealthState{
		models.WorkerHealthHealthy,
		models.WorkerHealthWatch,
	})
	if err != nil {
		return err
	}

	var totalLoad, totalCap float64
	for _, row := range rows {
		eff := row.BaseCapacity * row.HealthMultiplier * row.AgeMultiplier
		if eff <= 0 {
			eff = 1
		}
		totalCap += eff
		totalLoad += row.LoadScore
	}

	var util float64
	if totalCap > 0 {
		util = totalLoad / totalCap
	}

	severity := ""
	switch {
	case util >= s.CriticalThresh:
		severity = "critical"
	case util >= s.WarningThresh:
		severity = "warning"
	}

	if severity == "" {
		return nil
	}

	alertReason := fmt.Sprintf("fleet utilization %.0f%% (load=%.1f cap=%.1f)", util*100, totalLoad, totalCap)
	_ = s.Decisions.Insert(ctx, &repository.DecisionLog{
		Kind:        "scale_alert",
		Reason:      alertReason,
		TriggeredBy: "auto:scale",
	})
	log.Warn().
		Str("severity", severity).
		Float64("utilization", util).
		Msg(alertReason)

	if severity == "critical" {
		_ = s.Decisions.Insert(ctx, &repository.DecisionLog{
			Kind:        "scale_alert",
			Reason:      "fleet is nearly full; add a node with `warmbly join` on any machine you own",
			TriggeredBy: "auto:scale",
		})
	}

	return nil
}
