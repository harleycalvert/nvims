package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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
	StartDate  string // YYYY-MM-DD
	EndDate    string // YYYY-MM-DD
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
		SELECT id, period_code, period_name, year,
		       TO_CHAR(start_date,'YYYY-MM-DD'), TO_CHAR(end_date,'YYYY-MM-DD')
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
		if err := rows.Scan(&p.ID, &p.PeriodCode, &p.PeriodName, &p.Year, &p.StartDate, &p.EndDate); err != nil {
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
	PersonID     int64
	Username     string
	FullName     string
	Role         string
	PasswordHash string
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (AuthUser, error) {
	var u AuthUser
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, COALESCE(u.person_id, 0), u.username,
		       COALESCE(string_agg(r.role, ', ' ORDER BY r.role), '') AS role,
		       u.password_hash,
		       COALESCE(p.first_given_name || ' ' || p.family_name, u.username)
		FROM public.app_users u
		LEFT JOIN public.people p ON p.id = u.person_id
		LEFT JOIN public.app_user_roles r ON r.user_id = u.id AND r.revoked_at IS NULL
		WHERE u.username = $1 AND u.is_active = true
		GROUP BY u.id, u.person_id, u.username, u.password_hash,
		         p.first_given_name, p.family_name
	`, username).Scan(&u.ID, &u.PersonID, &u.Username, &u.Role, &u.PasswordHash, &u.FullName)
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
		SELECT cse.id,
		       sub.subject_code || ' — ' || sub.subject_name,
		       p.first_given_name || ' ' || p.family_name,
		       COALESCE(cse.result,''), cse.result_is_published
		FROM public.client_subject_enrolments cse
		JOIN public.students s ON s.id = cse.student_id
		JOIN public.people p ON p.id = s.id
		JOIN public.subjects sub ON sub.id = cse.subject_id
		WHERE cse.id = $1
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
	ID            int64
	FirstName     string
	FamilyName    string
	DOB           time.Time
	IsStudent     bool
	IsTeacher     bool
	IsStaff       bool
	StudentNumber string
	StaffNumber   string
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
	PoliceCheckStatus  string
	PoliceCheckDateStr string
	IsStudent     bool
	IsTeacher     bool
	IsStaff       bool
	StudentNumber string
	StaffNumber   string
}

type PeopleResult struct {
	Rows  []PersonListRow
	Total int // total matching rows across all pages
}

func (s *Store) ListPeople(ctx context.Context, search, role string, limit int) (PeopleResult, error) {
	const sel = `
		SELECT p.id, p.first_given_name, p.family_name, p.dob,
		       (st.id IS NOT NULL)            AS is_student,
		       (t.id  IS NOT NULL)            AS is_teacher,
		       (sf.id IS NOT NULL)            AS is_staff,
		       COALESCE(st.student_number, '') AS student_number,
		       COALESCE(sf.staff_number,   '') AS staff_number,
		       COUNT(*) OVER ()               AS total_count
		FROM public.people p
		LEFT JOIN public.students st ON st.id = p.id AND st.deleted_at IS NULL
		LEFT JOIN public.staff     sf ON sf.id = p.id
		LEFT JOIN public.teachers  t  ON t.id  = sf.id
	`
	var conds []string
	var args []any

	if search != "" {
		args = append(args, "%"+search+"%")
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			`(p.family_name ILIKE $%[1]d OR p.first_given_name ILIKE $%[1]d
			  OR st.student_number ILIKE $%[1]d OR st.student_email ILIKE $%[1]d
			  OR sf.staff_number   ILIKE $%[1]d OR sf.staff_email   ILIKE $%[1]d)`,
			n,
		))
	}
	switch role {
	case "Student":
		conds = append(conds, "st.id IS NOT NULL")
	case "Teacher":
		conds = append(conds, "t.id IS NOT NULL")
	case "Staff":
		conds = append(conds, "sf.id IS NOT NULL")
	}

	q := sel
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)
	q += fmt.Sprintf(" ORDER BY p.family_name, p.first_given_name LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return PeopleResult{}, err
	}
	defer rows.Close()
	var res PeopleResult
	for rows.Next() {
		var r PersonListRow
		if err := rows.Scan(&r.ID, &r.FirstName, &r.FamilyName, &r.DOB,
			&r.IsStudent, &r.IsTeacher, &r.IsStaff,
			&r.StudentNumber, &r.StaffNumber,
			&res.Total); err != nil {
			return PeopleResult{}, err
		}
		res.Rows = append(res.Rows, r)
	}
	return res, rows.Err()
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
		       COALESCE(p.police_check_status,''),
		       COALESCE(TO_CHAR(p.police_check_date,'YYYY-MM-DD'),''),
		       EXISTS(SELECT 1 FROM public.students st WHERE st.id = p.id AND st.deleted_at IS NULL),
		       EXISTS(SELECT 1 FROM public.teachers t  WHERE t.id  = p.id),
		       EXISTS(SELECT 1 FROM public.staff   sf WHERE sf.id  = p.id),
		       COALESCE((SELECT student_number FROM public.students WHERE id = p.id),''),
		       COALESCE((SELECT staff_number   FROM public.staff    WHERE id = p.id),'')
		FROM public.people p WHERE p.id = $1
	`, id).Scan(
		&d.ID, &d.Title, &d.FirstName, &d.FamilyName, &d.PreferredName,
		&d.DOB, &d.Gender, &d.Email, &d.PhoneMobile,
		&d.Suburb, &d.StateCode, &d.Postcode,
		&d.PhotoURL, &d.WWCCNumber, &d.WWCCExpiryStr,
		&d.PoliceCheckStatus, &d.PoliceCheckDateStr,
		&d.IsStudent, &d.IsTeacher, &d.IsStaff,
		&d.StudentNumber, &d.StaffNumber,
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

func (s *Store) CreatePerson(ctx context.Context, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode, wwccNumber, wwccExpiry, photoURL, policeCheckStatus, policeCheckDate string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.people
		    (title, first_given_name, family_name, preferred_name,
		     dob, gender, primary_email, phone_mobile,
		     suburb, state_code, postcode,
		     wwcc_number, wwcc_expiry,
		     photo_url, photo_uploaded_at,
		     police_check_status, police_check_date)
		VALUES
		    (NULLIF($1,''), $2, $3, NULLIF($4,''),
		     $5::date, $6, $7, NULLIF($8,''),
		     $9, $10, $11,
		     NULLIF($12,''), NULLIF($13,'')::date,
		     NULLIF($14,''), CASE WHEN NULLIF($14,'') IS NOT NULL THEN NOW() ELSE NULL END,
		     NULLIF($15,''), NULLIF($16,'')::date)
		RETURNING id
	`, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
		wwccNumber, wwccExpiry, photoURL,
		policeCheckStatus, policeCheckDate,
	).Scan(&id)
	return id, err
}

func (s *Store) UpdatePerson(ctx context.Context, id int64, title, firstName, familyName, preferredName, dob, gender, email, phoneMobile, suburb, stateCode, postcode, wwccNumber, wwccExpiry, photoURL, policeCheckStatus, policeCheckDate string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.people SET
		    title               = NULLIF($2,''),
		    first_given_name    = $3,
		    family_name         = $4,
		    preferred_name      = NULLIF($5,''),
		    dob                 = $6::date,
		    gender              = $7,
		    primary_email       = $8,
		    phone_mobile        = NULLIF($9,''),
		    suburb              = $10,
		    state_code          = $11,
		    postcode            = $12,
		    wwcc_number         = NULLIF($13,''),
		    wwcc_expiry         = NULLIF($14,'')::date,
		    photo_url           = NULLIF($15,''),
		    photo_uploaded_at   = CASE
		        WHEN NULLIF($15,'') IS NOT NULL THEN COALESCE(photo_uploaded_at, NOW())
		        ELSE NULL END,
		    police_check_status = NULLIF($16,''),
		    police_check_date   = NULLIF($17,'')::date,
		    updated_at          = NOW()
		WHERE id = $1
	`, id, title, firstName, familyName, preferredName,
		dob, gender, email, phoneMobile,
		suburb, stateCode, postcode,
		wwccNumber, wwccExpiry, photoURL,
		policeCheckStatus, policeCheckDate,
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

func (s *Store) AddTeacherRole(ctx context.Context, personID int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.teachers (id)
		VALUES ($1)
		ON CONFLICT (id) DO NOTHING
	`, personID)
	return err
}

func (s *Store) AddStaffRole(ctx context.Context, personID int64, staffNumber, staffEmail, employmentStatus string, fte float64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.staff (id, staff_number, staff_email, employment_status, fte)
		VALUES ($1, $2, $3, $4::public.employment_type, $5)
		ON CONFLICT (id) DO UPDATE SET
		    staff_number      = EXCLUDED.staff_number,
		    staff_email       = EXCLUDED.staff_email,
		    employment_status = EXCLUDED.employment_status,
		    fte               = EXCLUDED.fte
	`, personID, staffNumber, staffEmail, employmentStatus, fte)
	return err
}

type RoleDetail struct {
	Number           string
	Email            string
	EmploymentStatus string
	FTE              float64
}

func (s *Store) GetRoleDetail(ctx context.Context, personID int64, roleType string) (RoleDetail, error) {
	var d RoleDetail
	var err error
	switch roleType {
	case "student":
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(student_number,''), COALESCE(student_email,'')
			FROM public.students WHERE id = $1 AND deleted_at IS NULL
		`, personID).Scan(&d.Number, &d.Email)
	case "teacher":
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(sf.staff_number,''), COALESCE(sf.staff_email,''),
			       COALESCE(sf.employment_status::text,''), sf.fte
			FROM public.teachers t
			JOIN public.staff sf ON sf.id = t.id
			WHERE t.id = $1
		`, personID).Scan(&d.Number, &d.Email, &d.EmploymentStatus, &d.FTE)
	case "staff":
		err = s.pool.QueryRow(ctx, `
			SELECT COALESCE(staff_number,''), COALESCE(staff_email,''),
			       COALESCE(employment_status::text,''), fte
			FROM public.staff WHERE id = $1
		`, personID).Scan(&d.Number, &d.Email, &d.EmploymentStatus, &d.FTE)
	}
	return d, err
}

const scheduledSessionSQL = `
	SELECT cs.id, c.id, c.class_code,
	       cs.session_date, cs.start_time, cs.end_time,
	       cs.session_type, COALESCE(cs.notes,''), cs.cancelled,
	       COALESCE(string_agg(
	           DISTINCT p.family_name || ', ' || p.first_given_name,
	           ' · ' ORDER BY p.family_name || ', ' || p.first_given_name
	       ),''),
	       dl.name,
	       COALESCE(cs.room_id, 0),
	       COALESCE(r.building_id, 0),
	       COALESCE(b.building_name,''),
	       COALESCE(r.room_name,'')
	FROM public.class_sessions cs
	JOIN public.classes c ON c.id = cs.class_id
	JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
	LEFT JOIN public.rooms r ON r.id = cs.room_id
	LEFT JOIN public.buildings b ON b.id = r.building_id
	LEFT JOIN public.session_teachers st2 ON st2.session_id = cs.id
	LEFT JOIN public.teachers t2 ON t2.id = st2.teacher_id
	LEFT JOIN public.people p ON p.id = t2.id
`

func scanScheduledRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]ScheduledSessionRow, error) {
	defer rows.Close()
	var out []ScheduledSessionRow
	for rows.Next() {
		var r ScheduledSessionRow
		if err := rows.Scan(&r.ID, &r.ClassID, &r.ClassCode,
			&r.SessionDate, &r.StartTime, &r.EndTime,
			&r.SessionType, &r.Notes, &r.Cancelled,
			&r.Teachers, &r.Location,
			&r.RoomID, &r.BuildingID, &r.BuildingName, &r.RoomName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) SessionsForTeacher(ctx context.Context, teacherID int64, from, to string, classIDs []int64) ([]ScheduledSessionRow, error) {
	args := []any{teacherID, from, to}
	classFilter := ""
	if len(classIDs) > 0 {
		placeholders := make([]string, len(classIDs))
		for i, id := range classIDs {
			args = append(args, id)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		classFilter = " AND c.id IN (" + strings.Join(placeholders, ",") + ")"
	}
	q := scheduledSessionSQL + `
	JOIN public.session_teachers st ON st.session_id = cs.id AND st.teacher_id = $1
	WHERE cs.session_date >= $2::date AND cs.session_date <= $3::date` + classFilter + `
	GROUP BY cs.id, c.id, c.class_code, cs.session_date, cs.start_time, cs.end_time,
	         cs.session_type, cs.notes, cs.cancelled, dl.name,
	         cs.room_id, r.building_id, b.building_name, r.room_name
	ORDER BY cs.session_date, cs.start_time`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanScheduledRows(rows)
}

func (s *Store) TeacherClassesForPeriod(ctx context.Context, teacherID, periodID int64) ([]ClassListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT c.id, c.class_code, ap.period_name || ' ' || ap.year, dl.name
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		JOIN public.academic_periods ap ON ap.id = c.academic_period_id
		JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
		JOIN public.session_teachers st ON st.session_id = cs.id
		WHERE st.teacher_id = $1 AND c.academic_period_id = $2
		ORDER BY c.class_code
	`, teacherID, periodID)
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

func (s *Store) GroupClassesForPeriod(ctx context.Context, groupID, periodID int64) ([]ClassListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT c.id, c.class_code, ap.period_name || ' ' || ap.year, dl.name
		FROM public.classes c
		JOIN public.academic_periods ap ON ap.id = c.academic_period_id
		JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
		WHERE c.intake_group_id = $1 AND c.academic_period_id = $2
		ORDER BY c.class_code
	`, groupID, periodID)
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

func (s *Store) SessionsForIntakeGroup(ctx context.Context, groupID int64, from, to string, classIDs []int64) ([]ScheduledSessionRow, error) {
	args := []any{groupID, from, to}
	classFilter := ""
	if len(classIDs) > 0 {
		placeholders := make([]string, len(classIDs))
		for i, id := range classIDs {
			args = append(args, id)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		classFilter = " AND c.id IN (" + strings.Join(placeholders, ",") + ")"
	}
	q := scheduledSessionSQL + `
	WHERE c.intake_group_id = $1
	  AND cs.session_date >= $2::date AND cs.session_date <= $3::date` + classFilter + `
	GROUP BY cs.id, c.id, c.class_code, cs.session_date, cs.start_time, cs.end_time,
	         cs.session_type, cs.notes, cs.cancelled, dl.name,
	         cs.room_id, r.building_id, b.building_name, r.room_name
	ORDER BY cs.session_date, cs.start_time
	`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scanScheduledRows(rows)
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
	IntakeID   int64
	IntakeName string
	GroupCode  string
	GroupName  string
	Capacity   int
	Notes      string
}

type SubjectRow struct {
	ID               int64
	SubjectCode      string
	SubjectName      string
	FieldOfEducation string
	NominalHours     int
	CreditPoints     int
	VetFlag          bool
	ModuleFlag       string
}

type ProgramListRow struct {
	ID                   int64
	ProgramCode          string
	ProgramName          string
	FacultyID            int64
	FacultyName          string
	ProgramType          string
	ProgramRecognitionID string
	LevelOfEducation     string
	FieldOfEducation     string
	NominalHours         int
	VetFlag              bool
	HeFlag               bool
	AQFLevel             int // 0 means NULL
}

type ClassListRow struct {
	ID                 int64
	ClassCode          string
	PeriodName         string
	LocationName       string
	AcademicPeriodID   int64
	DeliveryLocationID int64
	IntakeGroupID      int64
	EnrolmentCap       int
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
		SELECT p.id, p.program_code, p.program_name, p.faculty_id, f.faculty_name,
		       COALESCE(p.program_type,''), p.program_recognition_id,
		       p.level_of_education, p.field_of_education, p.nominal_hours,
		       p.vet_flag, p.he_flag, COALESCE(p.aqf_level, 0)
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
		if err := rows.Scan(&r.ID, &r.ProgramCode, &r.ProgramName, &r.FacultyID, &r.FacultyName,
			&r.ProgramType, &r.ProgramRecognitionID, &r.LevelOfEducation,
			&r.FieldOfEducation, &r.NominalHours, &r.VetFlag, &r.HeFlag, &r.AQFLevel); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProgram(ctx context.Context, id, facultyID int64, programCode, programName, programRecognitionID, levelOfEducation, fieldOfEducation string, nominalHours int, vetFlag, heFlag bool, aqfLevel *int, programType string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.programs
		SET faculty_id=$2, program_code=$3, program_name=$4,
		    program_recognition_id=$5, level_of_education=$6, field_of_education=$7,
		    nominal_hours=$8, vet_flag=$9, he_flag=$10,
		    aqf_level=$11, program_type=NULLIF($12,'')
		WHERE id=$1
	`, id, facultyID, programCode, programName,
		programRecognitionID, levelOfEducation, fieldOfEducation,
		nominalHours, vetFlag, heFlag, aqfLevel, programType)
	return err
}

func (s *Store) DeleteProgram(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.programs WHERE id=$1`, id)
	return err
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
		SELECT id, name, COALESCE(suburb,''), COALESCE(state_code,''), COALESCE(postcode,'') FROM public.delivery_locations ORDER BY name
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
		SELECT ig.id, ig.intake_id, pi.intake_name, ig.group_code, ig.group_name,
		       COALESCE(ig.capacity,0), COALESCE(ig.notes,'')
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
		if err := rows.Scan(&r.ID, &r.IntakeID, &r.IntakeName, &r.GroupCode, &r.GroupName, &r.Capacity, &r.Notes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListSubjects(ctx context.Context) ([]SubjectRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, subject_code, subject_name,
		       field_of_education, COALESCE(nominal_hours,0), COALESCE(credit_points,0), vet_flag, module_flag
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
			&r.FieldOfEducation, &r.NominalHours, &r.CreditPoints, &r.VetFlag, &r.ModuleFlag); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListClasses(ctx context.Context) ([]ClassListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.class_code, ap.period_name || ' ' || ap.year, dl.name,
		       c.academic_period_id, c.delivery_location_id,
		       COALESCE(c.intake_group_id,0), COALESCE(c.enrolment_cap,0)
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
		if err := rows.Scan(&r.ID, &r.ClassCode, &r.PeriodName, &r.LocationName,
			&r.AcademicPeriodID, &r.DeliveryLocationID, &r.IntakeGroupID, &r.EnrolmentCap); err != nil {
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
	ID          int64
	Code        string
	Name        string
	OrgType     string
	Address     string
	Suburb      string
	StateCode   string
	Postcode    string
	ContactName string
	Telephone   string
	Email       string
}

type DeliveryLocationFull struct {
	ID            int64
	TrainingOrgID int64
	OrgName       string
	LocID         string
	Name          string
	IsVirtual     bool
	Address       string
	Suburb        string
	StateCode     string
	Postcode      string
	Latitude      string
	Longitude     string
}

type BuildingRow struct {
	ID           int64
	LocationID   int64
	LocationName string
	BuildingName string
	Address      string
	Suburb       string
	StateCode    string
	Postcode     string
	Latitude     string
	Longitude    string
}

type RoomRow struct {
	ID            int64
	BuildingID    int64
	BuildingName  string
	LocationName  string
	RoomName      string
	Capacity      int
	RoomType      string
	IsActive      bool
	IsComputerLab bool
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
	Notes       string
}

type TeacherListRow struct {
	ID          int64
	StaffNumber string
	FullName    string
}

type ScheduleSession struct {
	SessionDate  time.Time
	StartTime    time.Time
	EndTime      time.Time
	ClassCode    string
	SubjectCodes string
	SessionType  string
}

type ScheduledSessionRow struct {
	ID           int64
	ClassID      int64
	ClassCode    string
	SessionDate  time.Time
	StartTime    time.Time
	EndTime      time.Time
	SessionType  string
	Notes        string
	Cancelled    bool
	Teachers     string
	Location     string // delivery location name
	RoomID       int64
	BuildingID   int64
	BuildingName string
	RoomName     string
}

type SessionInput struct {
	Date      string // YYYY-MM-DD
	StartTime string // HH:MM
	EndTime   string // HH:MM
	Type      string
	Notes     string
}

// ── Timetable ──────────────────────────────────────────────────────────────

type TimetableFilters struct {
	PersonID int64 // show sessions where this person is a teacher OR a student (for "my schedule")
}

type TimetableSession struct {
	ID           int64
	ClassID      int64
	ClassCode    string
	SessionDate  time.Time
	StartTime    time.Time
	EndTime      time.Time
	SessionType  string
	LocationName string
	Teachers     string
	ProgramCode  string
	GroupInfo    string
	SubjectCodes string
	BuildingName string
	BuildingLat  string
	BuildingLng  string
}

func (s *Store) TimetableRange(ctx context.Context, start, end time.Time, f TimetableFilters) ([]TimetableSession, error) {
	var extraJoins []string
	var extraWheres []string
	args := []any{start, end}

	add := func(v int64) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.PersonID > 0 {
		p := add(f.PersonID)
		extraWheres = append(extraWheres, `(
			EXISTS (SELECT 1 FROM public.session_teachers _pst WHERE _pst.session_id = cs.id AND _pst.teacher_id = `+p+`)
			OR EXISTS (
				SELECT 1 FROM public.class_enrollments _pce
				JOIN public.client_subject_enrolments _pcse ON _pcse.id = _pce.client_subject_enrolment_id
				WHERE _pce.class_id = c.id AND _pcse.student_id = `+p+`
			)
		)`)
	}

	joinSQL := strings.Join(extraJoins, "\n")
	whereSQL := ""
	if len(extraWheres) > 0 {
		whereSQL = "AND " + strings.Join(extraWheres, "\n  AND ")
	}

	q := fmt.Sprintf(`
		SELECT cs.id, c.id, c.class_code,
		       cs.session_date, cs.start_time, cs.end_time, cs.session_type,
		       dl.name,
		       COALESCE(string_agg(
		           p.family_name || ', ' || p.first_given_name,
		           ' · ' ORDER BY p.family_name, p.first_given_name
		       ), ''),
		       COALESCE(prog.program_code, ''),
		       COALESCE(ig.group_code, '') || ' ' || COALESCE(ig.group_name, ''),
		       (SELECT COALESCE(string_agg(subj.subject_code, ' · ' ORDER BY subj.subject_code), '')
		        FROM public.class_subjects _cs3
		        JOIN public.subjects subj ON subj.id = _cs3.subject_id
		        WHERE _cs3.class_id = c.id),
		       COALESCE(b.building_name,''),
		       COALESCE(b.latitude::text,''),
		       COALESCE(b.longitude::text,'')
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		JOIN public.delivery_locations dl ON dl.id = c.delivery_location_id
		LEFT JOIN public.intake_groups ig ON ig.id = c.intake_group_id
		LEFT JOIN public.program_intakes pi ON pi.id = ig.intake_id
		LEFT JOIN public.programs prog ON prog.id = pi.program_id
		LEFT JOIN public.session_teachers st ON st.session_id = cs.id
		LEFT JOIN public.teachers t  ON t.id = st.teacher_id
		LEFT JOIN public.people   p  ON p.id = t.id
		LEFT JOIN public.rooms r ON r.id = cs.room_id
		LEFT JOIN public.buildings b ON b.id = r.building_id
		%s
		WHERE cs.session_date >= $1 AND cs.session_date < $2
		  AND NOT cs.cancelled
		  %s
		GROUP BY cs.id, c.id, c.class_code,
		         cs.session_date, cs.start_time, cs.end_time, cs.session_type, dl.name,
		         prog.program_code, ig.group_code, ig.group_name,
		         b.building_name, b.latitude, b.longitude
		ORDER BY cs.session_date, cs.start_time, c.class_code
	`, joinSQL, whereSQL)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimetableSession
	for rows.Next() {
		var ts TimetableSession
		if err := rows.Scan(&ts.ID, &ts.ClassID, &ts.ClassCode,
			&ts.SessionDate, &ts.StartTime, &ts.EndTime, &ts.SessionType,
			&ts.LocationName, &ts.Teachers,
			&ts.ProgramCode, &ts.GroupInfo, &ts.SubjectCodes,
			&ts.BuildingName, &ts.BuildingLat, &ts.BuildingLng); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
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

func (s *Store) UpdatePeriod(ctx context.Context, id int64, periodCode, periodName string, year int, startDate, endDate, periodType string, seqNum *int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.academic_periods
		SET period_code=$2, period_name=$3, year=$4,
		    start_date=$5::date, end_date=$6::date,
		    period_type=$7, sequence_number=$8
		WHERE id=$1
	`, id, periodCode, periodName, year, startDate, endDate, periodType, seqNum)
	return err
}

func (s *Store) DeletePeriod(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.academic_periods WHERE id=$1`, id)
	return err
}

func (s *Store) ListTrainingOrgs(ctx context.Context) ([]TrainingOrgRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, training_org_id, training_org_name, training_org_type,
		       address_first_line, suburb, state_code, postcode,
		       COALESCE(contact_name,''), COALESCE(telephone,''), COALESCE(email,'')
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
		if err := rows.Scan(&r.ID, &r.Code, &r.Name, &r.OrgType,
			&r.Address, &r.Suburb, &r.StateCode, &r.Postcode,
			&r.ContactName, &r.Telephone, &r.Email); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateTrainingOrg(ctx context.Context, code, name, orgType, address, suburb, stateCode, postcode, contactName, telephone, email string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.training_orgs
		    (training_org_id, training_org_name, training_org_type,
		     address_first_line, suburb, state_code, postcode,
		     contact_name, telephone, email)
		VALUES ($1, $2, $3, $4, $5, $6, $7,
		        NULLIF($8,''), NULLIF($9,''), NULLIF($10,''))
		RETURNING id
	`, code, name, orgType, address, suburb, stateCode, postcode,
		contactName, telephone, email).Scan(&id)
	return id, err
}

func (s *Store) UpdateTrainingOrg(ctx context.Context, id int64, code, name, orgType, address, suburb, stateCode, postcode, contactName, telephone, email string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.training_orgs SET
		    training_org_id   = $2,
		    training_org_name = $3,
		    training_org_type = $4,
		    address_first_line= $5,
		    suburb            = $6,
		    state_code        = $7,
		    postcode          = $8,
		    contact_name      = NULLIF($9,''),
		    telephone         = NULLIF($10,''),
		    email             = NULLIF($11,'')
		WHERE id = $1
	`, id, code, name, orgType, address, suburb, stateCode, postcode,
		contactName, telephone, email)
	return err
}

func (s *Store) DeleteTrainingOrg(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.training_orgs WHERE id=$1`, id)
	return err
}

func (s *Store) ListDeliveryLocationsFull(ctx context.Context) ([]DeliveryLocationFull, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dl.id, dl.training_org_id, t.training_org_name,
		       dl.delivery_loc_id, dl.name,
		       COALESCE(dl.is_virtual, false),
		       COALESCE(dl.address,''), COALESCE(dl.suburb,''),
		       COALESCE(dl.state_code,''), COALESCE(dl.postcode,''),
		       COALESCE(dl.latitude::text,''), COALESCE(dl.longitude::text,'')
		FROM public.delivery_locations dl
		JOIN public.training_orgs t ON t.id = dl.training_org_id
		ORDER BY t.training_org_name, dl.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeliveryLocationFull
	for rows.Next() {
		var r DeliveryLocationFull
		if err := rows.Scan(&r.ID, &r.TrainingOrgID, &r.OrgName,
			&r.LocID, &r.Name, &r.IsVirtual,
			&r.Address, &r.Suburb, &r.StateCode, &r.Postcode,
			&r.Latitude, &r.Longitude); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDeliveryLocation(ctx context.Context, id, trainingOrgID int64, locID, name string, isVirtual bool, address, suburb, stateCode, postcode, lat, lng string) error {
	var addrVal, suburbVal, stateVal, postcodeVal any
	if isVirtual {
		addrVal, suburbVal, stateVal, postcodeVal = nil, nil, nil, nil
	} else {
		addrVal, suburbVal, stateVal, postcodeVal = address, suburb, stateCode, postcode
	}
	var latVal, lngVal any
	if lat != "" {
		latVal = lat
	}
	if lng != "" {
		lngVal = lng
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE public.delivery_locations SET
		    training_org_id  = $2,
		    delivery_loc_id  = $3,
		    name             = $4,
		    is_virtual       = $5,
		    address          = $6,
		    suburb           = $7,
		    state_code       = $8,
		    postcode         = $9,
		    latitude         = NULLIF($10::text,'')::numeric,
		    longitude        = NULLIF($11::text,'')::numeric
		WHERE id = $1
	`, id, trainingOrgID, locID, name, isVirtual,
		addrVal, suburbVal, stateVal, postcodeVal, latVal, lngVal)
	return err
}

func (s *Store) DeleteDeliveryLocation(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.delivery_locations WHERE id=$1`, id)
	return err
}

func (s *Store) ListBuildings(ctx context.Context) ([]BuildingRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.id, b.delivery_location_id, dl.name, b.building_name,
		       COALESCE(b.address,''), COALESCE(b.suburb,''),
		       COALESCE(b.state_code,''), COALESCE(b.postcode,''),
		       COALESCE(b.latitude::text,''), COALESCE(b.longitude::text,'')
		FROM public.buildings b
		JOIN public.delivery_locations dl ON dl.id = b.delivery_location_id
		ORDER BY dl.name, b.building_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BuildingRow
	for rows.Next() {
		var r BuildingRow
		if err := rows.Scan(&r.ID, &r.LocationID, &r.LocationName, &r.BuildingName,
			&r.Address, &r.Suburb, &r.StateCode, &r.Postcode,
			&r.Latitude, &r.Longitude); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateBuilding(ctx context.Context, locationID int64, name, address, suburb, stateCode, postcode, lat, lng string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.buildings (delivery_location_id, building_name, address, suburb, state_code, postcode, latitude, longitude)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''),
		        NULLIF($7::text,'')::numeric, NULLIF($8::text,'')::numeric)
		RETURNING id
	`, locationID, name, address, suburb, stateCode, postcode, lat, lng).Scan(&id)
	return id, err
}

func (s *Store) UpdateBuilding(ctx context.Context, id, locationID int64, name, address, suburb, stateCode, postcode, lat, lng string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.buildings
		SET delivery_location_id=$2, building_name=$3,
		    address=NULLIF($4,''), suburb=NULLIF($5,''),
		    state_code=NULLIF($6,''), postcode=NULLIF($7,''),
		    latitude=NULLIF($8::text,'')::numeric, longitude=NULLIF($9::text,'')::numeric
		WHERE id=$1
	`, id, locationID, name, address, suburb, stateCode, postcode, lat, lng)
	return err
}

func (s *Store) DeleteBuilding(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.buildings WHERE id=$1`, id)
	return err
}

func (s *Store) ListRooms(ctx context.Context) ([]RoomRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.building_id, b.building_name, dl.name,
		       r.room_name, r.capacity,
		       COALESCE(r.room_type,'Classroom'),
		       COALESCE(r.is_active, true),
		       COALESCE(r.is_computer_lab, false)
		FROM public.rooms r
		JOIN public.buildings b ON b.id = r.building_id
		JOIN public.delivery_locations dl ON dl.id = b.delivery_location_id
		ORDER BY dl.name, b.building_name, r.room_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoomRow
	for rows.Next() {
		var r RoomRow
		if err := rows.Scan(&r.ID, &r.BuildingID, &r.BuildingName, &r.LocationName,
			&r.RoomName, &r.Capacity, &r.RoomType, &r.IsActive, &r.IsComputerLab); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRoom(ctx context.Context, buildingID int64, name, roomType string, capacity int, isActive, isComputerLab bool) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.rooms (building_id, room_name, room_type, capacity, is_active, is_computer_lab)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, buildingID, name, roomType, capacity, isActive, isComputerLab).Scan(&id)
	return id, err
}

func (s *Store) UpdateRoom(ctx context.Context, id, buildingID int64, name, roomType string, capacity int, isActive, isComputerLab bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.rooms SET
		    building_id    = $2,
		    room_name      = $3,
		    room_type      = $4,
		    capacity       = $5,
		    is_active      = $6,
		    is_computer_lab= $7
		WHERE id = $1
	`, id, buildingID, name, roomType, capacity, isActive, isComputerLab)
	return err
}

func (s *Store) DeleteRoom(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.rooms WHERE id=$1`, id)
	return err
}

func (s *Store) CreateDeliveryLocation(ctx context.Context, trainingOrgID int64, locID, name string, isVirtual bool, address, suburb, stateCode, postcode, lat, lng string) (int64, error) {
	var addrVal, suburbVal, stateVal, postcodeVal any
	if isVirtual {
		addrVal, suburbVal, stateVal, postcodeVal = nil, nil, nil, nil
	} else {
		addrVal, suburbVal, stateVal, postcodeVal = address, suburb, stateCode, postcode
	}
	var latVal, lngVal any
	if lat != "" {
		latVal = lat
	}
	if lng != "" {
		lngVal = lng
	}
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.delivery_locations
		    (training_org_id, delivery_loc_id, name, is_virtual, address, suburb, state_code, postcode, latitude, longitude)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9::text,'')::numeric, NULLIF($10::text,'')::numeric)
		RETURNING id
	`, trainingOrgID, locID, name, isVirtual, addrVal, suburbVal, stateVal, postcodeVal, latVal, lngVal).Scan(&id)
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

func (s *Store) UpdateIntakeGroup(ctx context.Context, id, intakeID int64, groupCode, groupName string, capacity *int, notes string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.intake_groups
		SET intake_id=$2, group_code=$3, group_name=$4, capacity=$5, notes=NULLIF($6,'')
		WHERE id=$1
	`, id, intakeID, groupCode, groupName, capacity, notes)
	return err
}

func (s *Store) DeleteIntakeGroup(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.intake_groups WHERE id=$1`, id)
	return err
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
		SELECT cs.id, c.class_code, cs.session_date, cs.start_time, cs.end_time,
		       cs.session_type, cs.cancelled, COALESCE(cs.notes,'')
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
		if err := rows.Scan(&r.ID, &r.ClassCode, &r.SessionDate, &r.StartTime, &r.EndTime,
			&r.SessionType, &r.Cancelled, &r.Notes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListTeachers(ctx context.Context) ([]TeacherListRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, sf.staff_number, p.family_name || ', ' || p.first_given_name
		FROM public.teachers t
		JOIN public.staff sf ON sf.id = t.id
		JOIN public.people p ON p.id = sf.id
		ORDER BY p.family_name, p.first_given_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeacherListRow
	for rows.Next() {
		var r TeacherListRow
		if err := rows.Scan(&r.ID, &r.StaffNumber, &r.FullName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSession(ctx context.Context, id int64, date, startTime, endTime, sessionType, notes string, roomID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.class_sessions
		SET session_date = $2::date,
		    start_time   = $3::time,
		    end_time     = $4::time,
		    session_type = $5,
		    notes        = NULLIF($6,''),
		    room_id      = NULLIF($7, 0)
		WHERE id = $1
	`, id, date, startTime, endTime, sessionType, notes, roomID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.class_sessions WHERE id = $1`, id)
	return err
}

func (s *Store) ScheduleForTeacher(ctx context.Context, teacherID int64, from, to string) ([]ScheduleSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cs.session_date, cs.start_time, cs.end_time, c.class_code,
		       COALESCE((
		           SELECT string_agg(subj.subject_code, ' · ' ORDER BY subj.subject_code)
		           FROM public.class_subjects _cs
		           JOIN public.subjects subj ON subj.id = _cs.subject_id
		           WHERE _cs.class_id = c.id
		       ),''),
		       cs.session_type
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		JOIN public.session_teachers st ON st.session_id = cs.id
		WHERE st.teacher_id = $1
		  AND cs.session_date >= $2::date
		  AND cs.session_date <= $3::date
		  AND NOT cs.cancelled
		ORDER BY cs.session_date, cs.start_time
	`, teacherID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleSession
	for rows.Next() {
		var r ScheduleSession
		if err := rows.Scan(&r.SessionDate, &r.StartTime, &r.EndTime,
			&r.ClassCode, &r.SubjectCodes, &r.SessionType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ScheduleForIntakeGroup(ctx context.Context, groupID int64, from, to string) ([]ScheduleSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT cs.session_date, cs.start_time, cs.end_time, c.class_code,
		       COALESCE((
		           SELECT string_agg(subj.subject_code, ' · ' ORDER BY subj.subject_code)
		           FROM public.class_subjects _cs
		           JOIN public.subjects subj ON subj.id = _cs.subject_id
		           WHERE _cs.class_id = c.id
		       ),''),
		       cs.session_type
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		WHERE c.intake_group_id = $1
		  AND cs.session_date >= $2::date
		  AND cs.session_date <= $3::date
		  AND NOT cs.cancelled
		ORDER BY cs.session_date, cs.start_time
	`, groupID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduleSession
	for rows.Next() {
		var r ScheduleSession
		if err := rows.Scan(&r.SessionDate, &r.StartTime, &r.EndTime,
			&r.ClassCode, &r.SubjectCodes, &r.SessionType); err != nil {
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

func (s *Store) UpdateFaculty(ctx context.Context, id int64, name string) error {
	_, err := s.pool.Exec(ctx, `UPDATE public.faculties SET faculty_name=$2 WHERE id=$1`, id, name)
	return err
}

func (s *Store) DeleteFaculty(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.faculties WHERE id=$1`, id)
	return err
}

type DepartmentRow struct {
	ID          int64
	Name        string
	FacultyID   pgtype.Int8
	FacultyName string
}

func (s *Store) ListDepartments(ctx context.Context) ([]DepartmentRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.dept_name, d.faculty_id, COALESCE(f.faculty_name, '')
		FROM public.departments d
		LEFT JOIN public.faculties f ON f.id = d.faculty_id
		WHERE d.deleted_at IS NULL
		ORDER BY d.dept_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DepartmentRow
	for rows.Next() {
		var r DepartmentRow
		if err := rows.Scan(&r.ID, &r.Name, &r.FacultyID, &r.FacultyName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateDepartment(ctx context.Context, name string, facultyID pgtype.Int8) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.departments (dept_name, faculty_id) VALUES ($1, $2) RETURNING id
	`, name, facultyID).Scan(&id)
	return id, err
}

func (s *Store) UpdateDepartment(ctx context.Context, id int64, name string, facultyID pgtype.Int8) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.departments SET dept_name=$2, faculty_id=$3 WHERE id=$1 AND deleted_at IS NULL
	`, id, name, facultyID)
	return err
}

func (s *Store) DeleteDepartment(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE public.departments SET deleted_at=NOW() WHERE id=$1`, id)
	return err
}

func (s *Store) UpdateSubject(ctx context.Context, id int64, subjectCode, subjectName, moduleFlag, fieldOfEducation string, nominalHours, creditPoints *int, vetFlag bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.subjects
		SET subject_code=$2, subject_name=$3, module_flag=$4, field_of_education=$5,
		    nominal_hours=$6, credit_points=$7, vet_flag=$8
		WHERE id=$1
	`, id, subjectCode, subjectName, moduleFlag, fieldOfEducation, nominalHours, creditPoints, vetFlag)
	return err
}

func (s *Store) DeleteSubject(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.subjects WHERE id=$1`, id)
	return err
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

func (s *Store) UpdateClass(ctx context.Context, id, academicPeriodID, deliveryLocationID int64, classCode string, intakeGroupID *int64, enrolmentCap *int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.classes
		SET class_code=$2, academic_period_id=$3, delivery_location_id=$4,
		    intake_group_id=$5, enrolment_cap=$6
		WHERE id=$1
	`, id, classCode, academicPeriodID, deliveryLocationID, intakeGroupID, enrolmentCap)
	return err
}

func (s *Store) DeleteClass(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM public.classes WHERE id=$1`, id)
	return err
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

// ── Backup / Export ───────────────────────────────────────────────────────────

func (s *Store) ListTableNames(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (s *Store) ExportTableRows(ctx context.Context, tableName string) ([]string, [][]any, error) {
	rows, err := s.pool.Query(ctx, "SELECT * FROM public."+tableName)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var cols []string
	for _, fd := range rows.FieldDescriptions() {
		cols = append(cols, fd.Name)
	}

	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, nil, err
		}
		out = append(out, vals)
	}
	return cols, out, rows.Err()
}

// ── VCC ───────────────────────────────────────────────────────────────────────

type VCCDocument struct {
	ID          int64
	Title       string
	Category    string
	Year        int
	URL         string
	FileName    string
	ExternalURL string
	UploadedAt  time.Time
}

type LibraryDocument struct {
	ID         int64
	Title      string
	Category   string
	Year       int
	FileName   string
	ObjectKey  string
	ExternalURL string
	UploadedAt time.Time
}

type VCCPQ struct {
	ID          int64
	Code        string
	Title       string
	Institution string
	AQFLevel    int // 0 = not set (NULL in DB)
	Status      string
	ApprovedAt  time.Time // zero means not set
	Notes       string
	Documents   []VCCDocument
}

type VCCUnitElement struct {
	ID            int64
	UnitID        int64
	Element       string
	Justification string
	SortOrder     int
	Documents     []VCCDocument
}

type VCCUnit struct {
	ID                  int64
	CourseID            int64
	UnitCode            string
	UnitTitle           string
	CompetencyMethod    string
	SupersededUnitCode  string
	SupersededUnitTitle string
	Description         string
	Status              string
	ApprovedAt          time.Time // zero means not set
	EnthusiasmRating    pgtype.Int2
	ConfidenceRating    pgtype.Int2
	SortOrder           int
	Elements            []VCCUnitElement
	Documents           []VCCDocument
}

type VCCCourse struct {
	ID          int64
	CourseCode  string
	CourseTitle string
	SortOrder   int
	Units       []VCCUnit
}

type VCCDetail struct {
	ID           int64
	TeacherID    int64
	TeacherName  string
	CalendarYear int
	VersionLabel string
	Status       string
	ApprovedAt   time.Time // zero means not set
	Notes        string
	PQs          []VCCPQ // Teaching qualifications
	VocQuals     []VCCPQ // Vocational qualifications
	Courses      []VCCCourse
}

func (s *Store) GetVCC(ctx context.Context, id int64) (*VCCDetail, error) {
	var v VCCDetail
	var approvedAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, `
		SELECT tv.id, tv.teacher_id, tv.calendar_year, tv.version_label, tv.status,
		       tv.approved_at, COALESCE(tv.notes, ''),
		       p.first_given_name || ' ' || p.family_name
		FROM teacher_vccs tv
		JOIN teachers t ON t.id = tv.teacher_id
		JOIN people p ON p.id = t.id
		WHERE tv.id = $1
	`, id).Scan(&v.ID, &v.TeacherID, &v.CalendarYear, &v.VersionLabel, &v.Status,
		&approvedAt, &v.Notes, &v.TeacherName)
	if err != nil {
		return nil, err
	}
	if approvedAt.Valid {
		v.ApprovedAt = approvedAt.Time
	}

	// Teaching qualifications
	pqRows, err := s.pool.Query(ctx, `
		SELECT id, qualification_code, qualification_title, COALESCE(institution,''),
		       COALESCE(aqf_level, 0), status, approved_at, COALESCE(notes,'')
		FROM teacher_vcc_professional_qualifications
		WHERE vcc_id = $1 ORDER BY id
	`, id)
	if err != nil {
		return nil, err
	}
	pqMap := map[int64]int{}
	for pqRows.Next() {
		var pq VCCPQ
		var at pgtype.Date
		if err := pqRows.Scan(&pq.ID, &pq.Code, &pq.Title, &pq.Institution, &pq.AQFLevel, &pq.Status, &at, &pq.Notes); err != nil {
			pqRows.Close()
			return nil, err
		}
		if at.Valid {
			pq.ApprovedAt = at.Time
		}
		pqMap[pq.ID] = len(v.PQs)
		v.PQs = append(v.PQs, pq)
	}
	if err := pqRows.Err(); err != nil {
		return nil, err
	}

	// Teaching qual documents
	if len(v.PQs) > 0 {
		pqIDs := make([]int64, len(v.PQs))
		for i, pq := range v.PQs {
			pqIDs[i] = pq.ID
		}
		dRows, err := s.pool.Query(ctx, `
			SELECT tdc.vcc_professional_qual_id, td.id, td.title, td.file_category,
			       COALESCE(td.year_of_document, 0), COALESCE(td.document_url, ''), COALESCE(td.external_url, '')
			FROM teacher_document_connections tdc
			JOIN teacher_documents td ON td.id = tdc.document_id
			WHERE tdc.vcc_professional_qual_id = ANY($1)
			ORDER BY tdc.vcc_professional_qual_id, td.id
		`, pqIDs)
		if err != nil {
			return nil, err
		}
		for dRows.Next() {
			var pqID int64
			var d VCCDocument
			if err := dRows.Scan(&pqID, &d.ID, &d.Title, &d.Category, &d.Year, &d.URL, &d.ExternalURL); err != nil {
				dRows.Close()
				return nil, err
			}
			if idx, ok := pqMap[pqID]; ok {
				v.PQs[idx].Documents = append(v.PQs[idx].Documents, d)
			}
		}
		if err := dRows.Err(); err != nil {
			return nil, err
		}
	}

	// Vocational qualifications
	vqRows, err := s.pool.Query(ctx, `
		SELECT id, qualification_code, qualification_title, COALESCE(institution,''),
		       COALESCE(aqf_level, 0), status, approved_at, COALESCE(notes,'')
		FROM teacher_vcc_vocational_qualifications
		WHERE vcc_id = $1 ORDER BY id
	`, id)
	if err != nil {
		return nil, err
	}
	vqMap := map[int64]int{}
	for vqRows.Next() {
		var vq VCCPQ
		var at pgtype.Date
		if err := vqRows.Scan(&vq.ID, &vq.Code, &vq.Title, &vq.Institution, &vq.AQFLevel, &vq.Status, &at, &vq.Notes); err != nil {
			vqRows.Close()
			return nil, err
		}
		if at.Valid {
			vq.ApprovedAt = at.Time
		}
		vqMap[vq.ID] = len(v.VocQuals)
		v.VocQuals = append(v.VocQuals, vq)
	}
	if err := vqRows.Err(); err != nil {
		return nil, err
	}

	// Vocational qual documents
	if len(v.VocQuals) > 0 {
		vqIDs := make([]int64, len(v.VocQuals))
		for i, vq := range v.VocQuals {
			vqIDs[i] = vq.ID
		}
		vdRows, err := s.pool.Query(ctx, `
			SELECT tdc.vcc_vocational_qual_id, td.id, td.title, td.file_category,
			       COALESCE(td.year_of_document, 0), COALESCE(td.document_url, ''), COALESCE(td.external_url, '')
			FROM teacher_document_connections tdc
			JOIN teacher_documents td ON td.id = tdc.document_id
			WHERE tdc.vcc_vocational_qual_id = ANY($1)
			ORDER BY tdc.vcc_vocational_qual_id, td.id
		`, vqIDs)
		if err != nil {
			return nil, err
		}
		for vdRows.Next() {
			var vqID int64
			var d VCCDocument
			if err := vdRows.Scan(&vqID, &d.ID, &d.Title, &d.Category, &d.Year, &d.URL, &d.ExternalURL); err != nil {
				vdRows.Close()
				return nil, err
			}
			if idx, ok := vqMap[vqID]; ok {
				v.VocQuals[idx].Documents = append(v.VocQuals[idx].Documents, d)
			}
		}
		if err := vdRows.Err(); err != nil {
			return nil, err
		}
	}

	// Courses
	cRows, err := s.pool.Query(ctx, `
		SELECT id, course_code, course_title, sort_order
		FROM teacher_vcc_courses WHERE vcc_id = $1 ORDER BY sort_order
	`, id)
	if err != nil {
		return nil, err
	}
	courseMap := map[int64]int{} // id → index in v.Courses
	for cRows.Next() {
		var c VCCCourse
		if err := cRows.Scan(&c.ID, &c.CourseCode, &c.CourseTitle, &c.SortOrder); err != nil {
			cRows.Close()
			return nil, err
		}
		courseMap[c.ID] = len(v.Courses)
		v.Courses = append(v.Courses, c)
	}
	if err := cRows.Err(); err != nil {
		return nil, err
	}

	// Units
	uRows, err := s.pool.Query(ctx, `
		SELECT id, COALESCE(vcc_course_id, 0), unit_code, unit_title, competency_method,
		       COALESCE(superseded_unit_code,''), COALESCE(superseded_unit_title,''),
		       COALESCE(description,''), status, approved_at,
		       enthusiasm_rating, confidence_rating, sort_order
		FROM teacher_vcc_units
		WHERE vcc_id = $1
		ORDER BY COALESCE(vcc_course_id, 0), sort_order
	`, id)
	if err != nil {
		return nil, err
	}
	unitMap := map[int64][2]int{} // unit id → [courseIdx, unitIdx]
	for uRows.Next() {
		var u VCCUnit
		var at pgtype.Date
		if err := uRows.Scan(&u.ID, &u.CourseID, &u.UnitCode, &u.UnitTitle, &u.CompetencyMethod,
			&u.SupersededUnitCode, &u.SupersededUnitTitle, &u.Description, &u.Status, &at,
			&u.EnthusiasmRating, &u.ConfidenceRating, &u.SortOrder); err != nil {
			uRows.Close()
			return nil, err
		}
		if at.Valid {
			u.ApprovedAt = at.Time
		}
		if cIdx, ok := courseMap[u.CourseID]; ok {
			uIdx := len(v.Courses[cIdx].Units)
			v.Courses[cIdx].Units = append(v.Courses[cIdx].Units, u)
			unitMap[u.ID] = [2]int{cIdx, uIdx}
		}
	}
	if err := uRows.Err(); err != nil {
		return nil, err
	}

	// Unit documents
	if len(unitMap) > 0 {
		unitIDs := make([]int64, 0, len(unitMap))
		for uid := range unitMap {
			unitIDs = append(unitIDs, uid)
		}
		udRows, err := s.pool.Query(ctx, `
			SELECT tdc.vcc_unit_id, td.id, td.title, td.file_category,
			       COALESCE(td.year_of_document, 0), COALESCE(td.document_url, ''), COALESCE(td.external_url, '')
			FROM teacher_document_connections tdc
			JOIN teacher_documents td ON td.id = tdc.document_id
			WHERE tdc.vcc_unit_id = ANY($1)
			ORDER BY tdc.vcc_unit_id, td.id
		`, unitIDs)
		if err != nil {
			return nil, err
		}
		for udRows.Next() {
			var unitID int64
			var d VCCDocument
			if err := udRows.Scan(&unitID, &d.ID, &d.Title, &d.Category, &d.Year, &d.URL, &d.ExternalURL); err != nil {
				udRows.Close()
				return nil, err
			}
			if pos, ok := unitMap[unitID]; ok {
				v.Courses[pos[0]].Units[pos[1]].Documents = append(v.Courses[pos[0]].Units[pos[1]].Documents, d)
			}
		}
		if err := udRows.Err(); err != nil {
			return nil, err
		}

		// Elements
		eRows, err := s.pool.Query(ctx, `
			SELECT id, vcc_unit_id, element, COALESCE(justification,''), sort_order
			FROM teacher_vcc_unit_elements
			WHERE vcc_unit_id = ANY($1)
			ORDER BY vcc_unit_id, sort_order
		`, unitIDs)
		if err != nil {
			return nil, err
		}
		elementMap := map[int64][3]int{} // element id → [courseIdx, unitIdx, elemIdx]
		for eRows.Next() {
			var e VCCUnitElement
			if err := eRows.Scan(&e.ID, &e.UnitID, &e.Element, &e.Justification, &e.SortOrder); err != nil {
				eRows.Close()
				return nil, err
			}
			if pos, ok := unitMap[e.UnitID]; ok {
				eIdx := len(v.Courses[pos[0]].Units[pos[1]].Elements)
				v.Courses[pos[0]].Units[pos[1]].Elements = append(v.Courses[pos[0]].Units[pos[1]].Elements, e)
				elementMap[e.ID] = [3]int{pos[0], pos[1], eIdx}
			}
		}
		if err := eRows.Err(); err != nil {
			return nil, err
		}

		// Element documents
		if len(elementMap) > 0 {
			elemIDs := make([]int64, 0, len(elementMap))
			for eid := range elementMap {
				elemIDs = append(elemIDs, eid)
			}
			edRows, err := s.pool.Query(ctx, `
				SELECT tdc.vcc_unit_element_id, td.id, td.title, td.file_category,
				       COALESCE(td.year_of_document, 0), COALESCE(td.document_url, ''), COALESCE(td.external_url, '')
				FROM teacher_document_connections tdc
				JOIN teacher_documents td ON td.id = tdc.document_id
				WHERE tdc.vcc_unit_element_id = ANY($1)
				ORDER BY tdc.vcc_unit_element_id, td.id
			`, elemIDs)
			if err != nil {
				return nil, err
			}
			for edRows.Next() {
				var elemID int64
				var d VCCDocument
				if err := edRows.Scan(&elemID, &d.ID, &d.Title, &d.Category, &d.Year, &d.URL, &d.ExternalURL); err != nil {
					edRows.Close()
					return nil, err
				}
				if pos, ok := elementMap[elemID]; ok {
					v.Courses[pos[0]].Units[pos[1]].Elements[pos[2]].Documents = append(
						v.Courses[pos[0]].Units[pos[1]].Elements[pos[2]].Documents, d)
				}
			}
			if err := edRows.Err(); err != nil {
				return nil, err
			}
		}
	}

	return &v, nil
}

func (s *Store) CreateVCCUnitElement(ctx context.Context, teacherID, unitID int64, element, justification string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_vcc_unit_elements (vcc_unit_id, element, justification)
		SELECT $1, $2, NULLIF($3,'')
		WHERE EXISTS (
			SELECT 1 FROM teacher_vcc_units u
			JOIN teacher_vccs v ON v.id = u.vcc_id
			WHERE u.id = $1 AND v.teacher_id = $4
		)
		RETURNING id
	`, unitID, element, justification, teacherID).Scan(&id)
	return id, err
}

func (s *Store) UpdateVCCUnitElement(ctx context.Context, teacherID, elemID int64, element, justification string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vcc_unit_elements e
		SET element = $2, justification = NULLIF($3,'')
		WHERE e.id = $1
		  AND EXISTS (
			SELECT 1 FROM teacher_vcc_units u
			JOIN teacher_vccs v ON v.id = u.vcc_id
			WHERE u.id = e.vcc_unit_id AND v.teacher_id = $4
		  )
	`, elemID, element, justification, teacherID)
	return err
}

func (s *Store) DeleteVCCUnitElement(ctx context.Context, teacherID, elemID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM teacher_vcc_unit_elements e
		WHERE e.id = $1
		  AND EXISTS (
			SELECT 1 FROM teacher_vcc_units u
			JOIN teacher_vccs v ON v.id = u.vcc_id
			WHERE u.id = e.vcc_unit_id AND v.teacher_id = $2
		  )
	`, elemID, teacherID)
	return err
}

