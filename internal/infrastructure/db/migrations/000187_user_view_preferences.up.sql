-- One member's own layout for one of the dashboard's lists: which columns show,
-- in what order, and how the list is sorted. Keyed by person and workspace,
-- because the columns a list can carry (custom fields) differ per workspace.
--
-- The column list is a jsonb array of column ids the dashboard defines
-- ("company", "custom:Industry"); it is read back whole and never filtered in
-- SQL. The sort is typed so it can be validated and read like any other column.

CREATE TABLE IF NOT EXISTS public.user_view_preferences (
    user_id         uuid NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES public.organizations (id) ON DELETE CASCADE,
    view            text NOT NULL,
    columns         jsonb NOT NULL DEFAULT '[]'::jsonb,
    sort_by         text NOT NULL DEFAULT '',
    sort_reverse    boolean NOT NULL DEFAULT false,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, organization_id, view),
    CONSTRAINT user_view_preferences_view_check CHECK (view ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT user_view_preferences_columns_check CHECK (jsonb_typeof(columns) = 'array')
);

COMMENT ON TABLE public.user_view_preferences IS
    'A member''s own column layout and sort for one dashboard list, per workspace. Read by GET /v1/me/views/:view.';
