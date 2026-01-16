-- Make flag names unique per environment (independent of agent)
-- 1) Normalize data: align flags.agent_id with their environment's agent_id
UPDATE public.flag f
SET agent_id = e.agent_id
FROM public.environment e
WHERE f.environment_id = e.id
  AND (f.agent_id IS DISTINCT FROM e.agent_id);

-- 2) Remove duplicates that would violate the new uniqueness (environment_id, name)
--    Keep the most recently updated record
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY environment_id, name
               ORDER BY updated_at DESC, id DESC
           ) AS rn
    FROM public.flag
)
DELETE FROM public.flag f
USING ranked r
WHERE f.id = r.id
  AND r.rn > 1;

-- 3) Drop the old unique constraint if it exists
ALTER TABLE public.flag
    DROP CONSTRAINT IF EXISTS flag_unique_agent_env_name;

-- 4) Add the new unique constraint on (environment_id, name)
ALTER TABLE public.flag
    ADD CONSTRAINT flag_unique_env_name UNIQUE (environment_id, name);
