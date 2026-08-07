// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the since-window digest rules.

package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var sinceCutoff = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

func submissionAt(userID, assignment, at string) string {
	return fmt.Sprintf(`{"user_id": %q, "submitted_at": %q, "assignment": {"name": %q}}`, userID, at, assignment)
}

func announcementAt(title, postedAt string) string {
	return fmt.Sprintf(`{"title": %q, "posted_at": %q}`, title, postedAt)
}

func enrollmentAt(userID, kind, createdAt string) string {
	return fmt.Sprintf(`{"user_id": %q, "type": %q, "created_at": %q, "user": {"name": "Enrolled User"}}`,
		userID, kind, createdAt)
}

func changeTimes(changes []sinceChange) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.At)
	}
	return out
}

// TestAnalyzeSince_MergesThreeStreams covers the core merge and its counters.
func TestAnalyzeSince_MergesThreeStreams(t *testing.T) {
	view, rows := analyzeSince(sinceInput{
		Window: "7d", Cutoff: sinceCutoff,
		Courses:               []courseRef{{ID: "1"}},
		SubmissionsByCourse:   map[string][]canvasObj{"1": objs(t, submissionAt("42", "Essay", "2026-08-03T12:00:00Z"))},
		AnnouncementsByCourse: map[string][]canvasObj{"1": objs(t, announcementAt("Midterm moved", "2026-08-04T12:00:00Z"))},
		EnrollmentsByCourse:   map[string][]canvasObj{"1": objs(t, enrollmentAt("43", "StudentEnrollment", "2026-08-02T12:00:00Z"))},
		NamesByCourse:         map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
	})

	if rows != nil {
		t.Errorf("since renders as JSON, rows should be nil, got %v", rows)
	}
	if view.NewSubmissions != 1 || view.NewAnnouncements != 1 || view.NewEnrollments != 1 {
		t.Errorf("counters = %d/%d/%d, want 1/1/1",
			view.NewSubmissions, view.NewAnnouncements, view.NewEnrollments)
	}
	if len(view.Changes) != 3 {
		t.Fatalf("len(Changes) = %d, want 3", len(view.Changes))
	}
	kinds := map[string]bool{}
	for _, c := range view.Changes {
		kinds[c.Kind] = true
	}
	for _, want := range []string{"submission", "announcement", "enrollment"} {
		if !kinds[want] {
			t.Errorf("missing a %q change in %+v", want, view.Changes)
		}
	}
}

// TestAnalyzeSince_ChangesAreChronological is the contract this command's name
// promises: a "what changed since" digest must read in time order.
//
// The fixtures are arranged so fetch order and chronological order disagree —
// course 1's submission is older than course 2's, and within a course the
// announcement predates the submission.
func TestAnalyzeSince_ChangesAreChronological(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Window: "7d", Cutoff: sinceCutoff,
		Courses: []courseRef{{ID: "1"}, {ID: "2"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t, submissionAt("42", "Late essay", "2026-08-05T12:00:00Z")),
			"2": objs(t, submissionAt("43", "Early essay", "2026-08-02T12:00:00Z")),
		},
		AnnouncementsByCourse: map[string][]canvasObj{
			"1": objs(t, announcementAt("Oldest notice", "2026-08-01T12:00:00Z")),
		},
		EnrollmentsByCourse: map[string][]canvasObj{
			"2": objs(t, enrollmentAt("44", "StudentEnrollment", "2026-08-06T12:00:00Z")),
		},
	})

	got := changeTimes(view.Changes)
	want := []string{
		"2026-08-01T12:00:00Z", // announcement, course 1
		"2026-08-02T12:00:00Z", // submission, course 2
		"2026-08-05T12:00:00Z", // submission, course 1
		"2026-08-06T12:00:00Z", // enrollment, course 2
	}
	if len(got) != len(want) {
		t.Fatalf("got %d changes, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("changes are not in time order:\n got  %v\n want %v", got, want)
		}
	}
}

// TestAnalyzeSince_CutoffFiltersSubmissionsAndEnrollments covers the
// client-side window check. Announcements are deliberately exempt: the API
// filters them by start_date.
func TestAnalyzeSince_CutoffFiltersSubmissionsAndEnrollments(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Window: "7d", Cutoff: sinceCutoff,
		Courses: []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{
			"1": objs(t,
				submissionAt("42", "Too old", "2026-07-01T12:00:00Z"),
				submissionAt("42", "In window", "2026-08-03T12:00:00Z")),
		},
		EnrollmentsByCourse: map[string][]canvasObj{
			"1": objs(t,
				enrollmentAt("43", "StudentEnrollment", "2026-07-01T12:00:00Z"),
				enrollmentAt("44", "StudentEnrollment", "2026-08-03T12:00:00Z")),
		},
		AnnouncementsByCourse: map[string][]canvasObj{
			// Before the cutoff, but the API already filtered — keep it.
			"1": objs(t, announcementAt("API-filtered", "2026-07-01T12:00:00Z")),
		},
	})

	if view.NewSubmissions != 1 {
		t.Errorf("NewSubmissions = %d, want 1 (the older one is outside the window)", view.NewSubmissions)
	}
	if view.NewEnrollments != 1 {
		t.Errorf("NewEnrollments = %d, want 1 (the older one is outside the window)", view.NewEnrollments)
	}
	if view.NewAnnouncements != 1 {
		t.Errorf("NewAnnouncements = %d, want 1 — announcements are filtered by the API, not re-checked here",
			view.NewAnnouncements)
	}
}

