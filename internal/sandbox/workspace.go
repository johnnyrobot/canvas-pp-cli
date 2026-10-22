// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the course workspace folder — which course it belongs to,
// which days the course doesn't meet, and whether a path is inside it.

package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is what Canvas Builder writes into <workspace>/workspace.json.
// Other fields may be there; they belong to Canvas Builder, and this ignores
// them.
type Workspace struct {
	// CourseID is the one course this workspace's agent may work on.
	CourseID int64 `json:"course_id"`
	// ClosureDays are the days the course doesn't meet, as YYYY-MM-DD. The
	// instructor enters them per course, so a workspace can have none yet.
	ClosureDays []string `json:"closure_days"`
}

// ReadWorkspace reads <dir>/workspace.json. Every fault is MsgWorkspace: a
// missing file, JSON that won't parse, and a missing or zero course_id are all
// the same thing to the agent, and the one thing that helps is opening the
// course from Canvas Builder again. A missing closure_days is not a fault; it
// leaves ClosureDays nil, which the date check reports as MsgNoClosureDays.
func ReadWorkspace(dir string) (Workspace, error) {
	body, err := os.ReadFile(filepath.Join(dir, "workspace.json"))
	if err != nil {
		return Workspace{}, NewError(MsgWorkspace)
	}
	var ws Workspace
	if err := json.Unmarshal(body, &ws); err != nil {
		return Workspace{}, NewError(MsgWorkspace)
	}
	if ws.CourseID <= 0 {
		return Workspace{}, NewError(MsgWorkspace)
	}
	return ws, nil
}

// Inside resolves path against the workspace folder and returns it as a clean
// absolute path, or MsgOutsideWorkspace. Symlinks are resolved on both sides
// before they are compared, so a link in materials/ pointing at the home
// directory is outside the workspace however it is spelled. A relative path is
// taken as relative to the workspace, because that is how the agent is told to
// write them ("materials/diagram.png").
func Inside(workspace, path string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		return "", NewError(MsgOutsideWorkspace)
	}
	root, err = resolve(root)
	if err != nil {
		return "", NewError(MsgOutsideWorkspace)
	}

	target := strings.TrimSpace(path)
	if target == "" {
		return "", NewError(MsgOutsideWorkspace)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = resolve(target)
	if err != nil {
		return "", NewError(MsgOutsideWorkspace)
	}
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", NewError(MsgOutsideWorkspace)
	}
	return target, nil
}

// resolve follows symlinks as far as the path exists. A file that isn't there
// yet — a download's destination — is resolved through its nearest existing
// parent instead, so the link that matters is still followed and the name that
// doesn't exist is kept.
func resolve(path string) (string, error) {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real, nil
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", NewError(MsgOutsideWorkspace)
	}
	real, err := resolve(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(real, filepath.Base(path)), nil
}
