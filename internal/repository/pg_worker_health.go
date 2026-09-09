package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/models"
)

// WorkerCapacityRowDB mirrors a single row of worker_capacity_view. It
// intentionally lives in the repository package (and not in models) so
// that the materialized view's column shape can evolve without breaking
// app-layer types: app code consumes the workerapp.WorkerCapacityRow,
// which the assignment service builds from this DB row.
type WorkerCapacityRowDB struct {
	WorkerID         uuid.UUID
	Region           string
	HealthState      models.WorkerHealthState
	LoadScore        float64
	BaseCapacity     float64
	HealthMultiplier float64
	AgeMultiplier    float64
	SendsAttempted1h int64
	SendsSucceeded1h int64
	BouncesHard1h    int64
	BouncesSoft1h    int64
	Complaints1h     int64
	AuthErrors1h     int64
}

// InsertWorkerHealthSample appends one telemetry row. Called by the
// consumer side once per incoming WorkerHealth event.
func (r *workerRepository) InsertWorkerHealthSample(ctx context.Context, sample *models.WorkerHealthSample) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO worker_health_samples (
			worker_id, observed_at,
			assigned_count, imap_idle_count, memory_mb, goroutine_count,
			sends_attempted, sends_succeeded,
			bounces_hard, bounces_soft, complaints,
			auth_errors, rate_limit_errors,
			smtp_latency_p50_ms, smtp_latency_p99_ms
		) VALUES (
			$1, $2,
			$3, $4, $5, $6,
			$7, $8,
			$9, $10, $11,
			$12, $13,
			$14, $15
		)
	`,
		sample.WorkerID, sample.ObservedAt,
		sample.AssignedCount, sample.ImapIdleCount, sample.MemoryMB, sample.GoroutineCount,
		sample.SendsAttempted, sample.SendsSucceeded,
		sample.BouncesHard, sample.BouncesSoft, sample.Complaints,
		sample.AuthErrors, sample.RateLimitErrors,
		sample.SMTPLatencyP50Ms, sample.SMTPLatencyP99Ms,
	)
	return err
}

// ListCapacityCandidates returns the rows the assignment loop should pick
// from. health_state filtering happens here so a quarantined worker never even
// appears as a candidate; load + capacity filtering happens in app code so we
// can keep the SQL stable across changes to the placement math.
//
// The view's WHERE n.active clause already excludes deactivated nodes.
// The result is unordered; the app layer sorts by utilization ratio.
func (r *workerRepository) ListCapacityCandidates(
	ctx context.Context,
	allowedStates []models.WorkerHealthState,
) ([]WorkerCapacityRowDB, error) {
	if len(allowedStates) == 0 {
		allowedStates = []models.WorkerHealthState{
			models.WorkerHealthHealthy,
			models.WorkerHealthWatch,
		}
	}
	states := make([]string, 0, len(allowedStates))
	for _, s := range allowedStates {
		states = append(states, string(s))
	}

	rows, err := r.db.Query(ctx, `
		SELECT v.worker_id, v.region, v.health_state,
		       v.load_score, v.base_capacity, v.health_multiplier, v.age_multiplier,
		       v.sends_attempted_1h, v.sends_succeeded_1h,
		       v.bounces_hard_1h, v.bounces_soft_1h, v.complaints_1h, v.auth_errors_1h
		  FROM worker_capacity_view v
		  JOIN fleet_nodes n ON n.id = v.worker_id
		 WHERE v.health_state = ANY($1::text[])
		   AND n.active
		   AND n.last_seen_at > now() - $2::interval
	`, states, WorkerLivenessWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkerCapacityRowDB
	for rows.Next() {
		var c WorkerCapacityRowDB
		if err := rows.Scan(
			&c.WorkerID, &c.Region, &c.HealthState,
			&c.LoadScore, &c.BaseCapacity, &c.HealthMultiplier, &c.AgeMultiplier,
			&c.SendsAttempted1h, &c.SendsSucceeded1h,
			&c.BouncesHard1h, &c.BouncesSoft1h, &c.Complaints1h, &c.AuthErrors1h,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCapacityRow fetches a single row by worker ID. Useful for follow-up
// reads after an assignment when the caller wants to log or audit the
// utilization that motivated the decision.
func (r *workerRepository) GetCapacityRow(ctx context.Context, workerID uuid.UUID) (*WorkerCapacityRowDB, error) {
	var c WorkerCapacityRowDB
	err := r.db.QueryRow(ctx, `
		SELECT worker_id, region, health_state,
		       load_score, base_capacity, health_multiplier, age_multiplier,
		       sends_attempted_1h, sends_succeeded_1h,
		       bounces_hard_1h, bounces_soft_1h, complaints_1h, auth_errors_1h
		  FROM worker_capacity_view
		 WHERE worker_id = $1
	`, workerID).Scan(
		&c.WorkerID, &c.Region, &c.HealthState,
		&c.LoadScore, &c.BaseCapacity, &c.HealthMultiplier, &c.AgeMultiplier,
		&c.SendsAttempted1h, &c.SendsSucceeded1h,
		&c.BouncesHard1h, &c.BouncesSoft1h, &c.Complaints1h, &c.AuthErrors1h,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// AddLoadScore adjusts a worker's load_score by delta. Positive deltas
// land on assignment, negative deltas on unassign. Done with a single
// UPDATE so it's atomic; the GREATEST clause keeps load_score from
// dipping below zero if a stale decrement arrives after the row already
// hit floor.
func (r *workerRepository) AddLoadScore(ctx context.Context, workerID uuid.UUID, delta float64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE workers
		   SET load_score = GREATEST(0, load_score + $2),
		       updated_at = NOW()
		 WHERE id = $1
	`, workerID, delta)
	return err
}

// SetWorkerHealthState writes a new health label. Driven by the
// placement loop, not by workers themselves: a worker reporting bad
// health doesn't get to declare itself blocked, the control plane
// decides.
func (r *workerRepository) SetWorkerHealthState(ctx context.Context, workerID uuid.UUID, state models.WorkerHealthState) error {
	_, err := r.db.Exec(ctx, `
		UPDATE workers
		   SET health_state = $2,
		       updated_at = NOW()
		 WHERE id = $1
	`, workerID, state)
	return err
}

// RefreshWorkerCapacityView refreshes the materialized view. CONCURRENTLY
// requires PG14+ and the unique index on worker_id created by the migration.
// Run from a backend cron (every minute or so); the placement loop reads stale
// data between refreshes, which is fine because load_score is the
// freshness-critical signal and that lives on workers directly.
func (r *workerRepository) RefreshWorkerCapacityView(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `REFRESH MATERIALIZED VIEW CONCURRENTLY worker_capacity_view`)
	return err
}

// GetEmailAccountPlacementHint returns the provider + warmup flag the
// placement service needs to weight a mailbox. Returns nil if the account
// isn't found, which the caller treats as "use the default weight" rather than
// erroring out: failing to place is worse than over-counting load slightly.
func (r *workerRepository) GetEmailAccountPlacementHint(ctx context.Context, emailAccountID uuid.UUID) (*EmailAccountPlacementHint, error) {
	var provider string
	var warmup *string
	err := r.db.QueryRow(ctx, `
		SELECT provider, warmup::text
		  FROM email_accounts
		 WHERE id = $1
	`, emailAccountID).Scan(&provider, &warmup)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &EmailAccountPlacementHint{
		Provider: provider,
		IsWarmup: warmup != nil,
	}, nil
}
