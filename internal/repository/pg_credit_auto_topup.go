package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// CreditAutoTopUpAttemptRepository persists the Stripe objects used by an
// automatic credit purchase until the charge reaches a terminal state.
type CreditAutoTopUpAttemptRepository interface {
	GetOrCreatePending(ctx context.Context, orgID uuid.UUID, packKey string, credits int) (*models.CreditAutoTopUpAttempt, error)
	SetTaxCalculation(ctx context.Context, id uuid.UUID, taxCalculationID string) (*models.CreditAutoTopUpAttempt, error)
	SetPaymentIntent(ctx context.Context, id uuid.UUID, paymentIntentID string) (*models.CreditAutoTopUpAttempt, error)
	MarkSucceededByPaymentIntent(ctx context.Context, paymentIntentID string) error
	MarkFailed(ctx context.Context, id uuid.UUID, message string) error
}

type creditAutoTopUpAttemptRepository struct {
	DB *db.DB
}

func NewCreditAutoTopUpAttemptRepository(database *db.DB) CreditAutoTopUpAttemptRepository {
	return &creditAutoTopUpAttemptRepository{DB: database}
}

const creditAutoTopUpAttemptCols = `id, organization_id, pack_key, credits, status, tax_calculation_id, payment_intent_id, failure_message, created_at, updated_at, completed_at`

func scanCreditAutoTopUpAttempt(row pgx.Row, attempt *models.CreditAutoTopUpAttempt) error {
	return row.Scan(
		&attempt.ID,
		&attempt.OrgID,
		&attempt.PackKey,
		&attempt.Credits,
		&attempt.Status,
		&attempt.TaxCalculationID,
		&attempt.PaymentIntentID,
		&attempt.FailureMessage,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
		&attempt.CompletedAt,
	)
}

func (r *creditAutoTopUpAttemptRepository) GetOrCreatePending(ctx context.Context, orgID uuid.UUID, packKey string, credits int) (*models.CreditAutoTopUpAttempt, error) {
	attempt := &models.CreditAutoTopUpAttempt{}
	err := scanCreditAutoTopUpAttempt(r.DB.QueryRow(ctx, `
		INSERT INTO credit_auto_topup_attempts (organization_id, pack_key, credits)
		VALUES ($1, $2, $3)
		ON CONFLICT (organization_id) WHERE status = 'pending'
		DO UPDATE SET updated_at = credit_auto_topup_attempts.updated_at
		RETURNING `+creditAutoTopUpAttemptCols, orgID, packKey, credits), attempt)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (r *creditAutoTopUpAttemptRepository) SetTaxCalculation(ctx context.Context, id uuid.UUID, taxCalculationID string) (*models.CreditAutoTopUpAttempt, error) {
	attempt := &models.CreditAutoTopUpAttempt{}
	err := scanCreditAutoTopUpAttempt(r.DB.QueryRow(ctx, `
		UPDATE credit_auto_topup_attempts
		SET tax_calculation_id = CASE WHEN tax_calculation_id = '' THEN $2 ELSE tax_calculation_id END,
			updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+creditAutoTopUpAttemptCols, id, taxCalculationID), attempt)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (r *creditAutoTopUpAttemptRepository) SetPaymentIntent(ctx context.Context, id uuid.UUID, paymentIntentID string) (*models.CreditAutoTopUpAttempt, error) {
	attempt := &models.CreditAutoTopUpAttempt{}
	err := scanCreditAutoTopUpAttempt(r.DB.QueryRow(ctx, `
		UPDATE credit_auto_topup_attempts
		SET payment_intent_id = CASE WHEN payment_intent_id = '' THEN $2 ELSE payment_intent_id END,
			updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING `+creditAutoTopUpAttemptCols, id, paymentIntentID), attempt)
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func (r *creditAutoTopUpAttemptRepository) MarkSucceededByPaymentIntent(ctx context.Context, paymentIntentID string) error {
	_, err := r.DB.Exec(ctx, `
		UPDATE credit_auto_topup_attempts
		SET status = 'succeeded', failure_message = '', completed_at = now(), updated_at = now()
		WHERE payment_intent_id = $1`, paymentIntentID)
	return err
}

func (r *creditAutoTopUpAttemptRepository) MarkFailed(ctx context.Context, id uuid.UUID, message string) error {
	_, err := r.DB.Exec(ctx, `
		UPDATE credit_auto_topup_attempts
		SET status = 'failed', failure_message = $2, completed_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending'`, id, message)
	return err
}
