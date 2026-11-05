-- A Clerk identity maps to one backend user. Required by the existing user upsert
-- and by concurrent first-login provisioning.
CREATE UNIQUE INDEX IF NOT EXISTS user_subject_unique ON public."user" (subject);
