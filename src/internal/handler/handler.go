package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"nvims/internal/auth"
	"nvims/internal/storage"
	"nvims/internal/store"
)

type Handler struct {
	store    *store.Store
	sessions *auth.Sessions
	storage  *storage.Client
	tmpl     *template.Template
	mu       sync.RWMutex
	lmsName  string
	lmsURL   string
}

func New(st *store.Store, sessions *auth.Sessions, stor *storage.Client) *Handler {
	funcs := template.FuncMap{
		"dateShort": func(t time.Time) string { return t.Format("2 Jan") },
		"dateFull":  func(t time.Time) string { return t.Format("Mon 2 Jan 2006") },
		"timeShort": func(t time.Time) string { return t.Format("15:04") },
		"statusAbbr": func(s string) string {
			switch s {
			case "Present":
				return "P"
			case "Online":
				return "O"
			case "Absent-Notified":
				return "AN"
			case "Absent-Unnotified":
				return "A"
			case "Excused":
				return "E"
			case "":
				return "–"
			default:
				return string([]rune(s)[:1])
			}
		},
		"statusCSS": func(s string) string {
			switch s {
			case "Present":
				return "att-present"
			case "Online":
				return "att-online"
			case "Absent-Notified":
				return "att-notified"
			case "Absent-Unnotified":
				return "att-absent"
			case "Excused":
				return "att-excused"
			case "":
				return "att-none"
			case "Approved":
				return "approved"
			case "Draft":
				return "draft"
			case "Submitted":
				return "submitted"
			case "Pending":
				return "pending"
			case "Rejected":
				return "rejected"
			default:
				return "att-absent"
			}
		},
	}

	funcs["attCell"] = func(sessionID, studentID int64, status string) store.AttendanceCell {
		return store.AttendanceCell{SessionID: sessionID, StudentID: studentID, Status: status}
	}
	funcs["resultCSS"] = func(result string, published bool) string {
		switch result {
		case "SC":
			if published {
				return "res-sc-pub"
			}
			return "res-sc-unpub"
		case "NS":
			if published {
				return "res-ns-pub"
			}
			return "res-ns-unpub"
		case "IP":
			return "res-ip"
		}
		return "res-none"
	}
	funcs["resultAbbr"] = func(result string) string {
		if result == "" {
			return "–"
		}
		return result
	}

	funcs["dateISO"] = func(t time.Time) string { return t.Format("2006-01-02") }
	funcs["dateDMY"] = func(t time.Time) string { return t.Format("02/01/2006") }
	funcs["dateDayName"] = func(t time.Time) string { return t.Format("Mon") }
	funcs["sub"] = func(a, b int) int { return a - b }
	funcs["sessionTypeClass"] = func(t string) string {
		switch t {
		case "Assessment":  return "sess-assessment"
		case "Online":      return "sess-online"
		case "Replacement": return "sess-replacement"
		case "Other":       return "sess-other"
		default:            return "sess-scheduled"
		}
	}

	tmpl := template.Must(
		template.New("").Funcs(funcs).ParseFiles(
			"templates/home.html",
			"templates/register.html",
			"templates/login.html",
			"templates/partials/programs.html",
			"templates/partials/groups.html",
			"templates/partials/classes.html",
			"templates/partials/attendance.html",
			"templates/partials/results.html",
			"templates/admin/menu.html",
			"templates/admin/people.html",
			"templates/admin/person.html",
			"templates/admin/role_form.html",
			"templates/admin/programs.html",
			"templates/admin/classes.html",
			"templates/admin/faculties.html",
			"templates/admin/subjects.html",
			"templates/timetable.html",
			"templates/admin/periods.html",
			"templates/admin/locations.html",
			"templates/admin/intake-groups.html",
			"templates/admin/sessions.html",
			"templates/backup.html",
			"templates/partials/sidebar.html",
			"templates/partials/admin-nav.html",
			"templates/workplan/menu.html",
			"templates/workplan/availability.html",
			"templates/vcc/menu.html",
			"templates/vcc/document-library.html",
			"templates/vcc/vocational-evidence.html",
			"templates/vcc/detail.html",
			"templates/vcc/vocational-qualifications.html",
			"templates/admin/infra-menu.html",
			"templates/admin/infra-orgs.html",
			"templates/admin/infra-locations.html",
			"templates/admin/infra-buildings.html",
			"templates/admin/infra-rooms.html",
			"templates/admin/enrollments.html",
			"templates/system/lms.html",
			"templates/assessment/menu.html",
			"templates/system/menu.html",
			"templates/admin/departments.html",
		),
	)
	h := &Handler{store: st, sessions: sessions, storage: stor, tmpl: tmpl}
	// pre-load LMS config from DB
	ctx := context.Background()
	h.lmsName, _ = st.GetSetting(ctx, "lms.name")
	h.lmsURL, _  = st.GetSetting(ctx, "lms.url")
	return h
}

func (h *Handler) Menu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "home", map[string]any{"User": user})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	periods, err := h.store.Periods(r.Context())
	if err != nil {
		log.Printf("Periods: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "register", map[string]any{"Periods": periods, "User": user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", map[string]any{"Error": "", "Username": ""})
}

func (h *Handler) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	u, err := h.store.GetUserByUsername(r.Context(), username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		h.render(w, "login", map[string]any{
			"Error":    "Invalid username or password.",
			"Username": username,
		})
		return
	}

	h.store.UpdateLastLogin(r.Context(), u.ID)
	h.sessions.Create(w, auth.User{ID: u.ID, PersonID: u.PersonID, Username: u.Username, FullName: u.FullName, Role: u.Role})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.Delete(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) Programs(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.URL.Query().Get("period_id"), 10, 64)
	if err != nil || periodID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a period to see its programs.</p>`))
		return
	}

	programs, err := h.store.ProgramsForPeriod(r.Context(), periodID)
	if err != nil {
		log.Printf("ProgramsForPeriod(%d): %v", periodID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "programs", map[string]any{"Programs": programs})
}

func (h *Handler) Groups(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.URL.Query().Get("period_id"), 10, 64)
	if err != nil || periodID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a program to see its groups.</p>`))
		return
	}
	programID, err := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	if err != nil || programID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a program to see its groups.</p>`))
		return
	}

	groups, err := h.store.GroupsForProgram(r.Context(), periodID, programID)
	if err != nil {
		log.Printf("GroupsForProgram(%d,%d): %v", periodID, programID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "groups", map[string]any{"Groups": groups})
}

func (h *Handler) Classes(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.URL.Query().Get("period_id"), 10, 64)
	groupID, err2 := strconv.ParseInt(r.URL.Query().Get("group_id"), 10, 64)
	if err != nil || err2 != nil || periodID == 0 || groupID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a group to see its classes.</p>`))
		return
	}

	classes, err := h.store.ClassesForGroup(r.Context(), periodID, groupID)
	if err != nil {
		log.Printf("ClassesForGroup(%d,%d): %v", periodID, groupID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "classes", map[string]any{"Classes": classes})
}