func (s *Store) GetLatestVCCForTeacher(ctx context.Context, teacherID int64) (*VCCDetail, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM teacher_vccs WHERE teacher_id = $1
		ORDER BY calendar_year DESC, version DESC LIMIT 1
	`, teacherID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetVCC(ctx, id)
}

func (s *Store) UpdateVCCStatus(ctx context.Context, teacherID int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vccs SET status = $1,
			approved_at = CASE WHEN $1 = 'Approved' THEN NOW() ELSE approved_at END
		WHERE teacher_id = $2
	`, status, teacherID)
	return err
}

func (s *Store) UpdateVCCUnit(ctx context.Context, teacherID, unitID int64,
	code, title, method, supersededCode, supersededTitle, description, status string,
	approvedAt pgtype.Date,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vcc_units
		SET unit_code             = $1,
		    unit_title            = $2,
		    competency_method     = $3,
		    superseded_unit_code  = NULLIF($4, ''),
		    superseded_unit_title = NULLIF($5, ''),
		    description           = NULLIF($6, ''),
		    status                = $7,
		    approved_at           = $8
		WHERE id = $9
		  AND vcc_id IN (SELECT id FROM teacher_vccs WHERE teacher_id = $10)
	`, code, title, method, supersededCode, supersededTitle,
		description, status, approvedAt, unitID, teacherID)
	return err
}

func (s *Store) UpdateVCCUnitRatings(ctx context.Context, teacherID, unitID int64, enthusiasm, confidence pgtype.Int2) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vcc_units
		SET enthusiasm_rating = $1,
		    confidence_rating = $2
		WHERE id = $3
		  AND vcc_id IN (SELECT id FROM teacher_vccs WHERE teacher_id = $4)
	`, enthusiasm, confidence, unitID, teacherID)
	return err
}

