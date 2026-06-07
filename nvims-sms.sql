-- =========================================================================
-- AVETMISS-compliant SMS schema  --  v9
-- =========================================================================
-- Changes from v8 (high-priority fixes):
--   1.  Shared-PK identity: people holds the PK; students/teachers/staff/
--       app_users carry that same id as PK *and* FK to people.
--   2.  Teaching-hours model reworked to derive from class_sessions +
--       session_teachers (the real occurrences), not the weekly class_slots
--       template. Fixes the per-week undercount and counts team teaching.
--   3.  Exclusion constraints on class_slots scoped by academic_period_id so
--       the same weekday/time in different terms is no longer a false clash.
--       Added session-level teacher double-booking protection.
--   4.  Missing FKs added (faculty links, all *_by columns -> app_users).
--   5.  app_users (auth/actor) + generic audit_log added.
--   6.  num_nonnulls() guard on message_deliveries; partial unique on
--       class_support_staff for class-wide rows.
--   7.  australian_states.avetmiss_state_id (numeric NAT code) added.
--   8.  students.highest_school_level_id / year added (NAT00080).
--   9.  client_subject_enrolments.student_course_enrollment_id now NULLABLE
--       to support standalone unit enrolments (NAT00120 blank program).
--  10.  Soft-delete columns + RESTRICT on the enrolment cascade chain so
--       reportable history can't be silently deleted.
--  11.  fn_generate_sessions(class_id) added (explodes slots -> sessions).
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

CREATE TABLE IF NOT EXISTS public.academic_periods (
    id bigserial NOT NULL,
    period_code varchar(20) NOT NULL,
    year smallint NOT NULL,
    period_name varchar(50) NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    period_type varchar(10) NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_academic_period_code UNIQUE (period_code),
    CONSTRAINT chk_period_type CHECK (period_type IN ('TERM', 'SEMESTER', 'TRIMESTER', 'YEAR')),
    CONSTRAINT chk_period_dates CHECK (end_date >= start_date)
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

-- NEW: small fixed lookup for NAT00080 "highest school level completed"
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
    -- v8 'phone' legacy fall-through column removed (redundant with the three above)
    emergency_contact_name varchar(100) NULL,
    emergency_contact_phone varchar(15) NULL,
    emergency_contact_relationship varchar(30) NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_people_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_people_email UNIQUE (primary_email),
    CONSTRAINT chk_people_email CHECK (primary_email LIKE '%@%.%'),
    CONSTRAINT chk_avetmiss_gender CHECK (gender IN ('M', 'F', 'X')),
    CONSTRAINT chk_postcode_format CHECK (postcode ~ '^[0-9]{4}$')
);

