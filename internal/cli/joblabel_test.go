// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for --job.

package cli

import (
	"strings"
	"testing"
)

func TestValidJobLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		label string
		ok    bool
	}{
		{"plain", "Term move", true},
		{"accented", "Déplacer le cours", true},
		{"empty", "", false},
		{"blank", "   ", false},
		{"newline", "a\nb", false},
		{"tab", "a\tb", false},
		{"at the limit", strings.Repeat("é", 100), true},
		{"one rune over", strings.Repeat("é", 101), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validJobLabel(tc.label)
			if tc.ok && err != nil {
				t.Fatalf("validJobLabel(%q) = %v, want nil", tc.label, err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("validJobLabel(%q) = nil, want an error", tc.label)
				}
				if !strings.Contains(err.Error(), "A job label is a short name on one line") {
					t.Fatalf("error = %q, want the MsgBadJobLabel wording", err)
				}
			}
		})
	}
}
