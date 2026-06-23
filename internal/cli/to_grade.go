// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"sort"
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
		Example: "  canvas-cli to-grade --all-my-courses --sort oldest --agent\n  canvas-cli to-grade --course 12345 --limit 20",
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

			var items []toGradeItem
			var failures []fetchFailure
			totalPages := 0
			now := time.Now()
			for _, course := range courses {
				names := studentNameMap(ctx, c, course.ID, maxScanPages)
				subs, pages, serr := canvasFetchList(ctx, c,
					"/api/v1/courses/"+course.ID+"/students/submissions",
					map[string]string{"student_ids[]": "all", "include[]": "assignment"}, maxScanPages)
				totalPages += pages
				if serr != nil {
					failures = append(failures, fetchFailure{Scope: "course:" + course.ID, Error: serr.Error()})
					continue
				}
				for _, s := range subs {
					if !needsGrading(s) {
						continue
					}
					it := toGradeItem{
						CourseID:       course.ID,
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
					if t, perr := time.Parse(time.RFC3339, it.SubmittedAt); perr == nil {
						it.DaysWaiting = int(now.Sub(t).Hours() / 24)
					}
					if anonymize {
						it.Student = anonLabel("student", it.UserID)
					}
					items = append(items, it)
				}
			}

			sort.Slice(items, func(i, j int) bool {
				if flagSort == "newest" {
					return items[i].SubmittedAt > items[j].SubmittedAt
				}
				return items[i].SubmittedAt < items[j].SubmittedAt
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}
			if items == nil {
				items = []toGradeItem{}
			}

			scope := "course " + flagCourse
			if flagAll {
				scope = "all my courses"
			}
			view := toGradeView{
				Scope:          scope,
				Sort:           flagSort,
				CoursesScanned: len(courses),
				ScannedPages:   totalPages,
				Count:          len(items),
				Anonymized:     anonymize,
				FetchFailures:  failures,
				Items:          items,
			}
			rows := make([]map[string]any, 0, len(items))
			for _, it := range items {
				rows = append(rows, map[string]any{
					"course_id": it.CourseID, "assignment": it.AssignmentName,
					"student": it.Student, "submitted_at": it.SubmittedAt, "days_waiting": it.DaysWaiting,
				})
			}
			if len(items) == 0 {
				view.Note = fmt.Sprintf("no ungraded submissions found across %d course(s), %d page(s); ensure CANVAS_API_TOKEN is a teacher/admin token and raise --max-scan-pages", len(courses), totalPages)
				rows = nil
			}
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
