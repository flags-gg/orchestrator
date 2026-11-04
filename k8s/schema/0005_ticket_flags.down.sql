DROP TABLE IF EXISTS public.integration_token;
DROP TABLE IF EXISTS public.flag_archive;
DROP INDEX IF EXISTS public.flag_ticket_ref_idx;
DROP INDEX IF EXISTS public.flag_env_ticket_unique;
ALTER TABLE public.flag DROP COLUMN IF EXISTS ticket_ref;
