// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: run the whole CLI in a test and read back what an agent
// would see.

package cli

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
)

// runCLI runs the command tree the way main does and returns stdout, stderr and
// the exit code. os.Stdout and os.Stderr are swapped for pipes, because the
// error envelopes are written to os.Stdout directly rather than through
// cobra's writers. It replaces process-wide state, so tests that use it don't
// call t.Parallel().
func runCLI(t testing.TB, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	outR, outW := pipe(t)
	errR, errW := pipe(t)
	realOut, realErr := os.Stdout, os.Stderr
	beforeNoColor, beforeHuman := noColor, humanFriendly
	os.Stdout, os.Stderr = outW, errW
	// A t.Fatal inside the run would skip the restore below, leaving every
	// later test writing into a closed pipe.
	t.Cleanup(func() {
		os.Stdout, os.Stderr = realOut, realErr
		noColor, humanFriendly = beforeNoColor, beforeHuman
	})

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go drain(&wg, &outBuf, outR)
	go drain(&wg, &errBuf, errR)

	root := RootCmd()
	root.SetArgs(args)
	runErr := root.Execute()

	os.Stdout, os.Stderr = realOut, realErr
	noColor, humanFriendly = beforeNoColor, beforeHuman
	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	_ = outR.Close()
	_ = errR.Close()

	if runErr != nil {
		code = ExitCode(runErr)
	}
	return outBuf.String(), errBuf.String(), code
}

func pipe(t testing.TB) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("opening a pipe: %v", err)
	}
	return r, w
}

// drain reads until the write end closes, so a command that writes more than
// the pipe buffer holds doesn't block.
func drain(wg *sync.WaitGroup, into *bytes.Buffer, from io.Reader) {
	defer wg.Done()
	_, _ = io.Copy(into, from)
}
