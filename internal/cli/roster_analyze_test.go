// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the roster join rules.

package cli

import (
	"strings"
	"testing"
)

const enrollmentJSON = `{
  "user_id": "42",
  "sis_user_id": "SIS-42",
  "course_section_id": "7",
  "role": "StudentEnrollment",
  "enrollment_state": "active",
  "last_activity_at": "2026-08-01T10:00:00Z",
  "user": {"name": "Ada Lovelace", "sortable_name": "Lovelace, Ada", "login_id": "ada@example.edu"},
  "grades": {"current_score": 91.5, "final_score": 88, "current_grade": "A-"}
}`

func sections() map[string]string { return map[string]string{"7": "Section A"} }

// TestRosterRowFor_JoinsUserSectionAndGrades covers the whole join: fields
// come from the enrollment, the nested user, the section map, and grades.
func TestRosterRowFor_JoinsUserSectionAndGrades(t *testing.T) {
	rs := rosterRowFor(obj(t, enrollmentJSON), sections(), false)

	if rs.UserID != "42" {
		t.Errorf("UserID = %q, want %q", rs.UserID, "42")
	}
	if rs.Name != "Ada Lovelace" {
		t.Errorf("Name = %q, want %q", rs.Name, "Ada Lovelace")
	}
	if rs.SortableName != "Lovelace, Ada" {
		t.Errorf("SortableName = %q", rs.SortableName)
	}
	if rs.LoginID != "ada@example.edu" {
		t.Errorf("LoginID = %q", rs.LoginID)
	}
	if rs.SISUserID != "SIS-42" {
		t.Errorf("SISUserID = %q", rs.SISUserID)
	}
	if rs.Section != "Section A" {
		t.Errorf("Section = %q, want %q (section map lookup)", rs.Section, "Section A")
	}
	if rs.Role != "StudentEnrollment" || rs.State != "active" {
		t.Errorf("Role/State = %q/%q", rs.Role, rs.State)
	}
	if rs.CurrentGrade != "A-" {
		t.Errorf("CurrentGrade = %q", rs.CurrentGrade)
	}
	if rs.CurrentScore == nil || *rs.CurrentScore != 91.5 {
		t.Errorf("CurrentScore = %v, want 91.5", rs.CurrentScore)
	}
	if rs.FinalScore == nil || *rs.FinalScore != 88 {
		t.Errorf("FinalScore = %v, want 88", rs.FinalScore)
	}
}

// TestRosterRowFor_UnknownSectionLeavesSectionEmpty pins the best-effort
// contract: a missing section map must not break the row.
func TestRosterRowFor_UnknownSectionLeavesSectionEmpty(t *testing.T) {
	rs := rosterRowFor(obj(t, enrollmentJSON), map[string]string{}, false)
	if rs.Section != "" {
		t.Errorf("Section = %q, want empty when the section is unknown", rs.Section)
	}
	if rs.UserID != "42" {
		t.Errorf("row should still build; UserID = %q", rs.UserID)
	}
}

// TestRosterRowFor_NameFallsBackToUserID covers enrollments whose user block
// carries no name.
func TestRosterRowFor_NameFallsBackToUserID(t *testing.T) {
	rs := rosterRowFor(obj(t, `{"user_id": "99", "user": {}}`), sections(), false)
	if rs.Name != "99" {
		t.Errorf("Name = %q, want the user id %q as fallback", rs.Name, "99")
	}
}

// TestRosterRowFor_AnonymizeDropsIdentifiers is the rule that matters for
// sharing output with an agent: no real identifier may survive.
func TestRosterRowFor_AnonymizeDropsIdentifiers(t *testing.T) {
	rs := rosterRowFor(obj(t, enrollmentJSON), sections(), true)

	for _, leaked := range []string{"Ada Lovelace", "Lovelace, Ada", "ada@example.edu", "SIS-42"} {
		if strings.Contains(rs.Name, leaked) {
			t.Errorf("anonymized Name %q leaks %q", rs.Name, leaked)
		}
	}
	if rs.SortableName != "" || rs.LoginID != "" || rs.SISUserID != "" {
		t.Errorf("anonymize must clear sortable_name/login_id/sis_user_id, got %q/%q/%q",
			rs.SortableName, rs.LoginID, rs.SISUserID)
	}
	// Non-identifying fields survive so the roster stays useful.
	if rs.Section != "Section A" || rs.Role != "StudentEnrollment" {
		t.Errorf("anonymize should not clear section/role, got %q/%q", rs.Section, rs.Role)
	}
	// The label must be stable, or successive runs cannot be correlated.
	again := rosterRowFor(obj(t, enrollmentJSON), sections(), true)
	if rs.Name != again.Name {
		t.Errorf("anonymized label not stable: %q vs %q", rs.Name, again.Name)
	}
}

// TestRosterTableRow_OmitsScoreWhenAbsent keeps the table from rendering a
// zero score for a student who has none.
func TestRosterTableRow_OmitsScoreWhenAbsent(t *testing.T) {
	withScore := rosterTableRow(rosterRowFor(obj(t, enrollmentJSON), sections(), false))
	if withScore["current_score"] != 91.5 {
		t.Errorf("current_score = %v, want 91.5", withScore["current_score"])
	}

	none := rosterTableRow(rosterRowFor(obj(t, `{"user_id": "7", "user": {"name": "No Grades"}}`), sections(), false))
	if _, present := none["current_score"]; present {
		t.Errorf("current_score must be absent when the student has no score, got %v", none["current_score"])
	}
}

// TestAnalyzeRoster_BuildsViewAndRows covers the happy path end to end.
func TestAnalyzeRoster_BuildsViewAndRows(t *testing.T) {
	view, rows := analyzeRoster(rosterInput{
		CourseID:     "12345",
		Enrollments:  []canvasObj{obj(t, enrollmentJSON), obj(t, `{"user_id": "43", "user": {"name": "Grace Hopper"}}`)},
		SectionNames: sections(),
		ScannedPages: 2,
	})

	if view.CourseID != "12345" || view.ScannedPages != 2 {
		t.Errorf("view scope = %q/%d, want 12345/2", view.CourseID, view.ScannedPages)
	}
	if view.Count != 2 || len(view.Students) != 2 {
		t.Errorf("Count = %d, len(Students) = %d, want 2/2", view.Count, len(view.Students))
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if view.Note != "" {
		t.Errorf("Note should be empty when the roster is populated, got %q", view.Note)
	}
	if view.Anonymized {
		t.Error("Anonymized should follow the input flag")
	}
}

// TestAnalyzeRoster_EmptyNotesAndForcesJSON pins the empty-roster contract:
// a note explaining the emptiness, and nil rows so the note is not hidden
// behind an empty table.
func TestAnalyzeRoster_EmptyNotesAndForcesJSON(t *testing.T) {
	view, rows := analyzeRoster(rosterInput{CourseID: "999", ScannedPages: 5})

	if rows != nil {
		t.Errorf("rows must be nil for an empty roster so the note renders, got %v", rows)
	}
	if view.Count != 0 {
		t.Errorf("Count = %d, want 0", view.Count)
	}
	if !strings.Contains(view.Note, "999") || !strings.Contains(view.Note, "5 page(s)") {
		t.Errorf("Note should name the course and pages scanned, got %q", view.Note)
	}
}
