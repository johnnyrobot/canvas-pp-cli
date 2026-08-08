// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"canvas-pp-cli/internal/client"
	"canvas-pp-cli/internal/cliutil"
)

type atRiskItem struct {
	CourseID       string   `json:"course_id"`
	AssignmentID   string   `json:"assignment_id"`
	AssignmentName string   `json:"assignment,omitempty"`
	Status         string   `json:"status"` // missing | late | unsubmitted
	DueAt          string   `json:"due_at,omitempty"`
	PointsPossible *float64 `json:"points_possible,omitempty"`
}

type atRiskStudent struct {
	UserID      string       `json:"user_id"`
	Name        string       `json:"name"`
	CourseIDs   []string     `json:"course_ids,omitempty"`
	Missing     int          `json:"missing"`
	Late        int          `json:"late"`
	Unsubmitted int          `json:"unsubmitted"`
	Total       int          `json:"total_concerns"`
	Items       []atRiskItem `json:"items,omitempty"`
}

type atRiskView struct {
	Scope          string          `json:"scope"`
	Since          string          `json:"since,omitempty"`
	CoursesScanned int             `json:"courses_scanned"`
	ScannedPages   int             `json:"scanned_pages"`
	Anonymized     bool            `json:"anonymized,omitempty"`
	FetchFailures  []fetchFailure  `json:"fetch_failures,omitempty"`
	Note           string          `json:"note,omitempty"`
	Students       []atRiskStudent `json:"students"`
}

func newNovelAtRiskCmd(flags *rootFlags) *cobra.Command {
	var flagCourse string
	var flagAll bool
	var flagSince string
	var anonymize bool
	var maxScanPages int
	var maxCourses int

	cmd := &cobra.Command{
		Use:   "at-risk",
		Short: "Students with missing/late/unsubmitted work across one or all of your courses, time-windowed",
		Long: "Aggregates Canvas's per-submission missing/late flags across a course (or every course you teach " +
			"with --all-my-courses), optionally limited to assignments due within --since, and ranks students by " +
			"concern count. No single Canvas endpoint spans all your courses or a time window. Read-only.",
		Example: "  canvas-pp-cli at-risk --course 12345 --since 14d --agent\n  canvas-pp-cli at-risk --all-my-courses --since 30d --anonymize",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "--course=12345;--since=14d",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch submissions + assignments + enrollments and rank at-risk students")
				return nil
			}
			if flagCourse == "" && !flagAll {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("one of --course <id> or --all-my-courses is required"))
			}
			var cutoff time.Time
			if flagSince != "" {
				dur, derr := cliutil.ParseDurationLoose(flagSince)
				if derr != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --since %q: %w", flagSince, derr))
				}
				cutoff = time.Now().Add(-dur)
			}
			if done, err := verifyEmpty(cmd, flags, "students"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: spans courses/submissions from the API with no
			// synced-store equivalent. Reject --data-source local.
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
			// aggregation and ranking run over data rather than over the network.
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
			view, rows := analyzeAtRisk(atRiskInput{
				Scope:               scope,
				Since:               flagSince,
				Courses:             courses,
				SubmissionsByCourse: submissionsByCourse,
				NamesByCourse:       namesByCourse,
				Cutoff:              cutoff,
				Anonymize:           anonymize,
				ScannedPages:        totalPages,
				Failures:            failures,
			})
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d course fetch(es) failed; results cover the remaining %d course(s)\n", len(failures), len(courses)-len(failures))
			}
			return emitNovel(cmd, flags, view, rows)
		},
	}
	cmd.Flags().StringVar(&flagCourse, "course", "", "Course ID to scan (omit and use --all-my-courses for every course you teach)")
	cmd.Flags().BoolVar(&flagAll, "all-my-courses", false, "Scan every active course where you are a teacher")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only count assignments due within this window (e.g. 14d, 1w, 72h)")
	cmd.Flags().BoolVar(&anonymize, "anonymize", false, "Hash student names for safe sharing with agents")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum submission pages to scan per course (100 per page)")
	cmd.Flags().IntVar(&maxCourses, "max-courses", 25, "Maximum courses to scan under --all-my-courses")
	return cmd
}

// classifyAtRisk returns "missing", "late", "unsubmitted", or "" for a Canvas
// submission, optionally filtered to assignments due on/after cutoff.
func classifyAtRisk(s canvasObj, cutoff time.Time, useCutoff bool) string {
	if s.boolv("excused") {
		return ""
	}
	if useCutoff {
		due := s.obj("assignment").str("due_at")
		if due == "" {
			// No due date: can't place it in the --since window, so exclude it
			// rather than reporting it for every window.
			return ""
		}
		if t, err := time.Parse(time.RFC3339, due); err == nil && t.Before(cutoff) {
			return ""
		}
	}
	if s.boolv("missing") {
		return "missing"
	}
	if s.boolv("late") {
		return "late"
	}
	if s.str("workflow_state") == "unsubmitted" {
		due := s.obj("assignment").str("due_at")
		if due != "" {
			if t, err := time.Parse(time.RFC3339, due); err == nil && t.Before(time.Now()) {
				return "unsubmitted"
			}
		}
	}
	return ""
}

// studentNameMap returns user_id -> display name for a course's student
// enrollments (best-effort; empty map on failure).
func studentNameMap(ctx context.Context, c *client.Client, courseID string, maxPages int) map[string]string {
	out := map[string]string{}
	items, _, err := canvasFetchList(ctx, c,
		"/api/v1/courses/"+courseID+"/enrollments",
		map[string]string{"type[]": "StudentEnrollment", "include[]": "user"}, maxPages)
	if err != nil {
		return out
	}
	for _, e := range items {
		if uid := e.str("user_id"); uid != "" {
			out[uid] = e.obj("user").str("name")
		}
	}
	return out
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
