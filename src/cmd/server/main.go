package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"nvims/internal/auth"
	"nvims/internal/handler"
	"nvims/internal/storage"
	"nvims/internal/store"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
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

	minioEndpoint := os.Getenv("MINIO_ENDPOINT")
	if minioEndpoint == "" {
		minioEndpoint = "localhost:9000"
	}
	minioBucket := os.Getenv("MINIO_BUCKET")
	if minioBucket == "" {
		minioBucket = "nvims-docs"
	}
	stor, err := storage.New(minioEndpoint, os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD"), minioBucket)
	if err != nil {
		log.Fatalf("storage client: %v", err)
	}
	if err := stor.EnsureBucket(context.Background()); err != nil {
		log.Fatalf("storage bucket: %v", err)
	}

	sessions := auth.NewSessions()
	st := store.New(pool)
	h := handler.New(st, sessions, stor)

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
	mux.HandleFunc("GET /admin/departments", protect(h.AdminDepartments))
	mux.HandleFunc("POST /admin/departments/new", protect(h.AdminDepartmentCreate))
	mux.HandleFunc("POST /admin/departments/{id}", protect(h.AdminDepartmentUpdate))
	mux.HandleFunc("POST /admin/departments/{id}/delete", protect(h.AdminDepartmentDelete))
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
	mux.HandleFunc("GET /assessment", protect(h.AssessmentMenu))
	mux.HandleFunc("GET /system", protect(h.SystemMenu))
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
	mux.HandleFunc("GET /admin/students/search", protect(h.AdminStudentSearch))
	mux.HandleFunc("GET /admin/enrollments", protect(h.AdminEnrollments))
	mux.HandleFunc("POST /admin/enrollments/new", protect(h.AdminEnrollmentCreate))
	mux.HandleFunc("POST /admin/enrollments/{id}", protect(h.AdminEnrollmentUpdate))
	mux.HandleFunc("POST /admin/enrollments/{id}/delete", protect(h.AdminEnrollmentDelete))
	mux.HandleFunc("GET /admin/sessions", protect(h.AdminSessions))
	mux.HandleFunc("GET /admin/sessions/schedule", protect(h.AdminSessionSchedule))
	mux.HandleFunc("POST /admin/sessions/new", protect(h.AdminSessionCreate))
	mux.HandleFunc("POST /admin/sessions/generate", protect(h.AdminSessionsGenerate))
	mux.HandleFunc("POST /admin/sessions/{id}", protect(h.AdminSessionUpdate))
	mux.HandleFunc("POST /admin/sessions/{id}/delete", protect(h.AdminSessionDelete))
	mux.HandleFunc("GET /vcc/documents", protect(h.VCCDocumentLibrary))
	mux.HandleFunc("POST /vcc/documents/upload", protect(h.VCCDocumentUpload))
	mux.HandleFunc("GET /vcc/documents/{id}/download", protect(h.VCCDocumentDownload))
	mux.HandleFunc("POST /vcc/documents/{id}/delete", protect(h.VCCDocumentDelete))
	mux.HandleFunc("GET /workplan", protect(h.WorkplanMenu))
	mux.HandleFunc("GET /workplan/availability", protect(h.WorkplanAvailability))
	mux.HandleFunc("POST /workplan/availability/{day}", protect(h.WorkplanAvailabilitySet))
	mux.HandleFunc("POST /workplan/availability/{day}/delete", protect(h.WorkplanAvailabilityDelete))
	mux.HandleFunc("POST /workplan/availability/defaults", protect(h.WorkplanAvailabilitySetDefaults))
	mux.HandleFunc("POST /workplan/leave", protect(h.WorkplanLeaveCreate))
	mux.HandleFunc("POST /workplan/leave/{id}/cancel", protect(h.WorkplanLeaveCancel))
	mux.HandleFunc("GET /vcc", protect(h.VCCMenu))
	mux.HandleFunc("GET /vcc/professional-evidence", protect(h.VCCProfessionalEvidence))
	mux.HandleFunc("GET /vcc/vocational-evidence", protect(h.VCCVocationalEvidence))
	mux.HandleFunc("GET /vcc/vocational-qualifications", protect(h.VCCVocQuals))
	mux.HandleFunc("GET /vcc/certifications", protect(h.VCCCredentials))
	mux.HandleFunc("POST /vcc/credentials", protect(h.VCCCredentialCreate))
	mux.HandleFunc("POST /vcc/credentials/{cid}", protect(h.VCCCredentialUpdate))
	mux.HandleFunc("POST /vcc/credentials/{cid}/delete", protect(h.VCCCredentialDelete))
	mux.HandleFunc("POST /vcc/credentials/{cid}/docs", protect(h.VCCCredentialAddDoc))
	mux.HandleFunc("POST /vcc/credentials/{cid}/docs/{did}/delete", protect(h.VCCCredentialDeleteDoc))
	mux.HandleFunc("GET /vcc/vet-knowledge-currency", protect(h.VCCVetKnowledge))
	mux.HandleFunc("POST /vcc/prof-evidence", protect(h.VCCProfEvidenceCreate))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}", protect(h.VCCProfEvidenceUpdate))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/delete", protect(h.VCCProfEvidenceDelete))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/docs", protect(h.VCCProfEvidenceAddDoc))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/docs/{did}/delete", protect(h.VCCProfEvidenceDeleteDoc))
	mux.HandleFunc("POST /vcc/vocquals", protect(h.VCCVocQualCreate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}", protect(h.VCCVocQualUpdate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/delete", protect(h.VCCVocQualDelete))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs", protect(h.VCCVocQualAddDoc))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs/{did}/delete", protect(h.VCCVocQualDeleteDoc))
	mux.HandleFunc("GET /vcc/detail", protect(h.VCCIndex))
	mux.HandleFunc("POST /vcc/status", protect(h.VCCUpdateStatus))
	mux.HandleFunc("POST /vcc/units/{uid}", protect(h.VCCUnitUpdate))
	mux.HandleFunc("POST /vcc/units/{uid}/rating", protect(h.VCCUnitRatingSave))
	mux.HandleFunc("POST /vcc/units/{uid}/elements/new", protect(h.VCCUnitElementCreate))
	mux.HandleFunc("POST /vcc/elements/{id}", protect(h.VCCUnitElementUpdate))
	mux.HandleFunc("POST /vcc/elements/{id}/delete", protect(h.VCCUnitElementDelete))
	mux.HandleFunc("POST /vcc/elements/{id}/docs", protect(h.VCCElementAddDoc))
	mux.HandleFunc("POST /vcc/elements/{id}/docs/{did}/delete", protect(h.VCCElementDeleteDoc))
	mux.HandleFunc("POST /vcc/pqs", protect(h.VCCPQCreate))
	mux.HandleFunc("POST /vcc/pqs/{pid}", protect(h.VCCPQUpdate))
	mux.HandleFunc("POST /vcc/pqs/{pid}/delete", protect(h.VCCPQDelete))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs", protect(h.VCCPQAddDoc))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs/{did}/delete", protect(h.VCCPQDeleteDoc))
	mux.HandleFunc("GET /student/panel", protect(h.StudentPanel))
	mux.HandleFunc("GET /results", protect(h.Results))
	mux.HandleFunc("GET /result/popup", protect(h.ResultPopup))
	mux.HandleFunc("POST /result", protect(h.SetResult))
	mux.HandleFunc("POST /result/publish", protect(h.PublishResult))
	mux.HandleFunc("POST /result/publish-sc", protect(h.PublishSCColumn))
	mux.HandleFunc("GET /system/lms", protect(h.SystemLMSConfig))
	mux.HandleFunc("POST /system/lms", protect(h.SystemLMSSave))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /img/", http.StripPrefix("/img/", http.FileServer(http.Dir("img"))))

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
