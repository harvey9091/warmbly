package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/models"
)

// ViewPreferencesRepository stores one member's saved layout per dashboard
// list per workspace.
type ViewPreferencesRepository interface {
	// Get returns nil, nil when the member has never saved this view.
	Get(ctx context.Context, userID, orgID uuid.UUID, view string) (*models.ViewPreferences, error)
	Upsert(ctx context.Context, userID, orgID uuid.UUID, prefs *models.ViewPreferences) error
	Delete(ctx context.Context, userID, orgID uuid.UUID, view string) error
}

type viewPreferencesRepository struct {
	db *pgxpool.Pool
}

func NewViewPreferencesRepository(db *pgxpool.Pool) ViewPreferencesRepository {
	return &viewPreferencesRepository{db: db}
}

func (r *viewPreferencesRepository) Get(ctx context.Context, userID, orgID uuid.UUID, view string) (*models.ViewPreferences, error) {
	var (
		raw       []byte
		sortBy    string
		reverse   bool
		updatedAt time.Time
	)
	err := r.db.QueryRow(ctx, `
		SELECT columns, sort_by, sort_reverse, updated_at
		FROM user_view_preferences
		WHERE user_id = $1 AND organization_id = $2 AND view = $3`,
		userID, orgID, view).Scan(&raw, &sortBy, &reverse, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	prefs := &models.ViewPreferences{View: view, Columns: []string{}, UpdatedAt: &updatedAt}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &prefs.Columns); err != nil {
			return nil, err
		}
	}
	if sortBy != "" {
		prefs.Sort = &models.ViewSort{By: sortBy, Reverse: reverse}
	}
	return prefs, nil
}

func (r *viewPreferencesRepository) Upsert(ctx context.Context, userID, orgID uuid.UUID, prefs *models.ViewPreferences) error {
	cols := prefs.Columns
	if cols == nil {
		cols = []string{}
	}
	raw, err := json.Marshal(cols)
	if err != nil {
		return err
	}
	sortBy, reverse := "", false
	if prefs.Sort != nil {
		sortBy, reverse = prefs.Sort.By, prefs.Sort.Reverse
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO user_view_preferences (user_id, organization_id, view, columns, sort_by, sort_reverse, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (user_id, organization_id, view) DO UPDATE
		SET columns = EXCLUDED.columns,
		    sort_by = EXCLUDED.sort_by,
		    sort_reverse = EXCLUDED.sort_reverse,
		    updated_at = now()`,
		userID, orgID, prefs.View, raw, sortBy, reverse)
	return err
}

func (r *viewPreferencesRepository) Delete(ctx context.Context, userID, orgID uuid.UUID, view string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM user_view_preferences
		WHERE user_id = $1 AND organization_id = $2 AND view = $3`,
		userID, orgID, view)
	return err
}
