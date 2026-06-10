package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type Period struct {
	ID         int64
	PeriodCode string
	PeriodName string
	Year       int
}

type Program struct {
	ID          int64
	ProgramCode string
	ProgramName string
}

type Group struct {
	GroupCode string
}

type Class struct {
	ID          int64
	ClassCode   string
	SubjectCode string
	SubjectName string
}

type Session struct {
	ID          int64
	ClassID     int64
	ClassCode   string
	SessionDate time.Time
	StartTime   time.Time
	EndTime     time.Time
}

type AttendanceRow struct {
	StudentID     int64
	StudentNumber string
	FirstName     string
	LastName      string
	Attendance    map[int64]string // session_id -> status ("" means not recorded)
}

func (s *Store) Periods(ctx context.Context) ([]Period, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, period_code, period_name, year
		FROM public.academic_periods
		ORDER BY year, sequence_number
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Period
	for rows.Next() {
		var p Period
		if err := rows.Scan(&p.ID, &p.PeriodCode, &p.PeriodName, &p.Year); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ProgramsForPeriod(ctx context.Context, periodID int64) ([]Program, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT p.id, p.program_code, p.program_name
		FROM public.programs p
		JOIN public.subject_programs sp ON sp.program_id = p.id
		JOIN public.class_subjects cs ON cs.subject_id = sp.subject_id
		JOIN public.classes c ON c.id = cs.class_id
		WHERE c.academic_period_id = $1
		ORDER BY p.program_name
	`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Program
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.ID, &p.ProgramCode, &p.ProgramName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GroupsForProgram(ctx context.Context, periodID, programID int64) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT c.group_code
		FROM public.classes c
		JOIN public.class_subjects cs ON cs.class_id = c.id
		JOIN public.subject_programs sp ON sp.subject_id = cs.subject_id
		WHERE c.academic_period_id = $1 AND sp.program_id = $2
		  AND c.group_code IS NOT NULL
		ORDER BY c.group_code
	`, periodID, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.GroupCode); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ClassesForGroup(ctx context.Context, periodID, programID int64, groupCode string) ([]Class, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.class_code, s.subject_code, s.subject_name
		FROM public.classes c
		JOIN public.class_subjects cs ON cs.class_id = c.id
		JOIN public.subjects s ON s.id = cs.subject_id
		JOIN public.subject_programs sp ON sp.subject_id = cs.subject_id
		WHERE c.academic_period_id = $1 AND sp.program_id = $2 AND c.group_code = $3
		ORDER BY s.subject_name
	`, periodID, programID, groupCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Class
	for rows.Next() {
		var c Class
		if err := rows.Scan(&c.ID, &c.ClassCode, &c.SubjectCode, &c.SubjectName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AttendanceGrid returns the sessions and per-student attendance for the given classes.
func (s *Store) AttendanceGrid(ctx context.Context, classIDs []int64) ([]Session, []AttendanceRow, error) {
	// Sessions ordered by date
	sessRows, err := s.pool.Query(ctx, `
		SELECT cs.id, cs.class_id, c.class_code, cs.session_date, cs.start_time, cs.end_time
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		WHERE cs.class_id = ANY($1) AND NOT cs.cancelled
		ORDER BY cs.session_date, cs.start_time, cs.class_id
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer sessRows.Close()

	var sessions []Session
	for sessRows.Next() {
		var ss Session
		if err := sessRows.Scan(&ss.ID, &ss.ClassID, &ss.ClassCode, &ss.SessionDate, &ss.StartTime, &ss.EndTime); err != nil {
			return nil, nil, err
		}
		sessions = append(sessions, ss)
	}
	if err := sessRows.Err(); err != nil {
		return nil, nil, err
	}
	if len(sessions) == 0 {
		return sessions, nil, nil
	}

	// Students enrolled in the selected classes
	studentRows, err := s.pool.Query(ctx, `
		SELECT DISTINCT s.id, s.student_number, p.first_given_name, p.family_name
		FROM public.class_enrollments ce
		JOIN public.client_subject_enrolments cse ON cse.id = ce.client_subject_enrolment_id
		JOIN public.students s ON s.id = cse.student_id
		JOIN public.people p ON p.id = s.id
		WHERE ce.class_id = ANY($1)
		ORDER BY p.family_name, p.first_given_name
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer studentRows.Close()

	var rows []AttendanceRow
	studentIdx := map[int64]int{}
	for studentRows.Next() {
		var r AttendanceRow
		if err := studentRows.Scan(&r.StudentID, &r.StudentNumber, &r.FirstName, &r.LastName); err != nil {
			return nil, nil, err
		}
		r.Attendance = make(map[int64]string)
		studentIdx[r.StudentID] = len(rows)
		rows = append(rows, r)
	}
	if err := studentRows.Err(); err != nil {
		return nil, nil, err
	}

	// Attendance records
	sessionIDs := make([]int64, len(sessions))
	for i, ss := range sessions {
		sessionIDs[i] = ss.ID
	}

	attRows, err := s.pool.Query(ctx, `
		SELECT session_id, student_id, status
		FROM public.session_attendance
		WHERE session_id = ANY($1)
	`, sessionIDs)
	if err != nil {
		return nil, nil, err
	}
	defer attRows.Close()

	for attRows.Next() {
		var sessID, studID int64
		var status string
		if err := attRows.Scan(&sessID, &studID, &status); err != nil {
			return nil, nil, err
		}
		if idx, ok := studentIdx[studID]; ok {
			rows[idx].Attendance[sessID] = status
		}
	}
	if err := attRows.Err(); err != nil {
		return nil, nil, err
	}

	return sessions, rows, nil
}

// ── Auth ───────────────────────────────────────────────────────────────────

type AuthUser struct {
	ID           int64
	Username     string
	FullName     string
	Role         string
	PasswordHash string
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (AuthUser, error) {
	var u AuthUser
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.role, u.password_hash,
		       COALESCE(p.first_given_name || ' ' || p.family_name, u.username)
		FROM public.app_users u
		LEFT JOIN public.people p ON p.id = u.person_id
		WHERE u.username = $1 AND u.is_active = true
	`, username).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.FullName)
	return u, err
}

func (s *Store) UpdateLastLogin(ctx context.Context, userID int64) {
	_, _ = s.pool.Exec(ctx, `UPDATE public.app_users SET last_login_at = NOW() WHERE id = $1`, userID)
}

// ── Results ────────────────────────────────────────────────────────────────

type ResultCol struct {
	ClassID      int64
	SubjectID    int64
	SubjectCode  string
	SubjectLabel string
}

type ResultCell struct {
	CSEID       int64
	Result      string // "SC", "NS", or ""
	IsPublished bool
}

type ResultRow struct {
	StudentID     int64
	StudentNumber string
	FirstName     string
	LastName      string
	Cells         []ResultCell // parallel to Cols slice
}

type ResultPopupData struct {
	CSEID        int64
	SubjectLabel string
	StudentName  string
	Result       string
	IsPublished  bool
}

// ResultsGrid returns columns and per-student result cells for the given classes.
func (s *Store) ResultsGrid(ctx context.Context, classIDs []int64) ([]ResultCol, []ResultRow, error) {
	colRows, err := s.pool.Query(ctx, `
		SELECT cs.class_id, cs.subject_id, cs.subject_label, sub.subject_code
		FROM public.class_subjects cs
		JOIN public.subjects sub ON sub.id = cs.subject_id
		WHERE cs.class_id = ANY($1)
		ORDER BY sub.subject_code, cs.class_id
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer colRows.Close()

	var cols []ResultCol
	type colKey struct{ c, s int64 }
	colIdx := map[colKey]int{}
	for colRows.Next() {
		var c ResultCol
		if err := colRows.Scan(&c.ClassID, &c.SubjectID, &c.SubjectLabel, &c.SubjectCode); err != nil {
			return nil, nil, err
		}
		colIdx[colKey{c.ClassID, c.SubjectID}] = len(cols)
		cols = append(cols, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, nil, err
	}
	if len(cols) == 0 {
		return cols, nil, nil
	}

	dataRows, err := s.pool.Query(ctx, `
		SELECT s.id, s.student_number, p.first_given_name, p.family_name,
		       cs.class_id, cs.subject_id,
		       cse.id, COALESCE(cse.result,''), cse.result_is_published
		FROM public.class_enrollments ce
		JOIN public.client_subject_enrolments cse ON cse.id = ce.client_subject_enrolment_id
		JOIN public.students s ON s.id = cse.student_id
		JOIN public.people p ON p.id = s.id
		JOIN public.class_subjects cs ON cs.class_id = ce.class_id AND cs.subject_id = cse.subject_id
		WHERE ce.class_id = ANY($1)
		ORDER BY p.family_name, p.first_given_name, cs.class_id, cs.subject_id
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer dataRows.Close()

	var rows []ResultRow
	studentIdx := map[int64]int{}
	for dataRows.Next() {
		var studID, classID, subjectID, cseID int64
		var studNum, firstName, lastName, result string
		var isPub bool
		if err := dataRows.Scan(&studID, &studNum, &firstName, &lastName,
			&classID, &subjectID, &cseID, &result, &isPub); err != nil {
			return nil, nil, err
		}
		idx, exists := studentIdx[studID]
		if !exists {
			rows = append(rows, ResultRow{
				StudentID:     studID,
				StudentNumber: studNum,
				FirstName:     firstName,
				LastName:      lastName,
				Cells:         make([]ResultCell, len(cols)),
			})
			idx = len(rows) - 1
			studentIdx[studID] = idx
		}
		if ci, ok := colIdx[colKey{classID, subjectID}]; ok {
			rows[idx].Cells[ci] = ResultCell{CSEID: cseID, Result: result, IsPublished: isPub}
		}
	}
	if err := dataRows.Err(); err != nil {
		return nil, nil, err
	}
	return cols, rows, nil
}

// GetResultPopupData returns display data for the result popup dialog.
func (s *Store) GetResultPopupData(ctx context.Context, cseID int64) (ResultPopupData, error) {
	var d ResultPopupData
	err := s.pool.QueryRow(ctx, `
		SELECT cse.id, cs.subject_label,
		       p.first_given_name || ' ' || p.family_name,
		       COALESCE(cse.result,''), cse.result_is_published
		FROM public.client_subject_enrolments cse
		JOIN public.students s ON s.id = cse.student_id
		JOIN public.people p ON p.id = s.id
		JOIN public.class_enrollments ce ON ce.client_subject_enrolment_id = cse.id
		JOIN public.class_subjects cs ON cs.class_id = ce.class_id AND cs.subject_id = cse.subject_id
		WHERE cse.id = $1
		LIMIT 1
	`, cseID).Scan(&d.CSEID, &d.SubjectLabel, &d.StudentName, &d.Result, &d.IsPublished)
	return d, err
}

// SetResult records SC or NS (empty string clears).
// Returns the updated cell for re-rendering.
func (s *Store) SetResult(ctx context.Context, cseID int64, result string) (ResultCell, error) {
	var cell ResultCell
	err := s.pool.QueryRow(ctx, `
		UPDATE public.client_subject_enrolments
		SET result = NULLIF($2,''), result_is_published = false
		WHERE id = $1
		RETURNING id, COALESCE(result,''), result_is_published
	`, cseID, result).Scan(&cell.CSEID, &cell.Result, &cell.IsPublished)
	return cell, err
}

// PublishResult marks a single result as published.
func (s *Store) PublishResult(ctx context.Context, cseID int64) (ResultCell, error) {
	var cell ResultCell
	err := s.pool.QueryRow(ctx, `
		UPDATE public.client_subject_enrolments
		SET result_is_published = true
		WHERE id = $1 AND result IS NOT NULL
		RETURNING id, COALESCE(result,''), result_is_published
	`, cseID).Scan(&cell.CSEID, &cell.Result, &cell.IsPublished)
	return cell, err
}

// ── Attendance write ───────────────────────────────────────────────────────

type AttendanceCell struct {
	SessionID int64
	StudentID int64
	Status    string
}

type AttendancePopupData struct {
	SessionID   int64
	StudentID   int64
	Status      string
	StudentName string
	SessionDate time.Time
	StartTime   time.Time
	EndTime     time.Time
	ClassCode   string
}

// GetAttendancePopupData fetches display data for the attendance popup.
func (s *Store) GetAttendancePopupData(ctx context.Context, sessionID, studentID int64) (AttendancePopupData, error) {
	d := AttendancePopupData{SessionID: sessionID, StudentID: studentID}
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sa.status,''), p.first_given_name || ' ' || p.family_name,
		       cs.session_date, cs.start_time, cs.end_time, c.class_code
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		JOIN public.students st ON st.id = $2
		JOIN public.people p ON p.id = st.id
		LEFT JOIN public.session_attendance sa ON sa.session_id = $1 AND sa.student_id = $2
		WHERE cs.id = $1
	`, sessionID, studentID).Scan(&d.Status, &d.StudentName, &d.SessionDate, &d.StartTime, &d.EndTime, &d.ClassCode)
	return d, err
}

// SetAttendance upserts or deletes a session_attendance row.
// Empty status means "not recorded" — deletes the row.
func (s *Store) SetAttendance(ctx context.Context, sessionID, studentID int64, status string) (AttendanceCell, error) {
	cell := AttendanceCell{SessionID: sessionID, StudentID: studentID}
	if status == "" {
		_, err := s.pool.Exec(ctx, `
			DELETE FROM public.session_attendance WHERE session_id=$1 AND student_id=$2
		`, sessionID, studentID)
		return cell, err
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.session_attendance (session_id, student_id, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id, student_id) DO UPDATE SET status = EXCLUDED.status
		RETURNING status
	`, sessionID, studentID, status).Scan(&cell.Status)
	return cell, err
}

