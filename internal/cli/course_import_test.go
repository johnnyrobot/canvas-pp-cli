// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCanvas plays the Canvas API side of a file-backed content migration.
// The upload target is a real httptest server so the multipart request and
// redirect handling are exercised end to end.
type fakeCanvas struct {
	mu sync.Mutex

	base      string
	uploadSrv *httptest.Server

	existing     []cmMigration // returned by the list call
	state        string        // current migration API state
	tokens       int           // upload tokens issued
	uploadStatus []int         // response per upload attempt; default 302
	confirmed    bool
	stateAfter   []string // states reported after confirm, one per poll; last repeats
	polls        int

	puts         []map[string]any
	uploadFields []map[string]string
	uploadBodies [][]byte
	confirmPath  string
	confirmQuery map[string]string
}

func newFakeCanvas(t *testing.T) *fakeCanvas {
	f := &fakeCanvas{base: "https://canvas.test", state: "pre_processing"}
	f.uploadSrv = httptest.NewServer(http.HandlerFunc(f.handleUpload))
	t.Cleanup(f.uploadSrv.Close)
	return f
}

func (f *fakeCanvas) RequestBaseURL() string { return f.base }

func (f *fakeCanvas) preAttachment() map[string]any {
	f.tokens++
	return map[string]any{
		"upload_url":    f.uploadSrv.URL + "/files_api",
		"upload_params": map[string]any{"token": fmt.Sprintf("tok-%d", f.tokens), "filename": "pkg.imscc", "ignored": nil},
		"file_param":    "attachment[uploaded_data]",
	}
}

func (f *fakeCanvas) handleUpload(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.ContentLength <= 0 {
		http.Error(w, "length required", http.StatusLengthRequired)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fields := map[string]string{}
	for k, v := range r.MultipartForm.Value {
		fields[k] = v[0]
	}
	f.uploadFields = append(f.uploadFields, fields)
	file, _, err := r.FormFile("attachment[uploaded_data]")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(file)
	f.uploadBodies = append(f.uploadBodies, data)

	status := http.StatusFound
	if i := len(f.uploadBodies) - 1; i < len(f.uploadStatus) {
		status = f.uploadStatus[i]
	}
	if status == http.StatusFound {
		w.Header().Set("Location", f.base+"/api/v1/files/77/create_success?uuid=u-"+fields["token"])
	}
	w.WriteHeader(status)
}

func (f *fakeCanvas) GetNoCache(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case path == "/api/v1/courses/259/content_migrations":
		return json.Marshal(f.existing)
	case strings.HasPrefix(path, "/api/v1/files/") && strings.HasSuffix(path, "/create_success"):
		f.confirmed = true
		f.confirmPath = path
		f.confirmQuery = params
		f.state = "queued"
		return json.RawMessage(`{"id":77,"display_name":"pkg.imscc"}`), nil
	case path == "/api/v1/courses/259/content_migrations/500":
		if f.confirmed && len(f.stateAfter) > 0 {
			i := f.polls
			if i >= len(f.stateAfter) {
				i = len(f.stateAfter) - 1
			}
			f.state = f.stateAfter[i]
			f.polls++
		}
		return json.Marshal(map[string]any{
			"id": 500, "workflow_state": f.state, "migration_issues_count": 1,
			"progress_url": f.base + "/api/v1/progress/9",
		})
	}
	return nil, fmt.Errorf("unexpected GET %s", path)
}

func (f *fakeCanvas) Post(_ context.Context, path string, body any) (json.RawMessage, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if path != "/api/v1/courses/259/content_migrations" {
		return nil, 0, fmt.Errorf("unexpected POST %s", path)
	}
	b := body.(map[string]any)
	if b["migration_type"] == nil || b["pre_attachment[name]"] == nil || b["pre_attachment[size]"] == nil {
		return nil, 0, fmt.Errorf("create missing fields: %v", b)
	}
	data, _ := json.Marshal(map[string]any{"id": 500, "workflow_state": "pre_processing", "pre_attachment": f.preAttachment()})
	return data, 200, nil
}

func (f *fakeCanvas) Put(_ context.Context, path string, body any) (json.RawMessage, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if path != "/api/v1/courses/259/content_migrations/500" {
		return nil, 0, fmt.Errorf("unexpected PUT %s", path)
	}
	b := body.(map[string]any)
	f.puts = append(f.puts, b)
	if size, _ := b["pre_attachment[size]"].(int64); size == releaseSize {
		f.state = "failed"
		data, _ := json.Marshal(map[string]any{"id": 500, "workflow_state": "failed",
			"pre_attachment": map[string]any{"error": true, "message": "file size exceeds quota"}})
		return data, 200, nil
	}
	data, _ := json.Marshal(map[string]any{"id": 500, "workflow_state": "pre_processing", "pre_attachment": f.preAttachment()})
	return data, 200, nil
}

func writeTestPackage(t *testing.T, files ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.imscc")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, name := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "contents of %s", name)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
	return path
}

func newTestImporter(f *fakeCanvas, file string) *courseImporter {
	return &courseImporter{
		api:           f,
		upload:        &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		log:           io.Discard,
		courseID:      "259",
		filePath:      file,
		migrationType: "canvas_cartridge_importer",
		wait:          true,
		waitTimeout:   time.Second,
		pollInterval:  time.Millisecond,
		startTimeout:  50 * time.Millisecond,
		maxAttempts:   3,
		now:           time.Now,
	}
}

