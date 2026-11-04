-- Targeting rules per flag and environment, evaluated in order on the server (never sent to SDKs):
-- [{"attribute":"email","operator":"ends_with","values":["@acme.com"],"serve":true}, ...]
ALTER TABLE public.flag ADD COLUMN rules jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE public.flag_archive ADD COLUMN rules jsonb NOT NULL DEFAULT '[]'::jsonb;
