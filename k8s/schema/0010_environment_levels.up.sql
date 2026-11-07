-- FLA-110: environments made before levels existed all have level 0, so every one of them reads as
-- production and promotion has no order. Renumber the agents whose levels collide (0..n), following
-- the agent's promotion chain where it has one, else local/dev first and prod last, then age; agents
-- whose levels are already distinct are left alone. Then enforce one environment per level, which
-- 0001 declares but databases older than it never got.
WITH RECURSIVE clashing AS (
  SELECT agent_id FROM public.environment
  WHERE agent_id IS NOT NULL
  GROUP BY agent_id HAVING count(*) <> count(DISTINCT level)
), depth AS (
  -- a chain's first environment is 0, each child one more; unchained environments are 0
  SELECT e.id, 0 AS d FROM public.environment e
  WHERE e.agent_id IN (SELECT agent_id FROM clashing)
    AND NOT EXISTS (SELECT 1 FROM public.environment_chain c WHERE c.child_environment_id = e.id)
  UNION ALL
  SELECT c.child_environment_id, depth.d + 1
  FROM depth JOIN public.environment_chain c ON c.parent_environment_id = depth.id
), renumbered AS (
  SELECT e.id, (row_number() OVER (PARTITION BY e.agent_id ORDER BY
      COALESCE(max(depth.d), 0),
      CASE WHEN lower(e.name) ~ '(local|dev)' THEN 0 WHEN lower(e.name) ~ 'prod' THEN 2 ELSE 1 END,
      e.id) - 1)::smallint AS level
  -- every environment of a clashing agent gets a level, even one the chain walk never reaches (a loop):
  -- a single leftover duplicate would fail the constraint below, and with it the server's start
  FROM public.environment e LEFT JOIN depth ON depth.id = e.id
  WHERE e.agent_id IN (SELECT agent_id FROM clashing)
  GROUP BY e.id, e.agent_id, e.name
)
UPDATE public.environment e SET level = r.level FROM renumbered r WHERE e.id = r.id AND e.level <> r.level;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'environment_agent_level_unique') THEN
    ALTER TABLE public.environment ADD CONSTRAINT environment_agent_level_unique UNIQUE (agent_id, level);
  END IF;
END $$;
