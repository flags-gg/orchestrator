-- Flags created by a ticket tracker (Tracklines) carry the ticket key; one row per environment as before.
ALTER TABLE public.flag ADD COLUMN ticket_ref character varying(255) NULL;
CREATE UNIQUE INDEX flag_env_ticket_unique ON public.flag (environment_id, ticket_ref) WHERE ticket_ref IS NOT NULL;
CREATE INDEX flag_ticket_ref_idx ON public.flag (ticket_ref) WHERE ticket_ref IS NOT NULL;

-- Removed flags are moved here, so SDK read paths never need an "archived" filter.
CREATE TABLE public.flag_archive (
    id serial PRIMARY KEY,
    archived_at timestamp without time zone NOT NULL DEFAULT now(),
    flag_id integer NOT NULL,
    name character varying(255) NULL,
    agent_id integer NOT NULL,
    environment_id integer NOT NULL,
    enabled boolean NOT NULL,
    ticket_ref character varying(255) NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);
CREATE INDEX flag_archive_ticket_ref_idx ON public.flag_archive (ticket_ref);

-- Machine credentials for integrations; only the sha256 of the token is stored.
CREATE TABLE public.integration_token (
    id serial PRIMARY KEY,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    company_id integer NOT NULL REFERENCES public.company(id) ON DELETE CASCADE,
    name character varying(255) NOT NULL,
    token_hash character(64) NOT NULL UNIQUE
);
