// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the check command: what reaches cb-checks, what the
// agent gets back, and the cases where the checks never run at all.
//
// The checks themselves are tested in canvas-builder-v2. What matters here is
// the handover: a snapshot of the course as it is right now on stdin, the
// course's own closure days, and the report passed back without a word
// changed.

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// helperReport is what the stand-in prints when a test doesn't say otherwise:
// one check's report the way cb-checks writes it, trailing newline and all.
const helperReport = `{"check":"dates","checked_items":1,"issues":[]}` + "\n"

// TestHelperChecks stands in for cb-checks. It is not one of this package's
// tests: the check command is handed this test binary as its checks_command and
// runs it as a child process, so what this records is exactly what the real
// cb-checks would have been given. It answers from the environment:
// CB_CHECKS_RECORD is where to write its argv and stdin, CB_CHECKS_STDOUT and
// CB_CHECKS_STDERR are what to say, CB_CHECKS_EXIT is what to exit with, and
// CB_CHECKS_SLEEP is how long to take about it.
//
// It ends with os.Exit so the testing framework never gets to print its own
// PASS line, which would land in the middle of the report.
func TestHelperChecks(t *testing.T) {
	if os.Getenv("CB_CHECKS_HELPER") != "1" {
		t.Skip("the stand-in for cb-checks, which the check tests run as a child process")
	}
	args := os.Args
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "the cb-checks stand-in couldn't read its stdin:", err)
		os.Exit(1)
	}
	if raw := os.Getenv("CB_CHECKS_SLEEP"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "CB_CHECKS_SLEEP isn't a length of time:", err)
			os.Exit(1)
		}
		time.Sleep(d)
	}
	if record := os.Getenv("CB_CHECKS_RECORD"); record != "" {
		body, err := json.Marshal(map[string]any{"argv": args, "stdin": string(stdin)})
		if err != nil {
			fmt.Fprintln(os.Stderr, "the cb-checks stand-in couldn't write its record:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(record, body, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "the cb-checks stand-in couldn't write its record:", err)
			os.Exit(1)
		}
	}
	fmt.Fprint(os.Stderr, os.Getenv("CB_CHECKS_STDERR"))
	if out, set := os.LookupEnv("CB_CHECKS_STDOUT"); set {
		fmt.Print(out)
	} else {
		fmt.Print(helperReport)
	}
	code := 0
	if raw := os.Getenv("CB_CHECKS_EXIT"); raw != "" {
		if code, err = strconv.Atoi(raw); err != nil {
			fmt.Fprintln(os.Stderr, "CB_CHECKS_EXIT isn't a number:", err)
			os.Exit(1)
		}
	}
	os.Exit(code)
}

// helperRun is one run of the stand-in, as it recorded itself.
type helperRun struct {
	Argv  []string `json:"argv"`
	Stdin string   `json:"stdin"`
}

// helperEnv turns this test binary into the cb-checks stand-in and returns
// where the run will be recorded.
func helperEnv(t *testing.T) string {
	t.Helper()
	record := filepath.Join(t.TempDir(), "cb-checks.json")
	t.Setenv("CB_CHECKS_HELPER", "1")
	t.Setenv("CB_CHECKS_RECORD", record)
	return record
}

// checksCommand is the stand-in written the way a workspace's checks_command
// is: the command, then the arguments the check command adds to it.
func checksCommand() []string {
	return []string{os.Args[0], "-test.run=^TestHelperChecks$", "--"}
}

func readRun(t *testing.T, record string) helperRun {
	t.Helper()
	body, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the checks left no record at %s, so they never ran: %v", record, err)
	}
	var got helperRun
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("the record the checks left isn't JSON: %v\n%s", err, body)
	}
	return got
}

func neverRan(t *testing.T, record string) {
	t.Helper()
	if _, err := os.Stat(record); err == nil {
		t.Error("the checks ran, and the command should have stopped before running them")
	}
}

// checkCourse answers the reads the snapshot makes, with one page whose body
// the caller chooses: a page body is where a placeholder or a bad link would
// be.
func checkCourse(f *gatewaytest.Fake, pageBody string) {
	f.Handle(http.MethodGet, "/api/v1/courses/7", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"id":7,"name":"Intro to Canvas","start_at":"2027-04-12T07:00:00Z",`+
			`"end_at":"2027-06-12T06:59:00Z","time_zone":"America/Los_Angeles","syllabus_body":"<p>Read the book.</p>"}`)
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"url":"week-1","title":"Week 1","body":%q,"published":true}]`, pageBody)
	})
	// One handler each for the rest, including discussion_topics, which the
	// snapshot reads twice: once for topics and once for announcements.
	for _, name := range []string{"assignments", "quizzes", "discussion_topics", "files"} {
		f.Handle(http.MethodGet, "/api/v1/courses/7/"+name, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `[]`)
		})
	}
}

