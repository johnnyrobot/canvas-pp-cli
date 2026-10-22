// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: the error for a change that may or may not
// have happened.

package client

import "github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"

// OutcomeUnknownError says a write went out and no answer came back, so nobody
// knows whether Canvas applied it. Sending it again could make a second copy of
// the thing, or ask the instructor a second time, so the CLI stops here and
// tells the agent to look at the course.
//
// Error() is only the message, with no method, path or cause in it, because the
// agent reads this sentence and the gateway sends the same one. Method and Path
// are there for a caller that logs the failure; Cause carries what actually
// went wrong.
type OutcomeUnknownError struct {
	Method string
	Path   string
	Cause  error
}

func (e *OutcomeUnknownError) Error() string { return sandbox.MsgUnknownOutcome }

func (e *OutcomeUnknownError) Unwrap() error { return e.Cause }
