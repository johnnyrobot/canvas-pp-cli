// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the grading-queue rules.

package cli

import (
	"strings"
	"testing"
	"time"
)

// fixedNow anchors every age assertion in this file.
var fixedNow = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

// ungradedSub builds a submission that needsGrading accepts.
func ungradedSub(userID, assignmentID, submittedAt string) string {
	return `{"user_id": "` + userID + `", "assignment_id": "` + assignmentID + `",
	         "workflow_state": "submitted", "submitted_at": "` + submittedAt + `",
	         "submission_type": "online_upload",
	         "assignment": {"name": "Essay ` + assignmentID + `", "points_possible": 20}}`
}

func queueIDs(items []toGradeItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.AssignmentID)
	}
	return out
}

func oneCourseQueue(t *testing.T, sortOrder string, subs ...string) (toGradeView, []map[string]any) {
	t.Helper()
	return analyzeToGrade(toGradeInput{
		Scope:               "course 1",
		Sort:                sortOrder,
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, subs...)},
		NamesByCourse:       map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
		Now:                 fixedNow,
	})
}

// TestDaysWaiting pins the age calculation, including the timestamps that do
// not parse.
func TestDaysWaiting(t *testing.T) {
	cases := []struct {
		name        string
		submittedAt string
		want        int
	}{
		{"same day", "2026-08-07T00:00:00Z", 0},
		{"one day", "2026-08-06T12:00:00Z", 1},
		{"ten days", "2026-07-28T12:00:00Z", 10},
		{"partial day rounds down", "2026-08-06T13:00:00Z", 0},
		{"empty timestamp", "", 0},
		{"unparseable timestamp", "not-a-date", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := daysWaiting(tc.submittedAt, fixedNow); got != tc.want {
				t.Errorf("daysWaiting(%q) = %d, want %d", tc.submittedAt, got, tc.want)
			}
		})
	}
}

// TestAnalyzeToGrade_FiltersToUngraded confirms only submissions needsGrading
// accepts reach the queue.
func TestAnalyzeToGrade_FiltersToUngraded(t *testing.T) {
	graded := `{"user_id": "43", "assignment_id": "done", "workflow_state": "graded",
	            "submitted_at": "2026-08-01T12:00:00Z", "score": 90}`
	view, _ := oneCourseQueue(t, "oldest",
		ungradedSub("42", "pending", "2026-08-01T12:00:00Z"), graded)

	if view.Count != 1 || len(view.Items) != 1 {
		t.Fatalf("Count = %d, items = %+v; want only the ungraded one", view.Count, view.Items)
	}
	if view.Items[0].AssignmentID != "pending" {
		t.Errorf("queued %q, want %q", view.Items[0].AssignmentID, "pending")
	}
}

// TestAnalyzeToGrade_OldestFirstByDefault pins the default ordering, which is
// the command's headline promise.
func TestAnalyzeToGrade_OldestFirstByDefault(t *testing.T) {
	view, _ := oneCourseQueue(t, "oldest",
		ungradedSub("42", "middle", "2026-08-03T12:00:00Z"),
		ungradedSub("42", "oldest", "2026-08-01T12:00:00Z"),
		ungradedSub("42", "newest", "2026-08-05T12:00:00Z"))

	got := queueIDs(view.Items)
	want := []string{"oldest", "middle", "newest"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("oldest-first order = %v, want %v", got, want)
		}
	}
}

// TestAnalyzeToGrade_NewestReversesOrder covers --sort newest.
func TestAnalyzeToGrade_NewestReversesOrder(t *testing.T) {
	view, _ := oneCourseQueue(t, "newest",
		ungradedSub("42", "oldest", "2026-08-01T12:00:00Z"),
		ungradedSub("42", "newest", "2026-08-05T12:00:00Z"))

	if got := queueIDs(view.Items); got[0] != "newest" {
		t.Errorf("newest-first order = %v, want newest first", got)
	}
	if view.Sort != "newest" {
		t.Errorf("view.Sort = %q, want it echoed back", view.Sort)
	}
}

// TestAnalyzeToGrade_UnknownSortFallsBackToOldest guards the flag default:
// anything other than "newest" behaves as oldest-first.
func TestAnalyzeToGrade_UnknownSortFallsBackToOldest(t *testing.T) {
	view, _ := oneCourseQueue(t, "sideways",
		ungradedSub("42", "newest", "2026-08-05T12:00:00Z"),
		ungradedSub("42", "oldest", "2026-08-01T12:00:00Z"))

	if got := queueIDs(view.Items); got[0] != "oldest" {
		t.Errorf("unknown sort = %v, want oldest-first fallback", got)
	}
}

