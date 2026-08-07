// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for local-mirror failure reporting.

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe and returns what fn wrote to it.
// writeMutationResponseToStore takes no writer — it is called from 400
// generated sites that pass only (ctx, resourceType, data, path) — so the
// warning goes to os.Stderr directly and this is how to observe it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	fn()
	if err := w.Close(); err != nil {
		t.Errorf("close pipe: %v", err)
	}
	return <-done
}

// breakTheStore points the data directory at a regular file, so opening the
// SQLite mirror underneath it cannot succeed.
func breakTheStore(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	t.Setenv("XDG_DATA_HOME", blocker)
}

// resetMirrorWarn lets each test observe the once-per-process warning.
func resetMirrorWarn(t *testing.T) {
	t.Helper()
	mirrorWarnOnce = sync.Once{}
	t.Cleanup(func() { mirrorWarnOnce = sync.Once{} })
}

// TestWriteMutationResponseToStore_WarnsWhenMirrorFails is the regression
// guard. Both the open error and the upsert result were discarded, so a
// corrupt or unwritable store stayed broken indefinitely with no signal, and
// later --data-source local reads served stale data.
func TestWriteMutationResponseToStore_WarnsWhenMirrorFails(t *testing.T) {
	resetMirrorWarn(t)
	breakTheStore(t)

	out := captureStderr(t, func() {
		writeMutationResponseToStore(context.Background(), "courses",
			json.RawMessage(`{"id":"1","name":"Intro"}`), "")
	})

	if !strings.Contains(out, "local mirror not updated") {
		t.Errorf("no mirror warning on stderr; got %q", out)
	}
	if !strings.Contains(out, "the request itself succeeded") {
		t.Errorf("warning must say the request still succeeded, got %q", out)
	}
}

// TestWriteMutationResponseToStore_WarnsOnlyOnce keeps a persistently broken
// store from emitting one line per mutation in a loop.
func TestWriteMutationResponseToStore_WarnsOnlyOnce(t *testing.T) {
	resetMirrorWarn(t)
	breakTheStore(t)

	out := captureStderr(t, func() {
		for i := 0; i < 5; i++ {
			writeMutationResponseToStore(context.Background(), "courses",
				json.RawMessage(`{"id":"1"}`), "")
		}
	})

	if n := strings.Count(out, "local mirror not updated"); n != 1 {
		t.Errorf("warned %d times across 5 mutations, want exactly 1", n)
	}
}

// TestWriteMutationResponseToStore_SucceedsQuietly keeps the warning from
// becoming noise on the healthy path.
func TestWriteMutationResponseToStore_SucceedsQuietly(t *testing.T) {
	resetMirrorWarn(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	out := captureStderr(t, func() {
		writeMutationResponseToStore(context.Background(), "courses",
			json.RawMessage(`{"id":"1","name":"Intro"}`), "")
	})

	if strings.TrimSpace(out) != "" {
		t.Errorf("a healthy mirror write must be silent, got %q", out)
	}
}

// TestWriteMutationResponseToStore_NoItemsIsSilent covers the early return:
// a response carrying nothing to mirror is not a failure.
func TestWriteMutationResponseToStore_NoItemsIsSilent(t *testing.T) {
	resetMirrorWarn(t)
	breakTheStore(t)

	out := captureStderr(t, func() {
		writeMutationResponseToStore(context.Background(), "courses",
			json.RawMessage(`null`), "")
	})

	if strings.TrimSpace(out) != "" {
		t.Errorf("nothing to mirror must not warn, got %q", out)
	}
}