func (s *Store) UpdateVCCPQ(ctx context.Context, teacherID, pqID int64,
	code, title, institution, status string, approvedAt pgtype.Date, notes string, aqfLevel int,
) error {
	var aqf pgtype.Int2
	if aqfLevel > 0 {
		aqf = pgtype.Int2{Int16: int16(aqfLevel), Valid: true}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vcc_professional_qualifications
		SET qualification_code  = $1,
		    qualification_title = $2,
		    institution         = NULLIF($3, ''),
		    status              = $4,
		    approved_at         = $5,
		    notes               = NULLIF($6, ''),
		    aqf_level           = $9
		WHERE id = $7
		  AND vcc_id IN (SELECT id FROM teacher_vccs WHERE teacher_id = $8)
	`, code, title, institution, status, approvedAt, notes, pqID, teacherID, aqf)
	return err
}

func (s *Store) CreateVCCPQ(ctx context.Context, teacherID int64, code, title, institution string) (VCCPQ, error) {
	var pq VCCPQ
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_vcc_professional_qualifications
		       (vcc_id, qualification_code, qualification_title, institution, status)
		SELECT id, $2, $3, NULLIF($4,''), 'Draft'
		FROM teacher_vccs WHERE teacher_id = $1
		ORDER BY calendar_year DESC, version DESC LIMIT 1
		RETURNING id, qualification_code, qualification_title, COALESCE(institution,''), status
	`, teacherID, code, title, institution).Scan(&pq.ID, &pq.Code, &pq.Title, &pq.Institution, &pq.Status)
	return pq, err
}

