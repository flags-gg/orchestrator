-- Percentage of visitors (by stable bucket) that see an enabled flag; 100 = everyone.
ALTER TABLE public.flag ADD COLUMN rollout smallint NOT NULL DEFAULT 100 CHECK (rollout BETWEEN 0 AND 100);
ALTER TABLE public.flag_archive ADD COLUMN rollout smallint NOT NULL DEFAULT 100;
