# A National VET Information Management System (NVIMS), Student Management System (SMS) — Database Design

PostgreSQL schema for a national, potentially AVETMISS-compliant Student Management System (SMS)
supporting both VET and Higher Education delivery for TAFEs and RTOs. This document
describes the design of `v0.26`: its entities, relationships, business rules, and the
mapping to the AVETMISS NAT reporting files.

> **Status:** design schema. Reference data (SACC countries, ASCL languages, full
> classification sets) is only partially seeded; see [Caveats](#caveats--not-yet-modelled).

---

## Contents

- [Scope](#scope)
- [Design principles](#design-principles)
- [Domain map](#domain-map)
- [Entity-relationship diagrams](#entity-relationship-diagrams)
  - [1. Identity](#1-identity)
  - [2. Curriculum](#2-curriculum)
  - [3. Enrolment](#3-enrolment)
  - [4. RTO infrastructure](#4-rto-infrastructure)
  - [5. Timetabling](#5-timetabling)
  - [6. Holidays](#6-holidays)
  - [7. Communications](#7-communications)
  - [8. Completions, compliance & audit](#8-completions-compliance--audit)
  - [9. Workplan](#9-workplan)
  - [10. Timesheet](#10-timesheet)
  - [11. Employment services](#11-employment-services)
  - [12. VCC](#12-vcc)
  - [13. Intakes & cohorts](#13-intakes--cohorts)
- [Table reference](#table-reference)
- [Data dictionary](#data-dictionary)
  - Identity & reference: [`people`](#people) · [`students`](#students) · [`teachers`](#teachers) · [`teacher_availability`](#teacher_availability) · [`staff`](#staff) · [`staff_availability`](#staff_availability) · [`app_users`](#app_users) · [`teacher_yearly_balances`](#teacher_yearly_balances) · [`teacher_period_allocations`](#teacher_period_allocations) · [`student_guardians`](#student_guardians) · [`student_disabilities`](#student_disabilities) · [`student_prior_achievements`](#student_prior_achievements) · [`australian_states`](#australian_states) · [`disability_types`](#disability_types) · [`prior_educational_achievements`](#prior_educational_achievements) · [`highest_school_levels`](#highest_school_levels) · [`secondary_schools`](#secondary_schools) · [`faculties`](#faculties)
  - Curriculum: [`programs`](#programs) · [`subjects`](#subjects) · [`subject_programs`](#subject_programs)
  - Intakes & cohorts: [`program_intakes`](#program_intakes) · [`intake_groups`](#intake_groups)
  - Enrolment & extensions: [`student_course_enrollments`](#student_course_enrollments) · [`client_subject_enrolments`](#client_subject_enrolments) · [`apprenticeship_details`](#apprenticeship_details) · [`traineeship_details`](#traineeship_details) · [`training_plans`](#training_plans) · [`learning_access_plans`](#learning_access_plans) · [`vet_student_loans`](#vet_student_loans) · [`he_enrolment_details`](#he_enrolment_details) · [`enrollment_credit_claims`](#enrollment_credit_claims) · [`state_funding_details`](#state_funding_details)
  - RTO infrastructure: [`training_orgs`](#training_orgs) · [`delivery_locations`](#delivery_locations) · [`person_location_preferences`](#person_location_preferences) · [`buildings`](#buildings) · [`rooms`](#rooms) · [`room_computer_lab_specs`](#room_computer_lab_specs) · [`room_lab_software`](#room_lab_software) · [`room_issues`](#room_issues) · [`employers`](#employers) · [`employer_workplaces`](#employer_workplaces) · [`aasn_providers`](#aasn_providers)
  - Timetabling: [`academic_periods`](#academic_periods) · [`classes`](#classes) · [`class_subjects`](#class_subjects) · [`class_enrollments`](#class_enrollments) · [`class_slots`](#class_slots) · [`class_sessions`](#class_sessions) · [`session_teachers`](#session_teachers) · [`session_attendance`](#session_attendance) · [`class_support_staff`](#class_support_staff) · [`class_exceptions`](#class_exceptions)
  - Holidays: [`holiday_rules`](#holiday_rules) · [`holiday_observances`](#holiday_observances)
  - Communications: [`message_templates`](#message_templates) · [`message_campaigns`](#message_campaigns) · [`message_deliveries`](#message_deliveries) · [`messages`](#messages) · [`message_recipients`](#message_recipients)
  - Compliance & audit: [`program_completions`](#program_completions) · [`student_progress_reports`](#student_progress_reports) · [`student_notes`](#student_notes) · [`avetmiss_submissions`](#avetmiss_submissions) · [`audit_log`](#audit_log)
  - Workplan: [`workplans`](#workplans) · [`workplan_approvals`](#workplan_approvals) · [`workplan_entries`](#workplan_entries)
  - Timesheet: [`pay_periods`](#pay_periods) · [`timesheets`](#timesheets) · [`timesheet_entries`](#timesheet_entries)
  - Employment services: [`student_employment_services`](#student_employment_services) · [`student_employment_registrations`](#student_employment_registrations)
  - VCC: [`teacher_vccs`](#teacher_vccs) · [`teacher_vcc_professional_qualifications`](#teacher_vcc_professional_qualifications) · [`teacher_vcc_courses`](#teacher_vcc_courses) · [`teacher_vcc_units`](#teacher_vcc_units) · [`teacher_documents`](#teacher_documents) · [`teacher_document_connections`](#teacher_document_connections) · [`teacher_currency_activities`](#teacher_currency_activities) · [`teacher_currency_unit_links`](#teacher_currency_unit_links) · [`teacher_vcc_profiling`](#teacher_vcc_profiling)
- [Business rules & constraints](#business-rules--constraints)
- [Functions & triggers](#functions--triggers)
- [AVETMISS NAT file mapping](#avetmiss-nat-file-mapping)
- [Notes for application developers](#notes-for-application-developers)
- [Caveats & not-yet-modelled](#caveats--not-yet-modelled)

---

## Scope

The schema covers the full SMS lifecycle for a national RTO/TAFE student management
system, designed for both VET and Higher Education delivery:

- **People & identity** — students, teachers, support staff, guardians, system users.
- **Curriculum** — qualifications/courses (programs) and units/modules/subjects.
- **Enrolment** — at both program level and subject level, including standalone units.
- **Extensions** — apprenticeships, traineeships, training plans, Learning Access Plans
  (LAPs), VET Student Loans, Higher Education unit details, RPL/credit transfer.
- **Timetabling** — recurring class templates, concrete sessions, attendance, room and
  teacher double-booking prevention, and configurable per-teacher teaching hour caps
  (annual and optionally per-academic-period for HE/DUAL-sector teachers).
- **Public holidays** — recurrence rules expanded into concrete observances.
- **Communications** — email/SMS bulk campaigns, per-recipient delivery logs, and
  teacher-to-recipient direct messages with automatic sender CC.
- **Intakes & cohorts** — scheduled program intake offerings (start period, delivery
  location, study mode, duration) and the sub-cohorts (groups) within each intake that
  attend classes together; students are enrolled into a group, classes are linked to a group.
- **Compliance** — AVETMISS NAT export sources, program completions, and an audit trail.
- **Workplan** — annual teacher workplan per VTSA 2024 clause 32.4: Teaching Delivery,
  CAPPS, and Education-Related Duties allocations with an approval workflow.
- **Timesheet** — fortnightly (or other pay-period) hour records auto-populated from
  sessions; submitted for external payroll processing (hours only, no banking or super
  data).
- **VCC** — annual Vocational Competency & Currency document per teacher: credentials,
  professional development activities, vocational and professional currency point records,
  and a supervisor approval workflow with spider/radar profiling scores.
- **Employment services** — Centrelink CRN and job seeker details, plus employment
  provider registration records for DEWR-funded students.

---

## Design principles

| Principle | How it shows up |
|---|---|
| **Surrogate keys everywhere** | Every operational table uses `BIGSERIAL` surrogate PKs; natural keys (USI, student number, AVETMISS codes) are enforced as `UNIQUE`. |
| **Shared-primary-key subtyping** | `people` owns identity. `students`, `teachers`, `staff` each have `id` that is *both* their PK *and* a FK to `people(id)`. A row's identity **is** its person, and one person may hold multiple roles. |
| **Sessions are the source of truth** | Teaching hours, attendance, and the per-teacher cap enforcement are all derived from concrete `class_sessions` + `session_teachers`, never from the recurring `class_slots` template. |
| **Definition vs occurrence** | Recurring concepts are stored once as rules and expanded into dated rows: `holiday_rules` → `holiday_observances`; `class_slots` → `class_sessions`. |
| **Compliance data is never hard-deleted** | Core records use soft-delete (`deleted_at`/`deleted_by`) and the enrolment chain uses `ON DELETE RESTRICT` so reportable history can't be cascaded away. |
| **Invariants enforced in the database** | Exclusion constraints, `CHECK`s, and `num_nonnulls()` keep the data correct regardless of which client wrote it, so the application layer can stay simple. |

---

## Domain map

```mermaid
graph TD
    P[Identity<br/>people, students, teachers, staff, app_users]
    C[Curriculum<br/>programs, subjects]
    IC[Intakes & Cohorts<br/>program_intakes, intake_groups]
    E[Enrolment<br/>course & subject enrolments + extensions]
    I[RTO Infrastructure<br/>training_orgs, delivery_locations, buildings, rooms]
    T[Timetabling<br/>classes, slots, sessions, attendance]
    H[Holidays<br/>rules, observances]
    M[Communications<br/>templates, campaigns, deliveries, messages, recipients]
    A[Compliance & Audit<br/>completions, avetmiss_submissions, audit_log]
    W[Workplan<br/>workplans, workplan_approvals, workplan_entries]
    S[Timesheet<br/>pay_periods, timesheets, timesheet_entries]
    ES[Employment Services<br/>student_employment_services, registrations]
    V[VCC<br/>teacher_vccs, qualifications, courses, units, currency, documents]

    P --> E
    C --> IC
    I --> IC
    IC --> E
    IC --> T
    C --> E
    I --> E
    E --> T
    P --> T
    I --> T
    H --> T
    P --> M
    E --> A
    P --> A
    I --> A
    P --> W
    T --> W
    P --> S
    T --> S
    W --> S
    P --> ES
    P --> V
```

---

## Entity-relationship diagrams

Diagrams are split by domain for readability. `||--o{` = one-to-many, `||--o|` = one-to-(zero-or-one), `}o--o{` = many-to-many (via a join table).

### 1. Identity

```mermaid
erDiagram
    PEOPLE ||--o| STUDENTS : "is-a (shared PK)"
    PEOPLE ||--o| TEACHERS : "is-a (shared PK)"
    PEOPLE ||--o| STAFF : "is-a (shared PK)"
    PEOPLE ||--o{ APP_USERS : "may log in as"
    FACULTIES ||--o{ TEACHERS : employs
    FACULTIES ||--o{ STAFF : employs
    STUDENTS ||--o{ STUDENT_GUARDIANS : has
    STUDENTS ||--o{ STUDENT_DISABILITIES : declares
    STUDENTS ||--o{ STUDENT_PRIOR_ACHIEVEMENTS : declares
    DISABILITY_TYPES ||--o{ STUDENT_DISABILITIES : classifies
    PRIOR_EDUCATIONAL_ACHIEVEMENTS ||--o{ STUDENT_PRIOR_ACHIEVEMENTS : classifies
    SECONDARY_SCHOOLS ||--o{ STUDENTS : "last attended"
    HIGHEST_SCHOOL_LEVELS ||--o{ STUDENTS : classifies
    TEACHERS ||--o{ TEACHER_YEARLY_BALANCES : "accrues hours"
    TEACHERS ||--o{ TEACHER_PERIOD_ALLOCATIONS : "per-period cap"
    ACADEMIC_PERIODS ||--o{ TEACHER_PERIOD_ALLOCATIONS : ""

    PEOPLE {
        bigserial id PK
        varchar first_given_name
        varchar family_name
        date dob
        varchar gender
        varchar state_code FK
        varchar photo_url
        timestamptz photo_uploaded_at
    }
    STUDENTS {
        bigint id PK,FK
        varchar student_number UK
        varchar usi UK
        varchar indigenous_status_id
        timestamptz deleted_at
    }
    TEACHERS {
        bigint id PK,FK
        enum sector "VET/HE/DUAL"
        numeric default_max_hours_per_year
        numeric max_hours_per_period "NULL = annual cap only"
    }
    APP_USERS {
        bigserial id PK
        bigint person_id FK
        varchar role
    }
```

### 2. Curriculum

```mermaid
erDiagram
    FACULTIES ||--o{ PROGRAMS : owns
    PROGRAMS }o--o{ SUBJECTS : "packaged via SUBJECT_PROGRAMS"
    SUBJECT_PROGRAMS }o--|| PROGRAMS : ""
    SUBJECT_PROGRAMS }o--|| SUBJECTS : ""

    PROGRAMS {
        bigserial id PK
        varchar program_code UK
        varchar level_of_education
        varchar field_of_education
        boolean vet_flag
        boolean he_flag
        integer credit_points "total qualification cp (HE)"
        smallint aqf_level "1-10, nullable"
        varchar program_type "nullable"
    }
    SUBJECTS {
        bigserial id PK
        varchar subject_code UK
        varchar field_of_education
        integer nominal_hours
        boolean vet_flag
        integer credit_points "HE unit credit points, nullable"
    }
```

### 3. Enrolment

```mermaid
erDiagram
    STUDENTS ||--o{ STUDENT_COURSE_ENROLLMENTS : "enrols in program"
    PROGRAMS ||--o{ STUDENT_COURSE_ENROLLMENTS : ""
    STUDENT_COURSE_ENROLLMENTS ||--o{ CLIENT_SUBJECT_ENROLMENTS : "has units"
    STUDENTS ||--o{ CLIENT_SUBJECT_ENROLMENTS : "owns (incl. standalone)"
    SUBJECTS ||--o{ CLIENT_SUBJECT_ENROLMENTS : ""
    DELIVERY_LOCATIONS ||--o{ CLIENT_SUBJECT_ENROLMENTS : "delivered at"

    STUDENT_COURSE_ENROLLMENTS ||--o| APPRENTICESHIP_DETAILS : "extends"
    STUDENT_COURSE_ENROLLMENTS ||--o| TRAINEESHIP_DETAILS : "extends"
    STUDENT_COURSE_ENROLLMENTS ||--o| TRAINING_PLANS : "extends"
    STUDENT_COURSE_ENROLLMENTS ||--o| HE_ENROLMENT_DETAILS : "extends"
    ACADEMIC_PERIODS ||--o{ HE_ENROLMENT_DETAILS : "period (nullable)"
    STUDENT_COURSE_ENROLLMENTS ||--o{ STATE_FUNDING_DETAILS : "extends"
    STUDENT_COURSE_ENROLLMENTS ||--o{ VET_STUDENT_LOANS : "extends"
    STUDENT_COURSE_ENROLLMENTS ||--o{ ENROLLMENT_CREDIT_CLAIMS : "RPL/CT"

    CLIENT_SUBJECT_ENROLMENTS {
        bigserial id PK
        bigint student_id FK
        bigint student_course_enrollment_id FK "nullable = standalone unit"
        bigint subject_id FK
        numeric scheduled_hours
        varchar outcome_id_national
        varchar result_status
    }
```

### 4. RTO infrastructure

```mermaid
erDiagram
    TRAINING_ORGS ||--o{ DELIVERY_LOCATIONS : has
    DELIVERY_LOCATIONS ||--o{ BUILDINGS : has
    BUILDINGS ||--o{ ROOMS : has
    EMPLOYERS ||--o{ EMPLOYER_WORKPLACES : has
    AUSTRALIAN_STATES ||--o{ DELIVERY_LOCATIONS : "located in"

    AUSTRALIAN_STATES {
        varchar state_code PK
        char avetmiss_state_id UK "numeric NAT id"
    }
    DELIVERY_LOCATIONS {
        bigserial id PK
        bigint training_org_id FK
        varchar delivery_loc_id
        varchar state_code FK
    }
```

### 5. Timetabling

```mermaid
erDiagram
    ACADEMIC_PERIODS ||--o{ CLASSES : "scheduled in"
    DELIVERY_LOCATIONS ||--o{ CLASSES : "delivered at"
    CLASSES ||--o{ CLASS_SUBJECTS : "delivers units"
    CLASSES ||--o{ CLASS_ENROLLMENTS : "has students"
    CLASSES ||--o{ CLASS_SLOTS : "weekly template"
    CLASSES ||--o{ CLASS_SESSIONS : "occurrences"
    CLASSES ||--o{ CLASS_EXCEPTIONS : "no-class dates"
    CLASSES ||--o{ CLASS_SUPPORT_STAFF : "support assigned"
    CLIENT_SUBJECT_ENROLMENTS ||--o{ CLASS_ENROLLMENTS : "mapped to class"
    CLASS_SESSIONS ||--o{ SESSION_TEACHERS : "taught by"
    CLASS_SESSIONS ||--o{ SESSION_ATTENDANCE : "attended by"
    TEACHERS ||--o{ SESSION_TEACHERS : ""
    TEACHERS ||--o{ CLASS_SLOTS : "assigned"
    STUDENTS ||--o{ SESSION_ATTENDANCE : ""
    STAFF ||--o{ CLASS_SUPPORT_STAFF : ""
    STUDENTS ||--o{ CLASS_SUPPORT_STAFF : "1:1 support (optional)"
    ROOMS ||--o{ CLASS_SLOTS : ""
    ROOMS ||--o{ CLASS_SESSIONS : ""

    CLASS_SLOTS {
        bigserial id PK
        bigint class_id FK
        bigint academic_period_id FK "for exclusion scope"
        bigint teacher_id FK
        smallint day_of_week
        time start_time
        time end_time
    }
    CLASS_SESSIONS {
        bigserial id PK
        bigint class_id FK
        date session_date
        time start_time
        time end_time
        boolean cancelled
    }
    SESSION_ATTENDANCE {
        bigserial id PK
        bigint session_id FK
        bigint student_id FK
        varchar status
        integer minutes_attended
        smallint units_nominated
        time arrived_at
        time departed_at
        smallint break_minutes
        varchar absence_reason
        boolean absence_is_acceptable
        boolean has_childcare
        boolean is_note_private
        text notes
    }
```

### 6. Holidays

```mermaid
erDiagram
    HOLIDAY_RULES ||--o{ HOLIDAY_OBSERVANCES : "expands into"
    AUSTRALIAN_STATES ||--o{ HOLIDAY_RULES : "applies to (NULL = national)"
    AUSTRALIAN_STATES ||--o{ HOLIDAY_OBSERVANCES : ""

    HOLIDAY_RULES {
        bigserial id PK
        varchar holiday_name
        varchar state_code FK "NULL = national"
        varchar recurrence "ONCE/ANNUAL_FIXED/NTH_DOW/EASTER_OFFSET"
        boolean observe_substitute
    }
    HOLIDAY_OBSERVANCES {
        bigserial id PK
        date holiday_date
        bigint rule_id FK "NULL = hand-entered one-off"
        boolean is_substitute
    }
```

### 7. Communications

```mermaid
erDiagram
    MESSAGE_TEMPLATES ||--o{ MESSAGE_CAMPAIGNS : "based on"
    MESSAGE_CAMPAIGNS ||--o{ MESSAGE_DELIVERIES : "fans out to"
    APP_USERS ||--o{ MESSAGE_CAMPAIGNS : "sent by"
    CLASSES ||--o{ MESSAGE_CAMPAIGNS : "targets (optional)"
    PROGRAMS ||--o{ MESSAGE_CAMPAIGNS : "targets (optional)"
    STUDENTS ||--o{ MESSAGE_DELIVERIES : recipient
    STUDENT_GUARDIANS ||--o{ MESSAGE_DELIVERIES : recipient
    STAFF ||--o{ MESSAGE_DELIVERIES : recipient
    APP_USERS ||--o{ MESSAGES : "sent by"
    MESSAGES ||--o{ MESSAGE_RECIPIENTS : "delivers to"
    STUDENTS ||--o{ MESSAGE_RECIPIENTS : recipient
    STUDENT_GUARDIANS ||--o{ MESSAGE_RECIPIENTS : recipient
    STAFF ||--o{ MESSAGE_RECIPIENTS : recipient
    TEACHERS ||--o{ MESSAGE_RECIPIENTS : recipient

    MESSAGE_DELIVERIES {
        bigserial id PK
        bigint campaign_id FK
        bigint student_id FK "one of four"
        bigint guardian_id FK "recipient FKs"
        bigint staff_id FK "is non-null"
        varchar status
    }
    MESSAGES {
        bigserial id PK
        bigint sender_id FK
        varchar channel
        varchar status
        timestamptz sent_at
    }
    MESSAGE_RECIPIENTS {
        bigserial id PK
        bigint message_id FK
        varchar recipient_type
        boolean is_cc
        bigint teacher_id FK "one of four"
        varchar status
        timestamptz read_at
    }
```

### 8. Completions, compliance & audit

```mermaid
erDiagram
    STUDENTS ||--o{ PROGRAM_COMPLETIONS : completes
    PROGRAMS ||--o{ PROGRAM_COMPLETIONS : ""
    TRAINING_ORGS ||--o{ PROGRAM_COMPLETIONS : "issued by"
    TRAINING_ORGS ||--o{ AVETMISS_SUBMISSIONS : submits
    APP_USERS ||--o{ AVETMISS_SUBMISSIONS : "submitted by"
    APP_USERS ||--o{ AUDIT_LOG : "actor"
    STUDENTS ||--o{ STUDENT_NOTES : "annotated by staff"
    STUDENTS ||--o{ STUDENT_PROGRESS_REPORTS : "documents"
```

### 9. Workplan

```mermaid
erDiagram
    TEACHERS ||--o{ WORKPLANS : "has annual workplan"
    APP_USERS ||--o{ WORKPLANS : "submitted by"
    WORKPLANS ||--o{ WORKPLAN_APPROVALS : "approval steps"
    APP_USERS ||--o{ WORKPLAN_APPROVALS : "approver"
    WORKPLANS ||--o{ WORKPLAN_ENTRIES : "line items"
    SUBJECTS ||--o{ WORKPLAN_ENTRIES : "unit context (nullable)"
    PROGRAMS ||--o{ WORKPLAN_ENTRIES : "course context (nullable)"
    ACADEMIC_PERIODS ||--o{ WORKPLAN_ENTRIES : "semester (nullable)"
    CLASS_SESSIONS ||--o{ WORKPLAN_ENTRIES : "session link (nullable)"

    WORKPLANS {
        bigserial id PK
        bigint teacher_id FK
        smallint calendar_year
        smallint version
        varchar status "Draft/Submitted/Approved"
        numeric time_fraction "FTE"
        numeric capps_ratio "0.750 = 45 min per teaching hour"
        numeric accountable_hours_required
        numeric agreed_overtime_hours
    }
    WORKPLAN_ENTRIES {
        bigserial id PK
        bigint workplan_id FK
        varchar entry_type "Teaching Delivery/CAPPS/ERD"
        varchar activity_name
        numeric total_hours
        date activity_start_date "nullable, session-linked"
        bigint class_session_id FK "nullable"
    }
    WORKPLAN_APPROVALS {
        bigserial id PK
        bigint workplan_id FK
        bigint approver_id FK
        varchar approval_role "Teacher/LineManager"
        timestamptz approved_at
    }
```

### 10. Timesheet

```mermaid
erDiagram
    PAY_PERIODS ||--o{ TIMESHEETS : "period for"
    TEACHERS ||--o{ TIMESHEETS : "submitted by teacher"
    APP_USERS ||--o{ TIMESHEETS : "submitted / approved by"
    TIMESHEETS ||--o{ TIMESHEET_ENTRIES : "hour lines"
    CLASS_SESSIONS ||--o{ TIMESHEET_ENTRIES : "source session (nullable)"
    WORKPLAN_ENTRIES ||--o{ TIMESHEET_ENTRIES : "plan reconciliation (nullable)"

    PAY_PERIODS {
        bigserial id PK
        date period_start
        date period_end
        varchar period_name "e.g. FN01 2026"
        smallint calendar_year
    }
    TIMESHEETS {
        bigserial id PK
        bigint teacher_id FK
        bigint pay_period_id FK
        varchar status "Draft/Submitted/Approved/Exported"
        timestamptz submitted_at
        timestamptz approved_at
        timestamptz exported_at
        varchar export_format "PDF/XLSX, nullable"
    }
    TIMESHEET_ENTRIES {
        bigserial id PK
        bigint timesheet_id FK
        date entry_date
        varchar entry_type "Teaching/CAPPS/ERD/Other"
        numeric hours
        boolean is_overtime
        bigint class_session_id FK "nullable, auto-populated"
        bigint workplan_entry_id FK "nullable, reconciliation"
    }
```

### 11. Employment services

```mermaid
erDiagram
    STUDENTS ||--o| STUDENT_EMPLOYMENT_SERVICES : "has Centrelink data"
    STUDENT_EMPLOYMENT_SERVICES ||--o{ STUDENT_EMPLOYMENT_REGISTRATIONS : "has provider registrations"
```

### 12. VCC

```mermaid
erDiagram
    TEACHERS ||--o{ TEACHER_VCCS : "has VCC versions"
    TEACHER_VCCS ||--o{ TEACHER_VCC_PROFESSIONAL_QUALIFICATIONS : "has credentials"
    TEACHER_VCCS ||--o{ TEACHER_VCC_COURSES : "maps courses"
    TEACHER_VCC_COURSES ||--o{ TEACHER_VCC_UNITS : "has units"
    TEACHER_VCCS ||--o{ TEACHER_VCC_UNITS : "standalone units"
    TEACHERS ||--o{ TEACHER_DOCUMENTS : "document library"
    TEACHER_DOCUMENTS ||--o{ TEACHER_DOCUMENT_CONNECTIONS : "linked to"
    TEACHERS ||--o{ TEACHER_CURRENCY_ACTIVITIES : "currency records"
    TEACHER_CURRENCY_ACTIVITIES ||--o{ TEACHER_CURRENCY_UNIT_LINKS : "related units"
    TEACHER_VCCS ||--o{ TEACHER_VCC_PROFILING : "dimension scores"
```

### 13. Intakes & cohorts

```mermaid
erDiagram
    PROGRAMS ||--o{ PROGRAM_INTAKES : "offered as"
    ACADEMIC_PERIODS ||--o{ PROGRAM_INTAKES : "starts in"
    DELIVERY_LOCATIONS ||--o{ PROGRAM_INTAKES : "delivered at"
    FACULTIES ||--o{ PROGRAM_INTAKES : "owned by"
    PROGRAM_INTAKES ||--o{ INTAKE_GROUPS : "has groups"
    INTAKE_GROUPS ||--o{ STUDENT_COURSE_ENROLLMENTS : "enrolled in"
    INTAKE_GROUPS ||--o{ CLASSES : "attends"

    PROGRAM_INTAKES {
        bigserial id PK
        bigint program_id FK
        varchar intake_code UK
        varchar intake_name
        bigint start_academic_period_id FK
        bigint delivery_location_id FK
        bigint faculty_id FK
        varchar study_mode "Full-Time/Part-Time"
        smallint duration_periods
        date enrolment_open_date
        date enrolment_close_date
        varchar status "Planned/Active/Closed/Cancelled"
    }
    INTAKE_GROUPS {
        bigserial id PK
        bigint intake_id FK
        varchar group_code
        varchar group_name
        integer capacity
    }
```

---

## Table reference

Tables are grouped by domain. "Key relationships" lists the most important foreign keys.

### Identity & reference

| Table | Purpose | Key relationships |
|---|---|---|
| `people` | Single identity spine — name, DOB, gender, address, contact, `preferred_contact_method`, WWCC number and expiry. Owns the surrogate id. | → `australian_states` |
| `students` | Student-specific data: student number, USI, AVETMISS demographics, photo, ID expiry. Shares PK with `people`. Soft-deletable. | PK = FK → `people`; → `secondary_schools`, `highest_school_levels` |
| `teachers` | Teacher-specific data: sector (`VET`/`HE`/`DUAL`), annual hours cap, optional per-period cap, police check status and date. Shares PK with `people`. | PK = FK → `people`; → `faculties` |
| `staff` | Support/admin staff. Police check status and date. Shares PK with `people`. | PK = FK → `people`; → `faculties` |
| `app_users` | Login/system accounts and RBAC role. Source of every `*_by` audit actor. | → `people` (nullable, for service accounts) |
| `teacher_yearly_balances` | Maintained cache of booked teaching hours per teacher per calendar year. Cap seeded from `teachers.default_max_hours_per_year`; overridable per-year. | → `teachers` |
| `teacher_period_allocations` | Per-academic-period hour cap and running total for HE/DUAL teachers with `max_hours_per_period` set. Auto-created on first session booking. | → `teachers`, `academic_periods` |
| `student_guardians` | Guardians/emergency contacts (`is_emergency_contact`); comms targets for under-18s. | → `students` |
| `student_disabilities` | Declared disabilities (NAT00090). | → `students`, `disability_types` |
| `student_prior_achievements` | Prior educational achievement (NAT00100). | → `students`, `prior_educational_achievements` |
| `australian_states` | State/territory reference, incl. the **numeric AVETMISS state id**. | — |
| `disability_types`, `prior_educational_achievements`, `highest_school_levels`, `secondary_schools`, `faculties` | Classification & org reference data. | — |

### Curriculum

| Table | Purpose | Key relationships |
|---|---|---|
| `programs` | Qualifications/courses (NAT00030). `he_flag` distinguishes HE qualifications. `credit_points` is the total qualification credit point value; `aqf_level` (1–10) applies to both VET and HE. `program_type` categorises the qualification type (e.g. `Qualification`, `Skill Set`). | → `faculties` |
| `subjects` | Units/modules/subjects (NAT00060). `credit_points` holds the HE unit credit point value (NULL for VET-only units). | — |
| `subject_programs` | Which subjects belong to which programs (many-to-many). `is_core` flags mandatory units; `group_code`/`group_title` support unit grouping within a qualification. | → `subjects`, `programs` |

### Intakes & cohorts

| Table | Purpose | Key relationships |
|---|---|---|
| `program_intakes` | A scheduled offering of a program — the when, where, mode, and duration of a particular intake cohort. `intake_code` is the unique human identifier (e.g. `ICT30120-2025-T1-FT`). `duration_periods` records how many academic periods the program runs; `duration_years` stores the equivalent calendar duration in years. | → `programs`, `academic_periods` (start), `delivery_locations`, `faculties` |
| `intake_groups` | Sub-cohorts within an intake that attend classes together. Each group has a short `group_code` and a display `group_name`; optional `capacity` caps enrolments. Students are linked to a group via `student_course_enrollments.intake_group_id`; classes are linked via `classes.intake_group_id`. | → `program_intakes` |

### Enrolment & extensions

| Table | Purpose | Key relationships |
|---|---|---|
| `student_course_enrollments` | Program-level enrolment; commencement/completion, funding state. `intake_group_id` links the student to their specific intake cohort group. Soft-deletable. | → `students`, `programs`, `intake_groups` |
| `client_subject_enrolments` | Subject-level training activity (NAT00120). `student_course_enrollment_id` is **nullable** to allow standalone unit enrolments. Holds the draft/finalised result workflow. | → `students`, `subjects`, `student_course_enrollments`, `delivery_locations` |
| `apprenticeship_details` | Apprenticeship contract, employer, AASN, training-plan milestones (1:1). | → `student_course_enrollments`, `employers`, `employer_workplaces`, `aasn_providers` |
| `traineeship_details` | Traineeship probation/extension data (1:1). | → `student_course_enrollments` |
| `training_plans` | Training-plan signing/review dates (1:1). | → `student_course_enrollments` |
| `learning_access_plans` | LAP: adjustments and resources for students with disabilities. | → `students`, `student_course_enrollments`, `staff` (assessor) |
| `vet_student_loans` | VSL/VET-FEE-HELP per census date (1:many). | → `student_course_enrollments` |
| `he_enrolment_details` | Higher-Ed EFTSL/census/HELP details (1:1). `academic_period_id` links to the specific semester/trimester; `credit_points_enrolled` tracks partial/full load. | → `student_course_enrollments`, `academic_periods` |
| `enrollment_credit_claims` | RPL and credit transfer grants. | → `student_course_enrollments`, `subjects` |
| `state_funding_details` | State-specific funding attributes (Skills First, Smart & Skilled…) as `jsonb`, off the national table. | → `student_course_enrollments`, `australian_states` |

### RTO infrastructure

| Table | Purpose | Key relationships |
|---|---|---|
| `training_orgs` | The RTO (NAT00010). | → `australian_states` |
| `delivery_locations` | Delivery/campus locations (NAT00020). | → `training_orgs`, `australian_states` |
| `person_location_preferences` | A person's ranked delivery location preferences. Equal ranks are allowed. | →&nbsp;`people`, `delivery_locations` |
| `buildings`, `rooms` | Physical spaces for timetabling. `is_computer_lab` flags lab rooms. | `buildings`→`delivery_locations`; `rooms`→`buildings` |
| `room_computer_lab_specs` | Hardware profile for a computer lab room (RAM, microphone, webcam, workstation count). | →&nbsp;`rooms` |
| `room_lab_software` | Software titles installed in a computer lab room. | →&nbsp;`rooms` |
| `room_issues` | Faults and maintenance issues reported for any room, with Open/Investigating/Resolved workflow. | →&nbsp;`rooms` |
| `employers`, `employer_workplaces` | Apprenticeship/traineeship workplaces. | `employer_workplaces`→`employers` |
| `aasn_providers` | Australian Apprenticeship Support Network providers. | — |

### Timetabling

| Table | Purpose | Key relationships |
|---|---|---|
| `academic_periods` | Terms/semesters/trimesters/years with date ranges. `period_type` supports `TERM`, `SEMESTER`, `TRIMESTER`, `YEAR`, `BLOCK` (6–8 week intensive), and `ROLLING` (monthly intake). `sequence_number` orders periods within a year. | — |
| `classes` | A delivery instance within a period at a location. `intake_group_id` links the class to the cohort group it is taught to. | → `academic_periods`, `delivery_locations`, `intake_groups` |
| `class_subjects` | The units a class delivers. | → `classes`, `subjects` |
| `class_enrollments` | Maps a student's subject enrolment to a class. | → `classes`, `client_subject_enrolments` |
| `class_slots` | **Recurring weekly template** (weekday + time + teacher + room). Carries `academic_period_id` for exclusion scoping. | → `classes`, `academic_periods`, `teachers`, `rooms` |
| `class_sessions` | **Concrete dated occurrences** — the source of truth for hours/attendance. | → `classes`, `rooms` |
| `session_teachers` | Teachers on a session, with role (supports team teaching). Drives the hours cap. | → `class_sessions`, `teachers` |
| `session_attendance` | Per-student attendance per session. Extended attendance-dialog fields: `units_nominated`, `arrived_at`/`departed_at`, `break_minutes`, `absence_reason`, `absence_is_acceptable`, `has_childcare`, `is_note_private`. | → `class_sessions`, `students`, `app_users` |
| `class_support_staff` | Support staff per class, optionally tied to one student. | → `classes`, `staff`, `students` |
| `class_exceptions` | Per-class no-class dates, skipped during session generation. | → `classes` |

### Holidays

| Table | Purpose | Key relationships |
|---|---|---|
| `holiday_rules` | Recurrence definitions (fixed-date, nth-weekday, Easter-offset, or one-off). | → `australian_states` (NULL = national) |
| `holiday_observances` | Concrete dated holidays that session generation reads. | → `holiday_rules` (NULL = hand-entered) |

### Communications

| Table | Purpose | Key relationships |
|---|---|---|
| `message_templates` | Reusable email/SMS templates. | → `app_users` |
| `message_campaigns` | A send to an audience (individual/class/program/cohort/broadcast/guardian). | → `message_templates`, `app_users`, `classes`, `programs` |
| `message_deliveries` | Per-recipient delivery log with provider status. Exactly one recipient relation set. | → `message_campaigns`, `students`/`student_guardians`/`staff` |
| `messages` | Teacher-composed individual direct message (not a campaign). Draft → Sent → Failed. | → `app_users` |
| `message_recipients` | Per-recipient delivery row for a direct message. `is_cc = true` marks the auto-CC row inserted for the sender. Exactly one of four recipient FKs is set. | → `messages`, `students`/`student_guardians`/`staff`/`teachers` |

### Compliance & audit

| Table | Purpose | Key relationships |
|---|---|---|
| `program_completions` | Qualifications completed/issued (NAT00130). | → `students`, `programs`, `training_orgs` |
| `student_progress_reports` | References to externally-stored report documents. `keywords` (`text[]`) supports tag-based filtering. | → `students`, `student_course_enrollments`, `app_users` |
| `student_notes` | Timestamped, authored notes per student (multiple per student). `note_type` includes `'Communication'`. | → `students`, `app_users` |
| `avetmiss_submissions` | Record of each NAT submission to the STA/NCVER. | → `training_orgs`, `app_users` |
| `audit_log` | Append-only change trail (old/new `jsonb`, actor, action). | → `app_users` |

### Workplan

| Table | Purpose | Key relationships |
|---|---|---|
| `workplans` | Annual VTSA 2024 cl. 32.4 workplan per teacher per year. | → `teachers`, `app_users` |
| `workplan_approvals` | Teacher/LineManager approval steps for a workplan. | → `workplans`, `app_users` |
| `workplan_entries` | Teaching Delivery / CAPPS / ERD line items on a workplan. | → `workplans`, `subjects`, `programs`, `academic_periods`, `class_sessions` |

### Timesheet

| Table | Purpose | Key relationships |
|---|---|---|
| `pay_periods` | Administrator-defined pay periods (fortnightly by default). | — |
| `timesheets` | One per teacher per pay period; hours only for external payroll. | → `teachers`, `pay_periods`, `app_users` |
| `timesheet_entries` | Hour lines per date; auto-populated for sessions, manual for ERD/Other. | → `timesheets`, `class_sessions`, `workplan_entries` |

### Employment services

| Table | Purpose | Key relationships |
|---|---|---|
| `student_employment_services` | Centrelink CRN, job seeker ID, participation hours/type — one row per student. | PK = FK → `students` |
| `student_employment_registrations` | Provider registration rows (e.g. jobactive, DES). Child of employment services. | → `student_employment_services` |

### VCC

| Table | Purpose | Key relationships |
|---|---|---|
| `teacher_vccs` | VCC document per teacher per year, versioned, with Draft→Submitted→Approved workflow. | → `teachers`, `app_users` (supervisor, approver) |
| `teacher_vcc_professional_qualifications` | Teacher's own credentials (TAE, degrees, industry certs). | → `teacher_vccs` |
| `teacher_vcc_courses` | Courses the teacher is mapped to deliver in a VCC; optionally linked to `programs`. | → `teacher_vccs`, `programs` |
| `teacher_vcc_units` | Units teacher has currency for, with competency method and justification. Multiple rows per unit allowed. | → `teacher_vccs`, `teacher_vcc_courses`, `subjects` |
| `teacher_documents` | Per-teacher document library (testamurs, transcripts, credentials, other evidence). | → `teachers` |
| `teacher_document_connections` | Links a document to exactly one VCC entity (professional qual, unit, or currency activity). `num_nonnulls = 1` enforced. | → `teacher_documents`, `teacher_vcc_professional_qualifications`, `teacher_vcc_units`, `teacher_currency_activities` |
| `teacher_currency_activities` | Vocational and professional currency point records with activity detail and approval tracking. Professional-specific fields (`domain_name`, `program_type`, etc.) are nullable columns on the same table. | → `teachers` |
| `teacher_currency_unit_links` | "Related Unit/s" M2M between currency activities and subjects. | → `teacher_currency_activities`, `subjects` |
| `teacher_vcc_profiling` | Spider/radar-chart dimension scores (self, supervisor, business ideal) per VCC version. PK is `(vcc_id, dimension)`. | → `teacher_vccs` |

---

## Data dictionary

Every table and column, generated from `v0.26`. **Null** = whether the column accepts NULL. **Key**: PK = primary key, UK = unique, FK &rarr; target = foreign key. Table-level constraints (checks, composite keys, exclusion constraints, unique indexes) are listed under each table.

### Identity & reference

#### `people`

The central identity spine for every person in the system. Stores all personal details — name, preferred name, date of birth, gender, full address, contact information (email, phone, emergency contact), WWCC number and expiry, police check status and date, and photo URL — regardless of the person's role. Every other person-subtype table (`students`, `teachers`, `staff`) uses `people.id` as its own primary key (shared-PK subtyping), so this table owns the surrogate identity and all role-specific data extends from it. Also referenced by `app_users`, `student_guardians`, `delivery_locations`, `training_orgs`, and any table that stores an address or contact.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `title` | `varchar(10)` | yes |  |  |
| `first_given_name` | `varchar(40)` | no |  |  |
| `family_name` | `varchar(40)` | no |  |  |
| `preferred_name` | `varchar(50)` | yes |  |  |
| `dob` | `date` | no |  |  |
| `gender` | `varchar(1)` | no |  |  |
| `building_property_name` | `varchar(50)` | yes |  |  |
| `unit_details` | `varchar(30)` | yes |  |  |
| `street_number` | `varchar(10)` | yes |  |  |
| `street_name` | `varchar(70)` | yes |  |  |
| `suburb` | `varchar(50)` | no |  |  |
| `state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `postcode` | `varchar(4)` | no |  |  |
| `postal_delivery_info` | `varchar(50)` | yes |  |  |
| `country_id` | `varchar(4)` | no | `'1101'` |  |
| `primary_email` | `varchar(100)` | no |  | UK |
| `secondary_email` | `varchar(100)` | yes |  |  |
| `phone_home` | `varchar(15)` | yes |  |  |
| `phone_work` | `varchar(15)` | yes |  |  |
| `phone_mobile` | `varchar(15)` | yes |  |  |
| `emergency_contact_name` | `varchar(100)` | yes |  |  |
| `emergency_contact_phone` | `varchar(15)` | yes |  |  |
| `emergency_contact_relationship` | `varchar(30)` | yes |  |  |
| `preferred_contact_method` | `varchar(20)` | yes |  |  |
| `wwcc_number` | `text` | yes |  |  |
| `wwcc_expiry` | `date` | yes |  |  |
| `police_check_status` | `text` | yes |  |  |
| `police_check_date` | `date` | yes |  |  |
| `photo_url` | `varchar(2048)` | yes |  |  |
| `photo_uploaded_at` | `timestamp with time zone` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_people_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT uq_people_email UNIQUE (primary_email)`
- `CONSTRAINT chk_people_email CHECK (primary_email LIKE '%@%.%')`
- `CONSTRAINT chk_avetmiss_gender CHECK (gender IN ('M', 'F', 'X'))`
- `CONSTRAINT chk_postcode_format CHECK (postcode ~ '^[0-9]{4}$')`

#### `students`

Student-specific demographic and AVETMISS compliance data. Extends `people` via a shared PK (`students.id = people.id`), so deleting a `students` row cascades from `people`. Holds the student number, USI, indigenous status, country of birth, language, prior education and disability flags, highest school level, secondary school, and identity document details. Soft-deletable via `deleted_at`/`deleted_by` to preserve AVETMISS reporting history; uniqueness of `student_number` and `student_email` is enforced on active rows only so a returning student is not blocked by a prior soft-deleted record. Referenced by `student_course_enrollments`, `client_subject_enrolments`, `session_attendance`, `student_guardians`, `student_disabilities`, `student_prior_achievements`, and all other student-centric tables.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;people |
| `student_number` | `varchar(20)` | no |  |  |
| `student_email` | `varchar(100)` | no |  |  |
| `usi` | `varchar(10)` | yes |  | UK |
| `indigenous_status_id` | `varchar(1)` | no | `'9'` |  |
| `country_of_birth_id` | `varchar(4)` | no | `'1101'` |  |
| `language_id` | `varchar(4)` | no | `'1201'` |  |
| `english_proficiency_id` | `varchar(1)` | yes |  |  |
| `labour_force_status_id` | `varchar(2)` | yes |  |  |
| `highest_school_level_id` | `varchar(2)` | yes |  | FK&nbsp;&rarr;&nbsp;highest_school_levels |
| `year_highest_school_completed` | `smallint` | yes |  |  |
| `disability_flag` | `varchar(1)` | no | `'N'` |  |
| `prior_educational_achievement_flag` | `varchar(1)` | no | `'N'` |  |
| `secondary_school_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;secondary_schools |
| `state_allocated_student_number` | `varchar(20)` | yes |  |  |
| `state_identity_issuing_body_code` | `varchar(3)` | yes |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `at_school_flag` | `varchar(1)` | no | `'N'` |  |
| `id_expiry_date` | `date` | yes |  |  |
| `id_document_type` | `varchar(50)` | yes |  |  |
| `id_document_number` | `varchar(50)` | yes |  |  |
| `deleted_at` | `timestamp with time zone` | yes |  |  |
| `deleted_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_students_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE`
- `CONSTRAINT fk_student_state_body FOREIGN KEY (state_identity_issuing_body_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT fk_student_school FOREIGN KEY (secondary_school_id) REFERENCES public.secondary_schools(id) ON DELETE SET NULL`
- `CONSTRAINT fk_student_school_level FOREIGN KEY (highest_school_level_id) REFERENCES public.highest_school_levels(level_id)`
- `CONSTRAINT fk_student_deleted_by FOREIGN KEY (deleted_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT uq_students_usi UNIQUE (usi)`
- `CONSTRAINT chk_usi_length CHECK (usi IS NULL OR length(usi) = 10)`
- `CONSTRAINT chk_usi_pattern CHECK (usi IS NULL OR usi ~* '^[2-9A-HJ-NP-Z]{10}$')`
- `CONSTRAINT chk_state_student_num_len CHECK (state_allocated_student_number IS NULL OR length(state_allocated_student_number) BETWEEN 5 AND 20)`
- `CONSTRAINT chk_avetmiss_indigenous CHECK (indigenous_status_id IN ('1', '2', '3', '4', '9', '@'))`
- `CONSTRAINT chk_disability_flag CHECK (disability_flag IN ('Y', 'N'))`
- `CONSTRAINT chk_prior_ed_flag CHECK (prior_educational_achievement_flag IN ('Y', 'N'))`
- `CONSTRAINT chk_english_proficiency CHECK (english_proficiency_id IN ('1', '2', '3', '4', '@'))`
- `CONSTRAINT chk_at_school_flag CHECK (at_school_flag IN ('Y', 'N'))`
- `UNIQUE INDEX uq_student_number_active (student_number) WHERE (deleted_at IS NULL)`
- `UNIQUE INDEX uq_student_email_active (student_email) WHERE (deleted_at IS NULL)`

#### `teachers`

Teacher-specific employment and capacity data. Extends `people` via a shared PK (`teachers.id = people.id`). Stores the teacher's sector (`VET`/`HE`/`DUAL`), employment status, FTE (Full-Time Equivalent — Casual = 0.00, Full-Time = 1.00, Part-Time = 0.01–0.99), annual teaching hour cap (`default_max_hours_per_year`), optional per-period hour cap for HE/DUAL teachers (`max_hours_per_period`), and faculty assignment. Police check details are stored on `people`. The hour caps seed `teacher_yearly_balances` and `teacher_period_allocations` for enforcement by triggers. Availability by day of week is stored in `teacher_availability`. Referenced by `class_slots`, `session_teachers`, `workplans`, `timesheets`, `teacher_vccs`, and `teacher_currency_activities`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;people |
| `faculty_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;faculties |
| `teacher_number` | `varchar(20)` | no |  | UK |
| `teacher_email` | `varchar(100)` | no |  | UK |
| `teacher_phone` | `varchar(15)` | yes |  |  |
| `employment_status` | `public.employment_type` | no | `'Casual'` |  |
| `fte` | `numeric(3,2)` | no | `0.00` |  |
| `sector` | `public.teacher_sector` | no | `'VET'` |  |
| `default_max_hours_per_year` | `numeric(6,2)` | no | `800.00` |  |
| `max_hours_per_period` | `numeric(6,2)` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_teachers_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE`
- `CONSTRAINT fk_teachers_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL`
- `CONSTRAINT uq_teachers_number UNIQUE (teacher_number)`
- `CONSTRAINT uq_teachers_email UNIQUE (teacher_email)`
- `CONSTRAINT chk_teacher_max_hours CHECK (default_max_hours_per_year > 0)`
- `CONSTRAINT chk_teacher_period_hours CHECK (max_hours_per_period IS NULL OR max_hours_per_period > 0)`
- `CONSTRAINT chk_teacher_fte CHECK ((employment_status='Casual' AND fte=0.00) OR (employment_status='Full-Time' AND fte=1.00) OR (employment_status='Part-Time' AND fte>0.00 AND fte<1.00))`

#### `staff`

Support and administrative staff. Extends `people` via a shared PK (`staff.id = people.id`). Holds the staff number, staff email, phone, employment status, FTE (Full-Time Equivalent), and faculty assignment. FTE is constrained by employment status: Casual = 0.00, Full-Time = 1.00, Part-Time = 0.01–0.99. Police check details are stored on `people`. Availability by day of week is stored in `staff_availability`. Staff appear as assessors on Learning Access Plans (`learning_access_plans.assessor_id`), support workers on classes (`class_support_staff`), recipients of bulk communications (`message_deliveries`), and audit actors (`app_users`).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;people |
| `faculty_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;faculties |
| `staff_number` | `varchar(20)` | no |  | UK |
| `staff_email` | `varchar(100)` | no |  | UK |
| `staff_phone` | `varchar(15)` | yes |  |  |
| `employment_status` | `public.employment_type` | no | `'Full-Time'` |  |
| `fte` | `numeric(3,2)` | no | `1.00` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_staff_people FOREIGN KEY (id) REFERENCES public.people(id) ON DELETE CASCADE`
- `CONSTRAINT fk_staff_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL`
- `CONSTRAINT uq_staff_number UNIQUE (staff_number)`
- `CONSTRAINT uq_staff_email UNIQUE (staff_email)`
- `CONSTRAINT chk_staff_fte CHECK ((employment_status='Casual' AND fte=0.00) OR (employment_status='Full-Time' AND fte=1.00) OR (employment_status='Part-Time' AND fte>0.00 AND fte<1.00))`

#### `staff_availability`

Days of the week a staff member is available to work. Each row marks one available day; days not represented are unavailable. `day_of_week` uses 0-based ISO weekday numbering: 0 = Monday, 1 = Tuesday, 2 = Wednesday, 3 = Thursday, 4 = Friday, 5 = Saturday, 6 = Sunday. The unique constraint prevents duplicate day entries per staff member.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `staff_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;staff |
| `day_of_week` | `smallint` | no |  | UK (with staff_id) |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_staff_avail FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE`
- `CONSTRAINT uq_staff_avail_day UNIQUE (staff_id, day_of_week)`
- `CONSTRAINT chk_staff_avail_day CHECK (day_of_week BETWEEN 0 AND 6)`

#### `app_users`

System login accounts and RBAC roles. Each account links to a person via `person_id` (nullable for service accounts). The `role` column drives access control (`Admin`, `Trainer`, `Compliance`, `Reception`, `SupportStaff`, `System`, `Staff`, `Student`). Every `*_by` audit column in the schema (e.g. `recorded_by`, `submitted_by`, `deleted_by`, `created_by`) references this table, making it the universal audit actor. Also used as the sender identity for `messages` and `message_campaigns`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `person_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;people |
| `username` | `varchar(100)` | no |  | UK |
| `role` | `varchar(30)` | no | `'Staff'` |  |
| `is_active` | `boolean` | no | `true` |  |
| `last_login_at` | `timestamp with time zone` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_app_user_username UNIQUE (username)`
- `CONSTRAINT fk_app_user_person FOREIGN KEY (person_id) REFERENCES public.people(id) ON DELETE SET NULL`
- `CONSTRAINT chk_app_user_role CHECK (role IN ('Admin','Trainer','Compliance','Reception','SupportStaff','System','Staff','Student'))`

#### `teacher_yearly_balances`

Cached running total of booked teaching hours per teacher per calendar year. Maintained automatically by database triggers (`fn_session_teacher_hours`, `fn_session_change_hours`) when session assignments are created, deleted, or time-edited. `allocated_max_hours` is seeded from `teachers.default_max_hours_per_year` and may be overridden per year. A `CHECK` constraint enforces that `booked_hours` never exceeds `allocated_max_hours`. Also referenced by `vw_workplan_summary` to supply actual teaching hours for CAPPS calculations.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `calendar_year` | `smallint` | no |  |  |
| `booked_hours` | `numeric(7,2)` | no | `0.00` |  |
| `allocated_max_hours` | `numeric(6,2)` | no | `800.00` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_balances_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers (id) ON DELETE CASCADE`
- `CONSTRAINT uq_teacher_year UNIQUE (teacher_id, calendar_year)`
- `CONSTRAINT chk_balance_nonneg CHECK (booked_hours >= 0)`
- `CONSTRAINT chk_balance_cap CHECK (booked_hours <= allocated_max_hours)`

#### `teacher_period_allocations`

Per-academic-period hour cap and balance for HE/DUAL teachers who have `teachers.max_hours_per_period` set. Auto-created on the first session booking for the period (via `fn_adjust_teacher_period_balance`). Enforces a secondary per-period cap in addition to the annual cap in `teacher_yearly_balances`. VET-only teachers with `max_hours_per_period IS NULL` never have rows here. References `teachers` and `academic_periods`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `academic_period_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `allocated_hours` | `numeric(6,2)` | no |  |  |
| `booked_hours` | `numeric(7,2)` | no | `0.00` |  |
| `notes` | `text` | yes |  |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_teacher_period UNIQUE (teacher_id, academic_period_id)`
- `CONSTRAINT fk_tpa_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE`
- `CONSTRAINT fk_tpa_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_tpa_allocated CHECK (allocated_hours > 0)`
- `CONSTRAINT chk_tpa_nonneg CHECK (booked_hours >= 0)`
- `CONSTRAINT chk_tpa_cap CHECK (booked_hours <= allocated_hours)`

#### `teacher_availability`

Days of the week a teacher is available to work. Each row marks one available day; days not represented are unavailable. `day_of_week` uses 0-based ISO weekday numbering: 0 = Monday, 1 = Tuesday, 2 = Wednesday, 3 = Thursday, 4 = Friday, 5 = Saturday, 6 = Sunday. The unique constraint prevents duplicate day entries per teacher.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `day_of_week` | `smallint` | no |  | UK (with teacher_id) |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_teacher_avail FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE`
- `CONSTRAINT uq_teacher_avail_day UNIQUE (teacher_id, day_of_week)`
- `CONSTRAINT chk_teacher_avail_day CHECK (day_of_week BETWEEN 0 AND 6)`

#### `student_guardians`

Guardians and emergency contacts for students. Used as communication targets for students under 18 in bulk campaigns (`message_deliveries.guardian_id`) and as recipients for direct messages (`message_recipients.guardian_id`). `is_primary` marks the primary guardian; `is_emergency_contact` marks who to call in an emergency; `receive_comms` controls whether the guardian receives automated communications. References `students`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `title` | `varchar(10)` | yes |  |  |
| `first_name` | `varchar(40)` | no |  |  |
| `family_name` | `varchar(40)` | no |  |  |
| `relationship` | `varchar(50)` | no |  |  |
| `is_primary` | `boolean` | no | `true` |  |
| `phone_mobile` | `varchar(15)` | yes |  |  |
| `phone_home` | `varchar(15)` | yes |  |  |
| `email` | `varchar(100)` | yes |  |  |
| `receive_comms` | `boolean` | no | `true` |  |
| `is_emergency_contact` | `boolean` | no | `false` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_guardian_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`

#### `student_disabilities`

Student disability declarations required for AVETMISS NAT00090 reporting. A composite-PK many-to-many join between `students` and `disability_types`; a student may declare multiple disabilities. Setting `students.disability_flag = 'Y'` and having rows here are complementary — the flag is the summary indicator; these rows carry the AVETMISS classification codes.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;students |
| `disability_id` | `varchar(2)` | no |  | PK, FK&nbsp;&rarr;&nbsp;disability_types |

*Constraints:*

- `PRIMARY KEY (student_id, disability_id)`
- `CONSTRAINT fk_stud_dis_student FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE CASCADE`
- `CONSTRAINT fk_stud_dis_type FOREIGN KEY (disability_id) REFERENCES public.disability_types (disability_id) ON DELETE RESTRICT`

#### `student_prior_achievements`

Prior educational achievement declarations required for AVETMISS NAT00100 reporting. A composite-PK many-to-many join between `students` and `prior_educational_achievements`; a student may declare multiple prior achievements. Complements `students.prior_educational_achievement_flag`, which is the AVETMISS summary indicator.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;students |
| `achievement_id` | `varchar(3)` | no |  | PK, FK&nbsp;&rarr;&nbsp;prior_educational_achievements |

*Constraints:*

- `PRIMARY KEY (student_id, achievement_id)`
- `CONSTRAINT fk_stud_ach_student FOREIGN KEY (student_id) REFERENCES public.students (id) ON DELETE CASCADE`
- `CONSTRAINT fk_stud_ach_type FOREIGN KEY (achievement_id) REFERENCES public.prior_educational_achievements (achievement_id) ON DELETE RESTRICT`

#### `australian_states`

Reference list of Australian states and territories. Holds the short state code (e.g. `VIC`), full state name, State Training Authority name, and the two-character numeric AVETMISS state identifier (e.g. `04` for SA) used when generating NAT export files. Referenced widely: by `people`, `students`, `delivery_locations`, `training_orgs`, `holiday_rules`, `holiday_observances`, `employer_workplaces`, and `state_funding_details`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `state_code` | `varchar(3)` | no |  | PK |
| `state_name` | `varchar(50)` | no |  |  |
| `state_training_authority_name` | `varchar(100)` | no |  |  |
| `avetmiss_state_id` | `char(2)` | no |  | UK |

*Constraints:*

- `PRIMARY KEY (state_code)`
- `CONSTRAINT uq_state_avetmiss_id UNIQUE (avetmiss_state_id)`

#### `disability_types`

AVETMISS-coded disability classification lookup. Each row is one disability category code and name from the AVETMISS standard. Referenced by `student_disabilities` to classify a student's declared disabilities.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `disability_id` | `varchar(2)` | no |  | PK |
| `disability_name` | `varchar(100)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (disability_id)`

#### `prior_educational_achievements`

AVETMISS-coded prior educational achievement classification lookup. Each row is one achievement category code and name from the AVETMISS standard. Referenced by `student_prior_achievements`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `achievement_id` | `varchar(3)` | no |  | PK |
| `achievement_name` | `varchar(100)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (achievement_id)`

#### `highest_school_levels`

AVETMISS-coded school completion level classification lookup. Referenced by `students.highest_school_level_id` to record the highest year of secondary schooling the student completed.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `level_id` | `varchar(2)` | no |  | PK |
| `level_name` | `varchar(100)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (level_id)`

#### `secondary_schools`

Registry of secondary schools that students may have last attended. Referenced by `students.secondary_school_id`. The `national_school_code` field holds the ACARA school identifier where known.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `school_name` | `varchar(100)` | no |  |  |
| `national_school_code` | `varchar(10)` | yes |  |  |
| `school_state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_school_state FOREIGN KEY (school_state_code) REFERENCES public.australian_states(state_code)`

#### `faculties`

Organisational units (faculties or departments) within the training organisation. Programs, teachers, and staff are assigned to a faculty for reporting and workload management purposes. Has no further parent; it is a top-level reference table.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `faculty_name` | `varchar(100)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`

### Curriculum

#### `programs`

Qualifications and courses (AVETMISS NAT00030). Each row is a single nationally-recognised or accredited qualification, skill set, or accredited course. Stores the program code, name, ANZSCO/ANZSIC industry codes, AQF level, field of education, nominal hours, VET/HE flags, total credit points (HE), and program type. Subject membership is defined in `subject_programs`. Owned by a faculty. Referenced by `student_course_enrollments`, `program_completions`, `workplan_entries`, `teacher_vcc_courses`, `message_campaigns`, and the NAT00030 export.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `faculty_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;faculties |
| `program_code` | `varchar(10)` | no |  | UK |
| `program_name` | `varchar(100)` | no |  |  |
| `program_recognition_id` | `varchar(2)` | no |  |  |
| `level_of_education` | `varchar(3)` | no |  |  |
| `field_of_education` | `varchar(4)` | no |  |  |
| `anzsco_code` | `varchar(6)` | yes |  |  |
| `anzsic_code` | `varchar(4)` | yes |  |  |
| `nominal_hours` | `integer` | no |  |  |
| `vet_flag` | `boolean` | no | `true` |  |
| `he_flag` | `boolean` | no | `false` |  |
| `credit_points` | `integer` | yes |  |  |
| `aqf_level` | `smallint` | yes |  |  |
| `program_type` | `varchar(20)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_programs_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE RESTRICT`
- `CONSTRAINT uq_programs_code UNIQUE (program_code)`
- `CONSTRAINT chk_program_credit_points CHECK (credit_points IS NULL OR credit_points > 0)`
- `CONSTRAINT chk_program_aqf_level CHECK (aqf_level IS NULL OR aqf_level BETWEEN 1 AND 10)`
- `CONSTRAINT chk_program_sector CHECK (vet_flag = true OR he_flag = true)`
- `CONSTRAINT chk_program_type CHECK (program_type IS NULL OR program_type IN ('Qualification','Skill Set','Course in a Package','Statement of Attainment','Accredited Course'))`

#### `subjects`

Units of competency, modules, or subjects (AVETMISS NAT00060). Each row is a single deliverable training unit with its national subject code, name, module flag, field of education, nominal hours (VET), and credit points (HE). Subjects belong to one or more programs via `subject_programs` and may also be enrolled as standalone units in `client_subject_enrolments`. Referenced by `class_subjects`, `workplan_entries`, `teacher_vcc_units`, `teacher_currency_unit_links`, `enrollment_credit_claims`, and the NAT00060 export.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `subject_code` | `varchar(30)` | no |  | UK |
| `subject_name` | `varchar(100)` | no |  |  |
| `module_flag` | `varchar(1)` | no | `'N'` |  |
| `field_of_education` | `varchar(6)` | no |  |  |
| `nominal_hours` | `integer` | yes |  |  |
| `vet_flag` | `boolean` | no | `true` |  |
| `credit_points` | `integer` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_subjects_code UNIQUE (subject_code)`
- `CONSTRAINT chk_module_flag CHECK (module_flag IN ('Y', 'N'))`
- `CONSTRAINT chk_subject_credit_points CHECK (credit_points IS NULL OR credit_points > 0)`

#### `subject_programs`

Many-to-many join between `subjects` and `programs` defining which units belong to which qualification. `is_core` flags mandatory units; `group_code`/`group_title` support unit grouping within a qualification (e.g. elective clusters). The same subject may appear in multiple programs; a program contains many subjects. This table is the curriculum packaging layer — it does not record enrolment or delivery.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `subject_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;subjects |
| `program_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;programs |
| `is_core` | `boolean` | no | `false` |  |
| `group_code` | `varchar(20)` | yes |  |  |
| `group_title` | `varchar(100)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (subject_id, program_id)`
- `CONSTRAINT fk_sp_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE CASCADE`
- `CONSTRAINT fk_sp_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE CASCADE`

### Intakes & cohorts

#### `program_intakes`

A scheduled offering of a program — the concrete instance of when, where, and how a program is delivered to a cohort of students. One program may have many intakes across different periods, locations, and study modes (e.g. Cert III IT delivered each term, full-time, at the city campus). `intake_code` is the unique human-readable identifier; `duration_periods` records how many academic periods it takes to complete the program on this intake; `duration_years` stores the equivalent calendar duration in years (e.g. `1.0`, `1.5`, `2.0`) for display and reporting. `graded_assessment` flags whether this intake requires a grade (e.g. P, CR, D, HD) alongside the standard VET competency outcome (SC / NS) for all enrolled subjects — the actual grade is stored in `client_subject_enrolments.grade`. `start_academic_period_id` is the period the first class is taught. Enrolment window dates are optional and informational. References `programs`, `academic_periods`, `delivery_locations`, and optionally `faculties`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `program_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;programs |
| `intake_code` | `varchar(30)` | no |  | UK |
| `intake_name` | `varchar(150)` | no |  |  |
| `start_academic_period_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `delivery_location_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;delivery_locations |
| `faculty_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;faculties |
| `study_mode` | `varchar(10)` | no | `'Full-Time'` |  |
| `duration_periods` | `smallint` | no |  |  |
| `duration_years` | `numeric(3,1)` | yes |  |  |
| `graded_assessment` | `boolean` | no | `false` |  |
| `enrolment_open_date` | `date` | yes |  |  |
| `enrolment_close_date` | `date` | yes |  |  |
| `status` | `varchar(20)` | no | `'Planned'` |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_intake_code UNIQUE (intake_code)`
- `CONSTRAINT fk_intake_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_intake_period FOREIGN KEY (start_academic_period_id) REFERENCES public.academic_periods(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_intake_location FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_intake_faculty FOREIGN KEY (faculty_id) REFERENCES public.faculties(id) ON DELETE SET NULL`
- `CONSTRAINT chk_intake_study_mode CHECK (study_mode IN ('Full-Time', 'Part-Time'))`
- `CONSTRAINT chk_intake_duration CHECK (duration_periods > 0)`
- `CONSTRAINT chk_intake_duration_years CHECK (duration_years IS NULL OR duration_years > 0)`
- `CONSTRAINT chk_intake_status CHECK (status IN ('Planned', 'Active', 'Closed', 'Cancelled'))`
- `CONSTRAINT chk_intake_enrolment_dates CHECK (open IS NULL OR close IS NULL OR close >= open)`

#### `intake_groups`

Sub-cohorts within an intake that attend classes together throughout the program. A single intake may split into multiple groups with different timetables (e.g. Group A on Monday/Wednesday, Group B on Tuesday/Thursday). Students are assigned to a group via `student_course_enrollments.intake_group_id`; classes are linked to a group via `classes.intake_group_id`. `group_code` is short and unique within the intake (e.g. `A`, `B`, `MON`); `group_name` is the display label. Optional `capacity` caps enrolment into the group. Cascade-deletes when its parent intake is deleted.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `intake_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;program_intakes |
| `group_code` | `varchar(20)` | no |  |  |
| `group_name` | `varchar(100)` | no |  |  |
| `capacity` | `integer` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_group_per_intake UNIQUE (intake_id, group_code)`
- `CONSTRAINT fk_ig_intake FOREIGN KEY (intake_id) REFERENCES public.program_intakes(id) ON DELETE CASCADE`
- `CONSTRAINT chk_ig_capacity CHECK (capacity IS NULL OR capacity > 0)`

### Enrolment & extensions

#### `student_course_enrollments`

Program-level enrolment record — one row per student per qualification attempt. Records commencement and completion dates, enrollment status, funding state, and optional apprenticeship/traineeship contract identifiers. Anchor for all subject-level enrolments (`client_subject_enrolments.student_course_enrollment_id`) and for enrolment extensions (apprenticeship details, training plans, VSL, HE details, etc.). Soft-deletable to preserve AVETMISS reporting history; a partial unique index prevents two active enrolments in the same program. Referenced by all extension tables and by `state_funding_details`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `program_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;programs |
| `intake_group_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;intake_groups |
| `enrollment_status` | `varchar(20)` | no | `'Active'` |  |
| `commencement_date` | `date` | no |  |  |
| `commencing_program_id` | `varchar(1)` | no | `'3'` |  |
| `completion_date` | `date` | yes |  |  |
| `funding_state_code` | `varchar(3)` | no | `'VIC'` | FK&nbsp;&rarr;&nbsp;australian_states |
| `training_contract_id` | `varchar(20)` | yes |  |  |
| `client_apprenticeship_id` | `varchar(20)` | yes |  |  |
| `deleted_at` | `timestamp with time zone` | yes |  |  |
| `deleted_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_enrollment_state FOREIGN KEY (funding_state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT fk_sce_deleted_by FOREIGN KEY (deleted_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT fk_sce_intake_group FOREIGN KEY (intake_group_id) REFERENCES public.intake_groups(id) ON DELETE SET NULL`
- `CONSTRAINT chk_enrollment_status CHECK (enrollment_status IN ('Active', 'Deferred', 'Suspended', 'Cancelled', 'Completed'))`
- `CONSTRAINT chk_commencing_program_id CHECK (commencing_program_id IN ('3', '4', '8'))`
- `CONSTRAINT fk_se_student FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE RESTRICT`
- `CONSTRAINT fk_se_program FOREIGN KEY (program_id) REFERENCES programs (id) ON DELETE RESTRICT`
- `UNIQUE INDEX idx_uq_active_course_enrollment (student_id, program_id) WHERE (enrollment_status IN ('Active', 'Deferred', 'Suspended'))`

#### `client_subject_enrolments`

Subject-level training activity record (AVETMISS NAT00120) — the key operational table for attendance and results tracking. One row per student per subject delivery. `student_course_enrollment_id` is nullable to allow standalone unit enrolments independent of a program. Stores scheduled hours, delivery mode, funding source, national outcome code, optional grade/mark, result status, and finalisation audit fields. The result status (`In Progress` → `Draft` → `Under Review` → `Finalised`) models the result publication workflow. `class_enrollments` links these rows to specific timetabled classes; `session_attendance` is populated for enrolled students via that mapping.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `student_course_enrollment_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `subject_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;subjects |
| `delivery_location_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;delivery_locations |
| `activity_start_date` | `date` | no |  |  |
| `activity_end_date` | `date` | no |  |  |
| `delivery_mode_id` | `varchar(3)` | no | `'YNN'` |  |
| `predominant_delivery_mode` | `varchar(1)` | no | `'I'` |  |
| `vet_in_schools_flag` | `varchar(1)` | no | `'N'` |  |
| `commencing_program_id` | `varchar(1)` | no | `'8'` |  |
| `scheduled_hours` | `numeric(5,2)` | no |  |  |
| `funding_source_national` | `varchar(2)` | no |  |  |
| `outcome_id_national` | `varchar(2)` | no | `'70'` |  |
| `specific_funding_id` | `varchar(2)` | yes |  |  |
| `outcome_id_training_org` | `varchar(10)` | yes |  |  |
| `funding_source_state` | `varchar(3)` | yes |  |  |
| `client_tuition_fee` | `numeric(8,2)` | yes |  |  |
| `fee_exemption_type_id` | `varchar(2)` | yes |  |  |
| `purchasing_contract_id` | `varchar(30)` | yes |  |  |
| `purchasing_contract_schedule_id` | `varchar(30)` | yes |  |  |
| `hours_attended` | `numeric(5,2)` | yes |  |  |
| `associated_course_id` | `varchar(10)` | yes |  |  |
| `grade` | `varchar(20)` | yes |  |  |
| `mark` | `numeric(5,2)` | yes |  |  |
| `finalised_date` | `date` | yes |  |  |
| `result_status` | `varchar(20)` | no | `'In Progress'` |  |
| `result_finalised_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `result_finalised_at` | `timestamp with time zone` | yes |  |  |
| `result_amended_at` | `timestamp with time zone` | yes |  |  |
| `result_amendment_reason` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_cse_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_cse_delivery_loc FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_cse_finalised_by FOREIGN KEY (result_finalised_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT uq_cse_unit_per_enrollment UNIQUE (student_course_enrollment_id, subject_id)`
- `CONSTRAINT chk_activity_dates CHECK (activity_end_date >= activity_start_date)`
- `CONSTRAINT chk_predominant_mode CHECK (predominant_delivery_mode IN ('I', 'E', 'W', 'N'))`
- `CONSTRAINT chk_delivery_mode_len CHECK (length(delivery_mode_id) = 3)`
- `CONSTRAINT chk_vet_in_schools_flag CHECK (vet_in_schools_flag IN ('Y', 'N'))`
- `CONSTRAINT chk_tuition_fee_positive CHECK (client_tuition_fee >= 0)`
- `CONSTRAINT chk_hours_attended_positive CHECK (hours_attended >= 0)`
- `CONSTRAINT chk_mark_range CHECK (mark IS NULL OR (mark BETWEEN 0.00 AND 100.00))`
- `CONSTRAINT chk_result_workflow CHECK (result_status IN ('In Progress', 'Draft', 'Under Review', 'Finalised', 'Appealed', 'Amended'))`
- `CONSTRAINT fk_cse_student FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE RESTRICT`
- `CONSTRAINT fk_cse_subject FOREIGN KEY (subject_id) REFERENCES subjects (id) ON DELETE RESTRICT`
- `UNIQUE INDEX idx_uq_standalone_unit (student_id, subject_id, activity_start_date) WHERE (student_course_enrollment_id IS NULL)`

#### `apprenticeship_details`

Apprenticeship contract extension for a program enrolment (1:1 shared PK with `student_course_enrollments`). Records the employer, specific workplace, AASN provider, DELTA registration number, TYIMS number, school-based flag, and the sequence of training-plan signature dates. References `employers`, `employer_workplaces`, and `aasn_providers`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_course_enrollment_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `employer_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;employers |
| `workplace_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;employer_workplaces |
| `aasn_provider_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;aasn_providers |
| `delta_registration_number` | `varchar(30)` | yes |  |  |
| `tyims_number` | `varchar(30)` | yes |  |  |
| `training_plan_drafted_date` | `date` | yes |  |  |
| `training_plan_employer_signed_date` | `date` | yes |  |  |
| `training_plan_student_signed_date` | `date` | yes |  |  |
| `training_plan_rto_signed_date` | `date` | yes |  |  |
| `training_plan_fully_executed_date` | `date` | yes |  |  |
| `is_school_based_apprenticeship` | `boolean` | no | `false` |  |

*Constraints:*

- `PRIMARY KEY (student_course_enrollment_id)`
- `CONSTRAINT fk_app_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT fk_app_employer FOREIGN KEY (employer_id) REFERENCES public.employers(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_app_workplace FOREIGN KEY (workplace_id) REFERENCES public.employer_workplaces(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_app_aasn FOREIGN KEY (aasn_provider_id) REFERENCES public.aasn_providers(id) ON DELETE SET NULL`

#### `traineeship_details`

Traineeship extension for a program enrolment (1:1 shared PK with `student_course_enrollments`). Records worker classification (New Worker or Existing Worker), probation start/end dates, probation clearance status, and STA-approved extension details if the traineeship was extended.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_course_enrollment_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `worker_classification` | `public.trainee_worker_type` | no | `'New Worker'` |  |
| `probation_start_date` | `date` | no |  |  |
| `probation_end_date` | `date` | no |  |  |
| `probation_cleared` | `boolean` | no | `false` |  |
| `has_approved_extension` | `boolean` | no | `false` |  |
| `extension_approved_date` | `date` | yes |  |  |
| `extension_revised_end_date` | `date` | yes |  |  |
| `sta_extension_reference` | `varchar(50)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (student_course_enrollment_id)`
- `CONSTRAINT fk_trainee_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT chk_probation_dates CHECK (probation_end_date >= probation_start_date)`
- `CONSTRAINT chk_extension_logic CHECK ( (has_approved_extension = false) OR (has_approved_extension = true AND extension_approved_date IS NOT NULL AND extension_revised_end_date IS NOT NULL) )`

#### `training_plans`

Training plan extension for a program enrolment (1:1 via unique FK to `student_course_enrollments`). Records the drafted, student-signed, RTO-signed, and fully-executed dates for the formalised training plan document, plus any review date and delivery strategy notes.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_course_enrollment_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments, UK |
| `drafted_date` | `date` | yes |  |  |
| `student_signed_date` | `date` | yes |  |  |
| `rto_signed_date` | `date` | yes |  |  |
| `fully_executed_date` | `date` | yes |  |  |
| `review_date` | `date` | yes |  |  |
| `delivery_strategy` | `text` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_training_plan_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT uq_training_plan_enrollment UNIQUE (student_course_enrollment_id)`

#### `learning_access_plans`

Disability and learning adjustment plans for students. Records the disability type codes declared, required adjustments, resources provided, student consent, and assessing staff member. May be linked to a specific program enrolment or be student-wide (when `student_course_enrollment_id IS NULL`). Progresses through Draft → Active → Under Review → Closed. References `students`, `student_course_enrollments`, and `staff`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `student_course_enrollment_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `plan_date` | `date` | no |  |  |
| `review_date` | `date` | yes |  |  |
| `disability_type_codes` | `varchar(2)[]` | no |  |  |
| `adjustments_required` | `text` | no |  |  |
| `resources_provided` | `text` | yes |  |  |
| `assessor_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;staff |
| `student_consent` | `boolean` | no | `false` |  |
| `status` | `varchar(20)` | no | `'Active'` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_lap_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_lap_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE SET NULL`
- `CONSTRAINT chk_lap_status CHECK (status IN ('Draft', 'Active', 'Under Review', 'Closed'))`
- `CONSTRAINT fk_lap_assessor FOREIGN KEY (assessor_id) REFERENCES staff (id) ON DELETE RESTRICT`

#### `vet_student_loans`

VET Student Loan (VSL) or VET-FEE-HELP records per census date, linked to a program enrolment. Multiple rows per enrolment are permitted (one per census date). Records the loan type, census date, loan amount, and re-credit details if the loan was refunded. References `student_course_enrollments`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_course_enrollment_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `loan_type` | `varchar(20)` | no |  |  |
| `census_date` | `date` | no |  |  |
| `loan_amount` | `numeric(10,2)` | no |  |  |
| `re_credit_flag` | `boolean` | no | `false` |  |
| `re_credit_date` | `date` | yes |  |  |
| `re_credit_reason` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_vsl_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT uq_vsl_enrol_census UNIQUE (student_course_enrollment_id, census_date)`
- `CONSTRAINT chk_vsl_type CHECK (loan_type IN ('VSL', 'VET-FEE-HELP'))`
- `CONSTRAINT chk_vsl_amount CHECK (loan_amount >= 0)`

#### `he_enrolment_details`

Higher Education enrolment details extension (1:1 shared PK with `student_course_enrollments`). Records EFTSL, census date, HECS-HELP eligibility, fee type, study load category, mode of attendance, basis for admission, and credit points enrolled. `academic_period_id` links the HE enrolment to the specific semester or trimester.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_course_enrollment_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `academic_period_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `eftsl` | `numeric(5,4)` | no |  |  |
| `census_date` | `date` | no |  |  |
| `hecs_help_eligible` | `boolean` | no | `false` |  |
| `fee_type` | `varchar(20)` | yes |  |  |
| `study_load_category` | `varchar(20)` | yes |  |  |
| `mode_of_attendance` | `varchar(30)` | yes |  |  |
| `basis_for_admission` | `varchar(10)` | yes |  |  |
| `credit_points_enrolled` | `smallint` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (student_course_enrollment_id)`
- `CONSTRAINT fk_he_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT fk_he_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE SET NULL`
- `CONSTRAINT chk_he_fee_type CHECK (fee_type IN ('HECS-HELP', 'FEE-HELP', 'DOMESTIC-FULL', 'INTERNATIONAL', 'EXEMPT'))`
- `CONSTRAINT chk_he_load CHECK (study_load_category IN ('Full-Time', 'Part-Time', 'Less Than Half-Time'))`
- `CONSTRAINT chk_he_mode CHECK (mode_of_attendance IN ('Internal', 'External', 'Multi-Modal'))`
- `CONSTRAINT chk_he_credit_points CHECK (credit_points_enrolled IS NULL OR credit_points_enrolled > 0)`

#### `enrollment_credit_claims`

RPL and credit transfer grants within a program enrolment. Each row grants credit for one subject within the enrolment, recording the claim type (RPL or Credit Transfer), grant date, nominal hours deducted, tuition fee adjustment, and evidence document reference. The unique constraint prevents duplicate claims for the same subject within the same enrolment.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_course_enrollment_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `subject_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;subjects |
| `claim_type` | `public.credit_type` | no |  |  |
| `granted_date` | `date` | no |  |  |
| `hours_deducted` | `numeric(5,2)` | no |  |  |
| `tuition_fee_adjustment` | `numeric(8,2)` | no | `0.00` |  |
| `evidence_document_reference` | `varchar(255)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_enrollment_subject_credit UNIQUE (student_course_enrollment_id, subject_id)`
- `CONSTRAINT fk_credit_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT fk_credit_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE RESTRICT`

#### `state_funding_details`

State-specific funding attributes stored as a flexible `jsonb` payload, keyed by enrolment and state. Captures Skills First (VIC), Smart & Skilled (NSW), or other jurisdiction-specific funding fields without requiring schema changes for each state program. The unique constraint ensures at most one funding record per enrolment per state.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_course_enrollment_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `attributes` | `jsonb` | no | `'{}'::jsonb` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_sfd_enrollment FOREIGN KEY (student_course_enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE CASCADE`
- `CONSTRAINT fk_sfd_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT uq_sfd_per_enrolment_state UNIQUE (student_course_enrollment_id, state_code)`

### RTO infrastructure

#### `training_orgs`

The Registered Training Organisation (AVETMISS NAT00010). Typically a single row per database instance identifying the RTO with its national RTO number, name, type, address, logo URL, and contact details. Referenced by `delivery_locations`, `program_completions`, `avetmiss_submissions`, and the NAT00010 export.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `training_org_id` | `varchar(30)` | no |  | UK |
| `training_org_name` | `varchar(100)` | no |  |  |
| `training_org_type` | `varchar(2)` | no |  |  |
| `address_first_line` | `varchar(50)` | no |  |  |
| `address_second_line` | `varchar(50)` | yes |  |  |
| `suburb` | `varchar(50)` | no |  |  |
| `state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `postcode` | `varchar(4)` | no |  |  |
| `logo_url` | `varchar(2048)` | yes |  |  |
| `contact_name` | `varchar(100)` | yes |  |  |
| `telephone` | `varchar(20)` | yes |  |  |
| `facsimile` | `varchar(20)` | yes |  |  |
| `email` | `varchar(100)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_org_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT uq_training_org_code UNIQUE (training_org_id)`

#### `delivery_locations`

Campus or delivery site locations (AVETMISS NAT00020). Each location belongs to a `training_org` and has its own NAT delivery location ID, name, and address. Referenced by `classes` (where training is delivered) and `client_subject_enrolments` (where a unit was delivered). Parent of `buildings` → `rooms`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `training_org_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;training_orgs |
| `delivery_loc_id` | `varchar(30)` | no |  |  |
| `name` | `varchar(100)` | no |  |  |
| `address` | `text` | no |  |  |
| `suburb` | `varchar(50)` | no |  |  |
| `state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `postcode` | `varchar(4)` | no |  |  |
| `postcode_override` | `varchar(4)` | yes |  |  |
| `country_id` | `varchar(4)` | no | `'1101'` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_loc_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT uq_delivery_loc_per_org UNIQUE (training_org_id, delivery_loc_id)`
- `CONSTRAINT fk_delivery_loc_parent FOREIGN KEY (training_org_id) REFERENCES training_orgs (id) ON DELETE CASCADE`

#### `person_location_preferences`

A person's ranked delivery location preferences. Each row records one location with a `preference_rank` (1 = most preferred). Equal ranks are permitted — two locations can both be rank 2. One row per person+location pair. References `people` and `delivery_locations`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `person_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;people |
| `delivery_location_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;delivery_locations |
| `preference_rank` | `smallint` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_person_location_pref UNIQUE (person_id, delivery_location_id)`
- `CONSTRAINT fk_plp_person FOREIGN KEY (person_id) REFERENCES public.people(id) ON DELETE CASCADE`
- `CONSTRAINT fk_plp_delivery_location FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id) ON DELETE CASCADE`
- `CONSTRAINT chk_plp_rank CHECK (preference_rank >= 1)`

*Indexes:* `idx_plp_person (person_id)`

#### `buildings`

Physical buildings within a delivery location. Groups rooms for timetabling. References `delivery_locations`; parent of `rooms`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `delivery_location_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;delivery_locations |
| `building_name` | `varchar(50)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_building_per_campus UNIQUE (delivery_location_id, building_name)`
- `CONSTRAINT fk_building_parent FOREIGN KEY (delivery_location_id) REFERENCES delivery_locations (id) ON DELETE CASCADE`

#### `rooms`

Classrooms and other spaces within a building. Each room has a capacity, type (Classroom, Lab, Workshop, etc.), and active flag. `is_computer_lab` marks rooms that have a computer lab hardware profile in `room_computer_lab_specs`. Referenced by `class_slots` and `class_sessions` for double-booking prevention via exclusion constraints.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `building_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;buildings |
| `room_name` | `varchar(50)` | no |  |  |
| `capacity` | `integer` | no |  |  |
| `room_type` | `varchar(30)` | no | `'Classroom'` |  |
| `is_active` | `boolean` | no | `true` |  |
| `is_computer_lab` | `boolean` | no | `false` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_room_per_building UNIQUE (building_id, room_name)`
- `CONSTRAINT chk_rooms_capacity CHECK (capacity > 0)`
- `CONSTRAINT fk_room_parent FOREIGN KEY (building_id) REFERENCES buildings (id) ON DELETE CASCADE`

#### `room_computer_lab_specs`

Hardware profile for a computer lab room — one row per room where `is_computer_lab = true`. Stores workstation count, per-workstation RAM, and peripheral flags. References `rooms`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `room_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;rooms |
| `workstations` | `smallint` | yes |  |  |
| `ram_gb` | `smallint` | yes |  |  |
| `has_microphone` | `boolean` | no | `false` |  |
| `has_webcam` | `boolean` | no | `false` |  |
| `notes` | `text` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_lab_specs_room UNIQUE (room_id)`
- `CONSTRAINT fk_lab_specs_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE`

#### `room_lab_software`

Software titles installed in a computer lab room. Multiple rows per room. `version` and `licence_type` are optional. References `rooms`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `room_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;rooms |
| `software_name` | `varchar(150)` | no |  |  |
| `version` | `varchar(50)` | yes |  |  |
| `licence_type` | `varchar(50)` | yes |  |  |
| `notes` | `text` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_lab_software_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE`

*Indexes:* `idx_lab_software_room (room_id)`

#### `room_issues`

Faults, maintenance requests, and AV/equipment problems reported for any room. Status moves through Open → Investigating → Resolved. `resolved_at` is set when status reaches Resolved. References `rooms`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `room_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;rooms |
| `title` | `varchar(200)` | no |  |  |
| `description` | `text` | yes |  |  |
| `status` | `varchar(20)` | no | `'Open'` |  |
| `reported_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `resolved_at` | `timestamp with time zone` | yes |  |  |
| `notes` | `text` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_room_issue_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE`
- `CONSTRAINT chk_room_issue_status CHECK (status IN ('Open', 'Investigating', 'Resolved'))`

*Indexes:* `idx_room_issues_room (room_id)` · `idx_room_issues_status (room_id, status)`

#### `employers`

Organisations that employ apprentices or trainees. Identified by ABN. Referenced by `apprenticeship_details` via `employer_workplaces`. Parent of `employer_workplaces`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `legal_name` | `varchar(100)` | no |  |  |
| `trading_name` | `varchar(100)` | yes |  |  |
| `abn` | `varchar(11)` | no |  | UK |
| `contact_person` | `varchar(100)` | yes |  |  |
| `contact_phone` | `varchar(15)` | yes |  |  |
| `contact_email` | `varchar(100)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_employer_abn UNIQUE (abn)`
- `CONSTRAINT chk_abn_length CHECK (abn ~ '^[0-9]{11}$')`

#### `employer_workplaces`

Individual workplace sites within an employer's business. The specific workplace is recorded on each `apprenticeship_details` row to identify where an apprentice is working. References `employers` and `australian_states`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `employer_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;employers |
| `workplace_name` | `varchar(100)` | no |  |  |
| `address` | `text` | no |  |  |
| `suburb` | `varchar(50)` | no |  |  |
| `state_code` | `varchar(3)` | no |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `postcode` | `varchar(4)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_workplace_employer FOREIGN KEY (employer_id) REFERENCES public.employers(id) ON DELETE CASCADE`
- `CONSTRAINT fk_workplace_state FOREIGN KEY (state_code) REFERENCES public.australian_states(state_code)`
- `CONSTRAINT chk_workplace_postcode CHECK (postcode ~ '^[0-9]{4}$')`

#### `aasn_providers`

Australian Apprenticeship Support Network providers. Referenced by `apprenticeship_details.aasn_provider_id` to record the AASN body managing the apprenticeship contract.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `provider_name` | `varchar(100)` | no |  | UK |
| `national_identifier` | `varchar(10)` | yes |  |  |
| `contact_email` | `varchar(100)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_aasn_name UNIQUE (provider_name)`

### Timetabling

#### `academic_periods`

Terms, semesters, trimesters, years, blocks, or rolling periods with calendar date ranges. The structural container for classes and the scope reference for class slot exclusion constraints (teacher/room double-booking is scoped per period). `period_type` supports `TERM`, `SEMESTER`, `TRIMESTER`, `YEAR`, `BLOCK` (6–8 week intensive), and `ROLLING` (monthly intake). `sequence_number` orders periods within a year for display. Inserting a new academic period triggers `fn_materialise_holidays_for_period` to auto-expand holiday rules for that year.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `period_code` | `varchar(20)` | no |  | UK |
| `year` | `smallint` | no |  |  |
| `period_name` | `varchar(50)` | no |  |  |
| `start_date` | `date` | no |  |  |
| `end_date` | `date` | no |  |  |
| `period_type` | `varchar(10)` | no |  |  |
| `sequence_number` | `smallint` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_academic_period_code UNIQUE (period_code)`
- `CONSTRAINT chk_period_type CHECK (period_type IN ('TERM', 'SEMESTER', 'TRIMESTER', 'YEAR', 'BLOCK', 'ROLLING'))`
- `CONSTRAINT chk_period_dates CHECK (end_date >= start_date)`
- `CONSTRAINT chk_sequence_number CHECK (sequence_number IS NULL OR sequence_number > 0)`

#### `classes`

A delivery instance of a program within an academic period at a delivery location. The central timetabling entity: all of `class_subjects`, `class_enrollments`, `class_slots`, `class_sessions`, `class_support_staff`, and `class_exceptions` attach to a class. `intake_group_id` links the class to the specific cohort group that attends it; `enrolment_cap` optionally limits enrolment. Also targeted by `message_campaigns` for class-level bulk communications.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_code` | `varchar(80)` | no |  | UK |
| `group_code` | `varchar(20)` | yes |  |  |
| `intake_group_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;intake_groups |
| `academic_period_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `delivery_location_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;delivery_locations |
| `enrolment_cap` | `integer` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_class_code UNIQUE (class_code)`
- `CONSTRAINT fk_class_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id)`
- `CONSTRAINT fk_class_location FOREIGN KEY (delivery_location_id) REFERENCES public.delivery_locations(id)`
- `CONSTRAINT fk_class_intake_group FOREIGN KEY (intake_group_id) REFERENCES public.intake_groups(id) ON DELETE SET NULL`
- `CONSTRAINT chk_class_cap CHECK (enrolment_cap > 0)`

#### `class_subjects`

Which subjects a class delivers. A class may deliver one or more subjects simultaneously; a subject may be taught across multiple classes. `subject_label` is a display-override for the subject name in the context of this class. References `classes` and `subjects`. Used by the attendance register to list subjects students are attending.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `class_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;classes |
| `subject_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;subjects |
| `subject_label` | `varchar(100)` | no |  |  |

*Constraints:*

- `PRIMARY KEY (class_id, subject_id)`
- `CONSTRAINT fk_cl_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE`
- `CONSTRAINT fk_cl_subject FOREIGN KEY (subject_id) REFERENCES subjects (id) ON DELETE CASCADE`

#### `class_enrollments`

Bridge between the enrolment chain and the timetabling chain. Maps a student's subject enrolment (`client_subject_enrolments`) to a specific class so the student appears on that class's attendance register. `session_attendance` rows are populated for students who have a `class_enrollment` in a class when sessions are taken. References `classes` and `client_subject_enrolments`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;classes |
| `client_subject_enrolment_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;client_subject_enrolments |
| `enrolled_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_class_enrolment_map UNIQUE (class_id, client_subject_enrolment_id)`
- `CONSTRAINT fk_ce_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE`
- `CONSTRAINT fk_ce_subject_enrolment FOREIGN KEY (client_subject_enrolment_id) REFERENCES client_subject_enrolments (id) ON DELETE CASCADE`

#### `class_slots`

Recurring weekly template entries for a class. Each row defines one regular teaching slot by weekday, start/end time, assigned teacher, optional room, and the academic period it belongs to. Exclusion constraints on `(academic_period_id, teacher_id, day_of_week, timerange)` and `(academic_period_id, room_id, day_of_week, timerange)` prevent teacher and room double-booking within the same period. Expanded into concrete `class_sessions` by calling `fn_generate_sessions`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;classes |
| `academic_period_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `room_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;rooms |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `day_of_week` | `smallint` | no |  |  |
| `start_time` | `time WITHOUT TIME ZONE` | no |  |  |
| `end_time` | `time WITHOUT TIME ZONE` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT chk_slots_day CHECK (day_of_week BETWEEN 1 AND 7)`
- `CONSTRAINT chk_slots_times CHECK (end_time > start_time)`
- `CONSTRAINT no_teacher_double_booking EXCLUDE USING gist ( academic_period_id WITH =, teacher_id WITH =, day_of_week WITH =, timerange(start_time, end_time) WITH && )`
- `CONSTRAINT no_room_double_booking EXCLUDE USING gist ( academic_period_id WITH =, room_id WITH =, day_of_week WITH =, timerange(start_time, end_time) WITH && ) WHERE (room_id IS NOT NULL)`
- `CONSTRAINT fk_cs_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE`
- `CONSTRAINT fk_cs_period FOREIGN KEY (academic_period_id) REFERENCES academic_periods (id)`
- `CONSTRAINT fk_cs_teacher FOREIGN KEY (teacher_id) REFERENCES teachers (id) ON DELETE RESTRICT`
- `CONSTRAINT fk_cs_room FOREIGN KEY (room_id) REFERENCES rooms (id) ON DELETE SET NULL`

#### `class_sessions`

Concrete dated teaching sessions — the authoritative source of truth for teaching hours, attendance, workload, and payroll. Each row carries the actual date, start/end time, room, session type, cancelled flag, and cancel reason. Teacher hour balances (`teacher_yearly_balances`, `teacher_period_allocations`) and timesheet entries are derived from this table via triggers, not from the `class_slots` template. Referenced by `session_teachers`, `session_attendance`, `workplan_entries`, and `timesheet_entries`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;classes |
| `session_date` | `date` | no |  |  |
| `start_time` | `time WITHOUT TIME ZONE` | no |  |  |
| `end_time` | `time WITHOUT TIME ZONE` | no |  |  |
| `room_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;rooms |
| `session_type` | `varchar(20)` | no | `'Scheduled'` |  |
| `notes` | `text` | yes |  |  |
| `cancelled` | `boolean` | no | `false` |  |
| `cancel_reason` | `varchar(255)` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_session_class FOREIGN KEY (class_id) REFERENCES public.classes(id) ON DELETE CASCADE`
- `CONSTRAINT fk_session_room FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE SET NULL`
- `CONSTRAINT uq_session_natural UNIQUE (class_id, session_date, start_time)`
- `CONSTRAINT chk_session_times CHECK (end_time > start_time)`
- `CONSTRAINT chk_session_type CHECK (session_type IN ('Scheduled', 'Replacement', 'Assessment', 'Online', 'Other'))`
- `CONSTRAINT no_room_session_double_booking EXCLUDE USING gist ( room_id WITH =, session_date WITH =, timerange(start_time, end_time) WITH && ) WHERE (room_id IS NOT NULL)`

#### `session_teachers`

Teachers assigned to a session and their role on that session (Lead, Support, Guest, Assessor). Non-Guest roles trigger hour accrual in both `teacher_yearly_balances` and `teacher_period_allocations` via the `fn_session_teacher_hours` trigger. Supports team teaching (multiple non-guest teachers per session each accrue hours). References `class_sessions` and `teachers`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `session_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;class_sessions |
| `teacher_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;teachers |
| `role` | `varchar(30)` | no | `'Lead'` |  |

*Constraints:*

- `PRIMARY KEY (session_id, teacher_id)`
- `CONSTRAINT fk_se_teach_session FOREIGN KEY (session_id) REFERENCES public.class_sessions(id) ON DELETE CASCADE`
- `CONSTRAINT fk_se_teach_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_session_teacher_role CHECK (role IN ('Lead', 'Support', 'Guest', 'Assessor'))`

#### `session_attendance`

Per-student attendance record for a session. One row per student per session; the unique constraint on `(session_id, student_id)` enforces this. Stores the attendance status, minutes attended, arrival/departure times, break duration, absence reason and acceptability, childcare flag, and optional private notes. The extended fields (arrived_at, departed_at, break_minutes, has_childcare, etc.) support the detailed attendance dialog in the web application. References `class_sessions`, `students`, and `app_users` (recorder).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `session_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;class_sessions |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `status` | `varchar(20)` | no | `'Present'` |  |
| `minutes_attended` | `integer` | yes |  |  |
| `units_nominated` | `smallint` | no | `0` |  |
| `arrived_at` | `time WITHOUT TIME ZONE` | yes |  |  |
| `departed_at` | `time WITHOUT TIME ZONE` | yes |  |  |
| `break_minutes` | `smallint` | no | `0` |  |
| `absence_reason` | `varchar(100)` | yes |  |  |
| `absence_is_acceptable` | `boolean` | no | `false` |  |
| `has_childcare` | `boolean` | no | `false` |  |
| `is_note_private` | `boolean` | no | `false` |  |
| `notes` | `text` | yes |  |  |
| `recorded_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `recorded_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_attendance_session FOREIGN KEY (session_id) REFERENCES public.class_sessions(id) ON DELETE CASCADE`
- `CONSTRAINT fk_attendance_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_attendance_recorder FOREIGN KEY (recorded_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT uq_attendance_student_per_session UNIQUE (session_id, student_id)`
- `CONSTRAINT chk_attendance_status CHECK (status IN ('Present', 'Absent-Notified', 'Absent-Unnotified', 'Online', 'Excused', 'Not-Applicable'))`
- `CONSTRAINT chk_minutes_attended CHECK (minutes_attended >= 0)`
- `CONSTRAINT chk_units_nominated CHECK (units_nominated >= 0)`
- `CONSTRAINT chk_break_minutes CHECK (break_minutes >= 0)`

#### `class_support_staff`

Support staff assigned to a class, optionally scoped to a single student for 1:1 support arrangements. `student_id IS NULL` means class-wide; a non-null `student_id` restricts the assignment to one student (enforced by a partial unique index). Roles include Interpreter, Aide, Note-Taker, Counsellor, Support, and Other. References `classes`, `staff`, and optionally `students`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;classes |
| `staff_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;staff |
| `student_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;students |
| `role` | `varchar(50)` | no | `'Support'` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_support_class FOREIGN KEY (class_id) REFERENCES public.classes(id) ON DELETE CASCADE`
- `CONSTRAINT fk_support_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_support_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT uq_support_scope UNIQUE (class_id, staff_id, student_id)`
- `CONSTRAINT chk_support_role CHECK (role IN ('Interpreter', 'Aide', 'Note-Taker', 'Counsellor', 'Support', 'Other'))`
- `UNIQUE INDEX idx_uq_support_classwide (class_id, staff_id) WHERE (student_id IS NULL)`

#### `class_exceptions`

Per-class no-class dates that `fn_generate_sessions` skips when expanding slots into sessions. Used for class-specific cancellation dates (e.g. excursions, assessment days) beyond the public holidays handled by `holiday_observances`. References `classes`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `class_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;classes |
| `exception_date` | `date` | no |  |  |
| `reason` | `varchar(255)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_exception_per_class UNIQUE (class_id, exception_date)`
- `CONSTRAINT fk_cx_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE`

### Holidays

#### `holiday_rules`

Recurrence rule definitions for public holidays. Supports four recurrence types: `ONCE` (fixed single date), `ANNUAL_FIXED` (same day/month each year), `ANNUAL_NTH_DOW` (nth weekday of a month, e.g. second Monday in June), and `ANNUAL_EASTER_OFFSET` (days before/after Easter Sunday). Rules may be state-scoped (`state_code`) or national (`state_code IS NULL`). Expanded into `holiday_observances` by `fn_materialise_holidays`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | yes |  | PK |
| `holiday_name` | `varchar(100)` | no |  |  |
| `state_code` | `varchar(3)` | yes |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `recurrence` | `varchar(20)` | no |  |  |
| `month` | `smallint` | yes |  |  |
| `day` | `smallint` | yes |  |  |
| `weekday` | `smallint` | yes |  |  |
| `nth` | `smallint` | yes |  |  |
| `easter_offset` | `smallint` | yes |  |  |
| `fixed_date` | `date` | yes |  |  |
| `observe_substitute` | `boolean` | no | `false` |  |
| `active_from` | `smallint` | yes |  |  |
| `active_to` | `smallint` | yes |  |  |
| `notes` | `text` | yes |  |  |

*Constraints:*

- `CONSTRAINT chk_holiday_recurrence CHECK (recurrence IN ('ONCE','ANNUAL_FIXED','ANNUAL_NTH_DOW','ANNUAL_EASTER_OFFSET'))`
- `CONSTRAINT chk_holiday_rule_shape CHECK ( (recurrence = 'ONCE' AND fixed_date IS NOT NULL) OR (recurrence = 'ANNUAL_FIXED' AND month BETWEEN 1 AND 12 AND day BETWEEN 1 AND 31) OR (recurrence = 'ANNUAL_NTH_DOW' AND month BETWEEN 1 AND 12 AND weekday BETWEEN 1 AND 7 AND (nth BETWEEN 1 AND 5 OR nth = -1)) OR (recurrence = 'ANNUAL_EASTER_OFFSET' AND easter_offset IS NOT NULL) )`

#### `holiday_observances`

Concrete dated holiday entries consumed by `fn_generate_sessions` to skip sessions on public holidays. Populated automatically by `fn_materialise_holidays` when a new academic period is inserted, or manually for one-off dates (where `rule_id IS NULL`). The unique index on `(holiday_date, COALESCE(state_code, '*'), holiday_name)` prevents duplicates.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | yes |  | PK |
| `holiday_date` | `date` | no |  |  |
| `holiday_name` | `varchar(100)` | no |  |  |
| `state_code` | `varchar(3)` | yes |  | FK&nbsp;&rarr;&nbsp;australian_states |
| `rule_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;holiday_rules |
| `is_substitute` | `boolean` | no | `false` |  |

*Constraints:*

- `UNIQUE INDEX uq_observance (holiday_date, COALESCE(state_code, '*'), holiday_name)`

### Communications

#### `message_templates`

Reusable email or SMS content templates. Each template has a unique name, channel (`Email`, `SMS`, or `Both`), optional subject line, HTML body, and plain-text body. Used as the content source when creating a `message_campaign`; the campaign copies the resolved content at creation time so subsequent template edits don't alter sent campaigns.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `template_name` | `varchar(100)` | no |  | UK |
| `channel` | `varchar(10)` | no |  |  |
| `subject` | `varchar(200)` | yes |  |  |
| `body_html` | `text` | yes |  |  |
| `body_plain` | `text` | no |  |  |
| `created_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_template_name UNIQUE (template_name)`
- `CONSTRAINT fk_template_author FOREIGN KEY (created_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT chk_template_channel CHECK (channel IN ('Email', 'SMS', 'Both'))`

#### `message_campaigns`

A bulk send to an audience segment. Stores the resolved message content (copied from the template at creation time), sender identity, audience type (Individual, Class, Program, Cohort, Broadcast, or Guardian), optional class or program targeting, schedule, and send status. Fans out into `message_deliveries` — one row per recipient. References `message_templates`, `app_users`, `classes`, and `programs`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `template_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;message_templates |
| `channel` | `varchar(10)` | no |  |  |
| `subject` | `varchar(200)` | yes |  |  |
| `body_html` | `text` | yes |  |  |
| `body_plain` | `text` | no |  |  |
| `sender_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;app_users |
| `audience_type` | `varchar(20)` | no |  |  |
| `target_class_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;classes |
| `target_program_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;programs |
| `scheduled_at` | `timestamp with time zone` | yes |  |  |
| `sent_at` | `timestamp with time zone` | yes |  |  |
| `status` | `varchar(20)` | no | `'Draft'` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_campaign_template FOREIGN KEY (template_id) REFERENCES public.message_templates(id) ON DELETE SET NULL`
- `CONSTRAINT fk_campaign_sender FOREIGN KEY (sender_id) REFERENCES public.app_users(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_campaign_class FOREIGN KEY (target_class_id) REFERENCES public.classes(id) ON DELETE SET NULL`
- `CONSTRAINT fk_campaign_program FOREIGN KEY (target_program_id) REFERENCES public.programs(id) ON DELETE SET NULL`
- `CONSTRAINT chk_campaign_channel CHECK (channel IN ('Email', 'SMS', 'Both'))`
- `CONSTRAINT chk_campaign_audience CHECK (audience_type IN ('Individual', 'Class', 'Program', 'Cohort', 'Broadcast', 'Guardian'))`
- `CONSTRAINT chk_campaign_status CHECK (status IN ('Draft', 'Scheduled', 'Sending', 'Sent', 'Failed', 'Cancelled'))`

#### `message_deliveries`

Per-recipient delivery log for a bulk campaign. Exactly one of `student_id`, `guardian_id`, or `staff_id` is non-null (enforced by `num_nonnulls = 1`). Tracks the channel used, address sent to, provider message ID, and per-recipient delivery status (Pending → Sent → Delivered or Failed/Bounced/OptedOut). References `message_campaigns` and the recipient relation.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `campaign_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;message_campaigns |
| `recipient_type` | `varchar(10)` | no |  |  |
| `student_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;students |
| `guardian_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;student_guardians |
| `staff_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;staff |
| `channel` | `varchar(10)` | no |  |  |
| `address_used` | `varchar(200)` | no |  |  |
| `status` | `varchar(20)` | no | `'Pending'` |  |
| `provider_message_id` | `varchar(100)` | yes |  |  |
| `sent_at` | `timestamp with time zone` | yes |  |  |
| `delivered_at` | `timestamp with time zone` | yes |  |  |
| `failure_reason` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_delivery_campaign FOREIGN KEY (campaign_id) REFERENCES public.message_campaigns(id) ON DELETE CASCADE`
- `CONSTRAINT fk_delivery_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_delivery_guardian FOREIGN KEY (guardian_id) REFERENCES public.student_guardians(id) ON DELETE CASCADE`
- `CONSTRAINT fk_delivery_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE`
- `CONSTRAINT chk_delivery_recipient CHECK (recipient_type IN ('Student', 'Guardian', 'Staff'))`
- `CONSTRAINT chk_delivery_channel CHECK (channel IN ('Email', 'SMS'))`
- `CONSTRAINT chk_delivery_status CHECK (status IN ('Pending', 'Sent', 'Delivered', 'Failed', 'Bounced', 'OptedOut'))`
- `CONSTRAINT chk_delivery_one_recipient CHECK (num_nonnulls(student_id, guardian_id, staff_id) = 1)`

#### `messages`

Individual direct messages composed by a teacher (not bulk campaigns). Progresses through Draft → Sent → Failed. When status transitions to `Sent`, the trigger `trg_cc_sender_on_send` automatically inserts a sender CC row in `message_recipients`. References `app_users` as the sender.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `sender_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;app_users |
| `channel` | `varchar(10)` | no |  |  |
| `subject` | `varchar(200)` | yes |  |  |
| `body_html` | `text` | yes |  |  |
| `body_plain` | `text` | no |  |  |
| `status` | `varchar(20)` | no | `'Draft'` |  |
| `sent_at` | `timestamp with time zone` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_msg_sender FOREIGN KEY (sender_id) REFERENCES public.app_users(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_msg_channel CHECK (channel IN ('Email', 'SMS'))`
- `CONSTRAINT chk_msg_status CHECK (status IN ('Draft', 'Sent', 'Failed'))`

#### `message_recipients`

Per-recipient delivery row for a direct `messages` record. Exactly one of four recipient FKs is non-null (`student_id`, `guardian_id`, `staff_id`, or `teacher_id`; enforced by `num_nonnulls = 1`). `is_cc = true` marks the auto-inserted sender copy added by the trigger. Tracks delivery status, provider message ID, `delivered_at`, and `read_at` for read receipts. References `messages` and the recipient relation.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `message_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;messages |
| `recipient_type` | `varchar(10)` | no |  |  |
| `is_cc` | `boolean` | no | `false` |  |
| `student_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;students |
| `guardian_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;student_guardians |
| `staff_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;staff |
| `teacher_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;teachers |
| `address_used` | `varchar(200)` | no |  |  |
| `status` | `varchar(20)` | no | `'Pending'` |  |
| `provider_message_id` | `varchar(100)` | yes |  |  |
| `sent_at` | `timestamp with time zone` | yes |  |  |
| `delivered_at` | `timestamp with time zone` | yes |  |  |
| `read_at` | `timestamp with time zone` | yes |  |  |
| `failure_reason` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_mr_message FOREIGN KEY (message_id) REFERENCES public.messages(id) ON DELETE CASCADE`
- `CONSTRAINT fk_mr_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_mr_guardian FOREIGN KEY (guardian_id) REFERENCES public.student_guardians(id) ON DELETE CASCADE`
- `CONSTRAINT fk_mr_staff FOREIGN KEY (staff_id) REFERENCES public.staff(id) ON DELETE CASCADE`
- `CONSTRAINT fk_mr_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE`
- `CONSTRAINT chk_mr_recipient CHECK (recipient_type IN ('Student', 'Guardian', 'Staff', 'Teacher'))`
- `CONSTRAINT chk_mr_status CHECK (status IN ('Pending', 'Sent', 'Delivered', 'Failed', 'Bounced', 'OptedOut', 'Read'))`
- `CONSTRAINT chk_mr_one_recipient CHECK (num_nonnulls(student_id, guardian_id, staff_id, teacher_id) = 1)`

### Compliance & audit

#### `program_completions`

Formal qualification completion and certificate issuance records (AVETMISS NAT00130). One row per student per program (unique constraint). Records the completion date, whether a certificate has been issued (`issued_flag`), and the parchment number once issued. References `students`, `programs`, and `training_orgs`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `program_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;programs |
| `training_org_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;training_orgs |
| `completion_date` | `date` | no |  |  |
| `issued_flag` | `varchar(1)` | no | `'N'` |  |
| `parchment_number` | `varchar(30)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_pc_org FOREIGN KEY (training_org_id) REFERENCES public.training_orgs(id) ON DELETE SET NULL`
- `UNIQUE (student_id, program_id)`
- `CONSTRAINT chk_pc_issued CHECK (issued_flag IN ('Y','N'))`
- `CONSTRAINT fk_pc_student FOREIGN KEY (student_id) REFERENCES students (id) ON DELETE RESTRICT`
- `CONSTRAINT fk_pc_program FOREIGN KEY (program_id) REFERENCES programs (id) ON DELETE RESTRICT`

#### `student_progress_reports`

References to externally-stored student progress report documents (e.g. PDFs in cloud storage). `document_url` points to the file; `keywords` (text array) enables tag-based filtering and search. Optionally linked to a specific program enrolment. References `students`, `student_course_enrollments`, and `app_users` (uploader).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `enrollment_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;student_course_enrollments |
| `report_period` | `varchar(50)` | yes |  |  |
| `report_date` | `date` | no |  |  |
| `document_url` | `varchar(2048)` | no |  |  |
| `keywords` | `text[]` | yes |  |  |
| `uploaded_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_report_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_report_enrollment FOREIGN KEY (enrollment_id) REFERENCES public.student_course_enrollments(id) ON DELETE SET NULL`
- `CONSTRAINT fk_report_uploader FOREIGN KEY (uploaded_by) REFERENCES public.app_users(id) ON DELETE SET NULL`

#### `student_notes`

Timestamped, typed staff notes on a student — the primary free-text annotation mechanism. Multiple notes per student are allowed. `note_type` categorises the note (General, Pastoral, Academic, Financial, Compliance, LAP, Incident, Communication). `is_private` restricts visibility to the creating user. References `students` and `app_users` (creator, non-null, ON DELETE RESTRICT to preserve authorship).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;students |
| `note_type` | `varchar(30)` | no | `'General'` |  |
| `subject` | `varchar(200)` | yes |  |  |
| `body` | `text` | no |  |  |
| `is_private` | `boolean` | no | `false` |  |
| `created_by` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;app_users |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_note_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT fk_note_creator FOREIGN KEY (created_by) REFERENCES public.app_users(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_note_type CHECK (note_type IN ('General', 'Pastoral', 'Academic', 'Financial', 'Compliance', 'LAP', 'Incident', 'Communication'))`

#### `avetmiss_submissions`

Audit log of AVETMISS NAT collection submissions to the STA/NCVER. Each row records the RTO, reporting year, collection type (Annual, Quarterly, or Activity), submission date, submitting user, and status. `nat_file_paths` is a `jsonb` map of NAT file codes to their storage paths, preserving a record of exactly which file versions were submitted. References `training_orgs` and `app_users`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `training_org_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;training_orgs |
| `reporting_year` | `smallint` | no |  |  |
| `collection_type` | `varchar(20)` | no |  |  |
| `submission_date` | `date` | no |  |  |
| `submitted_by` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;app_users |
| `status` | `varchar(20)` | no | `'Submitted'` |  |
| `nat_file_paths` | `jsonb` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_sub_org FOREIGN KEY (training_org_id) REFERENCES public.training_orgs(id)`
- `CONSTRAINT fk_sub_user FOREIGN KEY (submitted_by) REFERENCES public.app_users(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_sub_collection CHECK (collection_type IN ('Annual', 'Quarterly', 'Activity'))`
- `CONSTRAINT chk_sub_status CHECK (status IN ('Draft', 'Submitted', 'Accepted', 'Rejected', 'Resubmitted'))`

#### `audit_log`

Append-only change trail written by the `fn_audit` trigger for all audited tables. Each row captures the table name, affected record ID, action (`INSERT`/`UPDATE`/`DELETE`), actor (`app_users.id` read from `app.current_user_id` session variable), timestamp, and old/new row data as `jsonb`. Never updated or deleted — the schema and application treat this as an immutable log.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `table_name` | `text` | no |  |  |
| `record_id` | `bigint` | yes |  |  |
| `action` | `varchar(10)` | no |  |  |
| `actor_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `changed_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `old_data` | `jsonb` | yes |  |  |
| `new_data` | `jsonb` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_audit_actor FOREIGN KEY (actor_id) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT chk_audit_action CHECK (action IN ('INSERT', 'UPDATE', 'DELETE'))`

### Workplan

#### `workplans`

Annual VTSA 2024 clause 32.4 workplan per teacher per calendar year, versioned to allow amendments. Stores the teacher's FTE fraction (`time_fraction`), CAPPS ratio, total contracted hours (`accountable_hours_required`), and agreed overtime. Progresses through Draft → Submitted → Approved. `vw_workplan_summary` joins this table against `workplan_entries` and `teacher_yearly_balances` to compute planned vs actual hour comparisons. References `teachers` and `app_users`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `calendar_year` | `smallint` | no |  |  |
| `version` | `smallint` | no | `1` |  |
| `status` | `varchar(20)` | no | `'Draft'` |  |
| `time_fraction` | `numeric(4,3)` | no | `1.000` |  |
| `capps_ratio` | `numeric(4,3)` | no | `0.750` |  |
| `accountable_hours_required` | `numeric(7,2)` | no |  |  |
| `agreed_overtime_hours` | `numeric(6,2)` | no | `0.00` |  |
| `submitted_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `submitted_at` | `timestamp with time zone` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_workplan UNIQUE (teacher_id, calendar_year, version)`
- `CONSTRAINT fk_workplan_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_workplan_submitted_by FOREIGN KEY (submitted_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT chk_workplan_status CHECK (status IN ('Draft', 'Submitted', 'Approved'))`
- `CONSTRAINT chk_workplan_fraction CHECK (time_fraction > 0 AND time_fraction <= 1)`
- `CONSTRAINT chk_workplan_capps_ratio CHECK (capps_ratio > 0 AND capps_ratio <= 1)`
- `CONSTRAINT chk_workplan_req_hours CHECK (accountable_hours_required > 0)`
- `CONSTRAINT chk_workplan_overtime CHECK (agreed_overtime_hours >= 0)`

#### `workplan_approvals`

Approval step records for a workplan. One row per `approval_role` per workplan (unique constraint); roles are `Teacher` (self-sign) and `LineManager` (manager sign-off). The unique constraint prevents double-approvals for the same role. References `workplans` and `app_users` (approver).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `workplan_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;workplans |
| `approver_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;app_users |
| `approval_role` | `varchar(30)` | no |  |  |
| `approved_at` | `timestamp with time zone` | no | `CURRENT_TIMESTAMP` |  |
| `notes` | `text` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_workplan_approval UNIQUE (workplan_id, approval_role)`
- `CONSTRAINT fk_wa_workplan FOREIGN KEY (workplan_id) REFERENCES public.workplans(id) ON DELETE CASCADE`
- `CONSTRAINT fk_wa_approver FOREIGN KEY (approver_id) REFERENCES public.app_users(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_wa_role CHECK (approval_role IN ('Teacher', 'LineManager'))`

#### `workplan_entries`

Line items on a workplan, categorised as `Teaching Delivery`, `CAPPS`, or `Education Related Duties`. Session-linked entries have a non-null `class_session_id` and `activity_start_date`; manually-entered blocks have those fields NULL. Used by `vw_workplan_summary` to compute planned totals per category. Optional FKs to `subjects`, `programs`, and `academic_periods` provide context for each line item. Referenced by `timesheet_entries.workplan_entry_id` for plan/actual reconciliation.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `workplan_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;workplans |
| `entry_type` | `varchar(30)` | no |  |  |
| `activity_name` | `varchar(100)` | no |  |  |
| `subject_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;subjects |
| `program_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;programs |
| `academic_period_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;academic_periods |
| `activity_start_date` | `date` | yes |  |  |
| `activity_end_date` | `date` | yes |  |  |
| `total_hours` | `numeric(6,2)` | no |  |  |
| `comments` | `text` | yes |  |  |
| `class_session_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;class_sessions |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_we_workplan FOREIGN KEY (workplan_id) REFERENCES public.workplans(id) ON DELETE CASCADE`
- `CONSTRAINT fk_we_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE SET NULL`
- `CONSTRAINT fk_we_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE SET NULL`
- `CONSTRAINT fk_we_period FOREIGN KEY (academic_period_id) REFERENCES public.academic_periods(id) ON DELETE SET NULL`
- `CONSTRAINT fk_we_session FOREIGN KEY (class_session_id) REFERENCES public.class_sessions(id) ON DELETE SET NULL`
- `CONSTRAINT chk_we_type CHECK (entry_type IN ('Teaching Delivery', 'CAPPS', 'Education Related Duties'))`
- `CONSTRAINT chk_we_hours CHECK (total_hours > 0)`
- `CONSTRAINT chk_we_dates CHECK (activity_end_date IS NULL OR activity_end_date >= activity_start_date)`

### Timesheet

#### `pay_periods`

Administrator-defined pay periods (fortnightly by default). Each period has a unique start date, end date, human-readable name (e.g. `FN01 2026`), and calendar year. Seeded by an administrator before timesheet generation; no overlapping periods are permitted. Referenced by `timesheets` and `teacher_period_allocations`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `period_start` | `date` | no |  |  |
| `period_end` | `date` | no |  |  |
| `period_name` | `varchar(50)` | no |  |  |
| `calendar_year` | `smallint` | no |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_pay_period_start UNIQUE (period_start)`
- `CONSTRAINT uq_pay_period_name UNIQUE (calendar_year, period_name)`
- `CONSTRAINT chk_pp_dates CHECK (period_end > period_start)`

#### `timesheets`

One timesheet per teacher per pay period (unique constraint). Aggregates teaching, CAPPS, ERD, and other hours for payroll export. Hours only — no pay rates, banking, or super data. Progresses through Draft → Submitted → Approved → Exported; `chk_ts_export` enforces that `export_format` is set whenever `exported_at` is. `vw_timesheet_summary` provides pre-aggregated totals by category. References `teachers`, `pay_periods`, and `app_users` (submitted/approved by).

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `pay_period_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;pay_periods |
| `status` | `varchar(20)` | no | `'Draft'` |  |
| `submitted_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `submitted_at` | `timestamp with time zone` | yes |  |  |
| `approved_by` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `approved_at` | `timestamp with time zone` | yes |  |  |
| `exported_at` | `timestamp with time zone` | yes |  |  |
| `export_format` | `varchar(10)` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_timesheet UNIQUE (teacher_id, pay_period_id)`
- `CONSTRAINT fk_ts_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_ts_pay_period FOREIGN KEY (pay_period_id) REFERENCES public.pay_periods(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_ts_submitted_by FOREIGN KEY (submitted_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT fk_ts_approved_by FOREIGN KEY (approved_by) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT chk_ts_status CHECK (status IN ('Draft', 'Submitted', 'Approved', 'Exported'))`
- `CONSTRAINT chk_ts_export CHECK (exported_at IS NULL OR export_format IS NOT NULL)`

#### `timesheet_entries`

Individual hour-line items on a timesheet, dated by day. Session-linked Teaching Delivery entries (`class_session_id` non-null) are auto-populated by the application from `class_sessions` within the pay period. CAPPS entries are derived from total teaching hours × `workplans.capps_ratio`. ERD and Other entries are manually added. The optional `workplan_entry_id` FK enables plan/actual reconciliation without requiring the link to always be present.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `timesheet_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;timesheets |
| `entry_date` | `date` | no |  |  |
| `entry_type` | `varchar(30)` | no |  |  |
| `description` | `varchar(200)` | yes |  |  |
| `hours` | `numeric(5,2)` | no |  |  |
| `is_overtime` | `boolean` | no | `false` |  |
| `class_session_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;class_sessions |
| `workplan_entry_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;workplan_entries |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_te_timesheet FOREIGN KEY (timesheet_id) REFERENCES public.timesheets(id) ON DELETE CASCADE`
- `CONSTRAINT fk_te_session FOREIGN KEY (class_session_id) REFERENCES public.class_sessions(id) ON DELETE SET NULL`
- `CONSTRAINT fk_te_workplan_entry FOREIGN KEY (workplan_entry_id) REFERENCES public.workplan_entries(id) ON DELETE SET NULL`
- `CONSTRAINT chk_te_type CHECK (entry_type IN ('Teaching Delivery', 'CAPPS', 'Education Related Duties', 'Other'))`
- `CONSTRAINT chk_te_hours CHECK (hours > 0)`

### Employment services

#### `student_employment_services`

Centrelink and employment service details for a student (1:1 shared PK with `students`). Records the student's Centrelink CRN, job seeker ID, participation hours, and participation type. The shared PK means one row at most per student. Parent of `student_employment_registrations`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `student_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;students |
| `centrelink_crn` | `varchar(20)` | yes |  |  |
| `job_seeker_id` | `varchar(30)` | yes |  |  |
| `participation_hours` | `numeric(5,2)` | no | `0` |  |
| `participation_type` | `varchar(10)` | yes |  |  |
| `participation_comment` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (student_id)`
- `CONSTRAINT fk_ses_student FOREIGN KEY (student_id) REFERENCES public.students(id) ON DELETE CASCADE`
- `CONSTRAINT chk_ses_participation_type CHECK (participation_type IS NULL OR participation_type IN ('Full-Time','Part-Time'))`

#### `student_employment_registrations`

Employment provider registrations for a student (e.g. jobactive, DES, NDIS Employment). Multiple rows per student are permitted — one per provider registration. Stores the provider name, registration number, start/end dates, status, and notes. References `student_employment_services` via the student ID.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `student_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;student_employment_services |
| `provider_name` | `varchar(100)` | no |  |  |
| `registration_number` | `varchar(50)` | yes |  |  |
| `start_date` | `date` | yes |  |  |
| `end_date` | `date` | yes |  |  |
| `status` | `varchar(20)` | no | `'Active'` |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_ser_student FOREIGN KEY (student_id) REFERENCES public.student_employment_services(student_id) ON DELETE CASCADE`
- `CONSTRAINT chk_ser_status CHECK (status IN ('Active','Inactive','Suspended'))`
- `CONSTRAINT chk_ser_dates CHECK (end_date IS NULL OR end_date >= start_date)`

### VCC

#### `teacher_vccs`

Annual Vocational Competency & Currency document per teacher, versioned to allow amendments. Acts as the root container for all VCC sub-records (professional qualifications, courses, units, profiling scores) for that teacher and year. Progresses through Draft → Submitted → Approved → Rejected. Records the assigned supervisor and approver. References `teachers` and `app_users`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `calendar_year` | `smallint` | no |  |  |
| `version` | `smallint` | no | `1` |  |
| `version_label` | `varchar(20)` | yes |  |  |
| `status` | `varchar(20)` | no | `'Draft'` |  |
| `supervisor_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `approved_by_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;app_users |
| `approved_at` | `timestamp with time zone` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_teacher_vcc UNIQUE (teacher_id, calendar_year, version)`
- `CONSTRAINT fk_vcc_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT`
- `CONSTRAINT fk_vcc_supervisor FOREIGN KEY (supervisor_id) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT fk_vcc_approved_by FOREIGN KEY (approved_by_id) REFERENCES public.app_users(id) ON DELETE SET NULL`
- `CONSTRAINT chk_vcc_status CHECK (status IN ('Draft','Submitted','Approved','Rejected'))`

#### `teacher_vcc_professional_qualifications`

Teacher's own credentials declared in a VCC: TAE qualification, degrees, industry certifications, registrations. Each row is one qualification with its code, title, institution, and approval status (Draft → Pending → Approved → Rejected). Linked to evidence documents via `teacher_document_connections`. References `teacher_vccs`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `vcc_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teacher_vccs |
| `qualification_code` | `varchar(30)` | no |  |  |
| `qualification_title` | `varchar(200)` | no |  |  |
| `institution` | `varchar(200)` | yes |  |  |
| `status` | `varchar(20)` | no | `'Pending'` |  |
| `approved_at` | `date` | yes |  |  |
| `notes` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_vccpq_vcc FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE`
- `CONSTRAINT chk_vccpq_status CHECK (status IN ('Draft','Pending','Approved','Rejected'))`

#### `teacher_vcc_courses`

Courses the teacher is mapping to deliver in a VCC. Optionally linked to a `programs` record; stores the course code and title for display (allowing free-text where no matching program exists). Container for `teacher_vcc_units`. `sort_order` controls display sequence. References `teacher_vccs` and optionally `programs`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `vcc_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teacher_vccs |
| `program_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;programs |
| `course_code` | `varchar(20)` | no |  |  |
| `course_title` | `varchar(200)` | no |  |  |
| `sort_order` | `smallint` | no | `0` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_vccc_vcc FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE`
- `CONSTRAINT fk_vccc_program FOREIGN KEY (program_id) REFERENCES public.programs(id) ON DELETE SET NULL`

#### `teacher_vcc_units`

Units the teacher has currency for within a VCC. Each row records the unit code and title, competency method (one of five defined methods), an optional superseded equivalent unit, description, justification text, and approval status. Multiple rows are allowed per unit (e.g. different competency methods). May be grouped under a `teacher_vcc_courses` row or be standalone. Optionally linked to `subjects` for cross-referencing with the curriculum catalogue.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `vcc_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teacher_vccs |
| `vcc_course_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;teacher_vcc_courses |
| `subject_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;subjects |
| `unit_code` | `varchar(20)` | no |  |  |
| `unit_title` | `varchar(200)` | no |  |  |
| `competency_method` | `varchar(60)` | no |  |  |
| `superseded_unit_code` | `varchar(20)` | yes |  |  |
| `superseded_unit_title` | `varchar(200)` | yes |  |  |
| `description` | `text` | yes |  |  |
| `justification` | `text` | yes |  |  |
| `status` | `varchar(20)` | no | `'Pending'` |  |
| `approved_at` | `date` | yes |  |  |
| `sort_order` | `smallint` | no | `0` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_vccu_vcc FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE`
- `CONSTRAINT fk_vccu_course FOREIGN KEY (vcc_course_id) REFERENCES public.teacher_vcc_courses(id) ON DELETE SET NULL`
- `CONSTRAINT fk_vccu_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE SET NULL`
- `CONSTRAINT chk_vccu_status CHECK (status IN ('Draft','Pending','Approved','Rejected'))`
- `CONSTRAINT chk_vccu_competency_method CHECK (competency_method IN ('I hold the current unit of competency','I hold a superseded and equivalent unit of competency','I hold a recognition of relevant study','I have vocational work experience','Other'))`

#### `teacher_documents`

Per-teacher document library: testamurs, transcripts, accreditations, registrations, licences, job cards, and other evidence files. Each row stores the document title, category, optional year, file URL in cloud storage, and original filename. `external_url` can optionally link to a third-party verification page such as a digital badge or eQuals transcript. Documents are linked to specific VCC entities via `teacher_document_connections`. References `teachers`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `title` | `varchar(200)` | no |  |  |
| `file_category` | `varchar(30)` | no | `'Other'` |  |
| `year_of_document` | `smallint` | yes |  |  |
| `document_url` | `varchar(2048)` | no |  |  |
| `external_url` | `varchar(2048)` | yes |  |  |
| `file_name` | `varchar(255)` | no |  |  |
| `uploaded_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_tdoc_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE CASCADE`
- `CONSTRAINT chk_tdoc_category CHECK (file_category IN ('Testamurs','Accreditations','Registrations','Statement of attainment','Transcripts','Credentials','Licenses','Job cards','Other'))`

#### `teacher_document_connections`

Links a teacher document to exactly one VCC entity: a professional qualification, a VCC unit, or a currency activity. The `num_nonnulls(vcc_professional_qual_id, vcc_unit_id, vcc_currency_activity_id) = 1` constraint enforces the single-target rule — one document connection always points to exactly one entity. References `teacher_documents` and the target entity.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `document_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teacher_documents |
| `vcc_professional_qual_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;teacher_vcc_professional_qualifications |
| `vcc_unit_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;teacher_vcc_units |
| `vcc_currency_activity_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;teacher_currency_activities |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_tdc_document FOREIGN KEY (document_id) REFERENCES public.teacher_documents(id) ON DELETE CASCADE`
- `CONSTRAINT fk_tdc_pq FOREIGN KEY (vcc_professional_qual_id) REFERENCES public.teacher_vcc_professional_qualifications(id) ON DELETE CASCADE`
- `CONSTRAINT fk_tdc_unit FOREIGN KEY (vcc_unit_id) REFERENCES public.teacher_vcc_units(id) ON DELETE CASCADE`
- `CONSTRAINT fk_tdc_activity FOREIGN KEY (vcc_currency_activity_id) REFERENCES public.teacher_currency_activities(id) ON DELETE CASCADE`
- `CONSTRAINT chk_tdc_target CHECK (num_nonnulls(vcc_professional_qual_id, vcc_unit_id, vcc_currency_activity_id) = 1)`

#### `teacher_currency_activities`

Vocational and professional currency point records. Each row is one activity (workshop, conference, industry work experience, professional program, etc.) with its type, date, hours, points awarded, approval status, and reflective text fields (`inform_teaching_practice`, `student_benefit`). Professional-currency-specific fields (`domain_name`, `program_type`, `program_name`, etc.) are nullable columns on the same table, avoiding a separate subtype table. Linked to related subjects via `teacher_currency_unit_links` and to evidence documents via `teacher_document_connections`. References `teachers`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `teacher_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teachers |
| `currency_type` | `varchar(15)` | no |  |  |
| `is_external` | `boolean` | no | `true` |  |
| `activity_type` | `varchar(50)` | no | `'Other'` |  |
| `activity_name` | `varchar(200)` | no |  |  |
| `date_of_activity` | `date` | no |  |  |
| `date_approved` | `date` | yes |  |  |
| `points_awarded` | `smallint` | no | `0` |  |
| `duration_hours` | `numeric(5,2)` | yes |  |  |
| `inform_teaching_practice` | `text` | yes |  |  |
| `student_benefit` | `text` | yes |  |  |
| `approval_reason` | `text` | yes |  |  |
| `status` | `varchar(20)` | no | `'Pending'` |  |
| `domain_name` | `varchar(100)` | yes |  |  |
| `program_type` | `varchar(50)` | yes |  |  |
| `program_name` | `varchar(200)` | yes |  |  |
| `program_date` | `date` | yes |  |  |
| `workshop_count` | `smallint` | yes |  |  |
| `program_summary` | `text` | yes |  |  |
| `created_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |
| `updated_at` | `timestamp with time zone` | yes | `CURRENT_TIMESTAMP` |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT fk_tca_teacher FOREIGN KEY (teacher_id) REFERENCES public.teachers(id) ON DELETE RESTRICT`
- `CONSTRAINT chk_tca_currency_type CHECK (currency_type IN ('Vocational','Professional'))`
- `CONSTRAINT chk_tca_status CHECK (status IN ('Draft','Pending','Approved','Rejected'))`
- `CONSTRAINT chk_tca_points CHECK (points_awarded >= 0)`

#### `teacher_currency_unit_links`

Many-to-many join between currency activities and the subjects they contribute currency for. `unit_code` is always populated; `subject_id` is an optional FK to `subjects` set when the unit exists in the curriculum catalogue (allowing the link even for units not yet in the system). The unique constraint on `(currency_activity_id, unit_code)` prevents duplicate links.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `id` | `bigserial` | no |  | PK |
| `currency_activity_id` | `bigint` | no |  | FK&nbsp;&rarr;&nbsp;teacher_currency_activities |
| `subject_id` | `bigint` | yes |  | FK&nbsp;&rarr;&nbsp;subjects |
| `unit_code` | `varchar(20)` | no |  |  |
| `unit_title` | `varchar(200)` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (id)`
- `CONSTRAINT uq_tcul_activity_unit UNIQUE (currency_activity_id, unit_code)`
- `CONSTRAINT fk_tcul_activity FOREIGN KEY (currency_activity_id) REFERENCES public.teacher_currency_activities(id) ON DELETE CASCADE`
- `CONSTRAINT fk_tcul_subject FOREIGN KEY (subject_id) REFERENCES public.subjects(id) ON DELETE SET NULL`

#### `teacher_vcc_profiling`

Spider/radar-chart dimension scores for a VCC. Composite PK on `(vcc_id, dimension)` allows multiple profiling dimensions per VCC. Each row holds self, supervisor, and business-ideal scores for one dimension, supporting three-way comparison displays. References `teacher_vccs`.

| Column | Type | Null | Default | Key |
|---|---|---|---|---|
| `vcc_id` | `bigint` | no |  | PK, FK&nbsp;&rarr;&nbsp;teacher_vccs |
| `dimension` | `varchar(100)` | no |  | PK |
| `business_ideal_score` | `smallint` | yes |  |  |
| `self_score` | `smallint` | yes |  |  |
| `supervisor_score` | `smallint` | yes |  |  |

*Constraints:*

- `PRIMARY KEY (vcc_id, dimension)`
- `CONSTRAINT fk_vccp_vcc FOREIGN KEY (vcc_id) REFERENCES public.teacher_vccs(id) ON DELETE CASCADE`

---

## Business rules & constraints

### Teaching hour caps

Teaching hour limits are enforced from **actual sessions**, not the weekly template.

#### Annual cap

- Every teacher has `default_max_hours_per_year` (NOT NULL, default 800.00 — the standard
  VET industry contract figure; override per teacher for HE, part-time, or other
  arrangements). This is the **sole** source for the annual cap; there is no hardcoded fallback.
- `session_teachers` inserts/deletes and `class_sessions` cancel/time/date edits call
  `fn_adjust_teacher_balance`, which maintains `teacher_yearly_balances.booked_hours`.
- The cap is enforced in two places: a friendly `RAISE EXCEPTION` inside the function,
  and a hard `CHECK (booked_hours <= allocated_max_hours)` as a backstop.
- `Guest`-role assignments do not accrue hours; team teaching counts every non-guest teacher.
- `fn_recompute_teacher_balance(teacher, year)` rebuilds the yearly cache from scratch.

#### Per-period cap (HE / DUAL teachers)

- When `teachers.max_hours_per_period` is set, per-academic-period tracking is activated
  via `teacher_period_allocations`. VET-only teachers with `max_hours_per_period IS NULL`
  are unaffected.
- The same session trigger calls `fn_adjust_teacher_period_balance`, which auto-creates the
  `teacher_period_allocations` row on first use (UPSERT) and enforces the per-period cap.
- `fn_recompute_teacher_period_balance(teacher, period)` rebuilds a per-period balance
  from scratch (no-op if `max_hours_per_period` is NULL).

### Workplan (VTSA 2024 clause 32.4)

Each teacher produces one workplan per calendar year (versioned; `uq_workplan` enforces uniqueness on `teacher_id, calendar_year, version`). The workplan records three activity categories as line items in `workplan_entries`:

| `entry_type` | Source | Cap |
|---|---|---|
| `Teaching Delivery` | Session-linked (`class_session_id`) or manually entered | 800 h/year max (standard VET) |
| `CAPPS` | Derived from teaching hours | min `capps_ratio × actual_teaching_hours` |
| `Education Related Duties` | Manual or imported | No fixed cap |

**Key fields on `workplans`:**

- `time_fraction` — teacher FTE for this year (e.g. `0.800` for 80% part-time). Determines `accountable_hours_required`.
- `accountable_hours_required` — total contracted hours (e.g. 1740.40 for 1.0 FTE). Stored explicitly from employment conditions, not derived.
- `capps_ratio` — default `0.750` (45 minutes CAPPS per teaching hour, VTSA 2024 standard). Overridable per workplan.
- `agreed_overtime_hours` — approved overtime above the standard teaching cap.

**CAPPS minimum calculation:**

```
min_capps_required = actual_teaching_hours × capps_ratio
```

`actual_teaching_hours` is read from `teacher_yearly_balances.booked_hours` (session-derived, the authoritative hours source). `vw_workplan_summary` computes this alongside planned totals.

**Approval workflow:**

`status` progresses `Draft → Submitted → Approved`. Each step is recorded in `workplan_approvals` with `approval_role IN ('Teacher', 'LineManager')`. The unique constraint on `(workplan_id, approval_role)` prevents duplicate steps. `workplans.submitted_by` / `submitted_at` record who submitted the workplan to the line manager.

**Session-linked entries:**

`workplan_entries` rows with `entry_type = 'Teaching Delivery'` and a non-null `class_session_id` are linked to a timetabled session. Rows without a session link are manually entered (e.g. aggregated ERD blocks). `activity_start_date` / `activity_end_date` are populated for session-linked rows; NULL for aggregated entries.

### Double-booking prevention

| Conflict | Mechanism |
|---|---|
| Teacher in two overlapping **slots** (same period) | `EXCLUDE` constraint on `class_slots (academic_period_id, teacher_id, day_of_week, timerange)` |
| Room in two overlapping **slots** (same period) | `EXCLUDE` constraint on `class_slots (academic_period_id, room_id, day_of_week, timerange)` |
| Room in two overlapping **sessions** | `EXCLUDE` constraint on `class_sessions (room_id, session_date, timerange)` |
| Teacher in two overlapping **sessions** (incl. ad-hoc) | `fn_check_teacher_session_conflict` trigger on `session_teachers` |

Period scoping means the same weekday/time in *different* terms is **not** a clash.

### Result workflow

`client_subject_enrolments.result_status` moves through
`In Progress → Draft → Under Review → Finalised → Appealed → Amended`, with
`result_finalised_by` / `result_finalised_at` / `result_amended_at` /
`result_amendment_reason` capturing the audit detail.

### Soft-delete & retention

`students` and `student_course_enrollments` carry `deleted_at` / `deleted_by`. The
enrolment chain uses `ON DELETE RESTRICT`, so a student with history cannot be hard-deleted.
`student_number` and `student_email` uniqueness is enforced by **partial unique indexes
on active rows only** (`WHERE deleted_at IS NULL`), so a returning student isn't blocked
by their old soft-deleted record. The **USI stays absolutely unique** — a national
identifier must never be duplicated, even across deletes.

### USI validation

`usi` is `UNIQUE`, length-10, and pattern-checked against the documented USI alphabet
(`^[2-9A-HJ-NP-Z]{10}$`, excluding ambiguous `0/1/I/O`). This is a first-line guard only;
the authoritative check is verification against the OSIR registry.

### Single-recipient invariant

`message_deliveries` enforces `num_nonnulls(student_id, guardian_id, staff_id) = 1` and
`message_recipients` enforces `num_nonnulls(student_id, guardian_id, staff_id, teacher_id) = 1`,
so exactly one recipient relation is set per row in each table.

### Direct messages

Teachers compose individual messages via `messages` (channel `Email` or `SMS`). Recipients are
recorded in `message_recipients`, one row per address. The application layer restricts composition
to `app_users` with role `Trainer`.

**Sender CC.** When `messages.status` transitions from any state to `'Sent'`, the trigger
`trg_cc_sender_on_send` fires and inserts a `message_recipients` row with `is_cc = true` pointing
back to the sender. The sender's preferred address is `teachers.teacher_email` (falling back to
`people.primary_email`). Staff senders follow the same pattern using `staff.staff_email`. Service
accounts without a `person_id` produce no CC row.

**Read tracking.** When a recipient opens the message in the application, set `read_at` on their
`message_recipients` row. `status = 'Read'` may also be used where provider tracking is unavailable.

### Timesheet

Timesheets bridge session-derived actual hours to fortnightly payroll. The record carries **hours only** — no banking, super, pay rates, or employee identifiers beyond `teacher_id`. The expectation is export (PDF/XLSX) and upload to a separate, isolated payroll system.

**Pay periods** are seeded by an administrator before timesheet generation. `uq_pay_period_start` prevents overlapping definitions; `uq_pay_period_name` gives each period a human-readable label (`FN01 2026`, `FN02 2026`, …).

**Auto-population.** For each `class_sessions` row within the pay period where the teacher appears in `session_teachers`, the application creates a `timesheet_entries` row with `entry_type = 'Teaching Delivery'` and `class_session_id` set. CAPPS entries are derived by the application (teaching hours × `workplans.capps_ratio`). ERD and Other entries are manually added.

**Status workflow:**

| Status | Meaning |
|---|---|
| `Draft` | Being assembled; entries may still change. |
| `Submitted` | Teacher has submitted for manager approval; set `submitted_by` and `submitted_at`. |
| `Approved` | Line manager approved; set `approved_by` and `approved_at`. |
| `Exported` | Sent to payroll system; set `exported_at` and `export_format`. |

`chk_ts_export` ensures `export_format` is never null once an export timestamp is recorded.

**Reconciliation with workplan.** `timesheet_entries.workplan_entry_id` is an optional FK to `workplan_entries`. Populating it allows the application to show planned vs actual hours per activity without any additional schema. It is never required — timesheets are independent of workplans.

---

## Functions & triggers

| Function | Type | What it does |
|---|---|---|
| `fn_upper_family_name` | trigger fn | `BEFORE INSERT OR UPDATE OF family_name ON people` — normalises `family_name` to `UPPER()` so all insert paths store consistent uppercase surnames. |
| `fn_set_updated_at` | trigger fn | Touches `updated_at = NOW()` on UPDATE (attached to all `updated_at` tables). |
| `fn_set_slot_period` | trigger fn | Auto-fills `class_slots.academic_period_id` from its class. |
| `fn_adjust_teacher_balance` | helper | Single point for booking/un-booking yearly teacher hours. Cap read from `teachers.default_max_hours_per_year` (no hardcoded fallback). `FOR UPDATE` serialises concurrent writers. |
| `fn_adjust_teacher_period_balance` | helper | Single point for per-period hour adjustments; no-ops when `max_hours_per_period IS NULL`. Auto-creates the `teacher_period_allocations` row on first call. |
| `fn_session_teacher_hours` | trigger fn | Books/un-books hours on `session_teachers` insert/delete; drives both annual and per-period balances. |
| `fn_session_change_hours` | trigger fn | Re-applies hours when a session is cancelled or its date/time changes; updates both annual and per-period balances. |
| `fn_recompute_teacher_balance` | utility | Rebuilds a teacher's yearly balance from sessions (for backfills/repairs). Reads cap from `teachers.default_max_hours_per_year`. |
| `fn_recompute_teacher_period_balance` | utility | Rebuilds a teacher's per-period balance from sessions. No-op if `max_hours_per_period IS NULL`. |
| `fn_check_teacher_session_conflict` | trigger fn | Blocks overlapping session assignments for a teacher. |
| `fn_easter_sunday` | utility | Computus (Meeus/Jones/Butcher) for Easter Sunday. |
| `fn_nth_weekday` | utility | nth (or last) weekday of a month. |
| `fn_materialise_holidays` | utility | Expands `holiday_rules` into `holiday_observances` for a year (idempotent). |
| `fn_materialise_holidays_for_period` | trigger fn | Auto-materialises a year's holidays when an `academic_periods` row is inserted. |
| `fn_generate_sessions` | utility | Explodes `class_slots` into `class_sessions` across the period, skipping holidays/exceptions; seeds `session_teachers`. Session booking triggers the annual and per-period cap checks. |
| `fn_validate_training_plan_compliance`, `fn_validate_traineeship_constraints` | trigger fn | Apprenticeship/traineeship date guards. |
| `fn_audit` | trigger fn | Writes to `audit_log`; reads the actor from `app.current_user_id`. |
| `fn_cc_sender_on_send` | trigger fn | After `messages.status` transitions to `'Sent'`, inserts a `message_recipients` row with `is_cc = true` for the sender. Resolves sender as Teacher then Staff; service accounts produce no CC row. |

**Views:**
- `vw_teacher_academic_workloads` — per-teacher booked vs allocated hours, remaining annual capacity, % utilisation, and sector.
- `vw_teacher_period_workloads` — per-period breakdown for teachers with `max_hours_per_period` set (HE/DUAL); includes `sequence_number` for ordered reporting. VET-only teachers with no period allocation do not appear.
- `vw_workplan_summary` — per-workplan totals for each entry type (planned_teaching_hours, planned_capps_hours, planned_erd_hours, planned_total_hours), plus actual_teaching_hours from `teacher_yearly_balances` and min_capps_required (= `booked_hours × capps_ratio`).
- `vw_timesheet_summary` — per-timesheet ordinary and overtime hours broken down by entry type (teaching, CAPPS, ERD, other), plus total_hours. Joins `pay_periods` for period dates and name.

---

## AVETMISS NAT file mapping

The schema maps to the AVETMISS 8.0 NAT collection as follows. Validate field-level
detail against the current *AVETMISS Data Element Definitions* edition, as it is revised
periodically.

| NAT file | Content | Primary source table(s) |
|---|---|---|
| **NAT00010** | Training Organisation | `training_orgs` |
| **NAT00020** | Delivery Location | `delivery_locations` (+ `australian_states.avetmiss_state_id`) |
| **NAT00030** | Program (qualification/course) | `programs` |
| **NAT00060** | Subject (unit/module) | `subjects` |
| **NAT00080** | Client (demographics) | `people` + `students` |
| **NAT00085** | Client Postal Details | `people` (address columns) |
| **NAT00090** | Disability | `student_disabilities` → `disability_types` |
| **NAT00100** | Prior Educational Achievement | `student_prior_achievements` → `prior_educational_achievements` |
| **NAT00120** | Enrolment / Training Activity | `client_subject_enrolments` (+ `student_course_enrollments`, `subjects`, `programs`, `delivery_locations`) |
| **NAT00130** | Program Completed | `program_completions` |

**Notes**

- AVETMISS uses **numeric** state identifiers (e.g. `04` = SA), held in
  `australian_states.avetmiss_state_id` and joined in at export time.
- NAT00120 allows a **blank program identifier** for standalone units; this is why
  `client_subject_enrolments.student_course_enrollment_id` is nullable.
- Dates export as `ddmmyyyy`; the schema stores native `date` and formats on export.

---

## Notes for application developers

- **Set the audit actor per transaction.** Before writes, run
  `SET LOCAL app.current_user_id = '<app_users.id>';` so `fn_audit` records who acted.
- **Generate sessions after slots are final.** Call `SELECT fn_generate_sessions(:class_id);`
  once `class_slots` are set. It is idempotent (keyed on `class_id, session_date, start_time`).
  If a teacher would exceed their annual or per-period cap the call aborts — handle that error.
- **Re-materialise holidays after editing rules.** The auto-trigger only fires on new
  `academic_periods`. After adding/editing a `holiday_rule`, call
  `SELECT fn_materialise_holidays(:year);`.
- **Scan nullable recipient relations safely.** On `message_deliveries`, exactly one of
  `student_id`/`guardian_id`/`staff_id` is set. On `message_recipients`, exactly one of
  `student_id`/`guardian_id`/`staff_id`/`teacher_id` is set — use `*int64` / `sql.NullInt64`
  and branch on the non-null one.
- **Sending a direct message.** INSERT a `messages` row with `status = 'Draft'`, add
  `message_recipients` rows for each addressee, then UPDATE `messages SET status = 'Sent',
  sent_at = NOW()`. The trigger `trg_cc_sender_on_send` inserts the sender CC automatically;
  do not insert it manually.
- **Read tracking.** When the recipient reads the message in the UI, `UPDATE message_recipients
  SET read_at = NOW(), status = 'Read' WHERE id = $1`. Filter the inbox with
  `WHERE is_cc = false AND read_at IS NULL` for unread counts.
- **Per-period caps are opt-in.** Set `teachers.max_hours_per_period` only for HE or DUAL
  teachers on semester/trimester/block contracts. VET-only teachers with NULL are unaffected;
  their `teacher_period_allocations` rows will not be created.
- **Soft-delete, don't hard-delete.** Set `deleted_at`/`deleted_by`; the enrolment chain is
  `RESTRICT` by design.
- **`timestamptz` everywhere** for clean `pgx` round-tripping.
- **Workplan submission.** When a teacher submits their workplan, set `status = 'Submitted'`, `submitted_by = <app_users.id>`, and `submitted_at = NOW()` in a single UPDATE. Insert a `workplan_approvals` row with `approval_role = 'Teacher'` once the teacher self-certifies.
- **Query workplan vs actuals via the view.** `SELECT * FROM vw_workplan_summary WHERE teacher_id = $1 AND calendar_year = $2` gives planned totals, actual teaching hours, and the minimum CAPPS entitlement in one row. Do not recompute CAPPS in application code.
- **Session-linked Teaching Delivery entries.** When creating a workplan entry from a timetabled session, populate `class_session_id`, `activity_start_date`, and `activity_end_date` from the `class_sessions` record. Leave these NULL for manually entered ERD or aggregated blocks.
- **Generating a timesheet.** Query `class_sessions` joined to `session_teachers` for the teacher within `pay_periods.period_start .. period_end`, then INSERT one `timesheet_entries` row per session with `entry_type = 'Teaching Delivery'` and `class_session_id` set. Then insert CAPPS entries derived from total teaching hours × `workplans.capps_ratio` for the relevant year.
- **Exporting a timesheet.** After producing the PDF/XLSX at the application layer, UPDATE `timesheets SET exported_at = NOW(), export_format = 'PDF', status = 'Exported'` in a single statement. The `chk_ts_export` constraint will reject an `exported_at` without a matching `export_format`.
- **Query timesheet totals.** `SELECT * FROM vw_timesheet_summary WHERE teacher_id = $1 AND pay_period_id = $2` returns a single row with ordinary/overtime subtotals by category. Use this rather than aggregating `timesheet_entries` directly.

---

## Caveats & not-yet-modelled

- **Reference data:** SACC country codes, ASCL language codes, and several classification
  sets (labour force, English proficiency) are validated by `CHECK`/defaults but not seeded
  as full lookup tables. Seed these before generating live NAT files.
- **Higher Education / TCSI:** the HE extension is at course-enrolment granularity. Full
  TCSI reporting is built around units of study with their own census dates — model those if
  HE is a primary use case.
- **Address history:** only the current address is stored; add history if longitudinal
  postal reporting is required.
- **State holiday rules:** only the clearly-national holidays are seeded. State-specific
  holidays and weekend-in-lieu rules vary and should be verified against each state's gazette.
- **Session time edits:** hour re-accrual covers cancel/date/time changes on a session;
  bulk re-timing should be followed by `fn_recompute_teacher_balance` (annual) and, for HE
  teachers, `fn_recompute_teacher_period_balance` (per-period) as safety nets.

---

*Generated from `v0.24` (2026-06-11).*
