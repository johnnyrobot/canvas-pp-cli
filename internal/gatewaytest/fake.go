// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: a stand-in for cb-gateway, and the course workspace a Canvas
// Builder CLI run expects around it. Tasks 2 to 9 test against this rather than
// a real gateway, so the CLI's half of the contract can be pinned on its own.
//
// It imports neither client nor cli, so both can use it.

package gatewaytest

import (
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
)

// Recorded is one request the fake gateway saw.
type Recorded struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     []byte
}

// Fake is an httptest server on 127.0.0.1 that answers only the method and path
// pairs a test registers. Anything else is a 404 in Canvas's error shape, so a
// request the CLI shouldn't be making fails the test rather than passing
// quietly.
type Fake struct {
	// URL is http://127.0.0.1:<port>, the form a gateway address has to take.
	URL string

	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	requests []Recorded
}

// New starts a fake gateway and stops it when the test ends.
func New(t testing.TB) *Fake {
	t.Helper()
	f := &Fake{handlers: map[string]http.HandlerFunc{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Handle registers a handler for one exact method and path.
func (f *Fake) Handle(method, path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[method+" "+path] = h
}

// Requests returns every request the fake saw, oldest first.
func (f *Fake) Requests() []Recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Recorded(nil), f.requests...)
}

// HostPort is the fake's host:port, such as 127.0.0.1:53124.
func (f *Fake) HostPort() string {
	return strings.TrimPrefix(f.URL, "http://")
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	body := readAll(r)
	f.mu.Lock()
	f.requests = append(f.requests, Recorded{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Header:   r.Header.Clone(),
		Body:     body,
	})
	h := f.handlers[r.Method+" "+r.URL.Path]
	f.mu.Unlock()
	if h == nil {
		Refuse(w, http.StatusNotFound, "The specified resource does not exist.")
		return
	}
	h(w, r)
}

func readAll(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// A dropped or truncated request body is itself worth recording; the
		// test asserts on what arrived, not on how it ended.
		return body
	}
	return body
}

// Refuse answers in Canvas's error shape, which is also the shape the gateway
// uses for its own refusals: {"errors":[{"message":"…"}]}.
func Refuse(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]string{{"message": msg}},
	})
}

// Drop closes the connection without answering, the way a gateway that died
// mid-request looks from the client's side.
func Drop(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		panic("gatewaytest: the response writer can't be hijacked, so Drop can't close the connection")
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		panic("gatewaytest: hijacking the connection: " + err.Error())
	}
	_ = conn.Close()
}

// WorkspaceOpts tunes the workspace Workspace writes. The zero value is the
// ordinary case: a two-minute approval timeout, no checks command, and the two
// closure days the date tests use.
type WorkspaceOpts struct {
	ApprovalTimeout string
	ChecksCommand   []string
	ClosureDays     []string
	// OmitClosureDays writes a workspace.json with no closure_days at all,
	// for the course whose instructor hasn't entered them yet.
	OmitClosureDays bool
	// OmitSandbox leaves the sandbox key out of config.toml, so the same
	// layout runs as the plain CLI. It is how a test compares the two modes.
	OmitSandbox bool
}

// Home is where Workspace put things.
type Home struct {
	// Dir is the CLI home: config/config.toml and data/credentials.toml.
	Dir string
	// Workspace is the course workspace folder: workspace.json and materials/.
	Workspace string
}

// Workspace lays out a course workspace and its CLI home exactly as Canvas
// Builder does, points the settings at f, and clears every other way of finding
// a config, a base URL or a token so a test can't read the developer's own.
func Workspace(t testing.TB, f *Fake, o WorkspaceOpts) Home {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "cli-home")
	ws := filepath.Join(root, "ws")

	timeout := o.ApprovalTimeout
	if timeout == "" {
		timeout = "120s"
	}
	days := o.ClosureDays
	if days == nil {
		days = []string{"2027-05-31", "2027-06-04"}
	}

	cfg := &strings.Builder{}
	if !o.OmitSandbox {
		fmt.Fprintf(cfg, "sandbox = true\n")
	}
	fmt.Fprintf(cfg, "base_url = %s\n", tomlString(f.URL))
	fmt.Fprintf(cfg, "gateway = %s\n", tomlString(f.URL))
	fmt.Fprintf(cfg, "workspace = %s\n", tomlString(ws))
	fmt.Fprintf(cfg, "approval_timeout = %s\n", tomlString(timeout))
	fmt.Fprintf(cfg, "checks_command = %s\n", tomlStrings(o.ChecksCommand))
	writeFile(t, filepath.Join(home, "config", "config.toml"), cfg.String())
	writeFile(t, filepath.Join(home, "data", "credentials.toml"), "api_token = \"tA\"\n")

	wsJSON := map[string]any{"course_id": 7}
	if !o.OmitClosureDays {
		wsJSON["closure_days"] = days
	}
	body, err := json.Marshal(wsJSON)
	if err != nil {
		t.Fatalf("gatewaytest: marshaling workspace.json: %v", err)
	}
	writeFile(t, filepath.Join(ws, "workspace.json"), string(body)+"\n")
	if err := os.MkdirAll(filepath.Join(ws, "materials"), 0o700); err != nil {
		t.Fatalf("gatewaytest: creating materials/: %v", err)
	}

	t.Setenv("CANVAS_HOME", home)
	// HOME too: the legacy config path is $HOME/.config/canvas-pp-cli, and the
	// CLI falls back to it when the primary path has no file.
	t.Setenv("HOME", filepath.Join(root, "fake-home"))
	for _, name := range []string{
		"CANVAS_CONFIG", "CANVAS_BASE_URL", "CANVAS_API_TOKEN", "CANVAS_ACCESS_TOKEN",
		"CANVAS_CONFIG_DIR", "CANVAS_DATA_DIR", "CANVAS_STATE_DIR", "CANVAS_CACHE_DIR",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	} {
		t.Setenv(name, "")
	}
	return Home{Dir: home, Workspace: ws}
}

func writeFile(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("gatewaytest: mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("gatewaytest: write %s: %v", path, err)
	}
}

func tomlString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic("gatewaytest: quoting " + s)
	}
	return string(b)
}

func tomlStrings(items []string) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, tomlString(item))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
