// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"time"
)

// atRiskInput is everything analyzeAtRisk needs. The caller fetches each
// course's submissions and student-name map.
type atRiskInput struct {
	// Scope describes what was scanned, e.g. "course 12345" or "all my courses".
	Scope string
	// Since is the raw --since text, reported back on the view.
	Since   string
	Courses []courseRef
	// SubmissionsByCourse holds submissions keyed by course id. A course absent
	// from the map contributes nothing — that is how a failed fetch is
	// represented, alongside an entry in Failures.
	SubmissionsByCourse map[string][]canvasObj
	// NamesByCourse maps course id -> user id -> display name.
	NamesByCourse map[string]map[string]string
	// Cutoff limits the scan to assignments due on or after it. The zero value
	// means no window.
	Cutoff       time.Time
	Anonymize    bool
	ScannedPages int
	Failures     []fetchFailure
}

// lessAtRisk orders two at-risk students for the ranked output: more concerns
// first, ties broken by user ID.
//
// Extracted verbatim from the sort.Slice closure in at_risk.go so the ordering
// rule can be exercised directly. Behaviour is unchanged by this extraction.
func lessAtRisk(a, b atRiskStudent) bool {
	if a.Total != b.Total {
		return a.Total > b.Total
	}
	return a.UserID < b.UserID
}

// atRiskTableRow projects a student down to the columns the human table
// renders. The JSON view always carries the full record including items.
func atRiskTableRow(st atRiskStudent) map[string]any {
	return map[string]any{
		"user_id": st.UserID, "name": st.Name,
		"missing": st.Missing, "late": st.Late, "total": st.Total,
	}
}

// atRiskItemFor builds the per-submission concern record.
func atRiskItemFor(s canvasObj, courseID, status string) atRiskItem {
	assignment := s.obj("assignment")
	item := atRiskItem{
		CourseID:       courseID,
		AssignmentID:   s.str("assignment_id"),
		AssignmentName: assignment.str("name"),
		Status:         status,
		DueAt:          assignment.str("due_at"),
	}
	if pp, ok := assignment.num("points_possible"); ok {
		item.PointsPossible = &pp
	}
	return item
}

// analyzeAtRisk aggregates fetched submissions into ranked at-risk students.
// Pure: no HTTP, no cobra, no writers.
//
// A student's display name comes from the first course they appear in, and a
// nil rows return forces JSON so the empty-result note stays visible.
func analyzeAtRisk(in atRiskInput) (atRiskView, []map[string]any) {
	useCutoff := !in.Cutoff.IsZero()

	byStudent := map[string]*atRiskStudent{}
	for _, course := range in.Courses {
		names := in.NamesByCourse[course.ID]
		for _, s := range in.SubmissionsByCourse[course.ID] {
			status := classifyAtRisk(s, in.Cutoff, useCutoff)
			if status == "" {
				continue
			}
			uid := s.str("user_id")
			st := byStudent[uid]
			if st == nil {
				nm := names[uid]
				if nm == "" {
					nm = "user " + uid
				}
				st = &atRiskStudent{UserID: uid, Name: nm}
				byStudent[uid] = st
			}
			if !containsStr(st.CourseIDs, course.ID) {
				st.CourseIDs = append(st.CourseIDs, course.ID)
			}
			st.Items = append(st.Items, atRiskItemFor(s, course.ID, status))
			switch status {
			case "missing":
				st.Missing++
			case "late":
				st.Late++
			case "unsubmitted":
				st.Unsubmitted++
			}
			st.Total++
		}
	}

	students := make([]atRiskStudent, 0, len(byStudent))
	for _, st := range byStudent {
		if in.Anonymize {
			st.Name = anonLabel("student", st.UserID)
		}
		students = append(students, *st)
	}
	sort.Slice(students, func(i, j int) bool {
		return lessAtRisk(students[i], students[j])
	})

	view := atRiskView{
		Scope:          in.Scope,
		Since:          in.Since,
		CoursesScanned: len(in.Courses),
		ScannedPages:   in.ScannedPages,
		Anonymized:     in.Anonymize,
		FetchFailures:  in.Failures,
		Students:       students,
	}
	rows := make([]map[string]any, 0, len(students))
	for _, st := range students {
		rows = append(rows, atRiskTableRow(st))
	}
	if len(students) == 0 {
		view.Note = fmt.Sprintf("no at-risk submissions found across %d course(s), %d page(s); ensure CANVAS_API_TOKEN is a teacher/admin token and raise --max-scan-pages for large courses", len(in.Courses), in.ScannedPages)
		rows = nil
	}
	return view, rows
}
