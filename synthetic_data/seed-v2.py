#!/usr/bin/env python3
"""
seed.py — Populates the nvims-sms PostgreSQL database with synthetic Victorian TAFE data.

Connects directly to the database, inserts in dependency order, and retrieves
auto-generated PKs via RETURNING id for use in child rows. No explicit IDs are
provided (except shared-PK subtype tables which inherit people.id).

Usage:
    pip install psycopg2-binary faker
    python synthetic_data/seed.py [--dsn postgresql://localhost/nvims-sms] [--clean]

Options:
    --dsn    PostgreSQL connection string (default: postgresql://localhost/nvims-sms)
    --clean  TRUNCATE all seeded tables and restart sequences before inserting
"""

import argparse
import json
import os
import random
import sys
from collections import defaultdict
from datetime import date, time, timedelta

import psycopg2
import psycopg2.extras
from faker import Faker

random.seed(42)
fake = Faker("en_AU")
Faker.seed(42)

# postgresql://username:password@host:port/database
DEFAULT_DSN = "postgresql://nvims:jjnhbFC56RDWRTJHBjhb98uibe@localhost:5432/nvims-sms"
# DEFAULT_DSN = "postgresql://localhost/nvims-sms"

# Reverse-dependency order: children before parents so CASCADE handles FKs.
TRUNCATE_SQL = """
TRUNCATE
    public.message_recipients, public.messages,
    public.timesheet_entries, public.timesheets, public.pay_periods,
    public.workplan_entries, public.workplan_approvals, public.workplans,
    public.session_attendance, public.class_enrollments,
    public.client_subject_enrolments, public.student_course_enrollments,
    public.teacher_yearly_balances, public.teacher_period_allocations,
    public.session_teachers, public.class_sessions, public.class_slots,
    public.class_support_staff, public.class_exceptions,
    public.class_subjects, public.classes,
    public.student_guardians, public.students,
    public.staff, public.teachers, public.app_users, public.people,
    public.subject_programs, public.subjects, public.programs,
    public.rooms, public.buildings, public.delivery_locations,
    public.training_orgs, public.academic_periods,
    public.faculties, public.secondary_schools
CASCADE
"""

# setval needs only USAGE (not ownership), unlike TRUNCATE RESTART IDENTITY.
RESET_SEQS_SQL = """
SELECT setval(seq, 1, false)
FROM (
    SELECT pg_get_serial_sequence('public.' || t, 'id') AS seq
    FROM unnest(ARRAY[
        'message_recipients','messages',
        'timesheet_entries','timesheets','pay_periods',
        'workplan_entries','workplan_approvals','workplans',
        'session_attendance','class_enrollments',
        'client_subject_enrolments','student_course_enrollments',
        'teacher_yearly_balances','teacher_period_allocations',
        'class_sessions','class_slots','class_exceptions','classes',
        'student_guardians','students','app_users','people',
        'subjects','programs','rooms','buildings','delivery_locations',
        'training_orgs','faculties','secondary_schools'
    ]) AS t(t)
) sub WHERE seq IS NOT NULL
"""


# ---------------------------------------------------------------------------
# DB helpers
# ---------------------------------------------------------------------------

def one(cur, table, cols, values):
    """INSERT one row, return the auto-generated id."""
    ph = ", ".join(["%s"] * len(cols))
    cur.execute(
        f"INSERT INTO public.{table} ({', '.join(cols)}) VALUES ({ph}) RETURNING id",
        list(values),
    )
    return cur.fetchone()[0]


def many(cur, table, cols, rows):
    """INSERT rows via execute_values + RETURNING id; preserves insertion order."""
    if not rows:
        return []
    sql    = f"INSERT INTO public.{table} ({', '.join(cols)}) VALUES %s RETURNING id"
    result = psycopg2.extras.execute_values(cur, sql, rows, fetch=True)
    return [r[0] for r in result]


def bulk(cur, table, cols, rows):
    """INSERT rows where the generated id is not needed (join tables, subtypes)."""
    if not rows:
        return
    psycopg2.extras.execute_values(
        cur,
        f"INSERT INTO public.{table} ({', '.join(cols)}) VALUES %s",
        rows,
    )


# ---------------------------------------------------------------------------
# TGA data helpers
# ---------------------------------------------------------------------------

def load_tga_data():
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'tga_data.json')
    with open(path) as f:
        return json.load(f)


def _aqf_level(title: str) -> int:
    t = title.lower()
    if 'graduate diploma' in t: return 8
    if 'advanced diploma' in t: return 6
    if 'diploma'          in t: return 5
    if 'certificate iv'   in t: return 4
    if 'certificate iii'  in t: return 3
    if 'certificate ii'   in t: return 2
    if 'certificate i'    in t: return 1
    return 4


