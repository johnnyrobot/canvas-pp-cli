// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for what the client does when a request fails: which
// failures are safe to send again, which ones leave the outcome unknown, and
// what the agent is told while it waits for the instructor.

package client

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// normalClient is the same client outside Canvas Builder: no profile, so no
// gateway rule, no stretched timeout and no notices.
func normalClient(t *testing.T, baseURL string, timeout time.Duration) *Client {
	t.Helper()
	c := New(&config.Config{BaseURL: baseURL, CanvasApiToken: "tA"}, timeout, 0)
	if c.Sandbox != nil {
		t.Fatal("a plain config should not produce a sandbox profile")
	}
	c.WriteVerify = WriteVerifyOff
	c.NoCache = true
	c.Notices = &bytes.Buffer{}
	c.backoff = func(int) time.Duration { return 0 }
	return c
}

// countRequests reports how many requests of one method reached the fake.
func countRequests(f *gatewaytest.Fake, method string) int {
	n := 0
	for _, r := range f.Requests() {
		if r.Method == method {
			n++
		}
	}
	return n
}

func notices(t *testing.T, c *Client) string {
	t.Helper()
	buf, ok := c.Notices.(*bytes.Buffer)
	if !ok {
		t.Fatal("this test needs a buffer for notices")
	}
	return buf.String()
}

// [Review Focus 2] The answer to a create is lost on the way back. The change
// may well have been applied, so sending it again could make a second page.
func TestPostNotRetriedAfterLostAnswer(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPost, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		gatewaytest.Drop(w)
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	_, _, err := c.Post(context.Background(), "/api/v1/courses/7/pages", map[string]any{"wiki_page[title]": "Week 1"})
	var unknown *OutcomeUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v (%T), want an OutcomeUnknownError", err, err)
	}
	if err.Error() != sandbox.MsgUnknownOutcome {
		t.Fatalf("err.Error() = %q, want the unknown-outcome message", err)
	}
	if unknown.Method != http.MethodPost || !strings.Contains(unknown.Path, "/api/v1/courses/7/pages") {
		t.Fatalf("the error names %s %s, want the POST and its path", unknown.Method, unknown.Path)
	}
	if n := countRequests(f, http.MethodPost); n != 1 {
		t.Fatalf("the gateway saw %d POSTs, want exactly 1", n)
	}
}

// A 5xx is an answer: Canvas or the gateway said it failed, so nothing was
// applied and there is nothing unknown about it -- but a create is still not
// worth sending again on the same breath.
func TestPostNotRetriedAfter5xx(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPost, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		gatewaytest.Refuse(w, http.StatusInternalServerError, "Internal server error")
	})
	c := normalClient(t, f.URL, 5*time.Second)

	_, status, err := c.Post(context.Background(), "/api/v1/courses/7/pages", map[string]any{"wiki_page[title]": "Week 1"})
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("err = %v (%T), want an APIError", err, err)
	}
	if status != http.StatusInternalServerError || apiError.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, APIError.StatusCode = %d, want 500 for both", status, apiError.StatusCode)
	}
	var unknown *OutcomeUnknownError
	if errors.As(err, &unknown) {
		t.Fatal("a 500 is an answer, so the outcome is not unknown")
	}
	if n := countRequests(f, http.MethodPost); n != 1 {
		t.Fatalf("the server saw %d POSTs, want exactly 1", n)
	}
}

