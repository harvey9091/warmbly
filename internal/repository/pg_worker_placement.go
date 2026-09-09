package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/models"
)

// PlacementCandidateRow is a capacity row plus the neighbour counts the
// placement score reads. Resolved in one round trip because the placer runs on
// the mailbox-add path, where an extra query per candidate worker would be a
// per-onboarding cost.
type PlacementCandidateRow struct {
	WorkerCapacityRowDB

	// TotalMailboxes is every mailbox on the worker, from any org.
	TotalMailboxes int
	// OrgMailboxesHere is how many belong to the org being placed.
	OrgMailboxesHere int
	// ProviderMailboxesHere is how many share the provider being placed. Same
	// client IP plus same provider is the combination that earns a per-IP auth
	// throttle, so it is worth spreading.
	ProviderMailboxesHere int
}

// MailboxPlacementState is everything the rotation loop needs to decide
// whether one mailbox should leave its current worker.
type MailboxPlacementState struct {
	EmailAccountID uuid.UUID
	OrganizationID *uuid.UUID
	Provider       string
	IsWarmup       bool

	WorkerID   *uuid.UUID
	AssignedAt *time.Time

	WorkerActive bool
	WorkerLive   bool
	WorkerHealth models.WorkerHealthState
	// WorkerRegion is where the mailbox currently signs in from. Carried so a
	// migration can keep the sign-in geography stable, which is the whole
	// point of the region term.
	WorkerRegion string
	// WorkerUtilization is load / effective capacity, or 0 when the worker has
	// no capacity row yet.
	WorkerUtilization float64

	WorkerTotalMailboxes int
	WorkerOrgMailboxes   int

	// ReservedWorkerID is the worker reserved for this mailbox's organization,
	// when it has isolated egress. Nil for everyone else.
	ReservedWorkerID *uuid.UUID
	// WorkerReservedForOtherOrg is true when the mailbox sits on a worker some
	// OTHER organization has reserved. Those mailboxes are the ones that have
	// to leave for isolation to mean anything.
	WorkerReservedForOtherOrg bool
}

// Residency is how long the mailbox has sat on its current worker. Zero when
// unknown, which MayMove reads as "long enough" rather than "brand new".
func (s MailboxPlacementState) Residency(now time.Time) time.Duration {
	if s.AssignedAt == nil {
		return 0
	}
	d := now.Sub(*s.AssignedAt)
	if d < 0 {
		return 0
	}
	return d
}

const placementCandidateSelect = `
	SELECT v.worker_id, v.region, v.health_state,
	       v.load_score, v.base_capacity, v.health_multiplier, v.age_multiplier,
	       v.sends_attempted_1h, v.sends_succeeded_1h,
	       v.bounces_hard_1h, v.bounces_soft_1h, v.complaints_1h, v.auth_errors_1h,
	       COALESCE(neighbours.total_count, 0), COALESCE(neighbours.org_count, 0), COALESCE(neighbours.provider_count, 0)
	  FROM worker_capacity_view v
	  JOIN fleet_nodes node ON node.id = v.worker_id
	  LEFT JOIN LATERAL (
	      SELECT count(*) AS total_count,
	             count(*) FILTER (WHERE ea.organization_id = $1) AS org_count,
	             count(*) FILTER (WHERE ea.provider::text = $2) AS provider_count
	        FROM email_accounts ea
	       WHERE ea.worker_id = v.worker_id
	  ) neighbours ON true
	 WHERE v.health_state = ANY($3::text[])
	   AND node.active
	   AND node.last_seen_at > now() - $4::interval
`