# AVETMISS level-of-education codes
_LEVEL_OF_ED = {1: '527', 2: '524', 3: '521', 4: '514', 5: '411', 6: '410', 8: '420'}
# Rough nominal hours by AQF level
_NOMINAL_HRS = {1: 180, 2: 400, 3: 600, 4: 720, 5: 1000, 6: 1300, 8: 1500}
# Field-of-education by 3-char code prefix
_FOE_BY_PREFIX = {'ICT': '0200', 'ICP': '0200', 'BSB': '0800', 'MEM': '0300',
                  'CUA': '0100', 'UEE': '0300'}


def _foe(code: str) -> str:
    return _FOE_BY_PREFIX.get(code[:3].upper(), '0800')


# ---------------------------------------------------------------------------
# Synthetic-data helpers
# ---------------------------------------------------------------------------

USI_CHARS   = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
_used_usis  = set()
_used_emails = set()


def gen_usi():
    while True:
        u = "".join(random.choices(USI_CHARS, k=10))
        if u not in _used_usis:
            _used_usis.add(u)
            return u


def gen_email(first, last, domain):
    base = (first.lower().replace(" ", "").replace("-", "") + "." +
            last.lower().replace(" ", "").replace("-", ""))
    for suffix in ["", "2", "3", "4", "5"]:
        addr = f"{base}{suffix}@{domain}"
        if addr not in _used_emails:
            _used_emails.add(addr)
            return addr
    addr = f"{base}.x{random.randint(10, 99)}@{domain}"
    _used_emails.add(addr)
    return addr


VIC_LOCS = [
    ("Shepparton", "3630"), ("Wangaratta", "3677"), ("Wodonga",  "3689"),
    ("Benalla",    "3672"), ("Seymour",    "3660"), ("Echuca",   "3564"),
    ("Ballarat",   "3350"), ("Bendigo",    "3550"), ("Geelong",  "3220"),
    ("Traralgon",  "3844"), ("Sale",       "3850"), ("Mildura",  "3500"),
]


def rand_loc():    return random.choice(VIC_LOCS)
def rand_phone():  return f"04{random.randint(10000000, 99999999)}"
def rand_street(): return str(random.randint(1, 250)), fake.street_name()


def make_person(domain, min_age=17, max_age=60):
    gender = random.choice(["M", "F"])
    first  = fake.first_name_male() if gender == "M" else fake.first_name_female()
    last   = fake.last_name()
    dob    = fake.date_of_birth(minimum_age=min_age, maximum_age=max_age)
    suburb, postcode = rand_loc()
    snum, sname = rand_street()
    email = gen_email(first, last, domain)
    return dict(gender=gender, first=first, last=last, dob=dob,
                suburb=suburb, postcode=postcode,
                street_num=snum, street_name=sname,
                email=email, mobile=rand_phone())


def period_dates(start, end, weekday_iso, count=10):
    """Up to `count` dates in [start, end] falling on ISO weekday (Mon=1)."""
    off = (weekday_iso - 1 - start.weekday()) % 7
    fp, out = start + timedelta(days=off), []
    while fp <= end and len(out) < count:
        out.append(fp)
        fp += timedelta(weeks=1)
    return out


# ---------------------------------------------------------------------------
# Seed
# ---------------------------------------------------------------------------

