-- FLA-111: where a flag goes when it's promoted. Chain links are only written when an environment is
-- cloned, so an environment made any other way had nowhere to promote to in the dashboard, while the
-- Tracklines integration promotes by level. One answer for both: the chain's child if there is one,
-- else the same agent's environment with the next level up, else NULL (the last one).
CREATE OR REPLACE FUNCTION public.next_environment(env integer) RETURNS integer
LANGUAGE sql STABLE AS $$
  SELECT COALESCE(
    (SELECT ec.child_environment_id FROM public.environment_chain ec WHERE ec.parent_environment_id = env),
    (SELECT n.id FROM public.environment e
       JOIN public.environment n ON n.agent_id = e.agent_id AND n.level > e.level
     WHERE e.id = env ORDER BY n.level LIMIT 1)
  )
$$;
