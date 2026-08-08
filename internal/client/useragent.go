// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Survives `generate --force` regen-merge as a whole unit.

package client

import "sync/atomic"

// userAgent is the value sent on every outbound request that does not already
// carry one. It starts at a neutral default because this package cannot import
// internal/cli — cli imports client, so the version has to be pushed down
// rather than pulled up. SetUserAgentVersion does that push during root
// command setup.
var userAgent atomic.Pointer[string]

// UserAgent returns the current outbound User-Agent.
func UserAgent() string {
	if v := userAgent.Load(); v != nil {
		return *v
	}
	return "canvas-pp-cli/dev"
}

// SetUserAgentVersion sets the outbound User-Agent to "canvas-pp-cli/<version>".
// Called once from root command setup with the ldflags-injected version, so the
// value the API sees matches the binary that sent the request. An empty version
// leaves the default in place rather than emitting a trailing slash.
func SetUserAgentVersion(version string) {
	if version == "" {
		return
	}
	ua := "canvas-pp-cli/" + version
	userAgent.Store(&ua)
}