func (s *Store) CreateVCCVocQual(ctx context.Context, teacherID int64, code, title, institution string) (VCCPQ, error) {
	var vq VCCPQ
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_vcc_vocational_qualifications
		       (vcc_id, qualification_code, qualification_title, institution, status)
		SELECT id, $2, $3, NULLIF($4,''), 'Draft'
		FROM teacher_vccs WHERE teacher_id = $1
		ORDER BY calendar_year DESC, version DESC LIMIT 1
		RETURNING id, qualification_code, qualification_title, COALESCE(institution,''), status
	`, teacherID, code, title, institution).Scan(&vq.ID, &vq.Code, &vq.Title, &vq.Institution, &vq.Status)
	return vq, err
}

func (s *Store) UpdateVCCVocQual(ctx context.Context, teacherID, vqID int64,
	code, title, institution, status string, approvedAt pgtype.Date, notes string, aqfLevel int,
) error {
	var aqf pgtype.Int2
	if aqfLevel > 0 {
		aqf = pgtype.Int2{Int16: int16(aqfLevel), Valid: true}
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE teacher_vcc_vocational_qualifications
		SET qualification_code  = $1,
		    qualification_title = $2,
		    institution         = NULLIF($3, ''),
		    status              = $4,
		    approved_at         = $5,
		    notes               = NULLIF($6, ''),
		    aqf_level           = $9
		WHERE id = $7
		  AND vcc_id IN (SELECT id FROM teacher_vccs WHERE teacher_id = $8)
	`, code, title, institution, status, approvedAt, notes, vqID, teacherID, aqf)
	return err
}

