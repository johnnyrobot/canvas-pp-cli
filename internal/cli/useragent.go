// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Survives `generate --force` regen-merge as a whole unit.

package cli

import "github.com/johnnyrobot/canvas-pp-cli/internal/client"

// Push the CLI's version into the client package so outbound requests identify
// the binary that actually sent them.
//
// The direction is forced: client cannot import cli, because cli already
// imports client. So the version — which lives here and is injected by ldflags
// at build time — has to be pushed down rather than pulled up. Doing it in
// init() rather than at each call site means every client, including ones built
// by code paths that never touch rootFlags.newClient, carries the right value.
func init() {
	client.SetUserAgentVersion(version)
}
