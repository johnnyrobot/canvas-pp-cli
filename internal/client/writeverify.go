// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// A Canvas write can be accepted and ignored at the same time. Rails builds
// its params hash from the names it recognizes and drops the rest without
// complaint, so a PUT carrying parameter names the controller does not know
// comes back HTTP 200 with the unmodified resource. Nothing on the wire says
// the write was a no-op; the caller sees a success status and a plausible
// resource body and moves on.
//
// formencode.go removes the largest source of those names — bracket keys sent
// as JSON instead of form fields. What it cannot cover is --stdin, which hands
// the body through verbatim with no idea what the endpoint expects. A body
// like {"wiki_page_body": "..."} is well-formed JSON, is accepted, and changes
// nothing.
//
// This file closes that hole from the other side: instead of predicting which
// parameter names an endpoint accepts, compare the response against what was
// sent. If a mutating request comes back without a trace of any field it
// carried, the write did not land.

// WriteVerifyMode selects what happens when a mutating request returns a
// success status but its response carries no sign that the fields sent were
// understood.
type WriteVerifyMode string

const (
	// WriteVerifyOn fails the command on a hard finding and prints a warning
	// on a soft one. Default.
	WriteVerifyOn WriteVerifyMode = "on"
	// WriteVerifyWarn prints every finding to stderr and always exits zero.
	WriteVerifyWarn WriteVerifyMode = "warn"
	// WriteVerifyOff skips the check.
	WriteVerifyOff WriteVerifyMode = "off"
)

// ParseWriteVerifyMode maps a --verify-writes flag value onto a mode.
func ParseWriteVerifyMode(s string) (WriteVerifyMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "on", "true", "error", "strict":
		return WriteVerifyOn, nil
	case "warn", "warning":
		return WriteVerifyWarn, nil
	case "off", "none", "false":
		return WriteVerifyOff, nil
	}
	return "", fmt.Errorf("invalid --verify-writes value %q (want on, warn, or off)", s)
}

// WriteNotAppliedError reports a mutating request that the server accepted
// without applying. It is not a transport failure: the request succeeded, and
// the resource is unchanged.
type WriteNotAppliedError struct {
	Method string
	Path   string
	Detail string
}

func (e *WriteNotAppliedError) Error() string {
	return fmt.Sprintf("%s %s returned success but the write did not apply\n%s", e.Method, e.Path, e.Detail)
}

// writeFinding describes one failed verification.
type writeFinding struct {
	// hard marks a finding confident enough to fail the command. Only a flat,
	// non-bracketed, non-nested body earns it — the shape --stdin produces
	// when the caller guessed at parameter names. Bodies built from flags are
	// always bracketed and never reach it.
	hard bool
	// sentFields are the leaf names looked for in the response.
	sentFields []string
	// suggestion, when set, is the bracket name the sent key was probably
	// meant to be.
	suggestion string
	// sentKey is the top-level key that produced suggestion.
	sentKey string
}

// verifyWrite compares a mutating request's body against the response the
// server returned. It reports a finding only when nothing that was sent shows
// up in what came back; a nil finding means the write looks applied, or that
// there was not enough information to judge.
func verifyWrite(body any, respBody []byte) *writeFinding {
	m, ok := body.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}

	sent := collectSentFields(m)
	if len(sent) == 0 {
		return nil
	}

	// Only a JSON object can be compared field by field. A list response
	// (bulk endpoints), a bare scalar, or an empty 204 body carries no
	// resource to check against.
	var resp map[string]any
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp) == 0 {
		return nil
	}

	for _, field := range sent {
		if _, present := resp[field]; present {
			return nil
		}
	}

	// Nothing sent came back. How much that proves depends on the shape of the
	// body: a flat map with no brackets and no nesting is the shape --stdin
	// produces when the caller invented parameter names, and Canvas has no
	// endpoint that takes its write parameters that way.
	finding := &writeFinding{hard: isFlatGuessBody(m), sentFields: sent}
	sort.Strings(finding.sentFields)

	for _, k := range sortedKeys(m) {
		if s, ok := flattenedBracketSuggestion(k, resp); ok {
			finding.sentKey = k
			finding.suggestion = s
			break
		}
	}
	return finding
}

// isFlatGuessBody reports whether every top-level key is a plain name with no
// bracket and every value is a scalar. Bodies built from command flags always
// carry bracket names, and a correctly shaped --stdin body nests its fields
// under a resource key; neither reaches this.
func isFlatGuessBody(m map[string]any) bool {
	for k, v := range m {
		if strings.Contains(k, "[") {
			return false
		}
		switch v.(type) {
		case map[string]any, []any, []string:
			return false
		}
	}
	return true
}

