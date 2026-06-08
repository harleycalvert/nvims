-- Migration: v0.15 -> v0.16
-- Run as the database owner (postgres):
--   sudo -u postgres psql nvims-sms -f src/migrate_v0.16.sql

ALTER TABLE public.classes ALTER COLUMN class_code TYPE varchar(80);
ALTER TABLE public.classes ADD COLUMN IF NOT EXISTS group_code varchar(20) NULL;

CREATE INDEX IF NOT EXISTS idx_classes_group
    ON public.classes(academic_period_id, group_code)
    WHERE (group_code IS NOT NULL);

-- Grant the nvims user permission to use the new column
GRANT UPDATE (group_code, class_code) ON public.classes TO nvims;
