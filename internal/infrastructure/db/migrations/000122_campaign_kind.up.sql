-- A campaign is either a multi-step sequence or a one-time email: one message
-- to an audience, no follow-ups. Both send through the same pacer; the kind
-- only changes what the editor allows and how status reads in the list.
ALTER TABLE campaigns ADD COLUMN kind text NOT NULL DEFAULT 'sequence'
    CHECK (kind IN ('sequence', 'one_time'));
