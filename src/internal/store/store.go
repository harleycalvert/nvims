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
	ID        int64
	GroupCode string
	GroupName string
}

type Class struct {
	ID        int64
	ClassCode string
	Label     string // comma-joined subject codes for this cluster
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
	PhotoURL      string
	Attendance    map[int64]string // session_id -> status ("" means not recorded)
}

type StudentPanelData struct {
	StudentNumber string `json:"studentNumber"`
	FullName      string `json:"fullName"`
	PhotoURL      string `json:"photoURL"`
	DOBStr        string `json:"dob"`
	Gender        string `json:"gender"`
	Email         string `json:"email"`
	PhoneMobile   string `json:"phoneMobile"`
	Suburb        string `json:"suburb"`
	StateCode     string `json:"stateCode"`
	Postcode      string `json:"postcode"`
	WWCCNumber    string `json:"wwccNumber"`
	WWCCExpiryStr string `json:"wwccExpiry"`
	Competent     int `json:"competent"`
	NotCompetent  int `json:"notCompetent"`
	InProgress    int `json:"inProgress"`
	NotYetStarted int `json:"notYetStarted"`
	Total         int `json:"total"`
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
		SELECT DISTINCT ig.id, ig.group_code, ig.group_name
		FROM public.intake_groups ig
		JOIN public.program_intakes pi ON pi.id = ig.intake_id
		JOIN public.classes c ON c.intake_group_id = ig.id
		WHERE c.academic_period_id = $1 AND pi.program_id = $2
		ORDER BY ig.group_code
	`, periodID, programID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.GroupCode, &g.GroupName); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ClassesForGroup(ctx context.Context, periodID, groupID int64) ([]Class, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.class_code,
		       string_agg(s.subject_code, ', ' ORDER BY s.subject_code) AS label
		FROM public.classes c
		JOIN public.class_subjects cs ON cs.class_id = c.id
		JOIN public.subjects s ON s.id = cs.subject_id
		WHERE c.academic_period_id = $1 AND c.intake_group_id = $2
		GROUP BY c.id, c.class_code
		ORDER BY c.class_code
	`, periodID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Class
	for rows.Next() {
		var c Class
		if err := rows.Scan(&c.ID, &c.ClassCode, &c.Label); err != nil {
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
		SELECT DISTINCT s.id, s.student_number, p.first_given_name, p.family_name,
		       COALESCE(p.photo_url,'')
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
		if err := studentRows.Scan(&r.StudentID, &r.StudentNumber, &r.FirstName, &r.LastName, &r.PhotoURL); err != nil {
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
	PhotoURL      string
	Cells         []ResultCell // parallel to Cols slice
}

type ResultPopupData struct {
	CSEID        int64
	SubjectLabel string
	StudentName  string
	Result       string
	IsPublished  bool
}

// ResultsGrid returns all subject columns and result cells for students enrolled
// in the given classes. Columns are ALL subjects the students are enrolled in
// (via client_subject_enrolments), not just subjects attached to these classes.
func (s *Store) ResultsGrid(ctx context.Context, classIDs []int64) ([]ResultCol, []ResultRow, error) {
	colRows, err := s.pool.Query(ctx, `
		WITH enrolled AS (
			SELECT DISTINCT cse.student_id
			FROM public.class_enrollments ce
			JOIN public.client_subject_enrolments cse ON cse.id = ce.client_subject_enrolment_id
			WHERE ce.class_id = ANY($1)
		)
		SELECT DISTINCT cse.subject_id, sub.subject_code, sub.subject_name
		FROM enrolled e
		JOIN public.client_subject_enrolments cse ON cse.student_id = e.student_id
		JOIN public.subjects sub ON sub.id = cse.subject_id
		ORDER BY sub.subject_code, sub.subject_name
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer colRows.Close()

	var cols []ResultCol
	colIdx := map[int64]int{} // subjectID → column index
	for colRows.Next() {
		var c ResultCol
		if err := colRows.Scan(&c.SubjectID, &c.SubjectCode, &c.SubjectLabel); err != nil {
			return nil, nil, err
		}
		colIdx[c.SubjectID] = len(cols)
		cols = append(cols, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, nil, err
	}
	if len(cols) == 0 {
		return cols, nil, nil
	}

	dataRows, err := s.pool.Query(ctx, `
		WITH enrolled AS (
			SELECT DISTINCT cse.student_id
			FROM public.class_enrollments ce
			JOIN public.client_subject_enrolments cse ON cse.id = ce.client_subject_enrolment_id
			WHERE ce.class_id = ANY($1)
		)
		SELECT s.id, s.student_number, p.first_given_name, p.family_name,
		       COALESCE(p.photo_url,''),
		       cse.subject_id, cse.id, COALESCE(cse.result,''), cse.result_is_published
		FROM enrolled e
		JOIN public.students s ON s.id = e.student_id
		JOIN public.people p ON p.id = s.id
		JOIN public.client_subject_enrolments cse ON cse.student_id = s.id
		ORDER BY p.family_name, p.first_given_name, cse.subject_id
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer dataRows.Close()

	var rows []ResultRow
	studentIdx := map[int64]int{}
	for dataRows.Next() {
		var studID, subjectID, cseID int64
		var studNum, firstName, lastName, photoURL, result string
		var isPub bool
		if err := dataRows.Scan(&studID, &studNum, &firstName, &lastName, &photoURL,
			&subjectID, &cseID, &result, &isPub); err != nil {
			return nil, nil, err
		}
		idx, exists := studentIdx[studID]
		if !exists {
			rows = append(rows, ResultRow{
				StudentID:     studID,
				StudentNumber: studNum,
				FirstName:     firstName,
				LastName:      lastName,
				PhotoURL:      photoURL,
				Cells:         make([]ResultCell, len(cols)),
			})
			idx = len(rows) - 1
			studentIdx[studID] = idx
		}
		if ci, ok := colIdx[subjectID]; ok {
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

// PublishSCColumn publishes all SC results for a subject across the given classes.
func (s *Store) PublishSCColumn(ctx context.Context, subjectID int64, classIDs []int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.client_subject_enrolments cse
		SET result_is_published = true
		WHERE cse.student_id IN (
			SELECT DISTINCT cse2.student_id
			FROM public.class_enrollments ce
			JOIN public.client_subject_enrolments cse2 ON cse2.id = ce.client_subject_enrolment_id
			WHERE ce.class_id = ANY($2)
		)
		AND cse.subject_id = $1
		AND cse.result = 'SC'
	`, subjectID, classIDs)
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
	PhotoURL      string
	WWCCNumber    string
	WWCCExpiryStr string
	IsStudent     bool
	IsTeacher     bool
	IsStaff       bool
	StudentNumber             string
	TeacherNumber             string
	TeacherPoliceCheckStatus  string
	TeacherPoliceCheckDateStr string
	StaffNumber               string
	StaffPoliceCheckStatus    string
	StaffPoliceCheckDateStr   string
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
		       COALESCE(p.photo_url,''),
		       COALESCE(p.wwcc_number,''),
		       COALESCE(TO_CHAR(p.wwcc_expiry,'YYYY-MM-DD'),''),
		       EXISTS(SELECT 1 FROM public.students st WHERE st.id = p.id AND st.deleted_at IS NULL),
		       EXISTS(SELECT 1 FROM public.teachers t  WHERE t.id  = p.id),
		       EXISTS(SELECT 1 FROM public.staff   sf WHERE sf.id  = p.id),
		       COALESCE((SELECT student_number FROM public.students WHERE id = p.id),''),
		       COALESCE((SELECT teacher_number FROM public.teachers WHERE id = p.id),''),
		       COALESCE((SELECT COALESCE(police_check_status,'') FROM public.teachers WHERE id = p.id),''),
		       COALESCE((SELECT TO_CHAR(police_check_date,'YYYY-MM-DD') FROM public.teachers WHERE id = p.id),''),
		       COALESCE((SELECT staff_number FROM public.staff WHERE id = p.id),''),
		       COALESCE((SELECT COALESCE(police_check_status,'') FROM public.staff WHERE id = p.id),''),
		       COALESCE((SELECT TO_CHAR(police_check_date,'YYYY-MM-DD') FROM public.staff WHERE id = p.id),'')
		FROM public.people p WHERE p.id = $1
	`, id).Scan(
		&d.ID, &d.Title, &d.FirstName, &d.FamilyName, &d.PreferredName,
		&d.DOB, &d.Gender, &d.Email, &d.PhoneMobile,
		&d.Suburb, &d.StateCode, &d.Postcode,
		&d.PhotoURL, &d.WWCCNumber, &d.WWCCExpiryStr,
		&d.IsStudent, &d.IsTeacher, &d.IsStaff,
		&d.StudentNumber,
		&d.TeacherNumber, &d.TeacherPoliceCheckStatus, &d.TeacherPoliceCheckDateStr,
		&d.StaffNumber, &d.StaffPoliceCheckStatus, &d.StaffPoliceCheckDateStr,
	)
	return d, err
}

func (s *Store) GetStudentPanel(ctx context.Context, studentID int64) (StudentPanelData, error) {
	var d StudentPanelData
	err := s.pool.QueryRow(ctx, `
		SELECT s.student_number,
		       p.family_name || ', ' || p.first_given_name,
		       COALESCE(p.photo_url,''),
		       COALESCE(TO_CHAR(p.dob,'YYYY-MM-DD'),''),
		       COALESCE(p.gender,''),
		       COALESCE(p.primary_email,''),
		       COALESCE(p.phone_mobile,''),
		       COALESCE(p.suburb,''),
		       COALESCE(p.state_code,''),
		       COALESCE(p.postcode,''),
		       COALESCE(p.wwcc_number,''),
		       COALESCE(TO_CHAR(p.wwcc_expiry,'YYYY-MM-DD'),''),
		       COUNT(cse.id) FILTER (WHERE cse.result = 'SC'),
		       COUNT(cse.id) FILTER (WHERE cse.result = 'NS'),
		       COUNT(cse.id) FILTER (WHERE cse.result = 'IP'),
		       COUNT(cse.id) FILTER (WHERE cse.result IS NULL),
		       COUNT(cse.id)
		FROM public.students s
		JOIN public.people p ON p.id = s.id
		LEFT JOIN public.client_subject_enrolments cse ON cse.student_id = s.id
		WHERE s.id = $1
		GROUP BY s.student_number, p.family_name, p.first_given_name, p.photo_url,
		         p.dob, p.gender, p.primary_email, p.phone_mobile,
		         p.suburb, p.state_code, p.postcode, p.wwcc_number, p.wwcc_expiry
	`, studentID).Scan(
		&d.StudentNumber, &d.FullName, &d.PhotoURL,
		&d.DOBStr, &d.Gender, &d.Email, &d.PhoneMobile,
		&d.Suburb, &d.StateCode, &d.Postcode,
		&d.WWCCNumber, &d.WWCCExpiryStr,
		&d.Competent, &d.NotCompetent, &d.InProgress, &d.NotYetStarted, &d.Total,
	)
	return d, err
}

func (s *Store) CreatePerson(ctx context.Context, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode, wwccNumber, wwccExpiry, photoURL string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.people
		    (title, first_given_name, family_name, preferred_name,
		     dob, gender, primary_email, phone_mobile,
		     suburb, state_code, postcode,
		     wwcc_number, wwcc_expiry,
		     photo_url, photo_uploaded_at)
		VALUES
		    (NULLIF($1,''), $2, $3, NULLIF($4,''),
		     $5::date, $6, $7, NULLIF($8,''),
		     $9, $10, $11,
		     NULLIF($12,''), NULLIF($13,'')::date,
		     NULLIF($14,''), CASE WHEN NULLIF($14,'') IS NOT NULL THEN NOW() ELSE NULL END)
		RETURNING id
	`, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
		wwccNumber, wwccExpiry, photoURL,
	).Scan(&id)
	return id, err
}

func (s *Store) UpdatePerson(ctx context.Context, id int64, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode, wwccNumber, wwccExpiry, photoURL string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.people SET
		    title            = NULLIF($2,''),
		    first_given_name = $3,
		    family_name      = $4,
		    preferred_name   = NULLIF($5,''),
		    dob              = $6::date,
		    gender           = $7,
		    primary_email    = $8,
		    phone_mobile     = NULLIF($9,''),
		    suburb           = $10,
		    state_code       = $11,
		    postcode         = $12,
		    wwcc_number      = NULLIF($13,''),
		    wwcc_expiry      = NULLIF($14,'')::date,
		    photo_url        = NULLIF($15,''),
		    photo_uploaded_at = CASE
		        WHEN NULLIF($15,'') IS NOT NULL THEN COALESCE(photo_uploaded_at, NOW())
		        ELSE NULL END,
		    updated_at       = NOW()
		WHERE id = $1
	`, id, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
		wwccNumber, wwccExpiry, photoURL,
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

func (s *Store) AddTeacherRole(ctx context.Context, personID int64, teacherNumber, teacherEmail, employmentStatus, policeCheckStatus, policeCheckDate string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.teachers (id, teacher_number, teacher_email, employment_status, police_check_status, police_check_date)
		VALUES ($1, $2, $3, $4::public.employment_type, NULLIF($5,''), NULLIF($6,'')::date)
		ON CONFLICT (id) DO UPDATE SET
		    teacher_number      = EXCLUDED.teacher_number,
		    teacher_email       = EXCLUDED.teacher_email,
		    employment_status   = EXCLUDED.employment_status,
		    police_check_status = EXCLUDED.police_check_status,
		    police_check_date   = EXCLUDED.police_check_date
	`, personID, teacherNumber, teacherEmail, employmentStatus, policeCheckStatus, policeCheckDate)
	return err
}

func (s *Store) AddStaffRole(ctx context.Context, personID int64, staffNumber, staffEmail, policeCheckStatus, policeCheckDate string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.staff (id, staff_number, staff_email, police_check_status, police_check_date)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,'')::date)
		ON CONFLICT (id) DO UPDATE SET
		    staff_number         = EXCLUDED.staff_number,
		    staff_email          = EXCLUDED.staff_email,
		    police_check_status  = EXCLUDED.police_check_status,
		    police_check_date    = EXCLUDED.police_check_date
	`, personID, staffNumber, staffEmail, policeCheckStatus, policeCheckDate)
	return err
}

// ── Admin / Programs & Classes ─────────────────────────────────────────────

type FacultyRow struct {
	ID   int64
	Name string
}

type DeliveryLocationRow struct {
	ID        int64
	Name      string
	Suburb    string
	StateCode string
	Postcode  string
}

type IntakeGroupRow struct {
	ID         int64
	IntakeName string
	GroupCode  string
	GroupName  string
}

type SubjectRow struct {
	ID               int64
	SubjectCode      string
	SubjectName      string
	FieldOfEducation string
	NominalHours     int
	VetFlag          bool
	ModuleFlag       string
}

type ProgramListRow struct {
	ID          int64
	ProgramCode string
	ProgramName string
	FacultyName string
	ProgramType string
}

type ClassListRow struct {
	ID           int64
	ClassCode    string
	PeriodName   string
	LocationName string
}

type ClassSubject struct {
	SubjectID    int64
	SubjectLabel string
}

func (s *Store) ListFaculties(ctx context.Context) ([]FacultyRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, faculty_name FROM public.faculties ORDER BY faculty_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FacultyRow
	for rows.Next() {
		var r FacultyRow
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListPrograms(ctx context.Context) ([]ProgramListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.program_code, p.program_name, f.faculty_name, COALESCE(p.program_type,'')
		FROM public.programs p
		JOIN public.faculties f ON f.id = p.faculty_id
		ORDER BY p.program_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProgramListRow
	for rows.Next() {
		var r ProgramListRow
		if err := rows.Scan(&r.ID, &r.ProgramCode, &r.ProgramName, &r.FacultyName, &r.ProgramType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateProgram(ctx context.Context, facultyID int64, programCode, programName, programRecognitionID, levelOfEducation, fieldOfEducation string, nominalHours int, vetFlag, heFlag bool, aqfLevel *int, programType string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.programs
		    (faculty_id, program_code, program_name, program_recognition_id,
		     level_of_education, field_of_education, nominal_hours, vet_flag, he_flag,
		     aqf_level, program_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11,''))
		RETURNING id
	`, facultyID, programCode, programName, programRecognitionID,
		levelOfEducation, fieldOfEducation, nominalHours, vetFlag, heFlag,
		aqfLevel, programType,
	).Scan(&id)
	return id, err
}

func (s *Store) ListDeliveryLocations(ctx context.Context) ([]DeliveryLocationRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, suburb, state_code, postcode FROM public.delivery_locations ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeliveryLocationRow
	for rows.Next() {
		var r DeliveryLocationRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Suburb, &r.StateCode, &r.Postcode); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListIntakeGroups(ctx context.Context) ([]IntakeGroupRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ig.id, pi.intake_name, ig.group_code, ig.group_name
		FROM public.intake_groups ig
		JOIN public.program_intakes pi ON pi.id = ig.intake_id
		ORDER BY pi.intake_name, ig.group_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IntakeGroupRow
	for rows.Next() {
		var r IntakeGroupRow
		if err := rows.Scan(&r.ID, &r.IntakeName, &r.GroupCode, &r.GroupName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListSubjects(ctx context.Context) ([]SubjectRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, subject_code, subject_name,
		       field_of_education, COALESCE(nominal_hours, 0), vet_flag, module_flag
		FROM public.subjects ORDER BY subject_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubjectRow
	for rows.Next() {
		var r SubjectRow
		if err := rows.Scan(&r.ID, &r.SubjectCode, &r.SubjectName,
			&r.FieldOfEducation, &r.NominalHours, &r.VetFlag, &r.ModuleFlag); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListClasses(ctx context.Context) ([]ClassListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.class_code, ap.period_name || ' ' || ap.year, dl.name
		FROM public.classes c
		JOIN public.academic_periods ap ON ap.id = c.academic_period_id
		JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
		ORDER BY ap.year DESC, ap.sequence_number DESC, c.class_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClassListRow
	for rows.Next() {
		var r ClassListRow
		if err := rows.Scan(&r.ID, &r.ClassCode, &r.PeriodName, &r.LocationName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── Admin / Periods, Locations, Intakes, Sessions ─────────────────────────

type PeriodListRow struct {
	ID             int64
	PeriodCode     string
	Year           int
	PeriodName     string
	StartDate      time.Time
	EndDate        time.Time
	PeriodType     string
	SequenceNumber int
}

type TrainingOrgRow struct {
	ID   int64
	Code string
	Name string
}

type ProgramIntakeRow struct {
	ID         int64
	IntakeCode string
	IntakeName string
}

type SessionListRow struct {
	ID          int64
	ClassCode   string
	SessionDate time.Time
	StartTime   time.Time
	EndTime     time.Time
	SessionType string
	Cancelled   bool
}

type SessionInput struct {
	Date      string // YYYY-MM-DD
	StartTime string // HH:MM
	EndTime   string // HH:MM
	Type      string
	Notes     string
}

func (s *Store) ListPeriods(ctx context.Context) ([]PeriodListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, period_code, year, period_name,
		       start_date, end_date, period_type,
		       COALESCE(sequence_number, 0)
		FROM public.academic_periods
		ORDER BY year DESC, COALESCE(sequence_number, 999)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeriodListRow
	for rows.Next() {
		var r PeriodListRow
		if err := rows.Scan(&r.ID, &r.PeriodCode, &r.Year, &r.PeriodName,
			&r.StartDate, &r.EndDate, &r.PeriodType, &r.SequenceNumber); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreatePeriod(ctx context.Context, periodCode, periodName string, year int, startDate, endDate, periodType string, seqNum *int) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.academic_periods
		    (period_code, year, period_name, start_date, end_date, period_type, sequence_number)
		VALUES ($1, $2, $3, $4::date, $5::date, $6, $7)
		RETURNING id
	`, periodCode, year, periodName, startDate, endDate, periodType, seqNum).Scan(&id)
	return id, err
}

func (s *Store) ListTrainingOrgs(ctx context.Context) ([]TrainingOrgRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, training_org_id, training_org_name
		FROM public.training_orgs
		ORDER BY training_org_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrainingOrgRow
	for rows.Next() {
		var r TrainingOrgRow
		if err := rows.Scan(&r.ID, &r.Code, &r.Name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateDeliveryLocation(ctx context.Context, trainingOrgID int64, locID, name, address, suburb, stateCode, postcode, postcodeOverride string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.delivery_locations
		    (training_org_id, delivery_loc_id, name, address, suburb, state_code, postcode, postcode_override)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8,''))
		RETURNING id
	`, trainingOrgID, locID, name, address, suburb, stateCode, postcode, postcodeOverride).Scan(&id)
	return id, err
}

func (s *Store) ListProgramIntakes(ctx context.Context) ([]ProgramIntakeRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, intake_code, intake_name
		FROM public.program_intakes
		ORDER BY intake_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProgramIntakeRow
	for rows.Next() {
		var r ProgramIntakeRow
		if err := rows.Scan(&r.ID, &r.IntakeCode, &r.IntakeName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateIntakeGroup(ctx context.Context, intakeID int64, groupCode, groupName string, capacity *int, notes string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.intake_groups (intake_id, group_code, group_name, capacity, notes)
		VALUES ($1, $2, $3, $4, NULLIF($5,''))
		RETURNING id
	`, intakeID, groupCode, groupName, capacity, notes).Scan(&id)
	return id, err
}

func (s *Store) ListSessionsForClass(ctx context.Context, classID int64) ([]SessionListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cs.id, c.class_code, cs.session_date, cs.start_time, cs.end_time, cs.session_type, cs.cancelled
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		WHERE cs.class_id = $1
		ORDER BY cs.session_date, cs.start_time
	`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionListRow
	for rows.Next() {
		var r SessionListRow
		if err := rows.Scan(&r.ID, &r.ClassCode, &r.SessionDate, &r.StartTime, &r.EndTime, &r.SessionType, &r.Cancelled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateSession(ctx context.Context, classID int64, sessionDate, startTime, endTime, sessionType, notes string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.class_sessions
		    (class_id, session_date, start_time, end_time, session_type, notes)
		VALUES ($1, $2::date, $3::time, $4::time, $5, NULLIF($6,''))
		ON CONFLICT (class_id, session_date, start_time) DO NOTHING
		RETURNING id
	`, classID, sessionDate, startTime, endTime, sessionType, notes).Scan(&id)
	return id, err
}

// BulkCreateSessions inserts sessions idempotently, skipping any that already exist.
// Returns count of newly inserted sessions.
func (s *Store) BulkCreateSessions(ctx context.Context, classID int64, sessions []SessionInput) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	inserted := 0
	for _, si := range sessions {
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO public.class_sessions
			    (class_id, session_date, start_time, end_time, session_type, notes)
			VALUES ($1, $2::date, $3::time, $4::time, $5, NULLIF($6,''))
			ON CONFLICT (class_id, session_date, start_time) DO NOTHING
			RETURNING id
		`, classID, si.Date, si.StartTime, si.EndTime, si.Type, si.Notes).Scan(&id)
		if err == nil {
			inserted++
		}
	}
	return inserted, tx.Commit(ctx)
}

func (s *Store) CreateFaculty(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.faculties (faculty_name) VALUES ($1) RETURNING id
	`, name).Scan(&id)
	return id, err
}

func (s *Store) CreateSubject(ctx context.Context, subjectCode, subjectName, moduleFlag, fieldOfEducation string, nominalHours *int, vetFlag bool, creditPoints *int) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.subjects
		    (subject_code, subject_name, module_flag, field_of_education, nominal_hours, vet_flag, credit_points)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, subjectCode, subjectName, moduleFlag, fieldOfEducation, nominalHours, vetFlag, creditPoints).Scan(&id)
	return id, err
}

func (s *Store) CreateClass(ctx context.Context, classCode string, academicPeriodID, deliveryLocationID int64, intakeGroupID *int64, enrolmentCap *int, subjects []ClassSubject) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO public.classes (class_code, academic_period_id, delivery_location_id, intake_group_id, enrolment_cap)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, classCode, academicPeriodID, deliveryLocationID, intakeGroupID, enrolmentCap).Scan(&id)
	if err != nil {
		return 0, err
	}

	for _, cs := range subjects {
		_, err = tx.Exec(ctx, `
			INSERT INTO public.class_subjects (class_id, subject_id, subject_label)
			VALUES ($1, $2, $3)
		`, id, cs.SubjectID, cs.SubjectLabel)
		if err != nil {
			return 0, err
		}
	}

	return id, tx.Commit(ctx)
}
