package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// Issue #381: a saved Google Sheets source carries segment targets so a sync
// started from a segment's member list pins its rows into that segment, the
// same way a file import does. These prove the column round-trips and that the
// segment filter finds the sources feeding one segment.
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveLeadSyncSegment -v

func TestLiveLeadSyncSegmentTargets(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newSharedOrgFixture(t, pool)
	ctx := context.Background()
	repo := NewLeadSyncRepository(pool)
	segments := NewSegmentRepository(handle)

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM lead_sync_sources WHERE organization_id = $1`, f.org); err != nil {
			t.Errorf("cleanup sources: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `DELETE FROM segments WHERE organization_id = $1`, f.org); err != nil {
			t.Errorf("cleanup segments: %v", err)
		}
	})

	newSeg := func(name string) uuid.UUID {
		t.Helper()
		seg, xerr := segments.Create(ctx, f.org, &f.owner, &models.Segment{
			Name: name, Color: "#0284c7", Match: models.SegmentMatchAll,
		})
		if xerr != nil {
			t.Fatalf("create segment: %v", xerr)
		}
		return seg.ID
	}
	target := newSeg("Sheet target " + uuid.New().String()[:6])
	other := newSeg("Elsewhere " + uuid.New().String()[:6])

	pinned := &models.LeadSyncSource{
		OrganizationID: f.org, CreatedByUserID: f.owner, ConnectionID: uuid.New(),
		SheetID: "sheet-pinned", HasHeader: true,
		ColumnMapping: []models.ContactImportColumnMapping{{Index: 0, Target: models.ContactImportTargetEmail}},
		Dedup:         models.ContactImportDedupUpdate,
		SegmentIDs:    []string{target.String()},
	}
	loose := &models.LeadSyncSource{
		OrganizationID: f.org, CreatedByUserID: f.owner, ConnectionID: uuid.New(),
		SheetID: "sheet-loose", HasHeader: true,
		ColumnMapping: []models.ContactImportColumnMapping{{Index: 0, Target: models.ContactImportTargetEmail}},
		Dedup:         models.ContactImportDedupUpdate,
	}
	for _, src := range []*models.LeadSyncSource{pinned, loose} {
		if err := repo.Create(ctx, src); err != nil {
			t.Fatalf("create source: %v", err)
		}
	}

	got, err := repo.Get(ctx, f.org, pinned.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.SegmentIDs) != 1 || got.SegmentIDs[0] != target.String() {
		t.Fatalf("segment targets = %v, want [%s]", got.SegmentIDs, target)
	}
	// A source with no targets reads back as an empty list, never nil, so the
	// importer never sees a null segment list.
	back, err := repo.Get(ctx, f.org, loose.ID)
	if err != nil {
		t.Fatalf("get loose: %v", err)
	}
	if back.SegmentIDs == nil || len(back.SegmentIDs) != 0 {
		t.Fatalf("untargeted source segment ids = %v, want []", back.SegmentIDs)
	}

	list, err := repo.List(ctx, f.org, nil, &target)
	if err != nil {
		t.Fatalf("list by segment: %v", err)
	}
	if len(list) != 1 || list[0].ID != pinned.ID {
		t.Fatalf("segment filter returned %d sources, want only the pinned one", len(list))
	}
	if list, err = repo.List(ctx, f.org, nil, &other); err != nil || len(list) != 0 {
		t.Fatalf("filter on an unrelated segment returned %d sources (err %v)", len(list), err)
	}
	if list, err = repo.List(ctx, f.org, nil, nil); err != nil || len(list) != 2 {
		t.Fatalf("unfiltered list returned %d sources (err %v), want 2", len(list), err)
	}

	// The targets survive an edit that does not mention them.
	got.Label = "renamed"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	again, err := repo.Get(ctx, f.org, pinned.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if len(again.SegmentIDs) != 1 || again.SegmentIDs[0] != target.String() {
		t.Fatalf("segment targets after update = %v", again.SegmentIDs)
	}
}
