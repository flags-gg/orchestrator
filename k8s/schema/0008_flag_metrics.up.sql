-- Users seen per environment per day (hashed targeting key), only to count each user once a day; pruned after 2 days.
CREATE TABLE public.environment_user_seen (
    environment_id integer NOT NULL REFERENCES public.environment(id) ON DELETE CASCADE,
    day date NOT NULL,
    key_hash bytea NOT NULL,
    PRIMARY KEY (environment_id, day, key_hash)
);

-- What each flag was served to those users: the server's own decision (rules, then rollout).
CREATE TABLE public.flag_exposure_daily (
    environment_id integer NOT NULL REFERENCES public.environment(id) ON DELETE CASCADE,
    flag_name character varying(255) NOT NULL,
    day date NOT NULL,
    served_on integer NOT NULL DEFAULT 0,
    served_off integer NOT NULL DEFAULT 0,
    rule_hits integer NOT NULL DEFAULT 0,
    PRIMARY KEY (environment_id, flag_name, day)
);

-- Daily users and a response-time histogram of the SDK flags endpoint
-- (buckets: <=5, <=10, <=25, <=50, <=100, <=250, <=500, <=1000, >1000 ms).
CREATE TABLE public.environment_usage_daily (
    environment_id integer NOT NULL REFERENCES public.environment(id) ON DELETE CASCADE,
    day date NOT NULL,
    users integer NOT NULL DEFAULT 0,
    latency_buckets integer[] NOT NULL DEFAULT '{0,0,0,0,0,0,0,0,0}',
    PRIMARY KEY (environment_id, day)
);
