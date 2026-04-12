CREATE TABLE public.environment_request_audit (
    id serial PRIMARY KEY,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    project_id character varying(255) NOT NULL,
    agent_id character varying(255) NOT NULL,
    environment_id character varying(255) NOT NULL,
    request_kind character varying(32) NOT NULL,
    request_source character varying(32) NOT NULL
);

CREATE INDEX environment_request_audit_project_idx
    ON public.environment_request_audit (project_id);

CREATE INDEX environment_request_audit_environment_idx
    ON public.environment_request_audit (environment_id);

CREATE INDEX environment_request_audit_kind_idx
    ON public.environment_request_audit (request_kind);

CREATE TABLE public.api_key_audit (
    id serial PRIMARY KEY,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    project_id character varying(255) NOT NULL,
    agent_id character varying(255) NOT NULL,
    environment_id character varying(255) NULL,
    created_by_subject character varying(255) NOT NULL
);

CREATE INDEX api_key_audit_project_idx
    ON public.api_key_audit (project_id);

CREATE INDEX api_key_audit_created_by_idx
    ON public.api_key_audit (created_by_subject);
