-- Where warmup mail belongs in the customer's REAL mailbox.
--
-- Warmup traffic was always meant to be filed out of sight, but the
-- destination was a compile-time constant ("Warmbly") and the mode was not a
-- choice at all. A mailbox that already has another folder for this kind of
-- mail, or an owner who wants to watch the traffic in the inbox, had no way to
-- say so.
--
-- placement: 'folder'  file it in warmup_folder (default)
--            'inbox'   leave it where the provider put it
--            'archive' take it out of the inbox with no folder of its own
-- warmup_folder: '' means the instance default, so renaming the default later
-- moves every mailbox that never chose one.
ALTER TABLE public.email_accounts
    ADD COLUMN warmup_placement text NOT NULL DEFAULT 'folder',
    ADD COLUMN warmup_folder text NOT NULL DEFAULT '',
    ADD CONSTRAINT email_accounts_warmup_placement_check
        CHECK (warmup_placement IN ('folder', 'inbox', 'archive'));
