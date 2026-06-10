package handler

import (
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
	h.sessions.Create(w, auth.User{ID: u.ID, Username: u.Username, FullName: u.FullName, Role: u.Role})
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
	programID, err2 := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	groupCode := r.URL.Query().Get("group_code")
	if err != nil || err2 != nil || periodID == 0 || programID == 0 || groupCode == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a group to see its classes.</p>`))
		return
	}

	classes, err := h.store.ClassesForGroup(r.Context(), periodID, programID, groupCode)
	if err != nil {
		log.Printf("ClassesForGroup(%d,%d,%s): %v", periodID, programID, groupCode, err)
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
	if result != "SC" && result != "NS" && result != "" {
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
	colClassID, _ := strconv.ParseInt(r.FormValue("col_class_id"), 10, 64)
	colSubjectID, _ := strconv.ParseInt(r.FormValue("col_subject_id"), 10, 64)
	if colClassID > 0 && colSubjectID > 0 {
		if err := h.store.PublishSCColumn(r.Context(), colClassID, colSubjectID); err != nil {
			log.Printf("PublishSCColumn(%d,%d): %v", colClassID, colSubjectID, err)
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
	ID            int64
	Title         string
	FirstName     string
	FamilyName    string
	PreferredName string
	DOBStr        string
	Gender        string
	Email         string
	PhoneMobile   string
	Suburb        string
	StateCode     string
	Postcode      string
}

func personFormFrom(d store.PersonDetail) personForm {
	return personForm{
		ID: d.ID, Title: d.Title, FirstName: d.FirstName, FamilyName: d.FamilyName,
		PreferredName: d.PreferredName, DOBStr: d.DOB.Format("2006-01-02"),
		Gender: d.Gender, Email: d.Email, PhoneMobile: d.PhoneMobile,
		Suburb: d.Suburb, StateCode: d.StateCode, Postcode: d.Postcode,
	}
}

func personFormFromPost(r *http.Request, id int64) personForm {
	return personForm{
		ID: id, Title: r.FormValue("title"),
		FirstName: strings.TrimSpace(r.FormValue("first_name")),
		FamilyName: strings.TrimSpace(r.FormValue("family_name")),
		PreferredName: strings.TrimSpace(r.FormValue("preferred_name")),
		DOBStr: r.FormValue("dob"), Gender: r.FormValue("gender"),
		Email: strings.TrimSpace(r.FormValue("email")),
		PhoneMobile: strings.TrimSpace(r.FormValue("phone_mobile")),
		Suburb: strings.TrimSpace(r.FormValue("suburb")),
		StateCode: r.FormValue("state_code"),
		Postcode: strings.TrimSpace(r.FormValue("postcode")),
	}
}

func (h *Handler) AdminMenu(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.Current(r)
	h.render(w, "admin-menu", map[string]any{"User": user})
}

func (h *Handler) AdminPeople(w http.ResponseWriter, r *http.Request) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	people, err := h.store.ListPeople(r.Context(), search)
	if err != nil {
		log.Printf("ListPeople: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	user, _ := auth.Current(r)
	h.render(w, "admin-people", map[string]any{"People": people, "Search": search, "User": user})
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
		f.Suburb, f.StateCode, f.Postcode)
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
		f.Suburb, f.StateCode, f.Postcode); err != nil {
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
	user, _ := auth.Current(r)
	h.render(w, "admin-role-form", map[string]any{
		"Person": person, "RoleType": r.URL.Query().Get("type"),
		"Error": "", "User": user,
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
		roleErr = h.store.AddTeacherRole(r.Context(), id, number, email, r.FormValue("employment_status"))
	case "staff":
		roleErr = h.store.AddStaffRole(r.Context(), id, number, email)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if roleErr != nil {
		log.Printf("AddRole(%s,%d): %v", roleType, id, roleErr)
		person, _ := h.store.GetPerson(r.Context(), id)
		h.render(w, "admin-role-form", map[string]any{
			"Person": person, "RoleType": roleType,
			"Error": "Could not add role — check that the number is not already in use.",
			"User": user,
		})
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/people/%d?saved=1", id), http.StatusSeeOther)
}

func (h *Handler) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %q: %v", name, err)
	}
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
