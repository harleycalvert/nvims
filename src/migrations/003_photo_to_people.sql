-- Move photo_url / photo_uploaded_at from students to people.
-- Photos belong to the identity record so any person (teacher, staff)
-- can have a profile photo without holding a student row.

ALTER TABLE public.people
  ADD COLUMN IF NOT EXISTS photo_url         varchar(2048) NULL,
  ADD COLUMN IF NOT EXISTS photo_uploaded_at timestamp with time zone NULL;

-- Copy existing student photos to people before dropping the columns.
UPDATE public.people p
   SET photo_url         = s.photo_url,
       photo_uploaded_at = s.photo_uploaded_at
  FROM public.students s
 WHERE s.id = p.id
   AND s.photo_url IS NOT NULL;

ALTER TABLE public.students
  DROP COLUMN IF EXISTS photo_url,
  DROP COLUMN IF EXISTS photo_uploaded_at;