// PublishSCColumn publishes all SC results in the given class+subject column.
func (s *Store) PublishSCColumn(ctx context.Context, classID, subjectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.client_subject_enrolments cse
		SET result_is_published = true
		FROM public.class_enrollments ce
		WHERE ce.client_subject_enrolment_id = cse.id
		  AND ce.class_id = $1
		  AND cse.subject_id = $2
		  AND cse.result = 'SC'
	`, classID, subjectID)
	return err
}

// ── Admin / People ─────────────────────────────────────────────────────────

type PersonListRow struct {
	ID         int64
	FirstName  string
	FamilyName string
	Email      string
	DOB        time.Time
	IsStudent  bool
	IsTeacher  bool
	IsStaff    bool
}

type PersonDetail struct {
	ID            int64
	Title         string
	FirstName     string
	FamilyName    string
	PreferredName string
	DOB           time.Time
	Gender        string
	Email         string
	PhoneMobile   string
	Suburb        string
	StateCode     string
	Postcode      string
	IsStudent     bool
	IsTeacher     bool
	IsStaff       bool
	StudentNumber string
	TeacherNumber string
	StaffNumber   string
}

func (s *Store) ListPeople(ctx context.Context, search string) ([]PersonListRow, error) {
	const base = `
		SELECT p.id, p.first_given_name, p.family_name, p.primary_email, p.dob,
		       EXISTS(SELECT 1 FROM public.students st WHERE st.id = p.id AND st.deleted_at IS NULL),
		       EXISTS(SELECT 1 FROM public.teachers t  WHERE t.id  = p.id),
		       EXISTS(SELECT 1 FROM public.staff   sf WHERE sf.id  = p.id)
		FROM public.people p
	`
	var q string
	var args []any
	if search == "" {
		q = base + ` ORDER BY p.family_name, p.first_given_name`
	} else {
		q = base + ` WHERE p.family_name ILIKE $1 OR p.first_given_name ILIKE $1
		             OR p.primary_email ILIKE $1
		             ORDER BY p.family_name, p.first_given_name`
		args = []any{"%" + search + "%"}
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PersonListRow
	for rows.Next() {
		var r PersonListRow
		if err := rows.Scan(&r.ID, &r.FirstName, &r.FamilyName, &r.Email, &r.DOB,
			&r.IsStudent, &r.IsTeacher, &r.IsStaff); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetPerson(ctx context.Context, id int64) (PersonDetail, error) {
	var d PersonDetail
	err := s.pool.QueryRow(ctx, `
		SELECT p.id,
		       COALESCE(p.title,''), p.first_given_name, p.family_name,
		       COALESCE(p.preferred_name,''), p.dob, p.gender,
		       p.primary_email, COALESCE(p.phone_mobile,''),
		       p.suburb, p.state_code, p.postcode,
		       EXISTS(SELECT 1 FROM public.students st WHERE st.id = p.id AND st.deleted_at IS NULL),
		       EXISTS(SELECT 1 FROM public.teachers t  WHERE t.id  = p.id),
		       EXISTS(SELECT 1 FROM public.staff   sf WHERE sf.id  = p.id),
		       COALESCE((SELECT student_number FROM public.students WHERE id = p.id),''),
		       COALESCE((SELECT teacher_number FROM public.teachers WHERE id = p.id),''),
		       COALESCE((SELECT staff_number   FROM public.staff    WHERE id = p.id),'')
		FROM public.people p WHERE p.id = $1
	`, id).Scan(
		&d.ID, &d.Title, &d.FirstName, &d.FamilyName, &d.PreferredName,
		&d.DOB, &d.Gender, &d.Email, &d.PhoneMobile,
		&d.Suburb, &d.StateCode, &d.Postcode,
		&d.IsStudent, &d.IsTeacher, &d.IsStaff,
		&d.StudentNumber, &d.TeacherNumber, &d.StaffNumber,
	)
	return d, err
}

func (s *Store) CreatePerson(ctx context.Context, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.people
		    (title, first_given_name, family_name, preferred_name,
		     dob, gender, primary_email, phone_mobile,
		     suburb, state_code, postcode)
		VALUES
		    (NULLIF($1,''), $2, $3, NULLIF($4,''),
		     $5::date, $6, $7, NULLIF($8,''),
		     $9, $10, $11)
		RETURNING id
	`, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
	).Scan(&id)
	return id, err
}

