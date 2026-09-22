CREATE TABLE credit_auto_topup_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    pack_key TEXT NOT NULL,
    credits INTEGER NOT NULL CHECK (credits > 0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    tax_calculation_id TEXT NOT NULL DEFAULT '',
    payment_intent_id TEXT NOT NULL DEFAULT '',
    failure_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX credit_auto_topup_attempts_one_pending_org
    ON credit_auto_topup_attempts (organization_id)
    WHERE status = 'pending';

CREATE UNIQUE INDEX credit_auto_topup_attempts_payment_intent
    ON credit_auto_topup_attempts (payment_intent_id)
    WHERE payment_intent_id <> '';

CREATE INDEX credit_auto_topup_attempts_org_created
    ON credit_auto_topup_attempts (organization_id, created_at DESC);
