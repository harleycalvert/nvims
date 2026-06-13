-- =========================================================================
-- AVETMISS-compliant SMS schema  --  version 0.32, 2026-06-13
-- =========================================================================
-- Changes from v0.31:
--   1.  teacher_vcc_professional_qualifications: added aqf_level smallint NULL.
--   2.  teacher_vcc_vocational_qualifications: added aqf_level smallint NULL.
-- Changes from v0.30:
--   1.  Replaced qual_type discriminator with a dedicated
--       teacher_vcc_vocational_qualifications table (same schema as
--       teacher_vcc_professional_qualifications, no qual_type column).
--   2.  teacher_document_connections: added vcc_vocational_qual_id FK and
--       updated num_nonnulls check to include it.
-- Changes from v0.29:
--   1.  teacher_vcc_professional_qualifications: added qual_type (now removed).
-- Changes from v0.28:
--   1.  teacher_documents: document_url and file_name made nullable to
--       support link-only documents (title + external_url, no file upload)
--       for VCC evidence records.
-- Changes from v0.27:
--   1.  buildings: added address text NULL, suburb varchar(50) NULL,
--       state_code varchar(3) NULL (FK → australian_states), and
--       postcode varchar(4) NULL for buildings at distinct street addresses.
-- Changes from v0.26:
--   1.  delivery_locations: added latitude numeric(9,6) NULL and
--       longitude numeric(9,6) NULL for GPS coordinates.
--   2.  buildings: added latitude numeric(9,6) NULL and
--       longitude numeric(9,6) NULL for GPS coordinates.
-- Changes from v0.25:
--   1.  teacher_documents: added external_url varchar(2048) NULL for linking
--       to digital badges, eQuals transcripts, or other verification pages.
--   2.  rooms: added is_computer_lab boolean NOT NULL DEFAULT false.
--   3.  room_computer_lab_specs: new table — hardware profile for lab rooms
--       (workstations, ram_gb, has_microphone, has_webcam).
--   4.  room_lab_software: new table — software titles installed per lab room.
--   5.  room_issues: new table — fault/maintenance issues per room with
--       Open/Investigating/Resolved workflow.
--   6.  person_location_preferences: new table — per-person ranked delivery
--       location preferences; equal ranks permitted.
-- Changes from v0.24:
--   1.  app_users.role (single varchar) replaced by app_user_roles table.
--       A user may hold multiple roles simultaneously (e.g. Trainer + Student).
--       Roleless accounts are valid (service accounts, pending onboarding).
--       app_users.is_active remains as an account-level lock.
--       app_user_roles.revoked_at is NULL while the role is active; set to the
--       revocation timestamp when removed. The surrogate PK allows re-granting
--       a previously revoked role while preserving audit history. A partial
--       unique index (uq_aur_active_role) prevents duplicate active grants for
--       the same (user_id, role) pair. Login evaluation: account must be active
--       AND at least one role row must have revoked_at IS NULL.
-- Changes from v0.23:
--   1.  police_check_status / police_check_date moved from teachers and staff
--       to people. Any person (student, teacher, staff, guardian) may be subject
--       to a police check; storing it on people avoids duplication across role
--       tables and is consistent with the existing wwcc_number / wwcc_expiry
--       pattern.
-- Changes from v0.22:
--   1.  program_intakes gains graded_assessment boolean NOT NULL DEFAULT false.
--       When true, the results UI requires a grade value (e.g. P, CR, D, HD)
--       in addition to the standard VET competency outcome (SC / NS) for all
--       subjects in the intake. The actual grade is stored in
--       client_subject_enrolments.grade.
-- Changes from v0.21:
--   1.  program_intakes.intake_name widened varchar(100) → varchar(150) to
--       accommodate long program names with a term suffix appended.
-- Changes from v0.20:
--   1.  duration_years numeric(3,1) added to program_intakes — the calendar
--       duration of the program in years (e.g. 1.0, 1.5, 2.0). Nullable so
--       existing rows are unaffected; constrained > 0 when set.
-- Changes from v0.19:
--   1.  program_intakes and intake_groups tables added (section 4.5).
--       A program may be offered as multiple scheduled intakes (one per term,
--       per year, etc.). Each intake may have one or more groups — sub-cohorts
--       that attend classes together. intake_group_id (nullable FK) added to
--       student_course_enrollments and classes.
-- Changes from v0.18:
--   1.  photo_url varchar(2048) and photo_uploaded_at timestamptz moved from
--       public.students to public.people (both nullable). Photos belong to the
--       identity record, not the student role — teachers and staff can also
--       have profile photos without holding a student row.
-- Changes from v0.17:
--   1.  people gains wwcc_number text and wwcc_expiry date for Working with
--       Children Check details. Any person (student, teacher, staff) may hold
--       a WWCC card; storing it on people avoids duplication across role tables.
--   2.  teachers gains police_check_status text and police_check_date date
--       (moved to people in v0.24).
--   3.  staff gains the same police_check_status / police_check_date columns
--       (moved to people in v0.24).
-- Changes from v0.16:
--   1.  preferred_contact_method varchar(20) added to people.
--   2.  is_emergency_contact boolean added to student_guardians: marks a
--       guardian row as also being the primary emergency contact, superseding
--       the inline emergency_contact_* fields on people for multi-contact use.
--   3.  session_attendance extended: units_nominated smallint, arrived_at /
--       departed_at time (partial-attendance support), break_minutes smallint,
--       absence_reason varchar(100), absence_is_acceptable boolean,
--       has_childcare boolean, is_note_private boolean. chk_attendance_status
--       extended with 'Not-Applicable'.
--   4.  keywords text[] added to student_progress_reports.
--   5.  subject_programs gains is_core boolean, group_code varchar(20), and
--       group_title varchar(100) to record Core/Elective classification and
--       the TGA component group (e.g. "Specialisation — Cloud Technology").
--   6.  student_notes.chk_note_type extended to include 'Communication'.
--   7.  student_employment_services: Centrelink CRN, job seeker ID,
--       participation hours/type/comment — one row per student.
--   8.  student_employment_registrations: provider registration rows, child
--       of student_employment_services.
--   9.  programs gains program_type varchar(20): distinguishes Qualification,
--       Skill Set, Course in a Package, Statement of Attainment. Skill sets
--       and other enrolable products are rows in programs, differentiated by
--       this column. Specialisations are captured by group_code / group_title
--       on subject_programs (added in item 5).
--  10.  teacher_vccs: Vocational Competency & Currency document, versioned per year,
--       with approval workflow (Draft → Submitted → Approved).
--  11.  teacher_vcc_professional_qualifications: teacher's own credentials
--       (TAE, degrees, industry certs) recorded against a VCC version.
--  12.  teacher_vcc_courses: courses covered in a VCC (may link to programs).
--  13.  teacher_vcc_units: units the teacher has currency for, with competency
--       method and justification text. Multiple rows per unit allowed (one per
--       method). Standalone or grouped under a VCC course.
--  14.  teacher_documents: per-teacher document library (testamurs,
--       transcripts, credentials, and other supporting evidence).
--  15.  teacher_document_connections: M2M linking documents to exactly one VCC
--       entity — professional qualification, unit, or currency activity.
--  16.  teacher_currency_activities: vocational and professional currency
--       point records with activity detail and approval tracking.
--  17.  teacher_currency_unit_links: units related to a currency activity.
--  18.  teacher_vcc_profiling: spider-chart dimension scores (self, supervisor,
--       business ideal) stored per VCC version.
-- Changes from v0.15:
--   1.  classes.group_code varchar(20): optional cohort label (G1, G2 …).
--       Groups multiple subject-level classes into one student cohort.
--       Index idx_classes_group on (academic_period_id, group_code).
-- Changes from v0.14:
--   2.  fn_upper_family_name / trg_upper_family_name: BEFORE INSERT OR UPDATE
--       OF family_name ON people normalises family_name to UPPER() at the
--       database level so every insert path (seed, API, import) stores
--       consistent uppercase surnames without caller discipline.
-- Changes from v0.13:
--   1.  messages table: teacher-composed individual messages. Separate from
--       the campaign system. Status workflow Draft -> Sent -> Failed.
--       sender_id FK to app_users (must hold Teacher role).
--   2.  message_recipients table: per-recipient delivery rows for direct
--       messages. Mirrors message_deliveries but adds teacher_id FK and an
--       is_cc boolean. recipient_type covers Student/Guardian/Staff/Teacher.
--       Exactly one of the four nullable recipient FKs is set, enforced by
--       num_nonnulls(). read_at records inbox acknowledgement.
--   3.  fn_cc_sender_on_send / trg_cc_sender_on_send: after messages.status
--       transitions to 'Sent', automatically inserts a message_recipients
--       row with is_cc = true pointing back to the sender. Sender resolved
--       as Teacher first, then Staff; service accounts produce no CC row.
--   4.  trg_touch_messages wired to fn_set_updated_at().
--   5.  Indexes on messages(sender_id, status) and
--       message_recipients(message_id), (teacher_id).
-- Changes from v0.12 (retained as v0.13):
--   6.  pay_periods table: administrator-defined pay periods (typically
--       fortnightly). Unique on period_start and on (calendar_year,
--       period_name). Provides an ordered, named sequence for timesheet
--       generation and payroll export.
--   7.  timesheets table: one timesheet per teacher per pay period. Status
--       workflow Draft -> Submitted -> Approved -> Exported. Stores who
--       submitted and approved, export timestamp and format. No banking,
--       super, or rate data -- this record carries hours only.
--   8.  timesheet_entries table: individual hour lines. entry_type mirrors
--       workplan categories plus Other. class_session_id links
--       auto-populated Teaching Delivery rows to their source session;
--       workplan_entry_id optionally links to the annual plan for
--       reconciliation. is_overtime flag separates ordinary from overtime.
--   9.  Indexes on timesheets(teacher_id), (pay_period_id), (status) and
--       timesheet_entries(timesheet_id), (class_session_id).
--  10.  trg_touch_timesheets wired to fn_set_updated_at().
--  11.  vw_timesheet_summary: per-timesheet ordinary/overtime totals and
--       per-category breakdowns (teaching, CAPPS, ERD, other).
-- =========================================================================

BEGIN;

-- =========================================================================
-- 1. PRE-REQUISITES, CUSTOM TYPES & REFERENCE REGISTRIES
-- =========================================================================
CREATE EXTENSION IF NOT EXISTS btree_gist;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'employment_type') THEN
        CREATE TYPE public.employment_type AS ENUM ('Full-Time', 'Part-Time', 'Casual');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'trainee_worker_type') THEN
        CREATE TYPE public.trainee_worker_type AS ENUM ('New Worker', 'Existing Worker');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'credit_type') THEN
        CREATE TYPE public.credit_type AS ENUM ('RPL', 'Credit Transfer');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'timerange') THEN
        CREATE TYPE public.timerange AS RANGE (subtype = time);
    END IF;
    -- NEW v11: distinguishes VET-only, HE-only, and dual-sector teachers.
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'teacher_sector') THEN
        CREATE TYPE public.teacher_sector AS ENUM ('VET', 'HE', 'DUAL');
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS public.australian_states (
    state_code varchar(3) NOT NULL,
    state_name varchar(50) NOT NULL,
    state_training_authority_name varchar(100) NOT NULL,
    avetmiss_state_id char(2) NOT NULL,           -- numeric NAT identifier (NAT00020/00080)
    PRIMARY KEY (state_code),
    CONSTRAINT uq_state_avetmiss_id UNIQUE (avetmiss_state_id)
);

INSERT INTO public.australian_states (state_code, state_name, state_training_authority_name, avetmiss_state_id) VALUES
('NSW', 'New South Wales', 'Training Services NSW', '01'),
('VIC', 'Victoria', 'Department of Education and Training (Skills Victoria)', '02'),
('QLD', 'Queensland', 'Department of Employment, Small Business and Training', '03'),
('SA',  'South Australia', 'Skills SA', '04'),
('WA',  'Western Australia', 'Department of Training and Workforce Development', '05'),
('TAS', 'Tasmania', 'Skills Tasmania', '06'),
('NT',  'Northern Territory', 'Department of Industry, Tourism and Trade', '07'),
('ACT', 'Australian Capital Territory', 'Skills Canberra', '08')
ON CONFLICT (state_code) DO NOTHING;

-- NEW v11: BLOCK and ROLLING added to support 6-8 week intensive blocks and
-- rolling monthly intakes used by some RTOs and private HE colleges.
-- sequence_number orders periods within a year (Semester 1 = 1, Block 3 = 3, etc.).
CREATE TABLE IF NOT EXISTS public.academic_periods (
    id bigserial NOT NULL,
    period_code varchar(20) NOT NULL,
    year smallint NOT NULL,
    period_name varchar(50) NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    period_type varchar(10) NOT NULL,
    sequence_number smallint NULL,            -- ordinal within the year; NULL = unordered/rolling
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_academic_period_code UNIQUE (period_code),
    CONSTRAINT chk_period_type CHECK (period_type IN ('TERM', 'SEMESTER', 'TRIMESTER', 'YEAR', 'BLOCK', 'ROLLING')),
    CONSTRAINT chk_period_dates CHECK (end_date >= start_date),
    CONSTRAINT chk_sequence_number CHECK (sequence_number IS NULL OR sequence_number > 0)
);

CREATE TABLE IF NOT EXISTS public.disability_types (
    disability_id varchar(2) NOT NULL,
    disability_name varchar(100) NOT NULL,
    PRIMARY KEY (disability_id)
);

CREATE TABLE IF NOT EXISTS public.prior_educational_achievements (
    achievement_id varchar(3) NOT NULL,
    achievement_name varchar(100) NOT NULL,
    PRIMARY KEY (achievement_id)
);

