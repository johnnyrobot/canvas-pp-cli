// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the build stamp that makes Canvas Builder's settings
// mandatory. The bundled build is stamped at link time, so a missing or
// non-Canvas-Builder config is an error and never a fallback.

package sandbox

import "sync"

// requiredStamp is set at link time with
//
//	-ldflags "-X github.com/johnnyrobot/canvas-pp-cli/internal/sandbox.requiredStamp=canvas-builder"
//
// (see the Makefile's build-sandbox target). An ordinary build leaves it empty
// and behaves exactly as it always has.
var requiredStamp string

const requiredStampValue = "canvas-builder"

// stampMu guards requiredStamp against SetRequiredForTesting, which tests call
// while other goroutines may be reading it. The linker writes the variable
// before main starts, so the lock costs nothing in a real run.
var stampMu sync.RWMutex

// Required reports whether this build must run inside Canvas Builder.
func Required() bool {
	stampMu.RLock()
	defer stampMu.RUnlock()
	return requiredStamp == requiredStampValue
}

// SetRequiredForTesting turns the stamp on or off and returns a function that
// puts it back. Tests use it to exercise the bundled build's behaviour without
// linking a second binary.
func SetRequiredForTesting(on bool) (restore func()) {
	stampMu.Lock()
	previous := requiredStamp
	if on {
		requiredStamp = requiredStampValue
	} else {
		requiredStamp = ""
	}
	stampMu.Unlock()
	return func() {
		stampMu.Lock()
		requiredStamp = previous
		stampMu.Unlock()
	}
}
