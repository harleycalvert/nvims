package handler

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nvims-sms/internal/store"
)

type Handler struct {
	store *store.Store
	tmpl  *template.Template
}

func New(st *store.Store) *Handler {
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

	tmpl := template.Must(
		template.New("").Funcs(funcs).ParseFiles(
			"templates/index.html",
			"templates/partials/programs.html",
			"templates/partials/classes.html",
			"templates/partials/attendance.html",
		),
	)
	return &Handler{store: st, tmpl: tmpl}
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	periods, err := h.store.Periods(r.Context())
	if err != nil {
		log.Printf("Periods: %v", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	h.render(w, "index", map[string]any{"Periods": periods})
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

func (h *Handler) Classes(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.URL.Query().Get("period_id"), 10, 64)
	if err != nil || periodID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a program to see its classes.</p>`))
		return
	}
	programID, err := strconv.ParseInt(r.URL.Query().Get("program_id"), 10, 64)
	if err != nil || programID == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<p class="hint">Select a program to see its classes.</p>`))
		return
	}

	classes, err := h.store.ClassesForProgram(r.Context(), periodID, programID)
	if err != nil {
		log.Printf("ClassesForProgram(%d,%d): %v", periodID, programID, err)
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
