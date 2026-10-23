// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the gateway-only client: where it will send a
// request, what it does with a redirect, how long it waits for an approval, and
// the job label it puts on a change.

package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
)

// sandboxClient loads a real Canvas Builder config pointing at f and builds the
// client from it, which is the only way the profile is ever attached.
func sandboxClient(t *testing.T, f *gatewaytest.Fake, o gatewaytest.WorkspaceOpts, timeout time.Duration) *Client {
	t.Helper()
	gatewaytest.Workspace(t, f, o)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("loading the workspace config: %v", err)
	}
	c := New(cfg, timeout, 0)
	if c.Sandbox == nil {
		t.Fatal("the client has no sandbox profile, so none of these tests mean anything")
	}
	c.WriteVerify = WriteVerifyOff
	c.NoCache = true
	c.Notices = &bytes.Buffer{}
	return c
}

// elsewhere is a second server the CLI is never allowed to reach: same host,
// different port, so it fails the gateway rule on the port alone.
func elsewhere(t *testing.T) (url string, requests func() int) {
	t.Helper()
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() int { return int(n.Load()) }
}

// offHostRequest is what a host that isn't Canvas saw of one request: enough
// to say whether the token went there.
type offHostRequest struct {
	Path string
	Auth string
}

// otherHost is a second server that is not the Canvas the CLI was pointed at —
// a file store, in the flows that have one — and it keeps what each request
// carried.
func otherHost(t *testing.T, handler http.HandlerFunc) (base string, seen func() []offHostRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []offHostRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, offHostRequest{Path: r.URL.Path, Auth: r.Header.Get("Authorization")})
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []offHostRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]offHostRequest(nil), got...)
	}
}

func TestSandboxClient_RefusesRedirectOffGateway(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other+"/api/v1/courses/7/pages", http.StatusFound)
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	_, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil)
	if err == nil {
		t.Fatal("want an error for a redirect off the gateway")
	}
	if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Fatalf("error = %q, want the NotGateway wording", err)
	}
	if n := count(); n != 0 {
		t.Fatalf("the other server saw %d requests, want 0", n)
	}
	if strings.Contains(err.Error(), "tA") {
		t.Fatalf("error names the token: %q", err)
	}
}

// A refusal is the same answer every time, so retrying it only re-sends the
// write to the gateway -- and a write the gateway holds for approval would ask
// the instructor again on each attempt.
func TestSandboxClient_RedirectRefusalIsNotRetried(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other+"/api/v1/courses/7/pages/intro", http.StatusFound)
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	c.JobLabel = "Term move"

	_, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"})
	if err == nil {
		t.Fatal("want an error for a redirect off the gateway")
	}
	if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Fatalf("error = %q, want the NotGateway wording", err)
	}
	if n := countWrites(f, http.MethodPut); n != 1 {
		t.Fatalf("the gateway saw %d PUTs, want exactly 1", n)
	}
	if n := count(); n != 0 {
		t.Fatalf("the other server saw %d requests, want 0", n)
	}
}

func TestSandboxClient_FollowsRedirectOnGateway(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.URL+"/api/v1/courses/7/pages/second", http.StatusFound)
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages/second", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	if _, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	reqs := f.Requests()
	if len(reqs) != 2 {
		t.Fatalf("the gateway saw %d requests, want 2", len(reqs))
	}
	if got := reqs[1].Header.Get("Authorization"); got != "Bearer tA" {
		t.Fatalf("second hop Authorization = %q, want %q", got, "Bearer tA")
	}
}

func TestSandboxClient_RefusesBaseURLChangedAfterNew(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	c.BaseURL = other

	_, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil)
	if err == nil {
		t.Fatal("want an error for a base URL that isn't the gateway")
	}
	if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Fatalf("error = %q, want the NotGateway wording", err)
	}
	if n := count(); n != 0 {
		t.Fatalf("the other server saw %d requests, want 0", n)
	}
	if n := len(f.Requests()); n != 0 {
		t.Fatalf("the gateway saw %d requests, want 0", n)
	}
}