func (s *Store) CreateVocQualDocument(ctx context.Context, teacherID, vqID int64, title, externalURL string) (VCCDocument, error) {
	var d VCCDocument
	d.Title = title
	d.ExternalURL = externalURL
	d.Category = "Other"
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_documents (teacher_id, title, file_category, external_url)
		SELECT $1, $2, 'Other', NULLIF($3,'')
		WHERE EXISTS (
			SELECT 1 FROM teacher_vcc_vocational_qualifications vq
			JOIN teacher_vccs tv ON tv.id = vq.vcc_id
			WHERE vq.id = $4 AND tv.teacher_id = $1
		)
		RETURNING id
	`, teacherID, title, externalURL, vqID).Scan(&d.ID)
	if err != nil {
		return VCCDocument{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO teacher_document_connections (document_id, vcc_vocational_qual_id)
		VALUES ($1, $2)
	`, d.ID, vqID)
	return d, err
}

func (s *Store) CreatePQDocument(ctx context.Context, teacherID, pqID int64, title, externalURL string) (VCCDocument, error) {
	var d VCCDocument
	d.Title = title
	d.ExternalURL = externalURL
	d.Category = "Other"
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_documents (teacher_id, title, file_category, external_url)
		SELECT $1, $2, 'Other', NULLIF($3,'')
		WHERE EXISTS (
			SELECT 1 FROM teacher_vcc_professional_qualifications pq
			JOIN teacher_vccs tv ON tv.id = pq.vcc_id
			WHERE pq.id = $4 AND tv.teacher_id = $1
		)
		RETURNING id
	`, teacherID, title, externalURL, pqID).Scan(&d.ID)
	if err != nil {
		return VCCDocument{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO teacher_document_connections (document_id, vcc_professional_qual_id)
		VALUES ($1, $2)
	`, d.ID, pqID)
	return d, err
}

