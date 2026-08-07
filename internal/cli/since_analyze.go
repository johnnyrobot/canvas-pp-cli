// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"time"
)

// sinceInput is everything analyzeSince needs. The caller fetches three
// streams per course — submissions, announcements and enrollments — plus a
// student-name map.
type sinceInput struct {
	// Window is the raw window argument, e.g. "24h", reported back on the view.
	Window string
	// Cutoff is the start of the window.
	Cutoff time.Time
	// CutoffStr is Cutoff in RFC3339, echoed on the view.
	CutoffStr string
	Courses   []courseRef
	// Each map is keyed by course id. Unlike the other novel commands, a fetch
	// error here does not discard the course: whatever the API returned before
	// failing is still analyzed, and the failure is recorded alongside it.
	SubmissionsByCourse   map[string][]canvasObj
	AnnouncementsByCourse map[string][]canvasObj
	EnrollmentsByCourse   map[string][]canvasObj
	// NamesByCourse maps course id -> user id -> display name.
	NamesByCourse map[string]map[string]string
	Anonymize     bool
	ScannedPages  int
	Failures      []fetchFailure
}

// analyzeSince merges three fetched activity streams into one digest of what
// changed in the window. Pure: no HTTP, no cobra, no writers.
//
// Rows are always nil — this command renders as JSON because a change carries
// a kind-dependent shape rather than uniform columns.
func analyzeSince(in sinceInput) (sinceView, []map[string]any) {
	var changes []sinceChange
	nSub, nAnn, nEnr := 0, 0, 0

	for _, course := range in.Courses {
		names := in.NamesByCourse[course.ID]

		// New submissions since cutoff.
		for _, s := range in.SubmissionsByCourse[course.ID] {
			at := s.str("submitted_at")
			if !afterCutoff(at, in.Cutoff) {
				continue
			}
			who := names[s.str("user_id")]
			if in.Anonymize {
				who = anonLabel("student", s.str("user_id"))
			}
			changes = append(changes, sinceChange{
				CourseID: course.ID, Kind: "submission",
				Who: who, What: s.obj("assignment").str("name"), At: at,
			})
			nSub++
		}

		// New announcements. The API filters these by start_date, so there is
		// no client-side cutoff check.
		for _, a := range in.AnnouncementsByCourse[course.ID] {
			changes = append(changes, sinceChange{
				CourseID: course.ID, Kind: "announcement",
				What: a.str("title"), At: a.str("posted_at"),
			})
			nAnn++
		}

		// New enrollments.
		for _, e := range in.EnrollmentsByCourse[course.ID] {
			at := e.str("created_at")
			if !afterCutoff(at, in.Cutoff) {
				continue
			}
			who := e.obj("user").str("name")
			if who == "" {
				who = names[e.str("user_id")]
			}
			if in.Anonymize {
				who = anonLabel("user", e.str("user_id"))
			}
			changes = append(changes, sinceChange{
				CourseID: course.ID, Kind: "enrollment",
				Who: who, What: e.str("type"), At: at,
			})
			nEnr++
		}
	}
	if changes == nil {
		changes = []sinceChange{}
	}

	view := sinceView{
		Window:           in.Window,
		Cutoff:           in.CutoffStr,
		CoursesScanned:   len(in.Courses),
		ScannedPages:     in.ScannedPages,
		NewSubmissions:   nSub,
		NewAnnouncements: nAnn,
		NewEnrollments:   nEnr,
		Anonymized:       in.Anonymize,
		FetchFailures:    in.Failures,
		Changes:          changes,
	}
	if len(changes) == 0 {
		view.Note = fmt.Sprintf("no activity in the last %s across %d course(s); set CANVAS_API_TOKEN/CANVAS_BASE_URL or widen the window", in.Window, len(in.Courses))
	}
	return view, nil
}
