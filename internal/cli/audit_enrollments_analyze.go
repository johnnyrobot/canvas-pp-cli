// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import "fmt"

// auditInput is everything analyzeAuditEnrollments needs. The caller fetches
// account courses and, when the orphan check is on, each course's active
// student enrollments.
type auditInput struct {
	Account string
	// Courses as returned by /accounts/{id}/courses (include[]=teachers when
	// the ghost-teacher check is on).
	Courses []canvasObj
	// EnrollmentsByCourse holds active student enrollments keyed by course id.
	// A course missing from the map contributes no orphans — that is how a
	// failed enrollment fetch is represented, alongside an entry in Failures.
	EnrollmentsByCourse map[string][]canvasObj
	CheckGhostTeachers  bool
	CheckOrphans        bool
	Anonymize           bool
	ScannedPages        int
	Failures            []fetchFailure
}

// orphanReason is the explanation attached to every flagged student.
const orphanReason = "active enrollment, never accessed the course (no last_activity_at)"

// isGhostCourse reports whether a course has no teacher enrolled. Requires the
// course to have been fetched with include[]=teachers; without it every course
// looks teacher-less.
func isGhostCourse(course canvasObj) bool {
	return len(course.list("teachers")) == 0
}

// isOrphanEnrollment reports whether an active student has never accessed the
// course. Canvas leaves last_activity_at absent until first access.
func isOrphanEnrollment(en canvasObj) bool {
	return !en.present("last_activity_at")
}

// auditChecks names the checks that ran, in report order.
func auditChecks(ghostTeachers, orphans bool) []string {
	checks := []string{}
	if ghostTeachers {
		checks = append(checks, "ghost-teachers")
	}
	if orphans {
		checks = append(checks, "orphans")
	}
	return checks
}

// analyzeAuditEnrollments applies the anomaly checks to fetched courses and
// enrollments. Pure: no HTTP, no cobra, no writers.
//
// Rows are always nil — this command renders as JSON because its result is two
// separate lists rather than one table.
func analyzeAuditEnrollments(in auditInput) (auditView, []map[string]any) {
	ghosts := []ghostCourse{}
	orphans := []orphanStudent{}

	for _, course := range in.Courses {
		courseID := course.str("id")
		if courseID == "" {
			continue
		}
		if in.CheckGhostTeachers && isGhostCourse(course) {
			ghosts = append(ghosts, ghostCourse{
				CourseID: courseID,
				Name:     course.str("name"),
				State:    course.str("workflow_state"),
			})
		}
		if in.CheckOrphans {
			for _, en := range in.EnrollmentsByCourse[courseID] {
				if !isOrphanEnrollment(en) {
					continue
				}
				name := en.obj("user").str("name")
				userID := en.str("user_id")
				if in.Anonymize {
					// Same label in UserID and Name, derived from the real id,
					// so rows stay joinable across commands without carrying
					// the real id an admin could resolve to a named student.
					label := anonLabel("student", userID)
					userID = label
					name = label
				}
				orphans = append(orphans, orphanStudent{
					CourseID:        courseID,
					UserID:          userID,
					Name:            name,
					EnrollmentState: en.str("enrollment_state"),
					Reason:          orphanReason,
				})
			}
		}
	}

	view := auditView{
		Account:             in.Account,
		Checks:              auditChecks(in.CheckGhostTeachers, in.CheckOrphans),
		CoursesScanned:      len(in.Courses),
		ScannedPages:        in.ScannedPages,
		Anonymized:          in.Anonymize,
		GhostTeacherCourses: ghosts,
		OrphanStudents:      orphans,
		FetchFailures:       in.Failures,
	}
	if len(in.Courses) == 0 {
		view.Note = fmt.Sprintf("no courses returned for account %s; confirm the account id and that CANVAS_API_TOKEN has admin scope", in.Account)
	} else if len(ghosts) == 0 && len(orphans) == 0 {
		view.Note = fmt.Sprintf("no anomalies found across %d course(s)", len(in.Courses))
	}
	return view, nil
}
