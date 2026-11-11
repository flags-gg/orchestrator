DROP TRIGGER IF EXISTS flag_webhook ON public.flag;
DROP FUNCTION IF EXISTS public.queue_flag_webhook();
DROP TABLE IF EXISTS public.webhook_delivery;
DROP TABLE IF EXISTS public.webhook_endpoint;