-- NEW: authentication / system-actor accounts. Every *_by column FKs here.
CREATE TABLE IF NOT EXISTS public.app_users (
    id bigserial NOT NULL,
    person_id bigint NULL,                       -- NULL allowed for service accounts
    username varchar(100) NOT NULL,
    role varchar(30) NOT NULL DEFAULT 'Staff',
    is_active boolean NOT NULL DEFAULT true,
    last_login_at timestamp with time zone NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_app_user_username UNIQUE (username),
    CONSTRAINT fk_app_user_person FOREIGN KEY (person_id) REFERENCES public.people(id) ON DELETE SET NULL,
    CONSTRAINT chk_app_user_role CHECK (role IN ('Admin','Trainer','Compliance','Reception','SupportStaff','System','Staff'))
);

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
    highest_school_level_id varchar(2) NULL,            -- NEW (NAT00080)
    year_highest_school_completed smallint NULL,        -- NEW (NAT00080)
    disability_flag varchar(1) NOT NULL DEFAULT 'N',
    prior_educational_achievement_flag varchar(1) NOT NULL DEFAULT 'N',
    secondary_school_id bigint NULL,
    state_allocated_student_number varchar(20) NULL,
    state_identity_issuing_body_code varchar(3) NULL,
    at_school_flag varchar(1) NOT NULL DEFAULT 'N',
    photo_url varchar(2048) NULL,                       -- external object storage (signed URL/CDN), not base64
    photo_uploaded_at timestamp with time zone NULL,
    id_expiry_date date NULL,
    id_document_type varchar(50) NULL,
    id_document_number varchar(50) NULL,
    deleted_at timestamp with time zone NULL,           -- NEW soft-delete
    deleted_by bigint NULL,                             -- NEW
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_students_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_student_state_body FOREIGN KEY (state_identity_issuing_body_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT fk_student_school FOREIGN KEY (secondary_school_id) REFERENCES public.secondary_schools(id) ON DELETE SET NULL,
    CONSTRAINT fk_student_school_level FOREIGN KEY (highest_school_level_id) REFERENCES public.highest_school_levels(level_id),
    CONSTRAINT fk_student_deleted_by FOREIGN KEY (deleted_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT uq_students_number UNIQUE (student_number),
    CONSTRAINT uq_students_email UNIQUE (student_email),
    CONSTRAINT uq_students_usi UNIQUE (usi),
    CONSTRAINT chk_usi_length CHECK (usi IS NULL OR length(usi) = 10),
    CONSTRAINT chk_state_student_num_len CHECK (state_allocated_student_number IS NULL OR length(state_allocated_student_number) BETWEEN 5 AND 20),
    CONSTRAINT chk_avetmiss_indigenous CHECK (indigenous_status_id IN ('1', '2', '3', '4', '9', '@')),
    CONSTRAINT chk_disability_flag CHECK (disability_flag IN ('Y', 'N')),
    CONSTRAINT chk_prior_ed_flag CHECK (prior_educational_achievement_flag IN ('Y', 'N')),
    CONSTRAINT chk_english_proficiency CHECK (english_proficiency_id IN ('1', '2', '3', '4', '@')),
    CONSTRAINT chk_at_school_flag CHECK (at_school_flag IN ('Y', 'N'))
);

CREATE TABLE IF NOT EXISTS public.teachers (
    id bigint NOT NULL,                          -- shared PK: this IS people.id
    faculty_id bigint NULL,
    teacher_number varchar(20) NOT NULL,
    teacher_email varchar(100) NOT NULL,
    teacher_phone varchar(15) NULL,
    employment_status public.employment_type NOT NULL DEFAULT 'Casual',
    default_max_hours_per_year numeric(6,2) NOT NULL DEFAULT 800.00,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_teachers_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_teachers_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL,
    CONSTRAINT uq_teachers_number UNIQUE (teacher_number),
    CONSTRAINT uq_teachers_email UNIQUE (teacher_email),
    CONSTRAINT chk_teacher_max_hours CHECK (default_max_hours_per_year > 0)
);

-- Reworked: a maintained per-year cache of teaching hours, sourced from sessions.
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
    CONSTRAINT chk_balance_cap CHECK (booked_hours <= allocated_max_hours)  -- hard 800h backstop
);

CREATE TABLE IF NOT EXISTS public.staff (
    id bigint NOT NULL,                          -- shared PK: this IS people.id
    faculty_id bigint NULL,
    staff_number varchar(20) NOT NULL,
    staff_email varchar(100) NOT NULL,
    staff_phone varchar(15) NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_staff_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE,
    CONSTRAINT fk_staff_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL,
    CONSTRAINT uq_staff_number UNIQUE (staff_number),
    CONSTRAINT uq_staff_email UNIQUE (staff_email)
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
    CONSTRAINT fk_stud_ach_student FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE CASCADE,
    CONSTRAINT fk_stud_ach_type FOREIGN KEY (achievement_id) REFERENCES public.prior_educational_achievements (achievement_id) ON DELETE RESTRICT
);

-- =========================================================================
-- 3. ACADEMIC CURRICULUM
-- =========================================================================

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
    PRIMARY KEY (id),
    CONSTRAINT fk_programs_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE RESTRICT,
    CONSTRAINT uq_programs_code UNIQUE (program_code)
);

CREATE TABLE IF NOT EXISTS public.subjects (
    id bigserial NOT NULL,
    subject_code varchar(30) NOT NULL,
    subject_name varchar(100) NOT NULL,
    module_flag varchar(1) NOT NULL DEFAULT 'N',
    field_of_education varchar(6) NOT NULL,
    nominal_hours integer CHECK (nominal_hours > 0),
    vet_flag boolean NOT NULL DEFAULT true,
    PRIMARY KEY (id),
    CONSTRAINT uq_subjects_code UNIQUE (subject_code),
    CONSTRAINT chk_module_flag CHECK (module_flag IN ('Y', 'N'))
);

