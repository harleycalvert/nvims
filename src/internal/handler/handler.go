package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nvims-sms/internal/auth"
	"nvims-sms/internal/store"
)

type Handler struct {
	store    *store.Store
	sessions *auth.Sessions
	tmpl     *template.Template
}

func New(st *store.Store, sessions *auth.Sessions) *Handler {
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
			case "Excused":
				return "att-excused"
			case "":
				return "att-none"
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
			"templates/menu.html",
			"templates/index.html",
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
		),
	)
	return &Handler{store: st, sessions: sessions, tmpl: tmpl}
}

func (h *Handler) Menu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "menu", map[string]any{"User": user})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	periods, err := h.store.Periods(r.Context())
	if err != nil {
		log.Printf("Periods: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "index", map[string]any{"Periods": periods, "User": user})
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
	valid := map[string]bool{"Present": true, "Online": true, "Absent-Notified": true, "Excused": true, "Absent": true, "": true}
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
	user, _ := auth.Current(r)

	var roleErr error
	switch roleType {
	case "student":
		roleErr = h.store.AddStudentRole(r.Context(), id, number, email)
	case "teacher":
		roleErr = h.store.AddTeacherRole(r.Context(), id, number, email,
			r.FormValue("employment_status"))
	case "staff":
		roleErr = h.store.AddStaffRole(r.Context(), id, number, email)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if roleErr != nil {
		log.Printf("AddRole(%s,%d): %v", roleType, id, roleErr)
		person, _ := h.store.GetPerson(r.Context(), id)
		role := store.RoleDetail{
			Number: number, Email: email,
			EmploymentStatus: r.FormValue("employment_status"),
		}
		h.render(w, "admin-role-form", map[string]any{
			"Person": person, "RoleType": roleType,
			"Role": role,
			"Error": "Could not add role — check that the number is not already in use.",
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := programFormFromPost(r)

	nominalHours, nhErr := strconv.Atoi(strings.TrimSpace(f.NominalHoursStr))
	if f.FacultyID == 0 || f.ProgramCode == "" || f.ProgramName == "" ||
		f.ProgramRecognitionID == "" || f.LevelOfEducation == "" ||
		f.FieldOfEducation == "" || nhErr != nil || nominalHours < 0 {
		programs, _ := h.store.ListPrograms(r.Context())
		faculties, _ := h.store.ListFaculties(r.Context())
		h.render(w, "admin-programs", map[string]any{
			"Programs":     programs,
			"Faculties":    faculties,
			"ProgramTypes": programTypes,
			"Form":         f,
			"Error":        "Please fill in all required fields.",
			"User":         user,
		})
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
		programs, _ := h.store.ListPrograms(r.Context())
		faculties, _ := h.store.ListFaculties(r.Context())
		h.render(w, "admin-programs", map[string]any{
			"Programs":     programs,
			"Faculties":    faculties,
			"ProgramTypes": programTypes,
			"Form":         f,
			"Error":        "Could not save — check the program code is not already in use.",
			"User":         user,
		})
		return
	}
	http.Redirect(w, r, "/admin/programs?saved=1", http.StatusSeeOther)
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
	subjects, err := h.store.ListSubjects(r.Context())
	if err != nil {
		log.Printf("ListSubjects: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-classes", map[string]any{
		"Classes":      classes,
		"Periods":      periods,
		"Locations":    locations,
		"IntakeGroups": intakeGroups,
		"Subjects":     subjects,
		"Form":         classForm{},
		"Error":        "",
		"Success":      r.URL.Query().Get("saved") == "1",
		"User":         user,
	})
}

func (h *Handler) AdminClassCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)

	f := classForm{
		ClassCode:          strings.TrimSpace(r.FormValue("class_code")),
		AcademicPeriodID:   parseInt64(r.FormValue("academic_period_id")),
		DeliveryLocationID: parseInt64(r.FormValue("delivery_location_id")),
		IntakeGroupID:      parseInt64(r.FormValue("intake_group_id")),
		EnrolmentCapStr:    strings.TrimSpace(r.FormValue("enrolment_cap")),
	}

	renderErr := func(msg string) {
		classes, _ := h.store.ListClasses(r.Context())
		periods, _ := h.store.Periods(r.Context())
		locations, _ := h.store.ListDeliveryLocations(r.Context())
		intakeGroups, _ := h.store.ListIntakeGroups(r.Context())
		subjects, _ := h.store.ListSubjects(r.Context())
		h.render(w, "admin-classes", map[string]any{
			"Classes":      classes,
			"Periods":      periods,
			"Locations":    locations,
			"IntakeGroups": intakeGroups,
			"Subjects":     subjects,
			"Form":         f,
			"Error":        msg,
			"User":         user,
		})
	}

	if f.ClassCode == "" || f.AcademicPeriodID == 0 || f.DeliveryLocationID == 0 {
		renderErr("Please fill in all required fields.")
		return
	}

	var intakeGroupID *int64
	if f.IntakeGroupID > 0 {
		intakeGroupID = &f.IntakeGroupID
	}

	var enrolmentCap *int
	if f.EnrolmentCapStr != "" {
		if v, err := strconv.Atoi(f.EnrolmentCapStr); err == nil && v > 0 {
			enrolmentCap = &v
		}
	}

	subjectIDStrs := r.Form["subject_ids"]
	var classSubjects []store.ClassSubject
	for _, idStr := range subjectIDStrs {
		sid := parseInt64(idStr)
		if sid == 0 {
			continue
		}
		label := strings.TrimSpace(r.FormValue(fmt.Sprintf("label_%d", sid)))
		if label == "" {
			label = strings.TrimSpace(r.FormValue(fmt.Sprintf("subj_name_%d", sid)))
		}
		classSubjects = append(classSubjects, store.ClassSubject{SubjectID: sid, SubjectLabel: label})
	}

	_, err := h.store.CreateClass(r.Context(),
		f.ClassCode, f.AcademicPeriodID, f.DeliveryLocationID,
		intakeGroupID, enrolmentCap, classSubjects)
	if err != nil {
		log.Printf("CreateClass: %v", err)
		renderErr("Could not save — check the class code is not already in use.")
		return
	}
	http.Redirect(w, r, "/admin/classes?saved=1", http.StatusSeeOther)
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := periodForm{
		PeriodCode: strings.TrimSpace(r.FormValue("period_code")),
		PeriodName: strings.TrimSpace(r.FormValue("period_name")),
		YearStr:    strings.TrimSpace(r.FormValue("year")),
		StartDate:  r.FormValue("start_date"),
		EndDate:    r.FormValue("end_date"),
		PeriodType: r.FormValue("period_type"),
		SeqNumStr:  strings.TrimSpace(r.FormValue("sequence_number")),
	}

	year, yearErr := strconv.Atoi(f.YearStr)
	if f.PeriodCode == "" || f.PeriodName == "" || yearErr != nil ||
		f.StartDate == "" || f.EndDate == "" || f.PeriodType == "" {
		periods, _ := h.store.ListPeriods(r.Context())
		h.render(w, "admin-periods", map[string]any{
			"Periods": periods, "PeriodTypes": periodTypes,
			"Form": f, "Error": "Please fill in all required fields.", "User": user,
		})
		return
	}

	var seqNum *int
	if f.SeqNumStr != "" {
		if v, err := strconv.Atoi(f.SeqNumStr); err == nil && v > 0 {
			seqNum = &v
		}
	}

	_, err := h.store.CreatePeriod(r.Context(), f.PeriodCode, f.PeriodName, year,
		f.StartDate, f.EndDate, f.PeriodType, seqNum)
	if err != nil {
		log.Printf("CreatePeriod: %v", err)
		periods, _ := h.store.ListPeriods(r.Context())
		h.render(w, "admin-periods", map[string]any{
			"Periods": periods, "PeriodTypes": periodTypes,
			"Form": f, "Error": "Could not save — check the period code is not already in use.", "User": user,
		})
		return
	}
	http.Redirect(w, r, "/admin/periods?saved=1", http.StatusSeeOther)
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
		f.TrainingOrgID, f.LocID, f.Name, f.Address,
		f.Suburb, f.StateCode, f.Postcode, f.PostcodeOverride)
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
		"Form":    intakeGroupForm{},
		"Error":   "",
		"Success": r.URL.Query().Get("saved") == "1",
		"User":    user,
	})
}

func (h *Handler) AdminIntakeGroupCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := intakeGroupForm{
		IntakeID:    parseInt64(r.FormValue("intake_id")),
		GroupCode:   strings.TrimSpace(r.FormValue("group_code")),
		GroupName:   strings.TrimSpace(r.FormValue("group_name")),
		CapacityStr: strings.TrimSpace(r.FormValue("capacity")),
		Notes:       strings.TrimSpace(r.FormValue("notes")),
	}

	renderErr := func(msg string) {
		groups, _ := h.store.ListIntakeGroups(r.Context())
		intakes, _ := h.store.ListProgramIntakes(r.Context())
		h.render(w, "admin-intake-groups", map[string]any{
			"Groups": groups, "Intakes": intakes,
			"Form": f, "Error": msg, "User": user,
		})
	}

	if f.IntakeID == 0 || f.GroupCode == "" || f.GroupName == "" {
		renderErr("Please fill in all required fields.")
		return
	}

	var capacity *int
	if f.CapacityStr != "" {
		if v, err := strconv.Atoi(f.CapacityStr); err == nil && v > 0 {
			capacity = &v
		}
	}

	_, err := h.store.CreateIntakeGroup(r.Context(), f.IntakeID, f.GroupCode, f.GroupName, capacity, f.Notes)
	if err != nil {
		log.Printf("CreateIntakeGroup: %v", err)
		renderErr("Could not save — check the group code is not already in use for this intake.")
		return
	}
	http.Redirect(w, r, "/admin/intake-groups?saved=1", http.StatusSeeOther)
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
	classID := parseInt64(r.URL.Query().Get("class_id"))

	classes, err := h.store.ListClasses(r.Context())
	if err != nil {
		log.Printf("ListClasses: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	var sessions []store.SessionListRow
	if classID > 0 {
		sessions, err = h.store.ListSessionsForClass(r.Context(), classID)
		if err != nil {
			log.Printf("ListSessionsForClass: %v", err)
		}
	}

	user, _ := auth.Current(r)
	h.render(w, "admin-sessions", map[string]any{
		"Classes":      classes,
		"Sessions":     sessions,
		"SelectedClass": classID,
		"SessionTypes": sessionTypes,
		"SForm":        sessionForm{ClassID: classID, SessionType: "Scheduled"},
		"GForm":        generateForm{ClassID: classID, SessionType: "Scheduled"},
		"Error":        "",
		"Success":      r.URL.Query().Get("saved") == "1",
		"Generated":    r.URL.Query().Get("generated"),
		"User":         user,
	})
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
		http.Redirect(w, r, fmt.Sprintf("/admin/sessions?class_id=%d&error=missing", classID), http.StatusSeeOther)
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	name := strings.TrimSpace(r.FormValue("faculty_name"))

	if name == "" {
		faculties, _ := h.store.ListFaculties(r.Context())
		h.render(w, "admin-faculties", map[string]any{
			"Faculties":   faculties,
			"Error":       "Faculty name is required.",
			"FacultyName": name,
			"User":        user,
		})
		return
	}
	_, err := h.store.CreateFaculty(r.Context(), name)
	if err != nil {
		log.Printf("CreateFaculty: %v", err)
		faculties, _ := h.store.ListFaculties(r.Context())
		h.render(w, "admin-faculties", map[string]any{
			"Faculties":   faculties,
			"Error":       "Could not save — check the name is not already in use.",
			"FacultyName": name,
			"User":        user,
		})
		return
	}
	http.Redirect(w, r, "/admin/faculties?saved=1", http.StatusSeeOther)
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
		"Form":     subjectForm{VetFlag: true, ModuleFlag: "N"},
		"Error":    "",
		"Success":  r.URL.Query().Get("saved") == "1",
		"User":     user,
	})
}

