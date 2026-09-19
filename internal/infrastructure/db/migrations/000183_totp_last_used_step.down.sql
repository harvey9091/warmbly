ALTER TABLE public.user_totp_settings
    DROP COLUMN IF EXISTS last_used_step;
