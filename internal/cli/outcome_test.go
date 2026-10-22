// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the exit code and the envelope an unknown outcome
// gets, which is how an agent learns it has to look at the course itself.

package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// captureStdout runs fn with os.Stdout replaced by a pipe, because the error
// envelope is written to os.Stdout directly.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("opening a pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()
	fn()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatalf("closing the pipe: %v", err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatalf("closing the pipe: %v", err)
	}
	return out
}

func TestExitCodeOutcomeUnknown(t *testing.T) {
	unknown := &client.OutcomeUnknownError{
		Method: "POST",
		Path:   "/api/v1/courses/7/pages",
		Cause:  errors.New("POST /api/v1/courses/7/pages: EOF"),
	}

	var classified error
	out := captureStdout(t, func() {
		classified = classifyAPIError(unknown, &rootFlags{asJSON: true})
	})

	if got := ExitCode(classified); got != 8 {
		t.Fatalf("ExitCode = %d, want 8", got)
	}
	if classified.Error() != sandbox.MsgUnknownOutcome {
		t.Fatalf("error = %q, want exactly the unknown-outcome message", classified)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("the envelope %q isn't JSON: %v", out, err)
	}
	if envelope["error"] != sandbox.MsgUnknownOutcome {
		t.Fatalf("envelope error = %v, want the unknown-outcome message", envelope["error"])
	}
	if envelope["code"] != float64(8) {
		t.Fatalf("envelope code = %v, want 8", envelope["code"])
	}
	if envelope["outcome"] != "unknown" {
		t.Fatalf("envelope outcome = %v, want \"unknown\"", envelope["outcome"])
	}
}

// The envelope is the agent's only machine-readable channel, so a wrapped
// unknown outcome has to be found inside whatever the command wrapped it in.
func TestOutcomeUnknownFoundWhenWrapped(t *testing.T) {
	wrapped := errors.Join(errors.New("creating the page"), &client.OutcomeUnknownError{Method: "POST", Path: "/api/v1/courses/7/pages"})
	var classified error
	_ = captureStdout(t, func() {
		classified = classifyAPIError(wrapped, &rootFlags{})
	})
	if got := ExitCode(classified); got != 8 {
		t.Fatalf("ExitCode = %d, want 8", got)
	}
}
