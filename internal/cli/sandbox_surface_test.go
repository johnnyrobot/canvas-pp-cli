// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: what the CLI offers and refuses when it runs inside Canvas
// Builder. These tests run the whole command tree through runCLI, because the
// surface is a property of the tree rather than of any one command.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// whichOutput is the `which --json` envelope.
type whichOutput struct {
	Matches []struct {
		Entry struct {
			Command string `json:"command"`
		} `json:"entry"`
		Score int `json:"score"`
	} `json:"matches"`
}

// TestSandboxRefusesStudentDataCommands: the six cross-resource commands are
// built on student records, which Canvas Builder's copy of Canvas doesn't hold.
// The refusal comes before any request, so the fake sees nothing.
func TestSandboxRefusesStudentDataCommands(t *testing.T) {
	for _, args := range [][]string{
		{"roster", "7"},
		{"at-risk"},
		{"to-grade"},
		{"standings"},
		{"audit-enrollments"},
		{"since", "7d"},
	} {
		t.Run(args[0], func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, append(args, "--agent")...)
			if code != 9 {
				t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			var env refusalEnvelope
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("parsing %q: %v", stdout, err)
			}
			if env.Error != sandbox.MsgStudentData {
				t.Errorf("error = %q, want %q", env.Error, sandbox.MsgStudentData)
			}
			if env.RefusedBy != "Canvas Builder" {
				t.Errorf("refused_by = %q, want Canvas Builder", env.RefusedBy)
			}
			if got := len(f.Requests()); got != 0 {
				t.Errorf("the fake saw %d requests, want 0", got)
			}
		})
	}
}

// TestSandboxHidesStudentDataFromWhichAndHelp: an agent discovers commands
// through `which` and `--help`, so a command it may not run has to be absent
// from both. Offering it and then refusing it wastes a turn.
func TestSandboxHidesStudentDataFromWhichAndHelp(t *testing.T) {
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "which", "at risk students", "--json")
	if code != 0 {
		t.Fatalf("which exit = %d, want 0\nstderr: %s", code, stderr)
	}
	var out whichOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("parsing %q: %v", stdout, err)
	}
	for _, m := range out.Matches {
		if m.Entry.Command == "at-risk" {
			t.Errorf("which offered at-risk in Canvas Builder: %s", stdout)
		}
	}

	help, stderr, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("--help exit = %d, want 0\nstderr: %s", code, stderr)
	}
	for _, name := range studentDataCommands {
		if strings.Contains(help, name) {
			t.Errorf("--help names %q in Canvas Builder:\n%s", name, help)
		}
	}
}

// TestSandboxWhichOffersRecipesForItsJobs: Canvas Builder drops the six
// cross-resource capabilities from the index, so what is left has to answer
// the jobs an agent is actually sent here to do. A recipe is the answer to
// most of them, and a recipe nothing reaches is a recipe nobody runs.
func TestSandboxWhichOffersRecipesForItsJobs(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"move a course to a new term", "recipes move-to-new-term"},
		{"fix accessibility problems", "recipes accessibility-remediation"},
		{"put an image in a page", "recipes insert-image"},
		{"review the course design", "recipes design-review"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, "which", tc.query, "--json", "--limit", "1")
			if code != 0 {
				t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
			}
			var out whichOutput
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("parsing %q: %v", stdout, err)
			}
			if len(out.Matches) == 0 || out.Matches[0].Entry.Command != tc.want {
				t.Fatalf("which %q = %s, want %s", tc.query, stdout, tc.want)
			}
		})
	}
}

// TestSandboxWhichOffersOnlyCourseScopedCommands: every command `which`
// returns has to be one the gateway can trace to this course, or Canvas
// Builder would refuse it on arrival.
func TestSandboxWhichOffersOnlyCourseScopedCommands(t *testing.T) {
	for _, query := range []string{"list content migrations", "list files"} {
		t.Run(query, func(t *testing.T) {
			f := gatewaytest.New(t)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, "which", query, "--json", "--limit", "10")
			if code != 0 {
				t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
			}
			var out whichOutput
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("parsing %q: %v", stdout, err)
			}
			if len(out.Matches) == 0 {
				t.Fatalf("no matches for %q, so the test proves nothing", query)
			}
			root := RootCmd()
			for _, m := range out.Matches {
				cmd := findCommandByRoute(root, m.Entry.Command)
				if cmd == nil {
					t.Errorf("which offered %q, which isn't in the command tree", m.Entry.Command)
					continue
				}
				// A command with no pp:path is one of this CLI's own, such as
				// doctor or which, and reaches no Canvas endpoint at all.
				path, ok := cmd.Annotations["pp:path"]
				if ok && !courseTraceable(path) {
					t.Errorf("which offered %q (%s), which the gateway can't trace to this course", m.Entry.Command, path)
				}
			}
		})
	}
}

