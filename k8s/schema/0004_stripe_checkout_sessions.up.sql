-- Each Stripe checkout session can upgrade exactly one company, once
CREATE TABLE public.stripe_checkout_session (
    session_id character varying(255) PRIMARY KEY,
    company_id character varying(255) NOT NULL,
    created_at timestamp without time zone NOT NULL DEFAULT now()
);

-- Lets subscription webhooks find the company to downgrade
ALTER TABLE public.company
    ADD COLUMN stripe_customer_id character varying(255) NULL,
    ADD COLUMN stripe_subscription_id character varying(255) NULL;

CREATE INDEX company_stripe_subscription_idx
    ON public.company (stripe_subscription_id);
