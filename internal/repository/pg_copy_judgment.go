package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/warmbly/warmbly/internal/app/copyjudge"
)

// CopyJudgmentRepository caches one TypeSafe verdict per piece of copy per
// organization, keyed by content hash, so an unchanged step is judged once
// however many times the Advisor runs or the editor re-checks it.
type CopyJudgmentRepository interface {
	// Get returns the stored verdict, or nil when this copy has not been
	// judged.
	Get(ctx context.Context, orgID uuid.UUID, hash string) (*copyjudge.Verdict, error)
	// Put stores or replaces the verdict for this copy.
	Put(ctx context.Context, orgID uuid.UUID, hash string, v *copyjudge.Verdict) error
}

type copyJudgmentRepository struct {
	db *pgxpool.Pool
}

func NewCopyJudgmentRepository(db *pgxpool.Pool) CopyJudgmentRepository {
	return &copyJudgmentRepository{db: db}
}

func (r *copyJudgmentRepository) Get(ctx context.Context, orgID uuid.UUID, hash string) (*copyjudge.Verdict, error) {
	var (
		raw    []byte
		model  string
		tokens int
	)
	err := r.db.QueryRow(ctx, `
		SELECT verdict, model, input_tokens FROM copy_judgments
		WHERE organization_id = $1 AND content_hash = $2`, orgID, hash).Scan(&raw, &model, &tokens)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("copy judgment get: %w", err)
	}
	var v copyjudge.Verdict
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("copy judgment decode: %w", err)
	}
	// The columns are the record of what was paid; the jsonb is the answer.
	v.Model, v.InputTokens = model, tokens
	return &v, nil
}

func (r *copyJudgmentRepository) Put(ctx context.Context, orgID uuid.UUID, hash string, v *copyjudge.Verdict) error {
	if v == nil {
		return errors.New("copy judgment put: nil verdict")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("copy judgment encode: %w", err)
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO copy_judgments (organization_id, content_hash, verdict, model, input_tokens)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organization_id, content_hash) DO UPDATE SET
			verdict = EXCLUDED.verdict, model = EXCLUDED.model,
			input_tokens = EXCLUDED.input_tokens, created_at = NOW()`,
		orgID, hash, raw, v.Model, v.InputTokens)
	if err != nil {
		return fmt.Errorf("copy judgment put: %w", err)
	}
	return nil
}