CREATE TABLE IF NOT EXISTS public.subject_programs (
    subject_id bigint NOT NULL,
    program_id bigint NOT NULL,
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
    id bigserial NOT NULL,
    training_org_id bigint NOT NULL,
    delivery_loc_id varchar(30) NOT NULL,
    name varchar(100) NOT NULL,
    address text NOT NULL,
    suburb varchar(50) NOT NULL,
    state_code varchar(3) NOT NULL,
    postcode varchar(4) NOT NULL,
    postcode_override varchar(4) NULL,
    country_id varchar(4) NOT NULL DEFAULT '1101',
    PRIMARY KEY (id),
    CONSTRAINT fk_loc_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT uq_delivery_loc_per_org UNIQUE (training_org_id, delivery_loc_id)
);

CREATE TABLE IF NOT EXISTS public.buildings (
    id bigserial NOT NULL,
    delivery_location_id bigint NOT NULL,
    building_name varchar(50) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT uq_building_per_campus UNIQUE (delivery_location_id, building_name)
);

CREATE TABLE IF NOT EXISTS public.rooms (
    id bigserial NOT NULL,
    building_id bigint NOT NULL,
    room_name varchar(50) NOT NULL,
    capacity integer NOT NULL,
    room_type varchar(30) NOT NULL DEFAULT 'Classroom',
    is_active boolean NOT NULL DEFAULT true,
    PRIMARY KEY (id),
    CONSTRAINT uq_room_per_building UNIQUE (building_id, room_name),
    CONSTRAINT chk_rooms_capacity CHECK (capacity > 0)
);

-- =========================================================================
-- 5. PROGRESSION & ENROLMENT
-- =========================================================================

CREATE TABLE IF NOT EXISTS public.student_course_enrollments (
    id bigserial NOT NULL,
    student_id bigint NOT NULL,
    program_id bigint NOT NULL,
    enrollment_status varchar(20) NOT NULL DEFAULT 'Active',
    commencement_date date NOT NULL,
    commencing_program_id varchar(1) NOT NULL DEFAULT '3',
    completion_date date NULL,
    funding_state_code varchar(3) NOT NULL DEFAULT 'VIC',
    training_contract_id varchar(20) NULL,
    client_apprenticeship_id varchar(20) NULL,
    deleted_at timestamp with time zone NULL,           -- NEW soft-delete
    deleted_by bigint NULL,                             -- NEW
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_enrollment_state FOREIGN KEY (funding_state_code) REFERENCES public.australian_states(state_code),
    CONSTRAINT fk_sce_deleted_by FOREIGN KEY (deleted_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT chk_enrollment_status CHECK (enrollment_status IN ('Active', 'Deferred', 'Suspended', 'Cancelled', 'Completed')),
    CONSTRAINT chk_commencing_program_id CHECK (commencing_program_id IN ('3', '4', '8'))
);
-- NOTE: state-specific funding attributes (Skills First, Smart & Skilled, etc.)
-- moved out of this national table into state_funding_details below.

CREATE UNIQUE INDEX IF NOT EXISTS idx_uq_active_course_enrollment
ON public.student_course_enrollments(student_id, program_id)
WHERE (enrollment_status IN ('Active', 'Deferred', 'Suspended'));

-- NEW: state-specific funding attributes split off the national enrolment table
CREATE TABLE IF NOT EXISTS public.state_funding_details (
    id bigserial NOT NULL,
    student_course_enrollment_id bigint NOT NULL,
    state_code varchar(3) NOT NULL,
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,   -- e.g. {"vic_skills_first_eligible": true, "nsw_commitment_id": "..."}
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
    PRIMARY KEY (id),                                   -- now 1-to-many (multiple census dates)
    CONSTRAINT fk_vsl_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT uq_vsl_enrol_census UNIQUE (student_course_enrollment_id, census_date),
    CONSTRAINT chk_vsl_type CHECK (loan_type IN ('VSL', 'VET-FEE-HELP')),
    CONSTRAINT chk_vsl_amount CHECK (loan_amount >= 0)
);

CREATE TABLE IF NOT EXISTS public.he_enrolment_details (
    student_course_enrollment_id bigint NOT NULL,
    eftsl numeric(5,4) NOT NULL,
    census_date date NOT NULL,
    hecs_help_eligible boolean NOT NULL DEFAULT false,
    fee_type varchar(20) NULL,
    study_load_category varchar(20) NULL,
    mode_of_attendance varchar(30) NULL,
    basis_for_admission varchar(10) NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (student_course_enrollment_id),
    CONSTRAINT fk_he_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE,
    CONSTRAINT chk_he_fee_type CHECK (fee_type IN ('HECS-HELP', 'FEE-HELP', 'DOMESTIC-FULL', 'INTERNATIONAL', 'EXEMPT')),
    CONSTRAINT chk_he_load CHECK (study_load_category IN ('Full-Time', 'Part-Time', 'Less Than Half-Time')),
    CONSTRAINT chk_he_mode CHECK (mode_of_attendance IN ('Internal', 'External', 'Multi-Modal'))
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
    id bigserial NOT NULL,
    class_code varchar(50) NOT NULL,
    academic_period_id bigint NOT NULL,
    delivery_location_id bigint NOT NULL,
    enrolment_cap integer NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT uq_class_code UNIQUE (class_code),
    CONSTRAINT fk_class_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id),
    CONSTRAINT fk_class_location FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id),
    CONSTRAINT chk_class_cap CHECK (enrolment_cap > 0)
);

