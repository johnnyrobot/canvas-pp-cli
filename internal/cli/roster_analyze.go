// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import "fmt"

// rosterInput is everything analyzeRoster needs. The caller owns fetching;
// this type is the seam between fetching and the join.
type rosterInput struct {
	CourseID string
	// Enrollments as returned by /courses/{id}/enrollments?include[]=user.
	Enrollments []canvasObj
	// SectionNames maps section id -> name. Best-effort: the roster still
	// builds when it is empty, students just carry no section.
	SectionNames map[string]string
	Anonymize    bool
	ScannedPages int
}

// rosterRowFor joins one enrollment with its user and section into a single
// roster row. This is the rule that makes a roster a roster: no Canvas
// endpoint returns these fields together.
func rosterRowFor(e canvasObj, sectionNames map[string]string, anonymize bool) rosterStudent {
	user := e.obj("user")
	name := user.str("name")
	if name == "" {
		name = e.str("user_id")
	}
	rs := rosterStudent{
		UserID:         e.str("user_id"),
		Name:           name,
		SortableName:   user.str("sortable_name"),
		LoginID:        user.str("login_id"),
		SISUserID:      e.str("sis_user_id"),
		Section:        sectionNames[e.str("course_section_id")],
		Role:           e.str("role"),
		State:          e.str("enrollment_state"),
		CurrentGrade:   e.obj("grades").str("current_grade"),
		LastActivityAt: e.str("last_activity_at"),
	}
	if grades := e.obj("grades"); grades != nil {
		if v, ok := grades.num("current_score"); ok {
			rs.CurrentScore = &v
		}
		if v, ok := grades.num("final_score"); ok {
			rs.FinalScore = &v
		}
	}
	if anonymize {
		rs.Name = anonLabel("student", rs.SortableName+rs.Name+rs.UserID)
		rs.SortableName = ""
		rs.LoginID = ""
		rs.SISUserID = ""
	}
	return rs
}

// rosterTableRow projects a roster row down to the columns the human table
// renders. The JSON view always carries the full row.
func rosterTableRow(rs rosterStudent) map[string]any {
	row := map[string]any{
		"user_id": rs.UserID,
		"name":    rs.Name,
		"section": rs.Section,
		"role":    rs.Role,
	}
	if rs.CurrentScore != nil {
		row["current_score"] = *rs.CurrentScore
	}
	return row
}

// analyzeRoster joins fetched enrollments into the roster view and its table
// projection. Pure: no HTTP, no cobra, no writers.
//
// A nil rows return forces JSON output, which is how the empty-roster note
// stays visible.
func analyzeRoster(in rosterInput) (rosterView, []map[string]any) {
	students := make([]rosterStudent, 0, len(in.Enrollments))
	rows := make([]map[string]any, 0, len(in.Enrollments))
	for _, e := range in.Enrollments {
		rs := rosterRowFor(e, in.SectionNames, in.Anonymize)
		students = append(students, rs)
		rows = append(rows, rosterTableRow(rs))
	}

	view := rosterView{
		CourseID:     in.CourseID,
		Count:        len(students),
		ScannedPages: in.ScannedPages,
		Anonymized:   in.Anonymize,
		Students:     students,
	}
	if len(students) == 0 {
		view.Note = fmt.Sprintf("no enrollments returned for course %s (scanned %d page(s)); set CANVAS_API_TOKEN/CANVAS_BASE_URL, confirm the course id, or raise --max-scan-pages", in.CourseID, in.ScannedPages)
		rows = nil // force JSON so the note is visible
	}
	return view, rows
}
