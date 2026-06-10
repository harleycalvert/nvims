-- WWCC (Working with Children Check) — stored on people because any person
-- (student, teacher, staff) can hold one card.
ALTER TABLE public.people
  ADD COLUMN IF NOT EXISTS wwcc_number TEXT NULL,
  ADD COLUMN IF NOT EXISTS wwcc_expiry DATE NULL;

-- Police check — employment-related, stored per role.
ALTER TABLE public.teachers
  ADD COLUMN IF NOT EXISTS police_check_status TEXT NULL,
  ADD COLUMN IF NOT EXISTS police_check_date   DATE NULL;

ALTER TABLE public.staff
  ADD COLUMN IF NOT EXISTS police_check_status TEXT NULL,
  ADD COLUMN IF NOT EXISTS police_check_date   DATE NULL;