func (s *Store) DeletePQDocument(ctx context.Context, teacherID, docID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM teacher_documents WHERE id = $1 AND teacher_id = $2
	`, docID, teacherID)
	return err
}

func (s *Store) CreateElementDocument(ctx context.Context, teacherID, elementID int64, title, externalURL string) (VCCDocument, error) {
	var d VCCDocument
	d.Title = title
	d.ExternalURL = externalURL
	d.Category = "Other"
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_documents (teacher_id, title, file_category, external_url)
		SELECT $1, $2, 'Other', NULLIF($3,'')
		WHERE EXISTS (
			SELECT 1 FROM teacher_vcc_unit_elements el
			JOIN teacher_vcc_units u ON u.id = el.vcc_unit_id
			JOIN teacher_vccs tv ON tv.id = u.vcc_id
			WHERE el.id = $4 AND tv.teacher_id = $1
		)
		RETURNING id
	`, teacherID, title, externalURL, elementID).Scan(&d.ID)
	if err != nil {
		return VCCDocument{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO teacher_document_connections (document_id, vcc_unit_element_id)
		VALUES ($1, $2)
	`, d.ID, elementID)
	return d, err
}

func (s *Store) DeleteElementDocument(ctx context.Context, teacherID, docID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM teacher_documents WHERE id = $1 AND teacher_id = $2
	`, docID, teacherID)
	return err
}

