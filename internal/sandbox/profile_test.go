// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the Canvas Builder sandbox profile.

package sandbox

import (
	"testing"
	"time"
)

func settings(gateway string) Settings {
	return Settings{
		Gateway:         gateway,
		Workspace:       "/Users/someone/Canvas Builder/Course",
		ApprovalTimeout: "120s",
	}
}

// TestNew_GatewayMustBeLoopbackHTTP pins the Global Constraints rule: the only
// address Canvas Builder's CLI will treat as the gateway is http on 127.0.0.1
// with an explicit port and nothing else attached.
func TestNew_GatewayMustBeLoopbackHTTP(t *testing.T) {
	cases := []struct {
		gateway string
		ok      bool
	}{
		{"http://127.0.0.1:8181", true},
		{"http://127.0.0.1:8181/", true},
		{"https://127.0.0.1:8181", false},
		{"http://localhost:8181", false},
		{"http://127.0.0.1", false},
		{"http://u@127.0.0.1:8181", false},
		{"http://127.0.0.1:8181/api", false},
		{"http://127.0.0.1:8181?x=1", false},
		{"http://10.0.0.2:8181", false},
		{"", false},
		{"not a url", false},
	}
	for _, c := range cases {
		profile, err := New(settings(c.gateway))
		if c.ok && err != nil {
			t.Errorf("New(gateway=%q) = %v, want a profile", c.gateway, err)
			continue
		}
		if !c.ok {
			if err == nil {
				t.Errorf("New(gateway=%q) was accepted, want an error", c.gateway)
			}
			continue
		}
		if profile.HostPort() != "127.0.0.1:8181" {
			t.Errorf("HostPort() = %q, want 127.0.0.1:8181", profile.HostPort())
		}
	}
}

// TestNew_SettingsFaultsAreSettingsFaults covers everything the app writes that
// the instructor can't fix by hand: they get MsgSettings, not a technical error.
func TestNew_SettingsFaultsAreSettingsFaults(t *testing.T) {
	cases := map[string]Settings{
		"no workspace":           {Gateway: "http://127.0.0.1:8181", ApprovalTimeout: "120s"},
		"relative workspace":     {Gateway: "http://127.0.0.1:8181", Workspace: "Canvas Builder/Course", ApprovalTimeout: "120s"},
		"unparsable approval":    {Gateway: "http://127.0.0.1:8181", Workspace: "/w", ApprovalTimeout: "soon"},
		"zero approval":          {Gateway: "http://127.0.0.1:8181", Workspace: "/w", ApprovalTimeout: "0s"},
		"negative approval":      {Gateway: "http://127.0.0.1:8181", Workspace: "/w", ApprovalTimeout: "-5s"},
		"gateway not a url":      {Gateway: "://", Workspace: "/w", ApprovalTimeout: "120s"},
		"gateway somewhere else": {Gateway: "http://10.0.0.2:8181", Workspace: "/w", ApprovalTimeout: "120s"},
	}
	for name, s := range cases {
		_, err := New(s)
		if err == nil {
			t.Errorf("New(%s) was accepted, want an error", name)
			continue
		}
		if err.Error() != MsgSettings {
			t.Errorf("New(%s) = %q, want MsgSettings", name, err.Error())
		}
	}
}

// TestNew_EmptyApprovalTimeoutMeansTwoMinutes matches cb-gateway's own default.
func TestNew_EmptyApprovalTimeoutMeansTwoMinutes(t *testing.T) {
	s := settings("http://127.0.0.1:8181")
	s.ApprovalTimeout = ""
	profile, err := New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if profile.ApprovalTimeout != 2*time.Minute {
		t.Errorf("ApprovalTimeout = %v, want 2m", profile.ApprovalTimeout)
	}
}

func TestAllows(t *testing.T) {
	profile, err := New(settings("http://127.0.0.1:8181"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		url string
		ok  bool
	}{
		{"http://127.0.0.1:8181/api/v1/x", true},
		{"http://127.0.0.1:8181/api/v1/courses/7/pages?per_page=100", true},
		{"http://127.0.0.1:8182/api/v1/x", false},
		{"https://127.0.0.1:8181/api/v1/x", false},
		{"http://127.0.0.1:8181.evil.test/api/v1/x", false},
		{"http://127.0.0.1:8181@evil.test/api/v1/x", false},
		{"http://localhost:8181/api/v1/x", false},
		{"/api/v1/x", false},
		{"", false},
	}
	for _, c := range cases {
		if got := profile.Allows(c.url); got != c.ok {
			t.Errorf("Allows(%q) = %v, want %v", c.url, got, c.ok)
		}
	}
}

// TestHTTPTimeout guards the approval window: the gateway denies a held
// approval when the client hangs up, so the CLI must outwait it.
func TestHTTPTimeout(t *testing.T) {
	profile, err := New(settings("http://127.0.0.1:8181"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := profile.HTTPTimeout(60 * time.Second); got != 150*time.Second {
		t.Errorf("HTTPTimeout(60s) = %v, want 2m30s", got)
	}
	if got := profile.HTTPTimeout(300 * time.Second); got != 300*time.Second {
		t.Errorf("HTTPTimeout(300s) = %v, want 5m", got)
	}

	s := settings("http://127.0.0.1:8181")
	s.ApprovalTimeout = ""
	defaulted, err := New(s)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := defaulted.HTTPTimeout(0); got != 150*time.Second {
		t.Errorf("HTTPTimeout(0) with the default approval wait = %v, want 2m30s", got)
	}
}

func TestRequiredStamp(t *testing.T) {
	if Required() {
		t.Fatal("an unstamped build reports Required() = true")
	}
	restore := SetRequiredForTesting(true)
	if !Required() {
		t.Error("SetRequiredForTesting(true) did not take")
	}
	restore()
	if Required() {
		t.Error("restore did not put Required() back")
	}
}