func (s *Store) UpdatePerson(ctx context.Context, id int64, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.people SET
		    title          = NULLIF($2,''),
		    first_given_name = $3,
		    family_name    = $4,
		    preferred_name = NULLIF($5,''),
		    dob            = $6::date,
		    gender         = $7,
		    primary_email  = $8,
		    phone_mobile   = NULLIF($9,''),
		    suburb         = $10,
		    state_code     = $11,
		    postcode       = $12,
		    updated_at     = NOW()
		WHERE id = $1
	`, id, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
	)
	return err
}

func (s *Store) AddStudentRole(ctx context.Context, personID int64, studentNumber, studentEmail string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.students (id, student_number, student_email)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
		    student_number = EXCLUDED.student_number,
		    student_email  = EXCLUDED.student_email,
		    deleted_at     = NULL
	`, personID, studentNumber, studentEmail)
	return err
}

func (s *Store) AddTeacherRole(ctx context.Context, personID int64, teacherNumber, teacherEmail, employmentStatus string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.teachers (id, teacher_number, teacher_email, employment_status)
		VALUES ($1, $2, $3, $4::public.employment_type)
		ON CONFLICT (id) DO UPDATE SET
		    teacher_number    = EXCLUDED.teacher_number,
		    teacher_email     = EXCLUDED.teacher_email,
		    employment_status = EXCLUDED.employment_status
	`, personID, teacherNumber, teacherEmail, employmentStatus)
	return err
}

func (s *Store) AddStaffRole(ctx context.Context, personID int64, staffNumber, staffEmail string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.staff (id, staff_number, staff_email)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
		    staff_number = EXCLUDED.staff_number,
		    staff_email  = EXCLUDED.staff_email
	`, personID, staffNumber, staffEmail)
	return err
}
