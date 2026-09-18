package email

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// stubCloudLinkRepo supplies the mailbox's local cloud-enrollment row.
type stubCloudLinkRepo struct {
	repository.CloudLinkRepository

	row *models.CloudLinkMailbox
	err error
}

func (s *stubCloudLinkRepo) GetByAccount(context.Context, uuid.UUID) (*models.CloudLinkMailbox, error) {
	return s.row, s.err
}

// stubUnenroller records the revocation and can refuse it.
type stubUnenroller struct {
	trace *[]string

	calls []uuid.UUID
	orgs  []uuid.UUID
	err   *errx.Error
}

func (s *stubUnenroller) RevokeForDelete(_ context.Context, orgID, accountID uuid.UUID) *errx.Error {
	if s.trace != nil {
		*s.trace = append(*s.trace, "unenroll")
	}
	s.calls = append(s.calls, accountID)
	s.orgs = append(s.orgs, orgID)
	return s.err
}

func withCloudEnrollment(f *removalFixture, managed bool) *stubUnenroller {
	u := &stubUnenroller{trace: &f.trace}
	f.svc.cloudLink = &stubCloudLinkRepo{row: &models.CloudLinkMailbox{
		EmailAccountID: f.mailbox,
		RemoteID:       f.mailbox,
		Managed:        managed,
	}}
	f.svc.cloudUnenroll = u
	return u
}

// Cloud credential revocation must precede local mailbox deletion.
func TestDeleteRevokesTheCloudEnrollmentBeforeTheRowGoes(t *testing.T) {
	f := newRemovalFixture(t)
	u := withCloudEnrollment(f, false)

	if xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String()); xerr != nil {
		t.Fatalf("delete: %v", xerr)
	}
	if len(u.calls) != 1 || u.calls[0] != f.mailbox {
		t.Fatalf("unenrolled %v, want one call for %s", u.calls, f.mailbox)
	}
	if u.orgs[0] != f.org {
		t.Errorf("unenrolled under org %s, want the mailbox's own %s", u.orgs[0], f.org)
	}
	// The enrollment row must remain until remote revocation succeeds.
	want := []string{"remove", "unenroll", "delete"}
	if len(f.trace) != len(want) {
		t.Fatalf("order was %v, want %v", f.trace, want)
	}
	for i := range want {
		if f.trace[i] != want[i] {
			t.Fatalf("order was %v, want %v", f.trace, want)
		}
	}
}

// Failed revocation preserves the mailbox for retry.
func TestDeleteKeepsTheMailboxWhenTheCloudRefusesTheRevocation(t *testing.T) {
	f := newRemovalFixture(t)
	u := withCloudEnrollment(f, false)
	u.err = errx.InternalError()

	xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String())
	if xerr == nil {
		t.Fatal("the mailbox was deleted while the pool still held its password")
	}
	if xerr.Identifier != ErrCloudEnrollmentStuck.Identifier {
		t.Errorf("error = %q, want %q", xerr.Identifier, ErrCloudEnrollmentStuck.Identifier)
	}
	if f.repo.deleteCalls != 0 {
		t.Errorf("the row was deleted %d times, want 0", f.repo.deleteCalls)
	}
	// A refused delete restores the mailbox to its worker.
	if len(f.pub.added) != 1 || f.pub.added[0] != f.mailbox {
		t.Errorf("shipped %v back to the worker, want one %s", f.pub.added, f.mailbox)
	}
}

// An unreadable enrollment cannot prove the remote credential is gone.
func TestDeleteKeepsTheMailboxWhenTheEnrollmentCannotBeRead(t *testing.T) {
	f := newRemovalFixture(t)
	withCloudEnrollment(f, false)
	f.svc.cloudLink = &stubCloudLinkRepo{err: errors.New("db down")}

	if xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String()); xerr == nil {
		t.Fatal("the mailbox was deleted on an unreadable cloud enrollment")
	}
	if f.repo.deleteCalls != 0 {
		t.Errorf("the row was deleted %d times, want 0", f.repo.deleteCalls)
	}
}

// Missing cloud dependencies cannot prove that a credential was revoked.
func TestDeleteKeepsTheMailboxWhenCloudRevocationIsNotWired(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*emailService)
	}{
		{"repository missing", func(s *emailService) { s.cloudLink = nil }},
		{"revoker missing", func(s *emailService) { s.cloudUnenroll = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRemovalFixture(t)
			tc.setup(f.svc)

			xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String())
			if xerr == nil || xerr.Identifier != ErrCloudEnrollmentStuck.Identifier {
				t.Fatalf("error = %v, want %q", xerr, ErrCloudEnrollmentStuck.Identifier)
			}
			if f.repo.deleteCalls != 0 {
				t.Errorf("the row was deleted %d times, want 0", f.repo.deleteCalls)
			}
		})
	}
}

// Cloud-managed mirrors skip revocation to avoid recursive deletion.
func TestDeleteDoesNotCallBackForAManagedMailbox(t *testing.T) {
	f := newRemovalFixture(t)
	u := withCloudEnrollment(f, true)

	if xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String()); xerr != nil {
		t.Fatalf("delete: %v", xerr)
	}
	if len(u.calls) != 0 {
		t.Fatalf("a managed mailbox was unenrolled through its own delete path: %v", u.calls)
	}
	if f.repo.deleteCalls != 1 {
		t.Errorf("delete called %d times, want 1", f.repo.deleteCalls)
	}
}

// A mailbox the cloud never warmed is deleted with no round trip at all.
func TestDeleteSkipsTheCloudWhenTheMailboxIsNotEnrolled(t *testing.T) {
	f := newRemovalFixture(t)
	u := withCloudEnrollment(f, false)
	f.svc.cloudLink = &stubCloudLinkRepo{}

	if xerr := f.svc.Delete(context.Background(), f.user.String(), f.mailbox.String()); xerr != nil {
		t.Fatalf("delete: %v", xerr)
	}
	if len(u.calls) != 0 {
		t.Fatalf("an unenrolled mailbox was unenrolled anyway: %v", u.calls)
	}
}
