// Package perm defines the permission constants used for RBAC across NVIMS.
// Permissions are stored as "area:action" strings. Role→permission mappings
// are persisted in the role_permissions table and cached in memory.
package perm

// Permission constants.
const (
	PeopleView   = "people:view"
	PeopleManage = "people:manage"

	EnrolmentsView   = "enrolments:view"
	EnrolmentsManage = "enrolments:manage"

	ProgramsView   = "programs:view"
	ProgramsManage = "programs:manage"

	SessionsView   = "sessions:view"
	SessionsManage = "sessions:manage"

	AttendanceView = "attendance:view"
	AttendanceMark = "attendance:mark"

	ResultsView   = "results:view"
	ResultsManage = "results:manage"

	InfraView   = "infra:view"
	InfraManage = "infra:manage"

	VCCAccess = "vcc:access"
	VCCManage = "vcc:manage"

	WorkplanView   = "workplan:view"
	WorkplanManage = "workplan:manage"

	StudentPanel = "student:panel"

	SystemUsers  = "system:users"
	SystemBackup = "system:backup"
	SystemConfig = "system:config"
)

// Group collects permissions for display in the UI.
type Group struct {
	Name        string
	Permissions []Def
}

// Def describes a single permission for the UI.
type Def struct {
	Key         string
	Description string
}

// Groups is the ordered list of permission groups shown in the management UI.
var Groups = []Group{
	{
		Name: "People",
		Permissions: []Def{
			{PeopleView, "View person records and search"},
			{PeopleManage, "Create and edit person records"},
		},
	},
	{
		Name: "Enrolments",
		Permissions: []Def{
			{EnrolmentsView, "View student enrolments"},
			{EnrolmentsManage, "Create and edit enrolments"},
		},
	},
	{
		Name: "Programs & Subjects",
		Permissions: []Def{
			{ProgramsView, "View programs and subjects"},
			{ProgramsManage, "Create and edit programs and subjects"},
		},
	},
	{
		Name: "Sessions & Classes",
		Permissions: []Def{
			{SessionsView, "View sessions, classes and timetable"},
			{SessionsManage, "Create and edit sessions, classes, periods and exceptions"},
		},
	},
	{
		Name: "Attendance",
		Permissions: []Def{
			{AttendanceView, "View attendance records"},
			{AttendanceMark, "Record and update attendance"},
		},
	},
	{
		Name: "Results & Assessment",
		Permissions: []Def{
			{ResultsView, "View assessment results"},
			{ResultsManage, "Enter and publish results"},
		},
	},
	{
		Name: "Infrastructure",
		Permissions: []Def{
			{InfraView, "View locations, buildings, rooms and organisations"},
			{InfraManage, "Create and edit infrastructure records"},
		},
	},
	{
		Name: "VCC",
		Permissions: []Def{
			{VCCAccess, "Access vocational currency and competency records"},
			{VCCManage, "Edit VCC content and evidence"},
		},
	},
	{
		Name: "Workplan",
		Permissions: []Def{
			{WorkplanView, "View workplan and availability"},
			{WorkplanManage, "Edit workplan, availability and leave"},
		},
	},
	{
		Name: "Student",
		Permissions: []Def{
			{StudentPanel, "Access the student panel view"},
		},
	},
	{
		Name: "System",
		Permissions: []Def{
			{SystemUsers, "Manage system user accounts"},
			{SystemBackup, "Database backup and restore"},
			{SystemConfig, "System configuration (LMS, role types, departments, etc.)"},
		},
	},
}

// All returns every defined permission key in order.
func All() []string {
	var out []string
	for _, g := range Groups {
		for _, p := range g.Permissions {
			out = append(out, p.Key)
		}
	}
	return out
}

// Roles is the canonical ordered list of assignable roles.
var Roles = []string{
	"Admin", "Teacher", "Compliance", "Reception", "SupportStaff", "Staff", "Student",
}

// Defaults maps each role to its default set of permissions.
// Admin is intentionally absent — it bypasses all permission checks.
var Defaults = map[string][]string{
	"Teacher": {
		PeopleView,
		EnrolmentsView,
		ProgramsView,
		SessionsView, SessionsManage,
		AttendanceView, AttendanceMark,
		ResultsView, ResultsManage,
		VCCAccess, VCCManage,
		WorkplanView, WorkplanManage,
		StudentPanel,
	},
	"Compliance": {
		PeopleView,
		EnrolmentsView,
		ProgramsView,
		SessionsView,
		AttendanceView,
		ResultsView,
		StudentPanel,
	},
	"Reception": {
		PeopleView, PeopleManage,
		EnrolmentsView, EnrolmentsManage,
		ProgramsView,
		SessionsView,
		StudentPanel,
	},
	"SupportStaff": {
		PeopleView,
		SessionsView,
	},
	"Staff": {
		SessionsView,
		WorkplanView, WorkplanManage,
	},
	"Student": {
		StudentPanel,
	},
}
