// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type ghostCourse struct {
	CourseID string `json:"course_id"`
	Name     string `json:"name,omitempty"`
	State    string `json:"workflow_state,omitempty"`
}

type orphanStudent struct {
	CourseID        string `json:"course_id"`
	UserID          string `json:"user_id"`
	Name            string `json:"name,omitempty"`
	EnrollmentState string `json:"enrollment_state,omitempty"`
	Reason          string `json:"reason"`
}

type auditView struct {
	Account             string          `json:"account"`
	Checks              []string        `json:"checks"`
	CoursesScanned      int             `json:"courses_scanned"`
	ScannedPages        int             `json:"scanned_pages"`
	Anonymized          bool            `json:"anonymized,omitempty"`
	GhostTeacherCourses []ghostCourse   `json:"ghost_teacher_courses"`
	OrphanStudents      []orphanStudent `json:"orphan_students"`
	FetchFailures       []fetchFailure  `json:"fetch_failures,omitempty"`
	Note                string          `json:"note,omitempty"`
}

func newNovelAuditEnrollmentsCmd(flags *rootFlags) *cobra.Command {
	var flagAccount string
	var flagOrphans bool
	var flagGhostTeachers bool
	var anonymize bool
	var maxScanPages int
	var maxCourses int

	cmd := &cobra.Command{
		Use:   "audit-enrollments",
		Short: "Find enrollment anomalies in an account: teacher-less courses and never-accessed students",
		Long: "Scans an account's courses and enrollments for anomalies that no single endpoint surfaces: courses " +
			"with no teacher (--ghost-teachers) and active students who have never accessed the course " +
			"(--orphans, last_activity_at is null). With neither flag, runs both. Read-only.",
		Example: "  canvas-cli audit-enrollments --account 1 --ghost-teachers --agent\n  canvas-cli audit-enrollments --account 1 --orphans --anonymize",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "--account=1",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan account courses + enrollments for anomalies")
				return nil
			}
			if flagAccount == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--account <id> is required"))
			}
			// Default: run both checks when neither is requested.
			if !flagOrphans && !flagGhostTeachers {
				flagOrphans = true
				flagGhostTeachers = true
			}
			if done, err := verifyEmpty(cmd, flags, "orphan_students"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: scans account courses/enrollments from the API
			// with no synced-store equivalent. Reject --data-source local.
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}

			params := map[string]string{}
			if flagGhostTeachers {
				params["include[]"] = "teachers"
			}
			courses, pages, cerr := canvasFetchList(ctx, c,
				"/api/v1/accounts/"+flagAccount+"/courses", params, maxScanPages)
			if cerr != nil {
				return classifyAPIError(cerr, flags)
			}
			if maxCourses > 0 && len(courses) > maxCourses {
				courses = courses[:maxCourses]
			}

			ghosts := []ghostCourse{}
			orphans := []orphanStudent{}
			var failures []fetchFailure
			totalPages := pages
			checks := []string{}
			if flagGhostTeachers {
				checks = append(checks, "ghost-teachers")
			}
			if flagOrphans {
				checks = append(checks, "orphans")
			}

			for _, course := range courses {
				courseID := course.str("id")
				if courseID == "" {
					continue
				}
				if flagGhostTeachers {
					teachers := course.list("teachers")
					if len(teachers) == 0 {
						ghosts = append(ghosts, ghostCourse{
							CourseID: courseID,
							Name:     course.str("name"),
							State:    course.str("workflow_state"),
						})
					}
				}
				if flagOrphans {
					enr, p, e := canvasFetchList(ctx, c,
						"/api/v1/courses/"+courseID+"/enrollments",
						map[string]string{"type[]": "StudentEnrollment", "state[]": "active", "include[]": "user"}, maxScanPages)
					totalPages += p
					if e != nil {
						failures = append(failures, fetchFailure{Scope: "course:" + courseID, Error: e.Error()})
						continue
					}
					for _, en := range enr {
						if en.present("last_activity_at") {
							continue
						}
						name := en.obj("user").str("name")
						if anonymize {
							name = anonLabel("student", en.str("user_id"))
						}
						orphans = append(orphans, orphanStudent{
							CourseID:        courseID,
							UserID:          en.str("user_id"),
							Name:            name,
							EnrollmentState: en.str("enrollment_state"),
							Reason:          "active enrollment, never accessed the course (no last_activity_at)",
						})
					}
				}
			}

			view := auditView{
				Account:             flagAccount,
				Checks:              checks,
				CoursesScanned:      len(courses),
				ScannedPages:        totalPages,
				Anonymized:          anonymize,
				GhostTeacherCourses: ghosts,
				OrphanStudents:      orphans,
				FetchFailures:       failures,
			}
			if len(courses) == 0 {
				view.Note = fmt.Sprintf("no courses returned for account %s; confirm the account id and that CANVAS_API_TOKEN has admin scope", flagAccount)
			} else if len(ghosts) == 0 && len(orphans) == 0 {
				view.Note = fmt.Sprintf("no anomalies found across %d course(s)", len(courses))
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d course enrollment fetch(es) failed; audit covers the remaining courses\n", len(failures))
			}
			return emitNovel(cmd, flags, view, nil)
		},
	}
	cmd.Flags().StringVar(&flagAccount, "account", "", "Account ID to audit (required)")
	cmd.Flags().BoolVar(&flagOrphans, "orphans", false, "Flag active students who have never accessed their course")
	cmd.Flags().BoolVar(&flagGhostTeachers, "ghost-teachers", false, "Flag courses with no teacher enrollment")
	cmd.Flags().BoolVar(&anonymize, "anonymize", false, "Hash student names for safe sharing with agents")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum pages to scan per resource (100 per page)")
	cmd.Flags().IntVar(&maxCourses, "max-courses", 50, "Maximum courses to scan in the account")
	return cmd
}
