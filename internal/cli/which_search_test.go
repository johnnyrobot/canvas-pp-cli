// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for `which` capability resolution.

package cli

import (
	"strings"
	"testing"
)

// topWhichMatch resolves a query the way the command does and returns the
// winning command, or "" for no match.
func topWhichMatch(t *testing.T, query string) string {
	t.Helper()
	matches := resolveWhich(RootCmd(), query, 3)
	if len(matches) == 0 {
		return ""
	}
	return matches[0].Entry.Command
}

// TestWhichResolves is the contract: these are the queries an agent or a
// human actually types, and the command each must reach.
//
// The curated-capability rows all failed before this was fixed — the ranker
// tokenised on whitespace only, so "at-risk" was a single token no query
// could match, and it never scored WhyItMatters. The endpoint rows failed
// because there was no command-tree fallback at all.
func TestWhichResolves(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		// Curated capabilities — the six commands this CLI exists for.
		{"at risk students", "at-risk"},
		{"who is falling behind", "at-risk"},
		{"early alert outreach", "at-risk"},
		{"grading queue", "to-grade"},
		{"what should I grade next", "to-grade"},
		{"unified roster", "roster"},
		{"every student in a course", "roster"},
		{"term grade distribution", "standings"},
		{"pass fail rates", "standings"},
		{"enrollment anomalies", "audit-enrollments"},
		{"courses with no teacher", "audit-enrollments"},

		// Endpoint commands — reachable only via the command-tree fallback.
		{"create an assignment", "assignments create"},
		{"delete a quiz", "quizzes delete-quizzes"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			if got := topWhichMatch(t, tc.query); got != tc.want {
				t.Errorf("which %q = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

// TestWhichNoMatchStaysEmpty keeps the "no confident match" contract: agents
// branch on an empty result, so a nonsense query must not be answered with
// whatever scored 1.
func TestWhichNoMatchStaysEmpty(t *testing.T) {
	if got := topWhichMatch(t, "zzzz qqqq wwww"); got != "" {
		t.Errorf("nonsense query matched %q, want no match", got)
	}
}

// TestWhichTreeReachesHiddenAreaChildren guards the walk. Every per-resource
// area group is Hidden to keep root --help readable; skipping hidden subtrees
// would hide the entire endpoint surface, which is the set this fallback
// exists to reach.
func TestWhichTreeReachesHiddenAreaChildren(t *testing.T) {
	entries := whichTreeEntries(RootCmd())
	if len(entries) < 500 {
		t.Fatalf("tree walk found %d commands; the endpoint surface is ~1,000 — hidden area groups are being skipped", len(entries))
	}

	var nested int
	found := map[string]bool{}
	for _, e := range entries {
		if strings.Contains(e.Command, " ") {
			nested++
		}
		found[e.Command] = true
	}
	if nested < 500 {
		t.Errorf("only %d nested commands found; expected the bulk of the tree to be <area> <command>", nested)
	}
	for _, want := range []string{"assignments create", "courses index"} {
		if !found[want] {
			t.Errorf("tree walk missed %q", want)
		}
	}
}

// TestWhichTreeExcludesHiddenAndPlumbing checks the walk does not offer the
// hidden area groups themselves, or cobra's built-ins, as answers.
func TestWhichTreeExcludesHiddenAndPlumbing(t *testing.T) {
	for _, e := range whichTreeEntries(RootCmd()) {
		switch e.Command {
		case "assignments", "courses", "quizzes", "help", "completion":
			t.Errorf("tree walk offered %q, which is a hidden group or cobra plumbing", e.Command)
		}
	}
}

// TestWhichTokens pins the tokenizer. strings.Fields split on whitespace only,
// so every hyphenated command was a single unmatchable token.
func TestWhichTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"at-risk", []string{"at", "risk"}},
		{"audit-enrollments", []string{"audit", "enrollments"}},
		{"assignments create", []string{"assignments", "create"}},
		{"Grade-distribution and pass/fail rollups", []string{"grade", "distribution", "and", "pass", "fail", "rollups"}},
	}
	for _, tc := range cases {
		got := whichTokens(tc.in)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("whichTokens(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestWhichScoreEntry_MatchesFieldsByWholeToken pins token equality across
// every field.
//
// The original ranker substring-matched the group tag, so the two-letter
// token "at" scored against "Local snapshots thAT compound" and handed
// "at risk students" to `since`. Stopword filtering happens to hide that
// exact query now, so this asserts the underlying rule directly: a token
// that is only a substring of a field's word must not score.
func TestWhichScoreEntry_MatchesFieldsByWholeToken(t *testing.T) {
	e := whichEntry{
		Command:      "standings",
		Description:  "Grade-distribution rollups.",
		Group:        "Admin rollups",
		WhyItMatters: "Term-level reporting.",
	}

	// "min" is a substring of "Admin" but not a token of it.
	if got := whichScoreEntry(e, "min", []string{"min"}); got != 0 {
		t.Errorf("score for a substring-only group hit = %d, want 0", got)
	}
	// "admin" is a whole token of the group.
	if got := whichScoreEntry(e, "admin", []string{"admin"}); got == 0 {
		t.Error("score for a whole-token group hit = 0, want > 0")
	}
	// "distribu" is a substring of "distribution" but not a token.
	if got := whichScoreEntry(e, "distribu", []string{"distribu"}); got != 0 {
		t.Errorf("score for a substring-only description hit = %d, want 0", got)
	}
}

// TestWhichScoreEntry_TokenScoresOnceByStrongestField guards the coverage
// model. Summing a token across every field it appears in let one incidental
// word outweigh an entry that matched the whole query — "term grade
// distribution" picked to-grade over standings that way.
func TestWhichScoreEntry_TokenScoresOnceByStrongestField(t *testing.T) {
	// "grade" appears in the command, the description and why-it-matters.
	e := whichEntry{
		Command:      "grade",
		Description:  "grade grade grade",
		WhyItMatters: "grade",
		Group:        "grade",
	}
	if got := whichScoreEntry(e, "grade", []string{"grade"}); got != 3 {
		t.Errorf("score = %d, want 3 — the token counts once, by the command (its strongest field), "+
			"not additively across description, why-it-matters and group", got)
	}
}

// TestWhichStem covers the plural forms this API's nouns actually take.
// People ask "delete a quiz"; the command is "quizzes".
func TestWhichStem(t *testing.T) {
	cases := map[string]string{
		"assignments": "assignment",
		"courses":     "course",
		"quizzes":     "quiz",
		"policies":    "policy",
		"students":    "student",
		"quiz":        "quiz",
		"class":       "class", // -ss must not be stripped
		"is":          "is",    // too short to strip
	}
	for in, want := range cases {
		if got := whichStem(in); got != want {
			t.Errorf("whichStem(%q) = %q, want %q", in, got, want)
		}
	}
}
