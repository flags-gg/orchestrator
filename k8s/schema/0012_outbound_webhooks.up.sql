CREATE TABLE public.webhook_endpoint (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id integer NOT NULL REFERENCES public.company(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    name varchar(100) NOT NULL,
    url text NOT NULL,
    secret text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    event_types text[] NOT NULL CHECK (cardinality(event_types) > 0 AND event_types <@ ARRAY['flag.created','flag.state_changed','flag.archived']::text[])
);
CREATE INDEX webhook_endpoint_company ON public.webhook_endpoint(company_id);

CREATE TABLE public.webhook_delivery (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    endpoint_id uuid NOT NULL REFERENCES public.webhook_endpoint(id) ON DELETE CASCADE,
    event_id uuid NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','delivering','delivered','failed')),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_attempt_at timestamptz,
    response_status integer,
    last_error text,
    delivered_at timestamptz,
    UNIQUE(endpoint_id,event_id)
);
CREATE INDEX webhook_delivery_due ON public.webhook_delivery(next_attempt_at) WHERE status IN ('pending','delivering');
CREATE INDEX webhook_delivery_endpoint_log ON public.webhook_delivery(endpoint_id, id DESC);

-- An atomic outbox covers every writer: dashboard, integrations, MCP, promotion and direct API calls.
-- Unchanged updates (including idempotent integration calls) produce no duplicate event.
CREATE FUNCTION public.queue_flag_webhook() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    f public.flag%ROWTYPE;
    kind text;
    owner_id integer;
    company_key text;
    agent_key text;
    project_key text;
    env_key text;
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
    SELECT c.id, c.company_id, p.project_id, a.agent_id, e.env_id
    INTO owner_id, company_key, project_key, agent_key, env_key
    FROM public.agent a JOIN public.project p ON p.id=a.project_id
    JOIN public.company c ON c.id=p.company_id JOIN public.environment e ON e.id=f.environment_id
    WHERE a.id=f.agent_id;
    IF owner_id IS NOT NULL THEN
        event_body := jsonb_build_object('id',event_id,'type',kind,'created_at',clock_timestamp(),
            'company_id',company_key,'data',jsonb_build_object('flag_id',f.id::text,'name',f.name,
            'ticket_ref',f.ticket_ref,'project_id',project_key,'agent_id',agent_key,'environment_id',env_key,
            'enabled',f.enabled,'rollout',f.rollout));
        INSERT INTO public.webhook_delivery (endpoint_id,event_id,event_type,payload)
        SELECT id,event_id,kind,event_body FROM public.webhook_endpoint
        WHERE company_id=owner_id AND enabled AND kind=ANY(event_types);
    END IF;
    RETURN COALESCE(NEW,OLD);
END;
$$;
CREATE TRIGGER flag_webhook AFTER INSERT OR UPDATE OR DELETE ON public.flag
FOR EACH ROW EXECUTE FUNCTION public.queue_flag_webhook();