// collectSentFields reduces a request body to the leaf field names a response
// would carry, following one level of nesting for bodies handed in as real
// JSON.
func collectSentFields(m map[string]any) []string {
	var out []string
	for _, k := range sortedKeys(m) {
		if sub, ok := m[k].(map[string]any); ok && !strings.Contains(k, "[") {
			// {"wiki_page": {"body": "..."}} — the fields live one level down.
			for _, sk := range sortedKeys(sub) {
				out = append(out, leafFieldName(sk))
			}
			continue
		}
		out = append(out, leafFieldName(k))
	}
	return out
}

// leafFieldName strips Rails bracket decoration down to the name the resource
// itself uses: wiki_page[body] -> body, course_ids[] -> course_ids,
// assignment_overrides[][course_section_id] -> course_section_id.
func leafFieldName(key string) string {
	k := strings.TrimSuffix(key, "[]")
	if i := strings.LastIndex(k, "["); i >= 0 {
		inner := strings.TrimSuffix(k[i+1:], "]")
		inner = strings.TrimSuffix(inner, "[]")
		if inner != "" {
			return inner
		}
		k = k[:i]
	}
	if i := strings.Index(k, "["); i >= 0 {
		k = k[:i]
	}
	return k
}

// flattenedBracketSuggestion detects the specific mistake of writing a bracket
// parameter as an underscore-joined name — wiki_page_body for
// wiki_page[body]. It only fires when the tail half of the split is a real
// field on the returned resource, which makes a coincidental hit unlikely.
func flattenedBracketSuggestion(sentKey string, resp map[string]any) (string, bool) {
	if strings.Contains(sentKey, "[") || !strings.Contains(sentKey, "_") {
		return "", false
	}
	parts := strings.Split(sentKey, "_")
	// Prefer the longest prefix, so wiki_page_body suggests wiki_page[body]
	// rather than wiki[page_body].
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], "_")
		suffix := strings.Join(parts[i:], "_")
		if _, ok := resp[suffix]; ok {
			return prefix + "[" + suffix + "]", true
		}
	}
	return "", false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// message renders a finding as operator- and agent-readable prose. It names
// what to do next, because the caller that trips this is usually a script or
// an agent that has already decided the write succeeded.
func (f *writeFinding) message(method, path string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  the request carried %s, and the response object contains none of them\n",
		quoteList(f.sentFields))
	if f.suggestion != "" {
		fmt.Fprintf(&b, "  %q looks like a flattened Rails bracket name; the API parameter is %q\n",
			f.sentKey, f.suggestion)
		fmt.Fprintf(&b, "  send it as nested JSON, or use the matching flag:\n")
		fmt.Fprintf(&b, "    --stdin  <<< '{%q: {%q: \"...\"}}'\n",
			strings.SplitN(f.suggestion, "[", 2)[0],
			strings.TrimSuffix(strings.SplitN(f.suggestion, "[", 2)[1], "]"))
		fmt.Fprintf(&b, "    --%s \"...\"\n", flagNameFor(f.suggestion))
	} else {
		b.WriteString("  Canvas ignores parameter names it does not recognize and still answers 200;\n")
		b.WriteString("  check the parameter names against `" + path + "` in the Canvas API docs,\n")
		b.WriteString("  or use the command's flags, which carry the correct names.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// flagNameFor renders wiki_page[body] as the wiki-page-body flag the command
// tree exposes for it.
func flagNameFor(bracket string) string {
	s := strings.ReplaceAll(bracket, "[", "_")
	s = strings.ReplaceAll(s, "]", "")
	return strings.ReplaceAll(s, "_", "-")
}

func quoteList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, s := range items {
		quoted = append(quoted, strconv.Quote(s))
	}
	switch len(quoted) {
	case 0:
		return "no fields"
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + ", and " + quoted[len(quoted)-1]
}

// checkWriteApplied runs the post-write comparison and reports it according to
// the configured mode. A hard finding under WriteVerifyOn is returned as an
// error so the command exits non-zero; everything else goes to stderr, which
// keeps --json and --agent output on stdout intact.
func (c *Client) checkWriteApplied(method, path string, body any, respBody []byte, authHeader string) error {
	if c.WriteVerify == WriteVerifyOff || c.DryRun {
		return nil
	}
	finding := verifyWrite(body, respBody)
	if finding == nil {
		return nil
	}
	display := c.displayURL(path, authHeader)
	detail := finding.message(method, display)

	if finding.hard && c.WriteVerify == WriteVerifyOn {
		return &WriteNotAppliedError{Method: method, Path: display, Detail: detail}
	}
	fmt.Fprintf(os.Stderr, "warning: %s %s returned success but the write may not have applied\n%s\n",
		method, display, detail)
	return nil
}