// TestAnalyzeSince_SubmissionCarriesStudentAndAssignment covers the submission
// change's fields.
func TestAnalyzeSince_SubmissionCarriesStudentAndAssignment(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Cutoff:              sinceCutoff,
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, submissionAt("42", "Essay", "2026-08-03T12:00:00Z"))},
		NamesByCourse:       map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
	})

	c := view.Changes[0]
	if c.Who != "Ada Lovelace" || c.What != "Essay" || c.CourseID != "1" {
		t.Errorf("submission change = %+v", c)
	}
}

// TestAnalyzeSince_EnrollmentNameFallsBackToNameMap covers the two-step name
// lookup for enrollments.
func TestAnalyzeSince_EnrollmentNameFallsBackToNameMap(t *testing.T) {
	noUserBlock := `{"user_id": "43", "type": "StudentEnrollment", "created_at": "2026-08-03T12:00:00Z"}`
	view, _ := analyzeSince(sinceInput{
		Cutoff:              sinceCutoff,
		Courses:             []courseRef{{ID: "1"}},
		EnrollmentsByCourse: map[string][]canvasObj{"1": objs(t, noUserBlock)},
		NamesByCourse:       map[string]map[string]string{"1": {"43": "Grace Hopper"}},
	})

	if view.Changes[0].Who != "Grace Hopper" {
		t.Errorf("Who = %q, want the name-map fallback", view.Changes[0].Who)
	}
}

// TestAnalyzeSince_AnonymizeReplacesNames keeps real names out of a shared
// digest across both name-bearing streams.
func TestAnalyzeSince_AnonymizeReplacesNames(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Cutoff:              sinceCutoff,
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, submissionAt("42", "Essay", "2026-08-03T12:00:00Z"))},
		EnrollmentsByCourse: map[string][]canvasObj{"1": objs(t, enrollmentAt("43", "StudentEnrollment", "2026-08-04T12:00:00Z"))},
		NamesByCourse:       map[string]map[string]string{"1": {"42": "Ada Lovelace"}},
		Anonymize:           true,
	})

	for _, c := range view.Changes {
		for _, leaked := range []string{"Ada Lovelace", "Enrolled User"} {
			if strings.Contains(c.Who, leaked) {
				t.Errorf("%s change leaks %q: %q", c.Kind, leaked, c.Who)
			}
		}
	}
	if !view.Anonymized {
		t.Error("Anonymized should be reported on the view")
	}
}

// TestAnalyzeSince_PartialFetchStillContributes pins the behaviour that sets
// this command apart from its siblings: a failed stream does not discard the
// course, and whatever the API returned before failing is still reported.
func TestAnalyzeSince_PartialFetchStillContributes(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Window: "7d", Cutoff: sinceCutoff,
		Courses:             []courseRef{{ID: "1"}},
		SubmissionsByCourse: map[string][]canvasObj{"1": objs(t, submissionAt("42", "Essay", "2026-08-03T12:00:00Z"))},
		// The announcements fetch failed; enrollments were never populated.
		Failures: []fetchFailure{{Scope: "announcements:1", Error: "boom"}},
	})

	if view.NewSubmissions != 1 {
		t.Errorf("a failed stream must not discard the course's other streams, got %d submissions",
			view.NewSubmissions)
	}
	if len(view.FetchFailures) != 1 || view.FetchFailures[0].Scope != "announcements:1" {
		t.Errorf("failures should pass through with per-stream scope, got %+v", view.FetchFailures)
	}
}

// TestAnalyzeSince_EmptyNotesAndEmitsEmptySlice pins the quiet-window contract.
func TestAnalyzeSince_EmptyNotesAndEmitsEmptySlice(t *testing.T) {
	view, _ := analyzeSince(sinceInput{
		Window:  "24h",
		Cutoff:  sinceCutoff,
		Courses: []courseRef{{ID: "1"}, {ID: "2"}},
	})

	if view.Changes == nil {
		t.Error("Changes should be an empty slice, not nil, so the JSON carries []")
	}
	if !strings.Contains(view.Note, "last 24h") || !strings.Contains(view.Note, "2 course(s)") {
		t.Errorf("Note should name the window and course count, got %q", view.Note)
	}
}
