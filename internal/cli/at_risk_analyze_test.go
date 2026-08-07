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
