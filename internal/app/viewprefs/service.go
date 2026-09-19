// Package viewprefs keeps each member's own layout of the dashboard's lists:
// which columns show, in what order, and the sort. A layout is personal and
// per workspace, so two members of one workspace can look at the same contacts
// through different columns.
package viewprefs

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/observability/errs"
	"github.com/warmbly/warmbly/internal/repository"
	"github.com/warmbly/warmbly/internal/utils"
)

// Service reads and writes a member's saved list layouts.
type Service interface {
	// Get returns the saved layout, or the empty layout (default columns,
	// default sort) when none is saved.
	Get(ctx context.Context, userID, orgID uuid.UUID, view string) (*models.ViewPreferences, *errx.Error)
	// Put writes the fields the update carries and keeps the rest, so the
	// columns and the sort can be saved independently.
	Put(ctx context.Context, userID, orgID uuid.UUID, view string, upd models.ViewPreferencesUpdate) (*models.ViewPreferences, *errx.Error)
	// Reset forgets the saved layout, so the list shows its defaults again.
	Reset(ctx context.Context, userID, orgID uuid.UUID, view string) *errx.Error
}

type service struct {
	repo repository.ViewPreferencesRepository
}

func NewService(repo repository.ViewPreferencesRepository) Service {
	return &service{repo: repo}
}

func unknownView() *errx.Error {
	return errx.NewWithIdentifier(errx.NotFound, "unknown_view", "unknown view")
}

func (s *service) Get(ctx context.Context, userID, orgID uuid.UUID, view string) (*models.ViewPreferences, *errx.Error) {
	if !models.KnownViews[view] {
		return nil, unknownView()
	}
	prefs, err := s.repo.Get(ctx, userID, orgID, view)
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	if prefs == nil {
		return &models.ViewPreferences{View: view, Columns: []string{}}, nil
	}
	return prefs, nil
}

func (s *service) Put(ctx context.Context, userID, orgID uuid.UUID, view string, upd models.ViewPreferencesUpdate) (*models.ViewPreferences, *errx.Error) {
	if !models.KnownViews[view] {
		return nil, unknownView()
	}
	if xerr := validate(view, &upd); xerr != nil {
		return nil, xerr
	}
	saved, err := s.repo.Upsert(ctx, userID, orgID, view, upd)
	if err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return saved, nil
}

func (s *service) Reset(ctx context.Context, userID, orgID uuid.UUID, view string) *errx.Error {
	if !models.KnownViews[view] {
		return unknownView()
	}
	if err := s.repo.Delete(ctx, userID, orgID, view); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}
	return nil
}

// validate checks the update the way the dashboard will read it: every column
// id is one this view can render or a well-formed custom field, listed once;
// the sort names a column the contacts search knows or a well-formed custom
// field. Custom-field ids are normalized in place so the same field saved with
// different spacing is one column.
func validate(view string, upd *models.ViewPreferencesUpdate) *errx.Error {
	if upd.Columns != nil {
		cols := *upd.Columns
		if len(cols) > models.ViewPreferencesMaxColumns {
			return errx.NewWithIdentifier(errx.BadRequest, "too_many_columns", "too many columns")
		}
		known := make(map[string]bool, len(models.ViewBuiltinColumns[view]))
		for _, id := range models.ViewBuiltinColumns[view] {
			known[id] = true
		}
		seen := make(map[string]bool, len(cols))
		for i, id := range cols {
			norm, ok := normalizeID(id, known)
			if !ok {
				return errx.NewWithIdentifier(errx.BadRequest, "invalid_column", "invalid column: "+id)
			}
			if seen[norm] {
				return errx.NewWithIdentifier(errx.BadRequest, "duplicate_column", "column listed twice: "+norm)
			}
			seen[norm] = true
			cols[i] = norm
		}
	}
	if upd.Sort != nil {
		by := strings.TrimSpace(upd.Sort.By)
		if by == "" {
			// The default sort is stored as no sort at all.
			upd.Sort = &models.ViewSort{}
			return nil
		}
		norm, ok := normalizeID(by, models.ContactBuiltinSorts)
		if !ok {
			return errx.NewWithIdentifier(errx.BadRequest, "invalid_sort", "invalid sort: "+by)
		}
		upd.Sort.By = norm
	}
	return nil
}

// normalizeID accepts a known built-in id as is, or a custom field with a
// valid key, normalized.
func normalizeID(id string, known map[string]bool) (string, bool) {
	if len(id) > models.ViewColumnIDMaxLength {
		return "", false
	}
	if key, ok := models.ContactSortCustomField(id); ok {
		if !utils.IsValidJSONKey(key) {
			return "", false
		}
		return models.ContactSortCustomPrefix + key, true
	}
	return id, known[id]
}
