// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Survives `generate --force` regen-merge as a whole unit.

package client

import (
	"strings"
	"testing"
)

// TestUserAgent_ReflectsInjectedVersion pins the rule that the outbound
// User-Agent names the version that actually shipped. It used to be the string
// literal "canvas-pp-cli/0.1.0", so every release identified itself to the
// Canvas API as 0.1.0 no matter what was built.
func TestUserAgent_ReflectsInjectedVersion(t *testing.T) {
	orig := userAgent.Load()
	t.Cleanup(func() { userAgent.Store(orig) })

	SetUserAgentVersion("9.9.9")
	got := UserAgent()

	if got != "canvas-pp-cli/9.9.9" {
		t.Errorf("UserAgent() = %q, want canvas-pp-cli/9.9.9", got)
	}
	if strings.Contains(got, "0.1.0") {
		t.Errorf("UserAgent() still carries the hardcoded version: %q", got)
	}
}

// TestUserAgent_EmptyVersionKeepsDefault guards the trailing-slash case: an
// unset version must not produce "canvas-pp-cli/".
func TestUserAgent_EmptyVersionKeepsDefault(t *testing.T) {
	orig := userAgent.Load()
	t.Cleanup(func() { userAgent.Store(orig) })

	SetUserAgentVersion("1.2.3")
	SetUserAgentVersion("")

	if got := UserAgent(); got != "canvas-pp-cli/1.2.3" {
		t.Errorf("empty version overwrote a good value: %q", got)
	}
	if strings.HasSuffix(UserAgent(), "/") {
		t.Errorf("UserAgent() ends in a bare slash: %q", UserAgent())
	}
}
