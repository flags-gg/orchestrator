CREATE TABLE
    public.agent (
                     id serial NOT NULL,
                     created_at timestamp without time zone NOT NULL DEFAULT now(),
                     name character varying(255) NULL,
                     project_id integer NOT NULL,
                     "interval" integer NOT NULL DEFAULT 60,
                     enabled boolean NOT NULL DEFAULT true,
                     agent_id character varying(255) NULL
);

ALTER TABLE
    public.agent
    ADD
        CONSTRAINT agent_pkey PRIMARY KEY (id);

CREATE TABLE
    public.company (
                       id serial NOT NULL,
                       created_at timestamp without time zone NOT NULL DEFAULT now(),
                       name character varying(255) NOT NULL,
                       company_id character varying(255) NOT NULL,
                       logo character varying(255) NULL,
                       payment_plan_id integer NOT NULL DEFAULT 1,
                       domain character varying(255) NULL,
                       invite_code character varying(255) NOT NULL,
                       timezone character varying(255) NOT NULL DEFAULT 'Europe/London'::character varying,
                       api_key character varying(255) NULL,
                       api_secret character varying(255) NULL
);

ALTER TABLE
    public.company
    ADD
        CONSTRAINT company_pkey PRIMARY KEY (id);

CREATE TABLE
    public.company_user (
                            id serial NOT NULL,
                            created_at timestamp without time zone NOT NULL DEFAULT now(),
                            company_id integer NOT NULL,
                            user_id integer NOT NULL
);

ALTER TABLE
    public.company_user
    ADD
        CONSTRAINT company_user_pkey PRIMARY KEY (id);

CREATE TABLE
    public.environment (
                           id serial NOT NULL,
                           created_at timestamp without time zone NOT NULL DEFAULT now(),
                           name character varying(255) NULL,
                           env_id character varying(255) NULL,
                           agent_id integer NULL,
                           "default" boolean NOT NULL DEFAULT false,
                           enabled boolean NOT NULL DEFAULT true,
                           level smallint NOT NULL DEFAULT 0
);

-- Ensure only one environment per level per agent (ordered chain)
ALTER TABLE public.environment
    ADD CONSTRAINT environment_agent_level_unique UNIQUE (agent_id, level);

-- Explicit parent-child relationships per agent to enforce promotion adjacency
CREATE TABLE public.environment_chain (
    id serial PRIMARY KEY,
    agent_id integer NOT NULL,
    parent_environment_id integer NOT NULL,
    child_environment_id integer NOT NULL,
    created_at timestamp without time zone NOT NULL DEFAULT now(),
    CONSTRAINT fk_chain_agent FOREIGN KEY (agent_id) REFERENCES public.agent(id) ON DELETE CASCADE,
    CONSTRAINT fk_chain_parent FOREIGN KEY (parent_environment_id) REFERENCES public.environment(id) ON DELETE CASCADE,
    CONSTRAINT fk_chain_child FOREIGN KEY (child_environment_id) REFERENCES public.environment(id) ON DELETE CASCADE,
    -- Linear chain: ensure one child per parent and a child belongs to only one parent per agent
    CONSTRAINT chain_unique_parent UNIQUE (agent_id, parent_environment_id),
    CONSTRAINT chain_unique_child UNIQUE (agent_id, child_environment_id)
);

-- Guard that both parent and child belong to same agent
CREATE OR REPLACE FUNCTION check_chain_same_agent() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM public.environment e1
        WHERE e1.id = NEW.parent_environment_id AND e1.agent_id = NEW.agent_id
    ) OR NOT EXISTS (
        SELECT 1 FROM public.environment e2
        WHERE e2.id = NEW.child_environment_id AND e2.agent_id = NEW.agent_id
    ) THEN
        RAISE EXCEPTION 'Parent/child environments must belong to the same agent';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_chain_same_agent ON public.environment_chain;
CREATE TRIGGER trg_chain_same_agent
    BEFORE INSERT OR UPDATE ON public.environment_chain
    FOR EACH ROW EXECUTE FUNCTION check_chain_same_agent();

ALTER TABLE
    public.environment
    ADD
        CONSTRAINT environment_pkey PRIMARY KEY (id);

CREATE TABLE
    public.flag (
                    id serial NOT NULL,
                    created_at timestamp without time zone NOT NULL DEFAULT now(),
                    name character varying(255) NULL,
                    agent_id integer NOT NULL,
                    environment_id integer NOT NULL,
                    enabled boolean NOT NULL DEFAULT false,
                    updated_at timestamp without time zone NOT NULL DEFAULT now()
);

