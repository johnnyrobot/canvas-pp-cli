// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: a guess at which changes Canvas Builder
// will hold for the instructor, so the agent can be told it is waiting on a
// person rather than on a slow server.

package sandbox

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

// These mirror the gateway's own rules in
// gateway/internal/policy/policy.go (resetRe, migrationRe, migrationItemRe,
// courseCopyRe, courseRootRe, modulesRe, pageRe, frontPageRe). The format
// suffix is there because Rails reads /pages/week-1.json as the same route.
var (
	resetRe         = regexp.MustCompile(`^/api/v1/courses/\d+/reset_content(\.[^/.?]+)?$`)
	migrationRe     = regexp.MustCompile(`^/api/v1/courses/\d+/content_migrations(\.[^/.?]+)?$`)
	migrationItemRe = regexp.MustCompile(`^/api/v1/courses/\d+/content_migrations/\d+(\.[^/.?]+)?$`)
	courseCopyRe    = regexp.MustCompile(`^/api/v1/courses/\d+/course_copy(\.[^/.?]+)?$`)
	courseRootRe    = regexp.MustCompile(`^/api/v1/courses/(\d+)$`)
	modulesRe       = regexp.MustCompile(`^/api/v1/courses/\d+/modules(\.[^/.?]+)?$`)
	pageRe          = regexp.MustCompile(`^/api/v1/courses/\d+/pages/([^/]+)$`)
	frontPageRe     = regexp.MustCompile(`^/api/v1/courses/\d+/front_page(\.[^/.?]+)?$`)
)

// PredictApproval reports whether Canvas Builder is likely to ask the
// instructor about this request. It is only ever used to decide what to say
// while waiting: the gateway alone decides what is actually held, and its rule
// 6 (policy.needsApproval) covers more than this -- an upload preflight that
// would overwrite a file, a file copy or rename over an existing one, a course
// update that publishes the course, and a change to who an item is assigned
// to. Those cases fall through to the still-waiting notice, which says the same
// thing in weaker words, so a guess that comes up short is never wrong out
// loud.
func PredictApproval(method, path string, body any) bool {
	switch method {
	case http.MethodDelete:
		return true
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return false
	}
	switch {
	case resetRe.MatchString(path), migrationRe.MatchString(path), migrationItemRe.MatchString(path), courseCopyRe.MatchString(path):
		return true
	}
	fields := bodyFields(body)
	switch {
	case courseRootRe.MatchString(path):
		return anyValue(fields, "delete", "event") || anyValue(fields, "delete", "course", "event") ||
			anyValue(fields, "conclude", "event") || anyValue(fields, "conclude", "course", "event") ||
			anyBlank(fields, "syllabus_body") || anyBlank(fields, "course", "syllabus_body")
	case modulesRe.MatchString(path):
		return anyValue(fields, "delete", "event") || anyValue(fields, "delete", "module", "event")
	case pageRe.MatchString(path), frontPageRe.MatchString(path):
		return anyBlank(fields, "body") || anyBlank(fields, "wiki_page", "body")
	}
	return false
}

// bodyFields reads a request body the way the gateway will. A body written in
// Rails bracket convention ("course[event]") goes on the wire form-encoded and
// one written as nested maps or a struct goes as JSON, and the gateway reads
// both shapes back to the same parameter, so both are looked up here.
func bodyFields(body any) map[string]any {
	if body == nil {
		return nil
	}
	if m, ok := body.(map[string]any); ok {
		return m
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// values returns every value the body carries at keys, under the bracket
// spelling and the nested one. A list is flattened, and an empty list counts as
// one blank value, which is how Rack reads "key[]=".
func values(fields map[string]any, keys ...string) []any {
	if len(fields) == 0 || len(keys) == 0 {
		return nil
	}
	var out []any
	if v, ok := fields[bracketKey(keys)]; ok {
		out = append(out, flatten(v)...)
	}
	cur := []any{any(fields)}
	for _, k := range keys {
		var next []any
		for _, n := range cur {
			m, ok := n.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := m[k]; ok {
				next = append(next, flatten(v)...)
			}
		}
		cur = next
	}
	return append(out, cur...)
}

func bracketKey(keys []string) string {
	if len(keys) == 1 {
		return keys[0]
	}
	return keys[0] + "[" + strings.Join(keys[1:], "][") + "]"
}

func flatten(v any) []any {
	l, ok := v.([]any)
	if !ok {
		return []any{v}
	}
	if len(l) == 0 {
		return []any{nil}
	}
	var out []any
	for _, e := range l {
		out = append(out, flatten(e)...)
	}
	return out
}

// anyValue reports whether any value at keys is want, ignoring case and
// surrounding space, the way the gateway's own anyValue does.
func anyValue(fields map[string]any, want string, keys ...string) bool {
	for _, v := range values(fields, keys...) {
		if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), want) {
			return true
		}
	}
	return false
}

// anyBlank reports whether keys is present with a blank value: null, empty or
// only whitespace, zero-width characters included.
func anyBlank(fields map[string]any, keys ...string) bool {
	for _, v := range values(fields, keys...) {
		if isBlank(v) {
			return true
		}
	}
	return false
}

func isBlank(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.TrimFunc(t, isBlankRune) == ""
	case json.Number, float64, float32, int, int64, int32, bool:
		return false
	}
	return true
}

func isBlankRune(r rune) bool {
	switch r {
	case 0, 0x200b, 0x200c, 0x200d, 0x2060, 0xfeff: // NUL and the zero-width characters
		return true
	}
	return unicode.IsSpace(r)
}
