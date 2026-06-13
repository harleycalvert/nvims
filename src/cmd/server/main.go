package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"nvims/internal/auth"
	"nvims/internal/handler"
	"nvims/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://nvims:jjnhbFC56RDWRTJHBjhb98uibe@localhost:5432/nvims"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping: %v", err)
	}
	log.Println("database connected")

	sessions := auth.NewSessions()
	st := store.New(pool)
	h := handler.New(st, sessions)

	protect := func(fn http.HandlerFunc) http.HandlerFunc {
		return sessions.Middleware(fn).ServeHTTP
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.Login)
	mux.HandleFunc("POST /login", h.LoginPost)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("GET /{$}", protect(h.Menu))
	mux.HandleFunc("GET /backup", protect(h.BackupPage))
	mux.HandleFunc("POST /backup/sql", protect(h.BackupSQL))
	mux.HandleFunc("POST /backup/json", protect(h.BackupJSON))
	mux.HandleFunc("POST /backup/table", protect(h.BackupTable))
	mux.HandleFunc("GET /register", protect(h.Register))
	mux.HandleFunc("GET /programs", protect(h.Programs))
	mux.HandleFunc("GET /groups", protect(h.Groups))
	mux.HandleFunc("GET /classes", protect(h.Classes))
	mux.HandleFunc("GET /timetable", protect(h.Timetable))
	mux.HandleFunc("GET /attendance", protect(h.Attendance))
	mux.HandleFunc("GET /attendance/popup", protect(h.AttendancePopup))
	mux.HandleFunc("POST /attendance", protect(h.SetAttendance))
	mux.HandleFunc("GET /admin", protect(h.AdminMenu))
	mux.HandleFunc("GET /admin/people", protect(h.AdminPeople))
	mux.HandleFunc("GET /admin/people/new", protect(h.AdminPersonNew))
	mux.HandleFunc("POST /admin/people/new", protect(h.AdminPersonCreate))
	mux.HandleFunc("GET /admin/people/{id}", protect(h.AdminPersonView))
	mux.HandleFunc("POST /admin/people/{id}", protect(h.AdminPersonUpdate))
	mux.HandleFunc("GET /admin/people/{id}/role", protect(h.AdminRoleForm))
	mux.HandleFunc("POST /admin/people/{id}/role", protect(h.AdminRoleAdd))
	mux.HandleFunc("GET /admin/programs", protect(h.AdminPrograms))
	mux.HandleFunc("POST /admin/programs/new", protect(h.AdminProgramCreate))
	mux.HandleFunc("POST /admin/programs/{id}", protect(h.AdminProgramUpdate))
	mux.HandleFunc("POST /admin/programs/{id}/delete", protect(h.AdminProgramDelete))
	mux.HandleFunc("GET /admin/classes", protect(h.AdminClasses))
	mux.HandleFunc("POST /admin/classes/new", protect(h.AdminClassCreate))
	mux.HandleFunc("POST /admin/classes/{id}", protect(h.AdminClassUpdate))
	mux.HandleFunc("POST /admin/classes/{id}/delete", protect(h.AdminClassDelete))
	mux.HandleFunc("GET /admin/faculties", protect(h.AdminFaculties))
	mux.HandleFunc("POST /admin/faculties/new", protect(h.AdminFacultyCreate))
	mux.HandleFunc("POST /admin/faculties/{id}", protect(h.AdminFacultyUpdate))
	mux.HandleFunc("POST /admin/faculties/{id}/delete", protect(h.AdminFacultyDelete))
	mux.HandleFunc("GET /admin/subjects", protect(h.AdminSubjects))
	mux.HandleFunc("POST /admin/subjects/new", protect(h.AdminSubjectCreate))
	mux.HandleFunc("POST /admin/subjects/{id}", protect(h.AdminSubjectUpdate))
	mux.HandleFunc("POST /admin/subjects/{id}/delete", protect(h.AdminSubjectDelete))
	mux.HandleFunc("GET /admin/periods", protect(h.AdminPeriods))
	mux.HandleFunc("POST /admin/periods/new", protect(h.AdminPeriodCreate))
	mux.HandleFunc("POST /admin/periods/{id}", protect(h.AdminPeriodUpdate))
	mux.HandleFunc("POST /admin/periods/{id}/delete", protect(h.AdminPeriodDelete))
	mux.HandleFunc("GET /admin/infrastructure", protect(h.AdminInfrastructure))
	mux.HandleFunc("GET /admin/infrastructure/orgs", protect(h.AdminInfraOrgs))
	mux.HandleFunc("GET /admin/infrastructure/locations", protect(h.AdminInfraLocations))
	mux.HandleFunc("GET /admin/infrastructure/buildings", protect(h.AdminInfraBuildings))
	mux.HandleFunc("GET /admin/infrastructure/rooms", protect(h.AdminInfraRooms))
	mux.HandleFunc("POST /admin/orgs/new", protect(h.AdminOrgCreate))
	mux.HandleFunc("POST /admin/orgs/{id}", protect(h.AdminOrgUpdate))
	mux.HandleFunc("POST /admin/orgs/{id}/delete", protect(h.AdminOrgDelete))
	mux.HandleFunc("POST /admin/locs/new", protect(h.AdminLocCreate))
	mux.HandleFunc("POST /admin/locs/{id}", protect(h.AdminLocUpdate))
	mux.HandleFunc("POST /admin/locs/{id}/delete", protect(h.AdminLocDelete))
	mux.HandleFunc("POST /admin/buildings/new", protect(h.AdminBuildingCreate))
	mux.HandleFunc("POST /admin/buildings/{id}", protect(h.AdminBuildingUpdate))
	mux.HandleFunc("POST /admin/buildings/{id}/delete", protect(h.AdminBuildingDelete))
	mux.HandleFunc("POST /admin/rooms/new", protect(h.AdminRoomCreate))
	mux.HandleFunc("POST /admin/rooms/{id}", protect(h.AdminRoomUpdate))
	mux.HandleFunc("POST /admin/rooms/{id}/delete", protect(h.AdminRoomDelete))
	mux.HandleFunc("GET /admin/locations", protect(h.AdminLocations))
	mux.HandleFunc("POST /admin/locations/new", protect(h.AdminLocationCreate))
	mux.HandleFunc("GET /admin/intake-groups", protect(h.AdminIntakeGroups))
	mux.HandleFunc("POST /admin/intake-groups/new", protect(h.AdminIntakeGroupCreate))
	mux.HandleFunc("POST /admin/intake-groups/{id}", protect(h.AdminIntakeGroupUpdate))
	mux.HandleFunc("POST /admin/intake-groups/{id}/delete", protect(h.AdminIntakeGroupDelete))
	mux.HandleFunc("GET /admin/sessions", protect(h.AdminSessions))
	mux.HandleFunc("GET /admin/sessions/schedule", protect(h.AdminSessionSchedule))
	mux.HandleFunc("POST /admin/sessions/new", protect(h.AdminSessionCreate))
	mux.HandleFunc("POST /admin/sessions/generate", protect(h.AdminSessionsGenerate))
	mux.HandleFunc("POST /admin/sessions/{id}", protect(h.AdminSessionUpdate))
	mux.HandleFunc("POST /admin/sessions/{id}/delete", protect(h.AdminSessionDelete))
	mux.HandleFunc("GET /vcc", protect(h.VCCMenu))
	mux.HandleFunc("GET /vcc/vocational-evidence", protect(h.VCCVocationalEvidence))
	mux.HandleFunc("GET /vcc/vocational-qualifications", protect(h.VCCVocQuals))
	mux.HandleFunc("POST /vcc/vocquals", protect(h.VCCVocQualCreate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}", protect(h.VCCVocQualUpdate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs", protect(h.VCCVocQualAddDoc))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs/{did}/delete", protect(h.VCCVocQualDeleteDoc))
	mux.HandleFunc("GET /vcc/detail", protect(h.VCCIndex))
	mux.HandleFunc("POST /vcc/status", protect(h.VCCUpdateStatus))
	mux.HandleFunc("POST /vcc/units/{uid}", protect(h.VCCUnitUpdate))
	mux.HandleFunc("POST /vcc/pqs", protect(h.VCCPQCreate))
	mux.HandleFunc("POST /vcc/pqs/{pid}", protect(h.VCCPQUpdate))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs", protect(h.VCCPQAddDoc))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs/{did}/delete", protect(h.VCCPQDeleteDoc))
	mux.HandleFunc("GET /student/panel", protect(h.StudentPanel))
	mux.HandleFunc("GET /results", protect(h.Results))
	mux.HandleFunc("GET /result/popup", protect(h.ResultPopup))
	mux.HandleFunc("POST /result", protect(h.SetResult))
	mux.HandleFunc("POST /result/publish", protect(h.PublishResult))
	mux.HandleFunc("POST /result/publish-sc", protect(h.PublishSCColumn))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /img/", http.StripPrefix("/img/", http.FileServer(http.Dir("img"))))

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
