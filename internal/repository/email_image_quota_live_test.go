package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

// Email-body images and campaign attachments share one storage quota, because
// they share one object store. The reservation that enforces it is the insert
// itself, under the org's quota lock, so what has to hold is that an image sees
// the attachments already stored, that a refusal writes no row, and that the
// attachment path sees images the same way in return.
//
// Run against the dev stack:
//
//	WARMBLY_TEST_DB=postgres://warmbly:warmbly@localhost:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveEmailImageQuota -v
func TestLiveEmailImageQuota(t *testing.T) {
	handle, pool := liveContactDB(t)
	f := newSharedOrgFixture(t, pool)
	images := NewEmailImageRepository(handle)
	attachments := NewAttachmentRepository(handle)
	ctx := context.Background()

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM email_images WHERE organization_id = $1`, f.org); err != nil {
			t.Errorf("cleanup email_images: %v", err)
		}
		if _, err := pool.Exec(context.Background(),
			`DELETE FROM campaign_attachments WHERE campaign_id IN (SELECT id FROM campaigns WHERE organization_id = $1)`,
			f.org); err != nil {
			t.Errorf("cleanup campaign_attachments: %v", err)
		}
	})

	limit := func(n int64) StorageLimitFunc {
		return func(context.Context) (int64, error) { return n, nil }
	}
	newImage := func(name string, size int64) *models.EmailImage {
		return &models.EmailImage{
			OrganizationID: f.org,
			UserID:         &f.owner,
			Filename:       name,
			MimeType:       "image/png",
			Size:           size,
			StorageKey:     models.EmailImageObjectKey(f.org, name),
			URL:            "https://example.test/" + name,
		}
	}

	// A 400-byte attachment leaves 600 of a 1000-byte quota.
	att := &models.CampaignAttachment{
		CampaignID: f.campaign,
		UserID:     f.owner,
		Filename:   "brief.pdf",
		Size:       400,
		MimeType:   "application/pdf",
		S3Key:      "live/brief.pdf",
	}
	if created, _, _, err := attachments.CreateWithinQuota(ctx, att, f.org, limit(1000)); err != nil || !created {
		t.Fatalf("attachment reservation: created=%v err=%v", created, err)
	}

	if created, used, _, err := images.CreateWithinQuota(ctx, newImage("logo.png", 500), limit(1000)); err != nil {
		t.Fatalf("image reservation: %v", err)
	} else if !created || used != 900 {
		t.Fatalf("500-byte image into 600 bytes of room: created=%v used=%d, want true/900", created, used)
	}

	// 200 more would be 1100 against a 1000-byte quota.
	created, used, applied, err := images.CreateWithinQuota(ctx, newImage("hero.png", 200), limit(1000))
	if err != nil {
		t.Fatalf("over-quota image: %v", err)
	}
	if created {
		t.Error("an image past the quota was stored")
	}
	if used != 900 || applied != 1000 {
		t.Errorf("refusal reported %d of %d, want 900 of 1000", used, applied)
	}

	list, err := images.ListByOrg(ctx, f.org, 10, time.Time{}, uuid.Nil)
	if err != nil {
		t.Fatalf("ListByOrg: %v", err)
	}
	if len(list) != 1 || list[0].Filename != "logo.png" {
		t.Fatalf("library holds %d images (%v), want only the one that fit", len(list), list)
	}

	// The attachment path counts the image in return, so the two cannot each
	// spend the same last bytes.
	if total, serr := attachments.SumStorageUsedByOrg(ctx, f.org); serr != nil || total != 900 {
		t.Errorf("shared total is %d (err %v), want 900", total, serr)
	}

	// Keyset paging: two more tiny images, then walk the library a page at a
	// time. The cursor is (created_at, id), so rows sharing a timestamp still
	// page without repeating or skipping one.
	for _, name := range []string{"second.png", "third.png"} {
		if created, _, _, cerr := images.CreateWithinQuota(ctx, newImage(name, 10), limit(1000)); cerr != nil || !created {
			t.Fatalf("paging fixture %s: created=%v err=%v", name, created, cerr)
		}
	}
	first, err := images.ListByOrg(ctx, f.org, 2, time.Time{}, uuid.Nil)
	if err != nil || len(first) != 2 {
		t.Fatalf("first page: %d rows, err %v", len(first), err)
	}
	next, err := images.ListByOrg(ctx, f.org, 2, first[1].CreatedAt, first[1].ID)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(next) != 1 || next[0].ID == first[0].ID || next[0].ID == first[1].ID {
		t.Fatalf("second page returned %d rows and repeated the first page", len(next))
	}
	for _, extra := range next {
		if derr := images.Delete(ctx, extra.ID); derr != nil {
			t.Fatalf("paging cleanup: %v", derr)
		}
	}
	for _, row := range first {
		if row.ID == list[0].ID {
			continue
		}
		if derr := images.Delete(ctx, row.ID); derr != nil {
			t.Fatalf("paging cleanup: %v", derr)
		}
	}

	if err := images.Delete(ctx, list[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, gerr := images.GetByID(ctx, list[0].ID); gerr != nil || got != nil {
		t.Errorf("deleted image still readable: %v (err %v)", got, gerr)
	}
	if total, serr := attachments.SumStorageUsedByOrg(ctx, f.org); serr != nil || total != 400 {
		t.Errorf("total after delete is %d (err %v), want the attachment alone", total, serr)
	}

	// An image of another workspace is never in this one's library.
	if _, err := images.GetByID(ctx, uuid.New()); err != nil {
		t.Errorf("GetByID on an unknown id should be a miss, not an error: %v", err)
	}
}