// TestNormalModeSurfaceUnchanged: outside Canvas Builder this is the same CLI
// it was. TestWhichResolves's cases are the contract, so they are re-run here
// with no Canvas Builder config in sight.
func TestNormalModeSurfaceUnchanged(t *testing.T) {
	f := gatewaytest.New(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: true})

	for _, tc := range []struct{ query, want string }{
		{"at risk students", "at-risk"},
		{"grading queue", "to-grade"},
		{"unified roster", "roster"},
		{"term grade distribution", "standings"},
		{"enrollment anomalies", "audit-enrollments"},
		{"create an assignment", "assignments create"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, "which", tc.query, "--json", "--limit", "1")
			if code != 0 {
				t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
			}
			var out whichOutput
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("parsing %q: %v", stdout, err)
			}
			if len(out.Matches) == 0 || out.Matches[0].Entry.Command != tc.want {
				t.Fatalf("which %q = %s, want %s", tc.query, stdout, tc.want)
			}
		})
	}

	help, _, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
	if !strings.Contains(help, "roster") {
		t.Errorf("--help outside Canvas Builder lost roster:\n%s", help)
	}
}

// countingListener answers every connection and counts it, so a test can say
// "nothing was sent" about a destination that would have accepted it.
func countingListener(t *testing.T) (url string, connections *atomic.Int64) {
	t.Helper()
	connections = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connections.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/hook", connections
}

