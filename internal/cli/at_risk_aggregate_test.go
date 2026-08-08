// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for at-risk aggregation across courses.

package cli

import (
	"strings"
	"testing"
	"time"
)

// missingSub builds a submission the classifier flags as "missing".
func missingSub(userID, assignmentID string) string {
	return `{"user_id": "` + userID + `", "assignment_id": "` + assignmentID + `",
	         "missing": true, "assignment": {"name": "Essay ` + assignmentID + `", "points_possible": 10}}`
}

func lateSub(userID, assignmentID string) string {
	return `{"user_id": "` + userID + `", "assignment_id": "` + assignmentID + `",
	         "late": true, "assignment": {"name": "Lab ` + assignmentID + `"}}`
}

func studentByID(view atRiskView, userID string) *atRiskStudent {
	for i := range view.Students {
		if view.Students[i].UserID == userID {
			return &view.Students[i]
		}
	}
	return nil
}

// TestAnalyzeAtRisk_CountsByStatus pins the per-status counters and the total.
func TestAnalyzeAtRisk_CountsByStatus(t *testing.T) {
	view, rows := analyzeAtRisk(atRiskInput{
		Scope:   "course 1",
		Courses: []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t, missingSub("42", "a"), missingSub("42", "b"), lateSub("42", "c")),
		},
		NamesByCourse: map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
	})

	st := studentByID(view, "42")
	if st == nil {
		t.Fatalf("student 42 missing from %+v", view.Students)
	}
	if st.Missing != 2 || st.Late != 1 || st.Unsubmitted != 0 {
		t.Errorf("counters = missing %d, late %d, unsubmitted %d; want 2/1/0", st.Missing, st.Late, st.Unsubmitted)
	}
	if st.Total != 3 {
		t.Errorf("Total = %d, want 3", st.Total)
	}
	if len(st.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(st.Items))
	}
	if len(rows) != 1 {
		t.Errorf("len(rows) = %d, want 1", len(rows))
	}
}

// TestAnalyzeAtRisk_ItemCarriesAssignmentDetail covers the per-concern record,
// including the optional points_possible pointer.
func TestAnalyzeAtRisk_ItemCarriesAssignmentDetail(t *testing.T) {
	view, _ := analyzeAtRisk(atRiskInput{
		Courses: []courseRef{{ID: "7"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"7": objs(t, missingSub("42", "a"), lateSub("42", "c")),
		},
	})

	st := studentByID(view, "42")
	if st == nil {
		t.Fatal("student 42 missing")
	}
	var withPoints, withoutPoints *atRiskItem
	for i := range st.Items {
		if st.Items[i].AssignmentID == "a" {
			withPoints = &st.Items[i]
		}
		if st.Items[i].AssignmentID == "c" {
			withoutPoints = &st.Items[i]
		}
	}
	if withPoints == nil || withoutPoints == nil {
		t.Fatalf("expected both items, got %+v", st.Items)
	}
	if withPoints.CourseID != "7" || withPoints.Status != "missing" || withPoints.AssignmentName != "Essay a" {
		t.Errorf("item detail = %+v", *withPoints)
	}
	if withPoints.PointsPossible == nil || *withPoints.PointsPossible != 10 {
		t.Errorf("PointsPossible = %v, want 10", withPoints.PointsPossible)
	}
	if withoutPoints.PointsPossible != nil {
		t.Errorf("PointsPossible should stay nil when the assignment has none, got %v", *withoutPoints.PointsPossible)
	}
}

// TestAnalyzeAtRisk_AggregatesAcrossCourses is the cross-course join this
// command exists for: one student, concerns in two courses, one ranked row.
func TestAnalyzeAtRisk_AggregatesAcrossCourses(t *testing.T) {
	view, _ := analyzeAtRisk(atRiskInput{
		Scope:   "all my courses",
		Courses: []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t, missingSub("42", "a"), missingSub("42", "b")),
			"2": objs(t, lateSub("42", "c")),
		},
		NamesByCourse: map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
	})

	if len(view.Students) != 1 {
		t.Fatalf("want one aggregated student, got %d: %+v", len(view.Students), view.Students)
	}
	st := view.Students[0]
	if st.Total != 3 {
		t.Errorf("Total across courses = %d, want 3", st.Total)
	}
	if len(st.CourseIDs) != 2 {
		t.Errorf("CourseIDs = %v, want both courses once each", st.CourseIDs)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2", view.CoursesScanned)
	}
}

// TestAnalyzeAtRisk_CourseIDsDeduped guards containsStr: repeated concerns in
// one course must not repeat the course id.
func TestAnalyzeAtRisk_CourseIDsDeduped(t *testing.T) {
	view, _ := analyzeAtRisk(atRiskInput{
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, missingSub("42", "a"), missingSub("42", "b"))},
	})
	st := studentByID(view, "42")
	if st == nil {
		t.Fatal("student 42 missing")
	}
	if len(st.CourseIDs) != 1 || st.CourseIDs[0] != "1" {
		t.Errorf("CourseIDs = %v, want exactly [1]", st.CourseIDs)
	}
}