def seed(cur):
    SLOT_START    = time(9, 0)
    SLOT_END      = time(12, 0)
    SESSION_HOURS = 3.0

    # ── Reference data ────────────────────────────────────────────────────────
    print("reference tables...")
    cur.execute("""
        INSERT INTO public.disability_types (disability_id, disability_name) VALUES
          ('11','Hearing/Deaf'), ('12','Physical'), ('13','Intellectual'),
          ('14','Learning'), ('15','Mental illness'),
          ('16','Acquired brain impairment'), ('17','Vision'),
          ('18','Medical condition'), ('19','Other')
        ON CONFLICT DO NOTHING
    """)
    cur.execute("""
        INSERT INTO public.prior_educational_achievements (achievement_id, achievement_name) VALUES
          ('420','Bachelor degree or higher'),
          ('410','Advanced diploma or associate degree'),
          ('514','Certificate IV'), ('521','Certificate III'),
          ('524','Certificate II'), ('527','Certificate I'),
          ('811','Other certificates'), ('000','None'), ('999','Not stated')
        ON CONFLICT DO NOTHING
    """)

    # ── Faculties ─────────────────────────────────────────────────────────────
    print("faculties...")
    fac_ids = many(cur, "faculties", ["faculty_name"], [
        ("Business, Leadership & Technology",),
        ("Health, Community & Education",),
        ("Trades & Engineering",),
    ])
    fac_biz, fac_health, fac_trades = fac_ids

    # ── Secondary schools ─────────────────────────────────────────────────────
    print("secondary_schools...")
    school_ids = many(cur, "secondary_schools",
        ["school_name", "national_school_code", "school_state_code"], [
        ("Shepparton High School",           "VIC0001", "VIC"),
        ("Wangaratta High School",           "VIC0002", "VIC"),
        ("Bendigo Senior Secondary College", "VIC0003", "VIC"),
        ("Ballarat Clarendon College",       "VIC0004", "VIC"),
        ("The Geelong College",              "VIC0005", "VIC"),
    ])

    # ── Training org ──────────────────────────────────────────────────────────
    print("training_org, delivery_locations, buildings, rooms...")
    org_id = one(cur, "training_orgs",
        ["training_org_id", "training_org_name", "training_org_type",
         "address_first_line", "suburb", "state_code", "postcode",
         "contact_name", "telephone", "email"],
        ("4082", "Wattle Valley Institute of TAFE", "70",
         "1 Learning Drive", "Shepparton", "VIC", "3630",
         "Dr Sarah Brennan", "0358001000", "info@wattlevalley.edu.au"))

    # ── Delivery locations ────────────────────────────────────────────────────
    loc_ids = many(cur, "delivery_locations",
        ["training_org_id", "delivery_loc_id", "name",
         "address", "suburb", "state_code", "postcode"], [
        (org_id, "SHE-01", "Shepparton Campus",
         "1 Learning Drive", "Shepparton", "VIC", "3630"),
        (org_id, "WAN-01", "Wangaratta Campus",
         "45 Murphy Street", "Wangaratta", "VIC", "3677"),
    ])
    loc_shep, loc_wan = loc_ids

    # ── Buildings ─────────────────────────────────────────────────────────────
    bld_defs = [
        (loc_shep, "Building A"), (loc_shep, "Building B"),
        (loc_wan,  "Main Block"), (loc_wan,  "Trade Centre"),
    ]
    bld_ids = many(cur, "buildings",
        ["delivery_location_id", "building_name"],
        [(loc, name) for loc, name in bld_defs])

    # ── Rooms ─────────────────────────────────────────────────────────────────
    room_ids = []
    for bld_id, (_, bname) in zip(bld_ids, bld_defs):
        for i in range(1, 4):
            rtype = ("Workshop" if "Trade" in bname
                     else random.choice(["Classroom", "Computer Lab", "Seminar Room"]))
            room_ids.append(one(cur, "rooms",
                ["building_id", "room_name", "capacity", "room_type"],
                (bld_id, f"Room {i:02d}", random.randint(20, 35), rtype)))

    # ── TGA data ──────────────────────────────────────────────────────────────
    print("loading TGA data...")
    tga = load_tga_data()
    tga_quals      = {q['code']: q for q in tga['qualifications'] if q['type'] == 'Qualification'}
    tga_unit_title = {u['code']: u['title'] for u in tga['units']}

    # ── Programs ──────────────────────────────────────────────────────────────
    print("programs, subjects...")

    # Real ICT and BSB qualifications from TGA
    tga_prog_rows = []
    for code in sorted(tga_quals):
        q   = tga_quals[code]
        aqf = _aqf_level(q['title'])
        tga_prog_rows.append((
            fac_biz, code, q['title'],
            '11',                           # National Training Package
            _LEVEL_OF_ED.get(aqf, '514'),
            _foe(code),
            _NOMINAL_HRS.get(aqf, 600),
            True, False, None, aqf,
        ))

    # Synthetic programs for health and trades (not in TGA pull)
    synth_prog_rows = [
        (fac_health, "CHC52021", "Diploma of Community Services",
         "11", "411", "0900", 1085, True, False, None, 5),
        (fac_health, "CHC43015", "Certificate IV in Ageing Support",
         "11", "514", "0900",  720, True, False, None, 4),
        (fac_health, "CHC33015", "Certificate III in Individual Support",
         "11", "521", "0900",  993, True, False, None, 3),
        (fac_trades, "MEM30319", "Certificate III in Engineering",
         "11", "521", "0300",  740, True, False, None, 3),
    ]

    all_prog_rows = tga_prog_rows + synth_prog_rows
    prog_ids = many(cur, "programs",
        ["faculty_id","program_code","program_name","program_recognition_id",
         "level_of_education","field_of_education","nominal_hours",
         "vet_flag","he_flag","credit_points","aqf_level"],
        all_prog_rows)
    prog_by_code = {p[1]: pid for p, pid in zip(all_prog_rows, prog_ids)}

    # ── Subjects ──────────────────────────────────────────────────────────────

    # All TGA units of competency (real codes and titles)
    tga_subj_rows = [
        (code, tga_unit_title[code], 'N', _foe(code), 50, True)
        for code in sorted(tga_unit_title)
    ]

    # Synthetic units for health and trades programs only
    synth_subj_rows = [
        ("CHCCCS040", "Support independence and wellbeing",                  "N", "0900",  60, True),
        ("CHCCCS041", "Recognise healthy body systems",                      "N", "0900",  60, True),
        ("CHCCCS042", "Support community participation and inclusion",       "N", "0900",  60, True),
        ("CHCCOM005", "Communicate and work in health or community services","N", "0900",  80, True),
        ("CHCLEG001", "Work legally and ethically",                          "N", "0900",  40, True),
        ("HLTWHS002", "Follow safe work practices for direct client care",   "N", "0900",  20, True),
        ("CHCDIV001", "Work with diverse people",                            "N", "0900",  40, True),
        ("CHCMHS011", "Assess and promote social, emotional and physical wellbeing",
                                                                             "N", "0900", 100, True),
        ("MEM18001B", "Use hand tools",                                      "N", "0300",  40, True),
        ("MEM18002B", "Use power tools and hand-held operations",            "N", "0300",  40, True),
        ("MEM12023A", "Perform engineering measurements",                    "N", "0300",  40, True),
        ("MEM13003B", "Work safely",                                         "N", "0300",  20, True),
        ("MEM14004A", "Plan a complete activity",                            "N", "0300",  40, True),
    ]

    all_subj_rows = tga_subj_rows + synth_subj_rows
    subj_ids = many(cur, "subjects",
        ["subject_code","subject_name","module_flag",
         "field_of_education","nominal_hours","vet_flag"],
        all_subj_rows)
    subj_by_code = {s[0]: sid for s, sid in zip(all_subj_rows, subj_ids)}
    subj_nom_hrs = {s[0]: s[4] for s in all_subj_rows}

    # ── Subject-program links ─────────────────────────────────────────────────

    # Full TGA unit → qualification mappings from tga_data.json
    tga_sp_pairs = [
        (subj_by_code[uc], prog_by_code[qc])
        for qc, q in tga_quals.items() if qc in prog_by_code
        for uc in q['units']            if uc in subj_by_code
    ]

    # Synthetic links for health/trades programs
    SYNTH_PROG_SUBJS = {
        "CHC52021": ["CHCCCS040","CHCCCS041","CHCCOM005","CHCLEG001","CHCDIV001","CHCMHS011"],
        "CHC43015": ["CHCCCS040","CHCCCS041","CHCCCS042","CHCCOM005","HLTWHS002","CHCDIV001"],
        "CHC33015": ["CHCCCS040","CHCCOM005","HLTWHS002","CHCDIV001","CHCLEG001"],
        "MEM30319": ["MEM18001B","MEM18002B","MEM12023A","MEM13003B","MEM14004A"],
    }
    synth_sp_pairs = [
        (subj_by_code[sc], prog_by_code[pc])
        for pc, scs in SYNTH_PROG_SUBJS.items()
        for sc in scs
    ]

    bulk(cur, "subject_programs", ["subject_id","program_id"],
         list(set(tga_sp_pairs + synth_sp_pairs)))

    # PROG_SUBJS drives class scheduling: full TGA unit list per qual,
    # fixed synthetic list for health/trades. The class loop slices to N_SUBJS.
    PROG_SUBJS = {qc: q['units'] for qc, q in tga_quals.items()}
    PROG_SUBJS.update(SYNTH_PROG_SUBJS)

    # ── Academic periods ──────────────────────────────────────────────────────
    print("academic_periods...")
    period_defs = [
        ("SEM1-2025",2025,"Semester 1 2025",date(2025,2,3), date(2025,6,27),"SEMESTER",1),
        ("SEM2-2025",2025,"Semester 2 2025",date(2025,7,14),date(2025,11,28),"SEMESTER",2),
        ("SEM1-2026",2026,"Semester 1 2026",date(2026,2,2), date(2026,6,26),"SEMESTER",1),
        ("SEM2-2026",2026,"Semester 2 2026",date(2026,7,13),date(2026,11,27),"SEMESTER",2),
    ]
    period_ids = many(cur, "academic_periods",
        ["period_code","year","period_name",
         "start_date","end_date","period_type","sequence_number"],
        [tuple(p) for p in period_defs])
    periods = {p[0]: {"id": pid, "start": p[3], "end": p[4], "year": p[1]}
               for p, pid in zip(period_defs, period_ids)}

    # ── People ────────────────────────────────────────────────────────────────
    print("people, app_users, teachers, staff, students...")
    N_TEACHERS, N_STAFF, N_STUDENTS = 12, 3, 120
    teacher_persons = [make_person("staff.wattlevalley.edu.au",   25, 60) for _ in range(N_TEACHERS)]
    staff_persons   = [make_person("staff.wattlevalley.edu.au",   25, 60) for _ in range(N_STAFF)]
    student_persons = [make_person("student.wattlevalley.edu.au", 17, 45) for _ in range(N_STUDENTS)]

    def person_tuple(p):
        return (p["first"], p["last"], p["dob"], p["gender"],
                p["street_num"], p["street_name"], p["suburb"],
                "VIC", p["postcode"], "1101", p["email"], p["mobile"])

    all_persons    = teacher_persons + staff_persons + student_persons
    all_person_ids = many(cur, "people",
        ["first_given_name","family_name","dob","gender",
         "street_number","street_name","suburb","state_code","postcode",
         "country_id","primary_email","phone_mobile"],
        [person_tuple(p) for p in all_persons])

    teacher_pids = all_person_ids[:N_TEACHERS]
    staff_pids   = all_person_ids[N_TEACHERS:N_TEACHERS + N_STAFF]
    student_pids = all_person_ids[N_TEACHERS + N_STAFF:]

    # ── App users ─────────────────────────────────────────────────────────────
    admin_uid    = one(cur, "app_users", ["person_id","username","role"],
                       (None, "sysadmin", "Admin"))
    trainer_uids = many(cur, "app_users", ["person_id","username","role"],
        [(pid, f"t.trainer{i+1}", "Trainer") for i, pid in enumerate(teacher_pids[:6])])
    many(cur, "app_users", ["person_id","username","role"],
        [(staff_pids[0], "compliance1", "Compliance"),
         (staff_pids[1], "reception1",  "Reception")])
    # pid → app_user_id for teachers who have logins
    teacher_uid_map = {pid: uid for pid, uid in zip(teacher_pids[:6], trainer_uids)}

    # ── Teachers (shared PK = people.id) ─────────────────────────────────────
    FAC_CYCLE = [fac_biz]*4 + [fac_health]*4 + [fac_trades]*4
    EMP_CYCLE = (["Full-Time"]*3 + ["Part-Time"]) * 3
    teachers = []
    for i, (pid, p) in enumerate(zip(teacher_pids, teacher_persons)):
        emp   = EMP_CYCLE[i]
        max_h = 800.00 if emp == "Full-Time" else 640.00
        tf    = 1.000  if emp == "Full-Time" else 0.800
        teachers.append(dict(pid=pid, fac=FAC_CYCLE[i], email=p["email"],
                             phone=p["mobile"], emp=emp, max_h=max_h, tf=tf))
    bulk(cur, "teachers",
        ["id","faculty_id","teacher_number","teacher_email","teacher_phone",
         "employment_status","sector","default_max_hours_per_year"],
        [(t["pid"], t["fac"], f"T{1000+i}", t["email"], t["phone"],
          t["emp"], "VET", t["max_h"])
         for i, t in enumerate(teachers)])

    # ── Staff (shared PK = people.id) ────────────────────────────────────────
    bulk(cur, "staff",
        ["id","faculty_id","staff_number","staff_email","staff_phone"],
        [(pid, fac_biz, f"S{2000+i}", p["email"], p["mobile"])
         for i, (pid, p) in enumerate(zip(staff_pids, staff_persons))])

    # ── Students (shared PK = people.id) ─────────────────────────────────────
    students = []
    for i, (pid, p) in enumerate(zip(student_pids, student_persons)):
        lvl       = random.choice(["10","11","12"])
        yr_school = random.randint(2010, 2023) if lvl in ("11","12") else None
        school_fk = random.choice(school_ids) if random.random() < 0.7 else None
        students.append(dict(pid=pid, email=p["email"],
            number=f"S{10000+i:05d}", usi=gen_usi(),
            indigenous=random.choices(["9","1","2","3","4"], [85,5,5,3,2])[0],
            school_level=lvl, yr_school=yr_school, school_id=school_fk))
    bulk(cur, "students",
        ["id","student_number","student_email","usi","indigenous_status_id",
         "highest_school_level_id","year_highest_school_completed",
         "secondary_school_id","disability_flag",
         "prior_educational_achievement_flag","at_school_flag"],
        [(s["pid"], s["number"], s["email"], s["usi"], s["indigenous"],
          s["school_level"], s["yr_school"], s["school_id"], "N","N","N")
         for s in students])

    # ── Student guardians ─────────────────────────────────────────────────────
    print("student_guardians...")
    bulk(cur, "student_guardians",
        ["student_id","first_name","family_name",
         "relationship","is_primary","phone_mobile","email"],
        [(s["pid"], fake.first_name(), fake.last_name(),
          "Parent", True, rand_phone(), None)
         for s in students[:15]])

    # ── Classes ───────────────────────────────────────────────────────────────
    # Each class = one subject for one cohort group.
    # GROUP_CFG: (period_code, prog_code, group_code, loc_id, cap)
    # For each entry, N_SUBJS subject-level classes are created (one per subject).
    print("classes, class_subjects, class_slots...")

    N_SUBJS = 3   # number of subjects per group to create classes for

    # Weekday allocated per subject index within a group (0→Mon, 1→Wed, 2→Thu)
    SUBJ_WEEKDAY = [1, 3, 4]

    GROUP_CFG = [
        # SEM1-2025
        ("SEM1-2025","BSB50120","G1",loc_shep,25),
        ("SEM1-2025","BSB50120","G2",loc_shep,22),
        ("SEM1-2025","BSB50120","G3",loc_wan, 20),
        ("SEM1-2025","CHC43015","G1",loc_shep,20),
        ("SEM1-2025","CHC43015","G2",loc_shep,18),
        ("SEM1-2025","CHC43015","G3",loc_wan, 20),
        # SEM2-2025
        ("SEM2-2025","BSB40120","G1",loc_shep,25),
        ("SEM2-2025","BSB40120","G2",loc_shep,22),
        ("SEM2-2025","BSB40120","G3",loc_wan, 20),
        ("SEM2-2025","CHC52021","G1",loc_wan, 20),
        ("SEM2-2025","CHC52021","G2",loc_wan, 18),
        ("SEM2-2025","CHC52021","G3",loc_shep,20),
        # SEM1-2026
        ("SEM1-2026","MEM30319","G1",loc_wan, 16),
        ("SEM1-2026","MEM30319","G2",loc_wan, 14),
        ("SEM1-2026","ICT50220","G1",loc_shep,22),
        ("SEM1-2026","ICT50220","G2",loc_shep,20),
        ("SEM1-2026","ICT50220","G3",loc_shep,18),
        # SEM2-2026
        ("SEM2-2026","BSB30120","G1",loc_shep,25),
        ("SEM2-2026","BSB30120","G2",loc_shep,22),
        ("SEM2-2026","BSB30120","G3",loc_wan, 20),
        ("SEM2-2026","CHC33015","G1",loc_wan, 20),
        ("SEM2-2026","CHC33015","G2",loc_wan, 18),
        ("SEM2-2026","CHC33015","G3",loc_shep,20),
    ]

    # Build flat list of individual subject-classes
    classes = []
    for pc, prog_code, grp, loc_id, cap in GROUP_CFG:
        p = periods[pc]
        subj_codes = [sc for sc in PROG_SUBJS[prog_code] if sc in subj_by_code][:N_SUBJS]
        for si, sc in enumerate(subj_codes):
            cid = one(cur, "classes",
                ["class_code","group_code","academic_period_id",
                 "delivery_location_id","enrolment_cap"],
                (f"{prog_code}-{pc}-{grp}-{sc}", grp,
                 p["id"], loc_id, cap))
            classes.append(dict(id=cid, pc=pc, prog_code=prog_code, grp=grp,
                                subj_code=sc, subj_idx=si,
                                period_id=p["id"], loc_id=loc_id,
                                p_start=p["start"], p_end=p["end"]))

    # Each class has exactly one subject
    bulk(cur, "class_subjects", ["class_id","subject_id","subject_label"],
        [(c["id"], subj_by_code[c["subj_code"]], c["subj_code"])
         for c in classes])

    # ── Class slots ───────────────────────────────────────────────────────────
    # Subject index → weekday (Mon/Wed/Thu). Within a period+weekday, each
    # class must use a different teacher (no_teacher_double_booking EXCLUDE)
    # and a different room (no_room_session_double_booking EXCLUDE).
    period_day_room    = defaultdict(int)        # (pc, weekday) -> next room offset
    period_day_teacher = defaultdict(int)        # (pc, weekday) -> next teacher offset
    slots = []
    for c in classes:
        weekday = SUBJ_WEEKDAY[c["subj_idx"]]
        key = (c["pc"], weekday)

        r_off   = period_day_room[key]    % len(room_ids)
        t_off   = period_day_teacher[key] % N_TEACHERS
        period_day_room[key]    += 1
        period_day_teacher[key] += 1

        room_id = room_ids[r_off]
        teacher = teachers[t_off]
        slot_id = one(cur, "class_slots",
            ["class_id","academic_period_id","room_id","teacher_id",
             "day_of_week","start_time","end_time"],
            (c["id"], c["period_id"], room_id, teacher["pid"],
             weekday, SLOT_START, SLOT_END))
        slots.append(dict(id=slot_id, class_id=c["id"],
                         teacher_pid=teacher["pid"],
                         room_id=room_id, weekday=weekday))

    # ── Class sessions + session teachers ─────────────────────────────────────
    # Insert sessions individually to collect ids; batch session_teachers at end.
    # The fn_session_teacher_hours trigger auto-populates teacher_yearly_balances.
    print("class_sessions, session_teachers...")
    teacher_year_hours = defaultdict(float)
    all_sessions = []   # (session_id, class_id, teacher_pid, cancelled)

    for sl, c in zip(slots, classes):
        for sdate in period_dates(c["p_start"], c["p_end"], sl["weekday"]):
            cancelled = random.random() < 0.05
            sid = one(cur, "class_sessions",
                ["class_id","session_date","start_time","end_time",
                 "room_id","session_type","cancelled"],
                (c["id"], sdate, SLOT_START, SLOT_END,
                 sl["room_id"], "Scheduled", cancelled))
            all_sessions.append((sid, c["id"], sl["teacher_pid"], cancelled))
            if not cancelled:
                teacher_year_hours[(sl["teacher_pid"], sdate.year)] += SESSION_HOURS

    # session_teachers triggers update teacher_yearly_balances automatically
    bulk(cur, "session_teachers", ["session_id","teacher_id","role"],
        [(sid, tpid, "Lead") for sid, _, tpid, _ in all_sessions])

    # ── Session attendance ────────────────────────────────────────────────────
    # Each student is assigned to one (period, program, group); they attend all
    # subject-classes within that group.
    print("session_attendance...")
    group_keys = [(pc, prog, grp) for pc, prog, grp, *_ in GROUP_CFG]
    class_lookup = defaultdict(list)  # (pc, prog, grp) -> [class dict, ...]
    for c in classes:
        class_lookup[(c["pc"], c["prog_code"], c["grp"])].append(c)

    student_group = {}   # student_pid -> (pc, prog, grp)
    for i, s in enumerate(students):
        student_group[s["pid"]] = group_keys[i % len(group_keys)]

    class_student_map = defaultdict(list)
    for s in students:
        for c in class_lookup[student_group[s["pid"]]]:
            class_student_map[c["id"]].append(s["pid"])

    att_statuses = ["Present"]*7 + ["Absent-Notified"]*2 + ["Online", "Excused"]
    att_rows = []
    for sid, cls_id, _, cancelled in all_sessions:
        if cancelled:
            continue
        for spid in class_student_map[cls_id]:
            att_rows.append((sid, spid, random.choice(att_statuses),
                             None, None, admin_uid))
    bulk(cur, "session_attendance",
        ["session_id","student_id","status",
         "minutes_attended","notes","recorded_by"], att_rows)

    # ── Course enrolments ─────────────────────────────────────────────────────
    # Each student gets one SCE for their program, then one CSE + CE per
    # subject-class in their assigned group.
    print("enrolments...")
    ce_rows   = []
    cse_count = 0
    for s in students:
        pc, prog_code, grp = student_group[s["pid"]]
        group_classes = class_lookup[(pc, prog_code, grp)]
        if not group_classes:
            continue
        c0 = group_classes[0]
        sce_id = one(cur, "student_course_enrollments",
            ["student_id","program_id","enrollment_status",
             "commencement_date","commencing_program_id","funding_state_code"],
            (s["pid"], prog_by_code[prog_code], "Active",
             c0["p_start"], "4", "VIC"))
        for c in group_classes:
            cse_id = one(cur, "client_subject_enrolments",
                ["student_id","student_course_enrollment_id","subject_id",
                 "delivery_location_id","activity_start_date","activity_end_date",
                 "scheduled_hours","funding_source_national","outcome_id_national"],
                (s["pid"], sce_id, subj_by_code[c["subj_code"]], c["loc_id"],
                 c["p_start"], c["p_end"], subj_nom_hrs[c["subj_code"]], "11", "70"))
            cse_count += 1
            ce_rows.append((c["id"], cse_id))

    bulk(cur, "class_enrollments",
        ["class_id","client_subject_enrolment_id"], ce_rows)

    # ── Workplans ─────────────────────────────────────────────────────────────
    print("workplans, workplan_entries...")
    for t in teachers:
        acct = round(1740.40 * t["tf"], 2)
        for year in [2025, 2026]:
            status  = "Approved" if year == 2025 else random.choice(["Draft","Submitted"])
            teach_h = round(teacher_year_hours.get((t["pid"], year), 0.0), 2)
            capps_h = round(teach_h * 0.750, 2)
            erd_h   = max(0.0, round(acct - teach_h - capps_h, 2))
            wp_id = one(cur, "workplans",
                ["teacher_id","calendar_year","version","status",
                 "time_fraction","capps_ratio",
                 "accountable_hours_required","agreed_overtime_hours"],
                (t["pid"], year, 1, status, t["tf"], 0.750, acct, 0.00))
            bulk(cur, "workplan_entries",
                ["workplan_id","entry_type","activity_name","total_hours"],
                [(wp_id, et, nm, h) for et, nm, h in [
                    ("Teaching Delivery",        "Teaching sessions",       teach_h),
                    ("CAPPS",                    "CAPPS preparation",       capps_h),
                    ("Education Related Duties", "Education related duties", erd_h),
                ] if h > 0])

    # ── Pay periods (fortnightly: 2025-01 to 2026-06) ─────────────────────────
    print("pay_periods, timesheets, timesheet_entries...")
    pp_defs = []
    fn_date  = date(2025, 1, 6)   # first Monday of 2025
    fn_end   = date(2026, 6, 26)
    fn_num   = 1
    while fn_date <= fn_end:
        pend = fn_date + timedelta(days=13)
        pp_defs.append((fn_date, pend, f"FN{fn_num:02d} {fn_date.year}", fn_date.year))
        next_start = fn_date + timedelta(weeks=2)
        fn_num = 1 if next_start.year != fn_date.year else fn_num + 1
        fn_date = next_start

    pp_ids = many(cur, "pay_periods",
        ["period_start","period_end","period_name","calendar_year"],
        [tuple(p) for p in pp_defs])
    pay_periods = [{"id": pid, "start": d[0], "end": d[1], "year": d[3]}
                   for pid, d in zip(pp_ids, pp_defs)]

    # First pay period of each (year, quarter) → one timesheet per teacher
    quarter_pp = {}
    for pp in pay_periods:
        key = (pp["year"], (pp["start"].month - 1) // 3 + 1)
        if key not in quarter_pp:
            quarter_pp[key] = pp

    for t in teachers:
        uid = teacher_uid_map.get(t["pid"], admin_uid)
        for (year, qtr), pp in sorted(quarter_pp.items()):
            status  = ("Exported" if year == 2025
                       else "Approved" if qtr <= 2
                       else "Draft")
            sub_by  = uid    if status != "Draft" else None
            sub_at  = "2025-06-01 00:00:00+00" if status != "Draft" else None
            app_by  = admin_uid if status in ("Approved","Exported") else None
            app_at  = "2025-06-02 00:00:00+00" if status in ("Approved","Exported") else None
            exp_at  = "2025-06-03 00:00:00+00" if status == "Exported" else None
            exp_fmt = "PDF" if status == "Exported" else None
            ts_id = one(cur, "timesheets",
                ["teacher_id","pay_period_id","status",
                 "submitted_by","submitted_at",
                 "approved_by","approved_at",
                 "exported_at","export_format"],
                (t["pid"], pp["id"], status,
                 sub_by, sub_at, app_by, app_at, exp_at, exp_fmt))
            teach_pp = round(t["max_h"] / 26 * 0.6,  2)
            capps_pp = round(teach_pp * 0.75,          2)
            erd_pp   = round(t["max_h"] / 26 * 0.15, 2)
            edate    = pp["start"] + timedelta(days=1)
            bulk(cur, "timesheet_entries",
                ["timesheet_id","entry_date","entry_type","hours","is_overtime"],
                [(ts_id, edate, et, h, False) for et, h in [
                    ("Teaching Delivery",        teach_pp),
                    ("CAPPS",                    capps_pp),
                    ("Education Related Duties", erd_pp),
                ] if h > 0])

    # ── Summary ───────────────────────────────────────────────────────────────
    n_sessions = len(all_sessions)
    n_att      = len(att_rows)
    n_wps      = N_TEACHERS * 2
    n_ts       = N_TEACHERS * len(quarter_pp)
    print()
    print(f"  {N_TEACHERS} teachers  {N_STAFF} staff  {N_STUDENTS} students")
    print(f"  {len(classes)} classes  {n_sessions} sessions  {n_att} attendance records")
    print(f"  {N_STUDENTS} course enrolments  {cse_count} subject enrolments")
    print(f"  {len(pay_periods)} pay periods  {n_ts} timesheets")
    print(f"  {n_wps} workplans")
    print(f"  teacher_yearly_balances populated automatically by trigger")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser(description="Seed nvims-sms with synthetic data")
    ap.add_argument("--dsn",   default=DEFAULT_DSN,
                    help="PostgreSQL DSN (default: postgresql://localhost/nvims-sms)")
    ap.add_argument("--clean", action="store_true",
                    help="TRUNCATE seeded tables and restart sequences first")
    args = ap.parse_args()

    conn = psycopg2.connect(args.dsn)
    try:
        with conn:
            with conn.cursor() as cur:
                if args.clean:
                    print("Truncating seeded tables...")
                    cur.execute(TRUNCATE_SQL)
                    cur.execute(RESET_SEQS_SQL)
                print("Seeding...")
                seed(cur)
        print("\nCommitted.")
    except Exception as exc:
        conn.rollback()
        print(f"\nError: {exc}", file=sys.stderr)
        sys.exit(1)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
