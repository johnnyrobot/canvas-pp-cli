// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the course workspace folder.

package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorkspace(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "workspace.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing workspace.json: %v", err)
	}
	return dir
}

func TestReadWorkspace(t *testing.T) {
	t.Parallel()

	t.Run("a good file", func(t *testing.T) {
		t.Parallel()
		dir := writeWorkspace(t, `{"course_id":7,"closure_days":["2027-05-31","2027-06-04"],"opened_by":"Canvas Builder"}`)
		ws, err := ReadWorkspace(dir)
		if err != nil {
			t.Fatalf("ReadWorkspace = %v, want nil", err)
		}
		if ws.CourseID != 7 {
			t.Fatalf("CourseID = %d, want 7", ws.CourseID)
		}
		if len(ws.ClosureDays) != 2 || ws.ClosureDays[0] != "2027-05-31" || ws.ClosureDays[1] != "2027-06-04" {
			t.Fatalf("ClosureDays = %v, want the two days from the file", ws.ClosureDays)
		}
	})

	t.Run("no closure days yet", func(t *testing.T) {
		t.Parallel()
		dir := writeWorkspace(t, `{"course_id":7}`)
		ws, err := ReadWorkspace(dir)
		if err != nil {
			t.Fatalf("ReadWorkspace = %v, want nil", err)
		}
		if ws.ClosureDays != nil {
			t.Fatalf("ClosureDays = %v, want nil so the date check can say so", ws.ClosureDays)
		}
	})

	for _, tc := range []struct {
		name string
		body string
		// missing writes no file at all.
		missing bool
	}{
		{name: "a missing file", missing: true},
		{name: "bad JSON", body: `{"course_id":7`},
		{name: "a zero course", body: `{"course_id":0}`},
		{name: "no course at all", body: `{"closure_days":[]}`},
		{name: "a negative course", body: `{"course_id":-7}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if !tc.missing {
				dir = writeWorkspace(t, tc.body)
			}
			_, err := ReadWorkspace(dir)
			if err == nil {
				t.Fatalf("ReadWorkspace = nil, want an error")
			}
			if err.Error() != MsgWorkspace {
				t.Fatalf("error = %q, want MsgWorkspace", err)
			}
		})
	}
}

// TestInside is Review Focus 5: a path that escapes the course workspace.
func TestInside(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (workspace, outside string) {
		t.Helper()
		root := t.TempDir()
		workspace = filepath.Join(root, "ws")
		outside = filepath.Join(root, "elsewhere")
		for _, dir := range []string{filepath.Join(workspace, "materials"), outside} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		if err := os.WriteFile(filepath.Join(workspace, "materials", "diagram.png"), []byte("x"), 0o600); err != nil {
			t.Fatalf("writing diagram.png: %v", err)
		}
		if err := os.WriteFile(filepath.Join(outside, "secrets.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("writing secrets.txt: %v", err)
		}
		return workspace, outside
	}

	t.Run("a file in materials", func(t *testing.T) {
		t.Parallel()
		ws, _ := setup(t)
		got, err := Inside(ws, "materials/diagram.png")
		if err != nil {
			t.Fatalf("Inside = %v, want nil", err)
		}
		real, err := filepath.EvalSymlinks(filepath.Join(ws, "materials", "diagram.png"))
		if err != nil {
			t.Fatalf("resolving the file: %v", err)
		}
		if got != real {
			t.Fatalf("Inside = %q, want %q", got, real)
		}
	})

	t.Run("a file that isn't there yet", func(t *testing.T) {
		t.Parallel()
		ws, _ := setup(t)
		// A download's destination doesn't exist when it is checked.
		if _, err := Inside(ws, "materials/new.png"); err != nil {
			t.Fatalf("Inside = %v, want nil", err)
		}
	})

	t.Run("a dot-dot path", func(t *testing.T) {
		t.Parallel()
		ws, _ := setup(t)
		mustRefuse(t, ws, "materials/../../elsewhere/secrets.txt")
	})

	t.Run("an absolute path elsewhere", func(t *testing.T) {
		t.Parallel()
		ws, outside := setup(t)
		mustRefuse(t, ws, filepath.Join(outside, "secrets.txt"))
	})

	t.Run("a symlink pointing outside", func(t *testing.T) {
		t.Parallel()
		ws, outside := setup(t)
		link := filepath.Join(ws, "materials", "shortcut")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("this filesystem won't take a symlink: %v", err)
		}
		mustRefuse(t, ws, "materials/shortcut/secrets.txt")
	})

	t.Run("a blank path", func(t *testing.T) {
		t.Parallel()
		ws, _ := setup(t)
		mustRefuse(t, ws, "   ")
	})
}

func mustRefuse(t *testing.T, workspace, path string) {
	t.Helper()
	got, err := Inside(workspace, path)
	if err == nil {
		t.Fatalf("Inside = %q, want MsgOutsideWorkspace", got)
	}
	if err.Error() != MsgOutsideWorkspace {
		t.Fatalf("error = %q, want MsgOutsideWorkspace", err)
	}
}
