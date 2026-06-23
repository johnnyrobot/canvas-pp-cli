// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored transcendence command. Survives `generate --force` regen-merge.

// pp:data-source live

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type gradeDist struct {
	A        int `json:"A"`
	B        int `json:"B"`
	C        int `json:"C"`
	D        int `json:"D"`
	F        int `json:"F"`
	Ungraded int `json:"ungraded"`
}

type standingsGroup struct {
	Key      string    `json:"key"`
	Name     string    `json:"name,omitempty"`
	Students int       `json:"students"`
	Graded   int       `json:"graded"`
	Dist     gradeDist `json:"distribution"`
	AvgScore *float64  `json:"avg_score,omitempty"`
	PassRate *float64  `json:"pass_rate,omitempty"`
	DFWRate  *float64  `json:"dfw_rate,omitempty"`
}

type standingsView struct {
	Term           string           `json:"term"`
	Account        string           `json:"account"`
	By             string           `json:"by"`
	CoursesScanned int              `json:"courses_scanned"`
	ScannedPages   int              `json:"scanned_pages"`
	Definitions    string           `json:"definitions"`
	Overall        standingsGroup   `json:"overall"`
	Groups         []standingsGroup `json:"groups"`
	FetchFailures  []fetchFailure   `json:"fetch_failures,omitempty"`
	Note           string           `json:"note,omitempty"`
}

// standingsAcc accumulates scores for one group (course or section).
type standingsAcc struct {
	name   string
	dist   gradeDist
	scores []float64
}

func (a *standingsAcc) add(score float64, graded bool) {
	if !graded {
		a.dist.Ungraded++
		return
	}
	a.scores = append(a.scores, score)
	switch letterBucket(score) {
	case "A":
		a.dist.A++
	case "B":
		a.dist.B++
	case "C":
		a.dist.C++
	case "D":
		a.dist.D++
	default:
		a.dist.F++
	}
}

func (a *standingsAcc) group(key string) standingsGroup {
	g := standingsGroup{Key: key, Name: a.name, Dist: a.dist}
	g.Students = a.dist.A + a.dist.B + a.dist.C + a.dist.D + a.dist.F + a.dist.Ungraded
	g.Graded = len(a.scores)
	if g.Graded > 0 {
		var sum float64
		for _, s := range a.scores {
			sum += s
		}
		avg := sum / float64(g.Graded)
		pass := float64(a.dist.A+a.dist.B+a.dist.C) / float64(g.Graded)
		dfw := float64(a.dist.D+a.dist.F) / float64(g.Graded)
		g.AvgScore = &avg
		g.PassRate = &pass
		g.DFWRate = &dfw
	}
	return g
}

func newNovelStandingsCmd(flags *rootFlags) *cobra.Command {
	var flagTerm string
	var flagAccount string
	var flagBy string
	var maxScanPages int
	var maxCourses int

	cmd := &cobra.Command{
		Use:   "standings",
		Short: "Term-wide grade distribution and pass/DFW rollups across courses",
		Long: "Pulls every course in a term under an account, recomputes each student's standing from enrollment " +
			"grades, and rolls the distribution up by course or section. Department analytics is coarse and " +
			"account-scoped; this is consistent per-course math aggregated locally. Read-only. " +
			"Pass = C or better (score >= 70); DFW = D/F (score < 70).",
		Example: "  canvas-pp-cli standings --term 7 --by course --agent\n  canvas-pp-cli standings --term 7 --account 1 --by section",
		Annotations: map[string]string{
			"mcp:read-only":          "true",
			"pp:happy-args":          "--term=1;--account=1",
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch term courses + enrollments and roll up grade distributions")
				return nil
			}
			if flagTerm == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--term <enrollment_term_id> is required"))
			}
			if flagBy != "course" && flagBy != "section" {
				flagBy = "course"
			}
			if done, err := verifyEmpty(cmd, flags, "groups"); done {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Live-only command: rolls up term standings from the API with no
			// synced-store equivalent. Reject --data-source local.
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}

			courses, cpages, cerr := canvasFetchList(ctx, c,
				"/api/v1/accounts/"+flagAccount+"/courses",
				map[string]string{"enrollment_term_id": flagTerm}, maxScanPages)
			if cerr != nil {
				return classifyAPIError(cerr, flags)
			}
			if maxCourses > 0 && len(courses) > maxCourses {
				courses = courses[:maxCourses]
			}

			groups := map[string]*standingsAcc{}
			overall := &standingsAcc{}
			var failures []fetchFailure
			totalPages := cpages
			for _, course := range courses {
				courseID := course.str("id")
				if courseID == "" {
					continue
				}
				var sectionName map[string]string
				if flagBy == "section" {
					sectionName = map[string]string{}
					if secs, _, e := canvasFetchList(ctx, c, "/api/v1/courses/"+courseID+"/sections", nil, 2); e == nil {
						for _, s := range secs {
							sectionName[s.str("id")] = s.str("name")
						}
					}
				}
				enr, p, e := canvasFetchList(ctx, c,
					"/api/v1/courses/"+courseID+"/enrollments",
					map[string]string{"type[]": "StudentEnrollment"}, maxScanPages)
				totalPages += p
				if e != nil {
					failures = append(failures, fetchFailure{Scope: "course:" + courseID, Error: e.Error()})
					continue
				}
				for _, en := range enr {
					grades := en.obj("grades")
					score, scored := grades.num("current_score")
					var key, name string
					if flagBy == "section" {
						key = en.str("course_section_id")
						name = sectionName[key]
					} else {
						key = courseID
						name = course.str("name")
					}
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

			view := standingsView{
				Term:           flagTerm,
				Account:        flagAccount,
				By:             flagBy,
				CoursesScanned: len(courses),
				ScannedPages:   totalPages,
				Definitions:    "pass = score >= 70 (C+); dfw = score < 70 (D/F); rates over graded students only",
				Overall:        overall.group("overall"),
				Groups:         out,
				FetchFailures:  failures,
			}
			if overall.group("overall").Students == 0 {
				view.Note = fmt.Sprintf("no student enrollments found for term %s under account %s; confirm the term id, account, and that CANVAS_API_TOKEN has admin scope", flagTerm, flagAccount)
			}
			return emitNovel(cmd, flags, view, nil)
		},
	}
	cmd.Flags().StringVar(&flagTerm, "term", "", "Enrollment term ID (required)")
	cmd.Flags().StringVar(&flagAccount, "account", "1", "Account ID to scope courses (default root account 1)")
	cmd.Flags().StringVar(&flagBy, "by", "course", "Group rollups by: course or section")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum pages to scan per resource (100 per page)")
	cmd.Flags().IntVar(&maxCourses, "max-courses", 50, "Maximum courses to scan in the term")
	return cmd
}

// letterBucket maps a 0-100 score to a letter band.
func letterBucket(score float64) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}