func TestSandboxClient_TimeoutOutlastsApproval(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{ApprovalTimeout: "1s"}, 200*time.Millisecond)

	if got, want := c.HTTPClient.Timeout, 31*time.Second; got != want {
		t.Fatalf("HTTPClient.Timeout = %s, want %s", got, want)
	}
	c.JobLabel = "Term move"
	if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestJobLabelHeader(t *testing.T) {
	const label = "Déplacer — مقرر"
	f := gatewaytest.New(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	c.JobLabel = label

	if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got := f.Requests()[0].Header.Get("X-Job-Label"); got != label {
		t.Fatalf("X-Job-Label = %q, want %q", got, label)
	}

	c.JobLabel = ""
	if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
		t.Fatalf("Put without a label: %v", err)
	}
	if _, ok := f.Requests()[1].Header["X-Job-Label"]; ok {
		t.Fatal("an empty label still sent the header")
	}
}

func TestUnlabelledWriteWarns(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	notices := &bytes.Buffer{}
	c.Notices = notices

	for range 2 {
		if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if n := countWrites(f, http.MethodPut); n != 2 {
		t.Fatalf("the gateway saw %d PUTs, want 2: an unlabelled write still goes out", n)
	}
	if n := strings.Count(notices.String(), "no job label"); n != 1 {
		t.Fatalf("the job-label notice appeared %d times in %q, want once", n, notices.String())
	}

	notices.Reset()
	if _, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if notices.Len() != 0 {
		t.Fatalf("a read printed %q", notices.String())
	}
}

func TestUnlabelledWriteSilentInNormalMode(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	notices := &bytes.Buffer{}
	c := New(&config.Config{BaseURL: f.URL, CanvasApiToken: "tA"}, 5*time.Second, 0)
	c.WriteVerify = WriteVerifyOff
	c.Notices = notices

	if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if notices.Len() != 0 {
		t.Fatalf("normal mode printed %q", notices.String())
	}
}

func TestTokenOnlyInHeader(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/intro", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"intro"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	c.JobLabel = "Term move"

	if _, err := c.Get(context.Background(), "/api/v1/courses/7/pages", map[string]string{"per_page": "100"}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/intro", map[string]any{"wiki_page[title]": "Intro"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, r := range f.Requests() {
		if got := r.Header.Get("Authorization"); got != "Bearer tA" {
			t.Fatalf("%s %s Authorization = %q, want %q", r.Method, r.Path, got, "Bearer tA")
		}
		if strings.Contains(r.RawQuery, "access_token") || strings.Contains(r.RawQuery, "tA") {
			t.Fatalf("%s %s query carries the token: %q", r.Method, r.Path, r.RawQuery)
		}
		if bytes.Contains(r.Body, []byte("access_token")) || bytes.Contains(r.Body, []byte("tA")) {
			t.Fatalf("%s %s body carries the token: %q", r.Method, r.Path, r.Body)
		}
	}
}

func TestNormalClientUnchanged(t *testing.T) {
	var mu sync.Mutex
	var sawAuth string
	var hops int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hops++
		sawAuth = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	t.Cleanup(other.Close)
	f := gatewaytest.New(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/moved", http.StatusFound)
	})

	c := New(&config.Config{BaseURL: f.URL, CanvasApiToken: "tA"}, 5*time.Second, 0)
	c.NoCache = true
	if c.Sandbox != nil {
		t.Fatal("a config with no profile must not produce a sandbox client")
	}
	if _, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hops != 1 {
		t.Fatalf("the other host saw %d requests, want 1: a normal client still follows a cross-host redirect", hops)
	}
	if sawAuth != "" {
		t.Fatalf("cross-host hop carried Authorization %q, want none", sawAuth)
	}
}

func countWrites(f *gatewaytest.Fake, method string) int {
	n := 0
	for _, r := range f.Requests() {
		if r.Method == method {
			n++
		}
	}
	return n
}
