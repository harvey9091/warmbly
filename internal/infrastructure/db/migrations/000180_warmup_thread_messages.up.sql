-- A reply typed by hand at a partner mailbox carries no warmup token: it names
-- the warmup send it answers in In-Reply-To, and every later turn names the
-- turn before it. Each recognised turn is recorded here so the one answering
-- it is recognised too. Keyed by Message-ID first, because the same message is
-- looked up from whichever mailbox syncs a copy of the turn that follows.
CREATE TABLE warmup_thread_messages (
    message_id text NOT NULL,
    email_account_id uuid NOT NULL REFERENCES email_accounts(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, email_account_id)
);
CREATE INDEX idx_warmup_thread_messages_created ON warmup_thread_messages (created_at);
