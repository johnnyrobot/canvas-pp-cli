// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the Canvas Builder messages the CLI passes through
// and the ones it writes itself.

package sandbox

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatWait(t *testing.T) {
	cases := map[time.Duration]string{
		120 * time.Second: "2 minutes",
		60 * time.Second:  "1 minute",
		90 * time.Second:  "90 seconds",
		time.Second:       "1 second",
	}
	for d, want := range cases {
		if got := formatWait(d); got != want {
			t.Errorf("formatWait(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestApprovalNotices(t *testing.T) {
	notice := ApprovalNotice(120 * time.Second)
	if notice != "Canvas Builder is asking the instructor to approve this change. This can take up to 2 minutes." {
		t.Errorf("ApprovalNotice = %q", notice)
	}
	still := StillWaitingNotice(90 * time.Second)
	if !strings.HasSuffix(still, "this can take up to 90 seconds.") {
		t.Errorf("StillWaitingNotice = %q", still)
	}
}

// TestNotGateway names the address the CLI refused, and never its path or
// query, which can carry an upload verifier.
func TestNotGateway(t *testing.T) {
	gw, err := url.Parse("http://127.0.0.1:8181")
	if err != nil {
		t.Fatalf("parse gateway: %v", err)
	}
	err = NotGateway(gw, "https://college.instructure.com/files/9?verifier=secret-token")
	want := "The Canvas CLI here only talks to Canvas Builder at http://127.0.0.1:8181, so it won't use https://college.instructure.com."
	if err.Error() != want {
		t.Errorf("NotGateway = %q, want %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), "verifier") || strings.Contains(err.Error(), "secret-token") {
		t.Error("NotGateway leaked the query string")
	}
}

func TestCanvasBuilderMessage(t *testing.T) {
	for _, msg := range []string{
		MsgBlocked, MsgUnknownToken, MsgWrongCourse, MsgQuarantine,
		MsgDenied, MsgUnknownOutcome, MsgTooLarge, MsgNotRecorded,
	} {
		body := []byte(`{"errors":[{"message":` + quote(msg) + `}]}`)
		got, ok := CanvasBuilderMessage(body)
		if !ok || got != msg {
			t.Errorf("CanvasBuilderMessage(%q) = %q, %v; want the message back", msg, got, ok)
		}
	}

	notOurs := [][]byte{
		[]byte(`{"errors":[{"message":"The specified resource does not exist."}]}`),
		[]byte(`<html>502 Bad Gateway</html>`),
		[]byte(``),
		[]byte(`{"errors":[]}`),
		[]byte(`{"errors":"unauthorized"}`),
	}
	for _, body := range notOurs {
		if got, ok := CanvasBuilderMessage(body); ok {
			t.Errorf("CanvasBuilderMessage(%q) = %q, true; want false", body, got)
		}
	}
}

// TestCanvasBuilderMessage_FirstOfOurs: Canvas's own message may come first in
// the list; the gateway's is the one that matters.
func TestCanvasBuilderMessage_FirstOfOurs(t *testing.T) {
	body := []byte(`{"errors":[{"message":"The specified resource does not exist."},{"message":` + quote(MsgDenied) + `}]}`)
	got, ok := CanvasBuilderMessage(body)
	if !ok || got != MsgDenied {
		t.Errorf("CanvasBuilderMessage = %q, %v; want MsgDenied", got, ok)
	}
}

// TestGatewayMessagesMatchV2 keeps the copies honest: these eight strings are
// the gateway's own, and the CLI only recognises them by exact text.
func TestGatewayMessagesMatchV2(t *testing.T) {
	root := os.Getenv("CANVAS_BUILDER_V2")
	if root == "" {
		t.Skip("CANVAS_BUILDER_V2 is not set")
	}
	var sources string
	for _, rel := range []string{
		filepath.Join("gateway", "internal", "policy", "policy.go"),
		filepath.Join("gateway", "internal", "proxy", "errors.go"),
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		sources += string(data)
	}
	for _, msg := range []string{
		MsgBlocked, MsgUnknownToken, MsgWrongCourse, MsgQuarantine,
		MsgDenied, MsgUnknownOutcome, MsgTooLarge, MsgNotRecorded,
	} {
		if !strings.Contains(sources, msg) {
			t.Errorf("the gateway source no longer contains %q", msg)
		}
	}
}

// TestMessagesAvoidForbiddenWords: "sandbox" is not a word the glossary uses,
// and no output ever claims anything is "compliant".
func TestMessagesAvoidForbiddenWords(t *testing.T) {
	all := []string{
		MsgBlocked, MsgUnknownToken, MsgWrongCourse, MsgQuarantine, MsgDenied,
		MsgUnknownOutcome, MsgTooLarge, MsgNotRecorded, MsgSettings, MsgExtraHeaders,
		MsgStudentData, MsgTokenManaged, MsgStaysLocal, MsgNoJobLabel, MsgBadJobLabel,
		MsgOutsideWorkspace, MsgWorkspace, MsgNoClosureDays, MsgClosuresFromWorkspace,
		MsgChecksMissing, MsgDownloadShort,
		ApprovalNotice(2 * time.Minute), StillWaitingNotice(2 * time.Minute),
	}
	for _, msg := range all {
		lower := strings.ToLower(msg)
		for _, forbidden := range []string{"sandbox", "compliant"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("message %q contains %q", msg, forbidden)
			}
		}
	}
}

// quote renders s as a JSON string literal without pulling encoding/json into
// the test's expectations.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
