// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored test for the at-risk analysis rules.

package cli

import (
	"sort"
	"testing"
)

// sortAtRisk applies the production ordering to a copy of students.
func sortAtRisk(students []atRiskStudent) []atRiskStudent {
	out := append([]atRiskStudent(nil), students...)
	sort.Slice(out, func(i, j int) bool { return lessAtRisk(out[i], out[j]) })
	return out
}

func ids(students []atRiskStudent) []string {
	out := make([]string, 0, len(students))
	for _, s := range students {
		out = append(out, s.UserID)
	}
	return out
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestLessAtRisk_ConcernCountDominates guards the primary sort key: a student
// with more concerns always ranks first, regardless of user ID.
func TestLessAtRisk_ConcernCountDominates(t *testing.T) {
	got := ids(sortAtRisk([]atRiskStudent{
		{UserID: "1", Total: 2},
		{UserID: "2", Total: 9},
		{UserID: "3", Total: 5},
	}))
	want := []string{"2", "3", "1"}
	if !equalIDs(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestLessAtRisk_TieBreakIsNumeric pins the tie-break for students with equal
// concern counts. Canvas user IDs are numeric strings, so the order must be
// numeric — lexicographic comparison puts "10" ahead of "9", which reads as a
// ranking error to anyone using the output.
func TestLessAtRisk_TieBreakIsNumeric(t *testing.T) {
	got := ids(sortAtRisk([]atRiskStudent{
		{UserID: "10", Total: 3},
		{UserID: "9", Total: 3},
		{UserID: "100", Total: 3},
	}))
	want := []string{"9", "10", "100"}
	if !equalIDs(got, want) {
		t.Errorf("tie-break order = %v, want %v", got, want)
	}
}

// TestLessAtRisk_TieBreakFallsBackToString covers non-numeric IDs (SIS-style
// identifiers): ordering must stay deterministic rather than collapsing.
func TestLessAtRisk_TieBreakFallsBackToString(t *testing.T) {
	got := ids(sortAtRisk([]atRiskStudent{
		{UserID: "sis:beta", Total: 4},
		{UserID: "sis:alpha", Total: 4},
	}))
	want := []string{"sis:alpha", "sis:beta"}
	if !equalIDs(got, want) {
		t.Errorf("fallback order = %v, want %v", got, want)
	}
}

// TestLessUserID_TotalOrdering pins the comparator directly, including the
// mixed numeric/non-numeric case and IDs that are numerically equal but
// textually different. Ordering must be total, or sort.Slice leaves equal
// elements in arbitrary order and successive runs stop being diffable.
func TestLessUserID_TotalOrdering(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"9", "10", true},         // the original bug: numeric, not lexicographic
		{"10", "9", false},        //
		{"2", "100", true},        //
		{"sis:a", "sis:b", true},  // both non-numeric: string order
		{"sis:b", "sis:a", false}, //
		{"9", "sis:a", true},      // mixed: string order, deterministic
		{"sis:a", "9", false},     //
		{"007", "7", true},        // numerically equal, textually distinct
		{"7", "007", false},       //
		{"42", "42", false},       // identical: neither precedes the other
	}
	for _, tc := range cases {
		if got := lessUserID(tc.a, tc.b); got != tc.want {
			t.Errorf("lessUserID(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