-- The set of subjects/units a class delivers. (v8 called this 'clusters'.)
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
    academic_period_id bigint NOT NULL,             -- NEW: denormalised from class, auto-set by trigger
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
    recorded_by bigint NULL,
    recorded_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT fk_attendance_session FOREIGN KEY (session_id) REFERENCES public.class_sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_recorder FOREIGN KEY (recorded_by) REFERENCES public.app_users(id) ON DELETE SET NULL,
    CONSTRAINT uq_attendance_student_per_session UNIQUE (session_id, student_id),
    CONSTRAINT chk_attendance_status CHECK (status IN ('Present', 'Absent-Notified', 'Absent-Unnotified', 'Online', 'Excused')),
    CONSTRAINT chk_minutes_attended CHECK (minutes_attended >= 0)
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
-- Dedupe class-wide (student_id IS NULL) support rows, which the unique above can't.
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
-- COALESCE sentinel makes the uniqueness work for national (NULL state) rows too,
-- and gives ON CONFLICT a concrete target.
CREATE UNIQUE INDEX IF NOT EXISTS uq_observance
    ON public.holiday_observances (holiday_date, COALESCE(state_code, '*'), holiday_name);
CREATE INDEX IF NOT EXISTS idx_observance_date ON public.holiday_observances (holiday_date);

-- Seed the clearly-national, predictable holidays. State-specific ones
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
    training_org_id bigint NULL,                        -- NEW (NAT00130 carries TOID)
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
    document_url varchar(2048) NOT NULL,                -- external object storage, not inline blobs
    uploaded_by bigint NULL,
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
    CONSTRAINT chk_note_type CHECK (note_type IN ('General', 'Pastoral', 'Academic', 'Financial', 'Compliance', 'LAP', 'Incident'))
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
    -- NEW: exactly one recipient relation is set, so Go can switch on it safely.
    CONSTRAINT chk_delivery_one_recipient CHECK (num_nonnulls(student_id, guardian_id, staff_id) = 1)
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

-- NEW: generic append-only audit trail
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
ALTER TABLE IF EXISTS public.rooms                    ADD CONSTRAINT fk_room_parent    FOREIGN KEY (building_id) REFERENCES public.buildings(id) ON DELETE CASCADE;
ALTER TABLE IF EXISTS public.learning_access_plans    ADD CONSTRAINT fk_lap_assessor   FOREIGN KEY (assessor_id) REFERENCES public.staff(id) ON DELETE RESTRICT;

