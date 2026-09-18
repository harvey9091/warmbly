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

// stubCloudLinkRepo is the local half of the cloud enrollment: the row that
// says this mailbox is warmed by the pool, and whether the pool owns it.
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

// The pool holds the mailbox's own SMTP/IMAP password. Deleting the mailbox
// only cascaded the local link row away, so the cloud went on holding the
// credential and warming a mailbox this instance no longer knew was enrolled,
// and could no longer verify the warmup tokens of (#574).
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
	// Before the row goes: afterwards there is no row left to retry the
	// revocation from.
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

// Reliable, not best-effort, the same way the worker removal is: a credential
// that cannot be revoked keeps the mailbox, so the owner can try again.
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
	// The mailbox was already taken off its worker by then, so a refused
	// delete has to put it back, or the error's "nothing was removed" would be
	// a lie and the mailbox would sit dark until the reconciler's next pass.
	if len(f.pub.added) != 1 || f.pub.added[0] != f.mailbox {
		t.Errorf("shipped %v back to the worker, want one %s", f.pub.added, f.mailbox)
	}
}

// An unreadable enrollment is the same situation: we cannot tell whether the
// pool holds a credential, and guessing wrong leaks it for good.
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

// A managed mailbox is the cloud's own and its mirror here holds no
// credential. cloudlink deletes that mirror through this very path, so calling
// back into it would recurse until the stack ran out.
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
