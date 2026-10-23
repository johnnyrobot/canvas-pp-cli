// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the three-step upload: where each step is allowed to
// send, what the multipart body looks like on the wire, and what it means when
// the answer to the second step never comes back.

package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
)

// uploadFlow wires the fake the way Canvas's local-storage flow works: step 1
// hands out an upload ticket, the ticket's address answers 302, and
// create_success carries the file.
func uploadFlow(t *testing.T, f *gatewaytest.Fake, uploadURL string, params map[string]string, fileParam string) {
	t.Helper()
	f.Handle(http.MethodPost, "/api/v1/courses/7/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upload_url":    uploadURL,
			"upload_params": params,
			"file_param":    fileParam,
		})
	})
	f.Handle(http.MethodPost, "/files_api", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.URL+"/api/v1/files/31/create_success?uuid=u", http.StatusFound)
	})
	f.Handle(http.MethodGet, "/api/v1/files/31/create_success", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Canvas spells a file's type "content-type", with a hyphen.
		_, _ = w.Write([]byte(`{"id":31,"display_name":"diagram.png","size":1024,` +
			`"content-type":"image/png","folder_id":9,"url":"` + f.URL + `/files/31/download"}`))
	})
}

// Outside Canvas Builder nothing holds an upload_url to one host, and a real
// Canvas points it at its own file store, whose address authenticates itself.
// The file goes there. The token does not.
func TestUploadTokenStaysOnTheCanvasItWasPointedAt(t *testing.T) {
	f := gatewaytest.New(t)
	store, seen := otherHost(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files_api" {
			// S3-style storage answers an accepted upload with somewhere to
			// confirm it, which is the second request this test watches.
			http.Redirect(w, r, "/create_success?uuid=u", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":31,"display_name":"diagram.png","size":1024,"content-type":"image/png"}`))
	})
	f.Handle(http.MethodPost, "/api/v1/courses/7/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"upload_url":    store + "/files_api",
			"upload_params": map[string]string{},
			"file_param":    "file",
		})
	})
	path := fileOfSize(t, t.TempDir(), "diagram.png", 1024)
	c := New(&config.Config{BaseURL: f.URL, CanvasApiToken: "tA"}, 5*time.Second, 0)
	c.WriteVerify = WriteVerifyOff
	c.NoCache = true

	info, err := c.UploadFile(context.Background(), 7, UploadRequest{Path: path})
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	if info.ID != 31 {
		t.Fatalf("the file record came back as %+v", info)
	}
	requests := seen()
	if len(requests) != 2 {
		t.Fatalf("the file store saw %d requests, want 2: the file and the confirmation", len(requests))
	}
	for _, r := range requests {
		if r.Auth != "" {
			t.Errorf("%s carried Authorization %q, want none", r.Path, r.Auth)
		}
	}
	// The control: step 1 is Canvas's own request, and it still carries the
	// token, so an absent header above is the rule and not a lost token.
	if got := f.Requests(); len(got) == 0 {
		t.Fatal("Canvas saw no request at all")
	} else if auth := got[0].Header.Get("Authorization"); auth != "Bearer tA" {
		t.Errorf("step 1 carried Authorization %q, want %q", auth, "Bearer tA")
	}
}

// fileOfSize writes a file of exactly n bytes, filled so every 1 KiB block
// differs from the last: a body that arrives truncated or re-ordered fails the
// hash rather than passing on a run of zeroes.
func fileOfSize(t *testing.T, dir, name string, n int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	block := make([]byte, 1024)
	for written := 0; written < n; {
		for i := range block {
			block[i] = byte((written + i) % 251)
		}
		chunk := block
		if n-written < len(chunk) {
			chunk = chunk[:n-written]
		}
		if _, err := file.Write(chunk); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		written += len(chunk)
	}
	return path
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// uploadParts reads back a recorded multipart body as an ordered list of part
// names, with the file part's bytes.
func uploadParts(t *testing.T, rec gatewaytest.Recorded) (names []string, filename string, body []byte) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(rec.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("the upload's Content-Type %q: %v", rec.Header.Get("Content-Type"), err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("the upload's media type = %q, want multipart/form-data", mediaType)
	}
	mr := multipart.NewReader(bytes.NewReader(rec.Body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("reading the upload's parts: %v", err)
		}
		names = append(names, part.FormName())
		if part.FileName() != "" {
			filename = part.FileName()
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, part); err != nil {
				t.Fatalf("reading the file part: %v", err)
			}
			body = buf.Bytes()
		}
		_ = part.Close()
	}
	return names, filename, body
}

