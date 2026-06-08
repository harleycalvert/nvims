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

type Class struct {
	ID        int64
	ClassCode string
}

type Session struct {
	ID          int64
	ClassID     int64
	ClassCode   string
	SessionDate time.Time
}

type AttendanceRow struct {
	StudentID  int64
	FirstName  string
	LastName   string
	Attendance map[int64]string // session_id -> status ("" means not recorded)
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

func (s *Store) ClassesForPeriod(ctx context.Context, periodID int64) ([]Class, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, class_code
		FROM public.classes
		WHERE academic_period_id = $1
		ORDER BY class_code
	`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Class
	for rows.Next() {
		var c Class
		if err := rows.Scan(&c.ID, &c.ClassCode); err != nil {
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
		SELECT cs.id, cs.class_id, c.class_code, cs.session_date
		FROM public.class_sessions cs
		JOIN public.classes c ON c.id = cs.class_id
		WHERE cs.class_id = ANY($1) AND NOT cs.cancelled
		ORDER BY cs.session_date, cs.class_id
	`, classIDs)
	if err != nil {
		return nil, nil, err
	}
	defer sessRows.Close()

	var sessions []Session
	for sessRows.Next() {
		var ss Session
		if err := sessRows.Scan(&ss.ID, &ss.ClassID, &ss.ClassCode, &ss.SessionDate); err != nil {
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
		SELECT DISTINCT s.id, p.first_given_name, p.family_name
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
		if err := studentRows.Scan(&r.StudentID, &r.FirstName, &r.LastName); err != nil {
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
