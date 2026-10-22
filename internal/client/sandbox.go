// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the client's half of Canvas Builder's boundary. In sandbox
// mode this client talks to the gateway and to nothing else — not to a base URL
// something changed after start-up, and not to a redirect's target.
//
// The gateway is the real boundary (SPEC §3). These checks are defense in
// depth: they stop the agent token from leaving the machine even if the gateway
// is pointed somewhere unexpected, and they fail before anything is dialed.

package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// applySandbox attaches the workspace's profile, if there is one, and narrows
// the HTTP client to match it. New calls this last, after CheckRedirect is in
// place, so the gateway rule runs ahead of the existing auth handling on a hop.
func (c *Client) applySandbox(timeout time.Duration) {
	if c.Config == nil || c.Config.Profile == nil {
		return
	}
	c.Sandbox = c.Config.Profile
	if c.HTTPClient == nil {
		return
	}
	// The instructor has the whole approval window to answer, and the gateway
	// denies a held approval the moment the client hangs up.
	c.HTTPClient.Timeout = c.Sandbox.HTTPTimeout(timeout)
	inner := c.HTTPClient.CheckRedirect
	c.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := c.CheckURL(req.URL.String()); err != nil {
			return &refusalError{err: err}
		}
		if inner != nil {
			return inner(req, via)
		}
		return nil
	}
}

// CheckURL reports whether the CLI may fetch a URL. Outside sandbox mode every
// URL is fine and this is nil. Inside it, anything that isn't the gateway is
// refused by name, and the name is only the scheme and host: a file URL's path
// and query carry a verifier, which is a credential.
func (c *Client) CheckURL(raw string) error {
	if c.Sandbox == nil {
		return nil
	}
	if !c.Sandbox.Allows(raw) {
		return sandbox.NotGateway(c.Sandbox.Gateway, raw)
	}
	return nil
}

// notices is where a message for the agent goes. Tests set c.Notices; a real
// run leaves it nil and gets stderr.
func (c *Client) notices() io.Writer {
	if c.Notices != nil {
		return c.Notices
	}
	return os.Stderr
}

// applyJobLabel puts the job label on a request, and says something once when a
// change goes out without one. The change still goes out (decision D2): a
// missing label costs the instructor a grouped Undo, and refusing the write
// would cost them the work.
func (c *Client) applyJobLabel(req *http.Request, method string) {
	if c.JobLabel != "" {
		req.Header.Set("X-Job-Label", c.JobLabel)
		return
	}
	if c.Sandbox == nil || !isWriteMethod(method) {
		return
	}
	c.noJobLabelOnce.Do(func() {
		_, _ = fmt.Fprintln(c.notices(), sandbox.MsgNoJobLabel)
	})
}

// isWriteMethod reports whether a method changes anything. It is deliberately
// wider than isMutatingVerb, which the generator uses for its own purposes:
// anything that isn't a read is treated as a change.
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// refusalError marks an error the CLI raised on purpose, so the retry loop can
// tell it apart from a connection that failed. A refusal is the same answer
// every time; retrying one only sends the request to the gateway again, and a
// write the gateway holds for approval would ask the instructor again on each
// attempt.
type refusalError struct{ err error }

func (e *refusalError) Error() string { return e.err.Error() }

func (e *refusalError) Unwrap() error { return e.err }

// refusal reports whether err is a refusal the CLI raised, and returns it
// unwrapped: the caller should see the message, not net/http's url.Error
// envelope around it.
func refusal(err error) (error, bool) {
	var r *refusalError
	if errors.As(err, &r) {
		return r.err, true
	}
	return nil, false
}
