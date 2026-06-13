#!/usr/bin/env python3
"""
seed.py — Populates the nvims PostgreSQL database with synthetic Victorian TAFE data.

Programs, subjects, and class clusters are driven by tafe_data.json.
Delivery locations match the campuses listed in that file (Dandenong, Frankston, Berwick).

Usage:
    pip install psycopg2-binary faker bcrypt
    python synthetic_data/seed.py [--dsn postgresql://localhost/nvims] [--clean]

Options:
    --dsn    PostgreSQL connection string (default: see DEFAULT_DSN)
    --clean  TRUNCATE all seeded tables and restart sequences before inserting
"""

import argparse
import json
import random
import sys
from collections import defaultdict
from datetime import date, datetime, time, timedelta, timezone
from pathlib import Path

import bcrypt
import psycopg2
import psycopg2.extras
from faker import Faker

random.seed(42)
fake = Faker("en_AU")
Faker.seed(42)

DEFAULT_DSN = "postgresql://nvims:jjnhbFC56RDWRTJHBjhb98uibe@localhost:5432/nvims"

SCRIPT_DIR = Path(__file__).parent
TAFE_DATA  = json.loads((SCRIPT_DIR / "tafe_data.json").read_text())

# Reverse-dependency order: children before parents.
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
    public.intake_groups, public.program_intakes,
    public.student_guardians, public.students,
    public.staff, public.teachers, public.app_user_roles, public.app_users, public.people,
    public.subject_programs, public.subjects, public.programs,
    public.rooms, public.buildings, public.delivery_locations,
    public.training_orgs, public.academic_periods,
    public.faculties, public.secondary_schools
