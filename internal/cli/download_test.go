// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the download command: the two requests it makes,
// where it is allowed to write, and what it leaves behind when a download goes
// wrong — which is nothing.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// downloadFlow registers the file's record and its bytes, with the display
// name the record carries left to the caller: a name is one of the things a
// course file can lie about.
func downloadFlow(t *testing.T, f *gatewaytest.Fake, displayName string, body []byte) {
	t.Helper()
	f.Handle(http.MethodGet, "/api/v1/courses/7/files/31", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id":31,"display_name":%q,"size":%d,"content-type":"image/png","folder_id":9,"url":%q}`,
			displayName, len(body), f.URL+"/files/31/download?download_frd=1&verifier=v")
	})
	f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})
}

func TestDownloadSavesFile(t *testing.T) {
	f := gatewaytest.New(t)
	want := bytes.Repeat([]byte("canvas"), 400)
	downloadFlow(t, f, "diagram.png", want)
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials/diagram.png")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	saved := filepath.Join(home.Workspace, "materials", "diagram.png")
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the saved file isn't the one Canvas sent (%d bytes, want %d)", len(got), len(want))
	}

	gotReq := f.Requests()
	if names := paths(gotReq); len(names) != 2 ||
		names[0] != "GET /api/v1/courses/7/files/31" || names[1] != "GET /files/31/download" {
		t.Fatalf("the requests were %v", names)
	}
	for _, rec := range gotReq {
		if auth := rec.Header.Get("Authorization"); auth != "Bearer tA" {
			t.Errorf("%s %s carried Authorization %q", rec.Method, rec.Path, auth)
		}
		// A token in a query string is a token in the gateway's logs and in
		// Canvas's. The header is the only place it goes.
		if strings.Contains(rec.RawQuery, "access_token") {
			t.Errorf("%s %s put a token in its query: %s", rec.Method, rec.Path, rec.RawQuery)
		}
	}
	if q := gotReq[1].RawQuery; q != "download_frd=1&verifier=v" {
		t.Errorf("the download query was %q, so the CLI rewrote the address Canvas gave it", q)
	}

	var envelope struct {
		Action string `json:"action"`
		File   struct {
			ID          int64  `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"file"`
		SavedTo string `json:"saved_to"`
		Bytes   int64  `json:"bytes"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("reading the result: %v\n%s", err, stdout)
	}
	if envelope.Action != "download" || envelope.File.ID != 31 || envelope.File.DisplayName != "diagram.png" {
		t.Errorf("the result said: %+v", envelope)
	}
	// saved_to is the path the command actually wrote to, which is the
	// workspace path with its links resolved: that is the path the rule about
	// staying inside the course folder was checked against.
	wantSaved, err := filepath.EvalSymlinks(saved)
	if err != nil {
		t.Fatalf("resolving the saved path: %v", err)
	}
	if envelope.SavedTo != wantSaved {
		t.Errorf("saved_to is %q, want %q", envelope.SavedTo, wantSaved)
	}
	if envelope.Bytes != int64(len(want)) {
		t.Errorf("bytes is %d, want %d", envelope.Bytes, len(want))
	}
}

// TestDownloadShortBodyLeavesNothing is the whole reason the command writes to
// a temporary name first. Half a file on disk under the right name is worse
// than no file: the next check reads it and reports what isn't there.
func TestDownloadShortBodyLeavesNothing(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, "diagram.png", bytes.Repeat([]byte("z"), 2048))
	f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
		gatewaytest.Truncated(w, 2048, bytes.Repeat([]byte("z"), 1024))
	})
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials/diagram.png")
	if code == 0 {
		t.Fatalf("half a file was reported as saved\nstdout: %s", stdout)
	}
	if !strings.Contains(stdout+stderr, sandbox.MsgDownloadShort) {
		t.Errorf("the message didn't say the download stopped short:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	left, err := os.ReadDir(filepath.Join(home.Workspace, "materials"))
	if err != nil {
		t.Fatalf("reading the materials folder: %v", err)
	}
	if len(left) != 0 {
		names := make([]string, 0, len(left))
		for _, entry := range left {
			names = append(names, entry.Name())
		}
		t.Errorf("the folder still holds %v", names)
	}
}

func TestDownloadWontOverwrite(t *testing.T) {
	f := gatewaytest.New(t)
	want := bytes.Repeat([]byte("new"), 100)
	downloadFlow(t, f, "diagram.png", want)
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	target := filepath.Join(home.Workspace, "materials", "diagram.png")
	if err := os.WriteFile(target, []byte("the instructor's own copy"), 0o600); err != nil {
		t.Fatalf("writing the file that is already there: %v", err)
	}

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials/diagram.png")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "the instructor's own copy" {
		t.Fatalf("the file that was already there is now %q (%v)", got, err)
	}

	stdout, stderr, code = runCLI(t, "download", "31", "--output", "materials/diagram.png", "--force")
	if code != 0 {
		t.Fatalf("--force exited %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got, _ := os.ReadFile(target); !bytes.Equal(got, want) {
		t.Error("--force didn't replace the file")
	}
}

func TestDownloadOutsideWorkspaceRefused(t *testing.T) {
	cases := map[string]func(t *testing.T, home gatewaytest.Home) string{
		"a folder outside the course": func(t *testing.T, home gatewaytest.Home) string {
			return filepath.Join(t.TempDir(), "diagram.png")
		},
		"a link in the course folder pointing out": func(t *testing.T, home gatewaytest.Home) string {
			outside := t.TempDir()
			link := filepath.Join(home.Workspace, "materials", "elsewhere")
			if err := os.Symlink(outside, link); err != nil {
				t.Fatalf("making the link: %v", err)
			}
			return "materials/elsewhere/diagram.png"
		},
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			f := gatewaytest.New(t)
			downloadFlow(t, f, "diagram.png", []byte("bytes"))
			home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, "download", "31", "--output", target(t, home))
			if code != 9 {
				t.Fatalf("exit %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr, sandbox.MsgOutsideWorkspace) {
				t.Errorf("the message didn't name the workspace rule:\nstdout: %s\nstderr: %s", stdout, stderr)
			}
			if got := f.Requests(); len(got) != 0 {
				t.Errorf("it asked Canvas anyway: %v", paths(got))
			}
		})
	}
}

// TestDownloadDisplayNameCantEscape: a file's display name comes from Canvas,
// which an agent can write to. With --output naming a folder, the name decides
// the file, so it is a base name and nothing else.
func TestDownloadDisplayNameCantEscape(t *testing.T) {
	f := gatewaytest.New(t)
	want := []byte("not a script")
	downloadFlow(t, f, "../../evil.sh", want)
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	saved := filepath.Join(home.Workspace, "materials", "evil.sh")
	got, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("the file wasn't saved where the folder said: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("the saved bytes aren't the file")
	}
	if _, err := os.Stat(filepath.Join(home.Dir, "evil.sh")); err == nil {
		t.Fatal("the display name climbed out of the folder")
	}
}

// TestDownloadRefusesANameThatIsntOne covers the display names that leave
// nothing to save under: a folder's own name is not a file name.
func TestDownloadRefusesANameThatIsntOne(t *testing.T) {
	for _, displayName := range []string{"", ".", "..", "/"} {
		t.Run(fmt.Sprintf("display_name %q", displayName), func(t *testing.T) {
			f := gatewaytest.New(t)
			downloadFlow(t, f, displayName, []byte("bytes"))
			home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials")
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			left, err := os.ReadDir(filepath.Join(home.Workspace, "materials"))
			if err != nil {
				t.Fatalf("reading the materials folder: %v", err)
			}
			if len(left) != 0 {
				t.Errorf("%d files were saved anyway", len(left))
			}
		})
	}
}

func TestDownloadNormalModeNeedsCourse(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, "diagram.png", []byte("bytes"))
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: true})

	dir := t.TempDir()
	stdout, stderr, code := runCLI(t, "download", "31", "--output", filepath.Join(dir, "diagram.png"))
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("it asked Canvas without being told which course: %v", paths(got))
	}
}

// TestDownloadWontInventAFolder: a folder that isn't there is a typo in a
// path, and the course workspace is the instructor's folder. Saving the file as
// the typo would be worse, and creating the folder would leave it behind.
func TestDownloadWontInventAFolder(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, "diagram.png", []byte("bytes"))
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials/week3/")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("it asked Canvas for a file it had nowhere to put: %v", paths(got))
	}
	left, err := os.ReadDir(filepath.Join(home.Workspace, "materials"))
	if err != nil {
		t.Fatalf("reading the materials folder: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("the folder now holds %d entries", len(left))
	}
}

// TestDownloadDryRunSavesNothing: --dry-run shows the first of the two requests
// and stops. The second can't be shown — its address is in the answer to the
// first — and nothing is written, not even a half-made temporary file.
func TestDownloadDryRunSavesNothing(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, "diagram.png", []byte("bytes"))
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

	stdout, stderr, code := runCLI(t, "download", "31", "--output", "materials/diagram.png", "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var envelope struct {
		Action string `json:"action"`
		DryRun bool   `json:"dry_run"`
		FileID int64  `json:"file_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("reading the result: %v\n%s", err, stdout)
	}
	if envelope.Action != "download" || !envelope.DryRun || envelope.FileID != 31 {
		t.Errorf("the result said: %+v", envelope)
	}
	if got := f.Requests(); len(got) != 0 {
		t.Errorf("a dry run sent %v", paths(got))
	}
	left, err := os.ReadDir(filepath.Join(home.Workspace, "materials"))
	if err != nil {
		t.Fatalf("reading the materials folder: %v", err)
	}
	if len(left) != 0 {
		names := make([]string, 0, len(left))
		for _, entry := range left {
			names = append(names, entry.Name())
		}
		t.Errorf("a dry run left %v behind", names)
	}
}