func (s *Store) CreateLibraryDocument(ctx context.Context, teacherID int64, title, category string, year int, fileName, objectKey, externalURL string) (LibraryDocument, error) {
	var d LibraryDocument
	err := s.pool.QueryRow(ctx, `
		INSERT INTO teacher_documents
		    (teacher_id, title, file_category, year_of_document,
		     file_name, document_url, external_url)
		VALUES ($1, $2, $3, NULLIF($4,0), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''))
		RETURNING id, title, file_category,
		          COALESCE(year_of_document,0),
		          COALESCE(file_name,''), COALESCE(document_url,''),
		          COALESCE(external_url,''), uploaded_at
	`, teacherID, title, category, year, fileName, objectKey, externalURL).Scan(
		&d.ID, &d.Title, &d.Category, &d.Year,
		&d.FileName, &d.ObjectKey, &d.ExternalURL, &d.UploadedAt,
	)
	return d, err
}

func (s *Store) ListTeacherDocuments(ctx context.Context, teacherID int64) ([]LibraryDocument, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, file_category,
		       COALESCE(year_of_document,0),
		       COALESCE(file_name,''), COALESCE(document_url,''),
		       COALESCE(external_url,''), uploaded_at
		FROM teacher_documents
		WHERE teacher_id = $1
		ORDER BY file_category, uploaded_at DESC
	`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LibraryDocument
	for rows.Next() {
		var d LibraryDocument
		if err := rows.Scan(&d.ID, &d.Title, &d.Category, &d.Year,
			&d.FileName, &d.ObjectKey, &d.ExternalURL, &d.UploadedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetTeacherDocumentObjectKey returns the MinIO object key for a document the
// teacher owns, so the caller can delete it from object storage before removing
// the DB row.
func (s *Store) GetTeacherDocumentObjectKey(ctx context.Context, teacherID, docID int64) (string, error) {
	var key string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(document_url,'')
		FROM teacher_documents
		WHERE id = $1 AND teacher_id = $2
	`, docID, teacherID).Scan(&key)
	return key, err
}

