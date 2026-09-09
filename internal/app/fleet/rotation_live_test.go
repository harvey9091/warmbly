package fleet

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	workerapp "github.com/warmbly/warmbly/internal/app/worker"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/repository"
)

// Live end-to-end cover for placement and rotation against a real database.
// The unit tests pin the scoring and the residency gates; this pins the part
// that only breaks against Postgres: the queries, the counter bookkeeping and
// the loop wiring. Skipped unless WARMBLY_TEST_DB is set, and it wants a
// database this branch's migrations have been applied to:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/app/fleet/ -run Live -v
//
// Self-contained: it creates its own org, workers and mailboxes, and removes
// them again, so it can run against a database that already has data.
func TestLivePlacementAndRotation(t *testing.T) {
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB unset")
	}
	// Billing off is the self-host default and makes the pool assertion below
	// distinguishable from the column default.
	t.Setenv("BILLING_PROVIDER", "none")

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	repo := repository.NewWorkerRepository(pool)
	svc := workerapp.NewAssignmentService(repo, nil, nil)

	// Two workers, both heartbeating. A worker is a node plus a placement row,
	// so both halves are created the way an enrolling node creates them.
	first, second := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{first, second} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO fleet_nodes (id, role, name, address, active, region, last_seen_at)
			VALUES ($1, 'worker', $2, '10.77.0.1', true, 'eu-central', now())`,
			id, "live-test-"+id.String()[:8]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO workers (id, health_state, load_score) VALUES ($1, 'healthy', 0)`, id); err != nil {
			t.Fatal(err)
		}
	}

	userID, orgID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, first_name, last_name, email) VALUES ($1,'Live','Test',$2)`,
		userID, "live-"+userID.String()[:8]+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO organizations (id, name, owner_user_id) VALUES ($1,'Live Test',$2)`,
		orgID, userID); err != nil {
		t.Fatal(err)
	}

	mailboxes := make([]uuid.UUID, 0, 3)
	for i, provider := range []string{"smtp_imap", "gmail", "smtp_imap"} {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO email_accounts (id, user_id, organization_id, email, name,
			                            signature_plain, signature_html, provider, status)
			VALUES ($1,$2,$3,$4,'Live','','',$5::email_provider,'active')`,
			id, userID, orgID, id.String()[:8]+"@example.test", provider); err != nil {
			t.Fatalf("mailbox %d: %v", i, err)
		}
		mailboxes = append(mailboxes, id)
	}

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM email_accounts WHERE organization_id = $1`, orgID)
		_, _ = pool.Exec(c, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(c, `DELETE FROM fleet_nodes WHERE id = ANY($1)`, []uuid.UUID{first, second})
	})

	if err := repo.RefreshWorkerCapacityView(ctx); err != nil {
		t.Fatal(err)
	}

	// 1. Every mailbox places, and is stamped so rotation can enforce residency.
	//
	//    Which worker it lands on is deliberately NOT asserted. The database
	//    may already hold a fleet, any live worker is a legitimate answer, and
	//    live tests in other packages create and delete their own workers
	//    concurrently - re-reading the chosen worker here raced with one of
	//    those deletes. That placement only ever picks a live worker is
	//    covered by the placement unit tests, which need no database.
	for i, mb := range mailboxes {
		got, err := svc.AssignWorkerToEmail(ctx, mb, orgID)
		if err != nil || got == nil {
			t.Fatalf("mailbox %d did not place: %v", i, err)
		}

		var assigned *string
		if err := pool.QueryRow(ctx,
			`SELECT worker_assigned_at::text FROM email_accounts WHERE id=$1`, mb).Scan(&assigned); err != nil {
			t.Fatal(err)
		}
		if assigned == nil {
			t.Fatalf("mailbox %d: worker_assigned_at not stamped, so rotation cannot enforce residency", i)
		}
	}

	// 1b. Placement also settles warmup pool membership. It used to fall out of
	//     tier placement; with tiers gone it has to be set explicitly, and
	//     leaving it unset silently warms paying customers in the free pool.
	//
	//     The assertion has to distinguish "set" from "left at the default",
	//     so it runs with billing disabled, where every org resolves to the
	//     premium pool and the column default ('free') is a visible failure.
	for i, mb := range mailboxes {
		var poolType *string
		if err := pool.QueryRow(ctx,
			`SELECT warmup_pool_type FROM email_accounts WHERE id = $1`, mb).Scan(&poolType); err != nil {
			t.Fatalf("mailbox %d: read warmup pool: %v", i, err)
		}
		if poolType == nil || *poolType != "premium" {
			got := "<null>"
			if poolType != nil {
				got = *poolType
			}
			t.Fatalf("mailbox %d: warmup_pool_type is %q, want \"premium\"; placement is not assigning pool membership", i, got)
		}
	}

	// 2. Gather them onto this test's own worker, so the rotation assertions
	//    below are about this test's fleet and not whatever else is running.
	placed := map[uuid.UUID]uuid.UUID{}
	for _, mb := range mailboxes {
		var from *uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT worker_id FROM email_accounts WHERE id=$1`, mb).Scan(&from); err != nil {
			t.Fatal(err)
		}
		if err := svc.MoveMailbox(ctx, mb, from, first); err != nil {
			t.Fatalf("gather onto the test worker: %v", err)
		}
		placed[mb] = first
	}

	// 3. Load accounting follows MailboxWeight: 1.0 + 0.05 + 1.0.
	var testLoad float64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(sum(load_score),0) FROM workers WHERE id = ANY($1)`,
		[]uuid.UUID{first, second}).Scan(&testLoad); err != nil {
		t.Fatal(err)
	}
	if testLoad < 2.04 || testLoad > 2.06 {
		t.Fatalf("load_score on the test workers = %.2f, want 2.05 (smtp 1.0 + gmail 0.05 + smtp 1.0)", testLoad)
	}

	rot := &Rotator{
		WorkerRepo: repo,
		Assignment: svc,
		Decisions:  repository.NewDecisionLogRepository(&db.DB{Pool: pool}),
	}
	rot.defaults()

	// 4. A healthy fleet must not churn. Moving a mailbox changes the client IP
	//    its provider sees, so an idle tick doing nothing is the correct result.
	if err := rot.tick(ctx); err != nil {
		t.Fatalf("tick on a healthy fleet: %v", err)
	}
	for mb, want := range placed {
		var got *uuid.UUID
		if err := pool.QueryRow(ctx, `SELECT worker_id FROM email_accounts WHERE id=$1`, mb).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got == nil || *got != want {
			t.Fatalf("rotation moved a mailbox off a healthy worker (%v -> %v); it must not churn", want, got)
		}
	}

	// 5. A worker stops heartbeating: everything on it has to leave, because a
	//    command queued for a dead worker is never executed and never answered.
	if _, err := pool.Exec(ctx,
		`UPDATE fleet_nodes SET last_seen_at = now() - interval '1 hour' WHERE id = $1`, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.RefreshWorkerCapacityView(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rot.tick(ctx); err != nil {
		t.Fatalf("tick with a dead worker: %v", err)
	}

	var stranded int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM email_accounts WHERE organization_id=$1 AND worker_id=$2`,
		orgID, first).Scan(&stranded); err != nil {
		t.Fatal(err)
	}
	if stranded != 0 {
		t.Fatalf("%d mailboxes stranded on a worker that stopped heartbeating", stranded)
	}

	// 6. The move has to be auditable.
	var decisions int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM decision_log WHERE kind='rotate' AND worker_id=$1`, first).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions == 0 {
		t.Fatal("rotation moved mailboxes without writing decision_log entries")
	}
}