// TestAnalyzeToGrade_LimitAppliesAfterSorting is the ordering-sensitive case:
// a limit must keep the oldest work, not whichever course was fetched first.
func TestAnalyzeToGrade_LimitAppliesAfterSorting(t *testing.T) {
	view, _ := analyzeToGrade(toGradeInput{
		Sort:    "oldest",
		Courses: []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t, ungradedSub("42", "recent", "2026-08-05T12:00:00Z")),
			"2": objs(t, ungradedSub("43", "ancient", "2026-01-01T12:00:00Z")),
		},
		Now:   fixedNow,
		Limit: 1,
	})

	if len(view.Items) != 1 {
		t.Fatalf("Limit not applied, got %d items", len(view.Items))
	}
	if view.Items[0].AssignmentID != "ancient" {
		t.Errorf("limit kept %q; it must apply after sorting so the oldest work survives",
			view.Items[0].AssignmentID)
	}
	if view.Count != 1 {
		t.Errorf("Count = %d, want the limited length", view.Count)
	}
}

// TestAnalyzeToGrade_ItemDetail covers the queue entry's fields, including the
// injected clock and the optional points_possible pointer.
func TestAnalyzeToGrade_ItemDetail(t *testing.T) {
	view, rows := oneCourseQueue(t, "oldest", ungradedSub("42", "a", "2026-08-04T12:00:00Z"))

	it := view.Items[0]
	if it.Student != "Ada Lovelace" {
		t.Errorf("Student = %q, want the name map lookup", it.Student)
	}
	if it.DaysWaiting != 3 {
		t.Errorf("DaysWaiting = %d, want 3 against the injected clock", it.DaysWaiting)
	}
	if it.PointsPossible == nil || *it.PointsPossible != 20 {
		t.Errorf("PointsPossible = %v, want 20", it.PointsPossible)
	}
	if it.SubmissionType != "online_upload" {
		t.Errorf("SubmissionType = %q", it.SubmissionType)
	}
	if len(rows) != 1 || rows[0]["days_waiting"] != 3 {
		t.Errorf("table row should carry the age, got %v", rows)
	}
}

// TestAnalyzeToGrade_UnknownStudentFallsBack covers the placeholder name.
func TestAnalyzeToGrade_UnknownStudentFallsBack(t *testing.T) {
	view, _ := oneCourseQueue(t, "oldest", ungradedSub("99", "a", "2026-08-04T12:00:00Z"))
	if view.Items[0].Student != "user 99" {
		t.Errorf("Student = %q, want %q", view.Items[0].Student, "user 99")
	}
}

// TestAnalyzeToGrade_AnonymizeReplacesNames keeps real names out of a shared
// grading queue.
func TestAnalyzeToGrade_AnonymizeReplacesNames(t *testing.T) {
	view, rows := analyzeToGrade(toGradeInput{
		Sort:                "oldest",
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, ungradedSub("42", "a", "2026-08-04T12:00:00Z"))},
		NamesByCourse:       map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
		Now:                 fixedNow,
		Anonymize:           true,
	})

	if strings.Contains(view.Items[0].Student, "Ada Lovelace") {
		t.Errorf("anonymized student leaks the real name: %q", view.Items[0].Student)
	}
	if rows[0]["student"] != view.Items[0].Student {
		t.Errorf("table row must carry the anonymized name, got %v", rows[0]["student"])
	}
	if !view.Anonymized {
		t.Error("Anonymized should be reported on the view")
	}
}

// TestAnalyzeToGrade_FailedFetchContributesNothing covers the contract that a
// course absent from SubmissionsByCourse yields no queue entries while still
// counting as scanned.
func TestAnalyzeToGrade_FailedFetchContributesNothing(t *testing.T) {
	view, _ := analyzeToGrade(toGradeInput{
		Sort:                "oldest",
		Courses:             []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, ungradedSub("42", "a", "2026-08-04T12:00:00Z"))},
		Now:                 fixedNow,
		Failures:            []fetchFailure{{Scope: "course:2", Error: "boom"}},
	})

	if view.Count != 1 {
		t.Errorf("Count = %d, want only the fetched course's work", view.Count)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2 (a failed course was still scanned)", view.CoursesScanned)
	}
	if len(view.FetchFailures) != 1 {
		t.Errorf("failures should pass through, got %+v", view.FetchFailures)
	}
}

// TestAnalyzeToGrade_EmptyNotesAndForcesJSON pins the empty-queue contract.
func TestAnalyzeToGrade_EmptyNotesAndForcesJSON(t *testing.T) {
	view, rows := analyzeToGrade(toGradeInput{
		Courses:      []courseRef{{ID: "1"}, {ID: "2"}},
		Now:          fixedNow,
		ScannedPages: 3,
	})

	if rows != nil {
		t.Errorf("rows must be nil for an empty queue so the note renders, got %v", rows)
	}
	if view.Items == nil {
		t.Error("Items should be an empty slice, not nil, so the JSON carries []")
	}
	if !strings.Contains(view.Note, "2 course(s), 3 page(s)") {
		t.Errorf("Note should name courses and pages scanned, got %q", view.Note)
	}
}