// When Canvas Builder itself says the outcome is unknown, that answer stands as
// it is: no second attempt, and the agent reads the gateway's own sentence.
func TestGatewayUnknownOutcomeNotRetried(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPut, "/api/v1/courses/7/pages/week-1", func(w http.ResponseWriter, r *http.Request) {
		gatewaytest.Refuse(w, http.StatusBadGateway, sandbox.MsgUnknownOutcome)
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	_, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[title]": "Week 1"})
	if err == nil {
		t.Fatal("want an error for a 502 the gateway sent")
	}
	if err.Error() != sandbox.MsgUnknownOutcome {
		t.Fatalf("err.Error() = %q, want exactly the unknown-outcome message", err)
	}
	if n := countRequests(f, http.MethodPut); n != 1 {
		t.Fatalf("the gateway saw %d PUTs, want exactly 1", n)
	}
}

// A PUT is idempotent at Canvas, so outside Canvas Builder it can go again. In
// Canvas Builder every attempt is a separate change in the journal, and a held
// one is a separate question for the instructor.
func TestSandboxPutDeleteNotRetried(t *testing.T) {
	drop := func(w http.ResponseWriter, r *http.Request) { gatewaytest.Drop(w) }

	t.Run("in Canvas Builder", func(t *testing.T) {
		f := gatewaytest.New(t)
		f.Handle(http.MethodPut, "/api/v1/courses/7/pages/week-1", drop)
		c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

		_, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[title]": "Week 1"})
		var unknown *OutcomeUnknownError
		if !errors.As(err, &unknown) {
			t.Fatalf("err = %v (%T), want an OutcomeUnknownError", err, err)
		}
		if n := countRequests(f, http.MethodPut); n != 1 {
			t.Fatalf("the gateway saw %d PUTs, want exactly 1", n)
		}
	})

	t.Run("outside it", func(t *testing.T) {
		f := gatewaytest.New(t)
		f.Handle(http.MethodPut, "/api/v1/courses/7/pages/week-1", drop)
		c := normalClient(t, f.URL, 5*time.Second)

		_, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[title]": "Week 1"})
		if err == nil {
			t.Fatal("want an error when every attempt is dropped")
		}
		var unknown *OutcomeUnknownError
		if errors.As(err, &unknown) {
			t.Fatal("outside Canvas Builder a dropped PUT is retried, not reported as unknown")
		}
		if n := countRequests(f, http.MethodPut); n < 2 || n > 4 {
			t.Fatalf("the server saw %d PUTs, want more than one and at most four", n)
		}
	})
}

// Reads are safe to repeat, so a 503 is still worth waiting out.
func TestGetStillRetried(t *testing.T) {
	f := gatewaytest.New(t)
	var seen atomic.Int64
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		if seen.Add(1) <= 2 {
			gatewaytest.Refuse(w, http.StatusServiceUnavailable, "Service unavailable")
			return
		}
		_, _ = w.Write([]byte(`[{"url":"week-1"}]`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	c.backoff = func(int) time.Duration { return 0 }

	out, err := c.Get(context.Background(), "/api/v1/courses/7/pages", nil)
	if err != nil {
		t.Fatalf("want the third attempt to succeed: %v", err)
	}
	if !strings.Contains(string(out), "week-1") {
		t.Fatalf("body = %s, want the page list", out)
	}
	if n := countRequests(f, http.MethodGet); n != 3 {
		t.Fatalf("the gateway saw %d GETs, want exactly 3", n)
	}
}

// dialOnce fails the first connection the way a gateway that hasn't finished
// starting does. Nothing was sent, so even a create can go again.
type dialOnce struct {
	calls atomic.Int64
	inner http.RoundTripper
}

func (d *dialOnce) RoundTrip(r *http.Request) (*http.Response, error) {
	if d.calls.Add(1) == 1 {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}
	}
	return d.inner.RoundTrip(r)
}

func TestDialErrorRetriedForPost(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodPost, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"week-1"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	rt := &dialOnce{inner: c.HTTPClient.Transport}
	if rt.inner == nil {
		rt.inner = http.DefaultTransport
	}
	c.HTTPClient.Transport = rt

	_, _, err := c.Post(context.Background(), "/api/v1/courses/7/pages", map[string]any{"wiki_page[title]": "Week 1"})
	if err != nil {
		t.Fatalf("want the second attempt to succeed: %v", err)
	}
	if n := rt.calls.Load(); n != 2 {
		t.Fatalf("the transport was called %d times, want 2", n)
	}
	if n := countRequests(f, http.MethodPost); n != 1 {
		t.Fatalf("the gateway saw %d POSTs, want exactly 1", n)
	}
}

// [Review Focus 1] A delete is always held for the instructor, so the agent is
// told so before the wait starts -- once, and the request is never sent twice.
func TestApprovalNoticeForDelete(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodDelete, "/api/v1/courses/7/pages/week-1", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"url":"week-1"}`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
	// A labelled change, so the only thing said here is about the wait.
	c.JobLabel = "Spring 2027 setup"

	if _, _, err := c.Delete(context.Background(), "/api/v1/courses/7/pages/week-1"); err != nil {
		t.Fatalf("deleting the page: %v", err)
	}
	want := sandbox.ApprovalNotice(c.Sandbox.ApprovalTimeout)
	if got := strings.Count(notices(t, c), want); got != 1 {
		t.Fatalf("the approval notice appears %d times in %q, want exactly 1", got, notices(t, c))
	}
	if n := countRequests(f, http.MethodDelete); n != 1 {
		t.Fatalf("the gateway saw %d DELETEs, want exactly 1", n)
	}
}

// A change the gateway doesn't hold still takes a moment, and the agent should
// hear why if it takes longer than that.
func TestStillWaitingNotice(t *testing.T) {
	t.Run("a slow answer", func(t *testing.T) {
		f := gatewaytest.New(t)
		f.Handle(http.MethodPut, "/api/v1/courses/7/pages/week-1", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(`{"url":"week-1"}`))
		})
		c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
		c.JobLabel = "Spring 2027 setup"
		c.stillWaitingAfter = 50 * time.Millisecond

		if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[body]": "<p>x</p>"}); err != nil {
			t.Fatalf("writing the page: %v", err)
		}
		want := sandbox.StillWaitingNotice(c.Sandbox.ApprovalTimeout)
		if got := strings.Count(notices(t, c), want); got != 1 {
			t.Fatalf("the still-waiting notice appears %d times in %q, want exactly 1", got, notices(t, c))
		}
		if strings.Contains(notices(t, c), sandbox.ApprovalNotice(c.Sandbox.ApprovalTimeout)) {
			t.Fatal("a plain page edit is not held for approval, so it should not say so")
		}
	})

	t.Run("an answer that comes at once", func(t *testing.T) {
		f := gatewaytest.New(t)
		f.Handle(http.MethodPut, "/api/v1/courses/7/pages/week-1", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"url":"week-1"}`))
		})
		c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)
		c.JobLabel = "Spring 2027 setup"
		c.stillWaitingAfter = time.Second

		if _, _, err := c.Put(context.Background(), "/api/v1/courses/7/pages/week-1", map[string]any{"wiki_page[body]": "<p>x</p>"}); err != nil {
			t.Fatalf("writing the page: %v", err)
		}
		if got := notices(t, c); got != "" {
			t.Fatalf("notices = %q, want nothing said about waiting", got)
		}
	})
}

