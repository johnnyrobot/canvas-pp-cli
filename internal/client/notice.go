// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: what the agent is told while a change is
// in flight, so a wait on a person doesn't read as a hung command.

package client

import (
	"fmt"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// defaultStillWaitingAfter is how long an answer can take before the CLI says
// something. Short enough that an agent hears it well inside its own patience,
// long enough that an ordinary edit says nothing at all.
const defaultStillWaitingAfter = 5 * time.Second

// notice writes one line where the agent will see it. It holds a lock because
// the still-waiting line comes from a timer goroutine while the request is in
// flight.
func (c *Client) notice(line string) {
	c.noticeMu.Lock()
	defer c.noticeMu.Unlock()
	_, _ = fmt.Fprintln(c.notices(), line)
}

// writeNotice is the notice for one request, if there is one to give.
type writeNotice struct {
	c        *Client
	approval bool
	said     bool
	timer    *time.Timer
	// wrote is closed once the still-waiting line has been written, so a
	// request that comes back just as the timer fires can wait for it.
	wrote chan struct{}
}

// newWriteNotice decides what this request should say about waiting. It returns
// nil outside Canvas Builder and for reads, and nil is a usable value: both
// methods below do nothing with it.
func (c *Client) newWriteNotice(method, path string, body any) *writeNotice {
	if c.Sandbox == nil || !isWriteMethod(method) {
		return nil
	}
	return &writeNotice{c: c, approval: sandbox.PredictApproval(method, path, body)}
}

// sending is called immediately before the request goes out.
func (n *writeNotice) sending() {
	if n == nil || n.said {
		return
	}
	if n.approval {
		// The instructor is being asked, and that is worth saying before the
		// wait rather than part-way through it.
		n.said = true
		n.c.notice(sandbox.ApprovalNotice(n.c.Sandbox.ApprovalTimeout))
		return
	}
	wrote := make(chan struct{})
	n.wrote = wrote
	n.timer = time.AfterFunc(n.c.waitNoticeAfter(), func() {
		n.c.notice(sandbox.StillWaitingNotice(n.c.Sandbox.ApprovalTimeout))
		close(wrote)
	})
}

// answered is called as soon as the request comes back, one way or the other.
func (n *writeNotice) answered() {
	if n == nil || n.timer == nil {
		return
	}
	if !n.timer.Stop() {
		// The timer had already fired, so the line is written or being
		// written. Waiting for it means the caller can read the notices as
		// soon as this returns.
		<-n.wrote
		n.said = true
	}
	n.timer = nil
}

func (c *Client) waitNoticeAfter() time.Duration {
	if c.stillWaitingAfter > 0 {
		return c.stillWaitingAfter
	}
	return defaultStillWaitingAfter
}