-- =========================================================================
-- 10. INDEXES
-- =========================================================================
CREATE INDEX IF NOT EXISTS idx_class_enrollments_cse ON public.class_enrollments(client_subject_enrolment_id);
CREATE INDEX IF NOT EXISTS idx_slots_period_teacher  ON public.class_slots(academic_period_id, teacher_id);
CREATE INDEX IF NOT EXISTS idx_students_usi          ON public.students(usi) WHERE (usi IS NOT NULL);
CREATE INDEX IF NOT EXISTS idx_students_number       ON public.students(student_number);
CREATE INDEX IF NOT EXISTS idx_cse_student           ON public.client_subject_enrolments(student_id);
CREATE INDEX IF NOT EXISTS idx_cse_subject           ON public.client_subject_enrolments(subject_id);
CREATE INDEX IF NOT EXISTS idx_cse_outcome           ON public.client_subject_enrolments(outcome_id_national);
CREATE INDEX IF NOT EXISTS idx_cse_delivery_loc      ON public.client_subject_enrolments(delivery_location_id);
CREATE INDEX IF NOT EXISTS idx_sce_program           ON public.student_course_enrollments(program_id);
CREATE INDEX IF NOT EXISTS idx_sce_status            ON public.student_course_enrollments(enrollment_status);
CREATE INDEX IF NOT EXISTS idx_pc_program            ON public.program_completions(program_id);
CREATE INDEX IF NOT EXISTS idx_sessions_date         ON public.class_sessions(session_date);
CREATE INDEX IF NOT EXISTS idx_sessions_class        ON public.class_sessions(class_id);
CREATE INDEX IF NOT EXISTS idx_session_teachers_teacher ON public.session_teachers(teacher_id);
CREATE INDEX IF NOT EXISTS idx_attendance_session    ON public.session_attendance(session_id);
CREATE INDEX IF NOT EXISTS idx_attendance_student    ON public.session_attendance(student_id);
CREATE INDEX IF NOT EXISTS idx_student_notes_student ON public.student_notes(student_id);
CREATE INDEX IF NOT EXISTS idx_notes_type            ON public.student_notes(note_type);

-- =========================================================================
-- 11. COMPLIANCE & TEACHING-HOURS ENGINE (sessions are the source of truth)
-- =========================================================================

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

-- Single point of truth for adjusting a teacher's yearly balance, with the
-- 800h cap enforced here (friendly error) and by chk_balance_cap (hard backstop).
-- FOR UPDATE serialises concurrent writers; the prior UPSERT guarantees the row
-- exists, closing the v8 race where the first slot of the year locked nothing.
CREATE OR REPLACE FUNCTION public.fn_adjust_teacher_balance(p_teacher bigint, p_year smallint, p_delta numeric)
RETURNS void AS $$
DECLARE
    v_current numeric(7,2);
    v_max numeric(6,2);
BEGIN
    IF p_delta = 0 THEN RETURN; END IF;

    INSERT INTO public.teacher_yearly_balances (teacher_id, calendar_year, allocated_max_hours, booked_hours)
    VALUES (p_teacher, p_year,
            COALESCE((SELECT default_max_hours_per_year FROM public.teachers WHERE id = p_teacher), 800.00),
            0.00)
    ON CONFLICT (teacher_id, calendar_year) DO NOTHING;

    SELECT booked_hours, allocated_max_hours INTO v_current, v_max
    FROM public.teacher_yearly_balances
    WHERE teacher_id = p_teacher AND calendar_year = p_year
    FOR UPDATE;

    IF p_delta > 0 AND (v_current + p_delta) > v_max THEN
        RAISE EXCEPTION 'Teacher % would reach %.2f teaching hours in %, exceeding the % cap by %.2f.',
            p_teacher, (v_current + p_delta), p_year, v_max, ((v_current + p_delta) - v_max);
    END IF;

    UPDATE public.teacher_yearly_balances
    SET booked_hours = GREATEST(booked_hours + p_delta, 0.00),
        updated_at = CURRENT_TIMESTAMP
    WHERE teacher_id = p_teacher AND calendar_year = p_year;
END;
$$ LANGUAGE plpgsql;

-- Hours contribution of one (session, teacher) pair. Guest roles do not accrue.
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
    v_sign int;
