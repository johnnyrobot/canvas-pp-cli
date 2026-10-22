// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the guess at which changes Canvas Builder holds for
// the instructor. One row per rule in PredictApproval's list.

package sandbox

import (
	"net/http"
	"testing"
)

func TestPredictApproval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		want   bool
	}{
		{"delete a page", http.MethodDelete, "/api/v1/courses/7/pages/week-1", nil, true},
		{"delete a file", http.MethodDelete, "/api/v1/files/31", nil, true},
		{"reset the course", http.MethodPost, "/api/v1/courses/7/reset_content", nil, true},
		{"start an import", http.MethodPost, "/api/v1/courses/7/content_migrations", map[string]any{"migration_type": "common_cartridge_importer"}, true},
		{"update an import", http.MethodPut, "/api/v1/courses/7/content_migrations/12", nil, true},
		{"copy a course", http.MethodPost, "/api/v1/courses/7/course_copy", nil, true},
		{"delete the course, bracket key", http.MethodPut, "/api/v1/courses/7", map[string]any{"course[event]": "delete"}, true},
		{"conclude the course, plain key", http.MethodPut, "/api/v1/courses/7", map[string]any{"event": "conclude"}, true},
		{"conclude the course, nested body", http.MethodPut, "/api/v1/courses/7", map[string]any{"course": map[string]any{"event": "Conclude "}}, true},
		{"empty the syllabus", http.MethodPut, "/api/v1/courses/7", map[string]any{"course[syllabus_body]": ""}, true},
		{"delete modules in bulk", http.MethodPut, "/api/v1/courses/7/modules", map[string]any{"event": "delete"}, true},
		{"empty a page", http.MethodPut, "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[body]": "  "}, true},
		{"empty the front page", http.MethodPut, "/api/v1/courses/7/front_page", map[string]any{"wiki_page": map[string]any{"body": nil}}, true},

		{"write a page", http.MethodPut, "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[body]": "<p>x</p>"}, false},
		{"create a page", http.MethodPost, "/api/v1/courses/7/pages", map[string]any{"wiki_page[body]": "<p>x</p>"}, false},
		{"rename the course", http.MethodPut, "/api/v1/courses/7", map[string]any{"course[name]": "Art 101"}, false},
		{"publish a module", http.MethodPut, "/api/v1/courses/7/modules/3", map[string]any{"module[published]": "true"}, false},
		{"read the pages", http.MethodGet, "/api/v1/courses/7/pages", nil, false},
		{"read a course", http.MethodGet, "/api/v1/courses/7", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := PredictApproval(c.method, c.path, c.body); got != c.want {
				t.Fatalf("PredictApproval(%q, %q, %#v) = %v, want %v", c.method, c.path, c.body, got, c.want)
			}
		})
	}
}

// A struct body reaches the gateway as JSON, so the guess has to read it the
// same way the gateway will.
func TestPredictApprovalReadsAStructBody(t *testing.T) {
	t.Parallel()
	body := struct {
		Course struct {
			SyllabusBody string `json:"syllabus_body"`
		} `json:"course"`
	}{}
	if !PredictApproval(http.MethodPut, "/api/v1/courses/7", body) {
		t.Fatal("a blank syllabus_body in a struct body should be read as a change held for approval")
	}
}