CASCADE
"""

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
        'intake_groups','program_intakes',
        'student_guardians','students','app_user_roles','app_users','people',
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
    """INSERT rows where the generated id is not needed."""
    if not rows:
        return
    psycopg2.extras.execute_values(
        cur,
        f"INSERT INTO public.{table} ({', '.join(cols)}) VALUES %s",
        rows,
    )


# ---------------------------------------------------------------------------
# Synthetic-data helpers
# ---------------------------------------------------------------------------

DEFAULT_PASSWORD = "Wattle2025!"
DEFAULT_PW_HASH  = bcrypt.hashpw(DEFAULT_PASSWORD.encode(), bcrypt.gensalt()).decode()

USI_CHARS       = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
_used_usis      = set()
_used_emails    = set()
_used_usernames = set()


def gen_username(first, last):
    base = (
        first.lower().replace(" ", "").replace("-", "").replace("'", "") + "." +
        last.lower().replace(" ", "").replace("-", "").replace("'", "")
    )
    for suffix in ["", "2", "3", "4", "5"]:
        u = f"{base}{suffix}"
        if u not in _used_usernames:
            _used_usernames.add(u)
            return u
    u = f"{base}.x{random.randint(10, 99)}"
    _used_usernames.add(u)
    return u


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
    ("Frankston",  "3199"), ("Dandenong",  "3175"), ("Berwick",  "3806"),
]


def rand_loc():    return random.choice(VIC_LOCS)
def rand_phone():  return f"04{random.randint(10000000, 99999999)}"
def rand_street(): return str(random.randint(1, 250)), fake.street_name()

PHOTO_URLS = [f"https://nanopi.au/id_imgs/{i:02d}.png" for i in range(1, 19)]


def gen_wwcc():
    return f"WWC{random.randint(1000000, 9999999):07d}A"


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


# AQF level → AVETMISS level_of_education code
AQF_LEVEL_CODE = {3: "514", 4: "521", 5: "411", 6: "411"}

# Campus suburb → delivery_loc_id code, address, postcode
CAMPUS_INFO = {
    "Dandenong": ("DAN-01", "35 McCrae Street",    "3175"),
    "Frankston":  ("FRK-01", "30 Dandenong Road",   "3199"),
    "Berwick":    ("BER-01", "25 Education Avenue", "3806"),
}

# Suburb → short group-code suffix used in intake_groups
SUBURB_GRP = {"Dandenong": "DA", "Frankston": "FR", "Berwick": "BE"}

# "Term N" string → zero-based offset from intake start
def term_offset(period_label):
    return int(period_label.split()[-1]) - 1

# 2026 term codes in order, for offset indexing
TERMS_2026 = ["T1-2026", "T2-2026", "T3-2026", "T4-2026"]


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
    fac_biz = fac_ids[0]

    # ── Secondary schools ─────────────────────────────────────────────────────
    print("secondary_schools...")
    school_ids = many(cur, "secondary_schools",
        ["school_name", "national_school_code", "school_state_code"], [
        ("Dandenong High School",           "VIC0010", "VIC"),
        ("Frankston High School",           "VIC0011", "VIC"),
        ("Berwick College",                 "VIC0012", "VIC"),
        ("Hallam Senior Secondary College", "VIC0013", "VIC"),
        ("Cranbourne Secondary College",    "VIC0014", "VIC"),
    ])

    # ── Training org ──────────────────────────────────────────────────────────
    print("training_org, delivery_locations, buildings, rooms...")
    org_id = one(cur, "training_orgs",
        ["training_org_id", "training_org_name", "training_org_type",
         "address_first_line", "suburb", "state_code", "postcode",
         "contact_name", "telephone", "email"],
        ("4082", "Wattle Valley Institute of TAFE", "70",
         "35 McCrae Street", "Dandenong", "VIC", "3175",
         "Dr Sarah Brennan", "0358001000", "info@wattlevalley.edu.au"))

    # ── Delivery locations — one per campus suburb ────────────────────────────
    campus_suburbs = ["Dandenong", "Frankston", "Berwick"]
    loc_rows = [
        (org_id, CAMPUS_INFO[s][0], f"{s} Campus",
         CAMPUS_INFO[s][1], s, "VIC", CAMPUS_INFO[s][2])
        for s in campus_suburbs
    ]
    loc_id_list = many(cur, "delivery_locations",
        ["training_org_id", "delivery_loc_id", "name",
         "address", "suburb", "state_code", "postcode"], loc_rows)
    loc_by_suburb = dict(zip(campus_suburbs, loc_id_list))

    # ── Buildings and rooms ───────────────────────────────────────────────────
    bld_records = []
    for loc_id, suburb in zip(loc_id_list, campus_suburbs):
        for bname in ("Building A", "Building B"):
            bid = one(cur, "buildings",
                ["delivery_location_id", "building_name"], (loc_id, bname))
            bld_records.append((bid, suburb, bname))

    room_ids = []
    for bid, _, _ in bld_records:
        for i in range(1, 4):
            rtype = random.choice(["Classroom", "Computer Lab", "Seminar Room"])
            room_ids.append(one(cur, "rooms",
                ["building_id", "room_name", "capacity", "room_type"],
                (bid, f"Room {i:02d}", random.randint(20, 35), rtype)))

    # ── Programs — from tafe_data.json ───────────────────────────────────────
    print("programs, subjects, subject_programs...")
    prog_ids  = {}   # program_code → db id
    for p in TAFE_DATA:
        nominal_hours = sum(s["nominal_hours"] for s in p["subjects"])
        pid = one(cur, "programs",
            ["faculty_id", "program_code", "program_name",
             "program_recognition_id", "level_of_education", "field_of_education",
             "nominal_hours", "vet_flag", "he_flag", "credit_points", "aqf_level"],
            (fac_biz, p["program_code"], p["program_name"],
             "11", AQF_LEVEL_CODE[p["aqf_level"]], "0200",
             nominal_hours, True, False, None, p["aqf_level"]))
        prog_ids[p["program_code"]] = pid

    # ── Subjects — deduplicated across all programs ───────────────────────────
    subj_by_code = {}   # subject_code → db id
    subj_nom_hrs = {}   # subject_code → nominal_hours
    for p in TAFE_DATA:
        for s in p["subjects"]:
            if s["subject_code"] not in subj_by_code:
                sid = one(cur, "subjects",
                    ["subject_code", "subject_name", "module_flag",
                     "field_of_education", "nominal_hours", "vet_flag"],
                    (s["subject_code"], s["subject_name"], "N",
                     "0200", s["nominal_hours"], True))
                subj_by_code[s["subject_code"]] = sid
            subj_nom_hrs[s["subject_code"]] = s["nominal_hours"]

    # ── subject_programs ─────────────────────────────────────────────────────
    sp_pairs = [
        (subj_by_code[s["subject_code"]], prog_ids[p["program_code"]])
        for p in TAFE_DATA
        for s in p["subjects"]
    ]
    bulk(cur, "subject_programs", ["subject_id", "program_id"], sp_pairs)

    # ── Academic periods ──────────────────────────────────────────────────────
    print("academic_periods...")
    period_defs = [
        ("T1-2025",2025,"Term 1 2025",date(2025,2,3), date(2025,4,11), "TERM",1),
        ("T2-2025",2025,"Term 2 2025",date(2025,4,28),date(2025,7,4),  "TERM",2),
        ("T3-2025",2025,"Term 3 2025",date(2025,7,21),date(2025,9,26), "TERM",3),
        ("T4-2025",2025,"Term 4 2025",date(2025,10,13),date(2025,12,12),"TERM",4),
        ("T1-2026",2026,"Term 1 2026",date(2026,2,2), date(2026,4,10), "TERM",1),
        ("T2-2026",2026,"Term 2 2026",date(2026,4,27),date(2026,7,3),  "TERM",2),
        ("T3-2026",2026,"Term 3 2026",date(2026,7,20),date(2026,9,25), "TERM",3),
        ("T4-2026",2026,"Term 4 2026",date(2026,10,12),date(2026,12,11),"TERM",4),
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
                "VIC", p["postcode"], "1101", p["email"], p["mobile"],
                p.get("photo_url"), p.get("photo_uploaded_at"),
                p.get("wwcc_number"), p.get("wwcc_expiry"),
                p.get("police_check_status"), p.get("police_check_date"))

    all_persons = teacher_persons + staff_persons + student_persons
    for idx, p in enumerate(all_persons):
        p["photo_url"] = PHOTO_URLS[idx % 18]
        p["photo_uploaded_at"] = datetime(
            2024, random.randint(1, 12), random.randint(1, 28), 10, 0,
            tzinfo=timezone.utc)
    for p in teacher_persons + staff_persons:
        p["wwcc_number"] = gen_wwcc()
        p["wwcc_expiry"] = date(random.randint(2025, 2028),
                                random.randint(1, 12), random.randint(1, 28))
    for p in student_persons:
        if random.random() < 0.15:
            p["wwcc_number"] = gen_wwcc()
            p["wwcc_expiry"] = date(random.randint(2025, 2028),
                                    random.randint(1, 12), random.randint(1, 28))
        else:
            p["wwcc_number"] = None
            p["wwcc_expiry"] = None

    # Police check data now lives on people (v0.24). Assign before the people insert.
    _PC_CYCLE = ["Clear"] * 8 + ["Pending"] * 2 + ["Not Required"] * 2
    _STAFF_PC = [("Clear", date(2023, 5, 10)), ("Clear", date(2022, 8, 22)), ("Not Required", None)]
    for i, p in enumerate(teacher_persons):
        pcs = _PC_CYCLE[i % len(_PC_CYCLE)]
        p["police_check_status"] = pcs
        p["police_check_date"] = (date(random.randint(2022, 2024), random.randint(1, 12), random.randint(1, 28))
                                  if pcs in ("Clear", "Pending") else None)
    for i, p in enumerate(staff_persons):
        p["police_check_status"] = _STAFF_PC[i][0]
        p["police_check_date"]   = _STAFF_PC[i][1]
    for p in student_persons:
        p["police_check_status"] = None
        p["police_check_date"]   = None

    all_person_ids = many(cur, "people",
        ["first_given_name","family_name","dob","gender",
         "street_number","street_name","suburb","state_code","postcode",
         "country_id","primary_email","phone_mobile",
         "photo_url","photo_uploaded_at","wwcc_number","wwcc_expiry",
         "police_check_status","police_check_date"],
        [person_tuple(p) for p in all_persons])

    teacher_pids = all_person_ids[:N_TEACHERS]
    staff_pids   = all_person_ids[N_TEACHERS:N_TEACHERS + N_STAFF]
    student_pids = all_person_ids[N_TEACHERS + N_STAFF:]

    # ── App users ─────────────────────────────────────────────────────────────
    STAFF_ROLES = ["Compliance", "Reception", "Staff"]
    admin_uid = one(cur, "app_users",
        ["person_id","username","password_hash"],
        (None, "admin", DEFAULT_PW_HASH))

    trainer_uids = many(cur, "app_users",
        ["person_id","username","password_hash"],
        [(pid, gen_username(p["first"], p["last"]), DEFAULT_PW_HASH)
         for pid, p in zip(teacher_pids, teacher_persons)])

    staff_uids = many(cur, "app_users",
        ["person_id","username","password_hash"],
        [(pid, gen_username(p["first"], p["last"]), DEFAULT_PW_HASH)
         for pid, p in zip(staff_pids, staff_persons)])

    student_uids = many(cur, "app_users",
        ["person_id","username","password_hash"],
        [(pid, gen_username(p["first"], p["last"]), DEFAULT_PW_HASH)
         for pid, p in zip(student_pids, student_persons)])

    bulk(cur, "app_user_roles", ["user_id","role"], [
        (admin_uid, "Admin"),
        *[(uid, "Trainer") for uid in trainer_uids],
        *[(uid, STAFF_ROLES[i]) for i, uid in enumerate(staff_uids)],
        *[(uid, "Student") for uid in student_uids],
    ])

    teacher_uid_map = {pid: uid for pid, uid in zip(teacher_pids, trainer_uids)}

    # ── Teachers ──────────────────────────────────────────────────────────────
    EMP_CYCLE = (["Full-Time"] * 3 + ["Part-Time"]) * 3
    teachers = []
    for i, (pid, p) in enumerate(zip(teacher_pids, teacher_persons)):
        emp   = EMP_CYCLE[i]
        max_h = 800.00 if emp == "Full-Time" else 640.00
        tf    = 1.000  if emp == "Full-Time" else 0.800
        teachers.append(dict(pid=pid, fac=fac_biz, email=p["email"],
                             phone=p["mobile"], emp=emp, max_h=max_h, tf=tf))
    bulk(cur, "teachers",
        ["id","faculty_id","teacher_number","teacher_email","teacher_phone",
         "employment_status","sector","default_max_hours_per_year"],
        [(t["pid"], t["fac"], f"T{1000+i}", t["email"], t["phone"],
          t["emp"], "VET", t["max_h"])
         for i, t in enumerate(teachers)])

    # ── Staff ─────────────────────────────────────────────────────────────────
    bulk(cur, "staff",
        ["id","faculty_id","staff_number","staff_email","staff_phone"],
        [(pid, fac_biz, f"S{2000+i}", p["email"], p["mobile"])
         for i, (pid, p) in enumerate(zip(staff_pids, staff_persons))])

    # ── Students ──────────────────────────────────────────────────────────────
    students = []
    for i, (pid, p) in enumerate(zip(student_pids, student_persons)):
        lvl       = random.choice(["10", "11", "12"])
        yr_school = random.randint(2010, 2023) if lvl in ("11", "12") else None
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

    # ── Multi-role people ──────────────────────────────────────────────────────
    # 3 teachers who are also students; 2 staff who are also students.
    # They reuse the same people/email — only a new students row + Student role.
    print("multi-role people (teacher+student, staff+student)...")
    N_TEACHER_STUDENTS = 3
    N_STAFF_STUDENTS   = 2
    multi_stu_offset   = N_STUDENTS   # numbering continues after regular students

    for i in range(N_TEACHER_STUDENTS):
        pid = teacher_pids[i]
        num = f"S{10000 + multi_stu_offset + i:05d}"
        cur.execute("""
            INSERT INTO public.students
                (id, student_number, student_email, usi, indigenous_status_id,
                 highest_school_level_id, disability_flag,
                 prior_educational_achievement_flag, at_school_flag)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        """, (pid, num, teacher_persons[i]["email"], gen_usi(),
              "9", "12", "N", "N", "N"))
        bulk(cur, "app_user_roles", ["user_id","role"],
             [(trainer_uids[i], "Student")])

    for i in range(N_STAFF_STUDENTS):
        pid = staff_pids[i]
        num = f"S{10000 + multi_stu_offset + N_TEACHER_STUDENTS + i:05d}"
        cur.execute("""
            INSERT INTO public.students
                (id, student_number, student_email, usi, indigenous_status_id,
                 highest_school_level_id, disability_flag,
                 prior_educational_achievement_flag, at_school_flag)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        """, (pid, num, staff_persons[i]["email"], gen_usi(),
              "9", "12", "N", "N", "N"))
        bulk(cur, "app_user_roles", ["user_id","role"],
             [(staff_uids[i], "Student")])

    # ── Student guardians ─────────────────────────────────────────────────────
    print("student_guardians...")
    bulk(cur, "student_guardians",
        ["student_id","first_name","family_name",
         "relationship","is_primary","phone_mobile","email"],
        [(s["pid"], fake.first_name(), fake.last_name(),
          "Parent", True, rand_phone(), None)
         for s in students[:15]])

    # ── Program intakes ───────────────────────────────────────────────────────
    # One intake per program, all starting T1-2026.
    print("program_intakes, intake_groups...")
    intake_start = periods["T1-2026"]

    # intake_info[prog_code] = {id, groups: {suburb: group_id}}
    intake_info = {}
    for p in TAFE_DATA:
        pc        = p["program_code"]
        intake_id = one(cur, "program_intakes",
            ["program_id", "intake_code", "intake_name",
             "start_academic_period_id", "delivery_location_id",
             "faculty_id", "study_mode",
             "duration_periods", "duration_years", "graded_assessment",
             "enrolment_open_date", "enrolment_close_date", "status"],
            (prog_ids[pc],
             f"{pc}-2026-T1",
             f"{p['program_name']} — 2026 Term 1",
             intake_start["id"],
             loc_by_suburb[p["suburb"][0]],
             fac_biz,
             "Full-Time",
             p["duration_periods"],
             p["duration_years"],
             p.get("graded_assessment", False),
             date(2025, 11, 1),
             date(2025, 12, 19),
             "Active"))

        groups = {}
        for suburb in p["suburb"]:
            grp_id = one(cur, "intake_groups",
                ["intake_id", "group_code", "group_name", "capacity"],
                (intake_id, SUBURB_GRP[suburb],
                 f"{suburb} Group",
                 random.randint(18, 25)))
            groups[suburb] = grp_id

        intake_info[pc] = {"id": intake_id, "groups": groups}

    # ── Classes — one per (cluster, suburb) where created=True ───────────────
    print("classes, class_subjects, class_slots...")

    weekday_counter    = defaultdict(int)   # (period_code, prog_code, suburb) → next weekday
    period_day_teacher = defaultdict(int)   # (period_code, weekday) → next teacher index
    period_day_room    = defaultdict(int)   # (period_code, weekday) → next room index

    # classes_list: metadata dict for every created class row
    classes_list = []

    for p in TAFE_DATA:
        pc = p["program_code"]
        for cluster in p["class_subjects"]:
            if not cluster["created"]:
                continue
            offset      = term_offset(cluster["period_code"])
            period_code = TERMS_2026[offset]
            period      = periods[period_code]

            for suburb, grp_id in intake_info[pc]["groups"].items():
                loc_id  = loc_by_suburb[suburb]
                wk_key  = (period_code, pc, suburb)
                weekday = (weekday_counter[wk_key] % 5) + 1
                weekday_counter[wk_key] += 1

                slug       = cluster["cluster"][:20].replace(" ", "_")
                class_code = f"{pc}-2026T{offset+1}-{SUBURB_GRP[suburb]}-{slug}"

                cid = one(cur, "classes",
                    ["class_code", "intake_group_id", "academic_period_id",
                     "delivery_location_id", "enrolment_cap"],
                    (class_code, grp_id, period["id"], loc_id,
                     random.randint(18, 25)))

                classes_list.append(dict(
                    id=cid, prog_code=pc, suburb=suburb,
                    period_code=period_code, period_id=period["id"],
                    loc_id=loc_id, p_start=period["start"], p_end=period["end"],
                    weekday=weekday, subjects=cluster["subjects"],
                    grp_id=grp_id,
                ))

    # class_subjects join: one row per (class, subject) in the cluster
    cs_rows = [
        (c["id"], subj_by_code[sc], sc)
        for c in classes_list
        for sc in c["subjects"]
    ]
    bulk(cur, "class_subjects", ["class_id", "subject_id", "subject_label"], cs_rows)

    # ── Class slots ───────────────────────────────────────────────────────────
    slots = []
    for c in classes_list:
        key   = (c["period_code"], c["weekday"])
        r_off = period_day_room[key]    % len(room_ids)
        t_off = period_day_teacher[key] % N_TEACHERS
        period_day_room[key]    += 1
        period_day_teacher[key] += 1

        slot_id = one(cur, "class_slots",
            ["class_id","academic_period_id","room_id","teacher_id",
             "day_of_week","start_time","end_time"],
            (c["id"], c["period_id"], room_ids[r_off], teachers[t_off]["pid"],
             c["weekday"], SLOT_START, SLOT_END))
        slots.append(dict(id=slot_id, class_id=c["id"],
                         teacher_pid=teachers[t_off]["pid"],
                         room_id=room_ids[r_off], weekday=c["weekday"]))

    # ── Class sessions + session teachers ─────────────────────────────────────
    print("class_sessions, session_teachers...")
    teacher_year_hours = defaultdict(float)
    all_sessions = []

    for sl, c in zip(slots, classes_list):
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

    bulk(cur, "session_teachers", ["session_id","teacher_id","role"],
        [(sid, tpid, "Lead") for sid, _, tpid, _ in all_sessions])

    # ── Session attendance ────────────────────────────────────────────────────
    print("session_attendance...")

    # group_class_lookup: (prog_code, suburb) → [class dict, ...]
    group_class_lookup = defaultdict(list)
    for c in classes_list:
        group_class_lookup[(c["prog_code"], c["suburb"])].append(c)

    # Only assign students to groups that actually have classes
    group_keys = [
        (p["program_code"], suburb)
        for p in TAFE_DATA
        for suburb in p["suburb"]
        if group_class_lookup[(p["program_code"], suburb)]
    ]

    student_group = {s["pid"]: group_keys[i % len(group_keys)]
                     for i, s in enumerate(students)}

    class_student_map = defaultdict(list)
    for s in students:
        for c in group_class_lookup[student_group[s["pid"]]]:
            class_student_map[c["id"]].append(s["pid"])

    att_statuses = ["Present"] * 7 + ["Absent-Notified"] * 2 + ["Online", "Excused"]
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

    # ── Enrolments ────────────────────────────────────────────────────────────
    # Each student gets one SCE (linked to their intake group), then one CSE
    # per unique subject across all classes in their group, plus a
    # class_enrollment linking each class to the relevant CSE.
    print("enrolments...")
    ce_rows   = []
    cse_count = 0

    for s in students:
        prog_code, suburb = student_group[s["pid"]]
        group_classes     = group_class_lookup[(prog_code, suburb)]
        if not group_classes:
            continue

        grp_id = intake_info[prog_code]["groups"][suburb]
        c0     = group_classes[0]

        sce_id = one(cur, "student_course_enrollments",
            ["student_id","program_id","enrollment_status",
             "commencement_date","commencing_program_id","funding_state_code",
             "intake_group_id"],
            (s["pid"], prog_ids[prog_code], "Active",
             c0["p_start"], "4", "VIC", grp_id))

        # enrolled_subjs: subject_code → cse_id (deduplicated across clusters)
        enrolled_subjs = {}
        for c in group_classes:
            for sc in c["subjects"]:
                if sc not in enrolled_subjs:
                    cse_id = one(cur, "client_subject_enrolments",
                        ["student_id","student_course_enrollment_id","subject_id",
                         "delivery_location_id","activity_start_date","activity_end_date",
                         "scheduled_hours","funding_source_national","outcome_id_national"],
                        (s["pid"], sce_id, subj_by_code[sc], c["loc_id"],
                         c["p_start"], c["p_end"],
                         subj_nom_hrs[sc], "11", "70"))
                    enrolled_subjs[sc] = cse_id
                    cse_count += 1
                ce_rows.append((c["id"], enrolled_subjs[sc]))

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
                    ("Teaching Delivery",        "Teaching sessions",        teach_h),
                    ("CAPPS",                    "CAPPS preparation",        capps_h),
                    ("Education Related Duties", "Education related duties", erd_h),
                ] if h > 0])

    # ── Pay periods (fortnightly: 2025-01 to 2026-06) ─────────────────────────
    print("pay_periods, timesheets, timesheet_entries...")
    pp_defs = []
    fn_date = date(2025, 1, 6)
    fn_end  = date(2026, 6, 26)
    fn_num  = 1
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
            sub_by  = uid       if status != "Draft" else None
            sub_at  = "2025-06-01 00:00:00+00" if status != "Draft" else None
            app_by  = admin_uid if status in ("Approved", "Exported") else None
            app_at  = "2025-06-02 00:00:00+00" if status in ("Approved", "Exported") else None
            exp_at  = "2025-06-03 00:00:00+00" if status == "Exported" else None
            exp_fmt = "PDF" if status == "Exported" else None
            ts_id = one(cur, "timesheets",
                ["teacher_id","pay_period_id","status",
                 "submitted_by","submitted_at",
                 "approved_by","approved_at",
                 "exported_at","export_format"],
                (t["pid"], pp["id"], status,
                 sub_by, sub_at, app_by, app_at, exp_at, exp_fmt))
            teach_pp = round(t["max_h"] / 26 * 0.60, 2)
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
    n_prog     = len(TAFE_DATA)
    n_subj     = len(subj_by_code)
    n_sessions = len(all_sessions)
    n_att      = len(att_rows)
    n_wps      = N_TEACHERS * 2
    n_ts       = N_TEACHERS * len(quarter_pp)
    print()
    print(f"  {n_prog} programs  {n_subj} subjects")
    print(f"  {N_TEACHERS} teachers  {N_STAFF} staff  {N_STUDENTS} students")
    print(f"  {N_TEACHER_STUDENTS} teacher+student  {N_STAFF_STUDENTS} staff+student")
    print(f"  {len(classes_list)} classes  {n_sessions} sessions  {n_att} attendance records")
    print(f"  {N_STUDENTS} course enrolments  {cse_count} subject enrolments")
    print(f"  {len(pay_periods)} pay periods  {n_ts} timesheets  {n_wps} workplans")
    print(f"  teacher_yearly_balances populated automatically by trigger")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def main():
    ap = argparse.ArgumentParser(description="Seed nvims with synthetic data")
    ap.add_argument("--dsn",   default=DEFAULT_DSN,
                    help="PostgreSQL DSN")
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
