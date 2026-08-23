// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Canvas is a Rails application, and its API documents write parameters using
// Rails bracket convention: `wiki_page[body]`, `assignment[name]`,
// `course_ids[]`. Rails expands those names into a nested params hash only
// when they arrive as a query string or an
// application/x-www-form-urlencoded body — Rack's parse_nested_query does the
// expansion. A JSON body is parsed by JSON.parse and handed straight to
// params, with no bracket expansion at all, so
// `{"wiki_page[body]": "x"}` lands under the literal key `"wiki_page[body]"`
// and `params[:wiki_page]` stays nil.
//
// Canvas answers such a request with HTTP 200 and the unmodified resource:
// the write silently no-ops. Sending the same parameters form-encoded makes
// them visible to the controller.
const formContentType = "application/x-www-form-urlencoded"

// railsBracketBody reports whether body is a flat parameter map that uses
// Rails bracket convention, and therefore has to be form-encoded to survive
// the trip through Rack.
//
// Only top-level keys are inspected: the generated command tree always builds
// a flat map whose keys are the parameter names straight out of the API docs.
// A body handed in through --stdin as real nested JSON has no bracketed keys,
// stays on the JSON path, and keeps working.
func railsBracketBody(body any) (map[string]any, bool) {
	m, ok := body.(map[string]any)
	if !ok {
		return nil, false
	}
	for k := range m {
		if strings.Contains(k, "[") {
			return m, true
		}
	}
	return nil, false
}

// encodeRailsForm serializes a flat Rails-convention parameter map as a form
// body. Keys are emitted in sorted order so a given body always encodes
// byte-identically, which keeps --dry-run output and tests stable.
func encodeRailsForm(m map[string]any) string {
	vals := url.Values{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		appendFormValue(vals, k, m[k])
	}
	return vals.Encode()
}

// appendFormValue writes one parameter, expanding slices into repeated
// `key[]` entries and maps into `key[sub]` entries the way Rack expects to
// read them back.
func appendFormValue(vals url.Values, key string, v any) {
	switch t := v.(type) {
	case nil:
		// A nil value carries no instruction to the API; omitting it leaves
		// the field untouched rather than blanking it.
		return
	case string:
		vals.Add(key, t)
	case bool:
		vals.Add(key, strconv.FormatBool(t))
	case int:
		vals.Add(key, strconv.Itoa(t))
	case int64:
		vals.Add(key, strconv.FormatInt(t, 10))
	case float64:
		// Bodies decoded from --stdin arrive with every number as a float64.
		// 'f' with precision -1 renders 3 as "3", not "3e+00".
		vals.Add(key, strconv.FormatFloat(t, 'f', -1, 64))
	case json.Number:
		vals.Add(key, t.String())
	case []string:
		for _, e := range t {
			appendFormValue(vals, arrayKey(key), e)
		}
	case []any:
		for _, e := range t {
			appendFormValue(vals, arrayKey(key), e)
		}
	case map[string]any:
		subKeys := make([]string, 0, len(t))
		for sk := range t {
			subKeys = append(subKeys, sk)
		}
		sort.Strings(subKeys)
		for _, sk := range subKeys {
			appendFormValue(vals, key+"["+sk+"]", t[sk])
		}
	default:
		vals.Add(key, fmt.Sprintf("%v", t))
	}
}

// arrayKey returns the repeated-entry form of key.
//
// An empty bracket pair anywhere in the name already marks the list position:
// `course_ids[]` is a list of scalars and
// `assignment_overrides[][course_section_id]` is a field of a list of
// objects. Rack builds both from the same key repeated once per element, so
// appending a second `[]` would describe a list one level deeper than the API
// documents. Only names with no list position of their own get a suffix.
func arrayKey(key string) string {
	if strings.Contains(key, "[]") {
		return key
	}
	return key + "[]"
}
