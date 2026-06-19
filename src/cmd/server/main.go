package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

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

	st := store.New(pool)

	if adminUser := os.Getenv("BOOTSTRAP_ADMIN_USER"); adminUser != "" {
		if adminPass := os.Getenv("BOOTSTRAP_ADMIN_PASS"); adminPass != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			if err != nil {
				log.Fatalf("bootstrap: hash password: %v", err)
			}
			created, err := st.BootstrapAdmin(context.Background(), adminUser, string(hash))
			if err != nil {
				log.Fatalf("bootstrap: %v", err)
			}
			if created {
				log.Printf("bootstrap: created admin user %q", adminUser)
			}
		}
	}

	sessions := auth.NewSessions()
	h := handler.New(st, sessions, stor)

	if err := h.LoadPermissions(context.Background()); err != nil {
		log.Printf("warning: could not load role permissions: %v", err)
	}

	protect := func(fn http.HandlerFunc) http.HandlerFunc {
		return sessions.Middleware(fn).ServeHTTP
	}

	requirePerm := func(p string, fn http.HandlerFunc) http.HandlerFunc {
		return sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, _ := auth.Current(r)
			if !h.HasPerm(user, p) {
				http.Error(w, "403 Forbidden — you do not have permission to access this page.", http.StatusForbidden)
				return
			}
			fn(w, r)
		})).ServeHTTP
	}

	P := requirePerm // alias for brevity in route table
	_ = P

	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.Login)
	mux.HandleFunc("POST /login", h.LoginPost)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.HandleFunc("GET /{$}", protect(h.Menu))

	// ── Backup ──────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /backup",          P("system:backup", h.BackupPage))
	mux.HandleFunc("POST /backup/sql",     P("system:backup", h.BackupSQL))
	mux.HandleFunc("POST /backup/json",    P("system:backup", h.BackupJSON))
	mux.HandleFunc("POST /backup/table",   P("system:backup", h.BackupTable))
	mux.HandleFunc("POST /backup/restore", P("system:backup", h.RestoreSQL))

	// ── Timetable / sessions (view) ──────────────────────────────────────────
	mux.HandleFunc("GET /timetable",       P("sessions:view", h.Timetable))
	mux.HandleFunc("GET /programs",        P("sessions:view", h.Programs))
	mux.HandleFunc("GET /groups",          P("sessions:view", h.Groups))
	mux.HandleFunc("GET /classes",         P("sessions:view", h.Classes))
	mux.HandleFunc("GET /tas",             P("sessions:view", h.TAFTas))

	// ── Attendance ───────────────────────────────────────────────────────────
	mux.HandleFunc("GET /register",           P("attendance:mark",  h.Register))
	mux.HandleFunc("GET /attendance",         P("attendance:view",  h.Attendance))
	mux.HandleFunc("GET /attendance/popup",   P("attendance:view",  h.AttendancePopup))
	mux.HandleFunc("POST /attendance",        P("attendance:mark",  h.SetAttendance))

	// ── Results & assessment ─────────────────────────────────────────────────
	mux.HandleFunc("GET /assessment",         P("results:view",   h.AssessmentMenu))
	mux.HandleFunc("GET /results",            P("results:view",   h.Results))
	mux.HandleFunc("GET /result/popup",       P("results:view",   h.ResultPopup))
	mux.HandleFunc("POST /result",            P("results:manage", h.SetResult))
	mux.HandleFunc("POST /result/publish",    P("results:manage", h.PublishResult))
	mux.HandleFunc("POST /result/publish-sc", P("results:manage", h.PublishSCColumn))

	// ── Student panel ────────────────────────────────────────────────────────
	mux.HandleFunc("GET /student/panel", P("student:panel", h.StudentPanel))

	// ── VCC ──────────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /vcc",                                            P("vcc:access", h.VCCMenu))
	mux.HandleFunc("GET /vcc/understand",                                 P("vcc:access", h.VCCUnderstand))
	mux.HandleFunc("GET /vcc/detail",                                     P("vcc:access", h.VCCIndex))
	mux.HandleFunc("GET /vcc/programs",                                   P("vcc:access", h.VCCPrograms))
	mux.HandleFunc("GET /vcc/documents",                                  P("vcc:access", h.VCCDocumentLibrary))
	mux.HandleFunc("GET /vcc/documents/{id}/download",                    P("vcc:access", h.VCCDocumentDownload))
	mux.HandleFunc("GET /vcc/professional-evidence",                      P("vcc:access", h.VCCProfessionalEvidence))
	mux.HandleFunc("GET /vcc/vocational-evidence",                        P("vcc:access", h.VCCVocationalEvidence))
	mux.HandleFunc("GET /vcc/vocational-qualifications",                  P("vcc:access", h.VCCVocQuals))
	mux.HandleFunc("GET /vcc/certifications",                             P("vcc:access", h.VCCCredentials))
	mux.HandleFunc("GET /vcc/vet-knowledge-currency",                     P("vcc:access", h.VCCVetKnowledge))
	mux.HandleFunc("GET /vcc/industry-currency",                          P("vcc:access", h.VCCIndustryCurrency))
	mux.HandleFunc("GET /vcc/publications",                               P("vcc:access", h.VCCListPublications))
	mux.HandleFunc("GET /vcc/publications/{pid}/download",                P("vcc:access", h.VCCDownloadPublication))
	mux.HandleFunc("POST /vcc/documents/upload",                          P("vcc:manage", h.VCCDocumentUpload))
	mux.HandleFunc("POST /vcc/documents/{id}/delete",                     P("vcc:manage", h.VCCDocumentDelete))
	mux.HandleFunc("POST /vcc/credentials",                               P("vcc:manage", h.VCCCredentialCreate))
	mux.HandleFunc("POST /vcc/credentials/{cid}",                         P("vcc:manage", h.VCCCredentialUpdate))
	mux.HandleFunc("POST /vcc/credentials/{cid}/delete",                  P("vcc:manage", h.VCCCredentialDelete))
	mux.HandleFunc("POST /vcc/credentials/{cid}/docs",                    P("vcc:manage", h.VCCCredentialAddDoc))
	mux.HandleFunc("POST /vcc/credentials/{cid}/docs/{did}/delete",       P("vcc:manage", h.VCCCredentialDeleteDoc))
	mux.HandleFunc("POST /vcc/ind-evidence",                              P("vcc:manage", h.VCCIndEvidenceCreate))
	mux.HandleFunc("POST /vcc/ind-evidence/{iid}",                        P("vcc:manage", h.VCCIndEvidenceUpdate))
	mux.HandleFunc("POST /vcc/ind-evidence/{iid}/delete",                 P("vcc:manage", h.VCCIndEvidenceDelete))
	mux.HandleFunc("POST /vcc/ind-evidence/{iid}/docs",                   P("vcc:manage", h.VCCIndEvidenceAddDoc))
	mux.HandleFunc("POST /vcc/ind-evidence/{iid}/docs/{did}/delete",      P("vcc:manage", h.VCCIndEvidenceDeleteDoc))
	mux.HandleFunc("POST /vcc/prof-evidence",                             P("vcc:manage", h.VCCProfEvidenceCreate))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}",                       P("vcc:manage", h.VCCProfEvidenceUpdate))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/delete",                P("vcc:manage", h.VCCProfEvidenceDelete))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/docs",                  P("vcc:manage", h.VCCProfEvidenceAddDoc))
	mux.HandleFunc("POST /vcc/prof-evidence/{pid}/docs/{did}/delete",     P("vcc:manage", h.VCCProfEvidenceDeleteDoc))
	mux.HandleFunc("POST /vcc/vocquals",                                  P("vcc:manage", h.VCCVocQualCreate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}",                            P("vcc:manage", h.VCCVocQualUpdate))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/delete",                     P("vcc:manage", h.VCCVocQualDelete))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs",                       P("vcc:manage", h.VCCVocQualAddDoc))
	mux.HandleFunc("POST /vcc/vocquals/{pid}/docs/{did}/delete",          P("vcc:manage", h.VCCVocQualDeleteDoc))
	mux.HandleFunc("POST /vcc/tccp/publish",                              P("vcc:manage", h.VCCPublishTCCP))
	mux.HandleFunc("POST /vcc/publications/{pid}/delete",                 P("vcc:manage", h.VCCDeletePublication))
	mux.HandleFunc("POST /vcc/subject-evidence",                          P("vcc:manage", h.VCCAddSubjectEvidence))
	mux.HandleFunc("POST /vcc/subject-evidence/{lid}/notes",              P("vcc:manage", h.VCCUpdateSubjectEvidence))
	mux.HandleFunc("POST /vcc/subject-evidence/{lid}/delete",             P("vcc:manage", h.VCCDeleteSubjectEvidence))
	mux.HandleFunc("POST /vcc/status",                                    P("vcc:manage", h.VCCUpdateStatus))
	mux.HandleFunc("POST /vcc/units/{uid}",                               P("vcc:manage", h.VCCUnitUpdate))
	mux.HandleFunc("POST /vcc/units/{uid}/rating",                        P("vcc:manage", h.VCCUnitRatingSave))
	mux.HandleFunc("POST /vcc/units/{uid}/elements/new",                  P("vcc:manage", h.VCCUnitElementCreate))
	mux.HandleFunc("POST /vcc/elements/{id}",                             P("vcc:manage", h.VCCUnitElementUpdate))
	mux.HandleFunc("POST /vcc/elements/{id}/delete",                      P("vcc:manage", h.VCCUnitElementDelete))
	mux.HandleFunc("POST /vcc/elements/{id}/docs",                        P("vcc:manage", h.VCCElementAddDoc))
	mux.HandleFunc("POST /vcc/elements/{id}/docs/{did}/delete",           P("vcc:manage", h.VCCElementDeleteDoc))
	mux.HandleFunc("POST /vcc/pqs",                                       P("vcc:manage", h.VCCPQCreate))
	mux.HandleFunc("POST /vcc/pqs/{pid}",                                 P("vcc:manage", h.VCCPQUpdate))
	mux.HandleFunc("POST /vcc/pqs/{pid}/delete",                          P("vcc:manage", h.VCCPQDelete))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs",                            P("vcc:manage", h.VCCPQAddDoc))
	mux.HandleFunc("POST /vcc/pqs/{pid}/docs/{did}/delete",               P("vcc:manage", h.VCCPQDeleteDoc))

	// ── Workplan ─────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /workplan",                                          P("workplan:view",   h.WorkplanMenu))
	mux.HandleFunc("GET /workplan/availability",                             P("workplan:view",   h.WorkplanAvailability))
	mux.HandleFunc("GET /workplan/teaching-delivery",                        P("workplan:view",   h.WorkplanTeachingDelivery))
	mux.HandleFunc("GET /workplan/settings",                                 P("workplan:view",   h.WorkplanSettings))
	mux.HandleFunc("POST /workplan/availability/{day}",                      P("workplan:manage", h.WorkplanAvailabilitySet))
	mux.HandleFunc("POST /workplan/availability/{day}/delete",               P("workplan:manage", h.WorkplanAvailabilityDelete))
	mux.HandleFunc("POST /workplan/availability/defaults",                   P("workplan:manage", h.WorkplanAvailabilitySetDefaults))
	mux.HandleFunc("POST /workplan/leave",                                   P("workplan:manage", h.WorkplanLeaveCreate))
	mux.HandleFunc("POST /workplan/leave/{id}/cancel",                       P("workplan:manage", h.WorkplanLeaveCancel))
	mux.HandleFunc("POST /workplan/td/class/{id}",                           P("workplan:manage", h.WorkplanTDClassSave))
	mux.HandleFunc("POST /workplan/td/class/{class_id}/subject/{subject_id}",P("workplan:manage", h.WorkplanTDSubjectSave))
	mux.HandleFunc("POST /workplan/settings",                                P("workplan:manage", h.WorkplanSettingsSave))

	// ── Admin — people ───────────────────────────────────────────────────────
	mux.HandleFunc("GET /admin",                                   protect(h.AdminMenu))
	mux.HandleFunc("GET /admin/people",                            P("people:view",   h.AdminPeople))
	mux.HandleFunc("GET /admin/people/new",                        P("people:manage", h.AdminPersonNew))
	mux.HandleFunc("POST /admin/people/new",                       P("people:manage", h.AdminPersonCreate))
	mux.HandleFunc("GET /admin/people/{id}",                       P("people:view",   h.AdminPersonView))
	mux.HandleFunc("POST /admin/people/{id}",                      P("people:manage", h.AdminPersonUpdate))
	mux.HandleFunc("GET /admin/people/{id}/role",                  P("people:manage", h.AdminRoleForm))
	mux.HandleFunc("POST /admin/people/{id}/role",                 P("people:manage", h.AdminRoleAdd))
	mux.HandleFunc("POST /admin/people/{id}/org-roles/new",        P("people:manage", h.AdminOrgRoleAdd))
	mux.HandleFunc("POST /admin/people/{id}/org-roles/{rid}",      P("people:manage", h.AdminOrgRoleUpdateEnd))
	mux.HandleFunc("POST /admin/people/{id}/org-roles/{rid}/delete",P("people:manage",h.AdminOrgRoleDelete))
	mux.HandleFunc("POST /admin/people/{id}/change-password",      P("people:manage", h.AdminPersonChangePassword))
	mux.HandleFunc("POST /admin/people/{id}/location-pref",        P("people:manage", h.AdminPersonLocationPrefSave))
	mux.HandleFunc("GET /admin/students/search",                   P("people:view",   h.AdminStudentSearch))

	// ── Admin — enrolments ───────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/enrollments",              P("enrolments:view",   h.AdminEnrollments))
	mux.HandleFunc("POST /admin/enrollments/new",         P("enrolments:manage", h.AdminEnrollmentCreate))
	mux.HandleFunc("POST /admin/enrollments/{id}",        P("enrolments:manage", h.AdminEnrollmentUpdate))
	mux.HandleFunc("POST /admin/enrollments/{id}/delete", P("enrolments:manage", h.AdminEnrollmentDelete))
	mux.HandleFunc("GET /admin/intakes",               P("enrolments:view",   h.AdminIntakes))
	mux.HandleFunc("POST /admin/intakes/new",          P("enrolments:manage", h.AdminIntakeCreate))
	mux.HandleFunc("POST /admin/intakes/{id}",         P("enrolments:manage", h.AdminIntakeUpdate))
	mux.HandleFunc("POST /admin/intakes/{id}/delete",  P("enrolments:manage", h.AdminIntakeDelete))
	mux.HandleFunc("GET /admin/intake-groups",              P("enrolments:view",   h.AdminIntakeGroups))
	mux.HandleFunc("POST /admin/intake-groups/new",         P("enrolments:manage", h.AdminIntakeGroupCreate))
	mux.HandleFunc("POST /admin/intake-groups/{id}",        P("enrolments:manage", h.AdminIntakeGroupUpdate))
	mux.HandleFunc("POST /admin/intake-groups/{id}/delete", P("enrolments:manage", h.AdminIntakeGroupDelete))

	// ── Admin — programs & subjects ──────────────────────────────────────────
	mux.HandleFunc("GET /admin/programs",              P("programs:view",   h.AdminPrograms))
	mux.HandleFunc("POST /admin/programs/new",         P("programs:manage", h.AdminProgramCreate))
	mux.HandleFunc("POST /admin/programs/{id}",        P("programs:manage", h.AdminProgramUpdate))
	mux.HandleFunc("POST /admin/programs/{id}/delete", P("programs:manage", h.AdminProgramDelete))
	mux.HandleFunc("GET /admin/subjects",              P("programs:view",   h.AdminSubjects))
	mux.HandleFunc("POST /admin/subjects/new",         P("programs:manage", h.AdminSubjectCreate))
	mux.HandleFunc("POST /admin/subjects/{id}",        P("programs:manage", h.AdminSubjectUpdate))
	mux.HandleFunc("POST /admin/subjects/{id}/delete", P("programs:manage", h.AdminSubjectDelete))

	// ── Admin — sessions & classes ───────────────────────────────────────────
	mux.HandleFunc("GET /admin/sessions",                              P("sessions:view",   h.AdminSessions))
	mux.HandleFunc("GET /admin/sessions/schedule",                     P("sessions:view",   h.AdminSessionSchedule))
	mux.HandleFunc("POST /admin/sessions/new",                         P("sessions:manage", h.AdminSessionCreate))
	mux.HandleFunc("POST /admin/sessions/generate",                    P("sessions:manage", h.AdminSessionsGenerate))
	mux.HandleFunc("POST /admin/sessions/{id}",                        P("sessions:manage", h.AdminSessionUpdate))
	mux.HandleFunc("POST /admin/sessions/{id}/delete",                 P("sessions:manage", h.AdminSessionDelete))
	mux.HandleFunc("GET /admin/classes",                               P("sessions:view",   h.AdminClasses))
	mux.HandleFunc("POST /admin/classes/new",                          P("sessions:manage", h.AdminClassCreate))
	mux.HandleFunc("POST /admin/classes/{id}",                         P("sessions:manage", h.AdminClassUpdate))
	mux.HandleFunc("POST /admin/classes/{id}/delete",                  P("sessions:manage", h.AdminClassDelete))
	mux.HandleFunc("GET /admin/periods",                               P("sessions:manage", h.AdminPeriods))
	mux.HandleFunc("POST /admin/periods/new",                          P("sessions:manage", h.AdminPeriodCreate))
	mux.HandleFunc("POST /admin/periods/{id}",                         P("sessions:manage", h.AdminPeriodUpdate))
	mux.HandleFunc("POST /admin/periods/{id}/delete",                  P("sessions:manage", h.AdminPeriodDelete))
	mux.HandleFunc("GET /admin/exceptions",                            P("sessions:manage", h.AdminExceptions))
	mux.HandleFunc("POST /admin/exceptions/rules/new",                 P("sessions:manage", h.AdminExceptionRuleCreate))
	mux.HandleFunc("POST /admin/exceptions/rules/{id}",                P("sessions:manage", h.AdminExceptionRuleUpdate))
	mux.HandleFunc("POST /admin/exceptions/rules/{id}/delete",         P("sessions:manage", h.AdminExceptionRuleDelete))
	mux.HandleFunc("POST /admin/exceptions/observances/new",           P("sessions:manage", h.AdminExceptionObservanceCreate))
	mux.HandleFunc("POST /admin/exceptions/observances/{id}/delete",   P("sessions:manage", h.AdminExceptionObservanceDelete))
	mux.HandleFunc("POST /admin/exceptions/generate",                  P("sessions:manage", h.AdminExceptionGenerate))

	// ── Admin — infrastructure ───────────────────────────────────────────────
	mux.HandleFunc("GET /admin/infrastructure",               P("infra:view",   h.AdminInfrastructure))
	mux.HandleFunc("GET /admin/infrastructure/orgs",          P("infra:view",   h.AdminInfraOrgs))
	mux.HandleFunc("GET /admin/infrastructure/locations",     P("infra:view",   h.AdminInfraLocations))
	mux.HandleFunc("GET /admin/infrastructure/buildings",     P("infra:view",   h.AdminInfraBuildings))
	mux.HandleFunc("GET /admin/infrastructure/rooms",         P("infra:view",   h.AdminInfraRooms))
	mux.HandleFunc("POST /admin/orgs/new",                    P("infra:manage", h.AdminOrgCreate))
	mux.HandleFunc("POST /admin/orgs/{id}",                   P("infra:manage", h.AdminOrgUpdate))
	mux.HandleFunc("POST /admin/orgs/{id}/delete",            P("infra:manage", h.AdminOrgDelete))
	mux.HandleFunc("POST /admin/locs/new",                    P("infra:manage", h.AdminLocCreate))
	mux.HandleFunc("POST /admin/locs/{id}",                   P("infra:manage", h.AdminLocUpdate))
	mux.HandleFunc("POST /admin/locs/{id}/delete",            P("infra:manage", h.AdminLocDelete))
	mux.HandleFunc("POST /admin/buildings/new",               P("infra:manage", h.AdminBuildingCreate))
	mux.HandleFunc("POST /admin/buildings/{id}",              P("infra:manage", h.AdminBuildingUpdate))
	mux.HandleFunc("POST /admin/buildings/{id}/delete",       P("infra:manage", h.AdminBuildingDelete))
	mux.HandleFunc("POST /admin/rooms/new",                   P("infra:manage", h.AdminRoomCreate))
	mux.HandleFunc("POST /admin/rooms/{id}",                  P("infra:manage", h.AdminRoomUpdate))
	mux.HandleFunc("POST /admin/rooms/{id}/delete",           P("infra:manage", h.AdminRoomDelete))
	mux.HandleFunc("GET /admin/rooms/{id}/issues",            P("infra:view",   h.AdminRoomIssuesList))
	mux.HandleFunc("POST /admin/rooms/{id}/issues/new",       P("infra:manage", h.AdminRoomIssueCreate))
	mux.HandleFunc("POST /admin/rooms/{id}/issues/{iid}",     P("infra:manage", h.AdminRoomIssueUpdate))
	mux.HandleFunc("POST /admin/rooms/{id}/issues/{iid}/delete",P("infra:manage",h.AdminRoomIssueDelete))
	mux.HandleFunc("GET /admin/rooms/{id}/lab-specs",           P("infra:view",   h.AdminRoomLabSpecsGet))
	mux.HandleFunc("POST /admin/rooms/{id}/lab-specs",          P("infra:manage", h.AdminRoomLabSpecsUpsert))
	mux.HandleFunc("GET /admin/rooms/lab-software/known",       P("infra:view",   h.AdminRoomLabSoftwareKnown))
	mux.HandleFunc("GET /admin/rooms/{id}/lab-software",        P("infra:view",   h.AdminRoomLabSoftwareList))
	mux.HandleFunc("POST /admin/rooms/{id}/lab-software/new",   P("infra:manage", h.AdminRoomLabSoftwareCreate))
	mux.HandleFunc("POST /admin/rooms/{id}/lab-software/{swid}",P("infra:manage", h.AdminRoomLabSoftwareUpdate))
	mux.HandleFunc("POST /admin/rooms/{id}/lab-software/{swid}/delete",P("infra:manage",h.AdminRoomLabSoftwareDelete))
	mux.HandleFunc("GET /admin/locations",      P("infra:view",   h.AdminLocations))
	mux.HandleFunc("POST /admin/locations/new", P("infra:manage", h.AdminLocationCreate))

	// ── Admin — system config ────────────────────────────────────────────────
	mux.HandleFunc("GET /admin/roles",                  P("system:config", h.AdminRoles))
	mux.HandleFunc("POST /admin/role-types/new",        P("system:config", h.AdminRoleTypeCreate))
	mux.HandleFunc("POST /admin/role-types/{id}",       P("system:config", h.AdminRoleTypeUpdate))
	mux.HandleFunc("POST /admin/role-types/{id}/delete",P("system:config", h.AdminRoleTypeDelete))
	mux.HandleFunc("GET /admin/faculties",              P("system:config", h.AdminFaculties))
	mux.HandleFunc("GET /admin/departments",            P("system:config", h.AdminDepartments))
	mux.HandleFunc("POST /admin/departments/new",       P("system:config", h.AdminDepartmentCreate))
	mux.HandleFunc("POST /admin/departments/{id}",      P("system:config", h.AdminDepartmentUpdate))
	mux.HandleFunc("POST /admin/departments/{id}/delete",P("system:config",h.AdminDepartmentDelete))
	mux.HandleFunc("POST /admin/faculties/new",         P("system:config", h.AdminFacultyCreate))
	mux.HandleFunc("POST /admin/faculties/{id}",        P("system:config", h.AdminFacultyUpdate))
	mux.HandleFunc("POST /admin/faculties/{id}/delete", P("system:config", h.AdminFacultyDelete))

	// ── System ───────────────────────────────────────────────────────────────
	mux.HandleFunc("GET /system",                        protect(h.SystemMenu))
	mux.HandleFunc("GET /system/users",                  P("system:users",  h.SystemUsers))
	mux.HandleFunc("GET /system/users/staff-search",     P("system:users",  h.SystemUsersStaffSearch))
	mux.HandleFunc("POST /system/users/new",             P("system:users",  h.SystemUserCreate))
	mux.HandleFunc("POST /system/users/{id}/revoke",     P("system:users",  h.SystemUserRevoke))
	mux.HandleFunc("GET /system/file-storage",           P("system:config", h.SystemFileStorage))
	mux.HandleFunc("GET /system/lms",                    P("system:config", h.SystemLMSConfig))
	mux.HandleFunc("POST /system/lms",                   P("system:config", h.SystemLMSSave))
	mux.HandleFunc("GET /system/permissions",            P("system:config", h.SystemPermissions))
	mux.HandleFunc("POST /system/permissions",           P("system:config", h.SystemPermissionsSave))

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("GET /img/", http.StripPrefix("/img/", http.FileServer(http.Dir("img"))))

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