// The ticket's address is the gateway's or nothing happens: a Canvas that
// pointed the upload at S3 would be handing the file, and the token that
// carries it, to something outside Canvas Builder.
func TestUploadFileRefusesUploadURLOffGateway(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	uploadFlow(t, f, other+"/files_api", map[string]string{"Policy": "p"}, "file")
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	path := fileOfSize(t, t.TempDir(), "diagram.png", 1024)

	_, err := c.UploadFile(context.Background(), 7, UploadRequest{Path: path})
	if err == nil {
		t.Fatal("want a refusal for an upload address off the gateway")
	}
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("error = %T (%v), want a *RefusedError", err, err)
	}
	if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Fatalf("error = %q, want the NotGateway wording", err)
	}
	if n := count(); n != 0 {
		t.Fatalf("the other server saw %d requests, want 0", n)
	}
}

// The file went out and no answer came back, so nobody knows whether Canvas
// kept it. That is exit 8's whole point, and it must not be retried: a second
// attempt would upload the file again.
func TestUploadFileLostAnswerIsUnknown(t *testing.T) {
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api", map[string]string{"Policy": "p"}, "file")
	f.Handle(http.MethodPost, "/files_api", func(w http.ResponseWriter, _ *http.Request) {
		gatewaytest.Drop(w)
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	path := fileOfSize(t, t.TempDir(), "diagram.png", 1024)

	_, err := c.UploadFile(context.Background(), 7, UploadRequest{Path: path})
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T (%v), want an *OutcomeUnknownError", err, err)
	}
	var posts int
	for _, rec := range f.Requests() {
		if rec.Method == http.MethodPost && rec.Path == "/files_api" {
			posts++
		}
	}
	if posts != 1 {
		t.Fatalf("the fake saw %d uploads, want 1: a lost answer must not be retried", posts)
	}
}

// Canvas names the file part itself, and the name it picks depends on the
// endpoint. Sending our own name instead would put the file where nothing
// reads it.
func TestUploadUsesFileParamFromCanvas(t *testing.T) {
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api", map[string]string{"Policy": "p"}, "attachment[uploaded_data]")
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	path := fileOfSize(t, t.TempDir(), "diagram.png", 1024)

	if _, err := c.UploadFile(context.Background(), 7, UploadRequest{Path: path}); err != nil {
		t.Fatalf("uploading: %v", err)
	}
	rec := uploadRecord(t, f)
	names, filename, _ := uploadParts(t, rec)
	want := []string{"Policy", "attachment[uploaded_data]"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("parts = %v, want %v", names, want)
	}
	if filename != "diagram.png" {
		t.Fatalf("filename = %q, want diagram.png", filename)
	}
}

// 50 MB is an ordinary lecture recording. It has to go out as a stream with a
// length Canvas can trust, not as a body built in memory first.
func TestUploadLargeFileStreams(t *testing.T) {
	const size = 50 * 1024 * 1024
	f := gatewaytest.New(t)
	uploadFlow(t, f, f.URL+"/files_api", map[string]string{"Policy": "p", "Signature": "s"}, "file")
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 60*time.Second)
	path := fileOfSize(t, t.TempDir(), "lecture.mp4", size)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back the file: %v", err)
	}

	if _, err := c.UploadFile(context.Background(), 7, UploadRequest{Path: path}); err != nil {
		t.Fatalf("uploading: %v", err)
	}
	rec := uploadRecord(t, f)
	if rec.ContentLength != int64(len(rec.Body)) {
		t.Fatalf("Content-Length = %d, but %d bytes arrived", rec.ContentLength, len(rec.Body))
	}
	if rec.ContentLength <= size {
		t.Fatalf("Content-Length = %d, want more than the file's %d bytes", rec.ContentLength, size)
	}
	_, _, got := uploadParts(t, rec)
	if len(got) != size {
		t.Fatalf("the file part is %d bytes, want %d", len(got), size)
	}
	if sum(got) != sum(want) {
		t.Fatal("the file part's SHA-256 doesn't match the file on disk")
	}
}

// uploadRecord returns the one request the fake saw at the ticket's address.
func uploadRecord(t *testing.T, f *gatewaytest.Fake) gatewaytest.Recorded {
	t.Helper()
	for _, rec := range f.Requests() {
		if rec.Method == http.MethodPost && rec.Path == "/files_api" {
			return rec
		}
	}
	t.Fatal("the fake saw no upload at /files_api")
	return gatewaytest.Recorded{}
}

// TestUploadTimeoutScalesWithSize covers the one bound that is neither the
// context's nor the flag's: the file's own. A 500 MB video on a slow disk must
// not be cut off and reported as an outcome nobody knows.
func TestUploadTimeoutScalesWithSize(t *testing.T) {
	t.Parallel()
	const base = 90 * time.Second
	if got := transferTimeout(base, 0); got != base {
		t.Errorf("an empty file: got %s, want %s", got, base)
	}
	if got := transferTimeout(base, 500<<20); got != base+500*time.Second {
		t.Errorf("500 MB: got %s, want %s", got, base+500*time.Second)
	}
	if got := transferTimeout(0, 500<<20); got != 0 {
		t.Errorf("no bound asked for: got %s, want 0", got)
	}
}
