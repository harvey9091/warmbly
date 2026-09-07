package auth

import (
	"context"
	"net/mail"
	"testing"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/app/orgrisk"
	"github.com/warmbly/warmbly/internal/app/user"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
)

// recordingUserRepo captures the signup metadata write. Everything else is the
// minimum the account path touches.
type recordingUserRepo struct {
	repository.UserRepository
	created    *models.User
	gotIP      string
	gotUA      string
	gotRisk    int
	gotNorm    string
	recordHits int
}

func (r *recordingUserRepo) CreateUser(_ context.Context, email *mail.Address, _ string) (*models.User, error) {
	r.created = &models.User{ID: uuid.New(), Email: email.Address}
	return r.created, nil
}

func (r *recordingUserRepo) RecordSignupMetadata(_ context.Context, _ uuid.UUID, ip, ua string, risk int, normalized string) error {
	r.recordHits++
	r.gotIP, r.gotUA, r.gotRisk, r.gotNorm = ip, ua, risk, normalized
	return nil
}

type noopUserService struct{ user.UserService }

func (noopUserService) SaveUser(context.Context, *models.User) *errx.Error { return nil }

// The failure this guards against is the one that keeps recurring: a scorer
// that works perfectly and a call site that never runs. Testing signuprisk
// directly cannot see it; this exercises the account path itself.
func TestCreateAccountRecordsTheSignupOrigin(t *testing.T) {
	repo := &recordingUserRepo{}
	svc := &authService{userRepository: repo, userService: noopUserService{}}

	origin := SignupOrigin{IP: "203.0.113.5", UserAgent: "Mozilla/5.0 (test)"}
	if _, err := svc.createAccount(context.Background(), "Ada.Lovelace+signup@gmail.com", "hash", SignupAttribution{}, origin); err != nil {
		t.Fatalf("createAccount: %v", err.Message)
	}

	if repo.recordHits != 1 {
		t.Fatalf("signup metadata written %d times, want exactly once", repo.recordHits)
	}
	if repo.gotIP != origin.IP {
		t.Errorf("ip = %q, want %q", repo.gotIP, origin.IP)
	}
	if repo.gotUA != origin.UserAgent {
		t.Errorf("user agent = %q, want %q", repo.gotUA, origin.UserAgent)
	}
	// Normalized so the same person opening a second tagged account is visible.
	if repo.gotNorm != "adalovelace@gmail.com" {
		t.Errorf("normalized = %q, want the plus-tag and dots collapsed", repo.gotNorm)
	}
	if repo.gotRisk == 0 {
		t.Error("a tagged free-provider signup scored 0; the scorer is not reaching the write")
	}
}

// A signup with nothing notable still records its origin: the correlation data
// is the point, and only recording risky ones would leave the clusters #148
// looks for half-empty.
func TestCreateAccountRecordsACleanSignupToo(t *testing.T) {
	repo := &recordingUserRepo{}
	svc := &authService{userRepository: repo, userService: noopUserService{}}

	if _, err := svc.createAccount(context.Background(), "ada@acme.com", "hash", SignupAttribution{}, SignupOrigin{IP: "203.0.113.9"}); err != nil {
		t.Fatalf("createAccount: %v", err.Message)
	}
	if repo.recordHits != 1 {
		t.Fatalf("clean signup wrote metadata %d times, want once", repo.recordHits)
	}
	if repo.gotRisk != 0 {
		t.Errorf("risk = %d, want 0 for an ordinary business signup", repo.gotRisk)
	}
	if repo.gotNorm != "ada@acme.com" {
		t.Errorf("normalized = %q", repo.gotNorm)
	}
}

// capturingRisk records what the signup filed on the workspace.
type capturingRisk struct {
	orgrisk.Service
	got []orgrisk.Signal
}

func (c *capturingRisk) RecordSignal(_ context.Context, _ uuid.UUID, sig orgrisk.Signal) (*models.OrgRisk, *errx.Error) {
	c.got = append(c.got, sig)
	return &models.OrgRisk{}, nil
}

// Only the throwaway domain is evidence about the account. The softer findings
// describe how a signup looked, and shape must not restrict a workspace, so the
// two are filed separately instead of fused into one classed score.
func TestSignupRiskFilesEachClassSeparately(t *testing.T) {
	for _, tt := range []struct {
		name  string
		email string
		ip    string
		want  map[string]orgrisk.Signal
	}{
		{"an ordinary signup files nothing", "ada@acme.com", "203.0.113.5", map[string]orgrisk.Signal{}},
		{"soft findings only", "ada.lovelace+signup@gmail.com", "", map[string]orgrisk.Signal{
			"signup": {Weight: 10, Class: orgrisk.ClassCircumstantial},
		}},
		{"a throwaway domain with a soft finding beside it", "grow@mailinator.com", "", map[string]orgrisk.Signal{
			"signup_disposable": {Weight: 35, Class: orgrisk.ClassSubstantive},
			"signup":            {Weight: 5, Class: orgrisk.ClassCircumstantial},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			risk := &capturingRisk{}
			svc := &authService{orgRisk: risk}
			svc.recordSignupRisk(context.Background(), uuid.New(), tt.email, SignupOrigin{IP: tt.ip})

			if len(risk.got) != len(tt.want) {
				t.Fatalf("filed %d signals, want %d: %+v", len(risk.got), len(tt.want), risk.got)
			}
			for _, got := range risk.got {
				want, ok := tt.want[got.Key]
				if !ok {
					t.Errorf("unexpected signal %q", got.Key)
					continue
				}
				if got.Weight != want.Weight || got.Class != want.Class {
					t.Errorf("%s = %d/%q, want %d/%q", got.Key, got.Weight, got.Class, want.Weight, want.Class)
				}
				if got.Detail == "" {
					t.Errorf("%s carries no reason for review", got.Key)
				}
			}
		})
	}
}