BEGIN
    IF TG_OP = 'INSERT' THEN
        v_role := NEW.role; v_teacher := NEW.teacher_id; v_sign := 1;
        SELECT session_date, start_time, end_time, cancelled INTO v_date, v_start, v_end, v_cancelled
        FROM public.class_sessions WHERE id = NEW.session_id;
    ELSE
        v_role := OLD.role; v_teacher := OLD.teacher_id; v_sign := -1;
        SELECT session_date, start_time, end_time, cancelled INTO v_date, v_start, v_end, v_cancelled
        FROM public.class_sessions WHERE id = OLD.session_id;
    END IF;

    IF v_role = 'Guest' OR COALESCE(v_cancelled, true) THEN
        RETURN NULL;
    END IF;

    v_hours := EXTRACT(EPOCH FROM (v_end - v_start)) / 3600.00;
    PERFORM public.fn_adjust_teacher_balance(v_teacher, EXTRACT(YEAR FROM v_date)::smallint, v_sign * v_hours);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_session_teacher_hours
    AFTER INSERT OR DELETE ON public.session_teachers
    FOR EACH ROW EXECUTE FUNCTION public.fn_session_teacher_hours();

-- When a session is cancelled/uncancelled or its time/date is edited, re-apply
-- the delta for every (non-guest) teacher on it. Keeps the balance correct
-- regardless of which field changed.
CREATE OR REPLACE FUNCTION public.fn_session_change_hours()
RETURNS TRIGGER AS $$
DECLARE
    r RECORD;
    v_old_hours numeric(7,2) := EXTRACT(EPOCH FROM (OLD.end_time - OLD.start_time)) / 3600.00;
    v_new_hours numeric(7,2) := EXTRACT(EPOCH FROM (NEW.end_time - NEW.start_time)) / 3600.00;
BEGIN
    IF OLD.cancelled = NEW.cancelled
       AND OLD.start_time = NEW.start_time
       AND OLD.end_time = NEW.end_time
       AND OLD.session_date = NEW.session_date THEN
        RETURN NEW;  -- nothing hours-relevant changed
    END IF;

    FOR r IN SELECT teacher_id FROM public.session_teachers WHERE session_id = NEW.id AND role <> 'Guest' LOOP
        IF NOT OLD.cancelled THEN
            PERFORM public.fn_adjust_teacher_balance(r.teacher_id, EXTRACT(YEAR FROM OLD.session_date)::smallint, -v_old_hours);
        END IF;
        IF NOT NEW.cancelled THEN
            PERFORM public.fn_adjust_teacher_balance(r.teacher_id, EXTRACT(YEAR FROM NEW.session_date)::smallint,  v_new_hours);
        END IF;
    END LOOP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_session_change_hours
    AFTER UPDATE ON public.class_sessions
    FOR EACH ROW EXECUTE FUNCTION public.fn_session_change_hours();

-- Safety net: fully recompute a teacher's year from scratch (for backfills/repairs).
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
            COALESCE((SELECT default_max_hours_per_year FROM public.teachers WHERE id = p_teacher), 800.00),
            v_total)
    ON CONFLICT (teacher_id, calendar_year)
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
-- 13. SESSION GENERATION (explode slots -> concrete sessions)  [answers Q1]
-- =========================================================================
-- Set-based: one INSERT...SELECT over the period's calendar, honouring
-- per-state public holidays and class-specific exceptions. Idempotent via the
-- uq_session_natural key. Also seeds session_teachers from each slot's teacher,
-- which is what books the teaching hours (and triggers the 800h cap check).
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
-- 15. REPORTING VIEW
-- =========================================================================
CREATE OR REPLACE VIEW public.vw_teacher_academic_workloads AS
SELECT
    b.id AS balance_record_id,
    b.teacher_id,
    b.calendar_year,
    b.booked_hours,
    b.allocated_max_hours,
    (b.allocated_max_hours - b.booked_hours) AS remaining_yearly_capacity,
    ROUND((b.booked_hours / NULLIF(b.allocated_max_hours, 0)) * 100, 1) AS pct_utilised
FROM public.teacher_yearly_balances b;

COMMIT;
