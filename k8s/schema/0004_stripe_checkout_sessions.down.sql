DROP INDEX IF EXISTS public.company_stripe_subscription_idx;

ALTER TABLE public.company
    DROP COLUMN IF EXISTS stripe_subscription_id,
    DROP COLUMN IF EXISTS stripe_customer_id;

DROP TABLE IF EXISTS public.stripe_checkout_session;
