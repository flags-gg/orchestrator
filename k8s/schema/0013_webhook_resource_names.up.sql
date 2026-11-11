-- Snapshot display names with each new event; existing delivery payloads stay unchanged.
CREATE OR REPLACE FUNCTION public.queue_flag_webhook() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    f public.flag%ROWTYPE;
    kind text;
    owner_id integer;
    company_key text;
    agent_key text;
    project_key text;
    env_key text;
    project_name text;
    agent_name text;
    environment_name text;
    event_id uuid := gen_random_uuid();
    event_body jsonb;
BEGIN
    IF TG_OP = 'DELETE' THEN
        f := OLD; kind := 'flag.archived';
    ELSIF TG_OP = 'INSERT' THEN
        f := NEW; kind := 'flag.created';
    ELSE
        IF NEW.enabled IS NOT DISTINCT FROM OLD.enabled AND NEW.rollout IS NOT DISTINCT FROM OLD.rollout
           AND NEW.rules IS NOT DISTINCT FROM OLD.rules AND NEW.name IS NOT DISTINCT FROM OLD.name THEN
            RETURN NEW;
        END IF;
        f := NEW; kind := 'flag.state_changed';
    END IF;
    SELECT c.id, c.company_id, p.project_id, a.agent_id, e.env_id, p.name, a.name, e.name
    INTO owner_id, company_key, project_key, agent_key, env_key, project_name, agent_name, environment_name
    FROM public.agent a JOIN public.project p ON p.id=a.project_id
    JOIN public.company c ON c.id=p.company_id JOIN public.environment e ON e.id=f.environment_id
    WHERE a.id=f.agent_id;
    IF owner_id IS NOT NULL THEN
        event_body := jsonb_build_object('id',event_id,'type',kind,'created_at',clock_timestamp(),
            'company_id',company_key,'data',jsonb_build_object('flag_id',f.id::text,'name',f.name,
            'ticket_ref',f.ticket_ref,'project_id',project_key,'agent_id',agent_key,'environment_id',env_key,
            'project_name',project_name,'agent_name',agent_name,'environment_name',environment_name,
            'enabled',f.enabled,'rollout',f.rollout));
        INSERT INTO public.webhook_delivery (endpoint_id,event_id,event_type,payload)
        SELECT id,event_id,kind,event_body FROM public.webhook_endpoint
        WHERE company_id=owner_id AND enabled AND kind=ANY(event_types);
    END IF;
    RETURN COALESCE(NEW,OLD);
END;
$$;