func (s *Store) DeleteTeacherDocument(ctx context.Context, teacherID, docID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM teacher_documents WHERE id = $1 AND teacher_id = $2
	`, docID, teacherID)
	return err
}

// ── System Settings ───────────────────────────────────────────────────────────

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var val string
	err := s.pool.QueryRow(ctx, `
		SELECT value FROM public.system_settings WHERE key = $1
	`, key).Scan(&val)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.system_settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, value)
	return err
}

// ── Course Enrollments ────────────────────────────────────────────────────────

type StudentSelectRow struct {
	ID            int64  `json:"id"`
	StudentNumber string `json:"studentNumber"`
	FullName      string `json:"fullName"`
}

type EnrollmentRow struct {
	ID                     int64
	StudentID              int64
	StudentNumber          string
	StudentName            string
	ProgramID              int64
	ProgramCode            string
	ProgramName            string
	IntakeGroupID          int64
	IntakeGroupCode        string
	EnrollmentStatus       string
	CommencementDate       string
	CompletionDate         string
	FundingStateCode       string
	CommencingProgramID    string
	TrainingContractID     string
	ClientApprenticeshipID string
}

func (s *Store) SearchStudents(ctx context.Context, q string) ([]StudentSelectRow, error) {
	like := "%" + q + "%"
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.student_number, p.family_name || ', ' || p.first_given_name
		FROM public.students s
		JOIN public.people p ON p.id = s.id
		WHERE s.deleted_at IS NULL
		  AND (p.family_name ILIKE $1 OR p.first_given_name ILIKE $1
		       OR (p.family_name || ' ' || p.first_given_name) ILIKE $1
		       OR (p.first_given_name || ' ' || p.family_name) ILIKE $1
		       OR s.student_number ILIKE $1)
		ORDER BY p.family_name, p.first_given_name
		LIMIT 20
	`, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudentSelectRow
	for rows.Next() {
		var r StudentSelectRow
		if err := rows.Scan(&r.ID, &r.StudentNumber, &r.FullName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListStudentsForSelect(ctx context.Context) ([]StudentSelectRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.student_number, p.family_name || ', ' || p.first_given_name
		FROM public.students s
		JOIN public.people p ON p.id = s.id
		WHERE s.deleted_at IS NULL
		ORDER BY p.family_name, p.first_given_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudentSelectRow
	for rows.Next() {
		var r StudentSelectRow
		if err := rows.Scan(&r.ID, &r.StudentNumber, &r.FullName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListEnrollments(ctx context.Context) ([]EnrollmentRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT sce.id,
		       sce.student_id,
		       s.student_number,
		       p.family_name || ', ' || p.first_given_name,
		       sce.program_id,
		       pr.program_code,
		       pr.program_name,
		       COALESCE(sce.intake_group_id, 0),
		       COALESCE(ig.group_code, ''),
		       sce.enrollment_status,
		       TO_CHAR(sce.commencement_date, 'YYYY-MM-DD'),
		       COALESCE(TO_CHAR(sce.completion_date, 'YYYY-MM-DD'), ''),
		       sce.funding_state_code,
		       sce.commencing_program_id,
		       COALESCE(sce.training_contract_id, ''),
		       COALESCE(sce.client_apprenticeship_id, '')
		FROM public.student_course_enrollments sce
		JOIN public.students s ON s.id = sce.student_id
		JOIN public.people p ON p.id = sce.student_id
		JOIN public.programs pr ON pr.id = sce.program_id
		LEFT JOIN public.intake_groups ig ON ig.id = sce.intake_group_id
		WHERE sce.deleted_at IS NULL
		ORDER BY p.family_name, p.first_given_name, pr.program_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnrollmentRow
	for rows.Next() {
		var r EnrollmentRow
		if err := rows.Scan(
			&r.ID, &r.StudentID, &r.StudentNumber, &r.StudentName,
			&r.ProgramID, &r.ProgramCode, &r.ProgramName,
			&r.IntakeGroupID, &r.IntakeGroupCode,
			&r.EnrollmentStatus, &r.CommencementDate, &r.CompletionDate,
			&r.FundingStateCode, &r.CommencingProgramID,
			&r.TrainingContractID, &r.ClientApprenticeshipID,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateEnrollment(ctx context.Context,
	studentID, programID, intakeGroupID int64,
	status, commencementDate, completionDate, fundingStateCode, commencingProgramID string,
) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO public.student_course_enrollments
		    (student_id, program_id, intake_group_id, enrollment_status,
		     commencement_date, completion_date, funding_state_code, commencing_program_id)
		VALUES ($1, $2, NULLIF($3, 0), $4, $5::date, NULLIF($6, '')::date, $7, $8)
		RETURNING id
	`, studentID, programID, intakeGroupID, status,
		commencementDate, completionDate, fundingStateCode, commencingProgramID,
	).Scan(&id)
	return id, err
}

func (s *Store) UpdateEnrollment(ctx context.Context,
	id, intakeGroupID int64,
	status, commencementDate, completionDate, fundingStateCode, commencingProgramID,
	trainingContractID, clientApprenticeshipID string,
) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.student_course_enrollments SET
		    intake_group_id          = NULLIF($2, 0),
		    enrollment_status        = $3,
		    commencement_date        = $4::date,
		    completion_date          = NULLIF($5, '')::date,
		    funding_state_code       = $6,
		    commencing_program_id    = $7,
		    training_contract_id     = NULLIF($8, ''),
		    client_apprenticeship_id = NULLIF($9, '')
		WHERE id = $1 AND deleted_at IS NULL
	`, id, intakeGroupID, status, commencementDate, completionDate,
		fundingStateCode, commencingProgramID, trainingContractID, clientApprenticeshipID,
	)
	return err
}

func (s *Store) DeleteEnrollment(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.student_course_enrollments SET deleted_at = NOW() WHERE id = $1
	`, id)
	return err
}


// ── Teacher availability ──────────────────────────────────────────────────

type TeacherAvailability struct {
	Day       int    // 1=Mon, 2=Tue, 3=Wed, 4=Thu, 5=Fri, 6=Sat
	StartTime string // "08:00"
	EndTime   string // "22:00"
	Notes     string
}

func (s *Store) GetTeacherAvailability(ctx context.Context, teacherID int64) ([]TeacherAvailability, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT day_of_week,
		       TO_CHAR(start_time, 'HH24:MI'),
		       TO_CHAR(end_time,   'HH24:MI'),
		       COALESCE(notes, '')
		FROM public.teacher_availability
		WHERE teacher_id = $1
		ORDER BY day_of_week
	`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeacherAvailability
	for rows.Next() {
		var a TeacherAvailability
		if err := rows.Scan(&a.Day, &a.StartTime, &a.EndTime, &a.Notes); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) UpsertTeacherAvailability(ctx context.Context, teacherID int64, day int, startTime, endTime, notes string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO public.teacher_availability (teacher_id, day_of_week, start_time, end_time, notes)
		VALUES ($1, $2, $3::time, $4::time, NULLIF($5, ''))
		ON CONFLICT (teacher_id, day_of_week) DO UPDATE
		    SET start_time = EXCLUDED.start_time,
		        end_time   = EXCLUDED.end_time,
		        notes      = EXCLUDED.notes
	`, teacherID, day, startTime, endTime, notes)
	return err
}

func (s *Store) DeleteTeacherAvailability(ctx context.Context, teacherID int64, day int) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM public.teacher_availability
		WHERE teacher_id = $1 AND day_of_week = $2
	`, teacherID, day)
	return err
}

func (s *Store) GetTeacherEmploymentStatus(ctx context.Context, teacherID int64) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(sf.employment_status::text, '')
		FROM public.teachers t
		JOIN public.staff sf ON sf.id = t.id
		WHERE t.id = $1
	`, teacherID).Scan(&status)
	return status, err
}

// ── Leave requests ────────────────────────────────────────────────────────

type LeaveRequest struct {
	ID            int64
	LeaveType     string
	IsPartialDay  bool
	PartialStart  string // "HH:MM" or ""
	PartialEnd    string
	Notes         string
	Status        string
	ApproverNotes string
	CreatedAt     time.Time
	Dates         []string // "YYYY-MM-DD" sorted
}

func (s *Store) CreateLeaveRequest(
	ctx context.Context,
	teacherID int64,
	leaveType string,
	isPartialDay bool,
	partialStart, partialEnd string,
	notes string,
	dates []string,
) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	var ps, pe interface{}
	if isPartialDay && partialStart != "" && partialEnd != "" {
		ps, pe = partialStart, partialEnd
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO public.leave_requests
		    (teacher_id, leave_type, is_partial_day, partial_start, partial_end, notes)
		VALUES ($1, $2, $3, $4::time, $5::time, NULLIF($6,''))
		RETURNING id
	`, teacherID, leaveType, isPartialDay, ps, pe, notes).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, d := range dates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO public.leave_request_dates (request_id, leave_date)
			VALUES ($1, $2::date)
			ON CONFLICT DO NOTHING
		`, id, d); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

func (s *Store) ListLeaveRequests(ctx context.Context, teacherID int64) ([]LeaveRequest, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT lr.id, lr.leave_type, lr.is_partial_day,
		       COALESCE(TO_CHAR(lr.partial_start,'HH24:MI'),''),
		       COALESCE(TO_CHAR(lr.partial_end,  'HH24:MI'),''),
		       COALESCE(lr.notes,''), lr.status,
		       COALESCE(lr.approver_notes,''),
		       lr.created_at,
		       ARRAY_AGG(lrd.leave_date::text ORDER BY lrd.leave_date) AS dates
		FROM public.leave_requests lr
		JOIN public.leave_request_dates lrd ON lrd.request_id = lr.id
		WHERE lr.teacher_id = $1
		GROUP BY lr.id
		ORDER BY MIN(lrd.leave_date) DESC, lr.created_at DESC
	`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaveRequest
	for rows.Next() {
		var r LeaveRequest
		var dates pgtype.Array[string]
		if err := rows.Scan(
			&r.ID, &r.LeaveType, &r.IsPartialDay,
			&r.PartialStart, &r.PartialEnd,
			&r.Notes, &r.Status, &r.ApproverNotes,
			&r.CreatedAt, &dates,
		); err != nil {
			return nil, err
		}
		r.Dates = dates.Elements
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CancelLeaveRequest(ctx context.Context, teacherID, requestID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.leave_requests SET status = 'Cancelled'
		WHERE id = $1 AND teacher_id = $2 AND status = 'Pending'
	`, requestID, teacherID)
	return err
}

func (s *Store) ApproveLeaveRequest(ctx context.Context, approverID, requestID int64, notes string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.leave_requests
		SET status = 'Approved', approved_by = $1, approved_at = NOW(),
		    approver_notes = NULLIF($2,'')
		WHERE id = $3 AND status = 'Pending'
	`, approverID, notes, requestID)
	return err
}

func (s *Store) DeclineLeaveRequest(ctx context.Context, approverID, requestID int64, notes string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE public.leave_requests
		SET status = 'Declined', approved_by = $1, approved_at = NOW(),
		    approver_notes = NULLIF($2,'')
		WHERE id = $3 AND status = 'Pending'
	`, approverID, notes, requestID)
	return err
}