-- small fixed lookup for NAT00080 "highest school level completed"
CREATE TABLE IF NOT EXISTS public.highest_school_levels (
    level_id varchar(2) NOT NULL,
    level_name varchar(100) NOT NULL,
    PRIMARY KEY (level_id)
);
INSERT INTO public.highest_school_levels (level_id, level_name) VALUES
('02','Did not go to school'),
('08','Year 8 or below'),
('09','Year 9 or equivalent'),
('10','Year 10 or equivalent'),
('11','Year 11 or equivalent'),
('12','Year 12 or equivalent'),
('@@','Not specified')
ON CONFLICT (level_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS public.secondary_schools (
    id bigserial NOT NULL,
    school_name varchar(100) NOT NULL,
    national_school_code varchar(10) NULL,
    school_state_code varchar(3) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_school_state FOREIGN KEY (school_state_code) REFERENCES public.australian_states(state_code)
);

-- =========================================================================
-- 2. IDENTITY & CORE ENTITIES  (people is the spine of the shared-PK model)
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.people (
    id bigserial NOT NULL,
    title varchar(10) NULL,
    first_given_name varchar(40) NOT NULL,
    family_name varchar(40) NOT NULL,
    preferred_name varchar(50) NULL,
    dob date NOT NULL,
    gender varchar(1) NOT NULL,
    building_property_name varchar(50) NULL,
    unit_details varchar(30) NULL,
    street_number varchar(10) NULL,
    street_name varchar(70) NULL,
    suburb varchar(50) NOT NULL,
    state_code varchar(3) NOT NULL,
    postcode varchar(4) NOT NULL,
    postal_delivery_info varchar(50) NULL,
    country_id varchar(4) NOT NULL DEFAULT '1101',
    primary_email varchar(100) NOT NULL,
    secondary_email varchar(100) NULL,
    phone_home varchar(15) NULL,
    phone_work varchar(15) NULL,
    phone_mobile varchar(15) NULL,
    emergency_contact_name varchar(100) NULL,
    emergency_contact_phone varchar(15) NULL,
    emergency_contact_relationship varchar(30) NULL,
    preferred_contact_method varchar(20) NULL,
    wwcc_number text NULL,                           -- NEW v18: Working with Children Check
    wwcc_expiry date NULL,
    police_check_status text NULL,                   -- 'Pending', 'Clear', 'Not Required', or NULL
    police_check_date date NULL,
    photo_url varchar(2048) NULL,                    -- profile/ID photo
    photo_uploaded_at timestamp with time zone NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_people_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_people_email UNIQUE (primary_email),
    CONSTRAINT chk_people_email CHECK (primary_email LIKE '%@%.%'),
    CONSTRAINT chk_avetmiss_gender CHECK (gender IN ('M', 'F', 'X')),
    CONSTRAINT chk_postcode_format CHECK (postcode ~ '^[0-9]{4}$')
);

-- authentication / system-actor accounts. Every *_by column FKs here.
CREATE TABLE IF NOT EXISTS public.app_users (
    id bigserial NOT NULL,
    person_id bigint NULL,                       -- NULL allowed for service accounts
    username varchar(100) NOT NULL,
    password_hash varchar(255) NOT NULL DEFAULT '',
    is_active boolean NOT NULL DEFAULT true,     -- account-level lock; disables all roles when false
    last_login_at timestamp with time zone NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_app_user_username UNIQUE (username),
    CONSTRAINT fk_app_user_person FOREIGN KEY (person_id) REFERENCES public.people(id) ON DELETE SET NULL
);

-- Per-user role grants. revoked_at NULL = currently active; non-NULL = revoked at that time.
-- Surrogate PK allows the same role to be re-granted after revocation (history is preserved).
-- uq_aur_active_role prevents duplicate active grants for the same (user, role) pair.
CREATE TABLE IF NOT EXISTS public.app_user_roles (
    id         bigserial   NOT NULL,
    user_id    bigint      NOT NULL,
    role       varchar(30) NOT NULL,
    granted_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at timestamp with time zone NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_aur_user FOREIGN KEY (user_id) REFERENCES public.app_users(id) ON DELETE CASCADE,
    CONSTRAINT chk_aur_role CHECK (role IN (
        'Admin','Trainer','Compliance','Reception','SupportStaff','System','Staff','Student'
    ))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_aur_active_role
    ON public.app_user_roles(user_id, role)
    WHERE (revoked_at IS NULL);

CREATE TABLE IF NOT EXISTS public.faculties (
    id bigserial NOT NULL,
    faculty_name varchar(100) NOT NULL,
    PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS public.students (
    id bigint NOT NULL,                          -- shared PK: this IS people.id
    student_number varchar(20) NOT NULL,
    student_email varchar(100) NOT NULL,
    usi varchar(10) NULL,
    indigenous_status_id varchar(1) NOT NULL DEFAULT '9',
    country_of_birth_id varchar(4) NOT NULL DEFAULT '1101',
    language_id varchar(4) NOT NULL DEFAULT '1201',
    english_proficiency_id varchar(1) NULL,
    labour_force_status_id varchar(2) NULL,
    highest_school_level_id varchar(2) NULL,
    year_highest_school_completed smallint NULL,
    disability_flag varchar(1) NOT NULL DEFAULT 'N',
    prior_educational_achievement_flag varchar(1) NOT NULL DEFAULT 'N',
    secondary_school_id bigint NULL,
    state_allocated_student_number varchar(20) NULL,
    state_identity_issuing_body_code varchar(3) NULL,
    at_school_flag varchar(1) NOT NULL DEFAULT 'N',
    id_expiry_date date NULL,
    id_document_type varchar(50) NULL,
    id_document_number varchar(50) NULL,
    deleted_at timestamp with time zone NULL,
    deleted_by bigint NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_students_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_student_state_body FOREIGN KEY (state_identity_issuing_body_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT fk_student_school FOREIGN KEY (secondary_school_id) REFERENCES public.secondary_schools(id) ON DELETE SET NULL,
    CONSTRAINT fk_student_school_level FOREIGN KEY (highest_school_level_id) REFERENCES public.highest_school_levels(level_id),
    CONSTRAINT fk_student_deleted_by FOREIGN KEY (deleted_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT uq_students_usi UNIQUE (usi),
    CONSTRAINT chk_usi_length CHECK (usi IS NULL OR length(usi) = 10),
    CONSTRAINT chk_usi_pattern CHECK (usi IS NULL OR usi ~* '^[2-9A-HJ-NP-Z]{10}$'),
    CONSTRAINT chk_state_student_num_len CHECK (state_allocated_student_number IS NULL OR length(state_allocated_student_number) BETWEEN 5 AND 20),
    CONSTRAINT chk_avetmiss_indigenous CHECK (indigenous_status_id IN ('1', '2', '3', '4', '9', '@')),
    CONSTRAINT chk_disability_flag CHECK (disability_flag IN ('Y', 'N')),
    CONSTRAINT chk_prior_ed_flag CHECK (prior_educational_achievement_flag IN ('Y', 'N')),
    CONSTRAINT chk_english_proficiency CHECK (english_proficiency_id IN ('1', '2', '3', '4', '@')),
    CONSTRAINT chk_at_school_flag CHECK (at_school_flag IN ('Y', 'N'))
);

-- NEW v11: sector tracks whether this teacher delivers VET, HE, or both.
-- default_max_hours_per_year is fully configurable per teacher; the 800.00 default
-- is the standard VET industry contract figure — change it for HE, part-time, or
-- other employment arrangements.
-- max_hours_per_period: when set, activates per-period (semester/trimester/block)
-- hour tracking via teacher_period_allocations in addition to the annual balance.
CREATE TABLE IF NOT EXISTS public.teachers (
    id bigint NOT NULL,                          -- shared PK: this IS people.id
    faculty_id bigint NULL,
    teacher_number varchar(20) NOT NULL,
    teacher_email varchar(100) NOT NULL,
    teacher_phone varchar(15) NULL,
    employment_status public.employment_type NOT NULL DEFAULT 'Casual',
    fte numeric(3,2) NOT NULL DEFAULT 0.00,            -- Full-Time Equivalent: 0=Casual, 1=Full-Time, 0<x<1=Part-Time
    sector public.teacher_sector NOT NULL DEFAULT 'VET',
    default_max_hours_per_year numeric(6,2) NOT NULL DEFAULT 800.00,
    max_hours_per_period numeric(6,2) NULL,       -- NULL = use annual cap only; set for semester/block contracts
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_teachers_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_teachers_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL,
    CONSTRAINT uq_teachers_number UNIQUE (teacher_number),
    CONSTRAINT uq_teachers_email UNIQUE (teacher_email),
    CONSTRAINT chk_teacher_max_hours CHECK (default_max_hours_per_year > 0),
    CONSTRAINT chk_teacher_period_hours CHECK (max_hours_per_period IS NULL OR max_hours_per_period > 0),
    CONSTRAINT chk_teacher_fte CHECK (
        (employment_status = 'Casual'    AND fte = 0.00) OR
        (employment_status = 'Full-Time' AND fte = 1.00) OR
        (employment_status = 'Part-Time' AND fte > 0.00 AND fte < 1.00)
    )
);

-- Maintained per-year cache of teaching hours, sourced from sessions.
-- allocated_max_hours is seeded from teachers.default_max_hours_per_year at row
-- creation and can be overridden per-year without touching the teacher record.
CREATE TABLE IF NOT EXISTS public.teacher_yearly_balances (
    id bigserial NOT NULL,
    teacher_id bigint NOT NULL,
    calendar_year smallint NOT NULL,
    booked_hours numeric(7,2) NOT NULL DEFAULT 0.00,    -- maintained by triggers off class_sessions
    allocated_max_hours numeric(6,2) NOT NULL DEFAULT 800.00,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_balances_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers (id) ON DELETE CASCADE,
    CONSTRAINT uq_teacher_year UNIQUE (teacher_id, calendar_year),
    CONSTRAINT chk_balance_nonneg CHECK (booked_hours >= 0),
    CONSTRAINT chk_balance_cap CHECK (booked_hours <= allocated_max_hours)  -- hard cap backstop
);

-- NEW v11: per-period (semester/trimester/block) hour allocations for HE and
-- DUAL-sector teachers. Auto-created on first session booking when the teacher has
-- max_hours_per_period set; also supports explicit pre-allocation by admins.
CREATE TABLE IF NOT EXISTS public.teacher_period_allocations (
    id bigserial NOT NULL,
    teacher_id bigint NOT NULL,
    academic_period_id bigint NOT NULL,
    allocated_hours numeric(6,2) NOT NULL,
    booked_hours numeric(7,2) NOT NULL DEFAULT 0.00,
    notes text NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_teacher_period UNIQUE (teacher_id, academic_period_id),
    CONSTRAINT fk_tpa_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE,
    CONSTRAINT fk_tpa_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE RESTRICT,
    CONSTRAINT chk_tpa_allocated CHECK (allocated_hours > 0),
    CONSTRAINT chk_tpa_nonneg CHECK (booked_hours >= 0),
    CONSTRAINT chk_tpa_cap CHECK (booked_hours <= allocated_hours)  -- hard per-period backstop
);

-- Days of the week a teacher is available to work.
-- day_of_week: 0 = Monday … 6 = Sunday (ISO weekday, 0-based).
CREATE TABLE IF NOT EXISTS public.teacher_availability (
    id bigserial NOT NULL,
    teacher_id bigint NOT NULL,
    day_of_week smallint NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_teacher_avail FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE,
    CONSTRAINT uq_teacher_avail_day UNIQUE (teacher_id, day_of_week),
    CONSTRAINT chk_teacher_avail_day CHECK (day_of_week BETWEEN 0 AND 6)
);

CREATE TABLE IF NOT EXISTS public.staff (
    id bigint NOT NULL,                          -- shared PK: this IS people.id
    faculty_id bigint NULL,
    staff_number varchar(20) NOT NULL,
    staff_email varchar(100) NOT NULL,
    staff_phone varchar(15) NULL,
    employment_status public.employment_type NOT NULL DEFAULT 'Full-Time',
    fte numeric(3,2) NOT NULL DEFAULT 1.00,            -- Full-Time Equivalent: 0=Casual, 1=Full-Time, 0<x<1=Part-Time
    PRIMARY KEY (id),
    CONSTRAINT fk_staff_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_staff_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL,
    CONSTRAINT uq_staff_number UNIQUE (staff_number),
    CONSTRAINT uq_staff_email UNIQUE (staff_email),
    CONSTRAINT chk_staff_fte CHECK (
        (employment_status = 'Casual'    AND fte = 0.00) OR
        (employment_status = 'Full-Time' AND fte = 1.00) OR
        (employment_status = 'Part-Time' AND fte > 0.00 AND fte < 1.00)
    )
);

-- Days of the week a staff member is available to work.
-- day_of_week: 0 = Monday … 6 = Sunday (ISO weekday, 0-based).
CREATE TABLE IF NOT EXISTS public.staff_availability (
    id bigserial NOT NULL,
    staff_id bigint NOT NULL,
    day_of_week smallint NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_staff_avail FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE,
    CONSTRAINT uq_staff_avail_day UNIQUE (staff_id, day_of_week),
    CONSTRAINT chk_staff_avail_day CHECK (day_of_week BETWEEN 0 AND 6)
);

CREATE TABLE IF NOT EXISTS public.student_guardians (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    title varchar(10) NULL,
    first_name varchar(40) NOT NULL,
    family_name varchar(40) NOT NULL,
    relationship varchar(50) NOT NULL,
    is_primary boolean NOT NULL DEFAULT true,
    phone_mobile varchar(15) NULL,
    phone_home varchar(15) NULL,
    email varchar(100) NULL,
    receive_comms boolean NOT NULL DEFAULT true,
    is_emergency_contact boolean NOT NULL DEFAULT false,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_guardian_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS public.student_disabilities (
    student_id bigint NOT NULL,
    disability_id varchar(2) NOT NULL,
    PRIMARY KEY (student_id, disability_id),
    CONSTRAINT fk_stud_dis_student FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE CASCADE,
    CONSTRAINT fk_stud_dis_type FOREIGN KEY (disability_id) REFERENCES public.disability_types (disability_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS public.student_prior_achievements (
    student_id bigint NOT NULL,
    achievement_id varchar(3) NOT NULL,
    PRIMARY KEY (student_id, achievement_id),
    CONSTRAINT fk_spa_student FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE CASCADE,
    CONSTRAINT fk_spa_achievement FOREIGN KEY (achievement_id) REFERENCES public.prior_educational_achievements (achievement_id) ON DELETE RESTRICT
);

-- =========================================================================
-- 3. CURRICULUM
-- =========================================================================

-- NEW v11: he_flag distinguishes HE qualifications from VET programs.
-- credit_points holds the total credit point value of the qualification
-- (e.g. 192 cp for a Bachelor, 48 cp for a Graduate Certificate).
-- A program can have both vet_flag and he_flag true for dual-sector offerings.
CREATE TABLE IF NOT EXISTS public.programs (
    id bigserial NOT NULL,
    faculty_id bigint NOT NULL,
    program_code varchar(10) NOT NULL,
    program_name varchar(100) NOT NULL,
    program_recognition_id varchar(2) NOT NULL,
    level_of_education varchar(3) NOT NULL,
    field_of_education varchar(4) NOT NULL,
    anzsco_code varchar(6) NULL,
    anzsic_code varchar(4) NULL,
    nominal_hours integer NOT NULL CHECK (nominal_hours >= 0),
    vet_flag boolean NOT NULL DEFAULT true,
    he_flag boolean NOT NULL DEFAULT false,
    credit_points integer NULL,                   -- total qualification credit points (HE use)
    aqf_level smallint NULL,                      -- AQF level 1–10 (1=Cert I … 10=Doctoral)
    program_type varchar(20) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_programs_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE RESTRICT,
    CONSTRAINT uq_programs_code UNIQUE (program_code),
    CONSTRAINT chk_program_credit_points CHECK (credit_points IS NULL OR credit_points > 0),
    CONSTRAINT chk_program_aqf_level CHECK (aqf_level IS NULL OR aqf_level BETWEEN 1 AND 10),
    CONSTRAINT chk_program_sector CHECK (vet_flag = true OR he_flag = true),
    CONSTRAINT chk_program_type CHECK (program_type IS NULL OR program_type IN (
        'Qualification','Skill Set','Course in a Package','Statement of Attainment','Accredited Course'
    ))
);

-- NEW v11: credit_points is the HE credit point value of one unit/subject
-- (e.g. 6 cp, 12 cp, 24 cp). NULL for VET-only units.
CREATE TABLE IF NOT EXISTS public.subjects (
    id bigserial NOT NULL,
    subject_code varchar(30) NOT NULL,
    subject_name varchar(100) NOT NULL,
    module_flag varchar(1) NOT NULL DEFAULT 'N',
    field_of_education varchar(6) NOT NULL,
    nominal_hours integer CHECK (nominal_hours > 0),
    vet_flag boolean NOT NULL DEFAULT true,
    credit_points integer NULL,                   -- HE credit point value of this unit
    PRIMARY KEY (id),
    CONSTRAINT uq_subjects_code UNIQUE (subject_code),
    CONSTRAINT chk_module_flag CHECK (module_flag IN ('Y', 'N')),
    CONSTRAINT chk_subject_credit_points CHECK (credit_points IS NULL OR credit_points > 0)
);

CREATE TABLE IF NOT EXISTS public.subject_programs (
    subject_id  bigint       NOT NULL,
    program_id  bigint       NOT NULL,
    is_core     boolean      NOT NULL DEFAULT false,
    group_code  varchar(20)  NULL,
    group_title varchar(100) NULL,
    PRIMARY KEY (subject_id, program_id),
    CONSTRAINT fk_sp_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE CASCADE,
    CONSTRAINT fk_sp_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE CASCADE
);

-- =========================================================================
-- 4. THIRD PARTY NETWORKS, WORKPLACES & RTO INFRASTRUCTURE
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.aasn_providers (
    id bigserial NOT NULL,
    provider_name varchar(100) NOT NULL,
    national_identifier varchar(10) NULL,
    contact_email varchar(100) NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_aasn_name UNIQUE (provider_name)
);
INSERT INTO public.aasn_providers (provider_name) VALUES
('MEGT'), ('MAS National'), ('Sarina Russo Job Access'), ('Busy At Work')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS public.employers (
    id bigserial NOT NULL,
    legal_name varchar(100) NOT NULL,
    trading_name varchar(100) NULL,
    abn varchar(11) NOT NULL,
    contact_person varchar(100) NULL,
    contact_phone varchar(15) NULL,
    contact_email varchar(100) NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_employer_abn UNIQUE (abn),
    CONSTRAINT chk_abn_length CHECK (abn ~ '^[0-9]{11}$')
);

CREATE TABLE IF NOT EXISTS public.employer_workplaces (
    id bigserial NOT NULL,
    employer_id bigint NOT NULL,
    workplace_name varchar(100) NOT NULL,
    address text NOT NULL,
    suburb varchar(50) NOT NULL,
    state_code varchar(3) NOT NULL,
    postcode varchar(4) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_workplace_employer FOREIGN KEY (employer_id) REFERENCES public.employers(id) ON DELETE CASCADE,
    CONSTRAINT fk_workplace_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT chk_workplace_postcode CHECK (postcode ~ '^[0-9]{4}$')
);

CREATE TABLE IF NOT EXISTS public.training_orgs (
    id bigserial NOT NULL,
    training_org_id varchar(30) NOT NULL,
    training_org_name varchar(100) NOT NULL,
    training_org_type varchar(2) NOT NULL,
    address_first_line varchar(50) NOT NULL,
    address_second_line varchar(50) NULL,
    suburb varchar(50) NOT NULL,
    state_code varchar(3) NOT NULL,
    postcode varchar(4) NOT NULL,
    logo_url varchar(2048) NULL,
    contact_name varchar(100) NULL,
    telephone varchar(20) NULL,
    facsimile varchar(20) NULL,
    email varchar(100) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_org_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_training_org_code UNIQUE (training_org_id)
);

CREATE TABLE IF NOT EXISTS public.delivery_locations (
    id              bigserial    NOT NULL,
    training_org_id bigint       NOT NULL,
    delivery_loc_id varchar(30)  NOT NULL,
    name            varchar(100) NOT NULL,
    is_virtual      boolean      NOT NULL DEFAULT false,
    address         text         NULL,
    suburb          varchar(50)  NULL,
    state_code      varchar(3)   NULL,
    postcode        varchar(4)   NULL,
    postcode_override varchar(4) NULL,
    country_id      varchar(4)   NOT NULL DEFAULT '1101',
    latitude        numeric(9,6) NULL,
    longitude       numeric(9,6) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_loc_state          FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_delivery_loc_per_org UNIQUE (training_org_id, delivery_loc_id),
    CONSTRAINT chk_loc_physical_fields CHECK (
        is_virtual = true OR
        (address IS NOT NULL AND suburb IS NOT NULL AND state_code IS NOT NULL AND postcode IS NOT NULL)
    )
);

-- A person's ranked delivery location preferences. Equal ranks are allowed
-- (e.g. two locations both ranked 2). One row per person+location pair.
CREATE TABLE IF NOT EXISTS public.person_location_preferences (
    id                   bigserial NOT NULL,
    person_id            bigint    NOT NULL,
    delivery_location_id bigint    NOT NULL,
    preference_rank      smallint  NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_person_location_pref   UNIQUE (person_id, delivery_location_id),
    CONSTRAINT fk_plp_person             FOREIGN KEY (person_id)            REFERENCES public.people(id)             ON DELETE CASCADE,
    CONSTRAINT fk_plp_delivery_location  FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE CASCADE,
    CONSTRAINT chk_plp_rank              CHECK (preference_rank >= 1)
);

CREATE TABLE IF NOT EXISTS public.buildings (
    id                   bigserial    NOT NULL,
    delivery_location_id bigint       NOT NULL,
    building_name        varchar(50)  NOT NULL,
    address              text         NULL,
    suburb               varchar(50)  NULL,
    state_code           varchar(3)   NULL,
    postcode             varchar(4)   NULL,
    latitude             numeric(9,6) NULL,
    longitude            numeric(9,6) NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_building_per_campus UNIQUE (delivery_location_id, building_name)
);

CREATE TABLE IF NOT EXISTS public.rooms (
    id              bigserial    NOT NULL,
    building_id     bigint       NOT NULL,
    room_name       varchar(50)  NOT NULL,
    capacity        integer      NOT NULL,
    room_type       varchar(30)  NOT NULL DEFAULT 'Classroom',
    is_active       boolean      NOT NULL DEFAULT true,
    is_computer_lab boolean      NOT NULL DEFAULT false,
    PRIMARY KEY (id),
    CONSTRAINT uq_room_per_building UNIQUE (building_id, room_name),
    CONSTRAINT chk_rooms_capacity CHECK (capacity > 0)
);

-- Hardware profile for computer lab rooms (one row per lab room).
CREATE TABLE IF NOT EXISTS public.room_computer_lab_specs (
    id              bigserial NOT NULL,
    room_id         bigint    NOT NULL,
    workstations    smallint  NULL,
    ram_gb          smallint  NULL,
    has_microphone  boolean   NOT NULL DEFAULT false,
    has_webcam      boolean   NOT NULL DEFAULT false,
    notes           text      NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_lab_specs_room   UNIQUE (room_id),
    CONSTRAINT fk_lab_specs_room   FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE
);

-- Software installed in a computer lab room.
CREATE TABLE IF NOT EXISTS public.room_lab_software (
    id            bigserial    NOT NULL,
    room_id       bigint       NOT NULL,
    software_name varchar(150) NOT NULL,
    version       varchar(50)  NULL,
    licence_type  varchar(50)  NULL,
    notes         text         NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_lab_software_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE
);

-- Issues reported for any room (faults, maintenance, AV problems, etc.).
CREATE TABLE IF NOT EXISTS public.room_issues (
    id          bigserial    NOT NULL,
    room_id     bigint       NOT NULL,
    title       varchar(200) NOT NULL,
    description text         NULL,
    status      varchar(20)  NOT NULL DEFAULT 'Open',
    reported_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    resolved_at timestamp with time zone NULL,
    notes       text         NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_room_issue_room    FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE,
    CONSTRAINT chk_room_issue_status CHECK (status IN ('Open', 'Investigating', 'Resolved'))
);

-- =========================================================================
-- 4.5. INTAKES & COHORTS
-- A program may be delivered as multiple scheduled intakes (e.g. once per
-- term, once per year). Each intake may have one or more groups — sub-
-- cohorts that attend classes together throughout the program.  Students
-- are enrolled into a specific intake group; classes are linked to an
-- intake group so the register shows the right cohort.
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.program_intakes (
    id                       bigserial      NOT NULL,
    program_id               bigint         NOT NULL,
    intake_code              varchar(30)    NOT NULL,           -- e.g. 'ICT30120-2025-T1-FT'
    intake_name              varchar(150)   NOT NULL,           -- e.g. 'Cert III IT — 2025 Term 1 Full-time'
    start_academic_period_id bigint         NOT NULL,           -- academic period this intake begins
    delivery_location_id     bigint         NOT NULL,
    faculty_id               bigint         NULL,
    study_mode               varchar(10)    NOT NULL DEFAULT 'Full-Time',
    duration_periods         smallint       NOT NULL,           -- number of academic periods to complete
    duration_years           numeric(3,1)   NULL,               -- calendar duration in years (e.g. 1.0, 1.5, 2.0)
    graded_assessment        boolean        NOT NULL DEFAULT false,
    enrolment_open_date      date           NULL,
    enrolment_close_date     date           NULL,
    status                   varchar(20)    NOT NULL DEFAULT 'Planned',
    notes                    text           NULL,
    created_at               timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at               timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_intake_code            UNIQUE (intake_code),
    CONSTRAINT fk_intake_program         FOREIGN KEY (program_id)               REFERENCES public.programs(id)            ON DELETE RESTRICT,
    CONSTRAINT fk_intake_period          FOREIGN KEY (start_academic_period_id) REFERENCES public.academic_periods(id)    ON DELETE RESTRICT,
    CONSTRAINT fk_intake_location        FOREIGN KEY (delivery_location_id)     REFERENCES public.delivery_locations(id)  ON DELETE RESTRICT,
    CONSTRAINT fk_intake_faculty         FOREIGN KEY (faculty_id)               REFERENCES public.faculties(id)           ON DELETE SET NULL,
    CONSTRAINT chk_intake_study_mode     CHECK (study_mode IN ('Full-Time', 'Part-Time')),
    CONSTRAINT chk_intake_duration       CHECK (duration_periods > 0),
    CONSTRAINT chk_intake_duration_years CHECK (duration_years IS NULL OR duration_years > 0),
    CONSTRAINT chk_intake_status         CHECK (status IN ('Planned', 'Active', 'Closed', 'Cancelled')),
    CONSTRAINT chk_intake_enrolment_dates CHECK (
        enrolment_open_date IS NULL OR enrolment_close_date IS NULL OR
        enrolment_close_date >= enrolment_open_date
    )
);

-- Sub-cohorts within an intake that attend classes together.
-- e.g. Intake 2025-T1 may have Group A (Mon/Wed) and Group B (Tue/Thu).
CREATE TABLE IF NOT EXISTS public.intake_groups (
    id         bigserial    NOT NULL,
    intake_id  bigint       NOT NULL,
    group_code varchar(20)  NOT NULL,           -- e.g. 'A', 'B', 'MON'
    group_name varchar(100) NOT NULL,           -- e.g. 'Group A', 'Monday Group'
    capacity   integer      NULL,
    notes      text         NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_group_per_intake UNIQUE (intake_id, group_code),
    CONSTRAINT fk_ig_intake        FOREIGN KEY (intake_id) REFERENCES public.program_intakes(id) ON DELETE CASCADE,
    CONSTRAINT chk_ig_capacity     CHECK (capacity IS NULL OR capacity > 0)
);

-- =========================================================================
-- 5. PROGRESSION & ENROLMENT
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.student_course_enrollments (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    program_id bigint NOT NULL,
    intake_group_id bigint NULL,                -- which intake group this student belongs to
    enrollment_status varchar(20) NOT NULL DEFAULT 'Active',
    commencement_date date NOT NULL,
    commencing_program_id varchar(1) NOT NULL DEFAULT '3',
    completion_date date NULL,
    funding_state_code varchar(3) NOT NULL DEFAULT 'VIC',
    training_contract_id varchar(20) NULL,
    client_apprenticeship_id varchar(20) NULL,
    deleted_at timestamp with time zone NULL,
    deleted_by bigint NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_enrollment_state    FOREIGN KEY (funding_state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT fk_sce_deleted_by      FOREIGN KEY (deleted_by)         REFERENCES public.app_users(id)    ON DELETE SET NULL,
    CONSTRAINT fk_sce_intake_group    FOREIGN KEY (intake_group_id)    REFERENCES public.intake_groups(id) ON DELETE SET NULL,
    CONSTRAINT chk_enrollment_status  CHECK (enrollment_status IN ('Active', 'Deferred', 'Suspended', 'Cancelled', 'Completed')),
    CONSTRAINT chk_commencing_program_id CHECK (commencing_program_id IN ('3', '4', '8'))
);
-- NOTE: state-specific funding attributes (Skills First, Smart & Skilled, etc.)
-- moved out of this national table into state_funding_details below.

CREATE UNIQUE INDEX IF NOT EXISTS idx_uq_active_course_enrollment
ON public.student_course_enrollments(student_id, program_id)
WHERE (enrollment_status IN ('Active', 'Deferred', 'Suspended'));

-- state-specific funding attributes split off the national enrolment table
CREATE TABLE IF NOT EXISTS public.state_funding_details (
    id bigserial NOT NULL,
    student_course_enrollment_id bigint NOT NULL,
    state_code varchar(3) NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (id),
    CONSTRAINT fk_sfd_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT fk_sfd_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_sfd_per_enrolment_state UNIQUE (student_course_enrollment_id, state_code)
);

CREATE TABLE IF NOT EXISTS public.client_subject_enrolments (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    student_course_enrollment_id bigint NULL,           -- NULLABLE: supports standalone units (NAT00120 blank program)
    subject_id bigint NOT NULL,
    delivery_location_id bigint NULL,
    activity_start_date date NOT NULL,
    activity_end_date date NOT NULL,
    delivery_mode_id varchar(3) NOT NULL DEFAULT 'YNN',
    predominant_delivery_mode varchar(1) NOT NULL DEFAULT 'I',
    vet_in_schools_flag varchar(1) NOT NULL DEFAULT 'N',
    commencing_program_id varchar(1) NOT NULL DEFAULT '8',
    scheduled_hours numeric(5,2) NOT NULL,
    funding_source_national varchar(2) NOT NULL,
    outcome_id_national varchar(2) NOT NULL DEFAULT '70',
    specific_funding_id varchar(2) NULL,
    outcome_id_training_org varchar(10) NULL,
    funding_source_state varchar(3) NULL,
    client_tuition_fee numeric(8,2) NULL,
    fee_exemption_type_id varchar(2) NULL,
    purchasing_contract_id varchar(30) NULL,
    purchasing_contract_schedule_id varchar(30) NULL,
    hours_attended numeric(5,2) NULL,
    associated_course_id varchar(10) NULL,
    grade varchar(20) NULL,
    mark numeric(5,2) NULL,
    result varchar(3) NULL,                                    -- teacher assessment: SC | NS
    result_is_published boolean NOT NULL DEFAULT false,        -- true once pushed to the official record
    finalised_date date NULL,
    result_status varchar(20) NOT NULL DEFAULT 'In Progress',
    result_finalised_by bigint NULL,
    result_finalised_at timestamp with time zone NULL,
    result_amended_at timestamp with time zone NULL,
    result_amendment_reason text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_cse_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE RESTRICT,
    CONSTRAINT fk_cse_delivery_loc FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE RESTRICT,
    CONSTRAINT fk_cse_finalised_by FOREIGN KEY (result_finalised_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT uq_cse_unit_per_enrollment UNIQUE (student_course_enrollment_id, subject_id),
    CONSTRAINT chk_activity_dates CHECK (activity_end_date >= activity_start_date),
    CONSTRAINT chk_predominant_mode CHECK (predominant_delivery_mode IN ('I', 'E', 'W', 'N')),
    CONSTRAINT chk_delivery_mode_len CHECK (length(delivery_mode_id) = 3),
    CONSTRAINT chk_vet_in_schools_flag CHECK (vet_in_schools_flag IN ('Y', 'N')),
    CONSTRAINT chk_tuition_fee_positive CHECK (client_tuition_fee >= 0),
    CONSTRAINT chk_hours_attended_positive CHECK (hours_attended >= 0),
    CONSTRAINT chk_mark_range CHECK (mark IS NULL OR (mark BETWEEN 0.00 AND 100.00)),
    CONSTRAINT chk_result_workflow CHECK (result_status IN ('In Progress', 'Draft', 'Under Review', 'Finalised', 'Appealed', 'Amended'))
);
-- For standalone units (no parent program enrolment) uq_cse_unit_per_enrollment is not
-- enforced (NULLs are distinct); this partial index dedupes them per student instead.
CREATE UNIQUE INDEX IF NOT EXISTS idx_uq_standalone_unit
ON public.client_subject_enrolments(student_id, subject_id, activity_start_date)
WHERE (student_course_enrollment_id IS NULL);

CREATE INDEX IF NOT EXISTS idx_enrolments_contract ON public.client_subject_enrolments(purchasing_contract_id);

-- =========================================================================
-- 6. EXTENSION DOMAINS (Apprenticeships, Compliance Plans, Higher Ed)
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.apprenticeship_details (
    student_course_enrollment_id bigint NOT NULL,
    employer_id bigint NOT NULL,
    workplace_id bigint NOT NULL,
    aasn_provider_id bigint NULL,
    delta_registration_number varchar(30) NULL,
    tyims_number varchar(30) NULL,
    training_plan_drafted_date date NULL,
    training_plan_employer_signed_date date NULL,
    training_plan_student_signed_date date NULL,
    training_plan_rto_signed_date date NULL,
    training_plan_fully_executed_date date NULL,
    is_school_based_apprenticeship boolean NOT NULL DEFAULT false,
    PRIMARY KEY (student_course_enrollment_id),
    CONSTRAINT fk_app_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT fk_app_employer FOREIGN KEY (employer_id) REFERENCES public.employers(id) ON DELETE RESTRICT,
    CONSTRAINT fk_app_workplace FOREIGN KEY (workplace_id) REFERENCES public.employer_workplaces(id) ON DELETE RESTRICT,
    CONSTRAINT fk_app_aasn FOREIGN KEY (aasn_provider_id) REFERENCES public.aasn_providers(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS public.traineeship_details (
    student_course_enrollment_id bigint NOT NULL,
    worker_classification public.trainee_worker_type NOT NULL DEFAULT 'New Worker',
    probation_start_date date NOT NULL,
    probation_end_date date NOT NULL,
    probation_cleared boolean NOT NULL DEFAULT false,
    has_approved_extension boolean NOT NULL DEFAULT false,
    extension_approved_date date NULL,
    extension_revised_end_date date NULL,
    sta_extension_reference varchar(50) NULL,
    PRIMARY KEY (student_course_enrollment_id),
    CONSTRAINT fk_trainee_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT chk_probation_dates CHECK (probation_end_date >= probation_start_date),
    CONSTRAINT chk_extension_logic CHECK (
        (has_approved_extension = false) OR
        (has_approved_extension = true AND extension_approved_date IS NOT NULL AND extension_revised_end_date IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS public.training_plans (
    id bigserial NOT NULL,
    student_course_enrollment_id bigint NOT NULL,
    drafted_date date NULL,
    student_signed_date date NULL,
    rto_signed_date date NULL,
    fully_executed_date date NULL,
    review_date date NULL,
    delivery_strategy text NULL,
    notes text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_training_plan_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT uq_training_plan_enrollment UNIQUE (student_course_enrollment_id)
);

CREATE TABLE IF NOT EXISTS public.learning_access_plans (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    student_course_enrollment_id bigint NULL,
    plan_date date NOT NULL,
    review_date date NULL,
    disability_type_codes varchar(2)[] NOT NULL,
    adjustments_required text NOT NULL,
    resources_provided text NULL,
    assessor_id bigint NOT NULL,
    student_consent boolean NOT NULL DEFAULT false,
    status varchar(20) NOT NULL DEFAULT 'Active',
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_lap_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_lap_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE SET NULL,
    CONSTRAINT chk_lap_status CHECK (status IN ('Draft', 'Active', 'Under Review', 'Closed'))
);

CREATE TABLE IF NOT EXISTS public.vet_student_loans (
    id bigserial NOT NULL,
    student_course_enrollment_id bigint NOT NULL,
    loan_type varchar(20) NOT NULL,
    census_date date NOT NULL,
    loan_amount numeric(10,2) NOT NULL,
    re_credit_flag boolean NOT NULL DEFAULT false,
    re_credit_date date NULL,
    re_credit_reason text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vsl_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT uq_vsl_enrol_census UNIQUE (student_course_enrollment_id, census_date),
    CONSTRAINT chk_vsl_type CHECK (loan_type IN ('VSL', 'VET-FEE-HELP')),
    CONSTRAINT chk_vsl_amount CHECK (loan_amount >= 0)
);

-- NEW v11: academic_period_id links this HE enrolment to the specific
-- semester/trimester/block it applies to (e.g. "Semester 1 2026").
-- credit_points_enrolled is the load for this period (may be less than full load
-- for part-time students, leave of absence, etc.).
CREATE TABLE IF NOT EXISTS public.he_enrolment_details (
    student_course_enrollment_id bigint NOT NULL,
    academic_period_id bigint NULL,
    eftsl numeric(5,4) NOT NULL,
    census_date date NOT NULL,
    hecs_help_eligible boolean NOT NULL DEFAULT false,
    fee_type varchar(20) NULL,
    study_load_category varchar(20) NULL,
    mode_of_attendance varchar(30) NULL,
    basis_for_admission varchar(10) NULL,
    credit_points_enrolled smallint NULL,         -- credit point load for this enrolment period
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (student_course_enrollment_id),
    CONSTRAINT fk_he_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT fk_he_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE SET NULL,
    CONSTRAINT chk_he_fee_type CHECK (fee_type IN ('HECS-HELP', 'FEE-HELP', 'DOMESTIC-FULL', 'INTERNATIONAL', 'EXEMPT')),
    CONSTRAINT chk_he_load CHECK (study_load_category IN ('Full-Time', 'Part-Time', 'Less Than Half-Time')),
    CONSTRAINT chk_he_mode CHECK (mode_of_attendance IN ('Internal', 'External', 'Multi-Modal')),
    CONSTRAINT chk_he_credit_points CHECK (credit_points_enrolled IS NULL OR credit_points_enrolled > 0)
);

CREATE TABLE IF NOT EXISTS public.enrollment_credit_claims (
    id bigserial NOT NULL,
    student_course_enrollment_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    claim_type public.credit_type NOT NULL,
    granted_date date NOT NULL,
    hours_deducted numeric(5,2) NOT NULL CHECK (hours_deducted > 0),
    tuition_fee_adjustment numeric(8,2) NOT NULL DEFAULT 0.00,
    evidence_document_reference varchar(255) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_enrollment_subject_credit UNIQUE (student_course_enrollment_id, subject_id),
    CONSTRAINT fk_credit_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT fk_credit_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE RESTRICT
);

-- =========================================================================
-- 7. TIMETABLING: RECURRING TEMPLATES (slots) & CONCRETE OCCURRENCES (sessions)
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.classes (
    id                   bigserial    NOT NULL,
    class_code           varchar(80)  NOT NULL,
    group_code           varchar(20)  NULL,      -- legacy free-text; prefer intake_group_id
    intake_group_id      bigint       NULL,       -- the cohort group this class belongs to
    academic_period_id   bigint       NOT NULL,
    delivery_location_id bigint       NOT NULL,
    enrolment_cap        integer      NULL,
    created_at           timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at           timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_class_code         UNIQUE (class_code),
    CONSTRAINT fk_class_period       FOREIGN KEY (academic_period_id)   REFERENCES public.academic_periods(id),
    CONSTRAINT fk_class_location     FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id),
    CONSTRAINT fk_class_intake_group FOREIGN KEY (intake_group_id)      REFERENCES public.intake_groups(id) ON DELETE SET NULL,
    CONSTRAINT chk_class_cap         CHECK (enrolment_cap > 0)
);

-- The set of subjects/units a class delivers.
CREATE TABLE IF NOT EXISTS public.class_subjects (
    class_id bigint NOT NULL,
    subject_id bigint NOT NULL,
    subject_label varchar(100) NOT NULL,
    PRIMARY KEY (class_id, subject_id)
);

CREATE TABLE IF NOT EXISTS public.class_enrollments (
    id bigserial NOT NULL,
    class_id bigint NOT NULL,
    client_subject_enrolment_id bigint NOT NULL,
    enrolled_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_class_enrolment_map UNIQUE (class_id, client_subject_enrolment_id)
);

CREATE TABLE IF NOT EXISTS public.class_slots (
    id bigserial NOT NULL,
    class_id bigint NOT NULL,
    academic_period_id bigint NOT NULL,             -- denormalised from class, auto-set by trigger
    room_id bigint NULL,
    teacher_id bigint NOT NULL,
    day_of_week smallint NOT NULL,
    start_time time WITHOUT TIME ZONE NOT NULL,
    end_time time WITHOUT TIME ZONE NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT chk_slots_day CHECK (day_of_week BETWEEN 1 AND 7),
    CONSTRAINT chk_slots_times CHECK (end_time > start_time),
    -- Period-scoped: same weekday/time in a DIFFERENT term is no longer a false clash.
    CONSTRAINT no_teacher_double_booking EXCLUDE USING gist (
        academic_period_id WITH =,
        teacher_id WITH =,
        day_of_week WITH =,
        timerange(start_time, end_time) WITH &&
    ),
    CONSTRAINT no_room_double_booking EXCLUDE USING gist (
        academic_period_id WITH =,
        room_id WITH =,
        day_of_week WITH =,
        timerange(start_time, end_time) WITH &&
    ) WHERE (room_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS public.class_sessions (
    id bigserial NOT NULL,
    class_id bigint NOT NULL,
    session_date date NOT NULL,
    start_time time WITHOUT TIME ZONE NOT NULL,
    end_time time WITHOUT TIME ZONE NOT NULL,
    room_id bigint NULL,
    session_type varchar(20) NOT NULL DEFAULT 'Scheduled',
    notes text NULL,
    cancelled boolean NOT NULL DEFAULT false,
    cancel_reason varchar(255) NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_session_class FOREIGN KEY (class_id) REFERENCES public.classes(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE SET NULL,
    CONSTRAINT uq_session_natural UNIQUE (class_id, session_date, start_time),  -- enables idempotent generation
    CONSTRAINT chk_session_times CHECK (end_time > start_time),
    CONSTRAINT chk_session_type CHECK (session_type IN ('Scheduled', 'Replacement', 'Assessment', 'Online', 'Other')),
    CONSTRAINT no_room_session_double_booking EXCLUDE USING gist (
        room_id WITH =,
        session_date WITH =,
        timerange(start_time, end_time) WITH &&
    ) WHERE (room_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS public.session_teachers (
    session_id bigint NOT NULL,
    teacher_id bigint NOT NULL,
    role varchar(30) NOT NULL DEFAULT 'Lead',
    PRIMARY KEY (session_id, teacher_id),
    CONSTRAINT fk_se_teach_session FOREIGN KEY (session_id) REFERENCES public.class_sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_se_teach_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT,
    CONSTRAINT chk_session_teacher_role CHECK (role IN ('Lead', 'Support', 'Guest', 'Assessor'))
);

CREATE TABLE IF NOT EXISTS public.session_attendance (
    id bigserial NOT NULL,
    session_id bigint NOT NULL,
    student_id bigint NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'Present',
    minutes_attended integer NULL,
    notes text NULL,
    units_nominated       smallint               NOT NULL DEFAULT 0,
    arrived_at            time WITHOUT TIME ZONE NULL,
    departed_at           time WITHOUT TIME ZONE NULL,
    break_minutes         smallint               NOT NULL DEFAULT 0,
    absence_reason        varchar(100)           NULL,
    absence_is_acceptable boolean                NOT NULL DEFAULT false,
    has_childcare         boolean                NOT NULL DEFAULT false,
    is_note_private       boolean                NOT NULL DEFAULT false,
    recorded_by bigint NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_attendance_session FOREIGN KEY (session_id) REFERENCES public.class_sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_recorder FOREIGN KEY (recorded_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT uq_attendance_student_per_session UNIQUE (session_id, student_id),
    CONSTRAINT chk_attendance_status CHECK (status IN ('Present','Absent-Notified','Absent-Unnotified','Online','Excused','Not-Applicable')),
    CONSTRAINT chk_minutes_attended CHECK (minutes_attended >= 0),
    CONSTRAINT chk_units_nominated CHECK (units_nominated >= 0),
    CONSTRAINT chk_break_minutes CHECK (break_minutes >= 0)
);

CREATE TABLE IF NOT EXISTS public.class_support_staff (
    id bigserial NOT NULL,
    class_id bigint NOT NULL,
    staff_id bigint NOT NULL,
    student_id bigint NULL,
    role varchar(50) NOT NULL DEFAULT 'Support',
    PRIMARY KEY (id),
    CONSTRAINT fk_support_class FOREIGN KEY (class_id) REFERENCES public.classes(id) ON DELETE CASCADE,
    CONSTRAINT fk_support_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE RESTRICT,
    CONSTRAINT fk_support_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT uq_support_scope UNIQUE (class_id, staff_id, student_id),
    CONSTRAINT chk_support_role CHECK (role IN ('Interpreter', 'Aide', 'Note-Taker', 'Counsellor', 'Support', 'Other'))
);
-- Dedupe class-wide (student_id IS NULL) support rows.
CREATE UNIQUE INDEX IF NOT EXISTS idx_uq_support_classwide
ON public.class_support_staff(class_id, staff_id)
WHERE (student_id IS NULL);

CREATE TABLE IF NOT EXISTS public.class_exceptions (
    id bigserial NOT NULL,
    class_id bigint NOT NULL,
    exception_date date NOT NULL,
    reason varchar(255) NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_exception_per_class UNIQUE (class_id, exception_date)
);

-- Holiday DEFINITIONS. A rule is stored once and expanded into concrete dates
-- per year by fn_materialise_holidays(). state_code NULL = applies nationally.
-- Recurrence types:
--   ONCE                 - a single explicit date (fixed_date); never recurs.
--   ANNUAL_FIXED         - same month/day every year (e.g. Christmas 25 Dec).
--   ANNUAL_NTH_DOW       - nth weekday of a month (e.g. King's Birthday = 2nd Mon Jun;
--                          nth = -1 means "last", e.g. last Mon).
--   ANNUAL_EASTER_OFFSET - days from Easter Sunday (Good Friday = -2, Easter Mon = +1).
CREATE TABLE IF NOT EXISTS public.holiday_rules (
    id bigserial PRIMARY KEY,
    holiday_name varchar(100) NOT NULL,
    state_code varchar(3) NULL REFERENCES public.australian_states(state_code) ON DELETE RESTRICT,
    recurrence varchar(20) NOT NULL,
    month smallint NULL,          -- ANNUAL_FIXED / ANNUAL_NTH_DOW
    day smallint NULL,            -- ANNUAL_FIXED
    weekday smallint NULL,        -- ANNUAL_NTH_DOW (ISODOW 1=Mon .. 7=Sun)
    nth smallint NULL,            -- ANNUAL_NTH_DOW (1..5, or -1 for last)
    easter_offset smallint NULL,  -- ANNUAL_EASTER_OFFSET (days from Easter Sunday)
    fixed_date date NULL,         -- ONCE
    observe_substitute boolean NOT NULL DEFAULT false,  -- shift to next weekday if on a weekend
    active_from smallint NULL,    -- first year the rule applies (NULL = always)
    active_to smallint NULL,      -- last year the rule applies (NULL = ongoing)
    notes text NULL,
    CONSTRAINT chk_holiday_recurrence CHECK (recurrence IN ('ONCE','ANNUAL_FIXED','ANNUAL_NTH_DOW','ANNUAL_EASTER_OFFSET')),
    CONSTRAINT chk_holiday_rule_shape CHECK (
        (recurrence = 'ONCE'                 AND fixed_date IS NOT NULL) OR
        (recurrence = 'ANNUAL_FIXED'         AND month BETWEEN 1 AND 12 AND day BETWEEN 1 AND 31) OR
        (recurrence = 'ANNUAL_NTH_DOW'       AND month BETWEEN 1 AND 12 AND weekday BETWEEN 1 AND 7 AND (nth BETWEEN 1 AND 5 OR nth = -1)) OR
        (recurrence = 'ANNUAL_EASTER_OFFSET' AND easter_offset IS NOT NULL)
    )
);

-- Concrete OCCURRENCES. This is what scheduling/session generation reads.
-- Rows either come from expanding a rule (rule_id set) or are hand-entered
-- one-offs (rule_id NULL) for unpredictable dates (day of mourning, AFL GF Friday).
CREATE TABLE IF NOT EXISTS public.holiday_observances (
    id bigserial PRIMARY KEY,
    holiday_date date NOT NULL,
    holiday_name varchar(100) NOT NULL,
    state_code varchar(3) NULL REFERENCES public.australian_states(state_code) ON DELETE RESTRICT,  -- NULL = all states
    rule_id bigint NULL REFERENCES public.holiday_rules(id) ON DELETE CASCADE,
    is_substitute boolean NOT NULL DEFAULT false
);
-- COALESCE sentinel makes the uniqueness work for national (NULL state) rows too.
CREATE UNIQUE INDEX IF NOT EXISTS uq_observance
    ON public.holiday_observances (holiday_date, COALESCE(state_code, '*'), holiday_name);
CREATE INDEX IF NOT EXISTS idx_observance_date ON public.holiday_observances (holiday_date);

-- Seed clearly-national, predictable holidays. State-specific ones
-- (Labour Day, King's Birthday variants, Melbourne Cup, etc.) get their own
-- rules with state_code set.
INSERT INTO public.holiday_rules (holiday_name, state_code, recurrence, month, day, easter_offset, observe_substitute) VALUES
('New Year''s Day', NULL, 'ANNUAL_FIXED', 1, 1, NULL, true),
('Australia Day',   NULL, 'ANNUAL_FIXED', 1, 26, NULL, true),
('Anzac Day',       NULL, 'ANNUAL_FIXED', 4, 25, NULL, false),
('Christmas Day',   NULL, 'ANNUAL_FIXED', 12, 25, NULL, true),
('Boxing Day',      NULL, 'ANNUAL_FIXED', 12, 26, NULL, true),
('Good Friday',     NULL, 'ANNUAL_EASTER_OFFSET', NULL, NULL, -2, false),
('Easter Monday',   NULL, 'ANNUAL_EASTER_OFFSET', NULL, NULL,  1, false)
ON CONFLICT DO NOTHING;

-- =========================================================================
-- 8. COMPLETIONS, COMMUNICATIONS, AUDIT & SYSTEM RECORDS
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.program_completions (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    program_id bigint NOT NULL,
    training_org_id bigint NULL,
    completion_date date NOT NULL,
    issued_flag varchar(1) NOT NULL DEFAULT 'N',
    parchment_number varchar(30) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_pc_org FOREIGN KEY (training_org_id) REFERENCES public.training_orgs(id) ON DELETE SET NULL,
    UNIQUE (student_id, program_id),
    CONSTRAINT chk_pc_issued CHECK (issued_flag IN ('Y','N'))
);

CREATE TABLE IF NOT EXISTS public.student_progress_reports (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    enrollment_id bigint NULL,
    report_period varchar(50) NULL,
    report_date date NOT NULL,
    document_url varchar(2048) NOT NULL,
    uploaded_by bigint NULL,
    keywords    text[] NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_report_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_report_enrollment FOREIGN KEY (enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE SET NULL,
    CONSTRAINT fk_report_uploader FOREIGN KEY (uploaded_by) REFERENCES public.app_users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS public.student_notes (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    note_type varchar(30) NOT NULL DEFAULT 'General',
    subject varchar(200) NULL,
    body text NOT NULL,
    is_private boolean NOT NULL DEFAULT false,
    created_by bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_note_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_note_creator FOREIGN KEY (created_by) REFERENCES public.app_users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_note_type CHECK (note_type IN ('General','Pastoral','Academic','Financial','Compliance','LAP','Incident','Communication'))
);

CREATE TABLE IF NOT EXISTS public.message_templates (
    id bigserial NOT NULL,
    template_name varchar(100) NOT NULL,
    channel varchar(10) NOT NULL,
    subject varchar(200) NULL,
    body_html text NULL,
    body_plain text NOT NULL,
    created_by bigint NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_template_name UNIQUE (template_name),
    CONSTRAINT fk_template_author FOREIGN KEY (created_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT chk_template_channel CHECK (channel IN ('Email', 'SMS', 'Both'))
);

CREATE TABLE IF NOT EXISTS public.message_campaigns (
    id bigserial NOT NULL,
    template_id bigint NULL,
    channel varchar(10) NOT NULL,
    subject varchar(200) NULL,
    body_html text NULL,
    body_plain text NOT NULL,
    sender_id bigint NOT NULL,
    audience_type varchar(20) NOT NULL,
    target_class_id bigint NULL,
    target_program_id bigint NULL,
    scheduled_at timestamp with time zone NULL,
    sent_at timestamp with time zone NULL,
    status varchar(20) NOT NULL DEFAULT 'Draft',
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_campaign_template FOREIGN KEY (template_id) REFERENCES public.message_templates(id) ON DELETE SET NULL,
    CONSTRAINT fk_campaign_sender FOREIGN KEY (sender_id) REFERENCES public.app_users(id) ON DELETE RESTRICT,
    CONSTRAINT fk_campaign_class FOREIGN KEY (target_class_id) REFERENCES public.classes(id) ON DELETE SET NULL,
    CONSTRAINT fk_campaign_program FOREIGN KEY (target_program_id) REFERENCES public.programs(id) ON DELETE SET NULL,
    CONSTRAINT chk_campaign_channel CHECK (channel IN ('Email', 'SMS', 'Both')),
    CONSTRAINT chk_campaign_audience CHECK (audience_type IN ('Individual', 'Class', 'Program', 'Cohort', 'Broadcast', 'Guardian')),
    CONSTRAINT chk_campaign_status CHECK (status IN ('Draft', 'Scheduled', 'Sending', 'Sent', 'Failed', 'Cancelled'))
);

CREATE TABLE IF NOT EXISTS public.message_deliveries (
    id bigserial NOT NULL,
    campaign_id bigint NOT NULL,
    recipient_type varchar(10) NOT NULL,
    student_id bigint NULL,
    guardian_id bigint NULL,
    staff_id bigint NULL,
    channel varchar(10) NOT NULL,
    address_used varchar(200) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'Pending',
    provider_message_id varchar(100) NULL,
    sent_at timestamp with time zone NULL,
    delivered_at timestamp with time zone NULL,
    failure_reason text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_delivery_campaign FOREIGN KEY (campaign_id) REFERENCES public.message_campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_delivery_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_delivery_guardian FOREIGN KEY (guardian_id) REFERENCES public.student_guardians(id) ON DELETE CASCADE,
    CONSTRAINT fk_delivery_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE,
    CONSTRAINT chk_delivery_recipient CHECK (recipient_type IN ('Student', 'Guardian', 'Staff')),
    CONSTRAINT chk_delivery_channel CHECK (channel IN ('Email', 'SMS')),
    CONSTRAINT chk_delivery_status CHECK (status IN ('Pending', 'Sent', 'Delivered', 'Failed', 'Bounced', 'OptedOut')),
    -- exactly one recipient relation is set, so Go can switch on it safely.
    CONSTRAINT chk_delivery_one_recipient CHECK (num_nonnulls(student_id, guardian_id, staff_id) = 1)
);

-- NEW v14: teacher-composed individual messages (distinct from bulk campaigns).
-- sender_id must reference an app_users row whose role is 'Trainer'.
-- Application layer enforces the role check; the schema carries the FK only.
CREATE TABLE IF NOT EXISTS public.messages (
    id bigserial NOT NULL,
    sender_id bigint NOT NULL,
    channel varchar(10) NOT NULL,
    subject varchar(200) NULL,
    body_html text NULL,
    body_plain text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'Draft',
    sent_at timestamp with time zone NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_msg_sender FOREIGN KEY (sender_id) REFERENCES public.app_users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_msg_channel CHECK (channel IN ('Email', 'SMS')),
    CONSTRAINT chk_msg_status CHECK (status IN ('Draft', 'Sent', 'Failed'))
);

-- NEW v14: per-recipient delivery rows for direct messages.
-- Exactly one of student_id / guardian_id / staff_id / teacher_id is set.
-- is_cc = true marks the auto-CC inserted for the sender by trg_cc_sender_on_send.
-- read_at is set by the application when the recipient opens the message.
CREATE TABLE IF NOT EXISTS public.message_recipients (
    id bigserial NOT NULL,
    message_id bigint NOT NULL,
    recipient_type varchar(10) NOT NULL,
    is_cc boolean NOT NULL DEFAULT false,
    student_id bigint NULL,
    guardian_id bigint NULL,
    staff_id bigint NULL,
    teacher_id bigint NULL,
    address_used varchar(200) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'Pending',
    provider_message_id varchar(100) NULL,
    sent_at timestamp with time zone NULL,
    delivered_at timestamp with time zone NULL,
    read_at timestamp with time zone NULL,
    failure_reason text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_mr_message FOREIGN KEY (message_id) REFERENCES public.messages(id) ON DELETE CASCADE,
    CONSTRAINT fk_mr_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_mr_guardian FOREIGN KEY (guardian_id) REFERENCES public.student_guardians(id) ON DELETE CASCADE,
    CONSTRAINT fk_mr_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE,
    CONSTRAINT fk_mr_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE,
    CONSTRAINT chk_mr_recipient CHECK (recipient_type IN ('Student', 'Guardian', 'Staff', 'Teacher')),
    CONSTRAINT chk_mr_status CHECK (status IN ('Pending', 'Sent', 'Delivered', 'Failed', 'Bounced', 'OptedOut', 'Read')),
    CONSTRAINT chk_mr_one_recipient CHECK (num_nonnulls(student_id, guardian_id, staff_id, teacher_id) = 1)
);

CREATE TABLE IF NOT EXISTS public.avetmiss_submissions (
    id bigserial NOT NULL,
    training_org_id bigint NOT NULL,
    reporting_year smallint NOT NULL,
    collection_type varchar(20) NOT NULL,
    submission_date date NOT NULL,
    submitted_by bigint NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'Submitted',
    nat_file_paths jsonb NULL,
    notes text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_sub_org FOREIGN KEY (training_org_id) REFERENCES public.training_orgs(id),
    CONSTRAINT fk_sub_user FOREIGN KEY (submitted_by) REFERENCES public.app_users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_sub_collection CHECK (collection_type IN ('Annual', 'Quarterly', 'Activity')),
    CONSTRAINT chk_sub_status CHECK (status IN ('Draft', 'Submitted', 'Accepted', 'Rejected', 'Resubmitted'))
);

-- generic append-only audit trail
CREATE TABLE IF NOT EXISTS public.audit_log (
    id bigserial NOT NULL,
    table_name text NOT NULL,
    record_id bigint NULL,
    action varchar(10) NOT NULL,
    actor_id bigint NULL,
    changed_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    old_data jsonb NULL,
    new_data jsonb NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_audit_actor FOREIGN KEY (actor_id) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT chk_audit_action CHECK (action IN ('INSERT', 'UPDATE', 'DELETE'))
);

-- =========================================================================
-- 8.5. WORKPLAN (VTSA 2024 CLAUSE 32.4 — ANNUAL TEACHER ALLOCATION)
-- =========================================================================
-- Captures each teacher's agreed annual allocation of Teaching Delivery,
-- CAPPS, and Education-Related Duties hours, split by academic period.
-- Follows the Victorian TAFE Teaching Staff Agreement 2024 (clause 32.4):
--   (a) Teaching Delivery — face-to-face, online or other means, including
--       in-class assessment and supervision; capped at 800 h p.a.
--   (b) CAPPS — curriculum, assessment (out-of-class), planning, preparation,
--       student consultation; 45 minutes allocated per teaching hour (0.75).
--   (c) Education-Related Duties — compliance, industry/community engagement,
--       PD per Schedule 7, applied research, travel and meetings.
CREATE TABLE IF NOT EXISTS public.workplans (
    id bigserial NOT NULL,
    teacher_id bigint NOT NULL,
    calendar_year smallint NOT NULL,
    version smallint NOT NULL DEFAULT 1,
    status varchar(20) NOT NULL DEFAULT 'Draft',
    time_fraction numeric(4,3) NOT NULL DEFAULT 1.000,          -- full-year FTE
    capps_ratio numeric(4,3) NOT NULL DEFAULT 0.750,            -- CAPPS minutes per teaching minute (0.75 = 45 min)
    accountable_hours_required numeric(7,2) NOT NULL,           -- contractual total derived from employment conditions
    agreed_overtime_hours numeric(6,2) NOT NULL DEFAULT 0.00,   -- agreed excess teaching duty hours
    submitted_by bigint NULL,                                    -- app_users.id of the person who submitted for approval
    submitted_at timestamp with time zone NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_workplan UNIQUE (teacher_id, calendar_year, version),
    CONSTRAINT fk_workplan_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT,
    CONSTRAINT fk_workplan_submitted_by FOREIGN KEY (submitted_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT chk_workplan_status CHECK (status IN ('Draft', 'Submitted', 'Approved')),
    CONSTRAINT chk_workplan_fraction CHECK (time_fraction > 0 AND time_fraction <= 1),
    CONSTRAINT chk_workplan_capps_ratio CHECK (capps_ratio > 0 AND capps_ratio <= 1),
    CONSTRAINT chk_workplan_req_hours CHECK (accountable_hours_required > 0),
    CONSTRAINT chk_workplan_overtime CHECK (agreed_overtime_hours >= 0)
);

-- One row per approval step in the standard VTSA workflow: first the teacher
-- approves their own workplan, then the line manager confirms.
CREATE TABLE IF NOT EXISTS public.workplan_approvals (
    id bigserial NOT NULL,
    workplan_id bigint NOT NULL,
    approver_id bigint NOT NULL,
    approval_role varchar(30) NOT NULL,
    approved_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notes text NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_workplan_approval UNIQUE (workplan_id, approval_role),
    CONSTRAINT fk_wa_workplan FOREIGN KEY (workplan_id) REFERENCES public.workplans(id) ON DELETE CASCADE,
    CONSTRAINT fk_wa_approver FOREIGN KEY (approver_id) REFERENCES public.app_users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_wa_role CHECK (approval_role IN ('Teacher', 'LineManager'))
);

-- Individual activity line items that make up a workplan. Covers all three
-- clause 32.4 categories. Session-linked Teaching Delivery rows carry an
-- activity_start_date (the session date) and link to a class_session.
-- Aggregated or manually-entered rows leave dates NULL.
CREATE TABLE IF NOT EXISTS public.workplan_entries (
    id bigserial NOT NULL,
    workplan_id bigint NOT NULL,
    entry_type varchar(30) NOT NULL,           -- 'Teaching Delivery' | 'CAPPS' | 'Education Related Duties'
    activity_name varchar(100) NOT NULL,        -- e.g. 'Teaching Session', 'Planning', 'Mentoring'
    subject_id bigint NULL,                     -- unit/subject being taught or assessed (nullable)
    program_id bigint NULL,                     -- course context (nullable)
    academic_period_id bigint NULL,             -- semester or period this entry falls in (nullable)
    activity_start_date date NULL,              -- populated for session-linked entries
    activity_end_date date NULL,                -- populated for session-linked entries
    total_hours numeric(6,2) NOT NULL,
    comments text NULL,
    class_session_id bigint NULL,               -- FK to class_sessions for session-linked entries
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_we_workplan FOREIGN KEY (workplan_id) REFERENCES public.workplans(id) ON DELETE CASCADE,
    CONSTRAINT fk_we_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE SET NULL,
    CONSTRAINT fk_we_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE SET NULL,
    CONSTRAINT fk_we_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE SET NULL,
    CONSTRAINT fk_we_session FOREIGN KEY (class_session_id) REFERENCES public.class_sessions(id) ON DELETE SET NULL,
    CONSTRAINT chk_we_type CHECK (entry_type IN ('Teaching Delivery', 'CAPPS', 'Education Related Duties')),
    CONSTRAINT chk_we_hours CHECK (total_hours > 0),
    CONSTRAINT chk_we_dates CHECK (activity_end_date IS NULL OR activity_end_date >= activity_start_date)
);

-- =========================================================================
-- 8.6. TIMESHEET (FORTNIGHTLY PAYROLL HOURS — AUTO-POPULATED FROM SESSIONS)
-- =========================================================================

-- Administrator-defined pay periods (typically fortnightly).
CREATE TABLE IF NOT EXISTS public.pay_periods (
    id            bigserial    NOT NULL,
    period_start  date         NOT NULL,
    period_end    date         NOT NULL,
    period_name   varchar(50)  NOT NULL,    -- e.g. 'FN01 2026'
    calendar_year smallint     NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_pay_period_start UNIQUE (period_start),
    CONSTRAINT uq_pay_period_name  UNIQUE (calendar_year, period_name),
    CONSTRAINT chk_pp_dates CHECK (period_end > period_start)
);

-- One timesheet per teacher per pay period. Carries hours only —
-- no banking, super, or pay-rate data (submitted to external payroll).
CREATE TABLE IF NOT EXISTS public.timesheets (
    id             bigserial    NOT NULL,
    teacher_id     bigint       NOT NULL,
    pay_period_id  bigint       NOT NULL,
    status         varchar(20)  NOT NULL DEFAULT 'Draft',
    submitted_by   bigint       NULL,
    submitted_at   timestamp with time zone NULL,
    approved_by    bigint       NULL,
    approved_at    timestamp with time zone NULL,
    exported_at    timestamp with time zone NULL,
    export_format  varchar(10)  NULL,       -- 'PDF', 'XLSX', etc.
    notes          text         NULL,
    created_at     timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at     timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_timesheet       UNIQUE (teacher_id, pay_period_id),
    CONSTRAINT fk_ts_teacher      FOREIGN KEY (teacher_id)    REFERENCES public.teachers(id)    ON DELETE RESTRICT,
    CONSTRAINT fk_ts_pay_period   FOREIGN KEY (pay_period_id) REFERENCES public.pay_periods(id) ON DELETE RESTRICT,
    CONSTRAINT fk_ts_submitted_by FOREIGN KEY (submitted_by)  REFERENCES public.app_users(id)   ON DELETE SET NULL,
    CONSTRAINT fk_ts_approved_by  FOREIGN KEY (approved_by)   REFERENCES public.app_users(id)   ON DELETE SET NULL,
    CONSTRAINT chk_ts_status  CHECK (status IN ('Draft', 'Submitted', 'Approved', 'Exported')),
    CONSTRAINT chk_ts_export  CHECK (exported_at IS NULL OR export_format IS NOT NULL)
);

-- Individual hour lines on a timesheet.
-- Teaching Delivery rows are auto-populated from class_sessions (class_session_id set).
-- CAPPS rows are derived from teaching hours × workplan.capps_ratio.
-- Education Related Duties and Other rows are manually entered.
CREATE TABLE IF NOT EXISTS public.timesheet_entries (
    id                 bigserial    NOT NULL,
    timesheet_id       bigint       NOT NULL,
    entry_date         date         NOT NULL,
    entry_type         varchar(30)  NOT NULL,
    description        varchar(200) NULL,
    hours              numeric(5,2) NOT NULL,
    is_overtime        boolean      NOT NULL DEFAULT false,
    class_session_id   bigint       NULL,
    workplan_entry_id  bigint       NULL,
    created_at         timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_te_timesheet      FOREIGN KEY (timesheet_id)      REFERENCES public.timesheets(id)        ON DELETE CASCADE,
    CONSTRAINT fk_te_session        FOREIGN KEY (class_session_id)  REFERENCES public.class_sessions(id)   ON DELETE SET NULL,
    CONSTRAINT fk_te_workplan_entry FOREIGN KEY (workplan_entry_id) REFERENCES public.workplan_entries(id)  ON DELETE SET NULL,
    CONSTRAINT chk_te_type  CHECK (entry_type IN ('Teaching Delivery', 'CAPPS', 'Education Related Duties', 'Other')),
    CONSTRAINT chk_te_hours CHECK (hours > 0)
);

-- =========================================================================
-- 9. DEFERRED CROSS-FK RELATIONSHIPS
-- =========================================================================
ALTER TABLE IF EXISTS public.student_course_enrollments ADD CONSTRAINT fk_se_student   FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS public.student_course_enrollments ADD CONSTRAINT fk_se_program   FOREIGN KEY (program_id) REFERENCES public.programs (id) ON DELETE RESTRICT;

ALTER TABLE IF EXISTS public.client_subject_enrolments ADD CONSTRAINT fk_cse_student   FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS public.client_subject_enrolments ADD CONSTRAINT fk_cse_subject   FOREIGN KEY (subject_id) REFERENCES public.subjects (id) ON DELETE RESTRICT;

ALTER TABLE IF EXISTS public.program_completions      ADD CONSTRAINT fk_pc_student     FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS public.program_completions      ADD CONSTRAINT fk_pc_program     FOREIGN KEY (program_id) REFERENCES public.programs (id) ON DELETE RESTRICT;

ALTER TABLE IF EXISTS public.class_subjects           ADD CONSTRAINT fk_cl_class       FOREIGN KEY (class_id)   REFERENCES public.classes (id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.class_subjects           ADD CONSTRAINT fk_cl_subject     FOREIGN KEY (subject_id) REFERENCES public.subjects (id) ON DELETE CASCADE;

ALTER TABLE IF EXISTS public.class_enrollments        ADD CONSTRAINT fk_ce_class       FOREIGN KEY (class_id) REFERENCES public.classes (id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.class_enrollments        ADD CONSTRAINT fk_ce_subject_enrolment FOREIGN KEY (client_subject_enrolment_id) REFERENCES public.client_subject_enrolments (id) ON DELETE CASCADE;

ALTER TABLE IF EXISTS public.class_exceptions         ADD CONSTRAINT fk_cx_class       FOREIGN KEY (class_id) REFERENCES public.classes (id) ON DELETE CASCADE;

ALTER TABLE IF EXISTS public.class_slots              ADD CONSTRAINT fk_cs_class       FOREIGN KEY (class_id) REFERENCES public.classes (id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.class_slots              ADD CONSTRAINT fk_cs_period      FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods (id);
ALTER TABLE IF EXISTS public.class_slots              ADD CONSTRAINT fk_cs_teacher     FOREIGN KEY (teacher_id) REFERENCES public.teachers (id) ON DELETE RESTRICT;
ALTER TABLE IF EXISTS public.class_slots              ADD CONSTRAINT fk_cs_room        FOREIGN KEY (room_id) REFERENCES public.rooms (id) ON DELETE SET NULL;

ALTER TABLE IF EXISTS public.delivery_locations       ADD CONSTRAINT fk_delivery_loc_parent FOREIGN KEY (training_org_id) REFERENCES public.training_orgs(id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.buildings                ADD CONSTRAINT fk_building_parent FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.buildings                ADD CONSTRAINT fk_building_state  FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code);
ALTER TABLE IF EXISTS public.rooms                    ADD CONSTRAINT fk_room_parent    FOREIGN KEY (building_id) REFERENCES public.buildings(id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.learning_access_plans    ADD CONSTRAINT fk_lap_assessor   FOREIGN KEY (assessor_id) REFERENCES public.staff(id) ON DELETE RESTRICT;

-- =========================================================================
-- 10. INDEXES
-- =========================================================================
CREATE INDEX IF NOT EXISTS idx_class_enrollments_cse ON public.class_enrollments(client_subject_enrolment_id);
CREATE INDEX IF NOT EXISTS idx_slots_period_teacher  ON public.class_slots(academic_period_id, teacher_id);
CREATE INDEX IF NOT EXISTS idx_students_usi          ON public.students(usi) WHERE (usi IS NOT NULL);
-- student_number / student_email are unique among ACTIVE (non-deleted) rows only.
CREATE UNIQUE INDEX IF NOT EXISTS uq_student_number_active ON public.students(student_number) WHERE (deleted_at IS NULL);
CREATE UNIQUE INDEX IF NOT EXISTS uq_student_email_active  ON public.students(student_email)  WHERE (deleted_at IS NULL);
CREATE INDEX IF NOT EXISTS idx_cse_student           ON public.client_subject_enrolments(student_id);
CREATE INDEX IF NOT EXISTS idx_cse_course_enrolment  ON public.client_subject_enrolments(student_course_enrollment_id);
CREATE INDEX IF NOT EXISTS idx_cse_subject           ON public.client_subject_enrolments(subject_id);
CREATE INDEX IF NOT EXISTS idx_cse_outcome           ON public.client_subject_enrolments(outcome_id_national);
CREATE INDEX IF NOT EXISTS idx_cse_delivery_loc      ON public.client_subject_enrolments(delivery_location_id);
CREATE INDEX IF NOT EXISTS idx_sce_program           ON public.student_course_enrollments(program_id);
CREATE INDEX IF NOT EXISTS idx_sce_status            ON public.student_course_enrollments(enrollment_status);
CREATE INDEX IF NOT EXISTS idx_pc_program            ON public.program_completions(program_id);
CREATE INDEX IF NOT EXISTS idx_sessions_date         ON public.class_sessions(session_date);
CREATE INDEX IF NOT EXISTS idx_sessions_class        ON public.class_sessions(class_id);
CREATE INDEX IF NOT EXISTS idx_classes_group         ON public.classes(academic_period_id, group_code) WHERE (group_code IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_session_teachers_teacher ON public.session_teachers(teacher_id);
CREATE INDEX IF NOT EXISTS idx_attendance_session    ON public.session_attendance(session_id);
CREATE INDEX IF NOT EXISTS idx_attendance_student    ON public.session_attendance(student_id);
CREATE INDEX IF NOT EXISTS idx_student_notes_student ON public.student_notes(student_id);
CREATE INDEX IF NOT EXISTS idx_notes_type            ON public.student_notes(note_type);
-- NEW v11: support efficient lookups on the period allocation tables.
CREATE INDEX IF NOT EXISTS idx_tpa_teacher           ON public.teacher_period_allocations(teacher_id);
CREATE INDEX IF NOT EXISTS idx_tpa_period            ON public.teacher_period_allocations(academic_period_id);
CREATE INDEX IF NOT EXISTS idx_he_enrol_period       ON public.he_enrolment_details(academic_period_id) WHERE (academic_period_id IS NOT NULL);
-- NEW v12: workplan lookup indexes.
CREATE INDEX IF NOT EXISTS idx_workplans_teacher_year     ON public.workplans(teacher_id, calendar_year);
CREATE INDEX IF NOT EXISTS idx_we_workplan                ON public.workplan_entries(workplan_id);
CREATE INDEX IF NOT EXISTS idx_we_type_period             ON public.workplan_entries(workplan_id, entry_type, academic_period_id);
CREATE INDEX IF NOT EXISTS idx_we_session                 ON public.workplan_entries(class_session_id) WHERE (class_session_id IS NOT NULL);

-- NEW v13: timesheet lookup indexes.
CREATE INDEX IF NOT EXISTS idx_pay_periods_year           ON public.pay_periods(calendar_year);
CREATE INDEX IF NOT EXISTS idx_timesheets_teacher         ON public.timesheets(teacher_id);
CREATE INDEX IF NOT EXISTS idx_timesheets_pay_period      ON public.timesheets(pay_period_id);

-- NEW v20: intake & cohort indexes.
CREATE INDEX IF NOT EXISTS idx_pi_program       ON public.program_intakes(program_id);
CREATE INDEX IF NOT EXISTS idx_pi_period        ON public.program_intakes(start_academic_period_id);
CREATE INDEX IF NOT EXISTS idx_pi_status        ON public.program_intakes(status);
CREATE INDEX IF NOT EXISTS idx_ig_intake        ON public.intake_groups(intake_id);
CREATE INDEX IF NOT EXISTS idx_sce_intake_grp   ON public.student_course_enrollments(intake_group_id) WHERE (intake_group_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_class_intake_grp ON public.classes(intake_group_id) WHERE (intake_group_id IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_timesheets_status          ON public.timesheets(status) WHERE (status <> 'Exported');
CREATE INDEX IF NOT EXISTS idx_te_timesheet               ON public.timesheet_entries(timesheet_id);
CREATE INDEX IF NOT EXISTS idx_te_session                 ON public.timesheet_entries(class_session_id) WHERE (class_session_id IS NOT NULL);

-- NEW v14: direct message lookup indexes.
CREATE INDEX IF NOT EXISTS idx_messages_sender            ON public.messages(sender_id);
CREATE INDEX IF NOT EXISTS idx_messages_status            ON public.messages(status) WHERE (status = 'Draft');
CREATE INDEX IF NOT EXISTS idx_mr_message                 ON public.message_recipients(message_id);
CREATE INDEX IF NOT EXISTS idx_mr_teacher                 ON public.message_recipients(teacher_id) WHERE (teacher_id IS NOT NULL);

-- =========================================================================
-- 11. COMPLIANCE & TEACHING-HOURS ENGINE (sessions are the source of truth)
-- =========================================================================

-- Postgres does NOT auto-touch updated_at; this trigger does, on every UPDATE.
CREATE OR REPLACE FUNCTION public.fn_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_touch_academic_periods           BEFORE UPDATE ON public.academic_periods           FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_people                     BEFORE UPDATE ON public.people                     FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

CREATE OR REPLACE FUNCTION public.fn_upper_family_name()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.family_name = UPPER(NEW.family_name);
    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER trg_upper_family_name
    BEFORE INSERT OR UPDATE OF family_name ON public.people
    FOR EACH ROW EXECUTE FUNCTION public.fn_upper_family_name();
CREATE OR REPLACE TRIGGER trg_touch_app_users                  BEFORE UPDATE ON public.app_users                  FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_students                   BEFORE UPDATE ON public.students                   FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_teachers                   BEFORE UPDATE ON public.teachers                   FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_program_intakes            BEFORE UPDATE ON public.program_intakes             FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_course_enrollments         BEFORE UPDATE ON public.student_course_enrollments  FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_subject_enrolments         BEFORE UPDATE ON public.client_subject_enrolments   FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_training_plans             BEFORE UPDATE ON public.training_plans             FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_learning_access_plans      BEFORE UPDATE ON public.learning_access_plans      FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_classes                    BEFORE UPDATE ON public.classes                    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_student_notes              BEFORE UPDATE ON public.student_notes              FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_message_templates          BEFORE UPDATE ON public.message_templates          FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_workplans                  BEFORE UPDATE ON public.workplans                  FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_timesheets                 BEFORE UPDATE ON public.timesheets                 FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
CREATE OR REPLACE TRIGGER trg_touch_messages                   BEFORE UPDATE ON public.messages                   FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();
-- (teacher_yearly_balances.updated_at is set explicitly by the hours engine, so no trigger here.)

-- NEW v14: auto-insert a CC recipient row for the sender when a direct message
-- transitions to 'Sent'. Sender is resolved as Teacher first, then Staff;
-- service accounts without a person_id row silently produce no CC.
CREATE OR REPLACE FUNCTION public.fn_cc_sender_on_send()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    v_teacher_id bigint;
    v_staff_id   bigint;
    v_email      text;
BEGIN
    -- Resolve sender as teacher
    SELECT t.id, COALESCE(t.teacher_email, p.primary_email)
      INTO v_teacher_id, v_email
      FROM public.app_users au
      JOIN public.people   p ON p.id = au.person_id
      JOIN public.teachers t ON t.id = au.person_id
     WHERE au.id = NEW.sender_id;

    IF FOUND THEN
        INSERT INTO public.message_recipients(
            message_id, recipient_type, is_cc, teacher_id, address_used, status, sent_at
        ) VALUES (
            NEW.id, 'Teacher', true, v_teacher_id, v_email, 'Sent', NEW.sent_at
        );
        RETURN NEW;
    END IF;

    -- Fall back to staff sender
    SELECT s.id, COALESCE(s.staff_email, p.primary_email)
      INTO v_staff_id, v_email
      FROM public.app_users au
      JOIN public.people p ON p.id = au.person_id
      JOIN public.staff  s ON s.id = au.person_id
     WHERE au.id = NEW.sender_id;

    IF FOUND THEN
        INSERT INTO public.message_recipients(
            message_id, recipient_type, is_cc, staff_id, address_used, status, sent_at
        ) VALUES (
            NEW.id, 'Staff', true, v_staff_id, v_email, 'Sent', NEW.sent_at
        );
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_cc_sender_on_send
    AFTER UPDATE OF status ON public.messages
    FOR EACH ROW
    WHEN (NEW.status = 'Sent' AND OLD.status <> 'Sent')
    EXECUTE FUNCTION public.fn_cc_sender_on_send();

-- Auto-populate class_slots.academic_period_id from its class so the exclusion
-- constraint key stays correct without relying on the application.
CREATE OR REPLACE FUNCTION public.fn_set_slot_period()
RETURNS TRIGGER AS $$
BEGIN
    SELECT c.academic_period_id INTO NEW.academic_period_id
    FROM public.classes c WHERE c.id = NEW.class_id;
    IF NEW.academic_period_id IS NULL THEN
        RAISE EXCEPTION 'class_slots: class % has no academic_period_id', NEW.class_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_set_slot_period
    BEFORE INSERT OR UPDATE OF class_id ON public.class_slots
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_slot_period();

-- Single point of truth for adjusting a teacher's yearly balance.
-- Cap enforced here (friendly error) and by chk_balance_cap (hard backstop).
-- FOR UPDATE serialises concurrent writers; the prior UPSERT guarantees the row
-- exists, closing the v8 race where the first session of the year locked nothing.
-- NOTE (v11): the 800.00 literal has been removed. The cap is read exclusively
-- from teachers.default_max_hours_per_year, which is NOT NULL and configurable
-- per teacher. Change that column to match any employment contract.
CREATE OR REPLACE FUNCTION public.fn_adjust_teacher_balance(p_teacher bigint, p_year smallint, p_delta numeric)
RETURNS void AS $$
DECLARE
    v_current numeric(7,2);
    v_max numeric(6,2);
BEGIN
    IF p_delta = 0 THEN RETURN; END IF;

    INSERT INTO public.teacher_yearly_balances (teacher_id, calendar_year, allocated_max_hours, booked_hours)
    VALUES (p_teacher, p_year,
            (SELECT default_max_hours_per_year FROM public.teachers WHERE id = p_teacher),
            0.00)
    ON CONFLICT (teacher_id, calendar_year) DO NOTHING;

    SELECT booked_hours, allocated_max_hours INTO v_current, v_max
    FROM public.teacher_yearly_balances
    WHERE teacher_id = p_teacher AND calendar_year = p_year
    FOR UPDATE;

    IF p_delta > 0 AND (v_current + p_delta) > v_max THEN
        RAISE EXCEPTION 'Teacher % would reach %.2f teaching hours in %, exceeding the %.2f annual cap by %.2f.',
            p_teacher, (v_current + p_delta), p_year, v_max, ((v_current + p_delta) - v_max);
    END IF;

    UPDATE public.teacher_yearly_balances
    SET booked_hours = GREATEST(booked_hours + p_delta, 0.00),
        updated_at = CURRENT_TIMESTAMP
    WHERE teacher_id = p_teacher AND calendar_year = p_year;
END;
$$ LANGUAGE plpgsql;

-- NEW v11: single point of truth for adjusting a teacher's per-period balance.
-- Only acts when the teacher has max_hours_per_period set (NULL = no per-period
-- tracking for that teacher). Auto-creates the allocation row on first call.
-- FOR UPDATE serialises concurrent writers the same way as the yearly function.
CREATE OR REPLACE FUNCTION public.fn_adjust_teacher_period_balance(p_teacher bigint, p_period bigint, p_delta numeric)
RETURNS void AS $$
DECLARE
    v_current numeric(7,2);
    v_max numeric(6,2);
BEGIN
    IF p_delta = 0 THEN RETURN; END IF;

    SELECT max_hours_per_period INTO v_max
    FROM public.teachers WHERE id = p_teacher;

    IF v_max IS NULL THEN RETURN; END IF;  -- per-period tracking not enabled for this teacher

    INSERT INTO public.teacher_period_allocations (teacher_id, academic_period_id, allocated_hours, booked_hours)
    VALUES (p_teacher, p_period, v_max, 0.00)
    ON CONFLICT (teacher_id, academic_period_id) DO NOTHING;

    SELECT booked_hours, allocated_hours INTO v_current, v_max
    FROM public.teacher_period_allocations
    WHERE teacher_id = p_teacher AND academic_period_id = p_period
    FOR UPDATE;

    IF p_delta > 0 AND (v_current + p_delta) > v_max THEN
        RAISE EXCEPTION 'Teacher % would reach %.2f hours in academic period %, exceeding the %.2f per-period cap by %.2f.',
            p_teacher, (v_current + p_delta), p_period, v_max, ((v_current + p_delta) - v_max);
    END IF;

    UPDATE public.teacher_period_allocations
    SET booked_hours = GREATEST(booked_hours + p_delta, 0.00),
        updated_at = CURRENT_TIMESTAMP
    WHERE teacher_id = p_teacher AND academic_period_id = p_period;
END;
$$ LANGUAGE plpgsql;

-- Hours contribution of one (session, teacher) pair. Guest roles do not accrue.
-- v11: also drives teacher_period_allocations for teachers with max_hours_per_period set.
CREATE OR REPLACE FUNCTION public.fn_session_teacher_hours()
RETURNS TRIGGER AS $$
DECLARE
    v_date date;
    v_start time;
    v_end time;
    v_cancelled boolean;
    v_hours numeric(7,2);
    v_role varchar(30);
    v_teacher bigint;
    v_period bigint;
    v_sign int;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_role := NEW.role; v_teacher := NEW.teacher_id; v_sign := 1;
        SELECT cs.session_date, cs.start_time, cs.end_time, cs.cancelled, c.academic_period_id
        INTO v_date, v_start, v_end, v_cancelled, v_period
        FROM public.class_sessions cs
        JOIN public.classes c ON c.id = cs.class_id
        WHERE cs.id = NEW.session_id;
    ELSE
        v_role := OLD.role; v_teacher := OLD.teacher_id; v_sign := -1;
        SELECT cs.session_date, cs.start_time, cs.end_time, cs.cancelled, c.academic_period_id
        INTO v_date, v_start, v_end, v_cancelled, v_period
        FROM public.class_sessions cs
        JOIN public.classes c ON c.id = cs.class_id
        WHERE cs.id = OLD.session_id;
    END IF;

    IF v_role = 'Guest' OR COALESCE(v_cancelled, true) THEN
        RETURN NULL;
    END IF;

    v_hours := EXTRACT(EPOCH FROM (v_end - v_start)) / 3600.00;
    PERFORM public.fn_adjust_teacher_balance(v_teacher, EXTRACT(YEAR FROM v_date)::smallint, v_sign * v_hours);
    PERFORM public.fn_adjust_teacher_period_balance(v_teacher, v_period, v_sign * v_hours);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_session_teacher_hours
    AFTER INSERT OR DELETE ON public.session_teachers
    FOR EACH ROW EXECUTE FUNCTION public.fn_session_teacher_hours();

-- When a session is cancelled/uncancelled or its time/date is edited, re-apply
-- the delta for every (non-guest) teacher on it. Keeps both yearly and period
-- balances correct regardless of which field changed.
CREATE OR REPLACE FUNCTION public.fn_session_change_hours()
RETURNS TRIGGER AS $$
DECLARE
    r RECORD;
    v_period bigint;
    v_old_hours numeric(7,2) := EXTRACT(EPOCH FROM (OLD.end_time - OLD.start_time)) / 3600.00;
    v_new_hours numeric(7,2) := EXTRACT(EPOCH FROM (NEW.end_time - NEW.start_time)) / 3600.00;
BEGIN
    IF OLD.cancelled = NEW.cancelled
       AND OLD.start_time = NEW.start_time
       AND OLD.end_time = NEW.end_time
       AND OLD.session_date = NEW.session_date THEN
        RETURN NEW;  -- nothing hours-relevant changed
    END IF;

    SELECT c.academic_period_id INTO v_period
    FROM public.classes c WHERE c.id = NEW.class_id;

    FOR r IN SELECT teacher_id FROM public.session_teachers WHERE session_id = NEW.id AND role <> 'Guest' LOOP
        IF NOT OLD.cancelled THEN
            PERFORM public.fn_adjust_teacher_balance(r.teacher_id, EXTRACT(YEAR FROM OLD.session_date)::smallint, -v_old_hours);
            PERFORM public.fn_adjust_teacher_period_balance(r.teacher_id, v_period, -v_old_hours);
        END IF;
        IF NOT NEW.cancelled THEN
            PERFORM public.fn_adjust_teacher_balance(r.teacher_id, EXTRACT(YEAR FROM NEW.session_date)::smallint,  v_new_hours);
            PERFORM public.fn_adjust_teacher_period_balance(r.teacher_id, v_period,  v_new_hours);
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_session_change_hours
    AFTER UPDATE ON public.class_sessions
    FOR EACH ROW EXECUTE FUNCTION public.fn_session_change_hours();

-- Safety net: fully recompute a teacher's year from scratch (for backfills/repairs).
-- v11: no longer uses a hard-coded 800.00 fallback; reads from teachers.default_max_hours_per_year.
CREATE OR REPLACE FUNCTION public.fn_recompute_teacher_balance(p_teacher bigint, p_year smallint)
RETURNS void AS $$
DECLARE
    v_total numeric(7,2);
BEGIN
    SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (cs.end_time - cs.start_time)) / 3600.00), 0.00)
    INTO v_total
    FROM public.session_teachers st
    JOIN public.class_sessions cs ON cs.id = st.session_id
    WHERE st.teacher_id = p_teacher
      AND st.role <> 'Guest'
      AND cs.cancelled = false
      AND EXTRACT(YEAR FROM cs.session_date) = p_year;

    INSERT INTO public.teacher_yearly_balances (teacher_id, calendar_year, allocated_max_hours, booked_hours)
    VALUES (p_teacher, p_year,
            (SELECT default_max_hours_per_year FROM public.teachers WHERE id = p_teacher),
            v_total)
    ON CONFLICT (teacher_id, calendar_year)
    DO UPDATE SET booked_hours = EXCLUDED.booked_hours, updated_at = CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- NEW v11: fully recompute a teacher's per-period balance from scratch.
-- No-ops if the teacher has no max_hours_per_period configured.
CREATE OR REPLACE FUNCTION public.fn_recompute_teacher_period_balance(p_teacher bigint, p_period bigint)
RETURNS void AS $$
DECLARE
    v_total numeric(7,2);
    v_max numeric(6,2);
BEGIN
    SELECT max_hours_per_period INTO v_max
    FROM public.teachers WHERE id = p_teacher;

    IF v_max IS NULL THEN RETURN; END IF;

    SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (cs.end_time - cs.start_time)) / 3600.00), 0.00)
    INTO v_total
    FROM public.session_teachers st
    JOIN public.class_sessions cs ON cs.id = st.session_id
    JOIN public.classes c ON c.id = cs.class_id
    WHERE st.teacher_id = p_teacher
      AND st.role <> 'Guest'
      AND cs.cancelled = false
      AND c.academic_period_id = p_period;

    INSERT INTO public.teacher_period_allocations (teacher_id, academic_period_id, allocated_hours, booked_hours)
    VALUES (p_teacher, p_period, v_max, v_total)
    ON CONFLICT (teacher_id, academic_period_id)
    DO UPDATE SET booked_hours = EXCLUDED.booked_hours, updated_at = CURRENT_TIMESTAMP;
END;
$$ LANGUAGE plpgsql;

-- Session-level teacher double-booking guard (catches ad-hoc/replacement sessions
-- that bypass the slot template). Slot-level overlaps are still caught by the
-- class_slots exclusion constraint.
CREATE OR REPLACE FUNCTION public.fn_check_teacher_session_conflict()
RETURNS TRIGGER AS $$
DECLARE
    v_conflict bigint;
BEGIN
    SELECT cs2.id INTO v_conflict
    FROM public.class_sessions cs1
    JOIN public.session_teachers st2 ON st2.teacher_id = NEW.teacher_id
    JOIN public.class_sessions cs2 ON cs2.id = st2.session_id
    WHERE cs1.id = NEW.session_id
      AND cs2.id <> cs1.id
      AND cs2.session_date = cs1.session_date
      AND cs2.cancelled = false
      AND public.timerange(cs1.start_time, cs1.end_time) && public.timerange(cs2.start_time, cs2.end_time)
    LIMIT 1;

    IF v_conflict IS NOT NULL THEN
        RAISE EXCEPTION 'Teacher % is already booked in session % at an overlapping time.', NEW.teacher_id, v_conflict;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_check_teacher_session_conflict
    BEFORE INSERT ON public.session_teachers
    FOR EACH ROW EXECUTE FUNCTION public.fn_check_teacher_session_conflict();

-- Existing v8 compliance notices, retained.
CREATE OR REPLACE FUNCTION public.fn_validate_training_plan_compliance()
RETURNS TRIGGER AS $$
DECLARE
    v_commencement_date date;
    v_days_elapsed integer;
BEGIN
    SELECT commencement_date INTO v_commencement_date
    FROM public.student_course_enrollments WHERE id = NEW.student_course_enrollment_id;

    IF NEW.training_plan_fully_executed_date IS NOT NULL THEN
        v_days_elapsed := NEW.training_plan_fully_executed_date - v_commencement_date;
        IF v_days_elapsed > 42 THEN
            RAISE NOTICE 'Compliance Warning: Training Plan for Enrollment % executed % days after commencement (exceeds 42-day benchmark).',
                NEW.student_course_enrollment_id, v_days_elapsed;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_validate_apprenticeship_compliance
    BEFORE INSERT OR UPDATE ON public.apprenticeship_details
    FOR EACH ROW EXECUTE FUNCTION public.fn_validate_training_plan_compliance();

CREATE OR REPLACE FUNCTION public.fn_validate_traineeship_constraints()
RETURNS TRIGGER AS $$
DECLARE
    v_commencement date;
BEGIN
    SELECT commencement_date INTO v_commencement
    FROM public.student_course_enrollments WHERE id = NEW.student_course_enrollment_id;

    IF NEW.probation_start_date < v_commencement THEN
        RAISE EXCEPTION 'Traineeship Guard: Probation start (%) cannot precede commencement (%).',
            NEW.probation_start_date, v_commencement;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_validate_traineeship_milestones
    BEFORE INSERT OR UPDATE ON public.traineeship_details
    FOR EACH ROW EXECUTE FUNCTION public.fn_validate_traineeship_constraints();

-- =========================================================================
-- 12. HOLIDAY EXPANSION (rules -> concrete observances per year)
-- =========================================================================

-- Easter Sunday via the Anonymous Gregorian algorithm (Meeus/Jones/Butcher).
CREATE OR REPLACE FUNCTION public.fn_easter_sunday(p_year integer)
RETURNS date AS $$
DECLARE
    a int; b int; c int; d int; e int; f int; g int;
    h int; i int; k int; l int; m int; mon int; day int;
BEGIN
    a := p_year % 19;
    b := p_year / 100;
    c := p_year % 100;
    d := b / 4;
    e := b % 4;
    f := (b + 8) / 25;
    g := (b - f + 1) / 3;
    h := (19*a + b - d - g + 15) % 30;
    i := c / 4;
    k := c % 4;
    l := (32 + 2*e + 2*i - h - k) % 7;
    m := (a + 11*h + 22*l) / 451;
    mon := (h + l - 7*m + 114) / 31;
    day := ((h + l - 7*m + 114) % 31) + 1;
    RETURN make_date(p_year, mon, day);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- nth weekday of a month (nth = -1 -> last). Returns NULL if a positive nth
-- overflows the month (e.g. a 5th Monday that doesn't exist).
CREATE OR REPLACE FUNCTION public.fn_nth_weekday(p_year int, p_month int, p_weekday int, p_nth int)
RETURNS date AS $$
DECLARE
    v_first date := make_date(p_year, p_month, 1);
    v_last  date := (date_trunc('month', make_date(p_year, p_month, 1)) + interval '1 month - 1 day')::date;
    v_offset int;
    v_result date;
BEGIN
    IF p_nth = -1 THEN
        v_offset := (EXTRACT(ISODOW FROM v_last)::int - p_weekday + 7) % 7;
        RETURN v_last - v_offset;
    END IF;
    v_offset := (p_weekday - EXTRACT(ISODOW FROM v_first)::int + 7) % 7;
    v_result := v_first + v_offset + (p_nth - 1) * 7;
    IF v_result > v_last THEN RETURN NULL; END IF;  -- e.g. no 5th occurrence
    RETURN v_result;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Expand all active rules into concrete observances for one year. Idempotent
-- (ON CONFLICT DO NOTHING), so it is safe to re-run. Handles weekend-substitute
-- days by shifting to the next free weekday.
CREATE OR REPLACE FUNCTION public.fn_materialise_holidays(p_year smallint)
RETURNS integer AS $$
DECLARE
    r RECORD;
    v_date date;
    v_sub  date;
    v_count int := 0;
BEGIN
    FOR r IN
        SELECT * FROM public.holiday_rules
        WHERE (active_from IS NULL OR active_from <= p_year)
          AND (active_to   IS NULL OR active_to   >= p_year)
    LOOP
        v_date := CASE r.recurrence
            WHEN 'ONCE'                 THEN (CASE WHEN EXTRACT(YEAR FROM r.fixed_date) = p_year THEN r.fixed_date END)
            WHEN 'ANNUAL_FIXED'         THEN make_date(p_year, r.month, r.day)
            WHEN 'ANNUAL_NTH_DOW'       THEN public.fn_nth_weekday(p_year, r.month, r.weekday, r.nth)
            WHEN 'ANNUAL_EASTER_OFFSET' THEN public.fn_easter_sunday(p_year) + r.easter_offset
        END;

        IF v_date IS NULL THEN CONTINUE; END IF;

        INSERT INTO public.holiday_observances (holiday_date, holiday_name, state_code, rule_id, is_substitute)
        VALUES (v_date, r.holiday_name, r.state_code, r.id, false)
        ON CONFLICT (holiday_date, COALESCE(state_code, '*'), holiday_name) DO NOTHING;
        v_count := v_count + 1;

        -- Substitute day if it falls on a weekend (Sat ISODOW 6, Sun 7).
        IF r.observe_substitute AND EXTRACT(ISODOW FROM v_date) IN (6, 7) THEN
            v_sub := v_date + (CASE WHEN EXTRACT(ISODOW FROM v_date) = 6 THEN 2 ELSE 1 END);
            -- Bump past any day already claimed for this state (e.g. Christmas + Boxing Day overlap).
            WHILE EXISTS (
                SELECT 1 FROM public.holiday_observances h
                WHERE h.holiday_date = v_sub
                  AND COALESCE(h.state_code,'*') = COALESCE(r.state_code,'*')
            ) LOOP
                v_sub := v_sub + 1;
            END LOOP;
            INSERT INTO public.holiday_observances (holiday_date, holiday_name, state_code, rule_id, is_substitute)
            VALUES (v_sub, r.holiday_name || ' (observed)', r.state_code, r.id, true)
            ON CONFLICT (holiday_date, COALESCE(state_code, '*'), holiday_name) DO NOTHING;
            v_count := v_count + 1;
        END IF;
    END LOOP;
    RETURN v_count;
END;
$$ LANGUAGE plpgsql;

-- Auto-expand the year's holidays whenever an academic period is created.
-- Idempotent, so multiple periods in the same year are harmless.
CREATE OR REPLACE FUNCTION public.fn_materialise_holidays_for_period()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM public.fn_materialise_holidays(NEW.year);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_materialise_holidays
    AFTER INSERT ON public.academic_periods
    FOR EACH ROW EXECUTE FUNCTION public.fn_materialise_holidays_for_period();

-- =========================================================================
-- 13. SESSION GENERATION (explode slots -> concrete sessions)
-- =========================================================================
-- Set-based: one INSERT...SELECT over the period's calendar, honouring
-- per-state public holidays and class-specific exceptions. Idempotent via the
-- uq_session_natural key. Also seeds session_teachers from each slot's teacher,
-- which triggers the configured hour cap check (annual and/or per-period).
CREATE OR REPLACE FUNCTION public.fn_generate_sessions(p_class_id bigint)
RETURNS integer AS $$
DECLARE
    v_inserted integer;
    v_state varchar(3);
BEGIN
    SELECT dl.state_code INTO v_state
    FROM public.classes c
    JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
    WHERE c.id = p_class_id;

    WITH gen AS (
        INSERT INTO public.class_sessions (class_id, session_date, start_time, end_time, room_id, session_type)
        SELECT cs.class_id, d::date, cs.start_time, cs.end_time, cs.room_id, 'Scheduled'
        FROM public.class_slots cs
        JOIN public.classes c        ON c.id = cs.class_id
        JOIN public.academic_periods ap ON ap.id = c.academic_period_id
        CROSS JOIN generate_series(ap.start_date::timestamp, ap.end_date::timestamp, '1 day'::interval) d
        WHERE cs.class_id = p_class_id
          AND EXTRACT(ISODOW FROM d) = cs.day_of_week
          AND NOT EXISTS (
                SELECT 1 FROM public.holiday_observances h
                WHERE h.holiday_date = d::date
                  AND (h.state_code IS NULL OR h.state_code = v_state))
          AND NOT EXISTS (
                SELECT 1 FROM public.class_exceptions ce
                WHERE ce.class_id = cs.class_id AND ce.exception_date = d::date)
        ON CONFLICT ON CONSTRAINT uq_session_natural DO NOTHING
        RETURNING id, session_date, start_time
    )
    INSERT INTO public.session_teachers (session_id, teacher_id, role)
    SELECT g.id, cs.teacher_id, 'Lead'
    FROM gen g
    JOIN public.class_slots cs
      ON cs.class_id = p_class_id
     AND cs.day_of_week = EXTRACT(ISODOW FROM g.session_date)
     AND cs.start_time = g.start_time
    ON CONFLICT (session_id, teacher_id) DO NOTHING;

    GET DIAGNOSTICS v_inserted = ROW_COUNT;
    RETURN v_inserted;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================
-- 14. GENERIC AUDIT TRIGGER (attached to result-bearing tables)
-- =========================================================================
-- Actor is read from a per-transaction GUC the Go layer sets:
--   SET LOCAL app.current_user_id = '<app_users.id>';
CREATE OR REPLACE FUNCTION public.fn_audit()
RETURNS TRIGGER AS $$
DECLARE
    v_actor bigint;
BEGIN
    BEGIN
        v_actor := NULLIF(current_setting('app.current_user_id', true), '')::bigint;
    EXCEPTION WHEN others THEN v_actor := NULL;
    END;

    IF TG_OP = 'DELETE' THEN
        INSERT INTO public.audit_log (table_name, record_id, action, actor_id, old_data)
        VALUES (TG_TABLE_NAME, OLD.id, 'DELETE', v_actor, to_jsonb(OLD));
        RETURN OLD;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO public.audit_log (table_name, record_id, action, actor_id, old_data, new_data)
        VALUES (TG_TABLE_NAME, NEW.id, 'UPDATE', v_actor, to_jsonb(OLD), to_jsonb(NEW));
        RETURN NEW;
    ELSE
        INSERT INTO public.audit_log (table_name, record_id, action, actor_id, new_data)
        VALUES (TG_TABLE_NAME, NEW.id, 'INSERT', v_actor, to_jsonb(NEW));
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_audit_cse
    AFTER INSERT OR UPDATE OR DELETE ON public.client_subject_enrolments
    FOR EACH ROW EXECUTE FUNCTION public.fn_audit();

CREATE OR REPLACE TRIGGER trg_audit_sce
    AFTER INSERT OR UPDATE OR DELETE ON public.student_course_enrollments
    FOR EACH ROW EXECUTE FUNCTION public.fn_audit();

CREATE OR REPLACE TRIGGER trg_audit_completions
    AFTER INSERT OR UPDATE OR DELETE ON public.program_completions
    FOR EACH ROW EXECUTE FUNCTION public.fn_audit();

-- =========================================================================
-- 15. REPORTING VIEWS
-- =========================================================================

-- Annual workload overview. v11: surfaces teacher sector alongside hours.
CREATE OR REPLACE VIEW public.vw_teacher_academic_workloads AS
SELECT
    b.id AS balance_record_id,
    b.teacher_id,
    t.sector,
    b.calendar_year,
    b.booked_hours,
    b.allocated_max_hours,
    (b.allocated_max_hours - b.booked_hours) AS remaining_yearly_capacity,
    ROUND((b.booked_hours / NULLIF(b.allocated_max_hours, 0)) * 100, 1) AS pct_utilised
FROM public.teacher_yearly_balances b
JOIN public.teachers t ON t.id = b.teacher_id;

-- NEW v11: per-period workload for HE and DUAL-sector teachers.
-- Only rows with explicit period allocations appear here; VET-only teachers
-- who have no max_hours_per_period configured will not appear.
CREATE OR REPLACE VIEW public.vw_teacher_period_workloads AS
SELECT
    tpa.id AS allocation_record_id,
    tpa.teacher_id,
    t.sector,
    ap.period_code,
    ap.period_name,
    ap.period_type,
    ap.year,
    ap.sequence_number,
    ap.start_date,
    ap.end_date,
    tpa.allocated_hours,
    tpa.booked_hours,
    (tpa.allocated_hours - tpa.booked_hours) AS remaining_period_capacity,
    ROUND((tpa.booked_hours / NULLIF(tpa.allocated_hours, 0)) * 100, 1) AS pct_utilised,
    tpa.notes
FROM public.teacher_period_allocations tpa
JOIN public.teachers t ON t.id = tpa.teacher_id
JOIN public.academic_periods ap ON ap.id = tpa.academic_period_id;

-- NEW v12: workplan summary — planned hours by category, minimum CAPPS
-- required (capps_ratio × actual teaching delivery hours), and actual
-- teaching hours from teacher_yearly_balances. One row per workplan.
CREATE OR REPLACE VIEW public.vw_workplan_summary AS
SELECT
    w.id AS workplan_id,
    w.teacher_id,
    w.calendar_year,
    w.version,
    w.status,
    w.time_fraction,
    w.capps_ratio,
    w.accountable_hours_required,
    w.agreed_overtime_hours,
    COALESCE(SUM(CASE WHEN e.entry_type = 'Teaching Delivery'       THEN e.total_hours END), 0.00) AS planned_teaching_hours,
    COALESCE(SUM(CASE WHEN e.entry_type = 'CAPPS'                   THEN e.total_hours END), 0.00) AS planned_capps_hours,
    COALESCE(SUM(CASE WHEN e.entry_type = 'Education Related Duties' THEN e.total_hours END), 0.00) AS planned_erd_hours,
    COALESCE(SUM(e.total_hours), 0.00)                                                              AS planned_total_hours,
    COALESCE(tyb.booked_hours, 0.00)                                                                AS actual_teaching_hours,
    ROUND(COALESCE(tyb.booked_hours, 0.00) * w.capps_ratio, 2)                                     AS min_capps_required
FROM public.workplans w
LEFT JOIN public.workplan_entries e ON e.workplan_id = w.id
LEFT JOIN public.teacher_yearly_balances tyb
       ON tyb.teacher_id = w.teacher_id AND tyb.calendar_year = w.calendar_year
GROUP BY w.id, w.teacher_id, w.calendar_year, w.version, w.status,
         w.time_fraction, w.capps_ratio, w.accountable_hours_required,
         w.agreed_overtime_hours, tyb.booked_hours;

-- NEW v13: timesheet summary — ordinary and overtime hours by category.
-- Joins pay_periods for period dates and name; one row per timesheet.
CREATE OR REPLACE VIEW public.vw_timesheet_summary AS
SELECT
    ts.id                                                                                                      AS timesheet_id,
    ts.teacher_id,
    pp.period_start,
    pp.period_end,
    pp.period_name,
    pp.calendar_year,
    ts.status,
    ts.exported_at,
    ts.export_format,
    COALESCE(SUM(CASE WHEN te.entry_type = 'Teaching Delivery'        AND NOT te.is_overtime THEN te.hours END), 0.00) AS teaching_ordinary_hours,
    COALESCE(SUM(CASE WHEN te.entry_type = 'CAPPS'                    AND NOT te.is_overtime THEN te.hours END), 0.00) AS capps_ordinary_hours,
    COALESCE(SUM(CASE WHEN te.entry_type = 'Education Related Duties' AND NOT te.is_overtime THEN te.hours END), 0.00) AS erd_ordinary_hours,
    COALESCE(SUM(CASE WHEN te.entry_type = 'Other'                    AND NOT te.is_overtime THEN te.hours END), 0.00) AS other_ordinary_hours,
    COALESCE(SUM(CASE WHEN NOT te.is_overtime THEN te.hours END), 0.00)                                                AS ordinary_hours,
    COALESCE(SUM(CASE WHEN     te.is_overtime THEN te.hours END), 0.00)                                                AS overtime_hours,
    COALESCE(SUM(te.hours), 0.00)                                                                                      AS total_hours
FROM public.timesheets ts
JOIN  public.pay_periods pp ON pp.id = ts.pay_period_id
LEFT JOIN public.timesheet_entries te ON te.timesheet_id = ts.id
GROUP BY ts.id, ts.teacher_id, pp.period_start, pp.period_end,
         pp.period_name, pp.calendar_year, ts.status, ts.exported_at, ts.export_format;

COMMIT;

-- =========================================================================
-- v0.17 migration
-- =========================================================================
BEGIN;

-- =========================================================================
-- NEW TABLES — Employment Services
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.student_employment_services (
    student_id             bigint       NOT NULL,
    centrelink_crn         varchar(20)  NULL,
    job_seeker_id          varchar(30)  NULL,
    participation_hours    numeric(5,2) NOT NULL DEFAULT 0,
    participation_type     varchar(10)  NULL,
    participation_comment  text         NULL,
    created_at             timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at             timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (student_id),
    CONSTRAINT fk_ses_student  FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT chk_ses_type    CHECK (participation_type IN ('Full-Time','Part-Time')),
    CONSTRAINT chk_ses_hours   CHECK (participation_hours >= 0)
);

CREATE OR REPLACE TRIGGER trg_touch_employment_services
    BEFORE UPDATE ON public.student_employment_services
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

CREATE TABLE IF NOT EXISTS public.student_employment_registrations (
    id                  bigserial    NOT NULL,
    student_id          bigint       NOT NULL,
    provider_name       varchar(100) NOT NULL,
    registration_number varchar(50)  NULL,
    start_date          date         NULL,
    end_date            date         NULL,
    status              varchar(20)  NOT NULL DEFAULT 'Active',
    notes               text         NULL,
    created_at          timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_ser_student FOREIGN KEY (student_id)
        REFERENCES public.student_employment_services(student_id) ON DELETE CASCADE,
    CONSTRAINT chk_ser_status CHECK (status IN ('Active','Inactive','Suspended')),
    CONSTRAINT chk_ser_dates  CHECK (end_date IS NULL OR end_date >= start_date)
);

-- =========================================================================
-- NEW TABLES — Vocational Competency & Currency (VCC)
-- =========================================================================

-- Top-level VCC document: one per teacher per year per version.
CREATE TABLE IF NOT EXISTS public.teacher_vccs (
    id              bigserial    NOT NULL,
    teacher_id      bigint       NOT NULL,
    calendar_year   smallint     NOT NULL,
    version         smallint     NOT NULL DEFAULT 1,
    version_label   varchar(20)  NULL,           -- e.g. '2026_V1'
    status          varchar(20)  NOT NULL DEFAULT 'Draft',
    supervisor_id   bigint       NULL,            -- line manager / VCC supervisor
    approved_by_id  bigint       NULL,
    approved_at     timestamp with time zone NULL,
    notes           text         NULL,
    created_at      timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at      timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_teacher_vcc_version  UNIQUE (teacher_id, calendar_year, version),
    CONSTRAINT fk_vcc_teacher          FOREIGN KEY (teacher_id)     REFERENCES public.teachers(id)   ON DELETE RESTRICT,
    CONSTRAINT fk_vcc_supervisor       FOREIGN KEY (supervisor_id)  REFERENCES public.app_users(id)  ON DELETE SET NULL,
    CONSTRAINT fk_vcc_approved_by      FOREIGN KEY (approved_by_id) REFERENCES public.app_users(id)  ON DELETE SET NULL,
    CONSTRAINT chk_vcc_status          CHECK (status IN ('Draft','Submitted','Approved','Rejected'))
);

CREATE OR REPLACE TRIGGER trg_touch_teacher_vccs
    BEFORE UPDATE ON public.teacher_vccs
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- Teacher's own professional credentials (TAE, degrees, industry certs).
CREATE TABLE IF NOT EXISTS public.teacher_vcc_professional_qualifications (
    id                   bigserial    NOT NULL,
    vcc_id               bigint       NOT NULL,
    qualification_code   varchar(30)  NOT NULL,
    qualification_title  varchar(200) NOT NULL,
    institution          varchar(200) NULL,
    aqf_level            smallint     NULL,
    status               varchar(20)  NOT NULL DEFAULT 'Draft',
    approved_at          date         NULL,
    notes                text         NULL,
    created_at           timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vcc_pq_vcc    FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE,
    CONSTRAINT chk_vcc_pq_status CHECK (status IN ('Draft','Pending','Approved','Rejected')),
    CONSTRAINT chk_vcc_pq_aqf    CHECK (aqf_level BETWEEN 1 AND 10)
);

-- Industry/AQF qualifications declared in a VCC (separate from TAE teaching quals).
CREATE TABLE IF NOT EXISTS public.teacher_vcc_vocational_qualifications (
    id                   bigserial    NOT NULL,
    vcc_id               bigint       NOT NULL,
    qualification_code   varchar(30)  NOT NULL,
    qualification_title  varchar(200) NOT NULL,
    institution          varchar(200) NULL,
    aqf_level            smallint     NULL,
    status               varchar(20)  NOT NULL DEFAULT 'Draft',
    approved_at          date         NULL,
    notes                text         NULL,
    created_at           timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vcc_vocqual_vcc    FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE,
    CONSTRAINT chk_vcc_vocqual_status CHECK (status IN ('Draft','Pending','Approved','Rejected')),
    CONSTRAINT chk_vcc_vocqual_aqf    CHECK (aqf_level BETWEEN 1 AND 10)
);

-- Courses (qualifications) the teacher is mapped to deliver in this VCC.
CREATE TABLE IF NOT EXISTS public.teacher_vcc_courses (
    id           bigserial    NOT NULL,
    vcc_id       bigint       NOT NULL,
    program_id   bigint       NULL,          -- NULL for non-TGA courses (VIC codes etc.)
    course_code  varchar(20)  NOT NULL,
    course_title varchar(200) NOT NULL,
    sort_order   smallint     NOT NULL DEFAULT 0,
    created_at   timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vcc_course_vcc     FOREIGN KEY (vcc_id)     REFERENCES public.teacher_vccs(id) ON DELETE CASCADE,
    CONSTRAINT fk_vcc_course_program FOREIGN KEY (program_id) REFERENCES public.programs(id)     ON DELETE SET NULL
);

-- Units the teacher has currency for. Multiple rows per unit are allowed
-- (one per competency method, matching TGA VCC practice).
CREATE TABLE IF NOT EXISTS public.teacher_vcc_units (
    id                    bigserial    NOT NULL,
    vcc_id                bigint       NOT NULL,
    vcc_course_id         bigint       NULL,          -- NULL = standalone "Single Unit"
    subject_id            bigint       NULL,
    unit_code             varchar(20)  NOT NULL,
    unit_title            varchar(200) NOT NULL,
    competency_method     varchar(60)  NOT NULL,
    superseded_unit_code  varchar(20)  NULL,
    superseded_unit_title varchar(200) NULL,
    description           text         NULL,          -- study name or employer/dates (method-dependent)
    justification         text         NULL,
    status                varchar(20)  NOT NULL DEFAULT 'Pending',
    approved_at           date         NULL,
    sort_order            smallint     NOT NULL DEFAULT 0,
    created_at            timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at            timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vcc_unit_vcc     FOREIGN KEY (vcc_id)        REFERENCES public.teacher_vccs(id)                      ON DELETE CASCADE,
    CONSTRAINT fk_vcc_unit_course  FOREIGN KEY (vcc_course_id) REFERENCES public.teacher_vcc_courses(id)               ON DELETE SET NULL,
    CONSTRAINT fk_vcc_unit_subject FOREIGN KEY (subject_id)    REFERENCES public.subjects(id)                          ON DELETE SET NULL,
    CONSTRAINT chk_vcc_unit_method CHECK (competency_method IN (
        'I hold the current unit of competency',
        'I hold a superseded and equivalent unit of competency',
        'I hold a recognition of relevant study',
        'I have vocational work experience',
        'Other'
    )),
    CONSTRAINT chk_vcc_unit_status CHECK (status IN ('Draft','Pending','Approved','Rejected'))
);

CREATE OR REPLACE TRIGGER trg_touch_teacher_vcc_units
    BEFORE UPDATE ON public.teacher_vcc_units
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- =========================================================================
-- NEW TABLES — Teacher Document Library
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.teacher_documents (
    id               bigserial     NOT NULL,
    teacher_id       bigint        NOT NULL,
    title            varchar(200)  NOT NULL,
    file_category    varchar(30)   NOT NULL DEFAULT 'Other',
    year_of_document smallint      NULL,
    document_url     varchar(2048) NULL,
    external_url     varchar(2048) NULL,
    file_name        varchar(255)  NULL,
    uploaded_at      timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    created_at       timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_td_teacher   FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE,
    CONSTRAINT chk_td_category CHECK (file_category IN (
        'Testamurs','Accreditations','Registrations','Statement of attainment',
        'Transcripts','Credentials','Licenses','Job cards','Other'
    ))
);

-- Links a document to exactly one VCC entity. vcc_currency_activity_id FK is
-- added after teacher_currency_activities is created below.
CREATE TABLE IF NOT EXISTS public.teacher_document_connections (
    id                       bigserial NOT NULL,
    document_id              bigint    NOT NULL,
    vcc_professional_qual_id bigint    NULL,
    vcc_vocational_qual_id   bigint    NULL,
    vcc_unit_id              bigint    NULL,
    vcc_currency_activity_id bigint    NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_tdc_document  FOREIGN KEY (document_id)              REFERENCES public.teacher_documents(id)                        ON DELETE CASCADE,
    CONSTRAINT fk_tdc_pq        FOREIGN KEY (vcc_professional_qual_id) REFERENCES public.teacher_vcc_professional_qualifications(id)   ON DELETE CASCADE,
    CONSTRAINT fk_tdc_vocqual   FOREIGN KEY (vcc_vocational_qual_id)   REFERENCES public.teacher_vcc_vocational_qualifications(id)     ON DELETE CASCADE,
    CONSTRAINT fk_tdc_unit      FOREIGN KEY (vcc_unit_id)              REFERENCES public.teacher_vcc_units(id)                        ON DELETE CASCADE,
    CONSTRAINT chk_tdc_target   CHECK (num_nonnulls(vcc_professional_qual_id, vcc_vocational_qual_id, vcc_unit_id, vcc_currency_activity_id) = 1)
);

-- =========================================================================
-- NEW TABLES — Currency Activities
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.teacher_currency_activities (
    id                       bigserial    NOT NULL,
    teacher_id               bigint       NOT NULL,
    currency_type            varchar(15)  NOT NULL,    -- 'Vocational' | 'Professional'
    is_external              boolean      NOT NULL DEFAULT true,
    activity_type            varchar(50)  NOT NULL DEFAULT 'Other',
    activity_name            varchar(200) NOT NULL,
    date_of_activity         date         NOT NULL,
    date_approved            date         NULL,
    points_awarded           smallint     NOT NULL DEFAULT 0,
    duration_hours           numeric(5,2) NULL,
    inform_teaching_practice text         NULL,
    student_benefit          text         NULL,
    approval_reason          text         NULL,
    status                   varchar(20)  NOT NULL DEFAULT 'Pending',
    -- Professional currency fields
    domain_name              varchar(100) NULL,
    program_type             varchar(50)  NULL,
    program_name             varchar(200) NULL,
    program_date             date         NULL,
    workshop_count           smallint     NULL,
    program_summary          text         NULL,
    created_at               timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at               timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_tca_teacher        FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT,
    CONSTRAINT chk_tca_currency_type CHECK (currency_type IN ('Vocational','Professional')),
    CONSTRAINT chk_tca_status        CHECK (status IN ('Draft','Pending','Approved','Rejected')),
    CONSTRAINT chk_tca_points        CHECK (points_awarded >= 0)
);

-- Wire up the deferred FK now that teacher_currency_activities exists.
ALTER TABLE public.teacher_document_connections
    ADD CONSTRAINT fk_tdc_currency FOREIGN KEY (vcc_currency_activity_id)
    REFERENCES public.teacher_currency_activities(id) ON DELETE CASCADE;

CREATE OR REPLACE TRIGGER trg_touch_teacher_currency_activities
    BEFORE UPDATE ON public.teacher_currency_activities
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

-- Units referenced by a vocational currency activity ("Related Unit/s").
CREATE TABLE IF NOT EXISTS public.teacher_currency_unit_links (
    id                   bigserial    NOT NULL,
    currency_activity_id bigint       NOT NULL,
    subject_id           bigint       NULL,
    unit_code            varchar(20)  NOT NULL,
    unit_title           varchar(200) NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_currency_unit UNIQUE (currency_activity_id, unit_code),
    CONSTRAINT fk_tcul_activity FOREIGN KEY (currency_activity_id) REFERENCES public.teacher_currency_activities(id) ON DELETE CASCADE,
    CONSTRAINT fk_tcul_subject  FOREIGN KEY (subject_id)           REFERENCES public.subjects(id)                   ON DELETE SET NULL
);

-- =========================================================================
-- NEW TABLES — VCC Profiling Tool
-- =========================================================================

-- Stores spider/radar chart scores per dimension per VCC version.
CREATE TABLE IF NOT EXISTS public.teacher_vcc_profiling (
    vcc_id               bigint       NOT NULL,
    dimension            varchar(100) NOT NULL,
    business_ideal_score smallint     NULL,
    self_score           smallint     NULL,
    supervisor_score     smallint     NULL,
    PRIMARY KEY (vcc_id, dimension),
    CONSTRAINT fk_vcc_profiling FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE
);

-- =========================================================================
-- INDEXES — v0.17
-- =========================================================================

CREATE INDEX IF NOT EXISTS idx_guardian_emergency
    ON public.student_guardians(student_id) WHERE (is_emergency_contact = true);

CREATE INDEX IF NOT EXISTS idx_attendance_absence_reason
    ON public.session_attendance(absence_reason) WHERE (absence_reason IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_subject_programs_core
    ON public.subject_programs(program_id, is_core);

CREATE INDEX IF NOT EXISTS idx_ses_student
    ON public.student_employment_services(student_id);

CREATE INDEX IF NOT EXISTS idx_ser_student
    ON public.student_employment_registrations(student_id);

CREATE INDEX IF NOT EXISTS idx_programs_type
    ON public.programs(program_type) WHERE (program_type IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_teacher_vccs_teacher
    ON public.teacher_vccs(teacher_id);

CREATE INDEX IF NOT EXISTS idx_teacher_vccs_year
    ON public.teacher_vccs(calendar_year);

CREATE INDEX IF NOT EXISTS idx_vcc_pq_vcc
    ON public.teacher_vcc_professional_qualifications(vcc_id);

CREATE INDEX IF NOT EXISTS idx_vcc_courses_vcc
    ON public.teacher_vcc_courses(vcc_id);

CREATE INDEX IF NOT EXISTS idx_vcc_units_vcc
    ON public.teacher_vcc_units(vcc_id);

CREATE INDEX IF NOT EXISTS idx_vcc_units_course
    ON public.teacher_vcc_units(vcc_course_id) WHERE (vcc_course_id IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_vcc_units_subject
    ON public.teacher_vcc_units(subject_id) WHERE (subject_id IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_teacher_documents_teacher
    ON public.teacher_documents(teacher_id);

CREATE INDEX IF NOT EXISTS idx_tdc_document
    ON public.teacher_document_connections(document_id);

CREATE INDEX IF NOT EXISTS idx_teacher_currency_teacher
    ON public.teacher_currency_activities(teacher_id);

CREATE INDEX IF NOT EXISTS idx_teacher_currency_type_status
    ON public.teacher_currency_activities(teacher_id, currency_type, status);

CREATE INDEX IF NOT EXISTS idx_tcul_activity
    ON public.teacher_currency_unit_links(currency_activity_id);

CREATE INDEX IF NOT EXISTS idx_plp_person
    ON public.person_location_preferences(person_id);

CREATE INDEX IF NOT EXISTS idx_lab_software_room
    ON public.room_lab_software(room_id);

CREATE INDEX IF NOT EXISTS idx_room_issues_room
    ON public.room_issues(room_id);

CREATE INDEX IF NOT EXISTS idx_room_issues_status
    ON public.room_issues(room_id, status);

-- =========================================================================
-- Migration: v0.31 → v0.32  (also safe on a fresh v0.32 database)
-- =========================================================================
ALTER TABLE public.teacher_vcc_professional_qualifications ADD COLUMN IF NOT EXISTS aqf_level smallint NULL;
ALTER TABLE public.teacher_vcc_vocational_qualifications   ADD COLUMN IF NOT EXISTS aqf_level smallint NULL;
ALTER TABLE public.teacher_vcc_professional_qualifications DROP CONSTRAINT IF EXISTS chk_vcc_pq_aqf;
ALTER TABLE public.teacher_vcc_professional_qualifications ADD CONSTRAINT chk_vcc_pq_aqf CHECK (aqf_level BETWEEN 1 AND 10);
ALTER TABLE public.teacher_vcc_vocational_qualifications   DROP CONSTRAINT IF EXISTS chk_vcc_vocqual_aqf;
ALTER TABLE public.teacher_vcc_vocational_qualifications   ADD CONSTRAINT chk_vcc_vocqual_aqf CHECK (aqf_level BETWEEN 1 AND 10);

-- Migration: v0.30 → v0.31  (also safe on a fresh v0.31 database)
-- =========================================================================
-- Step 1: create the new vocational qualifications table (IF NOT EXISTS — safe on fresh DB)
CREATE TABLE IF NOT EXISTS public.teacher_vcc_vocational_qualifications (
    id bigserial NOT NULL, vcc_id bigint NOT NULL,
    qualification_code varchar(30) NOT NULL, qualification_title varchar(200) NOT NULL,
    institution varchar(200) NULL, status varchar(20) NOT NULL DEFAULT 'Draft',
    approved_at date NULL, notes text NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_vcc_vocqual_vcc    FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE,
    CONSTRAINT chk_vcc_vocqual_status CHECK (status IN ('Draft','Pending','Approved','Rejected'))
);
-- Step 2: add new FK column to connections (IF NOT EXISTS — safe on fresh DB)
ALTER TABLE public.teacher_document_connections
  ADD COLUMN IF NOT EXISTS vcc_vocational_qual_id bigint NULL;
-- Steps 3–4: data migration — only needed when upgrading from v0.30 where qual_type existed
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name   = 'teacher_vcc_professional_qualifications'
          AND column_name  = 'qual_type'
    ) THEN
        INSERT INTO public.teacher_vcc_vocational_qualifications
               (id, vcc_id, qualification_code, qualification_title, institution, status, approved_at, notes, created_at)
        SELECT  id, vcc_id, qualification_code, qualification_title, institution, status, approved_at, notes, created_at
        FROM public.teacher_vcc_professional_qualifications WHERE qual_type = 'Vocational';

        UPDATE public.teacher_document_connections
          SET vcc_vocational_qual_id = vcc_professional_qual_id,
              vcc_professional_qual_id = NULL
          WHERE vcc_professional_qual_id IN (
            SELECT id FROM public.teacher_vcc_professional_qualifications WHERE qual_type = 'Vocational'
          );

        DELETE FROM public.teacher_vcc_professional_qualifications WHERE qual_type = 'Vocational';
        EXECUTE 'ALTER TABLE public.teacher_vcc_professional_qualifications DROP COLUMN IF EXISTS qual_type';
    END IF;
END $$;
-- Step 5: FK and check constraints — DROP IF EXISTS then ADD, so safe on both fresh and upgrade
ALTER TABLE public.teacher_document_connections DROP CONSTRAINT IF EXISTS fk_tdc_vocqual;
ALTER TABLE public.teacher_document_connections
  ADD CONSTRAINT fk_tdc_vocqual FOREIGN KEY (vcc_vocational_qual_id)
    REFERENCES public.teacher_vcc_vocational_qualifications(id) ON DELETE CASCADE;
ALTER TABLE public.teacher_document_connections DROP CONSTRAINT IF EXISTS chk_tdc_target;
ALTER TABLE public.teacher_document_connections
  ADD CONSTRAINT chk_tdc_target CHECK (
    num_nonnulls(vcc_professional_qual_id, vcc_vocational_qual_id, vcc_unit_id, vcc_currency_activity_id) = 1
  );

-- =========================================================================
-- Migration: v0.29 → v0.30  (superseded by v0.31 — skip if going straight to v0.31)
-- =========================================================================

-- =========================================================================
-- Migration: v0.28 → v0.29  (run on existing databases)
-- =========================================================================
ALTER TABLE public.teacher_documents ALTER COLUMN document_url DROP NOT NULL;
ALTER TABLE public.teacher_documents ALTER COLUMN file_name    DROP NOT NULL;

COMMIT;
