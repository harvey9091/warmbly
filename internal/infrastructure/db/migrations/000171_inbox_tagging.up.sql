-- Automatic inbox tagging (phase 1: labels and a relevance score, no actions).
--
-- One row per inbound message that was classified. The raw probabilities are
-- stored alongside the conclusions on purpose: retuning the weights is the
-- whole product, and re-running the arithmetic over stored answers is free
-- while re-running the model over history costs money.
CREATE TABLE IF NOT EXISTS inbox_tag_results (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id   UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email_account_id  UUID NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,

    -- The provider Message-ID. Idempotency key: a webhook retry or a re-sync of
    -- the same message must not classify it twice, and must not spend a second
    -- call to reach the same answer.
    message_id        TEXT NOT NULL,
    thread_id         TEXT NOT NULL DEFAULT '',

    kind              TEXT NOT NULL DEFAULT '',
    kind_confidence   REAL NOT NULL DEFAULT 0,
    -- 'header' when the offline deterministic layer decided, 'model' when Jev
    -- did, and 'skipped' when nothing was asked.
    kind_source       TEXT NOT NULL DEFAULT '',

    intent            TEXT NOT NULL DEFAULT '',
    intent_confidence REAL NOT NULL DEFAULT 0,

    relevance         SMALLINT NOT NULL DEFAULT 0,
    priority          TEXT NOT NULL DEFAULT '',
    needs_review      BOOLEAN NOT NULL DEFAULT FALSE,
    review_reason     TEXT NOT NULL DEFAULT ''
                      CHECK (review_reason IN ('', 'kind', 'intent')),

    -- Every answer exactly as the API returned it, including the full
    -- probability distribution per question. This is what makes a retune
    -- reviewable instead of a guess.
    answers           JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- The labels this decision produced, for the review page and for telling
    -- an applied label apart from one a person added by hand.
    labels            TEXT[] NOT NULL DEFAULT '{}',

    model             TEXT NOT NULL DEFAULT '',
    input_tokens      INTEGER NOT NULL DEFAULT 0,

    status            TEXT NOT NULL DEFAULT 'complete'
                      CHECK (status IN ('processing', 'complete')),
    claimed_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The idempotency guarantee itself. Scoped by organization because a
-- Message-ID is only unique within the sending domain, not globally.
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_tag_results_message
    ON inbox_tag_results (organization_id, message_id);

-- The review page reads newest-first within a workspace, and sorts by
-- relevance. Both are the same index.
CREATE INDEX IF NOT EXISTS idx_inbox_tag_results_review
    ON inbox_tag_results (organization_id, relevance DESC, created_at DESC)
    WHERE status = 'complete';

CREATE INDEX IF NOT EXISTS idx_inbox_tag_results_thread
    ON inbox_tag_results (organization_id, thread_id);
