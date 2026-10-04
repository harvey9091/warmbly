package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/api/middleware"
	"github.com/warmbly/warmbly/internal/app/sequence"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
)

type recordingSteps struct {
	sequence.SequenceService
	created, deleted bool
	update           *models.UpdateSequence
	refuse           *errx.Error
}

func (r *recordingSteps) Create(context.Context, string, string) (*models.Sequence, *errx.Error) {
	r.created = true
	return &models.Sequence{ID: uuid.New()}, nil
}

func (r *recordingSteps) Update(_ context.Context, _, _, _ string, d *models.UpdateSequence) (*models.Sequence, *errx.Error) {
	r.update = d
	if r.refuse != nil {
		return nil, r.refuse
	}
	return &models.Sequence{ID: uuid.New()}, nil
}

func (r *recordingSteps) Delete(context.Context, string, string, string) *errx.Error {
	r.deleted = true
	return nil
}

func postStep(t *testing.T, svc *recordingSteps, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/campaigns/x/steps", strings.NewReader(body))
	c.Set(middleware.OrganizationIDKey, uuid.New())
	(&Handler{SequenceService: svc}).CreateSequence(c)
	return w
}

func TestCreateStepAppliesAnOptionalBody(t *testing.T) {
	for _, body := range []string{"", "{}"} {
		svc := &recordingSteps{}
		if w := postStep(t, svc, body); w.Code != http.StatusOK || !svc.created || svc.update != nil {
			t.Fatalf("body %q: status %d, created %v, update %+v", body, w.Code, svc.created, svc.update)
		}
	}

	svc := &recordingSteps{}
	w := postStep(t, svc, `{"subject":"Quick question","wait_after":2}`)
	if w.Code != http.StatusOK || svc.update == nil || *svc.update.Subject != "Quick question" || *svc.update.WaitAfter != 2 {
		t.Fatalf("status %d, update %+v", w.Code, svc.update)
	}

	svc = &recordingSteps{refuse: errx.ErrSequenceSubject}
	if w := postStep(t, svc, `{"subject":"x"}`); w.Code != http.StatusBadRequest || !svc.deleted {
		t.Fatalf("a refused body left the blank step: status %d, deleted %v", w.Code, svc.deleted)
	}

	svc = &recordingSteps{}
	if w := postStep(t, svc, `{"wait_after":"soon"}`); w.Code != http.StatusBadRequest || svc.created {
		t.Fatalf("a malformed body created a step: status %d", w.Code)
	}
}
