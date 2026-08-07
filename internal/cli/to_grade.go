// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

type toGradeItem struct {
	CourseID       string   `json:"course_id"`
	AssignmentID   string   `json:"assignment_id"`
	AssignmentName string   `json:"assignment,omitempty"`
	UserID         string   `json:"user_id"`
	Student        string   `json:"student,omitempty"`
	SubmittedAt    string   `json:"submitted_at,omitempty"`
	DaysWaiting    int      `json:"days_waiting"`
	PointsPossible *float64 `json:"points_possible,omitempty"`
	SubmissionType string   `json:"submission_type,omitempty"`
}

type toGradeView struct {
	Scope          string         `json:"scope"`
	Sort           string         `json:"sort"`
	CoursesScanned int            `json:"courses_scanned"`
	ScannedPages   int            `json:"scanned_pages"`
	Count          int            `json:"count"`
	Anonymized     bool           `json:"anonymized,omitempty"`
	FetchFailures  []fetchFailure `json:"fetch_failures,omitempty"`
	Note           string         `json:"note,omitempty"`
	Items          []toGradeItem  `json:"items"`
}

func newNovelToGradeCmd(flags *rootFlags) *cobra.Command {
	var flagCourse string
	var flagAll bool
	var flagSort string
	var limit int
	var anonymize bool
	var maxScanPages int
	var maxCourses int

	cmd := &cobra.Command{
		Use:   "to-grade",
		Short: "One queue of every submitted-but-ungraded item across your courses, oldest-first",
		Long: "Joins submissions to assignments across a course (or every course you teach with --all-my-courses) " +
			"and lists everything still awaiting a grade, aged by submission date. Canvas reports needs-grading " +
			"counts per assignment only; this is the single cross-course work queue. Read-only.",
		Example: "  canvas-pp-cli to-grade --all-my-courses --sort oldest --agent\n  canvas-pp-cli to-grade --course 12345 --limit 20",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "--course=12345;--sort=oldest",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch submissions + assignments and build an ungraded queue")
				return nil
			}
			if flagCourse == "" && !flagAll {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("one of --course <id> or --all-my-courses is required"))
			}
			if flagSort != "oldest" && flagSort != "newest" {
				flagSort = "oldest"
			}
			if done, err := verifyEmpty(cmd, flags, "items"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: builds the ungraded queue from API submissions
			// with no synced-store equivalent. Reject --data-source local.
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}

			var courses []courseRef
			if flagCourse != "" {
				courses = []courseRef{{ID: flagCourse}}
			} else {
				cs, cerr := teacherCourses(ctx, c, maxScanPages)
				if cerr != nil {
					return classifyAPIError(cerr, flags)
				}
				courses = cs
			}
			if maxCourses > 0 && len(courses) > maxCourses {
				courses = courses[:maxCourses]
			}

			// Fetch each course's submissions and student names up front so the
			// filtering and ordering run over data rather than over the network.
			var failures []fetchFailure
			totalPages := 0
			submissionsByCourse := map[string][]canvasObj{}
			namesByCourse := map[string]map[string]string{}
			for _, course := range courses {
				namesByCourse[course.ID] = studentNameMap(ctx, c, course.ID, maxScanPages)
				subs, pages, serr := canvasFetchList(ctx, c,
					"/api/v1/courses/"+course.ID+"/students/submissions",
					map[string]string{"student_ids[]": "all", "include[]": "assignment"}, maxScanPages)
				totalPages += pages
				if serr != nil {
					failures = append(failures, fetchFailure{Scope: "course:" + course.ID, Error: serr.Error()})
					continue
				}
				submissionsByCourse[course.ID] = subs
			}

			scope := "course " + flagCourse
			if flagAll {
				scope = "all my courses"
			}
			view, rows := analyzeToGrade(toGradeInput{
				Scope:               scope,
				Sort:                flagSort,
				Courses:             courses,
				SubmissionsByCourse: submissionsByCourse,
				NamesByCourse:       namesByCourse,
				Now:                 time.Now(),
				Limit:               limit,
				Anonymize:           anonymize,
				ScannedPages:        totalPages,
				Failures:            failures,
			})
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d course fetch(es) failed; queue covers the remaining %d course(s)\n", len(failures), len(courses)-len(failures))
			}
			return emitNovel(cmd, flags, view, rows)
		},
	}
	cmd.Flags().StringVar(&flagCourse, "course", "", "Course ID to scan (omit and use --all-my-courses)")
	cmd.Flags().BoolVar(&flagAll, "all-my-courses", false, "Scan every active course where you are a teacher")
	cmd.Flags().StringVar(&flagSort, "sort", "oldest", "Order: oldest or newest (by submission date)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum items to return (0 = all)")
	cmd.Flags().BoolVar(&anonymize, "anonymize", false, "Hash student names for safe sharing with agents")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum submission pages to scan per course (100 per page)")
	cmd.Flags().IntVar(&maxCourses, "max-courses", 25, "Maximum courses to scan under --all-my-courses")
	return cmd
}

// needsGrading reports whether a Canvas submission is submitted but not yet
// graded (workflow_state submitted/pending_review, has a submission, no score,
// not excused).
func needsGrading(s canvasObj) bool {
	if s.boolv("excused") {
		return false
	}
	ws := s.str("workflow_state")
	if ws != "submitted" && ws != "pending_review" {
		return false
	}
	if s.str("submitted_at") == "" {
		return false
	}
	if s.present("graded_at") {
		return false
	}
	if _, scored := s.num("score"); scored {
		return false
	}
	return true
}
