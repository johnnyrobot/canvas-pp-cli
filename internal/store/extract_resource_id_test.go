// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for primary-key extraction.

package store

import "testing"

// TestExtractResourceID is the corpus that proved the two copies of this rule
// agreed before the duplicate in internal/cli was deleted.
//
// The rule decides which field becomes a mirrored record's primary key, and
// both copies carried a comment warning that "divergence here produces silent
// drops on heterogeneous payloads". Keeping the corpus here pins the surviving
// implementation.
func TestExtractResourceID(t *testing.T) {
	cases := []struct {
		name     string
		resource string
		obj      map[string]any
		want     string
	}{
		{"empty object", "courses", map[string]any{}, ""},
		{"numeric id", "courses", map[string]any{"id": 42}, "42"},
		{"string id", "courses", map[string]any{"id": "42"}, "42"},
		{"PascalCase ID", "courses", map[string]any{"ID": "cap"}, "cap"},
		{"gid fallback", "courses", map[string]any{"gid": "g1"}, "g1"},
		{"name fallback", "courses", map[string]any{"name": "n"}, "n"},
		{"null id falls through", "courses", map[string]any{"id": nil, "name": "fallback"}, "fallback"},
		{"empty id yields nothing", "courses", map[string]any{"id": ""}, ""},
		{"bool id stringifies", "courses", map[string]any{"id": true}, "true"},

		// Suffix fallback: a resource keyed on its own <name>_code/_id/_slug.
		{"own code field", "courses", map[string]any{"course_code": "CS101"}, "CS101"},
		{"currency code (#2327)", "currencies", map[string]any{"currency_code": "USD"}, "USD"},
		{"own id field", "assignments", map[string]any{"assignment_id": "a1"}, "a1"},
		{"own slug field", "policies", map[string]any{"policy_slug": "ps"}, "ps"},
		{"verb-prefixed resource", "get_courses", map[string]any{"course_code": "GC"}, "GC"},

		// A foreign key must never be promoted to the primary key: the suffix
		// fallback is scoped to the resource's OWN name. The object carries
		// ONLY the foreign key, so the generic fallback cannot answer first
		// and the scoping rule is what is actually under test.
		{"foreign key alone yields nothing", "courses", map[string]any{"account_id": "nope"}, ""},
		{"parent id alone yields nothing", "assignments", map[string]any{"course_id": "7"}, ""},
		{"foreign key loses to a real field", "courses", map[string]any{"account_id": "nope", "name": "nm"}, "nm"},

		// Fallback precedence: id beats every softer key, and the softer keys
		// rank in declared order. Without a case holding several candidates at
		// once, reordering the list changes nothing observable.
		{"id beats name", "courses", map[string]any{"id": "1", "name": "n"}, "1"},
		{"id beats every softer key", "courses",
			map[string]any{"code": "c", "slug": "sl", "name": "n", "gid": "g", "id": "1"}, "1"},
		{"gid beats name", "courses", map[string]any{"name": "n", "gid": "g"}, "g"},
		{"name beats slug", "courses", map[string]any{"slug": "sl", "name": "n"}, "n"},
		{"slug beats code", "courses", map[string]any{"code": "c", "slug": "sl"}, "sl"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractResourceID(tc.resource, tc.obj); got != tc.want {
				t.Errorf("ExtractResourceID(%q, %v) = %q, want %q", tc.resource, tc.obj, got, tc.want)
			}
		})
	}
}

// TestExtractResourceID_QuizzesSuffixGap records observed behaviour rather
// than desired behaviour: depluralising "quizzes" does not reach "quiz", so a
// record keyed only on "quiz_key" resolves to no id.
//
// This is pre-existing and currently harmless — Canvas quiz payloads carry a
// real "id" field, which the generic fallback finds first. It is pinned so the
// gap is visible if a resource ever depends on it.
func TestExtractResourceID_QuizzesSuffixGap(t *testing.T) {
	got := ExtractResourceID("quizzes", map[string]any{"quiz_key": "qk"})
	if got != "" {
		t.Logf("depluralisation now reaches quiz -> %q; update this test and drop the note", got)
	}
	// A real id still wins, which is why the gap does not bite in practice.
	if id := ExtractResourceID("quizzes", map[string]any{"id": "7", "quiz_key": "qk"}); id != "7" {
		t.Errorf("ExtractResourceID with a real id = %q, want %q", id, "7")
	}
}

// TestExtractResourceID_LargeIntegerPrecision documents the float64 ceiling.
// JSON numbers decode through float64, so an id past 2^53 cannot round-trip.
// Canvas ids are well inside that range; string ids are unaffected.
func TestExtractResourceID_LargeIntegerPrecision(t *testing.T) {
	if got := ExtractResourceID("courses", map[string]any{"id": 9007199254740993.0}); got != "9007199254740992" {
		t.Errorf("large float id = %q, want the float64-rounded %q", got, "9007199254740992")
	}
	if got := ExtractResourceID("courses", map[string]any{"id": "9007199254740993"}); got != "9007199254740993" {
		t.Errorf("large string id = %q, want it preserved exactly", got)
	}
}
