package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// stubCreateRepo counts which of the two write paths the handler took.
type stubCreateRepo struct {
	repository.EmailAccountErrorRepository
	createdOnce int
	suppress    bool
}

func (s *stubCreateRepo) CreateOnce(_ context.Context, in *repository.CreateEmailAccountError) (*repository.EmailAccountError, *errx.Error) {
	s.createdOnce++
	if s.suppress {
		return nil, nil
	}
	return &repository.EmailAccountError{ErrorCode: in.ErrorCode}, nil
}

// A mail server that refuses the same command on every pass is relayed on
// every pass. Recording each one unconditionally filled a mailbox's error
// list with an identical row a minute for as long as the refusal lasted
// (issue #405), so the server-error path has to write conditionally.
func TestServerErrorRecordsOnePerUnresolvedCode(t *testing.T) {
	repo := &stubCreateRepo{}
	s := &JobsService{EmailAccountErrorRepository: repo}

	event := models.EmailErrorEvent{
		EmailAccountID: uuid.NewString(),
		UserID:         uuid.NewString(),
		ErrorCode:      string(errx.MailErrorCodeImapUnknown),
		Message:        "Something went wrong: NO System Error",
		UserVisible:    true,
	}

	for i := 0; i < 3; i++ {
		if err := s.HandleEmailServerError(context.Background(), event); err != nil {
			t.Fatalf("HandleEmailServerError: %v", err)
		}
	}

	if repo.createdOnce != 3 {
		t.Errorf("CreateOnce called %d times, want one per relayed failure", repo.createdOnce)
	}
}

// A suppressed write is not an error: the row it wanted is already on screen.
func TestServerErrorSurvivesASuppressedWrite(t *testing.T) {
	repo := &stubCreateRepo{suppress: true}
	s := &JobsService{EmailAccountErrorRepository: repo}

	if err := s.HandleEmailServerError(context.Background(), models.EmailErrorEvent{
		EmailAccountID: uuid.NewString(),
		UserID:         uuid.NewString(),
		ErrorCode:      string(errx.MailErrorCodeImapUnknown),
		UserVisible:    true,
	}); err != nil {
		t.Fatalf("HandleEmailServerError: %v", err)
	}
	if repo.createdOnce != 1 {
		t.Fatalf("CreateOnce called %d times, want 1", repo.createdOnce)
	}
}
