-- Revert: make flag names unique per agent and environment
-- NOTE: This cannot restore any duplicate rows removed by the UP migration.

-- 1) Drop the new unique constraint on (environment_id, name)
ALTER TABLE public.flag
    DROP CONSTRAINT IF EXISTS flag_unique_env_name;

-- 2) Recreate the previous unique constraint on (agent_id, environment_id, name)
ALTER TABLE public.flag
    ADD CONSTRAINT flag_unique_agent_env_name UNIQUE (agent_id, environment_id, name);
