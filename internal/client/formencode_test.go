// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Canvas is a Rails app. Params named with Rails bracket convention
// (`wiki_page[body]`) are only expanded into a nested params hash when they
// arrive as a query string or a form-encoded body. Rails does NOT
// bracket-expand keys inside an application/json body — `{"wiki_page[body]":
// "x"}` lands in params under the literal key `wiki_page[body]`, so
// `params[:wiki_page]` is nil and the update silently no-ops with HTTP 200.
//
// The generated command tree emits bracketed keys at 1,042 call sites, so the
// encoding decision has to happen at the single client chokepoint.
func TestDo_BracketedBodyIsFormEncoded(t *testing.T) {
	t.Parallel()

	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, _, err := c.Put(context.Background(), "/api/v1/courses/87/pages/x", map[string]any{
		"wiki_page[body]":      "<p>hi</p>",
		"wiki_page[published]": true,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	vals, err := url.ParseQuery(string(gotBody))
	if err != nil {
		t.Fatalf("body is not form-encoded (%v): %s", err, gotBody)
	}
	if got := vals.Get("wiki_page[body]"); got != "<p>hi</p>" {
		t.Errorf("wiki_page[body] = %q, want %q", got, "<p>hi</p>")
	}
	if got := vals.Get("wiki_page[published]"); got != "true" {
		t.Errorf("wiki_page[published] = %q, want %q", got, "true")
	}
}

// A body with no bracketed keys keeps the existing JSON encoding. The --stdin
// path lets callers hand over a real nested JSON document, which Canvas
// accepts; flipping that to form encoding would break it.
func TestDo_PlainBodyStaysJSON(t *testing.T) {
	t.Parallel()

	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, _, err := c.Put(context.Background(), "/api/v1/courses/87/pages/x", map[string]any{
		"wiki_page": map[string]any{"body": "<p>hi</p>"},
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if string(gotBody) != `{"wiki_page":{"body":"\u003cp\u003ehi\u003c/p\u003e"}}` {
		t.Errorf("body = %s", gotBody)
	}
}

// The generated tree emits several bracket shapes beyond the simple
// `parent[child]` case: repeated lists (`course_ids[]`), arrays of objects
// (`assignment_overrides[][course_section_id]`) and multi-level nesting
// (`account[settings][conditional_release][value]`). Rack reads all of them
// back out of a form body, so the encoder has to pass them through unchanged
// rather than inventing its own escaping.
func TestEncodeRailsForm_Shapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{
			name: "scalar nesting is passed through verbatim",
			in:   map[string]any{"account[settings][conditional_release][value]": "true"},
			want: "account%5Bsettings%5D%5Bconditional_release%5D%5Bvalue%5D=true",
		},
		{
			name: "documented list keys keep their single bracket pair",
			in:   map[string]any{"course_ids[]": []string{"1", "2"}},
			want: "course_ids%5B%5D=1&course_ids%5B%5D=2",
		},
		{
			name: "a list under a scalar key gains the list suffix",
			in:   map[string]any{"grade_data[7][file_ids]": []any{"3", "4"}},
			want: "grade_data%5B7%5D%5Bfile_ids%5D%5B%5D=3&grade_data%5B7%5D%5Bfile_ids%5D%5B%5D=4",
		},
		{
			name: "booleans and numbers render without Go formatting artifacts",
			in:   map[string]any{"assignment[published]": true, "assignment[points_possible]": float64(3)},
			want: "assignment%5Bpoints_possible%5D=3&assignment%5Bpublished%5D=true",
		},
		{
			name: "nil values are omitted rather than blanking the field",
			in:   map[string]any{"assignment[name]": "keep", "assignment[due_at]": nil},
			want: "assignment%5Bname%5D=keep",
		},
		{
			name: "reserved characters in values are escaped, not dropped",
			in:   map[string]any{"wiki_page[body]": "<p>a&b=c</p>"},
			want: "wiki_page%5Bbody%5D=%3Cp%3Ea%26b%3Dc%3C%2Fp%3E",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := encodeRailsForm(tc.in); got != tc.want {
				t.Errorf("encodeRailsForm()\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// Round-tripping through the same parser Rack uses is the property that
// actually matters: whatever the encoder emits has to decode back to the
// parameter names the API documents.
func TestEncodeRailsForm_RoundTrips(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"assignment_overrides[][course_section_id]": []any{"11", "12"},
		"wiki_page[body]": "<p>hi</p>",
	}
	vals, err := url.ParseQuery(encodeRailsForm(in))
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if got := vals.Get("wiki_page[body]"); got != "<p>hi</p>" {
		t.Errorf("wiki_page[body] = %q", got)
	}
	got := vals["assignment_overrides[][course_section_id]"]
	if len(got) != 2 || got[0] != "11" || got[1] != "12" {
		t.Errorf("assignment_overrides[][course_section_id] = %v", got)
	}
}

// A body with no keys at all must not flip to form encoding: an empty map
// still has to marshal as `{}` the way it always did.
func TestRailsBracketBody_EmptyMapStaysJSON(t *testing.T) {
	t.Parallel()

	if _, ok := railsBracketBody(map[string]any{}); ok {
		t.Error("empty map should not be treated as a bracketed body")
	}
	if _, ok := railsBracketBody(map[string]any{"name": "x"}); ok {
		t.Error("flat non-bracketed map should not be treated as a bracketed body")
	}
	if _, ok := railsBracketBody([]any{"a"}); ok {
		t.Error("non-map body should not be treated as a bracketed body")
	}
}