func (h *Handler) AdminSubjectCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	user, _ := auth.Current(r)
	f := subjectFormFromPost(r)

	if f.ModuleFlag != "Y" && f.ModuleFlag != "N" {
		f.ModuleFlag = "N"
	}

	if f.SubjectCode == "" || f.SubjectName == "" || f.FieldOfEducation == "" {
		subjects, _ := h.store.ListSubjects(r.Context())
		h.render(w, "admin-subjects", map[string]any{
			"Subjects": subjects,
			"Form":     f,
			"Error":    "Please fill in all required fields.",
			"User":     user,
		})
		return
	}

	var nominalHours *int
	if f.NominalHoursStr != "" {
		if v, err := strconv.Atoi(f.NominalHoursStr); err == nil && v > 0 {
			nominalHours = &v
		}
	}

	var creditPoints *int
	if f.CreditPointsStr != "" {
		if v, err := strconv.Atoi(f.CreditPointsStr); err == nil && v > 0 {
			creditPoints = &v
		}
	}

	_, err := h.store.CreateSubject(r.Context(),
		f.SubjectCode, f.SubjectName, f.ModuleFlag, f.FieldOfEducation,
		nominalHours, f.VetFlag, creditPoints)
	if err != nil {
		log.Printf("CreateSubject: %v", err)
		subjects, _ := h.store.ListSubjects(r.Context())
		h.render(w, "admin-subjects", map[string]any{
			"Subjects": subjects,
			"Form":     f,
			"Error":    "Could not save — check the subject code is not already in use.",
			"User":     user,
		})
		return
	}
	http.Redirect(w, r, "/admin/subjects?saved=1", http.StatusSeeOther)
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
