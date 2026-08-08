// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for MCP response bounding on the live execute path.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// executeAgainst runs the real canvas_execute handler against a stub Canvas
// that returns body, and gives back the tool result's text.
//
// This drives handleCodeOrchExecute end to end — config resolution, client
// construction, the HTTP call and result building — because the bug being
// guarded here lived in the last step of that chain and nothing above it.
func executeAgainst(t *testing.T, endpointID, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("stub write failed: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	// Point config at the stub and give it a token so auth resolution passes.
	t.Setenv("CANVAS_BASE_URL", srv.URL)
	t.Setenv("CANVAS_API_TOKEN", "test-token")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"endpoint_id": endpointID}

	res, err := handleCodeOrchExecute(context.Background(), req)
	if err != nil {
		t.Fatalf("handleCodeOrchExecute error = %v", err)
	}
	if res == nil {
		t.Fatal("handleCodeOrchExecute returned no result")
	}
	return mcpTextContent(t, res)
}

// firstGetEndpointID finds a GET endpoint with no required path parameters, so
// the test does not depend on any particular Canvas route surviving a regen.
func firstGetEndpointID(t *testing.T) string {
	t.Helper()
	for _, ep := range codeOrchEndpoints {
		if strings.EqualFold(ep.Method, "GET") && !strings.Contains(ep.Path, "{") {
			return ep.ID
		}
	}
	t.Skip("no parameterless GET endpoint in the catalogue")
	return ""
}

// TestCodeOrchExecute_BoundsLargeListResponse is the regression guard.
//
// handleCodeOrchExecute returned NewToolResultText(string(data)) — whatever
// Canvas sent, straight into the agent's context. Meanwhile ~280 lines of
// truncation logic sat in tools.go with zero callers, and four passing tests
// guarded code that could never run.
func TestCodeOrchExecute_BoundsLargeListResponse(t *testing.T) {
	// A list far past both the byte ceiling and the item ceiling.
	items := make([]string, 4000)
	for i := range items {
		items[i] = fmt.Sprintf(`{"id":%d,"name":"course %d","blurb":%q}`, i, i, strings.Repeat("x", 200))
	}
	body := "[" + strings.Join(items, ",") + "]"

	if len(body) <= mcpToolResultMaxBytes {
		t.Fatalf("fixture is only %d bytes; it must exceed the %d-byte ceiling to exercise bounding",
			len(body), mcpToolResultMaxBytes)
	}

	got := executeAgainst(t, firstGetEndpointID(t), body)

	if len(got) >= len(body) {
		t.Fatalf("result is %d bytes for a %d-byte response: not bounded", len(got), len(body))
	}
	if len(got) > mcpToolResultMaxBytes*2 {
		t.Errorf("result is %d bytes, far past the %d-byte ceiling", len(got), mcpToolResultMaxBytes)
	}

	// The envelope must say what was elided, or the agent silently sees a
	// partial list and treats it as the whole thing.
	var env map[string]any
	if err := json.Unmarshal([]byte(got), &env); err != nil {
		t.Fatalf("bounded result is not JSON: %v\n%s", err, truncate(got))
	}
	if _, ok := env["items"]; !ok {
		t.Errorf("bounded envelope has no items key: %v", keysOf(env))
	}
	if !mentionsTruncation(env) {
		t.Errorf("bounded envelope does not report truncation; keys = %v", keysOf(env))
	}
}

// TestCodeOrchExecute_SmallResponsePassesThrough keeps bounding from becoming
// a tax on ordinary calls: a response under the ceiling must arrive verbatim.
func TestCodeOrchExecute_SmallResponsePassesThrough(t *testing.T) {
	body := `{"id":1,"name":"Intro Physics"}`
	got := executeAgainst(t, firstGetEndpointID(t), body)

	if strings.TrimSpace(got) != body {
		t.Errorf("small response was altered:\n got  %s\n want %s", got, body)
	}
}

func mentionsTruncation(env map[string]any) bool {
	for k, v := range env {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "truncat") || strings.Contains(lk, "omitted") ||
			strings.Contains(lk, "total") || strings.Contains(lk, "max") ||
			strings.Contains(lk, "returned") || strings.Contains(lk, "count") {
			return true
		}
		if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), "truncat") {
			return true
		}
	}
	return false
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}
