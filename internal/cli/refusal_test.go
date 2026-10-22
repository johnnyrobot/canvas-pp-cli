// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests: Canvas Builder's refusals reach the agent word for
// word, and nothing in the output sends it looking for another token.

package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// refusalEnvelope is the exit-9 JSON an agent reads on stdout.
type refusalEnvelope struct {
	Error     string `json:"error"`
	Code      int    `json:"code"`
	Status    int    `json:"status"`
	RefusedBy string `json:"refused_by"`
}

// TestRefusalSurfacedUnchanged is Review Focus 7: one row per gateway message,
// each with the status it arrives with.
func TestRefusalSurfacedUnchanged(t *testing.T) {
	cases := []struct {
		name   string
		msg    string
		status int
	}{
		{"blocked", sandbox.MsgBlocked, http.StatusForbidden},
		{"unknown token", sandbox.MsgUnknownToken, http.StatusForbidden},
		{"wrong course", sandbox.MsgWrongCourse, http.StatusForbidden},
		{"quarantine", sandbox.MsgQuarantine, http.StatusLocked},
		{"denied", sandbox.MsgDenied, http.StatusForbidden},
		{"too large", sandbox.MsgTooLarge, http.StatusRequestEntityTooLarge},
		// MsgNotRecorded arrives as either 503 or 400. The 400 is the row
		// here because a 503 is also retried, which is Task 3's subject and
		// would only make this test wait out the backoff.
		{"not recorded", sandbox.MsgNotRecorded, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
			f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, _ *http.Request) {
				gatewaytest.Refuse(w, tc.status, tc.msg)
			})

			stdout, stderr, code := runCLI(t, "pages", "index", "7", "--agent")

			if code != 9 {
				t.Fatalf("exit code = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			var env refusalEnvelope
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("stdout isn't the exit-9 envelope: %v\nstdout: %s", err, stdout)
			}
			if env.Error != tc.msg {
				t.Fatalf("envelope error = %q, want %q", env.Error, tc.msg)
			}
			if env.Code != 9 {
				t.Fatalf("envelope code = %d, want 9", env.Code)
			}
			if env.Status != tc.status {
				t.Fatalf("envelope status = %d, want %d", env.Status, tc.status)
			}
			if env.RefusedBy != "Canvas Builder" {
				t.Fatalf("envelope refused_by = %q, want %q", env.RefusedBy, "Canvas Builder")
			}
			if want := "Error: " + tc.msg; !strings.Contains(stderr, want) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr, want)
			}
			for stream, text := range map[string]string{"stdout": stdout, "stderr": stderr} {
				for _, forbidden := range []string{"CANVAS_API_TOKEN", "export"} {
					if strings.Contains(text, forbidden) {
						t.Fatalf("%s mentions %q, which tells the agent to go find another token: %s", stream, forbidden, text)
					}
				}
			}
		})
	}
}

// TestCanvasErrorKeepsCanvasText checks the new branch only catches Canvas
// Builder's own sentences. Canvas's errors arrive in the same envelope shape.
func TestCanvasErrorKeepsCanvasText(t *testing.T) {
	const canvasMsg = "The specified resource does not exist."
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, _ *http.Request) {
		gatewaytest.Refuse(w, http.StatusNotFound, canvasMsg)
	})

	stdout, stderr, code := runCLI(t, "pages", "index", "7", "--agent")

	if code != 3 {
		t.Fatalf("exit code = %d, want 3\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, canvasMsg) {
		t.Fatalf("stderr = %q, want it to keep Canvas's own text %q", stderr, canvasMsg)
	}
}

// TestSandboxAuthErrorHasNoTokenHint is the other half of ADR 0001: in Canvas
// Builder the agent can't change the token, so the hint that it should is both
// wrong and an invitation to go looking.
func TestSandboxAuthErrorHasNoTokenHint(t *testing.T) {
	// A 401 from Canvas itself, not one of Canvas Builder's messages.
	const canvasMsg = "user authorization required"

	for _, tc := range []struct {
		name     string
		omit     bool
		wantHint bool
	}{
		{"in Canvas Builder", false, false},
		{"in the plain CLI", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: tc.omit})
			f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, _ *http.Request) {
				gatewaytest.Refuse(w, http.StatusUnauthorized, canvasMsg)
			})

			stdout, stderr, code := runCLI(t, "pages", "index", "7", "--agent")

			if code != 4 {
				t.Fatalf("exit code = %d, want 4\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			both := stdout + stderr
			if !strings.Contains(both, canvasMsg) {
				t.Fatalf("output = %q, want it to keep Canvas's own text %q", both, canvasMsg)
			}
			hinted := strings.Contains(both, "CANVAS_API_TOKEN")
			if hinted != tc.wantHint {
				t.Fatalf("output mentions CANVAS_API_TOKEN = %v, want %v: %s", hinted, tc.wantHint, both)
			}
		})
	}
}
