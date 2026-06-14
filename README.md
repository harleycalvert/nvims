# NVIMS — National VET Information Management System

NVIMS is a Student Management System (SMS) built for Australian RTOs, TAFEs, and Higher Education providers. It is implemented in PostgreSQL and Go, and is designed to support AVETMISS reporting, teacher workforce compliance (VCC), timetabling, attendance, and enrolment management.

> **Status:** Early development. Core data structures are stable; several modules are functional and in active use.  
> **Project commenced:** 5 June 2026  
> **Development cost to date:** $34.00

---

## Scope

### In Scope

NVIMS is a self-hosted, web-based SMS covering the full lifecycle of VET delivery:

- Student enrolment and profile management
- Program, class, and session scheduling (timetabling)
- Attendance recording and registers
- Results and competency tracking (SC / NS / IP)
- Teacher VCC (Vocational Competency and Currency) compliance tracking
- AVETMISS-aligned data structures (NAT file generation planned)
- RTO infrastructure management (organisations, campuses, buildings, rooms)
- Academic period, faculty, intake group, and subject management
- System configuration (LMS link, file storage — see Out of Scope below)

### Out of Scope

The following are deliberately outside the system boundary. NVIMS integrates with or links to these externally:

| Capability | Approach |
|---|---|
| Learning Management System (LMS) | External — NVIMS links to the configured LMS (Canvas, Moodle, Blackboard, etc.) via the sidebar |
| File / document storage | External — configurable storage endpoint (planned); documents are referenced, not stored |
| Student payment and fees | Out of scope |
| HR and payroll | Out of scope |
| Email and messaging | Out of scope |

---

## What Is Implemented

### People & Enrolments
- People registry (students, teachers, staff) with WWCC, photo, and police check tracking
- Student course enrolments with status lifecycle (Active, Deferred, Suspended, Cancelled, Completed)
- AVETMISS gender codes, country of birth, and address fields

### Programs & Delivery
- Program catalogue with AQF level, ASCED field of education, VET/HE flags, and nominal hours
- Academic periods, faculties, intake groups, and delivery locations
- Class management with subject assignments and enrolment caps
- Session scheduling with teacher and intake group views, period-era filtering

### Attendance & Results
- Attendance register with per-session, per-student recording (Present, Online, Absent-Notified, Excused, Absent)
- Results grid with competency outcomes (SC, NS, IP) and publish workflow

### Teacher VCC
- Teaching qualifications (TAE/training) with AQF level and document attachments
- Vocational qualifications with AQF level and document attachments
- Vocational evidence and industry currency tracking
- Programs and units taught

### Infrastructure
- Training organisation management
- Delivery locations, buildings, and rooms
- Faculty management

### System
- LMS configuration (name and URL, surfaced as a sidebar link)
- Database backup
- Role-based access (Student, Teacher, Staff, Admin)

---

## Planned Features

### Assessment
- **Assessment creation** — build assessments aligned to units of competency, with TGA training package integration (units sourced directly from training.gov.au)
- **Assessment maintenance** — version control and review workflows for assessment tools

### AVETMISS Reporting
- NAT file generation (NAT00010, NAT00020, NAT00030, NAT00060, NAT00080, NAT00085, NAT00090, NAT00120)
- State funding supplements (Skills First VIC, Smart & Skilled NSW, etc.)

### Enrolment Extensions
- Apprenticeship and traineeship details (training plan, employer, contract)
- VSL (VET Student Loans) and HE enrolment supplements
- Credit transfer and RPL recording

### System
- System user management (login accounts, role assignment)
- File storage configuration
- Audit logging

---

## Getting Started

### Prerequisites
- PostgreSQL 15+
- Go 1.22+


---

## Documentation

- [Database Schema](https://github.com/harleycalvert/nvims-sms/blob/main/docs/DATABASE.md)
- [NVIMS Forum](https://nvims.boards.net/)
