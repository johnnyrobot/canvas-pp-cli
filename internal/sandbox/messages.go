// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the words Canvas Builder and this CLI use when they stop an
// agent. Plain English, and they never say "sandbox" — the glossary's word for
// all of this is Canvas Builder.

package sandbox

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The gateway's own messages, copied verbatim from the Canvas Builder repo:
// gateway/internal/policy/policy.go (all but the last) and
// gateway/internal/proxy/errors.go (MsgNotRecorded). Each arrives as
// {"errors":[{"message":"…"}]}. The CLI recognises them by exact text, so
// TestGatewayMessagesMatchV2 checks these copies against that source.
const (
	// MsgBlocked is HTTP 403.
	MsgBlocked = "Canvas Builder doesn't let agents use this part of Canvas."
	// MsgUnknownToken is HTTP 403.
	MsgUnknownToken = "This isn't a key Canvas Builder gave out."
	// MsgWrongCourse is HTTP 403.
	MsgWrongCourse = "This workspace's agent can only work on its own course."
	// MsgQuarantine is HTTP 423.
	MsgQuarantine = "This course is still being checked for student material. Try again after the instructor finishes the check."
	// MsgDenied is HTTP 403.
	MsgDenied = "The instructor didn't approve this change."
	// MsgUnknownOutcome is HTTP 502, and the CLI turns it into exit code 8.
	MsgUnknownOutcome = "Canvas didn't answer, so this change may or may not have happened. Check the course before trying again."
	// MsgTooLarge is HTTP 413.
	MsgTooLarge = "This request is too large for Canvas Builder to check. Try a smaller upload."
	// MsgNotRecorded is HTTP 503 or 400.
	MsgNotRecorded = "Canvas Builder couldn't record this change, so it didn't send it to Canvas. Try again in a moment."
	// MsgTooManyChangesFmt is HTTP 403, for a write naming more things than
	// Canvas Builder keeps a before-copy of (today only a long
	// assignments bulk-update list). The gateway owns the number, so the CLI
	// recognises this one by the words either side of it rather than whole.
	MsgTooManyChangesFmt = "This change covers too many things at once. Canvas Builder keeps a copy of each one first, so the instructor can undo it. Send it as several changes of %d things or fewer."
)

// The CLI's own messages.
const (
	MsgSettings              = "Canvas Builder's settings for this workspace are missing or damaged, so the Canvas CLI won't run. Open the course from Canvas Builder again."
	MsgExtraHeaders          = "Canvas Builder's settings can't add extra headers to Canvas requests."
	MsgStudentData           = "This command works with student data, which Canvas Builder keeps out of its copy of Canvas."
	MsgTokenManaged          = "Canvas Builder manages this workspace's agent token, so it can't be changed here."
	MsgTokenSetup            = "Canvas Builder sets up this workspace's agent token. Open the course from Canvas Builder again."
	MsgStaysLocal            = "In Canvas Builder the Canvas CLI sends nothing anywhere except Canvas Builder, so this is turned off."
	MsgNoJobLabel            = `This change has no job label, so it can't be undone together with the rest of its job. Add --job "<name of the job>".`
	MsgBadJobLabel           = `A job label is a short name on one line, such as --job "Move to spring 2027".`
	MsgOutsideWorkspace      = "Canvas Builder only uploads and downloads files inside this course's workspace folder. Use a path inside it, such as materials/."
	MsgWorkspace             = "This course workspace's workspace.json is missing or damaged. Open the course from Canvas Builder again."
	MsgClosuresFromWorkspace = "In Canvas Builder, closure days come from the course workspace, so --closures isn't used."
	MsgChecksMissing         = "Canvas Builder's checks aren't set up for this workspace, so no checks ran."
	MsgDownloadShort         = "The download stopped before the whole file arrived, so nothing was saved."
	MsgNotAFileAddress       = "The address Canvas gave for this file isn't a file download, so the Canvas CLI won't fetch it."
	MsgTooManyPages          = "This course has more items of one kind than the CLI can read (10,000), so it stopped."
	MsgNoTimeZone            = "Canvas didn't say which time zone this course uses, so the checks can't judge its dates."
	// MsgChecksTimeout is the limit on one run of the checks. Ten minutes is
	// far longer than a course takes; past it, something is wrong with the
	// checks rather than slow about them.
	MsgChecksTimeout = "The checks took longer than 10 minutes and were stopped."
)

