// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type rosterStudent struct {
	UserID         string   `json:"user_id"`
	Name           string   `json:"name"`
	SortableName   string   `json:"sortable_name,omitempty"`
	LoginID        string   `json:"login_id,omitempty"`
	SISUserID      string   `json:"sis_user_id,omitempty"`
	Section        string   `json:"section,omitempty"`
	Role           string   `json:"role,omitempty"`
	State          string   `json:"enrollment_state,omitempty"`
	CurrentScore   *float64 `json:"current_score,omitempty"`
	FinalScore     *float64 `json:"final_score,omitempty"`
	CurrentGrade   string   `json:"current_grade,omitempty"`
	LastActivityAt string   `json:"last_activity_at,omitempty"`
}

type rosterView struct {
	CourseID     string          `json:"course_id"`
	Count        int             `json:"count"`
	ScannedPages int             `json:"scanned_pages"`
	Anonymized   bool            `json:"anonymized,omitempty"`
	Note         string          `json:"note,omitempty"`
	Students     []rosterStudent `json:"students"`
}

func newNovelRosterCmd(flags *rootFlags) *cobra.Command {
	var anonymize bool
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "roster <course_id>",
		Short: "Unified course roster: enrollments + sections + grades in one row per student",
		Long: "Builds one row per student joining enrollments, the enrolled user, the section, " +
			"and the current/final score — a view no single Canvas endpoint returns. Read-only.",
		Example: "  canvas-pp-cli roster 12345 --agent\n  canvas-pp-cli roster 12345 --anonymize --select students.name,students.current_score",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "course_id=12345",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch enrollments + sections and join into a roster")
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("course_id is required"))
			}
			courseID := args[0]
			if done, err := verifyEmpty(cmd, flags, "students"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: roster joins enrollments+sections+grades from
			// the API and has no synced-store equivalent. Reject --data-source local.
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}

			// Section id -> name map (best-effort; roster still works without it).
			sectionName := map[string]string{}
			if secs, _, serr := canvasFetchList(ctx, c, "/api/v1/courses/"+courseID+"/sections", nil, 2); serr == nil {
				for _, s := range secs {
					sectionName[s.str("id")] = s.str("name")
				}
			}

			enrollments, pages, err := canvasFetchList(ctx, c,
				"/api/v1/courses/"+courseID+"/enrollments",
				map[string]string{"include[]": "user"}, maxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			view, rows := analyzeRoster(rosterInput{
				CourseID:     courseID,
				Enrollments:  enrollments,
				SectionNames: sectionName,
				Anonymize:    anonymize,
				ScannedPages: pages,
			})
			return emitNovel(cmd, flags, view, rows)
		},
	}
	cmd.Flags().BoolVar(&anonymize, "anonymize", false, "Hash names and drop login/SIS IDs for safe sharing with agents")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum enrollment pages to scan (100 per page)")
	return cmd
}