// ListPlacementCandidates returns every worker that may host a mailbox for the
// given org and provider, with the neighbour counts attached. Filtering on
// capacity headroom and ranking both happen in app code (worker.SelectPlacement)
// so the scoring model can change without a migration.
func (r *workerRepository) ListPlacementCandidates(
	ctx context.Context,
	orgID uuid.UUID,
	provider string,
	allowedStates []models.WorkerHealthState,
) ([]PlacementCandidateRow, error) {
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

	rows, err := r.db.Query(ctx, placementCandidateSelect, orgID, provider, states, WorkerLivenessWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlacementCandidateRow
	for rows.Next() {
		var c PlacementCandidateRow
		if err := rows.Scan(
			&c.WorkerID, &c.Region, &c.HealthState,
			&c.LoadScore, &c.BaseCapacity, &c.HealthMultiplier, &c.AgeMultiplier,
			&c.SendsAttempted1h, &c.SendsSucceeded1h,
			&c.BouncesHard1h, &c.BouncesSoft1h, &c.Complaints1h, &c.AuthErrors1h,
			&c.TotalMailboxes, &c.OrgMailboxesHere, &c.ProviderMailboxesHere,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountOrgMailboxes is the denominator for the blast-radius term: how much of
// this customer's sending one worker would be carrying.
func (r *workerRepository) CountOrgMailboxes(ctx context.Context, orgID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM email_accounts WHERE organization_id = $1`, orgID).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

const mailboxPlacementStateSelect = `
	SELECT ea.id, ea.organization_id, ea.provider::text, (ea.warmup IS NOT NULL),
	       ea.worker_id, ea.worker_assigned_at,
	       COALESCE(node.active, false),
	       COALESCE(node.last_seen_at > now() - $1::interval, false),
	       COALESCE(w.health_state, 'healthy'),
	       COALESCE(node.region, ''),
	       COALESCE(v.load_score / GREATEST(v.base_capacity * v.health_multiplier * v.age_multiplier, 1), 0),
	       COALESCE(neighbours.total_count, 0), COALESCE(neighbours.org_count, 0),
	       own.worker_id,
	       (here.organization_id IS NOT NULL AND here.organization_id IS DISTINCT FROM ea.organization_id)
	  FROM email_accounts ea
	  LEFT JOIN workers w ON w.id = ea.worker_id
	  LEFT JOIN fleet_nodes node ON node.id = ea.worker_id
	  LEFT JOIN worker_capacity_view v ON v.worker_id = ea.worker_id
	  LEFT JOIN LATERAL (
	      SELECT count(*) AS total_count,
	             count(*) FILTER (WHERE peer.organization_id = ea.organization_id) AS org_count
	        FROM email_accounts peer
	       WHERE peer.worker_id = ea.worker_id
	  ) neighbours ON true
	  LEFT JOIN dedicated_worker_assignments own
	         ON own.organization_id = ea.organization_id AND own.released_at IS NULL
	  LEFT JOIN dedicated_worker_assignments here
	         ON here.worker_id = ea.worker_id AND here.released_at IS NULL
`

func scanMailboxPlacementState(row pgx.Row) (*MailboxPlacementState, error) {
	var s MailboxPlacementState
	err := row.Scan(
		&s.EmailAccountID, &s.OrganizationID, &s.Provider, &s.IsWarmup,
		&s.WorkerID, &s.AssignedAt,
		&s.WorkerActive, &s.WorkerLive, &s.WorkerHealth, &s.WorkerRegion,
		&s.WorkerUtilization,
		&s.WorkerTotalMailboxes, &s.WorkerOrgMailboxes,
		&s.ReservedWorkerID, &s.WorkerReservedForOtherOrg,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetMailboxPlacementState loads one mailbox's placement facts.
func (r *workerRepository) GetMailboxPlacementState(ctx context.Context, emailAccountID uuid.UUID) (*MailboxPlacementState, error) {
	s, err := scanMailboxPlacementState(
		r.db.QueryRow(ctx, mailboxPlacementStateSelect+` WHERE ea.id = $2`, WorkerLivenessWindow, emailAccountID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ListRotationCandidates returns assigned mailboxes whose current worker is
// worth re-evaluating: dead, degraded, or over the hot-utilization line. It
// deliberately does NOT return every mailbox. The overwhelming majority should
// never move, and scanning them all each tick would invite exactly the churn
// the residency floors exist to prevent.
func (r *workerRepository) ListRotationCandidates(ctx context.Context, hotUtilization float64, limit int) ([]MailboxPlacementState, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.db.Query(ctx, mailboxPlacementStateSelect+`
	 WHERE ea.worker_id IS NOT NULL
	   AND (
	         COALESCE(node.active, false) = false
	      OR COALESCE(node.last_seen_at > now() - $1::interval, false) = false
	      OR COALESCE(w.health_state, 'healthy') <> ALL (ARRAY['healthy', 'watch'])
	      OR COALESCE(v.load_score / GREATEST(v.base_capacity * v.health_multiplier * v.age_multiplier, 1), 0) > $2
	      -- isolation drift, in both directions: a stranger sitting on someone's
	      -- reserved worker, and an entitled org's mailbox that is not on theirs.
	      OR (here.organization_id IS NOT NULL AND here.organization_id IS DISTINCT FROM ea.organization_id)
	      OR (own.worker_id IS NOT NULL AND own.worker_id IS DISTINCT FROM ea.worker_id)
	   )
	 ORDER BY ea.worker_assigned_at NULLS FIRST
	 LIMIT $3
	`, WorkerLivenessWindow, hotUtilization, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MailboxPlacementState
	for rows.Next() {
		s, err := scanMailboxPlacementState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}
