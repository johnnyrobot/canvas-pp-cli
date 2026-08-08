// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the enrollment-audit anomaly rules.

package cli

import (
	"strings"
	"testing"
)

func objs(t *testing.T, raw ...string) []canvasObj {
	t.Helper()
	out := make([]canvasObj, 0, len(raw))
	for _, r := range raw {
		out = append(out, obj(t, r))
	}
	return out
}

// TestIsGhostCourse pins the teacher-less rule.
func TestIsGhostCourse(t *testing.T) {
	cases := []struct {
		name   string
		course string
		want   bool
	}{
		{"no teachers key at all", `{"id": "1"}`, true},
		{"empty teachers array", `{"id": "1", "teachers": []}`, true},
		{"teachers null", `{"id": "1", "teachers": null}`, true},
		{"one teacher", `{"id": "1", "teachers": [{"id": "9"}]}`, false},
		{"several teachers", `{"id": "1", "teachers": [{"id": "9"}, {"id": "10"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGhostCourse(obj(t, tc.course)); got != tc.want {
				t.Errorf("isGhostCourse = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsOrphanEnrollment pins the never-accessed rule. Canvas represents "never
// accessed" as an absent key or an explicit null, so both must count.
func TestIsOrphanEnrollment(t *testing.T) {
	cases := []struct {
		name string
		enr  string
		want bool
	}{
		{"key absent", `{"user_id": "1"}`, true},
		{"explicit null", `{"user_id": "1", "last_activity_at": null}`, true},
		{"has a timestamp", `{"user_id": "1", "last_activity_at": "2026-08-01T10:00:00Z"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOrphanEnrollment(obj(t, tc.enr)); got != tc.want {
				t.Errorf("isOrphanEnrollment = %v, want %v", got, tc.want)
			}
		})
	}
}

func bothChecks(courses []canvasObj, enr map[string][]canvasObj) auditInput {
	return auditInput{
		Account:             "1",
		Courses:             courses,
		EnrollmentsByCourse: enr,
		CheckGhostTeachers:  true,
		CheckOrphans:        true,
	}
}

// TestAnalyzeAuditEnrollments_FlagsBothAnomalies covers the happy path: a
// teacher-less course and a never-accessed student in a staffed course.
func TestAnalyzeAuditEnrollments_FlagsBothAnomalies(t *testing.T) {
	view, rows := analyzeAuditEnrollments(bothChecks(
		objs(t,
			`{"id": "10", "name": "Ghost Course", "workflow_state": "available"}`,
			`{"id": "20", "name": "Staffed", "teachers": [{"id": "5"}]}`,
		),
		map[string][]canvasObj{
			"20": objs(t,
				`{"user_id": "77", "enrollment_state": "active", "user": {"name": "Never Logged In"}}`,
				`{"user_id": "78", "last_activity_at": "2026-08-01T10:00:00Z", "user": {"name": "Active"}}`,
			),
		},
	))

	if rows != nil {
		t.Errorf("audit always renders JSON, rows should be nil, got %v", rows)
	}
	if len(view.GhostTeacherCourses) != 1 || view.GhostTeacherCourses[0].CourseID != "10" {
		t.Fatalf("ghosts = %+v, want exactly course 10", view.GhostTeacherCourses)
	}
	if view.GhostTeacherCourses[0].Name != "Ghost Course" || view.GhostTeacherCourses[0].State != "available" {
		t.Errorf("ghost detail = %+v", view.GhostTeacherCourses[0])
	}
	if len(view.OrphanStudents) != 1 || view.OrphanStudents[0].UserID != "77" {
		t.Fatalf("orphans = %+v, want exactly user 77", view.OrphanStudents)
	}
	if view.OrphanStudents[0].CourseID != "20" {
		t.Errorf("orphan should carry its course, got %q", view.OrphanStudents[0].CourseID)
	}
	if view.OrphanStudents[0].Reason != orphanReason {
		t.Errorf("orphan reason = %q", view.OrphanStudents[0].Reason)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2", view.CoursesScanned)
	}
	if view.Note != "" {
		t.Errorf("Note should be empty when anomalies were found, got %q", view.Note)
	}
}

// TestAnalyzeAuditEnrollments_ChecksAreIndependent verifies each toggle runs
// only its own check and reports itself in Checks.
func TestAnalyzeAuditEnrollments_ChecksAreIndependent(t *testing.T) {
	courses := objs(t, `{"id": "10"}`)
	enr := map[string][]canvasObj{"10": objs(t, `{"user_id": "77"}`)}

	ghostOnly, _ := analyzeAuditEnrollments(auditInput{
		Account: "1", Courses: courses, EnrollmentsByCourse: enr, CheckGhostTeachers: true,
	})
	if len(ghostOnly.GhostTeacherCourses) != 1 || len(ghostOnly.OrphanStudents) != 0 {
		t.Errorf("ghost-only: ghosts=%d orphans=%d, want 1/0",
			len(ghostOnly.GhostTeacherCourses), len(ghostOnly.OrphanStudents))
	}
	if strings.Join(ghostOnly.Checks, ",") != "ghost-teachers" {
		t.Errorf("Checks = %v, want [ghost-teachers]", ghostOnly.Checks)
	}

	orphanOnly, _ := analyzeAuditEnrollments(auditInput{
		Account: "1", Courses: courses, EnrollmentsByCourse: enr, CheckOrphans: true,
	})
	if len(orphanOnly.GhostTeacherCourses) != 0 || len(orphanOnly.OrphanStudents) != 1 {
		t.Errorf("orphan-only: ghosts=%d orphans=%d, want 0/1",
			len(orphanOnly.GhostTeacherCourses), len(orphanOnly.OrphanStudents))
	}
	if strings.Join(orphanOnly.Checks, ",") != "orphans" {
		t.Errorf("Checks = %v, want [orphans]", orphanOnly.Checks)
	}
}

