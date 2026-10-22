// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: which failures are safe to send again.
// This applies in every mode, not only in Canvas Builder. Resending a create
// after a lost answer has always risked making the thing twice; the Canvas
// Builder work is what brought it to light.

package client

import (
	"errors"
	"net"
	"net/http"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// retryDecision says what to do about one failed attempt.
//
//   - retry: send it again. Nothing was applied, or applying it twice is the
//     same as applying it once.
//   - unknown: stop, and tell the agent the outcome is unknown.
//   - neither: stop, and return the error as it is.
//
// The table, with sandboxMode meaning Canvas Builder:
//
//	| Failure                        | GET, HEAD, OPTIONS | POST, PATCH | PUT, DELETE (normal) | PUT, DELETE (Canvas Builder) |
//	| dial error: nothing was sent   | retry              | retry       | retry                | retry                        |
//	| any other transport error      | retry              | unknown     | retry                | unknown                      |
//	| 5xx                            | retry              | no retry    | retry                | no retry                     |
//	| 502 saying the outcome is unknown | unknown         | unknown     | unknown              | unknown                      |
//	| 429                            | retry              | retry       | retry                | retry                        |
//
// PUT and DELETE are idempotent at Canvas, so outside Canvas Builder they can
// go again. Inside it they can't: every attempt is its own line in the journal
// and, if it is held, its own question for the instructor.
func retryDecision(method string, sandboxMode bool, transportErr error, status int, body []byte) (retry, unknown bool) {
	if transportErr != nil {
		if isDialError(transportErr) {
			return true, false
		}
		if resendUnsafe(method, sandboxMode) {
			return false, true
		}
		return true, false
	}
	// Canvas Builder's own word on it outranks the status code: it sent the
	// change to Canvas and heard nothing back.
	if status == http.StatusBadGateway {
		if msg, ok := sandbox.CanvasBuilderMessage(body); ok && msg == sandbox.MsgUnknownOutcome {
			return false, true
		}
	}
	if status == http.StatusTooManyRequests {
		return true, false
	}
	if status >= 500 {
		return !resendUnsafe(method, sandboxMode), false
	}
	return false, false
}

// resendUnsafe reports whether sending this request a second time could apply
// the same change twice.
func resendUnsafe(method string, sandboxMode bool) bool {
	switch method {
	case http.MethodPost, http.MethodPatch:
		return true
	case http.MethodPut, http.MethodDelete:
		return sandboxMode
	default:
		return false
	}
}

// isDialError reports a failure to open the connection, where the request was
// never written and so cannot have been applied.
func isDialError(err error) bool {
	var op *net.OpError
	if errors.As(err, &op) {
		return op.Op == "dial"
	}
	return false
}
