// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"strings"
	"testing"
)

// pageResponse is the shape Canvas returns from GET/PUT on a wiki page, cut
// down to the fields the check reads.
const pageResponse = `{
  "url": "aiinfra-101",
  "title": "AIINFRA 101",
  "page_id": 11901,
  "published": true,
  "front_page": true,
  "editing_roles": "teachers",
  "body": "<p>hello</p>",
  "updated_at": "2026-08-24T19:43:09Z"
}`

func TestVerifyWriteAcceptsAppliedWrites(t *testing.T) {
	cases := []struct {
		name string
		body any
	}{
		{
			// The flag path: what the generated command tree builds.
			name: "rails bracket body",
			body: map[string]any{"wiki_page[body]": "<p>new</p>"},
		},
		{
			// The documented --stdin shape.
			name: "nested json body",
			body: map[string]any{"wiki_page": map[string]any{"body": "<p>new</p>"}},
		},
		{
			name: "bracket body with several fields",
			body: map[string]any{
				"wiki_page[body]":  "<p>new</p>",
				"wiki_page[title]": "AIINFRA 101",
			},
		},
		{
			// Only one field needs to be echoed for the write to look real.
			name: "one echoed field among unechoed ones",
			body: map[string]any{
				"wiki_page[title]":            "AIINFRA 101",
				"wiki_page[notify_of_update]": true,
			},
		},
		{
			// A top-level parameter that is not nested under a resource.
			name: "flat body whose field is echoed",
			body: map[string]any{"title": "AIINFRA 101"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if f := verifyWrite(tc.body, []byte(pageResponse)); f != nil {
				t.Fatalf("expected no finding, got %+v", f)
			}
		})
	}
}

func TestVerifyWriteFlagsFlattenedBracketKey(t *testing.T) {
	// The exact body that returned HTTP 200 and changed nothing.
	body := map[string]any{"wiki_page_body": "<p>new</p>", "wiki_page_published": true}

	f := verifyWrite(body, []byte(pageResponse))
	if f == nil {
		t.Fatal("expected a finding for a flattened bracket body")
	}
	if !f.hard {
		t.Error("a flat, non-bracketed, non-nested body should be a hard finding")
	}
	if f.suggestion != "wiki_page[body]" {
		t.Errorf("suggestion = %q, want %q", f.suggestion, "wiki_page[body]")
	}
	if f.sentKey != "wiki_page_body" {
		t.Errorf("sentKey = %q, want %q", f.sentKey, "wiki_page_body")
	}

	msg := f.message("PUT", "/api/v1/courses/88/pages/aiinfra-101")
	for _, want := range []string{"wiki_page[body]", "--wiki-page-body", "flattened Rails bracket name"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestVerifyWriteSoftFindingForBracketBody(t *testing.T) {
	// A bracket body whose fields are genuinely not echoed. Real endpoints do
	// this (course[event], wiki_page[notify_of_update]), so it must not be
	// hard enough to fail the command.
	body := map[string]any{"course[event]": "offer"}
	f := verifyWrite(body, []byte(pageResponse))
	if f == nil {
		t.Fatal("expected a finding")
	}
	if f.hard {
		t.Error("a bracketed body must never produce a hard finding")
	}
}

func TestVerifyWriteSkipsWhenItCannotJudge(t *testing.T) {
	cases := []struct {
		name string
		body any
		resp string
	}{
		{"nil body", nil, pageResponse},
		{"empty body", map[string]any{}, pageResponse},
		{"non-map body", []string{"a"}, pageResponse},
		{"array response", map[string]any{"wiki_page_body": "x"}, `[{"id":1}]`},
		{"empty response", map[string]any{"wiki_page_body": "x"}, `{}`},
		{"non-json response", map[string]any{"wiki_page_body": "x"}, `not json`},
		{"scalar response", map[string]any{"wiki_page_body": "x"}, `"ok"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if f := verifyWrite(tc.body, []byte(tc.resp)); f != nil {
				t.Fatalf("expected no finding, got %+v", f)
			}
		})
	}
}

func TestLeafFieldName(t *testing.T) {
	cases := map[string]string{
		"wiki_page[body]":  "body",
		"assignment[name]": "name",
		"course_ids[]":     "course_ids",
		"assignment_overrides[][course_section_id]": "course_section_id",
		"title": "title",
		"calendar_event[child_event_data][x][start]": "start",
	}
	for in, want := range cases {
		if got := leafFieldName(in); got != want {
			t.Errorf("leafFieldName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFlattenedBracketSuggestion(t *testing.T) {
	resp := map[string]any{"body": "x", "title": "y", "page_body": "z"}

	// Longest prefix wins: wiki_page[body], not wiki[page_body].
	if got, ok := flattenedBracketSuggestion("wiki_page_body", resp); !ok || got != "wiki_page[body]" {
		t.Errorf("wiki_page_body -> %q (%v), want wiki_page[body]", got, ok)
	}
	if got, ok := flattenedBracketSuggestion("assignment_title", resp); !ok || got != "assignment[title]" {
		t.Errorf("assignment_title -> %q (%v), want assignment[title]", got, ok)
	}
	// Already bracketed, or nothing to split, or no matching response field.
	for _, k := range []string{"wiki_page[body]", "title", "some_unknown_field"} {
		if got, ok := flattenedBracketSuggestion(k, resp); ok {
			t.Errorf("flattenedBracketSuggestion(%q) = %q, want no suggestion", k, got)
		}
	}
}

func TestIsFlatGuessBody(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want bool
	}{
		{"flat scalars", map[string]any{"a": "x", "b": true}, true},
		{"bracket key", map[string]any{"wiki_page[body]": "x"}, false},
		{"nested object", map[string]any{"wiki_page": map[string]any{"body": "x"}}, false},
		{"array value", map[string]any{"ids": []any{1.0, 2.0}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFlatGuessBody(tc.body); got != tc.want {
				t.Errorf("isFlatGuessBody() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseWriteVerifyMode(t *testing.T) {
	cases := map[string]WriteVerifyMode{
		"":       WriteVerifyOn,
		"on":     WriteVerifyOn,
		"error":  WriteVerifyOn,
		"strict": WriteVerifyOn,
		"warn":   WriteVerifyWarn,
		" WARN ": WriteVerifyWarn,
		"off":    WriteVerifyOff,
		"false":  WriteVerifyOff,
	}
	for in, want := range cases {
		got, err := ParseWriteVerifyMode(in)
		if err != nil {
			t.Errorf("ParseWriteVerifyMode(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseWriteVerifyMode(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseWriteVerifyMode("maybe"); err == nil {
		t.Error("expected an error for an unknown mode")
	}
}

func TestFlagNameFor(t *testing.T) {
	cases := map[string]string{
		"wiki_page[body]":  "wiki-page-body",
		"assignment[name]": "assignment-name",
	}
	for in, want := range cases {
		if got := flagNameFor(in); got != want {
			t.Errorf("flagNameFor(%q) = %q, want %q", in, got, want)
		}
	}
}