ALTER TABLE
    public.flag
    ADD
        CONSTRAINT flag_pkey PRIMARY KEY (id);

-- Ensure a flag name is unique per agent and environment for upsert
ALTER TABLE public.flag
    ADD CONSTRAINT flag_unique_agent_env_name UNIQUE (agent_id, environment_id, name);

CREATE TABLE
    public.payment_plans (
                             id serial NOT NULL,
                             created_at timestamp without time zone NOT NULL DEFAULT now(),
                             price integer NOT NULL DEFAULT 0,
                             team_members integer NOT NULL DEFAULT 10,
                             projects integer NOT NULL DEFAULT 1,
                             environments integer NOT NULL DEFAULT 2,
                             agents integer NOT NULL DEFAULT 1,
                             requests integer NOT NULL DEFAULT 50000,
                             support_category character varying(255) NULL,
                             name character varying(255) NULL,
                             popular boolean NOT NULL DEFAULT false,
                             custom boolean NOT NULL DEFAULT false,
                             stripe_id character varying(255) NULL,
                             stripe_id_dev character varying(255) NULL
);

ALTER TABLE
    public.payment_plans
    ADD
        CONSTRAINT payment_plans_pkey PRIMARY KEY (id);

CREATE TABLE
    public.project (
                       id serial NOT NULL,
                       created_at timestamp without time zone NOT NULL DEFAULT now(),
                       name character varying(255) NULL,
                       company_id integer NOT NULL,
                       project_id character varying(255) NOT NULL,
                       logo character varying(255) NULL,
                       enabled boolean NOT NULL DEFAULT false
);

ALTER TABLE
    public.project
    ADD
        CONSTRAINT project_pkey PRIMARY KEY (id);

CREATE TABLE
    public.secret_menu (
                           id serial NOT NULL,
                           created_at timestamp without time zone NOT NULL DEFAULT now(),
                           menu_id character varying(255) NULL,
                           code text NULL,
                           agent_id integer NULL,
                           enabled boolean NOT NULL DEFAULT false,
                           environment_id integer NULL
);

ALTER TABLE
    public.secret_menu
    ADD
        CONSTRAINT secret_menu_pkey PRIMARY KEY (id);

CREATE TABLE
    public.secret_menu_style (
                                 id serial NOT NULL,
                                 created_at timestamp without time zone NOT NULL DEFAULT now(),
                                 secret_menu_id integer NOT NULL,
                                 style_id character varying(255) NOT NULL,
                                 close_button text NULL,
                                 container text NULL,
                                 reset_button text NULL,
                                 flag text NULL,
                                 header text NULL,
                                 button_enabled text NULL,
                                 button_disabled text NULL
);

ALTER TABLE
    public.secret_menu_style
    ADD
        CONSTRAINT secret_menu_style_pkey PRIMARY KEY (id);

CREATE TABLE
    public."user" (
                      id serial NOT NULL,
                      created_at timestamp without time zone NOT NULL DEFAULT now(),
                      user_group_id integer NOT NULL DEFAULT 3,
                      onboarded boolean NOT NULL DEFAULT false,
                      known_as character varying(255) NULL,
                      email_address character varying(255) NULL,
                      subject character varying(255) NOT NULL,
                      avatar character varying(255) NULL,
                      job_title character varying(255) NULL,
                      location character varying(255) NULL,
                      timezone character varying(255) NOT NULL DEFAULT 'Europe/London'::character varying,
                      first_name character varying(255) NULL,
                      last_name character varying(255) NULL,
                      country character varying(255) NULL
);

ALTER TABLE
    public."user"
    ADD
        CONSTRAINT user_pkey PRIMARY KEY (id);

CREATE TABLE
    public.user_groups (
                           id serial NOT NULL,
                           created_at timestamp without time zone NOT NULL DEFAULT now(),
                           name character varying(255) NULL
);

ALTER TABLE
    public.user_groups
    ADD
        CONSTRAINT user_groups_pkey PRIMARY KEY (id);

CREATE TABLE
    public.user_notifications (
                                  id serial NOT NULL,
                                  created_at timestamp without time zone NOT NULL DEFAULT now(),
                                  user_id integer NULL,
                                  subject character varying(255) NULL,
                                  content character varying(255) NULL,
                                  action character varying(255) NULL,
                                  read boolean NOT NULL DEFAULT false,
                                  deleted boolean NOT NULL DEFAULT false
);

ALTER TABLE
    public.user_notifications
    ADD
        CONSTRAINT user_notifications_pkey PRIMARY KEY (id);
