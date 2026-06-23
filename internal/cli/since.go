// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"canvas-cli/internal/cliutil"
)

type sinceChange struct {
	CourseID string `json:"course_id"`
	Kind     string `json:"kind"` // submission | announcement | enrollment
	Who      string `json:"who,omitempty"`
	What     string `json:"what,omitempty"`
	At       string `json:"at,omitempty"`
}

type sinceView struct {
	Window           string         `json:"window"`
	Cutoff           string         `json:"cutoff"`
	CoursesScanned   int            `json:"courses_scanned"`
	ScannedPages     int            `json:"scanned_pages"`
	NewSubmissions   int            `json:"new_submissions"`
	NewAnnouncements int            `json:"new_announcements"`
	NewEnrollments   int            `json:"new_enrollments"`
	Anonymized       bool           `json:"anonymized,omitempty"`
	FetchFailures    []fetchFailure `json:"fetch_failures,omitempty"`
	Note             string         `json:"note,omitempty"`
	Changes          []sinceChange  `json:"changes"`
}

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var flagCourse string
	var anonymize bool
	var maxScanPages int
	var maxCourses int

	cmd := &cobra.Command{
		Use:   "since <window>",
		Short: "What changed: new submissions, announcements, and enrollments within a time window",
		Long: "A standup-style digest of recent course activity in <window> (e.g. 24h, 7d). Diffs across submissions, " +
			"announcements, and enrollments at once — activity streams are per-user and ephemeral, so no single " +
			"endpoint answers this. Read-only.",
		Example: "  canvas-cli since 24h --course 12345 --agent\n  canvas-cli since 7d --agent",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "window=24h;--course=12345",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff submissions + announcements + enrollments within the window")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a time window is required, e.g. 'since 24h'"))
			}
			dur, derr := cliutil.ParseDurationLoose(args[0])
			if derr != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid window %q: %w", args[0], derr))
			}
			cutoff := time.Now().Add(-dur)
			cutoffStr := cutoff.Format(time.RFC3339)
			if done, err := verifyEmpty(cmd, flags, "changes"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: diffs API activity within the window with no
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

			var changes []sinceChange
			var failures []fetchFailure
			totalPages := 0
			nSub, nAnn, nEnr := 0, 0, 0
			for _, course := range courses {
				names := studentNameMap(ctx, c, course.ID, maxScanPages)

				// New submissions since cutoff.
				subs, p1, e1 := canvasFetchList(ctx, c,
					"/api/v1/courses/"+course.ID+"/students/submissions",
					map[string]string{"student_ids[]": "all", "submitted_since": cutoffStr, "include[]": "assignment"}, maxScanPages)
				totalPages += p1
				if e1 != nil {
					failures = append(failures, fetchFailure{Scope: "submissions:" + course.ID, Error: e1.Error()})
				}
				for _, s := range subs {
					at := s.str("submitted_at")
					if !afterCutoff(at, cutoff) {
						continue
					}
					who := names[s.str("user_id")]
					if anonymize {
						who = anonLabel("student", s.str("user_id"))
					}
					changes = append(changes, sinceChange{
						CourseID: course.ID, Kind: "submission",
						Who: who, What: s.obj("assignment").str("name"), At: at,
					})
					nSub++
				}

				// New announcements.
				anns, p2, e2 := canvasFetchList(ctx, c, "/api/v1/announcements",
					map[string]string{"context_codes[]": "course_" + course.ID, "start_date": cutoffStr, "active_only": "true"}, 2)
				totalPages += p2
				if e2 != nil {
					failures = append(failures, fetchFailure{Scope: "announcements:" + course.ID, Error: e2.Error()})
				}
				for _, a := range anns {
					at := a.str("posted_at")
					changes = append(changes, sinceChange{
						CourseID: course.ID, Kind: "announcement",
						What: a.str("title"), At: at,
					})
					nAnn++
				}

				// New enrollments.
				enr, p3, e3 := canvasFetchList(ctx, c, "/api/v1/courses/"+course.ID+"/enrollments", nil, maxScanPages)
				totalPages += p3
				if e3 != nil {
					failures = append(failures, fetchFailure{Scope: "enrollments:" + course.ID, Error: e3.Error()})
				}
				for _, e := range enr {
					at := e.str("created_at")
					if !afterCutoff(at, cutoff) {
						continue
					}
					who := e.obj("user").str("name")
					if who == "" {
						who = names[e.str("user_id")]
					}
					if anonymize {
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
				Window:           args[0],
				Cutoff:           cutoffStr,
				CoursesScanned:   len(courses),
				ScannedPages:     totalPages,
				NewSubmissions:   nSub,
				NewAnnouncements: nAnn,
				NewEnrollments:   nEnr,
				Anonymized:       anonymize,
				FetchFailures:    failures,
				Changes:          changes,
			}
			if len(changes) == 0 {
				view.Note = fmt.Sprintf("no activity in the last %s across %d course(s); set CANVAS_API_TOKEN/CANVAS_BASE_URL or widen the window", args[0], len(courses))
			}
			return emitNovel(cmd, flags, view, nil)
		},
	}
	cmd.Flags().StringVar(&flagCourse, "course", "", "Course ID (omit to scan every course you teach)")
	cmd.Flags().BoolVar(&anonymize, "anonymize", false, "Hash names for safe sharing with agents")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum pages to scan per resource (100 per page)")
	cmd.Flags().IntVar(&maxCourses, "max-courses", 25, "Maximum courses to scan when --course is omitted")
	return cmd
}

func afterCutoff(ts string, cutoff time.Time) bool {
	if ts == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true // can't parse; don't drop it
	}
	return !t.Before(cutoff)
}
