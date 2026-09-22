package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/app/advanced"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

type replyRecordingAdvanced struct {
	advanced.Service
	calls int
}

func (s *replyRecordingAdvanced) ProcessIncomingReply(context.Context, uuid.UUID, *models.EmailMessageStoreData) *errx.Error {
	s.calls++
	return nil
}

type newEmailInboxRepo struct{ repository.UniboxRepository }

func (newEmailInboxRepo) CreateEntry(context.Context, uuid.UUID, *models.EmailMessageStoreData) error {
	return nil
}

type newEmailAccountRepo struct{ repository.EmailRepository }

func (newEmailAccountRepo) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	return nil, nil
}

func TestHandleNewEmailDoesNotProcessSentMailAsReply(t *testing.T) {
	for _, tc := range []struct {
		name, folder, providerFolder string
		wantCalls                    int
	}{
		{name: "sent follow-up", folder: models.FolderSent},
		{name: "provider still reports sent", folder: models.FolderInbox, providerFolder: models.FolderSent},
		{name: "draft", folder: models.FolderDrafts},
		{name: "inbox", folder: models.FolderInbox, wantCalls: 1},
		{name: "archived inbound", folder: models.FolderArchive, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			advancedService := &replyRecordingAdvanced{}
			service := &JobsService{
				UniboxRepository: newEmailInboxRepo{},
				EmailRepository:  newEmailAccountRepo{},
				AdvancedService:  advancedService,
			}

			err := service.HandleNewEmail(context.Background(), &models.JobEventNewEmail{
				UserID: uuid.New(),
				Message: &models.EmailMessageStoreData{
					ID:             uuid.New(),
					EmailID:        uuid.New(),
					Folder:         tc.folder,
					ProviderFolder: tc.providerFolder,
					FromAddr:       []string{"sender@example.test"},
					ToAddr:         []string{"recipient@example.test"},
					MessageID:      "<follow-up@example.test>",
					InReplyTo:      []string{"<opener@example.test>"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if advancedService.calls != tc.wantCalls {
				t.Fatalf("ProcessIncomingReply calls = %d, want %d", advancedService.calls, tc.wantCalls)
			}
		})
	}
}
