// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the standings rollup rules.

package cli

import (
	"fmt"
	"strings"
	"testing"
)

// scored builds a student enrollment carrying a current_score.
func scored(sectionID string, score float64) string {
	return fmt.Sprintf(`{"course_section_id": %q, "grades": {"current_score": %g}}`, sectionID, score)
}

// ungraded builds a student enrollment with no score yet.
func ungraded(sectionID string) string {
	return fmt.Sprintf(`{"course_section_id": %q, "grades": {}}`, sectionID)
}

func groupByKey(view standingsView, key string) *standingsGroup {
	for i := range view.Groups {
		if view.Groups[i].Key == key {
			return &view.Groups[i]
		}
	}
	return nil
}

// TestAnalyzeStandings_DistributionByCourse covers the default grouping and the
// letter-band tally.
func TestAnalyzeStandings_DistributionByCourse(t *testing.T) {
	view, rows := analyzeStandings(standingsInput{
		Term: "7", Account: "1", By: "course",
		Courses: objs(t, `{"id": "10", "name": "Intro Physics"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			"10": objs(t, scored("1", 95), scored("1", 85), scored("1", 75), scored("1", 65), scored("1", 30), ungraded("1")),
		},
	})

	if rows != nil {
		t.Errorf("standings renders as JSON, rows should be nil, got %v", rows)
	}
	g := groupByKey(view, "10")
	if g == nil {
		t.Fatalf("no group for course 10: %+v", view.Groups)
	}
	if g.Name != "Intro Physics" {
		t.Errorf("group name = %q, want the course name", g.Name)
	}
	if g.Dist.A != 1 || g.Dist.B != 1 || g.Dist.C != 1 || g.Dist.D != 1 || g.Dist.F != 1 {
		t.Errorf("distribution = %+v, want one in each band", g.Dist)
	}
	if g.Dist.Ungraded != 1 {
		t.Errorf("Ungraded = %d, want 1", g.Dist.Ungraded)
	}
	if g.Students != 6 {
		t.Errorf("Students = %d, want 6 (graded + ungraded)", g.Students)
	}
	if g.Graded != 5 {
		t.Errorf("Graded = %d, want 5", g.Graded)
	}
}

// TestAnalyzeStandings_PassAndDFWRates pins the headline arithmetic: pass is
// C or better, DFW is D/F, and both are over graded students only.
func TestAnalyzeStandings_PassAndDFWRates(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:      "course",
		Courses: objs(t, `{"id": "10"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			// 3 passing (A, B, C), 1 failing, 1 ungraded.
			"10": objs(t, scored("1", 95), scored("1", 85), scored("1", 75), scored("1", 30), ungraded("1")),
		},
	})

	g := groupByKey(view, "10")
	if g == nil {
		t.Fatal("no group for course 10")
	}
	if g.PassRate == nil || *g.PassRate != 0.75 {
		t.Errorf("PassRate = %v, want 0.75 (3 of 4 graded)", g.PassRate)
	}
	if g.DFWRate == nil || *g.DFWRate != 0.25 {
		t.Errorf("DFWRate = %v, want 0.25 (1 of 4 graded)", g.DFWRate)
	}
	// The ungraded student must not drag the average down.
	if g.AvgScore == nil || *g.AvgScore != 71.25 {
		t.Errorf("AvgScore = %v, want 71.25 over graded students only", g.AvgScore)
	}
}

// TestAnalyzeStandings_UngradedExcludedFromRates guards the one case where a
// naive implementation divides by the wrong denominator.
func TestAnalyzeStandings_UngradedExcludedFromRates(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:      "course",
		Courses: objs(t, `{"id": "10"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			"10": objs(t, scored("1", 95), ungraded("1"), ungraded("1"), ungraded("1")),
		},
	})

	g := groupByKey(view, "10")
	if g == nil {
		t.Fatal("no group for course 10")
	}
	if g.PassRate == nil || *g.PassRate != 1 {
		t.Errorf("PassRate = %v, want 1.0 — three ungraded students must not count against it", g.PassRate)
	}
	if g.Students != 4 || g.Graded != 1 {
		t.Errorf("Students/Graded = %d/%d, want 4/1", g.Students, g.Graded)
	}
}

// TestAnalyzeStandings_GroupsBySection covers --by section, including the
// section-name lookup.
func TestAnalyzeStandings_GroupsBySection(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:      "section",
		Courses: objs(t, `{"id": "10", "name": "Intro Physics"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			"10": objs(t, scored("s1", 95), scored("s1", 90), scored("s2", 50)),
		},
		SectionNamesByCourse: map[string]map[string]string{
			"10": {"s1": "Morning", "s2": "Evening"},
		},
	})

	if len(view.Groups) != 2 {
		t.Fatalf("want one group per section, got %+v", view.Groups)
	}
	morning := groupByKey(view, "s1")
	if morning == nil || morning.Name != "Morning" {
		t.Errorf("s1 group = %+v, want it named Morning", morning)
	}
	if morning.Dist.A != 2 {
		t.Errorf("Morning A count = %d, want 2", morning.Dist.A)
	}
	if evening := groupByKey(view, "s2"); evening == nil || evening.Dist.F != 1 {
		t.Errorf("s2 group = %+v, want one F", evening)
	}
}

