// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored test for the bundled build's stamp: it must fail closed when
// Canvas Builder's settings aren't there, and the ordinary build must not.

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const ldflagsStamp = "-X github.com/johnnyrobot/canvas-pp-cli/internal/sandbox.requiredStamp=canvas-builder"

const msgSettings = "Canvas Builder's settings for this workspace are missing or damaged, so the Canvas CLI won't run. Open the course from Canvas Builder again."

// TestStampedBinaryFailsClosed [Review Focus 3]: an agent that points the
// bundled CLI at an empty folder gets a stop, not a CLI talking to real Canvas.
func TestStampedBinaryFailsClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}

	build := func(t *testing.T, name string, ldflags string) string {
		t.Helper()
		out := filepath.Join(t.TempDir(), name)
		args := []string{"build", "-o", out}
		if ldflags != "" {
			args = append(args, "-ldflags", ldflags)
		}
		args = append(args, ".")
		cmd := exec.CommandContext(t.Context(), goTool, args...)
		if combined, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, combined)
		}
		return out
	}

	// An empty CLI home and an empty HOME: no settings, no legacy config, no token.
	emptyEnv := func(t *testing.T) []string {
		t.Helper()
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + t.TempDir(),
			"CANVAS_HOME=" + t.TempDir(),
		}
	}

	run := func(t *testing.T, bin string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), bin, args...)
		cmd.Env = emptyEnv(t)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			return 0, stderr.String()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), stderr.String()
		}
		t.Fatalf("run %s: %v", filepath.Base(bin), err)
		return -1, ""
	}

	t.Run("stamped", func(t *testing.T) {
		code, stderr := run(t, build(t, "canvas-pp-cli-sandbox", ldflagsStamp), "pages", "index", "7")
		if code != 10 {
			t.Errorf("exit code = %d, want 10\n%s", code, stderr)
		}
		if !strings.Contains(stderr, msgSettings) {
			t.Errorf("stderr = %q, want the settings message", stderr)
		}
	})

	t.Run("unstamped", func(t *testing.T) {
		code, stderr := run(t, build(t, "canvas-pp-cli", ""), "pages", "index", "7", "--dry-run")
		if code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, stderr)
		}
	})
}