// TestSandboxRefusesWebhookAndFeedbackSend [Review Focus 7]: in Canvas Builder
// this CLI talks to Canvas Builder and to nothing else. Both of the ways a run
// could post somewhere of its own choosing are turned off.
func TestSandboxRefusesWebhookAndFeedbackSend(t *testing.T) {
	t.Run("deliver webhook", func(t *testing.T) {
		f := gatewaytest.New(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
		hook, connections := countingListener(t)

		stdout, stderr, code := runCLI(t, "pages", "index", "7", "--agent", "--deliver", "webhook:"+hook)
		if code != 9 {
			t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		var env refusalEnvelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if env.Error != sandbox.MsgStaysLocal {
			t.Errorf("error = %q, want %q", env.Error, sandbox.MsgStaysLocal)
		}
		if got := connections.Load(); got != 0 {
			t.Errorf("the webhook saw %d connections, want 0", got)
		}
		if got := len(f.Requests()); got != 0 {
			t.Errorf("the fake saw %d requests, want 0", got)
		}
	})

	t.Run("feedback send", func(t *testing.T) {
		f := gatewaytest.New(t)
		home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
		hook, connections := countingListener(t)
		t.Setenv("CANVAS_FEEDBACK_ENDPOINT", hook)

		stdout, stderr, code := runCLI(t, "feedback", "the dates check missed one", "--send", "--agent")
		if code != 9 {
			t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		var env refusalEnvelope
		if err := json.Unmarshal([]byte(stdout), &env); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if env.Error != sandbox.MsgStaysLocal {
			t.Errorf("error = %q, want %q", env.Error, sandbox.MsgStaysLocal)
		}
		if got := connections.Load(); got != 0 {
			t.Errorf("the feedback endpoint saw %d connections, want 0", got)
		}
		// Only the sending is refused. The instructor's workspace still keeps
		// the note, so nothing an agent wrote is lost.
		ledger, err := os.ReadFile(filepath.Join(home.Dir, "data", "feedback.jsonl"))
		if err != nil {
			t.Fatalf("reading the feedback ledger: %v", err)
		}
		if !strings.Contains(string(ledger), "the dates check missed one") {
			t.Errorf("the feedback ledger didn't keep the entry: %s", ledger)
		}
	})

	t.Run("feedback auto-send", func(t *testing.T) {
		f := gatewaytest.New(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
		hook, connections := countingListener(t)
		t.Setenv("CANVAS_FEEDBACK_ENDPOINT", hook)
		t.Setenv("CANVAS_FEEDBACK_AUTO_SEND", "true")

		stdout, _, code := runCLI(t, "feedback", "a note", "--agent")
		if code != 9 {
			t.Fatalf("exit = %d, want 9\nstdout: %s", code, stdout)
		}
		if got := connections.Load(); got != 0 {
			t.Errorf("the feedback endpoint saw %d connections, want 0", got)
		}
	})
}

// TestSandboxRefusesTokenChanges: Canvas Builder sets this workspace's token up
// and an agent may not swap it for one with more reach (ADR 0001). The
// credentials file is compared byte for byte, because a refusal that still
// wrote would be worse than no refusal at all.
func TestSandboxRefusesTokenChanges(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "set-token", "a-token-of-its-own"},
		{"auth", "logout"},
		{"auth", "setup"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			f := gatewaytest.New(t)
			home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
			credentials := filepath.Join(home.Dir, "data", "credentials.toml")
			before, err := os.ReadFile(credentials)
			if err != nil {
				t.Fatalf("reading credentials.toml: %v", err)
			}

			stdout, stderr, code := runCLI(t, append(args, "--agent")...)
			if code != 9 {
				t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			var env refusalEnvelope
			if err := json.Unmarshal([]byte(stdout), &env); err != nil {
				t.Fatalf("parsing %q: %v", stdout, err)
			}
			if env.Error != sandbox.MsgTokenManaged {
				t.Errorf("error = %q, want %q", env.Error, sandbox.MsgTokenManaged)
			}
			after, err := os.ReadFile(credentials)
			if err != nil {
				t.Fatalf("re-reading credentials.toml: %v", err)
			}
			if string(after) != string(before) {
				t.Errorf("credentials.toml changed:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

// TestSandboxDataSourceIsLive: "auto" falls back to the local store when the
// network fails, which in Canvas Builder would answer a question about the
// course from a copy that may predate the last change. The coercion itself is
// the whole contract, so it is read off the flags rather than inferred from an
// unreachable gateway and a seeded store.
func TestSandboxDataSourceIsLive(t *testing.T) {
	run := func(t *testing.T, opts gatewaytest.WorkspaceOpts) string {
		t.Helper()
		f := gatewaytest.New(t)
		gatewaytest.Workspace(t, f, opts)
		var flags rootFlags
		root := newRootCmd(&flags)
		root.SetArgs([]string{"doctor", "--json", "--data-source", "auto"})
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor: %v", err)
		}
		return flags.dataSource
	}

	if got := run(t, gatewaytest.WorkspaceOpts{}); got != "live" {
		t.Errorf("--data-source auto stayed %q in Canvas Builder, want live", got)
	}
	if got := run(t, gatewaytest.WorkspaceOpts{OmitSandbox: true}); got != "auto" {
		t.Errorf("--data-source auto became %q outside Canvas Builder, want auto", got)
	}
}

// TestDoctorReportsSandbox [Review Focus 7]: doctor is where an agent looks
// when something is wrong, so it has to say which course this workspace is and
// whether the checks are set up — and it must not tell an agent to export a
// token of its own.
func TestDoctorReportsSandbox(t *testing.T) {
	t.Run("in Canvas Builder", func(t *testing.T) {
		f := gatewaytest.New(t)
		home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: []string{"/bin/sh", "-c", "true"}})

		stdout, stderr, code := runCLI(t, "doctor", "--json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		var report struct {
			Sandbox struct {
				Mode      string `json:"mode"`
				Gateway   string `json:"gateway"`
				Workspace string `json:"workspace"`
				CourseID  int64  `json:"course_id"`
				Checks    string `json:"checks"`
			} `json:"sandbox"`
			Auth     string `json:"auth"`
			AuthHint string `json:"auth_hint"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if report.Sandbox.Mode != "on" {
			t.Errorf("sandbox.mode = %q, want on", report.Sandbox.Mode)
		}
		if report.Sandbox.Gateway != f.URL {
			t.Errorf("sandbox.gateway = %q, want %q", report.Sandbox.Gateway, f.URL)
		}
		if report.Sandbox.Workspace != home.Workspace {
			t.Errorf("sandbox.workspace = %q, want %q", report.Sandbox.Workspace, home.Workspace)
		}
		if report.Sandbox.CourseID != 7 {
			t.Errorf("sandbox.course_id = %d, want 7", report.Sandbox.CourseID)
		}
		if report.Sandbox.Checks != "ready" {
			t.Errorf("sandbox.checks = %q, want ready", report.Sandbox.Checks)
		}
		// This workspace has its agent token, so there is nothing to set up:
		// telling the agent to open the course again would be advice to undo
		// a working workspace.
		if report.Auth != "configured" {
			t.Errorf("auth = %q, want configured", report.Auth)
		}
		if report.AuthHint != "" {
			t.Errorf("auth_hint = %q, want none on a workspace that has its token", report.AuthHint)
		}
		if strings.Contains(stdout, "export CANVAS_API_TOKEN") {
			t.Errorf("doctor told an agent to export a token:\n%s", stdout)
		}
	})

	t.Run("the workspace has no token yet", func(t *testing.T) {
		f := gatewaytest.New(t)
		home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: []string{"/bin/sh", "-c", "true"}})
		if err := os.Remove(filepath.Join(home.Dir, "data", "credentials.toml")); err != nil {
			t.Fatalf("removing the workspace's token: %v", err)
		}

		stdout, stderr, code := runCLI(t, "doctor", "--json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		var report struct {
			Auth     string `json:"auth"`
			AuthHint string `json:"auth_hint"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if report.Auth != "not configured" {
			t.Errorf("auth = %q, want not configured", report.Auth)
		}
		// Canvas Builder's own words, not the export-a-token ones (ADR 0001).
		if report.AuthHint != sandbox.MsgTokenSetup {
			t.Errorf("auth_hint = %q, want %q", report.AuthHint, sandbox.MsgTokenSetup)
		}
		if strings.Contains(stdout, "export CANVAS_API_TOKEN") {
			t.Errorf("doctor told an agent to export a token:\n%s", stdout)
		}
	})

	t.Run("checks not set up", func(t *testing.T) {
		f := gatewaytest.New(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

		stdout, _, code := runCLI(t, "doctor", "--json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstdout: %s", code, stdout)
		}
		var report struct {
			Sandbox struct {
				Checks string `json:"checks"`
			} `json:"sandbox"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if report.Sandbox.Checks != "not set up" {
			t.Errorf("sandbox.checks = %q, want \"not set up\"", report.Sandbox.Checks)
		}
	})

	// Without --json, doctor is what the instructor reads, and "which course
	// is this and are the checks set up" is the question they open it for.
	t.Run("without --json", func(t *testing.T) {
		f := gatewaytest.New(t)
		home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: []string{"/bin/sh", "-c", "true"}})

		stdout, stderr, code := runCLI(t, "doctor")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		for _, want := range []string{
			"Canvas Builder: on",
			"gateway: " + f.URL,
			"workspace: " + home.Workspace,
			"course_id: 7",
			"checks: ready",
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("doctor output is missing %q:\n%s", want, stdout)
			}
		}
		if strings.Contains(stdout, "export CANVAS_API_TOKEN") {
			t.Errorf("doctor told an agent to export a token:\n%s", stdout)
		}
		// This workspace has its token, so neither hint belongs here.
		if strings.Contains(stdout, sandbox.MsgTokenSetup) {
			t.Errorf("doctor told the instructor to set up a token they have:\n%s", stdout)
		}
	})

	t.Run("outside Canvas Builder", func(t *testing.T) {
		f := gatewaytest.New(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: true})

		stdout, _, code := runCLI(t, "doctor", "--json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstdout: %s", code, stdout)
		}
		var report struct {
			Sandbox string `json:"sandbox"`
		}
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("parsing %q: %v", stdout, err)
		}
		if report.Sandbox != "off" {
			t.Errorf("sandbox = %q, want off", report.Sandbox)
		}
	})
}
