// Package viewprefs keeps each member's own layout of the dashboard's lists:
// which columns show, in what order, and the sort. A layout is personal and
// per workspace, so two members of one workspace can look at the same contacts
// through different columns.
package viewprefs

import (
	"context"
	"regexp"
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
	// Put replaces the saved layout.
	Put(ctx context.Context, userID, orgID uuid.UUID, prefs *models.ViewPreferences) (*models.ViewPreferences, *errx.Error)
	// Reset forgets the saved layout, so the list shows its defaults again.
	Reset(ctx context.Context, userID, orgID uuid.UUID, view string) *errx.Error
}

type service struct {
	repo repository.ViewPreferencesRepository
}

func NewService(repo repository.ViewPreferencesRepository) Service {
	return &service{repo: repo}
}

// A built-in column id is a snake_case name the dashboard defines.
var builtinColumnPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (s *service) Get(ctx context.Context, userID, orgID uuid.UUID, view string) (*models.ViewPreferences, *errx.Error) {
	if !models.KnownViews[view] {
		return nil, errx.NewWithIdentifier(errx.NotFound, "unknown_view", "unknown view")
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

func (s *service) Put(ctx context.Context, userID, orgID uuid.UUID, prefs *models.ViewPreferences) (*models.ViewPreferences, *errx.Error) {
	if !models.KnownViews[prefs.View] {
		return nil, errx.NewWithIdentifier(errx.NotFound, "unknown_view", "unknown view")
	}
	if xerr := validate(prefs); xerr != nil {
		return nil, xerr
	}
	if err := s.repo.Upsert(ctx, userID, orgID, prefs); err != nil {
		errs.CaptureException(err)
		return nil, errx.InternalError()
	}
	return s.Get(ctx, userID, orgID, prefs.View)
}

func (s *service) Reset(ctx context.Context, userID, orgID uuid.UUID, view string) *errx.Error {
	if !models.KnownViews[view] {
		return errx.NewWithIdentifier(errx.NotFound, "unknown_view", "unknown view")
	}
	if err := s.repo.Delete(ctx, userID, orgID, view); err != nil {
		errs.CaptureException(err)
		return errx.InternalError()
	}
	return nil
}

// validate checks the layout the way the contacts search will read it: every
// column id is one the dashboard can render, listed once, and the sort names a
// built-in column or a well-formed custom field. Custom-field ids are
// normalized in place so the same field saved with different spacing is one
// column.
func validate(prefs *models.ViewPreferences) *errx.Error {
	if len(prefs.Columns) > models.ViewPreferencesMaxColumns {
		return errx.NewWithIdentifier(errx.BadRequest, "too_many_columns", "too many columns")
	}
	seen := make(map[string]bool, len(prefs.Columns))
	for i, id := range prefs.Columns {
		norm, ok := normalizeColumnID(id)
		if !ok {
			return errx.NewWithIdentifier(errx.BadRequest, "invalid_column", "invalid column: "+id)
		}
		if seen[norm] {
			return errx.NewWithIdentifier(errx.BadRequest, "duplicate_column", "column listed twice: "+norm)
		}
		seen[norm] = true
		prefs.Columns[i] = norm
	}
	if prefs.Sort != nil {
		by := strings.TrimSpace(prefs.Sort.By)
		if by == "" {
			prefs.Sort = nil
		} else {
			norm, ok := normalizeColumnID(by)
			if !ok {
				return errx.NewWithIdentifier(errx.BadRequest, "invalid_sort", "invalid sort: "+by)
			}
			prefs.Sort.By = norm
		}
	}
	return nil
}

func normalizeColumnID(id string) (string, bool) {
	if len(id) > models.ViewColumnIDMaxLength {
		return "", false
	}
	if key, ok := models.ContactSortCustomField(id); ok {
		if !utils.IsValidJSONKey(key) {
			return "", false
		}
		return models.ContactSortCustomPrefix + key, true
	}
	if !builtinColumnPattern.MatchString(id) {
		return "", false
	}
	return id, true
}