func (h *Handler) Attendance(w http.ResponseWriter, r *http.Request) {
	classIDs := parseIDs(r.URL.Query()["class_id"])
	if len(classIDs) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Check one or more classes above to view attendance.</p>`))
		return
	}

	sessions, rows, err := h.store.AttendanceGrid(r.Context(), classIDs)
	if err != nil {
		log.Printf("AttendanceGrid: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "attendance", map[string]any{
		"Sessions": sessions,
		"Rows":     rows,
	})
}

func (h *Handler) AttendancePopup(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseInt(r.URL.Query().Get("session_id"), 10, 64)
	studentID, err2 := strconv.ParseInt(r.URL.Query().Get("student_id"), 10, 64)
	if err != nil || err2 != nil || sessionID == 0 || studentID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, err := h.store.GetAttendancePopupData(r.Context(), sessionID, studentID)
	if err != nil {
		log.Printf("GetAttendancePopupData(%d,%d): %v", sessionID, studentID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "att-popup", data)
}

func (h *Handler) SetAttendance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sessionID, err := strconv.ParseInt(r.FormValue("session_id"), 10, 64)
	studentID, err2 := strconv.ParseInt(r.FormValue("student_id"), 10, 64)
	if err != nil || err2 != nil || sessionID == 0 || studentID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	valid := map[string]bool{"Present": true, "Online": true, "Absent-Notified": true, "Absent-Unnotified": true, "Excused": true, "": true}
	if !valid[status] {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cell, err := h.store.SetAttendance(r.Context(), sessionID, studentID, status)
	if err != nil {
		log.Printf("SetAttendance(%d,%d,%q): %v", sessionID, studentID, status, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "att-cell", cell)
}

func (h *Handler) Timetable(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	q := r.URL.Query()

	// ── view / display controls ───────────────────────────────────────────
	view := q.Get("view")
	switch view {
	case "list", "month":
	default:
		view = "week"
	}
	hideWeekends := q.Get("hide_weekends") == "1"

	showAll := q.Get("all") == "1"

	tf := store.TimetableFilters{}

	// ── role-based default: show only own sessions unless overridden ───────
	isDefault := false
	canToggleScope := user.PersonID > 0
	if !showAll && canToggleScope {
		tf.PersonID = user.PersonID
		isDefault = true
	}

	// ── reference date ────────────────────────────────────────────────────
	today := normaliseDay("")
	todayISO := today.Format("2006-01-02")

	var refDate time.Time
	if ms := q.Get("month"); ms != "" {
		if mt, err := time.Parse("2006-01", ms); err == nil {
			refDate = time.Date(mt.Year(), mt.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
	}
	if refDate.IsZero() {
		refDate = normaliseDay(q.Get("week"))
	}
	weekStart := mondayOf(refDate)
	monthStart := time.Date(refDate.Year(), refDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	weekISO  := weekStart.Format("2006-01-02")
	monthISO := monthStart.Format("2006-01")

	// ── URL suffix helpers ────────────────────────────────────────────────
	hwPart := ""
	if hideWeekends {
		hwPart = "&hide_weekends=1"
	}
	allPart := ""
	if showAll {
		allPart = "&all=1"
	}
	extras := hwPart + allPart

	hwToggle := "&hide_weekends=1"
	if hideWeekends {
		hwToggle = ""
	}

	showAllURL    := fmt.Sprintf("/timetable?view=%s&week=%s%s&all=1", view, weekISO, hwPart)
	mySessionsURL := fmt.Sprintf("/timetable?view=%s&week=%s%s", view, weekISO, hwPart)
	if view == "month" {
		showAllURL    = fmt.Sprintf("/timetable?view=month&month=%s%s&all=1", monthISO, hwPart)
		mySessionsURL = fmt.Sprintf("/timetable?view=month&month=%s%s", monthISO, hwPart)
	}

	// ── assemble common template data ─────────────────────────────────────
	data := map[string]any{
		"User":           user,
		"View":           view,
		"HideWeekends":   hideWeekends,
		"TodayISO":       todayISO,
		"IsDefault":      isDefault,
		"IsShowAll":      showAll && canToggleScope,
		"CanToggleScope": canToggleScope,
		"ShowAllURL":     showAllURL,
		"MySessionsURL":  mySessionsURL,
		"WeekViewURL":    fmt.Sprintf("/timetable?view=week&week=%s%s", weekISO, extras),
		"ListViewURL":    fmt.Sprintf("/timetable?view=list&week=%s%s", weekISO, extras),
		"MonthViewURL":   fmt.Sprintf("/timetable?view=month&month=%s%s", monthISO, extras),
		"CurrentWeekISO":  weekISO,
		"CurrentMonthISO": monthISO,
	}

	// ── month view ────────────────────────────────────────────────────────
	if view == "month" {
		lastDay  := monthStart.AddDate(0, 1, 0).AddDate(0, 0, -1)
		gridStart := monthStart.AddDate(0, 0, -((int(monthStart.Weekday())+6)%7))
		gridEnd  := lastDay.AddDate(0, 0, (7-int(lastDay.Weekday()))%7).AddDate(0, 0, 1)

		sessions, err := h.store.TimetableRange(r.Context(), gridStart, gridEnd, tf)
		if err != nil {
			log.Printf("TimetableRange: %v", err)
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		byDay := make(map[string][]store.TimetableSession)
		for _, s := range sessions {
			k := s.SessionDate.Format("2006-01-02")
			byDay[k] = append(byDay[k], s)
		}

		type monthCell struct {
			ISO         string
			DayNum      int
			IsThisMonth bool
			IsToday     bool
			Sessions    []store.TimetableSession
		}
		var monthGrid [][]monthCell
		for d := gridStart; d.Before(gridEnd); d = d.AddDate(0, 0, 7) {
			var row []monthCell
			for i := 0; i < 7; i++ {
				day := d.AddDate(0, 0, i)
				iso := day.Format("2006-01-02")
				row = append(row, monthCell{
					ISO:         iso,
					DayNum:      day.Day(),
					IsThisMonth: day.Month() == monthStart.Month(),
					IsToday:     iso == todayISO,
					Sessions:    byDay[iso],
				})
			}
			monthGrid = append(monthGrid, row)
		}

		prevM := monthStart.AddDate(0, -1, 0)
		nextM := monthStart.AddDate(0, 1, 0)
		data["PeriodLabel"]    = monthStart.Format("January 2006")
		data["PrevURL"]        = fmt.Sprintf("/timetable?view=month&month=%s%s", prevM.Format("2006-01"), extras)
		data["NextURL"]        = fmt.Sprintf("/timetable?view=month&month=%s%s", nextM.Format("2006-01"), extras)
		data["TodayURL"]       = fmt.Sprintf("/timetable?view=month&month=%s%s", today.Format("2006-01"), extras)
		data["MonthGrid"]      = monthGrid
		data["CurrentDateISO"] = monthStart.Format("2006-01-02")

	// ── week / list view ──────────────────────────────────────────────────
	} else {
		sessions, err := h.store.TimetableRange(r.Context(), weekStart, weekStart.AddDate(0, 0, 7), tf)
		if err != nil {
			log.Printf("TimetableRange: %v", err)
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		byDay := make(map[string][]store.TimetableSession)
		for _, s := range sessions {
			k := s.SessionDate.Format("2006-01-02")
			byDay[k] = append(byDay[k], s)
		}

		type ttDay struct {
			ISO       string
			DayName   string
			DayLabel  string
			IsToday   bool
			IsWeekend bool
			Sessions  []store.TimetableSession
		}
		allDays := make([]ttDay, 7)
		for i := 0; i < 7; i++ {
			d := weekStart.AddDate(0, 0, i)
			wd := d.Weekday()
			iso := d.Format("2006-01-02")
			allDays[i] = ttDay{
				ISO:       iso,
				DayName:   d.Format("Monday"),
				DayLabel:  d.Format("2 Jan"),
				IsToday:   iso == todayISO,
				IsWeekend: wd == time.Saturday || wd == time.Sunday,
				Sessions:  byDay[iso],
			}
		}
		displayDays := allDays[:]
		if hideWeekends {
			displayDays = allDays[:5]
		}

		weekEnd := weekStart.AddDate(0, 0, 6)
		data["PeriodLabel"]    = weekStart.Format("2 Jan") + " – " + weekEnd.Format("2 Jan 2006")
		data["PrevURL"]        = fmt.Sprintf("/timetable?view=%s&week=%s%s", view, weekStart.AddDate(0, 0, -7).Format("2006-01-02"), extras)
		data["NextURL"]        = fmt.Sprintf("/timetable?view=%s&week=%s%s", view, weekStart.AddDate(0, 0, 7).Format("2006-01-02"), extras)
		data["TodayURL"]       = fmt.Sprintf("/timetable?view=%s&week=%s%s", view, mondayOf(today).Format("2006-01-02"), extras)
		data["HideWeekendsURL"] = fmt.Sprintf("/timetable?view=%s&week=%s%s%s", view, weekISO, hwToggle, allPart)
		data["Days"]           = displayDays
		data["CurrentDateISO"] = weekISO

		if view == "list" {
			var listSessions []store.TimetableSession
			for _, d := range allDays {
				listSessions = append(listSessions, d.Sessions...)
			}
			data["Sessions"] = listSessions
		}
	}

	h.render(w, "timetable", data)
}

// normaliseDay parses a YYYY-MM-DD string (or uses today) and returns UTC midnight.
func normaliseDay(s string) time.Time {
	if s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.UTC()
		}
	}
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// mondayOf returns midnight-UTC of the Monday of the week containing t.
func mondayOf(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Mon=0 … Sun=6
	return t.AddDate(0, 0, -offset)
}

func (h *Handler) Results(w http.ResponseWriter, r *http.Request) {
	classIDs := parseIDs(r.URL.Query()["class_id"])
	if len(classIDs) == 0 {
		h.render(w, "results", map[string]any{"Cols": nil, "Rows": nil})
		return
	}
	cols, rows, err := h.store.ResultsGrid(r.Context(), classIDs)
	if err != nil {
		log.Printf("ResultsGrid: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "results", map[string]any{"Cols": cols, "Rows": rows})
}

func (h *Handler) ResultPopup(w http.ResponseWriter, r *http.Request) {
	cseID, err := strconv.ParseInt(r.URL.Query().Get("cse_id"), 10, 64)
	if err != nil || cseID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, err := h.store.GetResultPopupData(r.Context(), cseID)
	if err != nil {
		log.Printf("GetResultPopupData(%d): %v", cseID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "result-popup", data)
}

func (h *Handler) SetResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cseID, err := strconv.ParseInt(r.FormValue("cse_id"), 10, 64)
	if err != nil || cseID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	result := r.FormValue("result") // "SC", "NS", or ""
	if result != "SC" && result != "NS" && result != "IP" && result != "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cell, err := h.store.SetResult(r.Context(), cseID, result)
	if err != nil {
		log.Printf("SetResult(%d,%q): %v", cseID, result, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "result-cell", cell)
}

func (h *Handler) PublishResult(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cseID, err := strconv.ParseInt(r.FormValue("cse_id"), 10, 64)
	if err != nil || cseID == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cell, err := h.store.PublishResult(r.Context(), cseID)
	if err != nil {
		log.Printf("PublishResult(%d): %v", cseID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "result-cell", cell)
}

func (h *Handler) PublishSCColumn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	colSubjectID, _ := strconv.ParseInt(r.FormValue("col_subject_id"), 10, 64)
	if colSubjectID > 0 {
		if err := h.store.PublishSCColumn(r.Context(), colSubjectID, parseIDs(r.Form["class_id"])); err != nil {
			log.Printf("PublishSCColumn(sub=%d): %v", colSubjectID, err)
		}
	}
	classIDs := parseIDs(r.Form["class_id"])
	if len(classIDs) == 0 {
		h.render(w, "results", map[string]any{"Cols": nil, "Rows": nil})
		return
	}
	cols, rows, err := h.store.ResultsGrid(r.Context(), classIDs)
	if err != nil {
		log.Printf("ResultsGrid after publish-sc: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "results", map[string]any{"Cols": cols, "Rows": rows})
}

// ── Admin ──────────────────────────────────────────────────────────────────

var auStates = []string{"NSW", "VIC", "QLD", "SA", "WA", "TAS", "NT", "ACT"}

type personForm struct {
	ID                 int64
	Title              string
	FirstName          string
	FamilyName         string
	PreferredName      string
	DOBStr             string
	Gender             string
	Email              string
	PhoneMobile        string
	Suburb             string
	StateCode          string
	Postcode           string
	PhotoURL           string
	WWCCNumber         string
	WWCCExpiryStr      string
	PoliceCheckStatus  string
	PoliceCheckDateStr string
}

func personFormFrom(d store.PersonDetail) personForm {
	return personForm{
		ID: d.ID, Title: d.Title, FirstName: d.FirstName, FamilyName: d.FamilyName,
		PreferredName: d.PreferredName, DOBStr: d.DOB.Format("2006-01-02"),
		Gender: d.Gender, Email: d.Email, PhoneMobile: d.PhoneMobile,
		Suburb: d.Suburb, StateCode: d.StateCode, Postcode: d.Postcode,
		PhotoURL: d.PhotoURL, WWCCNumber: d.WWCCNumber, WWCCExpiryStr: d.WWCCExpiryStr,
		PoliceCheckStatus: d.PoliceCheckStatus, PoliceCheckDateStr: d.PoliceCheckDateStr,
	}
}

func personFormFromPost(r *http.Request, id int64) personForm {
	return personForm{
		ID: id, Title: r.FormValue("title"),
		FirstName:     strings.TrimSpace(r.FormValue("first_name")),
		FamilyName:    strings.TrimSpace(r.FormValue("family_name")),
		PreferredName: strings.TrimSpace(r.FormValue("preferred_name")),
		DOBStr: r.FormValue("dob"), Gender: r.FormValue("gender"),
		Email:       strings.TrimSpace(r.FormValue("email")),
		PhoneMobile: strings.TrimSpace(r.FormValue("phone_mobile")),
		Suburb:      strings.TrimSpace(r.FormValue("suburb")),
		StateCode:   r.FormValue("state_code"),
		Postcode:    strings.TrimSpace(r.FormValue("postcode")),
		PhotoURL:           strings.TrimSpace(r.FormValue("photo_url")),
		WWCCNumber:         strings.TrimSpace(r.FormValue("wwcc_number")),
		WWCCExpiryStr:      r.FormValue("wwcc_expiry"),
		PoliceCheckStatus:  r.FormValue("police_check_status"),
		PoliceCheckDateStr: r.FormValue("police_check_date"),
	}
}

func backupDSN() string {
	return os.Getenv("DATABASE_URL")
}

// jsonSafe converts pgx-native values to types that encoding/json handles cleanly.
func jsonSafe(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		return t
	case time.Time:
		return t.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func (h *Handler) BackupPage(w http.ResponseWriter, r *http.Request) {
	tables, err := h.store.ListTableNames(r.Context())
	if err != nil {
		log.Printf("ListTableNames: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "backup", map[string]any{
		"Tables": tables,
		"User":   user,
	})
}

func (h *Handler) BackupSQL(w http.ResponseWriter, r *http.Request) {
	filename := "nvims-" + time.Now().Format("2006-01-02-150405") + ".sql"
	cmd := exec.CommandContext(r.Context(), "pg_dump", backupDSN())
	out, err := cmd.Output()
	if err != nil {
		log.Printf("pg_dump: %v", err)
		http.Error(w, "Backup failed — check server logs.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
}

func (h *Handler) BackupJSON(w http.ResponseWriter, r *http.Request) {
	tables, err := h.store.ListTableNames(r.Context())
	if err != nil {
		log.Printf("ListTableNames: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	tablesData := make(map[string]any, len(tables))
	for _, tbl := range tables {
		cols, rows, err := h.store.ExportTableRows(r.Context(), tbl)
		if err != nil {
			log.Printf("ExportTableRows(%s): %v", tbl, err)
			continue
		}
		tableRows := make([]map[string]any, len(rows))
		for i, row := range rows {
			m := make(map[string]any, len(cols))
			for j, col := range cols {
				m[col] = jsonSafe(row[j])
			}
			tableRows[i] = m
		}
		tablesData[tbl] = tableRows
	}

	payload := map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"tables":      tablesData,
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		http.Error(w, "json error", http.StatusInternalServerError)
		return
	}
	filename := "nvims-" + time.Now().Format("2006-01-02-150405") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	_, _ = w.Write(out)
}

func (h *Handler) BackupTable(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	tableName := r.FormValue("table")
	format := r.FormValue("format")

	// validate table name against the live schema
	tables, err := h.store.ListTableNames(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	allowed := false
	for _, t := range tables {
		if t == tableName {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "invalid table", http.StatusBadRequest)
		return
	}

	cols, rows, err := h.store.ExportTableRows(r.Context(), tableName)
	if err != nil {
		log.Printf("ExportTableRows(%s): %v", tableName, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	ts := time.Now().Format("2006-01-02-150405")
	switch format {
	case "csv":
		filename := tableName + "-" + ts + ".csv"
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		cw := csv.NewWriter(w)
		_ = cw.Write(cols)
		for _, row := range rows {
			rec := make([]string, len(row))
			for i, v := range row {
				if v == nil {
					rec[i] = ""
				} else {
					rec[i] = fmt.Sprintf("%v", v)
				}
			}
			_ = cw.Write(rec)
		}
		cw.Flush()
	case "json":
		tableRows := make([]map[string]any, len(rows))
		for i, row := range rows {
			m := make(map[string]any, len(cols))
			for j, col := range cols {
				m[col] = jsonSafe(row[j])
			}
			tableRows[i] = m
		}
		out, err := json.MarshalIndent(tableRows, "", "  ")
		if err != nil {
			http.Error(w, "json error", http.StatusInternalServerError)
			return
		}
		filename := tableName + "-" + ts + ".json"
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(out)))
		_, _ = w.Write(out)
	default:
		http.Error(w, "invalid format", http.StatusBadRequest)
	}
}

func (h *Handler) AdminMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "admin-menu", map[string]any{"User": user})
}

func (h *Handler) AdminPeople(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	role := r.URL.Query().Get("role")
	if !map[string]bool{"Student": true, "Teacher": true, "Staff": true}[role] {
		role = ""
	}
	limit := 50
	switch r.URL.Query().Get("limit") {
	case "20":
		limit = 20
	case "100":
		limit = 100
	}
	result, err := h.store.ListPeople(r.Context(), search, role, limit)
	if err != nil {
		log.Printf("ListPeople: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-people", map[string]any{
		"People":    result.Rows,
		"Total":     result.Total,
		"Limit":     limit,
		"Search":    search,
		"Role":      role,
		"User":      user,
	})
}

func (h *Handler) AdminPersonNew(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "admin-person", map[string]any{
		"IsNew": true, "Form": personForm{}, "Error": "", "States": auStates, "User": user,
	})
}

func (h *Handler) AdminPersonCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := personFormFromPost(r, 0)
	if f.FirstName == "" || f.FamilyName == "" || f.DOBStr == "" || f.Gender == "" ||
		f.Email == "" || f.Suburb == "" || f.StateCode == "" || f.Postcode == "" {
		h.render(w, "admin-person", map[string]any{
			"IsNew": true, "Form": f, "Error": "Please fill in all required fields.", "States": auStates, "User": user,
		})
		return
	}
	id, err := h.store.CreatePerson(r.Context(),
		f.Title, f.FirstName, f.FamilyName, f.PreferredName,
		f.DOBStr, f.Gender, f.Email, f.PhoneMobile,
		f.Suburb, f.StateCode, f.Postcode,
		f.WWCCNumber, f.WWCCExpiryStr, f.PhotoURL,
		f.PoliceCheckStatus, f.PoliceCheckDateStr)
	if err != nil {
		log.Printf("CreatePerson: %v", err)
		h.render(w, "admin-person", map[string]any{
			"IsNew": true, "Form": f,
			"Error": "Could not save — check the email address is not already in use.",
			"States": auStates, "User": user,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d", id), http.StatusSeeOther)
}

func (h *Handler) AdminPersonView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	person, err := h.store.GetPerson(r.Context(), id)
	if err != nil {
		log.Printf("GetPerson(%d): %v", id, err)
		http.NotFound(w, r)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-person", map[string]any{
		"IsNew": false, "Form": personFormFrom(person), "Person": person,
		"Error": "", "Success": r.URL.Query().Get("saved") == "1",
		"States": auStates, "User": user,
	})
}

func (h *Handler) AdminPersonUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := personFormFromPost(r, id)
	if f.FirstName == "" || f.FamilyName == "" || f.DOBStr == "" || f.Gender == "" ||
		f.Email == "" || f.Suburb == "" || f.StateCode == "" || f.Postcode == "" {
		person, _ := h.store.GetPerson(r.Context(), id)
		h.render(w, "admin-person", map[string]any{
			"IsNew": false, "Form": f, "Person": person,
			"Error": "Please fill in all required fields.", "States": auStates, "User": user,
		})
		return
	}
	if err := h.store.UpdatePerson(r.Context(), id,
		f.Title, f.FirstName, f.FamilyName, f.PreferredName,
		f.DOBStr, f.Gender, f.Email, f.PhoneMobile,
		f.Suburb, f.StateCode, f.Postcode,
		f.WWCCNumber, f.WWCCExpiryStr, f.PhotoURL,
		f.PoliceCheckStatus, f.PoliceCheckDateStr); err != nil {
		log.Printf("UpdatePerson(%d): %v", id, err)
		person, _ := h.store.GetPerson(r.Context(), id)
		h.render(w, "admin-person", map[string]any{
			"IsNew": false, "Form": f, "Person": person,
			"Error": "Could not save — check the email address is not already in use.",
			"States": auStates, "User": user,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d?saved=1", id), http.StatusSeeOther)
}

func (h *Handler) AdminRoleForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	person, err := h.store.GetPerson(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	roleType := r.URL.Query().Get("type")
	role, _ := h.store.GetRoleDetail(r.Context(), id, roleType)
	user, _ := auth.Current(r)
	h.render(w, "admin-role-form", map[string]any{
		"Person": person, "RoleType": roleType,
		"Role": role, "Error": "", "User": user,
	})
}

func (h *Handler) AdminRoleAdd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	roleType := r.FormValue("role_type")
	number := strings.TrimSpace(r.FormValue("number"))
	email := strings.TrimSpace(r.FormValue("email"))
	empStatus := r.FormValue("employment_status")
	fte, _ := strconv.ParseFloat(r.FormValue("fte"), 64)
	user, _ := auth.Current(r)

	var roleErr error
	switch roleType {
	case "student":
		roleErr = h.store.AddStudentRole(r.Context(), id, number, email)
	case "teacher":
		person, pErr := h.store.GetPerson(r.Context(), id)
		if pErr != nil || !person.IsStaff {
			roleErr = fmt.Errorf("person must have a Staff role before a Teacher role can be added")
		} else {
			roleErr = h.store.AddTeacherRole(r.Context(), id)
		}
	case "staff":
		roleErr = h.store.AddStaffRole(r.Context(), id, number, email, empStatus, fte)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if roleErr != nil {
		log.Printf("AddRole(%s,%d): %v", roleType, id, roleErr)
		person, _ := h.store.GetPerson(r.Context(), id)
		errMsg := "Could not add role — check that the number is not already in use."
		if roleType == "teacher" {
			errMsg = roleErr.Error()
		}
		role := store.RoleDetail{
			Number: number, Email: email,
			EmploymentStatus: empStatus,
			FTE:              fte,
		}
		h.render(w, "admin-role-form", map[string]any{
			"Person": person, "RoleType": roleType,
			"Role": role,
			"Error": errMsg,
			"User": user,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d?saved=1", id), http.StatusSeeOther)
}

func (h *Handler) StudentPanel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("student_id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	d, err := h.store.GetStudentPanel(r.Context(), id)
	if err != nil {
		log.Printf("GetStudentPanel(%d): %v", id, err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	if m, ok := data.(map[string]any); ok {
		h.mu.RLock()
		m["LMSName"] = h.lmsName
		m["LMSURL"]  = h.lmsURL
		h.mu.RUnlock()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %q: %v", name, err)
	}
}

// ── Admin / Programs & Classes ─────────────────────────────────────────────

var programTypes = []string{
	"Qualification", "Skill Set", "Course in a Package",
	"Statement of Attainment", "Accredited Course",
}

type programForm struct {
	FacultyID            int64
	ProgramCode          string
	ProgramName          string
	ProgramRecognitionID string
	LevelOfEducation     string
	FieldOfEducation     string
	NominalHoursStr      string
	VetFlag              bool
	HeFlag               bool
	AQFLevelStr          string
	ProgramType          string
}

func programFormFromPost(r *http.Request) programForm {
	return programForm{
		FacultyID:            parseInt64(r.FormValue("faculty_id")),
		ProgramCode:          strings.TrimSpace(r.FormValue("program_code")),
		ProgramName:          strings.TrimSpace(r.FormValue("program_name")),
		ProgramRecognitionID: strings.TrimSpace(r.FormValue("program_recognition_id")),
		LevelOfEducation:     strings.TrimSpace(r.FormValue("level_of_education")),
		FieldOfEducation:     strings.TrimSpace(r.FormValue("field_of_education")),
		NominalHoursStr:      r.FormValue("nominal_hours"),
		VetFlag:              r.FormValue("vet_flag") == "on",
		HeFlag:               r.FormValue("he_flag") == "on",
		AQFLevelStr:          r.FormValue("aqf_level"),
		ProgramType:          r.FormValue("program_type"),
	}
}

func (h *Handler) AdminPrograms(w http.ResponseWriter, r *http.Request) {
	programs, err := h.store.ListPrograms(r.Context())
	if err != nil {
		log.Printf("ListPrograms: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	faculties, err := h.store.ListFaculties(r.Context())
	if err != nil {
		log.Printf("ListFaculties: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-programs", map[string]any{
		"Programs":     programs,
		"Faculties":    faculties,
		"ProgramTypes": programTypes,
		"Form":         programForm{VetFlag: true},
		"Error":        "",
		"Success":      r.URL.Query().Get("saved") == "1",
		"User":         user,
	})
}

func (h *Handler) AdminProgramCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	f := programFormFromPost(r)
	nominalHours, nhErr := strconv.Atoi(strings.TrimSpace(f.NominalHoursStr))
	if f.FacultyID == 0 || f.ProgramCode == "" || f.ProgramName == "" ||
		f.ProgramRecognitionID == "" || f.LevelOfEducation == "" ||
		f.FieldOfEducation == "" || nhErr != nil || nominalHours < 0 {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}

	var aqfLevel *int
	if f.AQFLevelStr != "" {
		if v, err := strconv.Atoi(f.AQFLevelStr); err == nil && v >= 1 && v <= 10 {
			aqfLevel = &v
		}
	}
	if !f.VetFlag && !f.HeFlag {
		f.VetFlag = true
	}

	_, err := h.store.CreateProgram(r.Context(),
		f.FacultyID, f.ProgramCode, f.ProgramName,
		f.ProgramRecognitionID, f.LevelOfEducation, f.FieldOfEducation,
		nominalHours, f.VetFlag, f.HeFlag, aqfLevel, f.ProgramType)
	if err != nil {
		log.Printf("CreateProgram: %v", err)
		http.Error(w, `{"error":"could not save — program code may already be in use"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminProgramUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	f := programFormFromPost(r)
	nominalHours, nhErr := strconv.Atoi(strings.TrimSpace(f.NominalHoursStr))
	if f.FacultyID == 0 || f.ProgramCode == "" || f.ProgramName == "" ||
		f.ProgramRecognitionID == "" || f.LevelOfEducation == "" ||
		f.FieldOfEducation == "" || nhErr != nil || nominalHours < 0 {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}

	var aqfLevel *int
	if f.AQFLevelStr != "" {
		if v, err := strconv.Atoi(f.AQFLevelStr); err == nil && v >= 1 && v <= 10 {
			aqfLevel = &v
		}
	}

	if err := h.store.UpdateProgram(r.Context(), id, f.FacultyID,
		f.ProgramCode, f.ProgramName,
		f.ProgramRecognitionID, f.LevelOfEducation, f.FieldOfEducation,
		nominalHours, f.VetFlag, f.HeFlag, aqfLevel, f.ProgramType); err != nil {
		log.Printf("UpdateProgram(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminProgramDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteProgram(r.Context(), id); err != nil {
		log.Printf("DeleteProgram(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type classForm struct {
	ClassCode          string
	AcademicPeriodID   int64
	DeliveryLocationID int64
	IntakeGroupID      int64
	EnrolmentCapStr    string
}

func (h *Handler) AdminClasses(w http.ResponseWriter, r *http.Request) {
	classes, err := h.store.ListClasses(r.Context())
	if err != nil {
		log.Printf("ListClasses: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	periods, err := h.store.Periods(r.Context())
	if err != nil {
		log.Printf("Periods: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	locations, err := h.store.ListDeliveryLocations(r.Context())
	if err != nil {
		log.Printf("ListDeliveryLocations: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	intakeGroups, err := h.store.ListIntakeGroups(r.Context())
	if err != nil {
		log.Printf("ListIntakeGroups: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-classes", map[string]any{
		"Classes":      classes,
		"Periods":      periods,
		"Locations":    locations,
		"IntakeGroups": intakeGroups,
		"User":         user,
	})
}

func classOptionalInt64(r *http.Request, field string) *int64 {
	v := parseInt64(r.FormValue(field))
	if v == 0 {
		return nil
	}
	return &v
}

func classOptionalInt(r *http.Request, field string) *int {
	s := strings.TrimSpace(r.FormValue(field))
	if s == "" {
		return nil
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return &v
	}
	return nil
}

func (h *Handler) AdminClassCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	classCode  := strings.TrimSpace(r.FormValue("class_code"))
	periodID   := parseInt64(r.FormValue("academic_period_id"))
	locationID := parseInt64(r.FormValue("delivery_location_id"))

	if classCode == "" || periodID == 0 || locationID == 0 {
		http.Error(w, `{"error":"code, period and location are required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateClass(r.Context(), classCode, periodID, locationID,
		classOptionalInt64(r, "intake_group_id"), classOptionalInt(r, "enrolment_cap"), nil)
	if err != nil {
		log.Printf("CreateClass: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminClassUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	classCode  := strings.TrimSpace(r.FormValue("class_code"))
	periodID   := parseInt64(r.FormValue("academic_period_id"))
	locationID := parseInt64(r.FormValue("delivery_location_id"))

	if classCode == "" || periodID == 0 || locationID == 0 {
		http.Error(w, `{"error":"code, period and location are required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateClass(r.Context(), id, periodID, locationID, classCode,
		classOptionalInt64(r, "intake_group_id"), classOptionalInt(r, "enrolment_cap")); err != nil {
		log.Printf("UpdateClass(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminClassDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteClass(r.Context(), id); err != nil {
		log.Printf("DeleteClass(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Admin / Periods ────────────────────────────────────────────────────────

var periodTypes = []string{"TERM", "SEMESTER", "TRIMESTER", "YEAR", "BLOCK", "ROLLING"}

type periodForm struct {
	PeriodCode  string
	PeriodName  string
	YearStr     string
	StartDate   string
	EndDate     string
	PeriodType  string
	SeqNumStr   string
}

func (h *Handler) AdminPeriods(w http.ResponseWriter, r *http.Request) {
	periods, err := h.store.ListPeriods(r.Context())
	if err != nil {
		log.Printf("ListPeriods: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-periods", map[string]any{
		"Periods":     periods,
		"PeriodTypes": periodTypes,
		"Form":        periodForm{},
		"Error":       "",
		"Success":     r.URL.Query().Get("saved") == "1",
		"User":        user,
	})
}

func (h *Handler) AdminPeriodCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	periodCode := strings.TrimSpace(r.FormValue("period_code"))
	periodName := strings.TrimSpace(r.FormValue("period_name"))
	yearStr    := strings.TrimSpace(r.FormValue("year"))
	startDate  := r.FormValue("start_date")
	endDate    := r.FormValue("end_date")
	periodType := r.FormValue("period_type")
	seqStr     := strings.TrimSpace(r.FormValue("sequence_number"))

	year, yearErr := strconv.Atoi(yearStr)
	if periodCode == "" || periodName == "" || yearErr != nil ||
		startDate == "" || endDate == "" || periodType == "" {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}

	var seqNum *int
	if seqStr != "" {
		if v, err := strconv.Atoi(seqStr); err == nil && v > 0 {
			seqNum = &v
		}
	}

	_, err := h.store.CreatePeriod(r.Context(), periodCode, periodName, year,
		startDate, endDate, periodType, seqNum)
	if err != nil {
		log.Printf("CreatePeriod: %v", err)
		http.Error(w, `{"error":"could not save — period code may already be in use"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminPeriodUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	periodCode := strings.TrimSpace(r.FormValue("period_code"))
	periodName := strings.TrimSpace(r.FormValue("period_name"))
	yearStr    := strings.TrimSpace(r.FormValue("year"))
	startDate  := r.FormValue("start_date")
	endDate    := r.FormValue("end_date")
	periodType := r.FormValue("period_type")
	seqStr     := strings.TrimSpace(r.FormValue("sequence_number"))

	year, yearErr := strconv.Atoi(yearStr)
	if periodCode == "" || periodName == "" || yearErr != nil ||
		startDate == "" || endDate == "" || periodType == "" {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}

	var seqNum *int
	if seqStr != "" {
		if v, err := strconv.Atoi(seqStr); err == nil && v > 0 {
			seqNum = &v
		}
	}

	if err := h.store.UpdatePeriod(r.Context(), id, periodCode, periodName, year,
		startDate, endDate, periodType, seqNum); err != nil {
		log.Printf("UpdatePeriod(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminPeriodDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeletePeriod(r.Context(), id); err != nil {
		log.Printf("DeletePeriod(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Admin / Delivery Locations ─────────────────────────────────────────────

type locationForm struct {
	TrainingOrgID    int64
	LocID            string
	Name             string
	Address          string
	Suburb           string
	StateCode        string
	Postcode         string
	PostcodeOverride string
}

func (h *Handler) AdminLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := h.store.ListDeliveryLocations(r.Context())
	if err != nil {
		log.Printf("ListDeliveryLocations: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	orgs, err := h.store.ListTrainingOrgs(r.Context())
	if err != nil {
		log.Printf("ListTrainingOrgs: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-locations", map[string]any{
		"Locations": locations,
		"Orgs":      orgs,
		"States":    auStates,
		"Form":      locationForm{},
		"Error":     "",
		"Success":   r.URL.Query().Get("saved") == "1",
		"User":      user,
	})
}

func (h *Handler) AdminLocationCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := locationForm{
		TrainingOrgID:    parseInt64(r.FormValue("training_org_id")),
		LocID:            strings.TrimSpace(r.FormValue("delivery_loc_id")),
		Name:             strings.TrimSpace(r.FormValue("name")),
		Address:          strings.TrimSpace(r.FormValue("address")),
		Suburb:           strings.TrimSpace(r.FormValue("suburb")),
		StateCode:        r.FormValue("state_code"),
		Postcode:         strings.TrimSpace(r.FormValue("postcode")),
		PostcodeOverride: strings.TrimSpace(r.FormValue("postcode_override")),
	}

	renderErr := func(msg string) {
		locations, _ := h.store.ListDeliveryLocations(r.Context())
		orgs, _ := h.store.ListTrainingOrgs(r.Context())
		h.render(w, "admin-locations", map[string]any{
			"Locations": locations, "Orgs": orgs, "States": auStates,
			"Form": f, "Error": msg, "User": user,
		})
	}

	if f.TrainingOrgID == 0 || f.LocID == "" || f.Name == "" ||
		f.Address == "" || f.Suburb == "" || f.StateCode == "" || f.Postcode == "" {
		renderErr("Please fill in all required fields.")
		return
	}

	_, err := h.store.CreateDeliveryLocation(r.Context(),
		f.TrainingOrgID, f.LocID, f.Name, false,
		f.Address, f.Suburb, f.StateCode, f.Postcode, "", "")
	if err != nil {
		log.Printf("CreateDeliveryLocation: %v", err)
		renderErr("Could not save — check the location ID is not already in use for this organisation.")
		return
	}
	http.Redirect(w, r, "/admin/locations?saved=1", http.StatusSeeOther)
}

// ── Admin / Intake Groups ──────────────────────────────────────────────────

type intakeGroupForm struct {
	IntakeID    int64
	GroupCode   string
	GroupName   string
	CapacityStr string
	Notes       string
}

func (h *Handler) AdminIntakeGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListIntakeGroups(r.Context())
	if err != nil {
		log.Printf("ListIntakeGroups: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	intakes, err := h.store.ListProgramIntakes(r.Context())
	if err != nil {
		log.Printf("ListProgramIntakes: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-intake-groups", map[string]any{
		"Groups":  groups,
		"Intakes": intakes,
		"User":    user,
	})
}

func intakeGroupCapacity(r *http.Request) *int {
	s := strings.TrimSpace(r.FormValue("capacity"))
	if s == "" {
		return nil
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return &v
	}
	return nil
}

func (h *Handler) AdminIntakeGroupCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	intakeID := parseInt64(r.FormValue("intake_id"))
	groupCode := strings.TrimSpace(r.FormValue("group_code"))
	groupName := strings.TrimSpace(r.FormValue("group_name"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	if intakeID == 0 || groupCode == "" || groupName == "" {
		http.Error(w, `{"error":"intake, code and name are required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateIntakeGroup(r.Context(), intakeID, groupCode, groupName, intakeGroupCapacity(r), notes)
	if err != nil {
		log.Printf("CreateIntakeGroup: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminIntakeGroupUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	intakeID := parseInt64(r.FormValue("intake_id"))
	groupCode := strings.TrimSpace(r.FormValue("group_code"))
	groupName := strings.TrimSpace(r.FormValue("group_name"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	if intakeID == 0 || groupCode == "" || groupName == "" {
		http.Error(w, `{"error":"intake, code and name are required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateIntakeGroup(r.Context(), id, intakeID, groupCode, groupName, intakeGroupCapacity(r), notes); err != nil {
		log.Printf("UpdateIntakeGroup(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminIntakeGroupDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteIntakeGroup(r.Context(), id); err != nil {
		log.Printf("DeleteIntakeGroup(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Admin / Class Sessions ─────────────────────────────────────────────────

var sessionTypes = []string{"Scheduled", "Replacement", "Assessment", "Online", "Other"}

type sessionForm struct {
	ClassID     int64
	SessionDate string
	StartTime   string
	EndTime     string
	SessionType string
	Notes       string
}

type generateForm struct {
	ClassID     int64
	StartDate   string
	EndDate     string
	StartTime   string
	EndTime     string
	SessionType string
	DaysOfWeek  []int
}

func (h *Handler) AdminSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	view := q.Get("view")
	if view != "group" {
		view = "teacher"
	}
	entityID := parseInt64(q.Get("id"))
	periodID := parseInt64(q.Get("period_id"))

	// class filter — only meaningful for teacher view
	var classIDs []int64
	for _, s := range q["class_id"] {
		if id := parseInt64(s); id > 0 {
			classIDs = append(classIDs, id)
		}
	}
	filterSet := make(map[int64]bool)
	for _, id := range classIDs {
		filterSet[id] = true
	}

	teachers, _ := h.store.ListTeachers(r.Context())
	intakeGroups, _ := h.store.ListIntakeGroups(r.Context())
	classes, _ := h.store.ListClasses(r.Context())
	periods, _ := h.store.ListPeriods(r.Context())
	rooms, _ := h.store.ListRooms(r.Context())
	buildings, _ := h.store.ListBuildings(r.Context())

	// find selected period
	var selPeriod *store.PeriodListRow
	for i := range periods {
		if periods[i].ID == periodID {
			selPeriod = &periods[i]
			break
		}
	}

	var sessions []store.ScheduledSessionRow
	var viewClasses []store.ClassListRow

	if entityID > 0 && selPeriod != nil {
		from := selPeriod.StartDate.Format("2006-01-02")
		to := selPeriod.EndDate.Format("2006-01-02")
		var err error
		switch view {
		case "teacher":
			sessions, err = h.store.SessionsForTeacher(r.Context(), entityID, from, to, classIDs)
			if err != nil {
				log.Printf("SessionsForTeacher(%d): %v", entityID, err)
			}
			viewClasses, _ = h.store.TeacherClassesForPeriod(r.Context(), entityID, periodID)
		case "group":
			sessions, err = h.store.SessionsForIntakeGroup(r.Context(), entityID, from, to, classIDs)
			if err != nil {
				log.Printf("SessionsForIntakeGroup(%d): %v", entityID, err)
			}
			viewClasses, _ = h.store.GroupClassesForPeriod(r.Context(), entityID, periodID)
		}
	}

	var periodEndDate, periodEndDMY string
	if selPeriod != nil {
		periodEndDate = selPeriod.EndDate.Format("2006-01-02")
		periodEndDMY = selPeriod.EndDate.Format("02/01/2006")
	}

	user, _ := auth.Current(r)
	h.render(w, "admin-sessions", map[string]any{
		"View":           view,
		"EntityID":       entityID,
		"PeriodID":       periodID,
		"HasClassFilter": len(classIDs) > 0,
		"FilterClassSet": filterSet,
		"Sessions":       sessions,
		"Teachers":       teachers,
		"IntakeGroups":   intakeGroups,
		"Classes":        classes,
		"ViewClasses":    viewClasses,
		"Periods":        periods,
		"SessionTypes":   sessionTypes,
		"PeriodEndDate":  periodEndDate,
		"PeriodEndDMY":   periodEndDMY,
		"Rooms":          rooms,
		"Buildings":      buildings,
		"User":           user,
	})
}

// checkSessionTimes returns an error if start/end are not valid HH:MM or end <= start.
func checkSessionTimes(startTime, endTime string) error {
	layout := "15:04"
	st, err1 := time.Parse(layout, startTime)
	et, err2 := time.Parse(layout, endTime)
	if err1 != nil {
		return fmt.Errorf("invalid start time %q", startTime)
	}
	if err2 != nil {
		return fmt.Errorf("invalid end time %q", endTime)
	}
	if !et.After(st) {
		return fmt.Errorf("end time must be after start time")
	}
	return nil
}

func (h *Handler) AdminSessionUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad form"}`, http.StatusBadRequest)
		return
	}
	if err := checkSessionTimes(r.FormValue("start_time"), r.FormValue("end_time")); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
		return
	}
	roomID, _ := strconv.ParseInt(r.FormValue("room_id"), 10, 64)
	buildingID, _ := strconv.ParseInt(r.FormValue("building_id"), 10, 64)
	if err := h.store.UpdateSession(r.Context(), id,
		r.FormValue("session_date"),
		r.FormValue("start_time"),
		r.FormValue("end_time"),
		r.FormValue("session_type"),
		r.FormValue("notes"),
		roomID, buildingID,
	); err != nil {
		log.Printf("UpdateSession(%d): %v", id, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"save failed"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminSessionDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteSession(r.Context(), id); err != nil {
		log.Printf("DeleteSession(%d): %v", id, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"delete failed"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminSessionSchedule(w http.ResponseWriter, r *http.Request) {
	schedType := r.URL.Query().Get("type") // "teacher" or "group"
	entityID := parseInt64(r.URL.Query().Get("id"))
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if entityID == 0 || from == "" || to == "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
		return
	}
	var sessions []store.ScheduleSession
	var err error
	switch schedType {
	case "teacher":
		sessions, err = h.store.ScheduleForTeacher(r.Context(), entityID, from, to)
	case "group":
		sessions, err = h.store.ScheduleForIntakeGroup(r.Context(), entityID, from, to)
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
		return
	}
	if err != nil {
		log.Printf("AdminSessionSchedule(%s,%d): %v", schedType, entityID, err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	type row struct {
		Date     string `json:"date"`
		Day      string `json:"day"`
		Start    string `json:"start"`
		End      string `json:"end"`
		Class    string `json:"class"`
		Subjects string `json:"subjects"`
		Type     string `json:"type"`
	}
	out := make([]row, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, row{
			Date:     s.SessionDate.Format("2006-01-02"),
			Day:      s.SessionDate.Format("Mon"),
			Start:    s.StartTime.Format("15:04"),
			End:      s.EndTime.Format("15:04"),
			Class:    s.ClassCode,
			Subjects: s.SubjectCodes,
			Type:     s.SessionType,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (h *Handler) AdminSessionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	classID := parseInt64(r.FormValue("class_id"))
	sessionDate := r.FormValue("session_date")
	startTime := r.FormValue("start_time")
	endTime := r.FormValue("end_time")
	sessionType := r.FormValue("session_type")
	notes := strings.TrimSpace(r.FormValue("notes"))

	if classID == 0 || sessionDate == "" || startTime == "" || endTime == "" {
		http.Error(w, `{"error":"missing fields"}`, http.StatusBadRequest)
		return
	}
	if err := checkSessionTimes(startTime, endTime); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}
	if sessionType == "" {
		sessionType = "Scheduled"
	}

	_, err := h.store.CreateSession(r.Context(), classID, sessionDate, startTime, endTime, sessionType, notes)
	if err != nil {
		log.Printf("CreateSession: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=db", classID), http.StatusSeeOther)
		return
	}

	if r.FormValue("autofill") == "1" {
		if periodEnd := r.FormValue("period_end"); periodEnd != "" {
			firstDate, err1 := time.Parse("2006-01-02", sessionDate)
			endDate, err2 := time.Parse("2006-01-02", periodEnd)
			if err1 == nil && err2 == nil && endDate.After(firstDate) {
				var inputs []store.SessionInput
				for d := firstDate.AddDate(0, 0, 7); !d.After(endDate); d = d.AddDate(0, 0, 7) {
					inputs = append(inputs, store.SessionInput{
						Date:      d.Format("2006-01-02"),
						StartTime: startTime,
						EndTime:   endTime,
						Type:      sessionType,
						Notes:     notes,
					})
				}
				if len(inputs) > 0 {
					if _, err2 := h.store.BulkCreateSessions(r.Context(), classID, inputs); err2 != nil {
						log.Printf("BulkCreateSessions (autofill): %v", err2)
					}
				}
			}
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&saved=1", classID), http.StatusSeeOther)
}

func (h *Handler) AdminSessionsGenerate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	classID := parseInt64(r.FormValue("class_id"))
	startDateStr := r.FormValue("start_date")
	endDateStr := r.FormValue("end_date")
	startTime := r.FormValue("start_time")
	endTime := r.FormValue("end_time")
	sessionType := r.FormValue("session_type")
	if sessionType == "" {
		sessionType = "Scheduled"
	}
	notes := strings.TrimSpace(r.FormValue("notes"))

	if classID == 0 || startDateStr == "" || endDateStr == "" || startTime == "" || endTime == "" {
		http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=missing", classID), http.StatusSeeOther)
		return
	}

	// Parse selected days of week (0=Sun … 6=Sat)
	selectedDays := map[int]bool{}
	for _, ds := range r.Form["day_of_week"] {
		if d, err := strconv.Atoi(ds); err == nil && d >= 0 && d <= 6 {
			selectedDays[d] = true
		}
	}
	if len(selectedDays) == 0 {
		http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=missing", classID), http.StatusSeeOther)
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	endDate, err2 := time.Parse("2006-01-02", endDateStr)
	if err != nil || err2 != nil || endDate.Before(startDate) {
		http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=dates", classID), http.StatusSeeOther)
		return
	}

	var inputs []store.SessionInput
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		if selectedDays[int(d.Weekday())] {
			inputs = append(inputs, store.SessionInput{
				Date:      d.Format("2006-01-02"),
				StartTime: startTime,
				EndTime:   endTime,
				Type:      sessionType,
				Notes:     notes,
			})
		}
	}

	inserted := 0
	if len(inputs) > 0 {
		inserted, err = h.store.BulkCreateSessions(r.Context(), classID, inputs)
		if err != nil {
			log.Printf("BulkCreateSessions: %v", err)
			http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=db", classID), http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&generated=%d", classID, inserted), http.StatusSeeOther)
}

func (h *Handler) AdminFaculties(w http.ResponseWriter, r *http.Request) {
	faculties, err := h.store.ListFaculties(r.Context())
	if err != nil {
		log.Printf("ListFaculties: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-faculties", map[string]any{
		"Faculties": faculties,
		"Error":     "",
		"Success":   r.URL.Query().Get("saved") == "1",
		"User":      user,
	})
}

func (h *Handler) AdminFacultyCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("faculty_name"))
	if name == "" {
		http.Error(w, `{"error":"faculty name is required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateFaculty(r.Context(), name)
	if err != nil {
		log.Printf("CreateFaculty: %v", err)
		http.Error(w, `{"error":"could not save — name may already be in use"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminFacultyUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("faculty_name"))
	if name == "" {
		http.Error(w, `{"error":"faculty name is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateFaculty(r.Context(), id, name); err != nil {
		log.Printf("UpdateFaculty(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminFacultyDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteFaculty(r.Context(), id); err != nil {
		log.Printf("DeleteFaculty(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseFacultyID(s string) pgtype.Int8 {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
		return pgtype.Int8{Int64: v, Valid: true}
	}
	return pgtype.Int8{}
}

func (h *Handler) AdminDepartments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	depts, err := h.store.ListDepartments(ctx)
	if err != nil {
		log.Printf("ListDepartments: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	faculties, err := h.store.ListFaculties(ctx)
	if err != nil {
		log.Printf("ListFaculties: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-departments", map[string]any{
		"Departments": depts,
		"Faculties":   faculties,
		"User":        user,
	})
}

func (h *Handler) AdminDepartmentCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("dept_name"))
	if name == "" {
		http.Error(w, `{"error":"department name is required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateDepartment(r.Context(), name, parseFacultyID(r.FormValue("faculty_id")))
	if err != nil {
		log.Printf("CreateDepartment: %v", err)
		http.Error(w, `{"error":"could not save"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminDepartmentUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("dept_name"))
	if name == "" {
		http.Error(w, `{"error":"department name is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateDepartment(r.Context(), id, name, parseFacultyID(r.FormValue("faculty_id"))); err != nil {
		log.Printf("UpdateDepartment(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminDepartmentDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteDepartment(r.Context(), id); err != nil {
		log.Printf("DeleteDepartment(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type subjectForm struct {
	SubjectCode      string
	SubjectName      string
	ModuleFlag       string
	FieldOfEducation string
	NominalHoursStr  string
	VetFlag          bool
	CreditPointsStr  string
}

func subjectFormFromPost(r *http.Request) subjectForm {
	return subjectForm{
		SubjectCode:      strings.TrimSpace(r.FormValue("subject_code")),
		SubjectName:      strings.TrimSpace(r.FormValue("subject_name")),
		ModuleFlag:       r.FormValue("module_flag"),
		FieldOfEducation: strings.TrimSpace(r.FormValue("field_of_education")),
		NominalHoursStr:  strings.TrimSpace(r.FormValue("nominal_hours")),
		VetFlag:          r.FormValue("vet_flag") == "on",
		CreditPointsStr:  strings.TrimSpace(r.FormValue("credit_points")),
	}
}

func (h *Handler) AdminSubjects(w http.ResponseWriter, r *http.Request) {
	subjects, err := h.store.ListSubjects(r.Context())
	if err != nil {
		log.Printf("ListSubjects: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-subjects", map[string]any{
		"Subjects": subjects,
		"User":     user,
	})
}

func subjectNullableInt(r *http.Request, field string) *int {
	s := strings.TrimSpace(r.FormValue(field))
	if s == "" {
		return nil
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return &v
	}
	return nil
}

func parseSubjectFields(r *http.Request) (code, name, mod, field string, vet bool) {
	code  = strings.TrimSpace(r.FormValue("subject_code"))
	name  = strings.TrimSpace(r.FormValue("subject_name"))
	field = strings.TrimSpace(r.FormValue("field_of_education"))
	mod   = r.FormValue("module_flag")
	if mod != "Y" && mod != "N" {
		mod = "N"
	}
	vet = r.FormValue("vet_flag") == "1"
	return
}

func (h *Handler) AdminSubjectCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code, name, mod, field, vet := parseSubjectFields(r)
	if code == "" || name == "" || field == "" {
		http.Error(w, `{"error":"code, name and field are required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateSubject(r.Context(), code, name, mod, field,
		subjectNullableInt(r, "nominal_hours"), vet, subjectNullableInt(r, "credit_points"))
	if err != nil {
		log.Printf("CreateSubject: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminSubjectUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code, name, mod, field, vet := parseSubjectFields(r)
	if code == "" || name == "" || field == "" {
		http.Error(w, `{"error":"code, name and field are required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateSubject(r.Context(), id, code, name, mod, field,
		subjectNullableInt(r, "nominal_hours"), subjectNullableInt(r, "credit_points"), vet); err != nil {
		log.Printf("UpdateSubject(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminSubjectDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteSubject(r.Context(), id); err != nil {
		log.Printf("DeleteSubject(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func parseIDs(vals []string) []int64 {
	var out []int64
	for _, v := range vals {
		for _, part := range strings.Split(v, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && id > 0 {
				out = append(out, id)
			}
		}
	}
	return out
}

// ── VCC ───────────────────────────────────────────────────────────────────────

var validVCCStatuses = []string{"Draft", "Submitted", "Approved", "Rejected"}
var validVCCItemStatuses = []string{"Draft", "Pending", "Approved", "Rejected"}

var roomTypes = []string{"Classroom", "Computer Lab", "Seminar Room", "Workshop", "Lecture Theatre", "Other"}
var validVCCMethods = []string{
	"I hold the current unit of competency",
	"I hold a superseded and equivalent unit of competency",
	"I hold a recognition of relevant study",
	"I have vocational work experience",
	"Other",
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (h *Handler) WorkplanMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "workplan-menu", map[string]any{"User": user})
}

type availDay struct {
	DayNum    int
	DayName   string
	ShortName string
	Available bool
	StartTime string
	EndTime   string
	Notes     string
}

func (h *Handler) WorkplanAvailability(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, "no linked person", http.StatusForbidden)
		return
	}

	empStatus, _ := h.store.GetTeacherEmploymentStatus(r.Context(), user.PersonID)
	rows, err := h.store.GetTeacherAvailability(r.Context(), user.PersonID)
	if err != nil {
		log.Printf("GetTeacherAvailability(%d): %v", user.PersonID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	byDay := make(map[int]store.TeacherAvailability)
	for _, a := range rows {
		byDay[a.Day] = a
	}

	days := []availDay{
		{1, "Monday", "Mon", false, "08:00", "22:00", ""},
		{2, "Tuesday", "Tue", false, "08:00", "22:00", ""},
		{3, "Wednesday", "Wed", false, "08:00", "22:00", ""},
		{4, "Thursday", "Thu", false, "08:00", "22:00", ""},
		{5, "Friday", "Fri", false, "08:00", "22:00", ""},
		{6, "Saturday", "Sat", false, "08:00", "22:00", ""},
	}
	for i, d := range days {
		if a, ok := byDay[d.DayNum]; ok {
			days[i].Available = true
			days[i].StartTime = a.StartTime
			days[i].EndTime = a.EndTime
			days[i].Notes = a.Notes
		}
	}

	leaveReqs, err := h.store.ListLeaveRequests(r.Context(), user.PersonID)
	if err != nil {
		log.Printf("ListLeaveRequests(%d): %v", user.PersonID, err)
		leaveReqs = nil
	}

	h.render(w, "workplan-availability", map[string]any{
		"User":           user,
		"EmploymentType": empStatus,
		"Days":           days,
		"HasAny":         len(rows) > 0,
		"LeaveRequests":  leaveReqs,
	})
}

func (h *Handler) WorkplanAvailabilitySet(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, `{"error":"no linked person"}`, http.StatusForbidden)
		return
	}
	dayStr := r.PathValue("day")
	var day int
	fmt.Sscanf(dayStr, "%d", &day)
	if day < 1 || day > 6 {
		http.Error(w, `{"error":"invalid day"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		r.ParseForm()
	}
	start := strings.TrimSpace(r.FormValue("start"))
	end := strings.TrimSpace(r.FormValue("end"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	if start == "" || end == "" {
		http.Error(w, `{"error":"start and end required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpsertTeacherAvailability(r.Context(), user.PersonID, day, start, end, notes); err != nil {
		log.Printf("UpsertTeacherAvailability(%d, %d): %v", user.PersonID, day, err)
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func (h *Handler) WorkplanAvailabilityDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, `{"error":"no linked person"}`, http.StatusForbidden)
		return
	}
	dayStr := r.PathValue("day")
	var day int
	fmt.Sscanf(dayStr, "%d", &day)
	if day < 1 || day > 6 {
		http.Error(w, `{"error":"invalid day"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteTeacherAvailability(r.Context(), user.PersonID, day); err != nil {
		log.Printf("DeleteTeacherAvailability(%d, %d): %v", user.PersonID, day, err)
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

func (h *Handler) WorkplanAvailabilitySetDefaults(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, `{"error":"no linked person"}`, http.StatusForbidden)
		return
	}
	for day := 1; day <= 5; day++ {
		if err := h.store.UpsertTeacherAvailability(r.Context(), user.PersonID, day, "08:00", "22:00", ""); err != nil {
			log.Printf("UpsertTeacherAvailability defaults (%d, %d): %v", user.PersonID, day, err)
			http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, "/workplan/availability", http.StatusSeeOther)
}

func (h *Handler) WorkplanLeaveCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, `{"error":"no linked person"}`, http.StatusForbidden)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		r.ParseForm()
	}

	leaveType := strings.TrimSpace(r.FormValue("leave_type"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	mode := r.FormValue("mode") // "range" or "individual"

	var dates []string
	if mode == "range" {
		start := r.FormValue("range_start")
		end := r.FormValue("range_end")
		if start == "" || end == "" {
			http.Error(w, `{"error":"start and end date required"}`, http.StatusBadRequest)
			return
		}
		startDate, err1 := time.Parse("2006-01-02", start)
		endDate, err2 := time.Parse("2006-01-02", end)
		if err1 != nil || err2 != nil || endDate.Before(startDate) {
			http.Error(w, `{"error":"invalid date range"}`, http.StatusBadRequest)
			return
		}
		for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			dates = append(dates, d.Format("2006-01-02"))
		}
	} else {
		for _, d := range r.Form["dates[]"] {
			d = strings.TrimSpace(d)
			if d != "" {
				if _, err := time.Parse("2006-01-02", d); err == nil {
					dates = append(dates, d)
				}
			}
		}
	}

	if len(dates) == 0 {
		http.Error(w, `{"error":"no dates selected"}`, http.StatusBadRequest)
		return
	}

	isPartial := r.FormValue("is_partial") == "1"
	partialStart := strings.TrimSpace(r.FormValue("partial_start"))
	partialEnd := strings.TrimSpace(r.FormValue("partial_end"))
	if isPartial && len(dates) > 1 {
		http.Error(w, `{"error":"partial day only allowed for a single day"}`, http.StatusBadRequest)
		return
	}

	id, err := h.store.CreateLeaveRequest(r.Context(), user.PersonID,
		leaveType, isPartial, partialStart, partialEnd, notes, dates)
	if err != nil {
		log.Printf("CreateLeaveRequest(%d): %v", user.PersonID, err)
		http.Error(w, `{"error":"save failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"id":%d}`, id)
}

func (h *Handler) WorkplanLeaveCancel(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if user.PersonID == 0 {
		http.Error(w, `{"error":"no linked person"}`, http.StatusForbidden)
		return
	}
	var id int64
	fmt.Sscanf(r.PathValue("id"), "%d", &id)
	if err := h.store.CancelLeaveRequest(r.Context(), user.PersonID, id); err != nil {
		log.Printf("CancelLeaveRequest(%d, %d): %v", user.PersonID, id, err)
		http.Error(w, `{"error":"cancel failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// VCCMenu — landing page with section tiles.
func (h *Handler) VCCMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "vcc-menu", map[string]any{"User": user})
}

// VCCVocationalEvidence — sub-menu for vocational qualifications and certifications.
func (h *Handler) VCCVocationalEvidence(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "vcc-vocational-evidence", map[string]any{"User": user})
}

// VCCIndex — show the current user's own VCC as an editable page.
func (h *Handler) VCCIndex(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	vcc, err := h.store.GetLatestVCCForTeacher(r.Context(), user.PersonID)
	if err == pgx.ErrNoRows {
		h.render(w, "vcc-detail", map[string]any{"VCC": nil, "User": user})
		return
	}
	if err != nil {
		log.Printf("GetLatestVCCForTeacher(%d): %v", user.PersonID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "vcc-detail", map[string]any{
		"VCC": vcc, "User": user,
		"VCCStatuses":  validVCCStatuses,
		"ItemStatuses": validVCCItemStatuses,
		"VCCMethods":   validVCCMethods,
	})
}

func (h *Handler) VCCUpdateStatus(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	if !containsStr(validVCCStatuses, status) {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateVCCStatus(r.Context(), user.PersonID, status); err != nil {
		log.Printf("UpdateVCCStatus(%d): %v", user.PersonID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCUnitUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	unitID, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("unit_code"))
	title := strings.TrimSpace(r.FormValue("unit_title"))
	method := r.FormValue("competency_method")
	status := r.FormValue("status")
	if code == "" || title == "" || !containsStr(validVCCMethods, method) || !containsStr(validVCCItemStatuses, status) {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	approvedAt := parseDateField(r.FormValue("approved_at"))
	if err := h.store.UpdateVCCUnit(r.Context(), user.PersonID, unitID,
		code, title, method,
		strings.TrimSpace(r.FormValue("superseded_unit_code")),
		strings.TrimSpace(r.FormValue("superseded_unit_title")),
		strings.TrimSpace(r.FormValue("description")),
		status, approvedAt,
	); err != nil {
		log.Printf("UpdateVCCUnit(%d,%d): %v", user.PersonID, unitID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func parseRating(s string) pgtype.Int2 {
	if v, err := strconv.ParseInt(s, 10, 16); err == nil && v >= 1 && v <= 5 {
		return pgtype.Int2{Int16: int16(v), Valid: true}
	}
	return pgtype.Int2{}
}

func (h *Handler) VCCUnitElementCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	unitID, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	element := strings.TrimSpace(r.FormValue("element"))
	if element == "" {
		http.Error(w, `{"error":"element text is required"}`, http.StatusBadRequest)
		return
	}
	id, err := h.store.CreateVCCUnitElement(r.Context(), user.PersonID, unitID,
		element, strings.TrimSpace(r.FormValue("justification")))
	if err != nil {
		log.Printf("CreateVCCUnitElement(%d): %v", unitID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true,"id":%d}`, id)
}

func (h *Handler) VCCUnitElementUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	elemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	element := strings.TrimSpace(r.FormValue("element"))
	if element == "" {
		http.Error(w, `{"error":"element text is required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateVCCUnitElement(r.Context(), user.PersonID, elemID,
		element, strings.TrimSpace(r.FormValue("justification"))); err != nil {
		log.Printf("UpdateVCCUnitElement(%d): %v", elemID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCUnitElementDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	elemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteVCCUnitElement(r.Context(), user.PersonID, elemID); err != nil {
		log.Printf("DeleteVCCUnitElement(%d): %v", elemID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) VCCUnitRatingSave(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	unitID, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	err = h.store.UpdateVCCUnitRatings(r.Context(), user.PersonID, unitID,
		parseRating(r.FormValue("enthusiasm_rating")),
		parseRating(r.FormValue("confidence_rating")),
	)
	if err != nil {
		log.Printf("UpdateVCCUnitRatings(%d,%d): %v", user.PersonID, unitID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCPQUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	pqID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("qualification_code"))
	title := strings.TrimSpace(r.FormValue("qualification_title"))
	status := r.FormValue("status")
	if code == "" || title == "" || !containsStr(validVCCItemStatuses, status) {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	approvedAt := parseDateField(r.FormValue("approved_at"))
	aqfLevel, _ := strconv.Atoi(r.FormValue("aqf_level"))
	if err := h.store.UpdateVCCPQ(r.Context(), user.PersonID, pqID,
		code, title,
		strings.TrimSpace(r.FormValue("institution")),
		status, approvedAt,
		strings.TrimSpace(r.FormValue("notes")),
		aqfLevel,
	); err != nil {
		log.Printf("UpdateVCCPQ(%d,%d): %v", user.PersonID, pqID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCPQCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("qualification_code"))
	title := strings.TrimSpace(r.FormValue("qualification_title"))
	if code == "" || title == "" {
		http.Error(w, `{"error":"code and title required"}`, http.StatusBadRequest)
		return
	}
	pq, err := h.store.CreateVCCPQ(r.Context(), user.PersonID, code, title,
		strings.TrimSpace(r.FormValue("institution")))
	if err != nil {
		log.Printf("CreateVCCPQ(%d): %v", user.PersonID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"code":%q,"title":%q,"status":%q}`, pq.ID, pq.Code, pq.Title, pq.Status)
}

func (h *Handler) VCCVocQuals(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	vcc, err := h.store.GetLatestVCCForTeacher(r.Context(), user.PersonID)
	if err == pgx.ErrNoRows {
		h.render(w, "vcc-vocquals", map[string]any{"VCC": nil, "User": user, "ItemStatuses": validVCCItemStatuses})
		return
	}
	if err != nil {
		log.Printf("GetLatestVCCForTeacher(%d): %v", user.PersonID, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "vcc-vocquals", map[string]any{
		"VCC": vcc, "User": user, "ItemStatuses": validVCCItemStatuses,
	})
}

func (h *Handler) VCCVocQualCreate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("qualification_code"))
	title := strings.TrimSpace(r.FormValue("qualification_title"))
	if code == "" || title == "" {
		http.Error(w, `{"error":"code and title required"}`, http.StatusBadRequest)
		return
	}
	vq, err := h.store.CreateVCCVocQual(r.Context(), user.PersonID, code, title,
		strings.TrimSpace(r.FormValue("institution")))
	if err != nil {
		log.Printf("CreateVCCVocQual(%d): %v", user.PersonID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"code":%q,"title":%q,"status":%q}`, vq.ID, vq.Code, vq.Title, vq.Status)
}

func (h *Handler) VCCVocQualUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	vqID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("qualification_code"))
	title := strings.TrimSpace(r.FormValue("qualification_title"))
	status := r.FormValue("status")
	if code == "" || title == "" || !containsStr(validVCCItemStatuses, status) {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	approvedAt := parseDateField(r.FormValue("approved_at"))
	aqfLevel, _ := strconv.Atoi(r.FormValue("aqf_level"))
	if err := h.store.UpdateVCCVocQual(r.Context(), user.PersonID, vqID,
		code, title,
		strings.TrimSpace(r.FormValue("institution")),
		status, approvedAt,
		strings.TrimSpace(r.FormValue("notes")),
		aqfLevel,
	); err != nil {
		log.Printf("UpdateVCCVocQual(%d,%d): %v", user.PersonID, vqID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCVocQualAddDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	vqID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	doc, err := h.store.CreateVocQualDocument(r.Context(), user.PersonID, vqID,
		title, strings.TrimSpace(r.FormValue("external_url")))
	if err != nil {
		log.Printf("CreateVocQualDocument(%d,%d): %v", user.PersonID, vqID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"title":%q,"external_url":%q}`, doc.ID, doc.Title, doc.ExternalURL)
}

func (h *Handler) VCCVocQualDeleteDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docID, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.store.DeletePQDocument(r.Context(), user.PersonID, docID); err != nil {
		log.Printf("DeleteVocQualDoc(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCPQAddDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	pqID, err := strconv.ParseInt(r.PathValue("pid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	doc, err := h.store.CreatePQDocument(r.Context(), user.PersonID, pqID,
		title, strings.TrimSpace(r.FormValue("external_url")))
	if err != nil {
		log.Printf("CreatePQDocument(%d,%d): %v", user.PersonID, pqID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"title":%q,"external_url":%q}`, doc.ID, doc.Title, doc.ExternalURL)
}

func (h *Handler) VCCPQDeleteDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docID, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.store.DeletePQDocument(r.Context(), user.PersonID, docID); err != nil {
		log.Printf("DeletePQDocument(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCElementAddDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	elemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	doc, err := h.store.CreateElementDocument(r.Context(), user.PersonID, elemID,
		title, strings.TrimSpace(r.FormValue("external_url")))
	if err != nil {
		log.Printf("CreateElementDocument(%d,%d): %v", user.PersonID, elemID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"title":%q,"external_url":%q}`, doc.ID, doc.Title, doc.ExternalURL)
}

func (h *Handler) VCCElementDeleteDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docID, err := strconv.ParseInt(r.PathValue("did"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.store.DeleteElementDocument(r.Context(), user.PersonID, docID); err != nil {
		log.Printf("DeleteElementDocument(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCDocumentLibrary(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docs, err := h.store.ListTeacherDocuments(r.Context(), user.PersonID)
	if err != nil {
		log.Printf("ListTeacherDocuments(%d): %v", user.PersonID, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.render(w, "vcc-document-library", map[string]any{
		"User": user,
		"Docs": docs,
		"Categories": []string{
			"Testamurs", "Accreditations", "Registrations",
			"Statement of attainment", "Transcripts",
			"Credentials", "Licenses", "Job cards", "Other",
		},
	})
}

func (h *Handler) VCCDocumentUpload(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	category := r.FormValue("category")
	if category == "" {
		category = "Other"
	}
	year, _ := strconv.Atoi(r.FormValue("year"))
	externalURL := strings.TrimSpace(r.FormValue("external_url"))

	var fileName, objectKey string
	if file, fh, ferr := r.FormFile("file"); ferr == nil {
		defer file.Close()
		fileName = fh.Filename
		ct := fh.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}
		objectKey = fmt.Sprintf("teacher/%d/%s_%s", user.PersonID, time.Now().Format("20060102150405"), fileName)
		if err := h.storage.Upload(r.Context(), objectKey, file, fh.Size, ct); err != nil {
			log.Printf("VCCDocumentUpload upload(%d): %v", user.PersonID, err)
			http.Error(w, `{"error":"upload failed"}`, http.StatusInternalServerError)
			return
		}
	}

	doc, err := h.store.CreateLibraryDocument(r.Context(), user.PersonID,
		title, category, year, fileName, objectKey, externalURL)
	if err != nil {
		if objectKey != "" {
			_ = h.storage.Delete(r.Context(), objectKey)
		}
		log.Printf("CreateLibraryDocument(%d): %v", user.PersonID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%d,"title":%q,"category":%q,"year":%d,"file_name":%q,"external_url":%q}`,
		doc.ID, doc.Title, doc.Category, doc.Year, doc.FileName, doc.ExternalURL)
}

func (h *Handler) VCCDocumentDelete(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	objectKey, err := h.store.GetTeacherDocumentObjectKey(r.Context(), user.PersonID, docID)
	if err != nil {
		log.Printf("GetTeacherDocumentObjectKey(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if objectKey != "" {
		if err := h.storage.Delete(r.Context(), objectKey); err != nil {
			log.Printf("VCCDocumentDelete storage(%d,%d): %v", user.PersonID, docID, err)
		}
	}
	if err := h.store.DeleteTeacherDocument(r.Context(), user.PersonID, docID); err != nil {
		log.Printf("DeleteTeacherDocument(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) VCCDocumentDownload(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	docID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	objectKey, err := h.store.GetTeacherDocumentObjectKey(r.Context(), user.PersonID, docID)
	if err != nil || objectKey == "" {
		http.NotFound(w, r)
		return
	}
	url, err := h.storage.PresignedURL(r.Context(), objectKey)
	if err != nil {
		log.Printf("VCCDocumentDownload presign(%d,%d): %v", user.PersonID, docID, err)
		http.Error(w, "could not generate download link", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func parseDateField(s string) pgtype.Date {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

// ── Admin / Infrastructure ─────────────────────────────────────────────────

func (h *Handler) AdminInfrastructure(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "admin-infra-menu", map[string]any{"User": user})
}

func (h *Handler) AssessmentMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "assessment-menu", map[string]any{"User": user})
}

func (h *Handler) SystemMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "system-menu", map[string]any{"User": user})
}

func (h *Handler) AdminInfraOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.store.ListTrainingOrgs(r.Context())
	if err != nil {
		log.Printf("ListTrainingOrgs: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-infra-orgs", map[string]any{
		"Orgs":   orgs,
		"States": auStates,
		"User":   user,
	})
}

func (h *Handler) AdminInfraLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := h.store.ListDeliveryLocationsFull(r.Context())
	if err != nil {
		log.Printf("ListDeliveryLocationsFull: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	orgs, err := h.store.ListTrainingOrgs(r.Context())
	if err != nil {
		log.Printf("ListTrainingOrgs: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-infra-locations", map[string]any{
		"Locations": locations,
		"Orgs":      orgs,
		"States":    auStates,
		"User":      user,
	})
}

func (h *Handler) AdminInfraBuildings(w http.ResponseWriter, r *http.Request) {
	buildings, err := h.store.ListBuildings(r.Context())
	if err != nil {
		log.Printf("ListBuildings: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	locations, err := h.store.ListDeliveryLocationsFull(r.Context())
	if err != nil {
		log.Printf("ListDeliveryLocationsFull: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-infra-buildings", map[string]any{
		"Buildings": buildings,
		"Locations": locations,
		"States":    auStates,
		"User":      user,
	})
}

func (h *Handler) AdminInfraRooms(w http.ResponseWriter, r *http.Request) {
	rooms, err := h.store.ListRooms(r.Context())
	if err != nil {
		log.Printf("ListRooms: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	buildings, err := h.store.ListBuildings(r.Context())
	if err != nil {
		log.Printf("ListBuildings: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-infra-rooms", map[string]any{
		"Rooms":     rooms,
		"Buildings": buildings,
		"RoomTypes": roomTypes,
		"User":      user,
	})
}

// ── Admin / Training Orgs ──────────────────────────────────────────────────

func (h *Handler) AdminOrgCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code       := strings.TrimSpace(r.FormValue("org_code"))
	name       := strings.TrimSpace(r.FormValue("org_name"))
	orgType    := strings.TrimSpace(r.FormValue("org_type"))
	address    := strings.TrimSpace(r.FormValue("address"))
	suburb     := strings.TrimSpace(r.FormValue("suburb"))
	stateCode  := strings.TrimSpace(r.FormValue("state_code"))
	postcode   := strings.TrimSpace(r.FormValue("postcode"))
	contact    := strings.TrimSpace(r.FormValue("contact_name"))
	telephone  := strings.TrimSpace(r.FormValue("telephone"))
	email      := strings.TrimSpace(r.FormValue("email"))

	if code == "" || name == "" || orgType == "" || address == "" || suburb == "" || stateCode == "" || postcode == "" {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateTrainingOrg(r.Context(), code, name, orgType, address, suburb, stateCode, postcode, contact, telephone, email)
	if err != nil {
		log.Printf("CreateTrainingOrg: %v", err)
		http.Error(w, `{"error":"could not save — org code may already be in use"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminOrgUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	code      := strings.TrimSpace(r.FormValue("org_code"))
	name      := strings.TrimSpace(r.FormValue("org_name"))
	orgType   := strings.TrimSpace(r.FormValue("org_type"))
	address   := strings.TrimSpace(r.FormValue("address"))
	suburb    := strings.TrimSpace(r.FormValue("suburb"))
	stateCode := strings.TrimSpace(r.FormValue("state_code"))
	postcode  := strings.TrimSpace(r.FormValue("postcode"))
	contact   := strings.TrimSpace(r.FormValue("contact_name"))
	telephone := strings.TrimSpace(r.FormValue("telephone"))
	email     := strings.TrimSpace(r.FormValue("email"))

	if code == "" || name == "" || orgType == "" || address == "" || suburb == "" || stateCode == "" || postcode == "" {
		http.Error(w, `{"error":"missing required fields"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateTrainingOrg(r.Context(), id, code, name, orgType, address, suburb, stateCode, postcode, contact, telephone, email); err != nil {
		log.Printf("UpdateTrainingOrg(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminOrgDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteTrainingOrg(r.Context(), id); err != nil {
		log.Printf("DeleteTrainingOrg(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ── Admin / Delivery Locations (full CRUD) ─────────────────────────────────

func (h *Handler) AdminLocCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	trainingOrgID := parseInt64(r.FormValue("training_org_id"))
	locID         := strings.TrimSpace(r.FormValue("delivery_loc_id"))
	name          := strings.TrimSpace(r.FormValue("name"))
	isVirtual     := r.FormValue("is_virtual") == "1"
	address       := strings.TrimSpace(r.FormValue("address"))
	suburb        := strings.TrimSpace(r.FormValue("suburb"))
	stateCode     := strings.TrimSpace(r.FormValue("state_code"))
	postcode      := strings.TrimSpace(r.FormValue("postcode"))
	lat           := strings.TrimSpace(r.FormValue("latitude"))
	lng           := strings.TrimSpace(r.FormValue("longitude"))

	if locID == "" || name == "" || trainingOrgID == 0 {
		http.Error(w, `{"error":"loc_id, name and training_org_id are required"}`, http.StatusBadRequest)
		return
	}
	if !isVirtual && (address == "" || suburb == "" || stateCode == "" || postcode == "") {
		http.Error(w, `{"error":"address fields are required for non-virtual locations"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateDeliveryLocation(r.Context(), trainingOrgID, locID, name, isVirtual, address, suburb, stateCode, postcode, lat, lng)
	if err != nil {
		log.Printf("CreateDeliveryLocation: %v", err)
		http.Error(w, `{"error":"could not save — loc ID may already be in use"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminLocUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	trainingOrgID := parseInt64(r.FormValue("training_org_id"))
	locID         := strings.TrimSpace(r.FormValue("delivery_loc_id"))
	name          := strings.TrimSpace(r.FormValue("name"))
	isVirtual     := r.FormValue("is_virtual") == "1"
	address       := strings.TrimSpace(r.FormValue("address"))
	suburb        := strings.TrimSpace(r.FormValue("suburb"))
	stateCode     := strings.TrimSpace(r.FormValue("state_code"))
	postcode      := strings.TrimSpace(r.FormValue("postcode"))
	lat           := strings.TrimSpace(r.FormValue("latitude"))
	lng           := strings.TrimSpace(r.FormValue("longitude"))

	if locID == "" || name == "" || trainingOrgID == 0 {
		http.Error(w, `{"error":"loc_id, name and training_org_id are required"}`, http.StatusBadRequest)
		return
	}
	if !isVirtual && (address == "" || suburb == "" || stateCode == "" || postcode == "") {
		http.Error(w, `{"error":"address fields are required for non-virtual locations"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateDeliveryLocation(r.Context(), id, trainingOrgID, locID, name, isVirtual, address, suburb, stateCode, postcode, lat, lng); err != nil {
		log.Printf("UpdateDeliveryLocation(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminLocDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteDeliveryLocation(r.Context(), id); err != nil {
		log.Printf("DeleteDeliveryLocation(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ── Admin / Buildings ──────────────────────────────────────────────────────

func (h *Handler) AdminBuildingCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	locationID := parseInt64(r.FormValue("delivery_location_id"))
	name       := strings.TrimSpace(r.FormValue("building_name"))
	if locationID == 0 || name == "" {
		http.Error(w, `{"error":"delivery_location_id and building_name are required"}`, http.StatusBadRequest)
		return
	}
	address   := strings.TrimSpace(r.FormValue("address"))
	suburb    := strings.TrimSpace(r.FormValue("suburb"))
	stateCode := strings.TrimSpace(r.FormValue("state_code"))
	postcode  := strings.TrimSpace(r.FormValue("postcode"))
	lat       := strings.TrimSpace(r.FormValue("latitude"))
	lng       := strings.TrimSpace(r.FormValue("longitude"))
	_, err := h.store.CreateBuilding(r.Context(), locationID, name, address, suburb, stateCode, postcode, lat, lng)
	if err != nil {
		log.Printf("CreateBuilding: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminBuildingUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	locationID := parseInt64(r.FormValue("delivery_location_id"))
	name       := strings.TrimSpace(r.FormValue("building_name"))
	if locationID == 0 || name == "" {
		http.Error(w, `{"error":"delivery_location_id and building_name are required"}`, http.StatusBadRequest)
		return
	}
	address   := strings.TrimSpace(r.FormValue("address"))
	suburb    := strings.TrimSpace(r.FormValue("suburb"))
	stateCode := strings.TrimSpace(r.FormValue("state_code"))
	postcode  := strings.TrimSpace(r.FormValue("postcode"))
	lat       := strings.TrimSpace(r.FormValue("latitude"))
	lng       := strings.TrimSpace(r.FormValue("longitude"))
	if err := h.store.UpdateBuilding(r.Context(), id, locationID, name, address, suburb, stateCode, postcode, lat, lng); err != nil {
		log.Printf("UpdateBuilding(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminBuildingDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteBuilding(r.Context(), id); err != nil {
		log.Printf("DeleteBuilding(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ── Admin / Rooms ──────────────────────────────────────────────────────────

func (h *Handler) AdminRoomCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	buildingID    := parseInt64(r.FormValue("building_id"))
	roomName      := strings.TrimSpace(r.FormValue("room_name"))
	roomType      := strings.TrimSpace(r.FormValue("room_type"))
	capacityStr   := strings.TrimSpace(r.FormValue("capacity"))
	isActive      := r.FormValue("is_active") == "1"
	isComputerLab := r.FormValue("is_computer_lab") == "1"

	capacity := parseInt(capacityStr)
	if buildingID == 0 || roomName == "" || roomType == "" || capacityStr == "" || capacity <= 0 {
		http.Error(w, `{"error":"building_id, room_name, room_type and capacity (>0) are required"}`, http.StatusBadRequest)
		return
	}
	_, err := h.store.CreateRoom(r.Context(), buildingID, roomName, roomType, capacity, isActive, isComputerLab)
	if err != nil {
		log.Printf("CreateRoom: %v", err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminRoomUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	buildingID    := parseInt64(r.FormValue("building_id"))
	roomName      := strings.TrimSpace(r.FormValue("room_name"))
	roomType      := strings.TrimSpace(r.FormValue("room_type"))
	capacityStr   := strings.TrimSpace(r.FormValue("capacity"))
	isActive      := r.FormValue("is_active") == "1"
	isComputerLab := r.FormValue("is_computer_lab") == "1"

	capacity := parseInt(capacityStr)
	if buildingID == 0 || roomName == "" || roomType == "" || capacityStr == "" || capacity <= 0 {
		http.Error(w, `{"error":"building_id, room_name, room_type and capacity (>0) are required"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateRoom(r.Context(), id, buildingID, roomName, roomType, capacity, isActive, isComputerLab); err != nil {
		log.Printf("UpdateRoom(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminRoomDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, `{"error":"bad id"}`, http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteRoom(r.Context(), id); err != nil {
		log.Printf("DeleteRoom(%d): %v", id, err)
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ── Course Enrollments ────────────────────────────────────────────────────────

func (h *Handler) AdminStudentSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}
	results, err := h.store.SearchStudents(r.Context(), q)
	if err != nil {
		log.Printf("SearchStudents: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []store.StudentSelectRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *Handler) AdminEnrollments(w http.ResponseWriter, r *http.Request) {
	enrollments, err := h.store.ListEnrollments(r.Context())
	if err != nil {
		log.Printf("ListEnrollments: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	programs, err := h.store.ListPrograms(r.Context())
	if err != nil {
		log.Printf("ListPrograms: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	groups, err := h.store.ListIntakeGroups(r.Context())
	if err != nil {
		log.Printf("ListIntakeGroups: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-enrollments", map[string]any{
		"User":        user,
		"Enrollments": enrollments,
		"Programs":    programs,
		"Groups":      groups,
	})
}

func (h *Handler) AdminEnrollmentCreate(w http.ResponseWriter, r *http.Request) {
	studentID, _ := strconv.ParseInt(r.FormValue("student_id"), 10, 64)
	programID, _ := strconv.ParseInt(r.FormValue("program_id"), 10, 64)
	intakeGroupID, _ := strconv.ParseInt(r.FormValue("intake_group_id"), 10, 64)
	status := r.FormValue("enrollment_status")
	if status == "" {
		status = "Active"
	}
	commencementDate := r.FormValue("commencement_date")
	completionDate := r.FormValue("completion_date")
	fundingStateCode := r.FormValue("funding_state_code")
	if fundingStateCode == "" {
		fundingStateCode = "VIC"
	}
	commencingProgramID := r.FormValue("commencing_program_id")
	if commencingProgramID == "" {
		commencingProgramID = "3"
	}
	if studentID == 0 || programID == 0 || commencementDate == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	if _, err := h.store.CreateEnrollment(r.Context(),
		studentID, programID, intakeGroupID,
		status, commencementDate, completionDate, fundingStateCode, commencingProgramID,
	); err != nil {
		log.Printf("CreateEnrollment: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminEnrollmentUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	intakeGroupID, _ := strconv.ParseInt(r.FormValue("intake_group_id"), 10, 64)
	if err := h.store.UpdateEnrollment(r.Context(),
		id, intakeGroupID,
		r.FormValue("enrollment_status"),
		r.FormValue("commencement_date"),
		r.FormValue("completion_date"),
		r.FormValue("funding_state_code"),
		r.FormValue("commencing_program_id"),
		r.FormValue("training_contract_id"),
		r.FormValue("client_apprenticeship_id"),
	); err != nil {
		log.Printf("UpdateEnrollment(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (h *Handler) AdminEnrollmentDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteEnrollment(r.Context(), id); err != nil {
		log.Printf("DeleteEnrollment(%d): %v", id, err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// ── System ────────────────────────────────────────────────────────────────────

func (h *Handler) SystemLMSConfig(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	name, url := h.lmsName, h.lmsURL
	h.mu.RUnlock()
	user, _ := auth.Current(r)
	h.render(w, "system-lms", map[string]any{
		"User":    user,
		"Name":    name,
		"URL":     url,
		"Saved":   r.URL.Query().Get("saved") == "1",
	})
}

func (h *Handler) SystemLMSSave(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("lms_name"))
	url  := strings.TrimSpace(r.FormValue("lms_url"))
	if err := h.store.SetSetting(r.Context(), "lms.name", name); err != nil {
		log.Printf("SetSetting lms.name: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	if err := h.store.SetSetting(r.Context(), "lms.url", url); err != nil {
		log.Printf("SetSetting lms.url: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.mu.Lock()
	h.lmsName = name
	h.lmsURL  = url
	h.mu.Unlock()
	http.Redirect(w, r, "/system/lms?saved=1", http.StatusSeeOther)
}
