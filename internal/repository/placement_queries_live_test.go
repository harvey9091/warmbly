package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

func placementLiveDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB unset")
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// Live guards on the SQL that migration 000140 rewrote. Dropping four columns
// from `workers` meant hand-editing every column list and scan target that
// named them, plus renumbering the provisioning-template placeholders. None of
// that fails to compile: it fails at runtime, on the mailbox-onboarding path.
// Skipped unless WARMBLY_TEST_DB is set, and it wants a database this branch's
// migrations have been applied to:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run Live -v
func TestPlacementQueriesLive(t *testing.T) {
	pool := placementLiveDB(t)
	ctx := context.Background()
	repo := NewWorkerRepository(pool)

	nodes := NewFleetNodeRepository(&db.DB{Pool: pool})
	workerID := uuid.New()
	beat := models.NodeHeartbeat{
		NodeID: workerID, Role: models.NodeRoleWorker,
		Address: "10.9.9.9", Region: "eu-central",
	}
	if err := nodes.UpsertOnHeartbeat(ctx, beat); err != nil {
		t.Fatalf("UpsertOnHeartbeat: %v", err)
	}
	if err := repo.EnsureWorkerRow(ctx, workerID); err != nil {
		t.Fatalf("EnsureWorkerRow: %v", err)
	}
	// A later beat that omits the region must not wipe the stored one: a beat
	// is a partial report, not a full replacement.
	blank := beat
	blank.Region = ""
	if err := nodes.UpsertOnHeartbeat(ctx, blank); err != nil {
		t.Fatalf("UpsertOnHeartbeat (blank region): %v", err)
	}
	w, err := repo.GetByID(ctx, workerID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if w == nil || w.Region != "eu-central" {
		t.Fatalf("blank region on a later heartbeat must not clear the stored one, got %+v", w)
	}

	if _, err := repo.ListPlaceableWorkers(ctx); err != nil {
		t.Fatalf("ListPlaceableWorkers: %v", err)
	}
	if _, err := repo.GetAllActiveWorkers(ctx); err != nil {
		t.Fatalf("GetAllActiveWorkers: %v", err)
	}
	if _, err := repo.GetIdleUnboundWorker(ctx); err != nil {
		t.Fatalf("GetIdleUnboundWorker: %v", err)
	}
	if _, err := repo.IsWorkerLive(ctx, workerID); err != nil {
		t.Fatalf("IsWorkerLive: %v", err)
	}
	if _, err := repo.GetWorkerDetail(ctx, workerID); err != nil {
		t.Fatalf("GetWorkerDetail: %v", err)
	}
	if _, err := repo.ListWorkersDetail(ctx); err != nil {
		t.Fatalf("ListWorkersDetail: %v", err)
	}

	if err := repo.RefreshWorkerCapacityView(ctx); err != nil {
		t.Fatalf("RefreshWorkerCapacityView: %v", err)
	}
	if _, err := repo.ListCapacityCandidates(ctx, nil); err != nil {
		t.Fatalf("ListCapacityCandidates: %v", err)
	}
	if _, err := repo.GetCapacityRow(ctx, workerID); err != nil {
		t.Fatalf("GetCapacityRow: %v", err)
	}

	orgID := uuid.New()
	if _, err := repo.ListPlacementCandidates(ctx, orgID, "smtp_imap", nil); err != nil {
		t.Fatalf("ListPlacementCandidates: %v", err)
	}
	if _, err := repo.CountOrgMailboxes(ctx, orgID); err != nil {
		t.Fatalf("CountOrgMailboxes: %v", err)
	}
	if _, err := repo.GetMailboxPlacementState(ctx, uuid.New()); err != nil {
		t.Fatalf("GetMailboxPlacementState: %v", err)
	}
	if _, err := repo.ListRotationCandidates(ctx, 0.85, 10); err != nil {
		t.Fatalf("ListRotationCandidates: %v", err)
	}
	if _, err := repo.ListRiskCandidates(ctx, 10); err != nil {
		t.Fatalf("ListRiskCandidates: %v", err)
	}

	_, _ = pool.Exec(ctx, `DELETE FROM fleet_nodes WHERE id = $1`, workerID)
}

func TestAdminWorkerQueriesLive(t *testing.T) {
	pool := placementLiveDB(t)
	ctx := context.Background()

	fleet := NewAdminFleetRepository(&db.DB{Pool: pool})
	if _, err := fleet.Capacity(ctx); err != nil {
		t.Fatalf("fleet Capacity: %v", err)
	}

	admin := NewAdminRepository(pool)
	res, err := admin.ListWorkers(ctx, nil, 5)
	if err != nil {
		t.Fatalf("admin ListWorkers: %v", err)
	}
	_ = res

	id := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_nodes (id, role, name, address, active)
		VALUES ($1,'worker','t','10.9.9.11',true)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workers (id) VALUES ($1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.GetWorkerDetail(ctx, id); err != nil {
		t.Fatalf("admin GetWorkerDetail: %v", err)
	}
	region := "eu-west"
	if err := admin.UpdateWorker(ctx, id, &models.AdminUpdateWorker{Region: &region}); err != nil {
		t.Fatalf("admin UpdateWorker: %v", err)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM fleet_nodes WHERE id = $1`, id)
}
