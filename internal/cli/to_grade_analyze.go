// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"time"
)

// toGradeInput is everything analyzeToGrade needs. The caller fetches each
// course's submissions and student-name map.
type toGradeInput struct {
	// Scope describes what was scanned, e.g. "course 12345" or "all my courses".
	Scope string
	// Sort is "oldest" or "newest" by submission date. Anything else is
	// treated as "oldest", matching the flag default.
	Sort    string
	Courses []courseRef
	// SubmissionsByCourse holds submissions keyed by course id. A course absent
	// from the map contributes nothing — that is how a failed fetch is
	// represented, alongside an entry in Failures.
	SubmissionsByCourse map[string][]canvasObj
	// NamesByCourse maps course id -> user id -> display name.
	NamesByCourse map[string]map[string]string
	// Now anchors the days-waiting age. Injected rather than read from the
	// clock so the queue is reproducible and testable.
	Now time.Time
	// Limit caps the returned queue after sorting. Zero means no cap.
	Limit        int
	Anonymize    bool
	ScannedPages int
	Failures     []fetchFailure
}

// daysWaiting reports how long a submission has been sitting ungraded, in whole
// days. Unparseable or absent timestamps age to zero rather than erroring — a
// queue entry with a bad date is still work to grade.
func daysWaiting(submittedAt string, now time.Time) int {
	t, err := time.Parse(time.RFC3339, submittedAt)
	if err != nil {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}

// toGradeItemFor builds one queue entry from an ungraded submission.
func toGradeItemFor(s canvasObj, courseID string, names map[string]string, now time.Time, anonymize bool) toGradeItem {
	it := toGradeItem{
		CourseID:       courseID,
		AssignmentID:   s.str("assignment_id"),
		AssignmentName: s.obj("assignment").str("name"),
		UserID:         s.str("user_id"),
		Student:        names[s.str("user_id")],
		SubmittedAt:    s.str("submitted_at"),
		SubmissionType: s.str("submission_type"),
	}
	if it.Student == "" {
		it.Student = "user " + it.UserID
	}
	if pp, ok := s.obj("assignment").num("points_possible"); ok {
		it.PointsPossible = &pp
	}
	it.DaysWaiting = daysWaiting(it.SubmittedAt, now)
	if anonymize {
		it.Student = anonLabel("student", it.UserID)
	}
	return it
}

// toGradeTableRow projects a queue entry down to the human-table columns.
func toGradeTableRow(it toGradeItem) map[string]any {
	return map[string]any{
		"course_id": it.CourseID, "assignment": it.AssignmentName,
		"student": it.Student, "submitted_at": it.SubmittedAt, "days_waiting": it.DaysWaiting,
	}
}

// analyzeToGrade filters fetched submissions down to those awaiting a grade and
// orders them into one cross-course queue. Pure: no HTTP, no cobra, no writers,
// and no clock — the caller supplies Now.
func analyzeToGrade(in toGradeInput) (toGradeView, []map[string]any) {
	var items []toGradeItem
	for _, course := range in.Courses {
		names := in.NamesByCourse[course.ID]
		for _, s := range in.SubmissionsByCourse[course.ID] {
			if !needsGrading(s) {
				continue
			}
			items = append(items, toGradeItemFor(s, course.ID, names, in.Now, in.Anonymize))
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if in.Sort == "newest" {
			return items[i].SubmittedAt > items[j].SubmittedAt
		}
		return items[i].SubmittedAt < items[j].SubmittedAt
	})
	if in.Limit > 0 && len(items) > in.Limit {
		items = items[:in.Limit]
	}
	if items == nil {
		items = []toGradeItem{}
	}

	view := toGradeView{
		Scope:          in.Scope,
		Sort:           in.Sort,
		CoursesScanned: len(in.Courses),
		ScannedPages:   in.ScannedPages,
		Count:          len(items),
		Anonymized:     in.Anonymize,
		FetchFailures:  in.Failures,
		Items:          items,
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rows = append(rows, toGradeTableRow(it))
	}
	if len(items) == 0 {
		view.Note = fmt.Sprintf("no ungraded submissions found across %d course(s), %d page(s); ensure CANVAS_API_TOKEN is a teacher/admin token and raise --max-scan-pages", len(in.Courses), in.ScannedPages)
		rows = nil
	}
	return view, rows
}