func TestCheckPassesSnapshotAndClosures(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
		ChecksCommand: checksCommand(),
		ClosureDays:   []string{"2027-05-06", "2027-05-31"},
	})

	stdout, stderr, code := runCLI(t, "check", "dates")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Byte for byte: the report is cb-checks' answer, and anything this CLI
	// did to it on the way past would be a second opinion the agent can't see.
	if stdout != helperReport {
		t.Errorf("stdout = %q, want the report unchanged: %q", stdout, helperReport)
	}

	got := readRun(t, record)
	want := []string{"dates", "--snapshot", "-", "--closures", "2027-05-06,2027-05-31", "--local-host", f.HostPort()}
	if !slices.Equal(got.Argv, want) {
		t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
	}

	var snapshot struct {
		Course struct {
			ID int64 `json:"id"`
		} `json:"course"`
		Pages []struct {
			Body string `json:"body"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(got.Stdin), &snapshot); err != nil {
		t.Fatalf("the snapshot on the checks' stdin isn't JSON: %v\nstdin: %s", err, got.Stdin)
	}
	if snapshot.Course.ID != 7 {
		t.Errorf("the snapshot was of course %d, want the workspace's course 7", snapshot.Course.ID)
	}
	if len(snapshot.Pages) != 1 || !strings.Contains(snapshot.Pages[0].Body, "Week 1") {
		t.Errorf("the snapshot didn't carry the course's page:\n%s", got.Stdin)
	}
}

func TestCheckAllPassesThrough(t *testing.T) {
	const report = `{"reports":[{"check":"a11y","checked_items":2,"issues":[]},` +
		`{"check":"dates","checked_items":2,"issues":[],"schedule":[]}]}` + "\n"

	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	t.Setenv("CB_CHECKS_STDOUT", report)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "all")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if stdout != report {
		t.Errorf("stdout = %q, want the four reports unchanged: %q", stdout, report)
	}
	if got := readRun(t, record); len(got.Argv) == 0 || got.Argv[0] != "all" {
		t.Errorf("the checks ran as %q, want all of them", got.Argv)
	}
}

// TestCheckClosureDays covers the three answers a course workspace can give
// about its closure days. Only one of them is "none"; the other two are a list
// and nothing said, and nothing said is passed on as nothing said rather than
// being turned into none or into a refusal (JR-59).
func TestCheckClosureDays(t *testing.T) {
	t.Run("the instructor hasn't entered them", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand:   checksCommand(),
			OmitClosureDays: true,
		})

		// The date check still runs: --closures is left off altogether, and
		// cb-checks reports the one rule it can't apply as not checked while
		// checking the rest of the dates. Refusing the whole run over a list
		// the agent can't supply only taught agents to skip the check.
		stdout, stderr, code := runCLI(t, "check", "dates")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"dates", "--snapshot", "-", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})

	t.Run("a check that doesn't use them", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand:   checksCommand(),
			OmitClosureDays: true,
		})

		// Closure days are the date check's business. A course whose
		// instructor hasn't entered them yet can still be checked for
		// accessibility, and that check is handed no closure days either way.
		stdout, stderr, code := runCLI(t, "check", "a11y")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"a11y", "--snapshot", "-", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})

	t.Run("all four with none entered", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand:   checksCommand(),
			OmitClosureDays: true,
		})

		// check all is the one an agent reaches for first, so it is the one
		// that must not be held up by a list nobody has entered.
		stdout, stderr, code := runCLI(t, "check", "all")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"all", "--snapshot", "-", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})

	t.Run("the course has none", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand: checksCommand(),
			ClosureDays:   []string{},
		})

		// An empty list is an answer: this course closes on no days.
		stdout, stderr, code := runCLI(t, "check", "dates")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"dates", "--snapshot", "-", "--closures", "", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})
}

func TestCheckRefusesClosuresFlagInSandbox(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "dates", "--closures", "2027-05-06")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, sandbox.MsgClosuresFromWorkspace) {
		t.Errorf("stderr doesn't say where closure days come from:\n%s", stderr)
	}
	neverRan(t, record)
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("the command asked Canvas for %v before refusing the flag", paths(got))
	}
}

// A --course that names a different course used to be accepted and then
// ignored, which reported the workspace's course as clean under another
// course's number.
func TestCheckRefusesAnotherCourseInSandbox(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "dates", "--course", "99")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, sandbox.MsgCourseFromWorkspace) {
		t.Errorf("stderr doesn't say where the course comes from:\n%s", stderr)
	}
	neverRan(t, record)
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("the command asked Canvas for %v before refusing the flag", paths(got))
	}
}

// The workspace's own course number is still accepted: saying the same thing
// twice is not a mistake.
func TestCheckAcceptsTheWorkspaceCourseInSandbox(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "dates", "--course", "7")
	if code != 0 {
		t.Fatalf("exit %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := readRun(t, record); len(got.Argv) == 0 {
		t.Error("the checks never ran")
	}
}

func TestCheckMissingHelper(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "check", "dates")
	if code != 10 {
		t.Fatalf("exit %d, want 10\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, sandbox.MsgChecksMissing) {
		t.Errorf("stderr doesn't say the checks aren't set up:\n%s", stderr)
	}
	// Nothing can be done with a snapshot nothing will read, so the course is
	// left alone.
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("the command asked Canvas for %v with no checks to run", paths(got))
	}
}

func TestCheckHelperFailure(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		stderr string
		exit   string
		want   string
	}{
		{
			name:   "a non-zero exit",
			stdout: "",
			stderr: "Unknown check \"x\".\nand a second line nobody needs\n",
			exit:   "2",
			want:   "The checks couldn't run: Unknown check \"x\".",
		},
		{
			name:   "stdout that isn't a report",
			stdout: "node:internal/modules/run_main\n",
			stderr: "SyntaxError: Unexpected token\nat line 4\n",
			exit:   "",
			want:   "The checks couldn't run: SyntaxError: Unexpected token",
		},
		{
			name:   "nothing at all",
			stdout: "",
			stderr: "",
			exit:   "1",
			want:   "The checks couldn't run: they produced no report.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := gatewaytest.New(t)
			checkCourse(f, "<p>Week 1</p>")
			helperEnv(t)
			t.Setenv("CB_CHECKS_STDOUT", tc.stdout)
			t.Setenv("CB_CHECKS_STDERR", tc.stderr)
			t.Setenv("CB_CHECKS_EXIT", tc.exit)
			gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

			stdout, stderr, code := runCLI(t, "check", "dates")
			if code != 1 {
				t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr is\n%s\nwant it to carry %q", stderr, tc.want)
			}
			if strings.Contains(stderr, "second line") || strings.Contains(stderr, "at line 4") {
				t.Errorf("stderr carries more of the checks' output than its first line:\n%s", stderr)
			}
			// Half a report is worse than none: the agent would read it as the
			// whole answer.
			if stdout != "" {
				t.Errorf("stdout is %q, want nothing when there is no report", stdout)
			}
		})
	}
}

func TestCheckStopsALongRun(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	t.Setenv("CB_CHECKS_SLEEP", "30s")
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	// The real limit is ten minutes, which no test should wait for.
	before := checksTimeLimit
	checksTimeLimit = 50 * time.Millisecond
	t.Cleanup(func() { checksTimeLimit = before })

	stdout, stderr, code := runCLI(t, "check", "dates")
	if code != 1 {
		t.Fatalf("exit %d, want 1\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, sandbox.MsgChecksTimeout) {
		t.Errorf("stderr doesn't say the checks were stopped:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout is %q, want nothing when the checks were stopped", stdout)
	}
	neverRan(t, record)
}

func TestCheckUnknownName(t *testing.T) {
	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	record := helperEnv(t)
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "spelling")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "a11y") || !strings.Contains(stderr, "spelling") {
		t.Errorf("stderr doesn't name what was asked for and what there is:\n%s", stderr)
	}
	neverRan(t, record)
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("the command asked Canvas for %v before reading the check's name", paths(got))
	}
}

func TestCheckGatewayRefusalSurfaced(t *testing.T) {
	f := gatewaytest.New(t)
	record := helperEnv(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7", func(w http.ResponseWriter, _ *http.Request) {
		gatewaytest.Refuse(w, http.StatusLocked, sandbox.MsgQuarantine)
	})
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: checksCommand()})

	stdout, stderr, code := runCLI(t, "check", "dates")
	if code != 9 {
		t.Fatalf("exit %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, sandbox.MsgQuarantine) {
		t.Errorf("stderr doesn't carry Canvas Builder's own words:\n%s", stderr)
	}
	// A report on a course that couldn't be read would be a report on nothing.
	neverRan(t, record)
}

func TestCheckOutsideCanvasBuilder(t *testing.T) {
	t.Run("the course and the closure days are the caller's", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand: checksCommand(),
			OmitSandbox:   true,
		})

		stdout, stderr, code := runCLI(t, "check", "dates", "--course", "7", "--closures", "2027-05-06")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"dates", "--snapshot", "-", "--closures", "2027-05-06", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})

	t.Run("no closure days given", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand: checksCommand(),
			OmitSandbox:   true,
		})

		// Outside Canvas Builder nobody has said either, and the flag being
		// left off says so: --closures with nothing after it would claim this
		// course closes on no days.
		stdout, stderr, code := runCLI(t, "check", "dates", "--course", "7")
		if code != 0 {
			t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		got := readRun(t, record)
		want := []string{"dates", "--snapshot", "-", "--local-host", f.HostPort()}
		if !slices.Equal(got.Argv, want) {
			t.Errorf("the checks ran as %q,\nwant %q", got.Argv, want)
		}
	})

	t.Run("no course to check", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		record := helperEnv(t)
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
			ChecksCommand: checksCommand(),
			OmitSandbox:   true,
		})

		stdout, stderr, code := runCLI(t, "check", "dates")
		if code != 2 {
			t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "--course") {
			t.Errorf("stderr doesn't say which course is missing:\n%s", stderr)
		}
		neverRan(t, record)
	})

	t.Run("no checks to run", func(t *testing.T) {
		f := gatewaytest.New(t)
		checkCourse(f, "<p>Week 1</p>")
		gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: true})

		stdout, stderr, code := runCLI(t, "check", "dates", "--course", "7")
		if code != 10 {
			t.Fatalf("exit %d, want 10\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "checks_command") {
			t.Errorf("stderr doesn't say what to set:\n%s", stderr)
		}
	})
}

// TestCheckRealHelper runs the command against the real cb-checks, so the two
// halves of the handover are tested together at least once: the snapshot this
// CLI builds, read by the checks that will read it in Canvas Builder.
func TestCheckRealHelper(t *testing.T) {
	repo := os.Getenv("CANVAS_BUILDER_V2")
	if repo == "" {
		t.Skip("set CANVAS_BUILDER_V2 to the canvas-builder-v2 repo to run the real checks")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't on PATH, so cb-checks can't run")
	}
	checks := filepath.Join(repo, "checks", "src", "cli.ts")
	if _, err := os.Stat(checks); err != nil {
		t.Skipf("no cb-checks at %s", checks)
	}

	f := gatewaytest.New(t)
	checkCourse(f, "<p>We meet in [Check: room number].</p>")
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{ChecksCommand: []string{node, checks}})

	stdout, stderr, code := runCLI(t, "check", "placeholders")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var report struct {
		Check  string `json:"check"`
		Issues []struct {
			Rule    string `json:"rule"`
			Message string `json:"message"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("what cb-checks reported isn't JSON: %v\nstdout: %s", err, stdout)
	}
	if report.Check != "placeholders" {
		t.Errorf("the report is for %q, want placeholders", report.Check)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("cb-checks found %d placeholders, want the one in the page: %s", len(report.Issues), stdout)
	}
}