// gatewayMessages is every message the gateway writes with no number in it.
// CanvasBuilderMessage matches on this set, so a Canvas message that happens to
// arrive in the same envelope is left alone.
var gatewayMessages = []string{
	MsgBlocked, MsgUnknownToken, MsgWrongCourse, MsgQuarantine,
	MsgDenied, MsgUnknownOutcome, MsgTooLarge, MsgNotRecorded,
}

// tooManyChanges reports whether msg is MsgTooManyChangesFmt with a number in
// it. The limit is the gateway's to set and may change without this CLI
// changing, so the match is on the sentence around the number; anything else
// between those words, including nothing, is not the gateway's message.
func tooManyChanges(msg string) bool {
	before, after, found := strings.Cut(MsgTooManyChangesFmt, "%d")
	if !found {
		return false
	}
	rest, ok := strings.CutPrefix(msg, before)
	if !ok {
		return false
	}
	digits, ok := strings.CutSuffix(rest, after)
	if !ok || digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ApprovalNotice tells the agent its change is waiting on the instructor, and
// how long that can take, so it waits instead of retrying.
func ApprovalNotice(d time.Duration) string {
	return "Canvas Builder is asking the instructor to approve this change. This can take up to " + formatWait(d) + "."
}

// StillWaitingNotice is the same thing said again, part-way through the wait.
func StillWaitingNotice(d time.Duration) string {
	return "Still waiting for an answer. If Canvas Builder is asking the instructor about this change, this can take up to " + formatWait(d) + "."
}

// NewError wraps one of the messages above as an error. The messages are
// user-facing sentences, with a capital and a full stop, because the gateway and
// this CLI match them by exact text; passing one through here keeps staticcheck
// from reading it as a constant error string it should rewrite (ST1005).
func NewError(msg string) error { return errors.New(msg) }

// NotGateway says the CLI won't use an address that isn't Canvas Builder's. It
// names the scheme and host only: a file URL's path and query carry a verifier,
// which is a credential and never belongs in a message.
func NotGateway(gw *url.URL, raw string) error {
	return errors.New("The Canvas CLI here only talks to Canvas Builder at " + gatewayText(gw) + ", so it won't use " + originText(raw) + ".")
}

// ChecksFailed says the checks produced no report. The detail is the first
// line of what they said on stderr, passed through as it is: the checks
// explain their own failures better than a guess from out here would, and the
// lines after the first are for whoever is reading the checks' own output.
func ChecksFailed(detail string) error {
	if detail == "" {
		detail = "they produced no report."
	}
	return errors.New("The checks couldn't run: " + detail)
}

// NotTheCanvasRead says the next page of a list is somewhere other than the
// Canvas this CLI was pointed at. Like NotGateway it names the scheme and host
// only: the rest of a Canvas address can carry a verifier, which is a
// credential and never belongs in a message.
func NotTheCanvasRead(raw string) error {
	return errors.New("Canvas said the next page of this list is at " + originText(raw) + ", which isn't the Canvas this CLI reads, so it stopped.")
}

// CanvasBuilderMessage returns the first gateway message in an error envelope.
// It reports false for Canvas's own errors and for a body that isn't the
// envelope at all, such as an HTML error page from something in between.
func CanvasBuilderMessage(body []byte) (string, bool) {
	var envelope struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", false
	}
	for _, e := range envelope.Errors {
		if tooManyChanges(e.Message) {
			return e.Message, true
		}
		for _, known := range gatewayMessages {
			if e.Message == known {
				return e.Message, true
			}
		}
	}
	return "", false
}

// formatWait says a duration the way a person would: whole minutes as minutes,
// anything else as seconds.
func formatWait(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		return plural(int(d/time.Minute), "minute")
	}
	seconds := int(d.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return plural(seconds, "second")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

func gatewayText(gw *url.URL) string {
	if gw == nil {
		return "this course's workspace"
	}
	return (&url.URL{Scheme: gw.Scheme, Host: gw.Host}).String()
}

// originText reduces a URL to scheme://host. Anything that isn't a URL with
// both is described rather than echoed, so a malformed value can't put
// arbitrary text in a message.
func originText(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "that address"
	}
	return u.Scheme + "://" + u.Host
}
