// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: --job, the name Canvas Builder groups a run's changes under so
// the instructor can review them and undo them together.

package cli

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// jobLabelMaxRunes keeps a label to something that reads as a line in Canvas
// Builder's review list. It counts runes, not bytes, so a label in Arabic gets
// the same room as one in English.
const jobLabelMaxRunes = 100

// validJobLabel holds a job label to one short line. A control character would
// break the header it travels in, and an empty label is the same as not passing
// the flag, which is worth saying rather than silently ignoring.
func validJobLabel(s string) error {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return sandbox.NewError(sandbox.MsgBadJobLabel)
	}
	if utf8.RuneCountInString(trimmed) > jobLabelMaxRunes {
		return sandbox.NewError(sandbox.MsgBadJobLabel)
	}
	for _, r := range trimmed {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return sandbox.NewError(sandbox.MsgBadJobLabel)
		}
	}
	return nil
}
