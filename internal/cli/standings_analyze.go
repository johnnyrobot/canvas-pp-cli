// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"sort"
)

// standingsDefinitions states the pass/DFW thresholds in the output so a reader
// never has to infer them. It is the same rule letterBucket implements: pass is
// C or better.
const standingsDefinitions = "pass = score >= 70 (C+); dfw = score < 70 (D/F); rates over graded students only"

// standingsInput is everything analyzeStandings needs. The caller fetches the
// term's courses and each course's student enrollments, plus section names when
// grouping by section.
type standingsInput struct {
	Term    string
	Account string
	// By is "course" or "section". Anything else is treated as "course",
	// matching the flag default.
	By string
	// Courses as returned by /accounts/{id}/courses?enrollment_term_id=.
	Courses []canvasObj
	// EnrollmentsByCourse holds student enrollments keyed by course id. A
	// course absent from the map contributes nothing — that is how a failed
	// fetch is represented, alongside an entry in Failures.
	EnrollmentsByCourse map[string][]canvasObj
	// SectionNamesByCourse maps course id -> section id -> section name. Only
	// populated when By is "section"; a missing name leaves the group unnamed.
	SectionNamesByCourse map[string]map[string]string
	ScannedPages         int
	Failures             []fetchFailure
}

// standingsGroupKey decides which rollup bucket an enrollment belongs to, and
// the display name for that bucket.
func standingsGroupKey(in standingsInput, course canvasObj, courseID string, en canvasObj) (key, name string) {
	if in.By == "section" {
		key = en.str("course_section_id")
		return key, in.SectionNamesByCourse[courseID][key]
	}
	return courseID, course.str("name")
}

// analyzeStandings rolls fetched enrollments up into grade distributions per
// group plus an overall total. Pure: no HTTP, no cobra, no writers.
//
// Rows are always nil — this command renders as JSON because a group carries a
// nested distribution rather than flat columns.
func analyzeStandings(in standingsInput) (standingsView, []map[string]any) {
	groups := map[string]*standingsAcc{}
	overall := &standingsAcc{}

	for _, course := range in.Courses {
		courseID := course.str("id")
		if courseID == "" {
			continue
		}
		for _, en := range in.EnrollmentsByCourse[courseID] {
			score, scored := en.obj("grades").num("current_score")
			key, name := standingsGroupKey(in, course, courseID, en)
			acc := groups[key]
			if acc == nil {
				acc = &standingsAcc{name: name}
				groups[key] = acc
			}
			acc.add(score, scored)
			overall.add(score, scored)
		}
	}

	out := make([]standingsGroup, 0, len(groups))
	for k, acc := range groups {
		out = append(out, acc.group(k))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	overallGroup := overall.group("overall")
	view := standingsView{
		Term:           in.Term,
		Account:        in.Account,
		By:             in.By,
		CoursesScanned: len(in.Courses),
		ScannedPages:   in.ScannedPages,
		Definitions:    standingsDefinitions,
		Overall:        overallGroup,
		Groups:         out,
		FetchFailures:  in.Failures,
	}
	if overallGroup.Students == 0 {
		view.Note = fmt.Sprintf("no student enrollments found for term %s under account %s; confirm the term id, account, and that CANVAS_API_TOKEN has admin scope", in.Term, in.Account)
	}
	return view, nil
}
