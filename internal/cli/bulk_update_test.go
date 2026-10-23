// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for `assignments bulk-update`: the list an agent writes
// on stdin is the list Canvas gets, and anything that isn't a list is turned
// away before a request goes out.
//
// The shape matters more here than in most commands. Canvas's bulk_update
// takes a JSON array, and Canvas Builder reads that same array to work out
// which assignments to snapshot before the write — so a body that arrives as
// an object, or as an array the CLI has rebuilt, costs the undo record as well
// as the edit.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
)

// termMove is the body the term move sends: two assignments, each with its
// base date row.
const termMove = `[{"id":11,"all_dates":[{"base":true,"due_at":"2027-05-06T06:59:00Z"}]},
	{"id":12,"all_dates":[{"base":true,"due_at":"2027-05-13T06:59:00Z","unlock_at":"2027-05-06T07:00:00Z"}]}]`

// progressBody is what Canvas answers bulk_update with: the job, not the
// assignments. The gateway's undo tests use the same shape.
const progressBody = `{"id":1001,"workflow_state":"queued","tag":"assignment_bulk_update"}`

// withStdin points os.Stdin at a file holding body for the length of one test.
// The --stdin flag reads os.Stdin directly rather than through cobra, so there
// is no writer to inject. A file rather than a pipe means a command that never
// reads stdin can't leave the test blocked on a write.
func withStdin(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the stdin file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the stdin file: %v", err)
	}
	real := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = real
		_ = f.Close()
	})
}

// compact is JSON with the whitespace taken out, so a body can be compared to
// what was written without the comparison being about indentation.
func compact(t *testing.T, raw string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		t.Fatalf("compacting %q: %v", raw, err)
	}
	return buf.String()
}

// handleBulkUpdate answers the one endpoint with a Progress and reports what
// it was sent.
func handleBulkUpdate(f *gatewaytest.Fake) {
	f.Handle(http.MethodPut, "/api/v1/courses/7/assignments/bulk_update", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(progressBody))
	})
}

// TestBulkUpdateSendsArray is the whole point of the task: the list goes out as
// a JSON list, under a content type Canvas Builder will read as JSON.
func TestBulkUpdateSendsArray(t *testing.T) {
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	handleBulkUpdate(f)
	withStdin(t, termMove)

	stdout, stderr, code := runCLI(t, "assignments", "bulk-update", "7", "--stdin", "--agent")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	reqs := f.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1: %+v", len(reqs), reqs)
	}
	got := reqs[0]
	if ct := got.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want %q; Canvas Builder reads the list only from a JSON body", ct, "application/json")
	}
	if want := compact(t, termMove); string(got.Body) != want {
		t.Fatalf("body = %s\nwant %s", got.Body, want)
	}
	// A list rebuilt from a Go value would still look like a list; what this
	// checks is that the ids themselves came through, since those are what the
	// before-snapshot is built from.
	var sent []map[string]any
	if err := json.Unmarshal(got.Body, &sent); err != nil {
		t.Fatalf("the body Canvas got isn't a JSON list: %v", err)
	}
	if len(sent) != 2 || sent[0]["id"] != float64(11) || sent[1]["id"] != float64(12) {
		t.Fatalf("ids sent = %+v, want 11 and 12", sent)
	}
	// Canvas's answer is the queued job, and it reaches the agent as it is.
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			ID            int    `json:"id"`
			WorkflowState string `json:"workflow_state"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout isn't the envelope: %v\nstdout: %s", err, stdout)
	}
	if !env.Success || env.Data.ID != 1001 || env.Data.WorkflowState != "queued" {
		t.Fatalf("envelope doesn't carry the queued job: %s", stdout)
	}
}

// TestBulkUpdateRefusesObject covers every body that isn't a list of objects.
// All of them stop before a request goes out: an agent that sent the wrong
// shape gets the example back, and the course is left alone.
func TestBulkUpdateRefusesObject(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
	}{
		{"an object", `{"id":11,"all_dates":[]}`, []string{"--stdin"}},
		{"an empty list", `[]`, []string{"--stdin"}},
		{"a list of numbers", `[11,12]`, []string{"--stdin"}},
		{"a list holding a list", `[[{"id":11}]]`, []string{"--stdin"}},
		{"not JSON at all", "id=11", []string{"--stdin"}},
		{"nothing at all", "", []string{"--stdin"}},
		{"no --stdin", termMove, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
			handleBulkUpdate(f)
			withStdin(t, tc.stdin)

			args := append([]string{"assignments", "bulk-update", "7"}, tc.args...)
			stdout, stderr, code := runCLI(t, append(args, "--agent")...)

			if code != 2 {
				t.Fatalf("exit code = %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stderr, msgBulkUpdateNeedsList) {
				t.Fatalf("stderr doesn't carry the example: %s", stderr)
			}
			if n := len(f.Requests()); n != 0 {
				t.Fatalf("requests = %d, want 0: a body the CLI can see is wrong shouldn't reach Canvas", n)
			}
		})
	}
}

// TestBulkUpdateDryRun: --dry-run shows the list it would have sent and sends
// nothing.
func TestBulkUpdateDryRun(t *testing.T) {
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	handleBulkUpdate(f)
	withStdin(t, termMove)

	stdout, stderr, code := runCLI(t, "assignments", "bulk-update", "7", "--stdin", "--dry-run", "--agent")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if n := len(f.Requests()); n != 0 {
		t.Fatalf("requests = %d, want 0", n)
	}
	for _, want := range []string{"PUT", "/api/v1/courses/7/assignments/bulk_update", `"id": 11`, "no request sent"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("the preview doesn't mention %q: %s", want, stderr)
		}
	}
	var env struct {
		DryRun  bool `json:"dry_run"`
		Success bool `json:"success"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout isn't the envelope: %v\nstdout: %s", err, stdout)
	}
	if !env.DryRun || env.Success {
		t.Fatalf("envelope says dry_run=%v success=%v, want true and false: %s", env.DryRun, env.Success, stdout)
	}
}

// TestBulkUpdateDoesNotMirrorProgress: Canvas answers with a Progress, whose id
// is a job id. Filing it in the local copy of the course would leave an
// assignment 1001 that doesn't exist, and `--data-source local` would then
// serve it. Nothing is written at all, so the mirror file never appears.
func TestBulkUpdateDoesNotMirrorProgress(t *testing.T) {
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	handleBulkUpdate(f)
	withStdin(t, termMove)

	stdout, stderr, code := runCLI(t, "assignments", "bulk-update", "7", "--stdin", "--agent")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	db := defaultDBPath("canvas-pp-cli")
	if _, err := os.Stat(db); err == nil {
		t.Fatalf("the local copy at %s was written; a Progress is not an assignment", db)
	}
}
