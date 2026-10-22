// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the upload command: the three requests it makes, the
// course it picks, and the two things it refuses — a file from outside the
// course folder, and an upload address that isn't Canvas Builder's.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// uploadFlow wires the fake the way Canvas's local-storage flow works: step 1
// hands out an upload ticket, the ticket's address answers 302, and
// create_success carries the file.
func uploadFlow(t *testing.T, f *gatewaytest.Fake, uploadURL string) {
	t.Helper()
	f.Handle(http.MethodPost, "/api/v1/courses/7/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upload_url":    uploadURL,
			"upload_params": map[string]string{"Policy": "p", "Signature": "s"},
			"file_param":    "file",
		})
	})
	f.Handle(http.MethodPost, "/files_api", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.URL+"/api/v1/files/31/create_success?uuid=u", http.StatusFound)
	})
	f.Handle(http.MethodGet, "/api/v1/files/31/create_success", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":31,"display_name":"diagram.png","size":1024,` +
			`"content-type":"image/png","folder_id":9,"url":"` + f.URL + `/files/31/download"}`))
	})
}

// material writes n bytes into the course folder and returns the path, which is
// where Canvas Builder tells the agent to put a file it means to upload.
func material(t *testing.T, ws, name string, n int) string {
	t.Helper()
	path := filepath.Join(ws, "materials", name)
	body := make([]byte, n)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestUploadThreeSteps(t *testing.T) {
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api")
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	material(t, home.Workspace, "diagram.png", 1024)

	stdout, stderr, code := runCLI(t, "upload", "materials/diagram.png", "--job", "Add diagram")
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	got := f.Requests()
	want := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/courses/7/files"},
		{http.MethodPost, "/files_api"},
		{http.MethodGet, "/api/v1/files/31/create_success"},
	}
	if len(got) != len(want) {
		t.Fatalf("the fake saw %d requests, want %d: %+v", len(got), len(want), paths(got))
	}
	for i, w := range want {
		if got[i].Method != w.method || got[i].Path != w.path {
			t.Errorf("request %d = %s %s, want %s %s", i+1, got[i].Method, got[i].Path, w.method, w.path)
		}
		if auth := got[i].Header.Get("Authorization"); auth != "Bearer tA" {
			t.Errorf("request %d Authorization = %q, want Bearer tA", i+1, auth)
		}
		if label := got[i].Header.Get("X-Job-Label"); label != "Add diagram" {
			t.Errorf("request %d X-Job-Label = %q, want Add diagram", i+1, label)
		}
	}

	form, err := url.ParseQuery(string(got[0].Body))
	if err != nil {
		t.Fatalf("step 1's body %q isn't a form: %v", got[0].Body, err)
	}
	for key, value := range map[string]string{
		"name":         "diagram.png",
		"size":         "1024",
		"content_type": "image/png",
		"on_duplicate": "rename",
	} {
		if form.Get(key) != value {
			t.Errorf("step 1 sent %s=%q, want %q (body: %s)", key, form.Get(key), value, got[0].Body)
		}
	}
	if ct := got[0].Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		t.Errorf("step 1 Content-Type = %q, want a form (a JSON body is at Rails' mercy)", ct)
	}
	if got[2].RawQuery != "uuid=u" {
		t.Errorf("step 3 query = %q, want uuid=u", got[2].RawQuery)
	}

	var out struct {
		Action   string `json:"action"`
		CourseID int64  `json:"course_id"`
		File     struct {
			ID          int64  `json:"id"`
			DisplayName string `json:"display_name"`
			Size        int64  `json:"size"`
			ContentType string `json:"content_type"`
			FolderID    int64  `json:"folder_id"`
		} `json:"file"`
		EmbedSrc string `json:"embed_src"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("output isn't the upload envelope: %v\n%s", err, stdout)
	}
	if out.Action != "upload" || out.CourseID != 7 || out.File.ID != 31 || out.File.FolderID != 9 {
		t.Errorf("output = %+v, want action upload, course 7, file 31 in folder 9", out)
	}
	if out.File.ContentType != "image/png" || out.File.Size != 1024 || out.File.DisplayName != "diagram.png" {
		t.Errorf("file = %+v, want diagram.png, 1024 bytes, image/png", out.File)
	}
	if out.EmbedSrc != "/courses/7/files/31/preview" {
		t.Errorf("embed_src = %q, want /courses/7/files/31/preview", out.EmbedSrc)
	}
}

// Review Focus 3: the ticket's address decides where the file and the token go,
// and Canvas is the one that fills it in.
func TestUploadRefusesUploadURLOffGateway(t *testing.T) {
	f := gatewaytest.New(t)
	var seen atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		seen.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(other.Close)
	uploadFlow(t, f, other.URL+"/files_api")
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	material(t, home.Workspace, "diagram.png", 1024)

	stdout, stderr, code := runCLI(t, "upload", "materials/diagram.png")
	if code != 9 {
		t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr+stdout, "only talks to Canvas Builder at") {
		t.Errorf("neither stream carries the refusal\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if n := seen.Load(); n != 0 {
		t.Errorf("the other server saw %d requests, want 0", n)
	}
}

// Review Focus 2: the file went out and the answer was lost, so the agent has
// to look at the course before it tries again.
func TestUploadLostAnswerIsUnknown(t *testing.T) {
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api")
	f.Handle(http.MethodPost, "/files_api", func(w http.ResponseWriter, _ *http.Request) {
		gatewaytest.Drop(w)
	})
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	material(t, home.Workspace, "diagram.png", 1024)

	stdout, stderr, code := runCLI(t, "upload", "materials/diagram.png")
	if code != 8 {
		t.Fatalf("exit = %d, want 8\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	counts := map[string]int{}
	for _, rec := range f.Requests() {
		counts[rec.Method+" "+rec.Path]++
	}
	if counts["POST /api/v1/courses/7/files"] != 1 || counts["POST /files_api"] != 1 {
		t.Fatalf("the fake saw %v, want one of each: a lost upload must not be retried", counts)
	}
}

// Review Focus 5: the course folder is the boundary. A path outside it, however
// it is spelled, is not this course's material.
func TestUploadOutsideWorkspaceRefused(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "elsewhere.png")
	if err := os.WriteFile(outside, []byte("not this course's"), 0o600); err != nil {
		t.Fatalf("writing the outside file: %v", err)
	}

	for _, tc := range []struct {
		name string
		arg  func(t *testing.T, ws string) string
	}{
		{
			name: "a path in another folder",
			arg:  func(_ *testing.T, _ string) string { return outside },
		},
		{
			name: "a link in materials that points out",
			arg: func(t *testing.T, ws string) string {
				link := filepath.Join(ws, "materials", "link.png")
				if err := os.Symlink(outside, link); err != nil {
					t.Fatalf("linking: %v", err)
				}
				return "materials/link.png"
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := gatewaytest.New(t)
			uploadFlow(t, f, f.URL+"/files_api")
			home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})

			stdout, stderr, code := runCLI(t, "upload", tc.arg(t, home.Workspace))
			if code != 9 {
				t.Fatalf("exit = %d, want 9\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stderr+stdout, sandbox.MsgOutsideWorkspace) {
				t.Errorf("neither stream carries the refusal\nstdout: %s\nstderr: %s", stdout, stderr)
			}
			if n := len(f.Requests()); n != 0 {
				t.Errorf("the fake saw %d requests, want 0: nothing should be sent", n)
			}
		})
	}
}

// Outside Canvas Builder there is no workspace to read the course from, so the
// command asks for one rather than guessing.
func TestUploadNormalModeNeedsCourse(t *testing.T) {
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api")
	home := gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{OmitSandbox: true})
	path := material(t, home.Workspace, "diagram.png", 1024)

	stdout, stderr, code := runCLI(t, "upload", path)
	if code != 2 {
		t.Fatalf("exit = %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if n := len(f.Requests()); n != 0 {
		t.Errorf("the fake saw %d requests, want 0", n)
	}
}

func paths(got []gatewaytest.Recorded) []string {
	out := make([]string, 0, len(got))
	for _, rec := range got {
		out = append(out, rec.Method+" "+rec.Path)
	}
	return out
}