// TestAnalyzeAuditEnrollments_SkipsCoursesWithoutID guards the id guard: an
// id-less course is not flagged, but still counts as scanned.
func TestAnalyzeAuditEnrollments_SkipsCoursesWithoutID(t *testing.T) {
	view, _ := analyzeAuditEnrollments(bothChecks(
		objs(t, `{"name": "No ID"}`, `{"id": "20", "teachers": [{"id": "5"}]}`),
		map[string][]canvasObj{},
	))
	if len(view.GhostTeacherCourses) != 0 {
		t.Errorf("id-less course must not be flagged, got %+v", view.GhostTeacherCourses)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2 (id-less courses still count as scanned)", view.CoursesScanned)
	}
}

// TestAnalyzeAuditEnrollments_FailedFetchYieldsNoOrphans covers the contract
// that a course missing from EnrollmentsByCourse contributes nothing, which is
// how a failed enrollment fetch reaches analysis.
func TestAnalyzeAuditEnrollments_FailedFetchYieldsNoOrphans(t *testing.T) {
	in := bothChecks(objs(t, `{"id": "10", "teachers": [{"id": "5"}]}`), map[string][]canvasObj{})
	in.Failures = []fetchFailure{{Scope: "course:10", Error: "boom"}}

	view, _ := analyzeAuditEnrollments(in)
	if len(view.OrphanStudents) != 0 {
		t.Errorf("a course with no fetched enrollments yields no orphans, got %+v", view.OrphanStudents)
	}
	if len(view.FetchFailures) != 1 || view.FetchFailures[0].Scope != "course:10" {
		t.Errorf("failures should pass through, got %+v", view.FetchFailures)
	}
}

// TestAnalyzeAuditEnrollments_AnonymizeReplacesNames keeps real student names
// out of shared audit output.
func TestAnalyzeAuditEnrollments_AnonymizeReplacesNames(t *testing.T) {
	in := bothChecks(
		objs(t, `{"id": "20", "teachers": [{"id": "5"}]}`),
		map[string][]canvasObj{"20": objs(t, `{"user_id": "77", "user": {"name": "Ada Lovelace"}}`)},
	)
	in.Anonymize = true

	view, _ := analyzeAuditEnrollments(in)
	if len(view.OrphanStudents) != 1 {
		t.Fatalf("orphans = %d, want 1", len(view.OrphanStudents))
	}
	if strings.Contains(view.OrphanStudents[0].Name, "Ada Lovelace") {
		t.Errorf("anonymized name leaks the real name: %q", view.OrphanStudents[0].Name)
	}
	if !view.Anonymized {
		t.Error("Anonymized should be reported on the view")
	}
	// The user id is replaced, not retained. It used to be kept as "the join
	// key an admin needs", but a stable salted label is equally a join key —
	// the same student carries the same label in every command — while a raw
	// Canvas user id resolves to a named student in one API call, which would
	// defeat the whole flag. An admin who needs to act on a real student runs
	// without --anonymize.
	o := view.OrphanStudents[0]
	if o.UserID == "77" {
		t.Errorf("anonymize must replace the real user id, got %q", o.UserID)
	}
	if o.UserID != o.Name {
		t.Errorf("user_id and name must carry the same label, got %q vs %q", o.UserID, o.Name)
	}
	if o.UserID != anonLabel("student", "77") {
		t.Errorf("label must derive from the real user id, got %q", o.UserID)
	}
}

// TestAnalyzeAuditEnrollments_Notes pins both explanatory notes.
func TestAnalyzeAuditEnrollments_Notes(t *testing.T) {
	empty, _ := analyzeAuditEnrollments(bothChecks(nil, nil))
	if !strings.Contains(empty.Note, "no courses returned for account 1") {
		t.Errorf("empty-account note = %q", empty.Note)
	}

	clean, _ := analyzeAuditEnrollments(bothChecks(
		objs(t, `{"id": "20", "teachers": [{"id": "5"}]}`),
		map[string][]canvasObj{"20": objs(t, `{"user_id": "78", "last_activity_at": "2026-08-01T10:00:00Z"}`)},
	))
	if !strings.Contains(clean.Note, "no anomalies found across 1 course(s)") {
		t.Errorf("clean-audit note = %q", clean.Note)
	}
}
