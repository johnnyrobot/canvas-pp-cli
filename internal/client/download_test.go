// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for downloading a course file: where the bytes are
// allowed to come from, which hops may be followed to reach them, and what it
// means when fewer arrive than Canvas said there were.

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// downloadFlow registers the two requests a download makes: the file's own
// record, which says where the bytes are, and the bytes.
func downloadFlow(t *testing.T, f *gatewaytest.Fake, fileURL string, size int64, body []byte) {
	t.Helper()
	f.Handle(http.MethodGet, "/api/v1/courses/7/files/31", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id":31,"display_name":"diagram.png","size":%d,"content-type":"image/png","folder_id":9,"url":%q}`, size, fileURL)
	})
	f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})
}

func TestDownloadFileSavesWhatCanvasSent(t *testing.T) {
	f := gatewaytest.New(t)
	want := bytes.Repeat([]byte("canvas"), 400)
	downloadFlow(t, f, f.URL+"/files/31/download?download_frd=1&verifier=v", int64(len(want)), want)
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	var got bytes.Buffer
	info, n, err := c.DownloadFile(context.Background(), 7, 31, &got)
	if err != nil {
		t.Fatalf("downloading the file: %v", err)
	}
	if n != int64(len(want)) || !bytes.Equal(got.Bytes(), want) {
		t.Errorf("got %d bytes, want %d, equal=%v", n, len(want), bytes.Equal(got.Bytes(), want))
	}
	if info.DisplayName != "diagram.png" || info.ContentType != "image/png" {
		t.Errorf("file record: %+v", info)
	}
	// The verifier in a file's address is a credential. The second request has
	// to carry it, because that is what it is for, but nothing else may.
	last := f.Requests()[len(f.Requests())-1]
	if last.RawQuery != "download_frd=1&verifier=v" {
		t.Errorf("the download query was %q", last.RawQuery)
	}
	if auth := last.Header.Get("Authorization"); auth != "Bearer tA" {
		t.Errorf("the download carried Authorization %q", auth)
	}
}

// The same rule on the way out: outside Canvas Builder a file's address is
// whatever the record says, and a real Canvas's is its file store's, with a
// verifier of its own in the query.
func TestDownloadTokenStaysOnTheCanvasItWasPointedAt(t *testing.T) {
	f := gatewaytest.New(t)
	want := []byte("canvas")
	store, seen := otherHost(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(want)
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/files/31", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"id":31,"display_name":"diagram.png","size":%d,"content-type":"image/png","url":%q}`,
			len(want), store+"/files/31/download?token=jwt")
	})
	c := New(&config.Config{BaseURL: f.URL, CanvasApiToken: "tA"}, 5*time.Second, 0)
	c.NoCache = true

	var got bytes.Buffer
	_, n, err := c.DownloadFile(context.Background(), 7, 31, &got)
	if err != nil {
		t.Fatalf("downloading the file: %v", err)
	}
	if n != int64(len(want)) || !bytes.Equal(got.Bytes(), want) {
		t.Errorf("got %q (%d bytes), want %q", got.Bytes(), n, want)
	}
	requests := seen()
	if len(requests) != 1 {
		t.Fatalf("the file store saw %d requests, want 1", len(requests))
	}
	if requests[0].Auth != "" {
		t.Errorf("the fetch carried Authorization %q, want none", requests[0].Auth)
	}
	// The control: the file's record came from Canvas itself, with the token.
	if auth := f.Requests()[0].Header.Get("Authorization"); auth != "Bearer tA" {
		t.Errorf("the record request carried Authorization %q, want %q", auth, "Bearer tA")
	}
}

func TestDownloadFileRefusesURLOffGateway(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	downloadFlow(t, f, other+"/files/31/download", 100, nil)
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	var got bytes.Buffer
	_, _, err := c.DownloadFile(context.Background(), 7, 31, &got)
	if err == nil {
		t.Fatal("a file address outside Canvas Builder was fetched")
	}
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("got %T (%v), want a RefusedError", err, err)
	}
	if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Errorf("the refusal said: %v", err)
	}
	if count() != 0 {
		t.Errorf("the other server saw %d requests", count())
	}
	if got.Len() != 0 {
		t.Errorf("%d bytes were written anyway", got.Len())
	}
}

// TestDownloadFileRefusesURLThatIsntAFile covers the half of the rule the
// gateway check can't see: an address on the gateway, with a path that isn't a
// file download. A file's url is the one address Canvas hands this CLI to
// fetch with its token, so what it points at is checked too.
func TestDownloadFileRefusesURLThatIsntAFile(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, f.URL+"/api/v1/users/self", 100, nil)
	f.Handle(http.MethodGet, "/api/v1/users/self", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	var got bytes.Buffer
	_, _, err := c.DownloadFile(context.Background(), 7, 31, &got)
	var refused *RefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("got %T (%v), want a RefusedError", err, err)
	}
	for _, rec := range f.Requests() {
		if rec.Path == "/api/v1/users/self" {
			t.Error("the CLI fetched the address Canvas named without checking what it was")
		}
	}
}

func TestDownloadFileFollowsGatewayRedirectOnly(t *testing.T) {
	t.Run("a hop inside Canvas Builder", func(t *testing.T) {
		f := gatewaytest.New(t)
		want := bytes.Repeat([]byte("x"), 512)
		downloadFlow(t, f, f.URL+"/files/31/download", int64(len(want)), nil)
		f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, f.URL+"/courses/7/files/31/download", http.StatusFound)
		})
		f.Handle(http.MethodGet, "/courses/7/files/31/download", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(want)
		})
		c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

		var got bytes.Buffer
		if _, _, err := c.DownloadFile(context.Background(), 7, 31, &got); err != nil {
			t.Fatalf("a hop within Canvas Builder was refused: %v", err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Error("the bytes after the hop aren't the file")
		}
	})

	t.Run("a hop out of Canvas Builder", func(t *testing.T) {
		f := gatewaytest.New(t)
		other, count := elsewhere(t)
		downloadFlow(t, f, f.URL+"/files/31/download", 512, nil)
		f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other+"/files/31/download", http.StatusFound)
		})
		c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

		var got bytes.Buffer
		_, _, err := c.DownloadFile(context.Background(), 7, 31, &got)
		if err == nil {
			t.Fatal("a hop out of Canvas Builder was followed")
		}
		if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
			t.Errorf("the refusal said: %v", err)
		}
		if count() != 0 {
			t.Errorf("the other server saw %d requests", count())
		}
	})
}

// TestDownloadFileShortBodyIsShort is the failure a file transfer has that a
// request carrying JSON doesn't: a body that stops partway looks exactly like a
// whole file once it is on disk, so the size Canvas gave is checked against
// what arrived.
func TestDownloadFileShortBodyIsShort(t *testing.T) {
	f := gatewaytest.New(t)
	downloadFlow(t, f, f.URL+"/files/31/download", 2048, nil)
	f.Handle(http.MethodGet, "/files/31/download", func(w http.ResponseWriter, r *http.Request) {
		gatewaytest.Truncated(w, 2048, bytes.Repeat([]byte("y"), 1024))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	var got bytes.Buffer
	_, _, err := c.DownloadFile(context.Background(), 7, 31, &got)
	if err == nil {
		t.Fatal("half a file came back as a whole one")
	}
	if !strings.Contains(err.Error(), sandbox.MsgDownloadShort) {
		t.Errorf("got %q, want it to say %q", err, sandbox.MsgDownloadShort)
	}
}