// TestAnalyzeAtRisk_NameFromFirstCourseThenFallback covers name resolution: the
// first course a student appears in supplies the name, and an unknown student
// falls back to a readable placeholder.
func TestAnalyzeAtRisk_NameFromFirstCourseThenFallback(t *testing.T) {
	view, _ := analyzeAtRisk(atRiskInput{
		Courses: []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t, missingSub("42", "a")),
			"2": objs(t, missingSub("42", "b"), missingSub("99", "c")),
		},
		NamesByCourse: map[string]map[string]string{
			"1": {"42": "Ada Lovelace"},
			"2": {"42": "A. Lovelace"},
		},
	})

	if st := studentByID(view, "42"); st == nil || st.Name != "Ada Lovelace" {
		t.Errorf("name should come from the first course the student appears in, got %+v", st)
	}
	if st := studentByID(view, "99"); st == nil || st.Name != "user 99" {
		t.Errorf("unknown student should fall back to %q, got %+v", "user 99", st)
	}
}

// TestAnalyzeAtRisk_CutoffExcludesOlderWork covers the --since window reaching
// the classifier.
func TestAnalyzeAtRisk_CutoffExcludesOlderWork(t *testing.T) {
	old := `{"user_id": "42", "assignment_id": "old", "missing": true,
	         "assignment": {"due_at": "2020-01-01T00:00:00Z"}}`
	recent := `{"user_id": "42", "assignment_id": "new", "missing": true,
	           "assignment": {"due_at": "2999-01-01T00:00:00Z"}}`

	in := atRiskInput{
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, old, recent)},
	}

	noWindow, _ := analyzeAtRisk(in)
	if st := studentByID(noWindow, "42"); st == nil || st.Total != 2 {
		t.Errorf("without a cutoff both concerns count, got %+v", st)
	}

	in.Cutoff = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	windowed, _ := analyzeAtRisk(in)
	st := studentByID(windowed, "42")
	if st == nil || st.Total != 1 {
		t.Fatalf("with a cutoff only the recent concern counts, got %+v", st)
	}
	if st.Items[0].AssignmentID != "new" {
		t.Errorf("wrong item survived the window: %+v", st.Items[0])
	}
}

// TestAnalyzeAtRisk_FailedFetchContributesNothing covers the contract that a
// course absent from SubmissionsByCourse yields no concerns, while still
// counting as scanned and passing its failure through.
func TestAnalyzeAtRisk_FailedFetchContributesNothing(t *testing.T) {
	view, _ := analyzeAtRisk(atRiskInput{
		Courses:             []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, missingSub("42", "a"))},
		Failures:            []fetchFailure{{Scope: "course:2", Error: "boom"}},
	})

	if len(view.Students) != 1 {
		t.Errorf("only the fetched course contributes, got %+v", view.Students)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2 (a failed course was still scanned)", view.CoursesScanned)
	}
	if len(view.FetchFailures) != 1 || view.FetchFailures[0].Scope != "course:2" {
		t.Errorf("failures should pass through, got %+v", view.FetchFailures)
	}
}

// TestAnalyzeAtRisk_AnonymizeReplacesNames keeps every real identifier out of
// shared output while leaving the ranking intact. Under --anonymize the user id
// is replaced by the same label as the name, not merely accompanied by it: a
// Canvas user id resolves to a named student in one API call, so keeping it
// would make hashing the name pointless.
func TestAnalyzeAtRisk_AnonymizeReplacesNames(t *testing.T) {
	view, rows := analyzeAtRisk(atRiskInput{
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, missingSub("42", "a"))},
		NamesByCourse:       map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
		Anonymize:           true,
	})

	if len(view.Students) != 1 {
		t.Fatalf("students = %d, want 1", len(view.Students))
	}
	st := view.Students[0]

	if st.UserID == "42" {
		t.Errorf("anonymize must replace the real user id, got %q", st.UserID)
	}
	if strings.Contains(st.Name, "Ada Lovelace") {
		t.Errorf("anonymized name leaks the real name: %q", st.Name)
	}
	// Label goes in both fields so anonymized rows remain joinable across
	// commands — that is the join key an admin actually needs.
	if st.UserID != st.Name {
		t.Errorf("user_id and name must carry the same label, got %q vs %q", st.UserID, st.Name)
	}
	if st.UserID != anonLabel("student", "42") {
		t.Errorf("label must derive from the real user id, got %q", st.UserID)
	}
	if !view.Anonymized {
		t.Error("Anonymized should be reported on the view")
	}
	if rows[0]["name"] != st.Name {
		t.Errorf("table row must carry the anonymized name, got %v", rows[0]["name"])
	}
}

// TestAnalyzeAtRisk_EmptyNotesAndForcesJSON pins the empty-result contract.
func TestAnalyzeAtRisk_EmptyNotesAndForcesJSON(t *testing.T) {
	view, rows := analyzeAtRisk(atRiskInput{
		Courses:      []courseRef{{ID: "1"}, {ID: "2"}},
		ScannedPages: 4,
	})

	if rows != nil {
		t.Errorf("rows must be nil when nothing is at risk so the note renders, got %v", rows)
	}
	if !strings.Contains(view.Note, "2 course(s), 4 page(s)") {
		t.Errorf("Note should name courses and pages scanned, got %q", view.Note)
	}
}