// Outside Canvas Builder there is no instructor to ask, so neither notice makes
// sense.
func TestNoNoticesOutsideSandbox(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodDelete, "/api/v1/courses/7/pages/week-1", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"url":"week-1"}`))
	})
	c := normalClient(t, f.URL, 5*time.Second)
	c.stillWaitingAfter = 10 * time.Millisecond

	if _, _, err := c.Delete(context.Background(), "/api/v1/courses/7/pages/week-1"); err != nil {
		t.Fatalf("deleting the page: %v", err)
	}
	if got := notices(t, c); got != "" {
		t.Fatalf("notices = %q, want nothing", got)
	}
}

func TestRetryDecision(t *testing.T) {
	t.Parallel()
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	lost := errors.New("EOF")
	unknownBody := []byte(`{"errors":[{"message":"` + sandbox.MsgUnknownOutcome + `"}]}`)

	cases := []struct {
		name        string
		method      string
		sandboxMode bool
		transport   error
		status      int
		body        []byte
		retry       bool
		unknown     bool
	}{
		{"dial error, read", http.MethodGet, true, dial, 0, nil, true, false},
		{"dial error, create", http.MethodPost, true, dial, 0, nil, true, false},
		{"dial error, write in Canvas Builder", http.MethodPut, true, dial, 0, nil, true, false},
		{"lost answer, read", http.MethodGet, true, lost, 0, nil, true, false},
		{"lost answer, create", http.MethodPost, false, lost, 0, nil, false, true},
		{"lost answer, patch", http.MethodPatch, false, lost, 0, nil, false, true},
		{"lost answer, write outside Canvas Builder", http.MethodPut, false, lost, 0, nil, true, false},
		{"lost answer, delete outside Canvas Builder", http.MethodDelete, false, lost, 0, nil, true, false},
		{"lost answer, write in Canvas Builder", http.MethodPut, true, lost, 0, nil, false, true},
		{"lost answer, delete in Canvas Builder", http.MethodDelete, true, lost, 0, nil, false, true},
		{"5xx, read", http.MethodGet, true, nil, 503, nil, true, false},
		{"5xx, create", http.MethodPost, false, nil, 500, nil, false, false},
		{"5xx, write outside Canvas Builder", http.MethodPut, false, nil, 500, nil, true, false},
		{"5xx, write in Canvas Builder", http.MethodPut, true, nil, 500, nil, false, false},
		{"unknown outcome from the gateway, read", http.MethodGet, true, nil, 502, unknownBody, false, true},
		{"unknown outcome from the gateway, write", http.MethodPut, true, nil, 502, unknownBody, false, true},
		{"a plain 502, write in Canvas Builder", http.MethodPut, true, nil, 502, []byte(`{"errors":[{"message":"Bad gateway"}]}`), false, false},
		{"429, create", http.MethodPost, true, nil, 429, nil, true, false},
		{"429, write in Canvas Builder", http.MethodPut, true, nil, 429, nil, true, false},
		{"404", http.MethodGet, true, nil, 404, nil, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			retry, unknown := retryDecision(c.method, c.sandboxMode, c.transport, c.status, c.body)
			if retry != c.retry || unknown != c.unknown {
				t.Fatalf("retryDecision = (retry %v, unknown %v), want (retry %v, unknown %v)", retry, unknown, c.retry, c.unknown)
			}
		})
	}
}