// TestAnalyzeStandings_UnknownSectionLeavesNameEmpty keeps a missing section
// name from breaking the rollup.
func TestAnalyzeStandings_UnknownSectionLeavesNameEmpty(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:                  "section",
		Courses:             objs(t, `{"id": "10"}`),
		EnrollmentsByCourse: map[string][]canvasObj{"10": objs(t, scored("s9", 95))},
	})
	g := groupByKey(view, "s9")
	if g == nil {
		t.Fatalf("group should still exist, got %+v", view.Groups)
	}
	if g.Name != "" {
		t.Errorf("Name = %q, want empty when the section is unknown", g.Name)
	}
}

// TestAnalyzeStandings_OverallSpansCourses checks the overall accumulator adds
// up across every course, not just the last one.
func TestAnalyzeStandings_OverallSpansCourses(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:      "course",
		Courses: objs(t, `{"id": "10"}`, `{"id": "20"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			"10": objs(t, scored("1", 95), scored("1", 95)),
			"20": objs(t, scored("2", 30)),
		},
	})

	if view.Overall.Key != "overall" {
		t.Errorf("Overall.Key = %q", view.Overall.Key)
	}
	if view.Overall.Students != 3 || view.Overall.Graded != 3 {
		t.Errorf("Overall = %d students / %d graded, want 3/3", view.Overall.Students, view.Overall.Graded)
	}
	if view.Overall.Dist.A != 2 || view.Overall.Dist.F != 1 {
		t.Errorf("Overall distribution = %+v, want 2 A and 1 F", view.Overall.Dist)
	}
	if view.Overall.PassRate == nil || *view.Overall.PassRate < 0.66 || *view.Overall.PassRate > 0.67 {
		t.Errorf("Overall PassRate = %v, want ~0.667", view.Overall.PassRate)
	}
}

// TestAnalyzeStandings_GroupsSortedByKey keeps output stable between runs; the
// groups come out of a map, whose iteration order is deliberately random.
func TestAnalyzeStandings_GroupsSortedByKey(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:      "course",
		Courses: objs(t, `{"id": "30"}`, `{"id": "10"}`, `{"id": "20"}`),
		EnrollmentsByCourse: map[string][]canvasObj{
			"30": objs(t, scored("1", 95)),
			"10": objs(t, scored("1", 95)),
			"20": objs(t, scored("1", 95)),
		},
	})

	want := []string{"10", "20", "30"}
	for i, w := range want {
		if view.Groups[i].Key != w {
			t.Fatalf("group order = %v, want sorted %v",
				[]string{view.Groups[0].Key, view.Groups[1].Key, view.Groups[2].Key}, want)
		}
	}
}

// TestAnalyzeStandings_SkipsCoursesWithoutID guards the id guard.
func TestAnalyzeStandings_SkipsCoursesWithoutID(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{
		By:                  "course",
		Courses:             objs(t, `{"name": "No ID"}`, `{"id": "10"}`),
		EnrollmentsByCourse: map[string][]canvasObj{"10": objs(t, scored("1", 95))},
	})
	if len(view.Groups) != 1 || view.Groups[0].Key != "10" {
		t.Errorf("groups = %+v, want only course 10", view.Groups)
	}
	if view.CoursesScanned != 2 {
		t.Errorf("CoursesScanned = %d, want 2 (id-less courses still count as scanned)", view.CoursesScanned)
	}
}

// TestAnalyzeStandings_EmptyNotesAndStatesDefinitions pins the empty case and
// the definitions string, which is the output's only statement of what pass
// and DFW mean.
func TestAnalyzeStandings_EmptyNotesAndStatesDefinitions(t *testing.T) {
	view, _ := analyzeStandings(standingsInput{Term: "7", Account: "1", By: "course"})

	if !strings.Contains(view.Note, "term 7") || !strings.Contains(view.Note, "account 1") {
		t.Errorf("Note should name the term and account, got %q", view.Note)
	}
	if !strings.Contains(view.Definitions, "score >= 70") {
		t.Errorf("Definitions must state the pass threshold, got %q", view.Definitions)
	}
	if view.Overall.PassRate != nil {
		t.Errorf("PassRate should stay nil with no graded students, got %v", *view.Overall.PassRate)
	}
}

// TestStandingsDefinitions_MatchesLetterBucket is the guard against the review's
// drift finding: the threshold is stated in the help text, in the runtime
// definitions string, and implemented in letterBucket. If letterBucket moves,
// this fails.
func TestStandingsDefinitions_MatchesLetterBucket(t *testing.T) {
	if !strings.Contains(standingsDefinitions, "score >= 70") {
		t.Fatalf("definitions no longer state the 70 threshold: %q", standingsDefinitions)
	}
	if letterBucket(70) != "C" {
		t.Errorf("letterBucket(70) = %q, want C — the documented pass threshold", letterBucket(70))
	}
	if letterBucket(69.9) != "D" {
		t.Errorf("letterBucket(69.9) = %q, want D — just below the pass threshold", letterBucket(69.9))
	}
}