// Regression: the 2026-11-07 orphan happened because the redirect from the
// upload was never followed, so Canvas never attached the file.
func TestCourseImportConfirmsRedirectedUpload(t *testing.T) {
	f := newFakeCanvas(t)
	f.stateAfter = []string{"queued", "running", "completed"}
	file := writeTestPackage(t, "imsmanifest.xml", "course_settings/canvas_export.txt")

	res, err := newTestImporter(f, file).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !f.confirmed {
		t.Fatal("upload redirect was not followed to create_success")
	}
	if f.confirmPath != "/api/v1/files/77/create_success" || f.confirmQuery["uuid"] != "u-tok-1" {
		t.Errorf("confirm hit %s %v", f.confirmPath, f.confirmQuery)
	}
	want, _ := os.ReadFile(file)
	if len(f.uploadBodies) != 1 || string(f.uploadBodies[0]) != string(want) {
		t.Errorf("uploaded file content mismatch")
	}
	if f.uploadFields[0]["token"] != "tok-1" || f.uploadFields[0]["filename"] != "pkg.imscc" {
		t.Errorf("upload_params not forwarded: %v", f.uploadFields[0])
	}
	if _, ok := f.uploadFields[0]["ignored"]; ok {
		t.Errorf("null upload_param should be skipped")
	}
	if res.MigrationID != 500 || res.AttachmentID != 77 || res.WorkflowState != "completed" || res.UploadAttempts != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if res.MigrationIssues != 1 || !strings.Contains(res.Note, "migration issue") {
		t.Errorf("issues not surfaced: %+v", res)
	}
}

func TestCourseImportRetriesWithFreshToken(t *testing.T) {
	f := newFakeCanvas(t)
	f.uploadStatus = []int{http.StatusBadRequest, http.StatusFound}
	f.stateAfter = []string{"completed"}
	file := writeTestPackage(t, "imsmanifest.xml")

	res, err := newTestImporter(f, file).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(f.puts) != 1 {
		t.Fatalf("want 1 token refresh PUT, got %d", len(f.puts))
	}
	if f.puts[0]["migration_type"] != "canvas_cartridge_importer" {
		t.Errorf("refresh PUT should carry migration_type: %v", f.puts[0])
	}
	if f.uploadFields[1]["token"] != "tok-2" {
		t.Errorf("retry reused the spent token: %v", f.uploadFields[1])
	}
	if res.UploadAttempts != 2 || res.WorkflowState != "completed" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestCourseImportReleasesOrphanWhenUploadNeverSucceeds(t *testing.T) {
	f := newFakeCanvas(t)
	f.uploadStatus = []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusBadRequest}
	file := writeTestPackage(t, "imsmanifest.xml")

	res, err := newTestImporter(f, file).run(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "migration 500") || !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error should name the migration and the cause: %v", err)
	}
	last := f.puts[len(f.puts)-1]
	if last["pre_attachment[size]"] != releaseSize {
		t.Errorf("last PUT should be the release: %v", last)
	}
	if !res.Released || res.WorkflowState != "failed" || res.UploadAttempts != 3 {
		t.Errorf("orphan not released: %+v", res)
	}
}

func TestCourseImportRefusesWhileAnotherImportIsActive(t *testing.T) {
	recent := time.Now().Add(-10 * time.Minute)
	old := time.Now().Add(-5 * time.Hour)
	f := newFakeCanvas(t)
	f.existing = []cmMigration{
		{ID: 268, WorkflowState: "pre_processing", StartedAt: &recent},
		{ID: 100, WorkflowState: "running", StartedAt: &old}, // past the block window
		{ID: 99, WorkflowState: "completed", StartedAt: &recent},
	}
	file := writeTestPackage(t, "imsmanifest.xml")

	_, err := newTestImporter(f, file).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "268 (pre_processing)") || strings.Contains(err.Error(), "100") {
		t.Fatalf("want refusal naming only migration 268, got %v", err)
	}
	if f.tokens != 0 {
		t.Error("must not create a migration when refusing")
	}

	f.stateAfter = []string{"completed"}
	imp := newTestImporter(f, file)
	imp.force = true
	if _, err := imp.run(context.Background()); err != nil {
		t.Fatalf("--force run: %v", err)
	}
}

func TestCourseImportNoWaitReturnsOnceQueued(t *testing.T) {
	f := newFakeCanvas(t)
	file := writeTestPackage(t, "imsmanifest.xml")
	imp := newTestImporter(f, file)
	imp.wait = false

	res, err := imp.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.WorkflowState != "queued" {
		t.Errorf("want queued, got %+v", res)
	}
}

func TestCourseImportReportsFailedMigration(t *testing.T) {
	f := newFakeCanvas(t)
	f.stateAfter = []string{"running", "failed"}
	file := writeTestPackage(t, "imsmanifest.xml")

	res, err := newTestImporter(f, file).run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "migration 500 failed") {
		t.Fatalf("want failure error, got %v", err)
	}
	if res.WorkflowState != "failed" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestDetectMigrationType(t *testing.T) {
	canvas := writeTestPackage(t, "imsmanifest.xml", "course_settings/canvas_export.txt")
	if got, err := detectMigrationType(canvas); err != nil || got != "canvas_cartridge_importer" {
		t.Errorf("canvas export: %q %v", got, err)
	}
	cc := writeTestPackage(t, "imsmanifest.xml", "web_resources/a.html")
	if got, err := detectMigrationType(cc); err != nil || got != "common_cartridge_importer" {
		t.Errorf("common cartridge: %q %v", got, err)
	}
	other := writeTestPackage(t, "readme.txt")
	if _, err := detectMigrationType(other); err == nil {
		t.Error("zip without a manifest should need --type")
	}
	notZip := filepath.Join(t.TempDir(), "x.imscc")
	os.WriteFile(notZip, []byte("not a zip"), 0o644)
	if _, err := detectMigrationType(notZip); err == nil {
		t.Error("non-zip should need --type")
	}
}