// TestCheckRealHelperWithoutClosureDays is the other half of JR-59, tested
// across both repos at once: this CLI leaves --closures off when nobody has
// entered the days, and cb-checks reads that as not having been told rather
// than as a course that closes on no days. Either side alone would look
// right; only together do they show the date check still running and saying
// which part of it couldn't be applied.
func TestCheckRealHelperWithoutClosureDays(t *testing.T) {
	repo := os.Getenv("CANVAS_BUILDER_V2")
	if repo == "" {
		t.Skip("set CANVAS_BUILDER_V2 to the canvas-builder-v2 repo to run the real checks")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't on PATH, so cb-checks can't run")
	}
	checks := filepath.Join(repo, "checks", "src", "cli.ts")
	if _, err := os.Stat(checks); err != nil {
		t.Skipf("no cb-checks at %s", checks)
	}

	f := gatewaytest.New(t)
	checkCourse(f, "<p>Week 1</p>")
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{
		ChecksCommand:   []string{node, checks},
		OmitClosureDays: true,
	})

	stdout, stderr, code := runCLI(t, "check", "dates")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var report struct {
		Check  string `json:"check"`
		Issues []struct {
			Rule string `json:"rule"`
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
			Message string `json:"message"`
		} `json:"issues"`
		Schedule []struct{} `json:"schedule"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("what cb-checks reported isn't JSON: %v\nstdout: %s", err, stdout)
	}
	if report.Check != "dates" {
		t.Errorf("the report is for %q, want dates", report.Check)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("cb-checks reported %d issues, want the one not-checked entry: %s", len(report.Issues), stdout)
	}
	entry := report.Issues[0]
	if entry.Rule != "not-checked" || entry.Item.Type != "course" {
		t.Errorf("the entry is %q about %q, want not-checked about the course: %s", entry.Rule, entry.Item.Type, stdout)
	}
	if !strings.Contains(entry.Message, "closure days") {
		t.Errorf("the entry doesn't say what wasn't checked:\n%s", entry.Message)
	}
}
