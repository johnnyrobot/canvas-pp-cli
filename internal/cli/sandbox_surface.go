// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: what this CLI offers and refuses when it runs inside Canvas
// Builder.
//
// Two different jobs live here. Narrowing the surface is about discovery: an
// agent reads `which` and `--help` to decide what to try, so a command Canvas
// Builder would refuse should not be offered in the first place. Refusing is
// about the commands it asks for anyway, by name or from an older skill file.
// Both are needed — hiding is not a guardrail, and the gateway refuses on its
// own regardless of what this file does.

package cli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/johnnyrobot/canvas-pp-cli/internal/cliutil"
	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
	"github.com/spf13/cobra"
)

// studentDataCommands are the cross-resource commands built on student
// records: submissions, grades, enrollments. Canvas Builder's copy of Canvas
// holds no student records at all, so these have nothing to work with even
// before the gateway's read blocks are reached.
var studentDataCommands = []string{"roster", "at-risk", "to-grade", "standings", "audit-enrollments", "since"}

// tokenCommands change or clear the token Canvas Builder set up. An agent that
// could swap it would be choosing its own reach (ADR 0001).
var tokenCommands = []string{"auth set-token", "auth logout", "auth setup"}

// courseTraceableExact are the endpoint templates with no course id in them
// that the gateway can still trace to this course: the course list (filtered to
// the one course), a progress poll, and the three calendar and announcement
// endpoints that take a course as a parameter.
var courseTraceableExact = []string{
	"/api/v1/courses",
	"/api/v1/progress/{id}",
	"/api/v1/announcements",
	"/api/v1/calendar_events",
}

// courseTraceablePrefixes are the templates whose first id belongs to
// something the gateway can look up a course for: the course itself, a file, a
// folder, or a calendar event.
var courseTraceablePrefixes = []string{
	"/api/v1/courses/{",
	"/api/v1/files/{",
	"/api/v1/folders/{",
	"/api/v1/calendar_events/{",
}

// courseTraceable reports whether the gateway can tell which course an
// endpoint template belongs to. Anything else reaches past the one course
// Canvas Builder has open, so offering it would waste an agent's turn on a
// refusal it could not have predicted.
func courseTraceable(pathTemplate string) bool {
	for _, exact := range courseTraceableExact {
		if pathTemplate == exact {
			return true
		}
	}
	for _, prefix := range courseTraceablePrefixes {
		if strings.HasPrefix(pathTemplate, prefix) {
			return true
		}
	}
	return false
}

const sandboxRootShort = `Canvas CLI — the course Canvas Builder has open, from the command line`

const sandboxRootLong = `Canvas CLI — the course Canvas Builder has open, from the command line.

Everything here goes to Canvas Builder's own copy of Canvas, and only to the one
course it has open. A change the instructor has to approve is held until they
answer, and this command waits for them.

Add --agent to any command for JSON output and no prompts.
Run 'canvas-pp-cli doctor' to see which course this workspace is for and whether
the checks are set up.
See the bundled SKILL.md for what to run and in what order.`

// ensureSandbox decides whether this run is inside Canvas Builder and narrows
// the command surface if it is. It never fails: a damaged config is a
// config error the first command to load it reports, and failing here would
// turn `--help` into an error message.
//
// It applies the --home override itself, because the help path never reaches
// PersistentPreRunE and would otherwise read the developer's own config.
func (f *rootFlags) ensureSandbox(cmd *cobra.Command) {
	if f.sandboxChecked {
		return
	}
	f.sandboxChecked = true
	_, _ = cliutil.SetHomeOverride(f.homePath)

	// Required() is the bundled build's link-time stamp; the config key is how
	// a development build is told to behave the same way. Either one is enough
	// to narrow the surface — the settings are validated later, by config.Load.
	if !sandbox.Required() && !config.SandboxRequested(f.configPath) {
		return
	}
	f.sandboxActive = true
	if cfg, err := config.Load(f.configPath); err == nil {
		f.sandboxProfile = cfg.Profile
	}
	applySandboxSurface(cmd.Root())
}

// applySandboxSurface hides every command Canvas Builder would refuse and
// rewrites the root help, so what an agent reads is what it can run.
func applySandboxSurface(root *cobra.Command) {
	for _, route := range studentDataCommands {
		if cmd := findCommandByRoute(root, route); cmd != nil {
			cmd.Hidden = true
		}
	}
	for _, route := range tokenCommands {
		if cmd := findCommandByRoute(root, route); cmd != nil {
			cmd.Hidden = true
		}
	}
	hideUntraceable(root)
	root.Short = sandboxRootShort
	root.Long = sandboxRootLong
}

// hideUntraceable hides every command whose endpoint the gateway can't trace to
// this course. A command with no pp:path annotation reaches no Canvas endpoint
// — a resource group, or one of this CLI's own commands such as doctor — and
// stays visible.
func hideUntraceable(parent *cobra.Command) {
	for _, cmd := range parent.Commands() {
		if path, ok := cmd.Annotations["pp:path"]; ok && !courseTraceable(path) {
			cmd.Hidden = true
		}
		hideUntraceable(cmd)
	}
}

// findCommandByRoute walks a space-separated command route, such as
// "auth set-token", and returns the command or nil.
func findCommandByRoute(root *cobra.Command, route string) *cobra.Command {
	cur := root
	for _, name := range strings.Fields(route) {
		var next *cobra.Command
		for _, sub := range cur.Commands() {
			if sub.Name() == name {
				next = sub
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	if cur == root {
		return nil
	}
	return cur
}

// commandRoute is a command's path without the program name, so it can be
// compared against the routes above.
func commandRoute(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, " ")
}

// sandboxPreRun is Canvas Builder's half of PersistentPreRunE: the refusals
// that happen before a command sends anything, and the one flag whose default
// means something different here.
func (f *rootFlags) sandboxPreRun(cmd *cobra.Command) error {
	if !f.sandboxActive {
		return nil
	}
	// "auto" answers from the local store when the network fails. In Canvas
	// Builder that would answer a question about the course from a copy that
	// may predate the last change, without saying so loudly enough.
	if f.dataSource == "auto" {
		f.dataSource = "live"
	}
	msg := f.sandboxRefusal(cmd)
	if msg == "" {
		return nil
	}
	// status 0: this CLI stopped the command, so no gateway answered it.
	refused := refusedErr(msg, 0)
	writeRefusalEnvelope(f, refused)
	return refused
}

// sandboxRefusal returns the message Canvas Builder refuses this command with,
// or "" when it may run.
func (f *rootFlags) sandboxRefusal(cmd *cobra.Command) string {
	route := commandRoute(cmd)
	for _, name := range studentDataCommands {
		if route == name {
			return sandbox.MsgStudentData
		}
	}
	for _, name := range tokenCommands {
		if route == name {
			return sandbox.MsgTokenManaged
		}
	}
	if f.deliverSink.Scheme == "webhook" {
		return sandbox.MsgStaysLocal
	}
	return ""
}

// addSandboxReport is doctor's Canvas Builder section: which course this
// workspace is for, where the gateway is, and whether the checks are set up.
// In Canvas Builder it also replaces the auth hint, because the ordinary one
// tells an agent to export a token of its own (ADR 0001).
func (f *rootFlags) addSandboxReport(report map[string]any, cfg *config.Config) {
	if !f.sandboxActive || cfg == nil || cfg.Profile == nil {
		report["sandbox"] = "off"
		return
	}
	profile := cfg.Profile
	section := map[string]any{
		"mode":      "on",
		"gateway":   profile.Gateway.String(),
		"workspace": profile.Workspace,
		"checks":    checksState(profile.ChecksCommand),
	}
	// A damaged workspace.json is reported by leaving course_id out rather
	// than by failing: doctor is what an instructor runs to find out why.
	if ws, err := sandbox.ReadWorkspace(profile.Workspace); err == nil {
		section["course_id"] = ws.CourseID
	}
	report["sandbox"] = section
	report["auth_hint"] = sandbox.MsgTokenSetup
}

// renderSandboxReport prints doctor's Canvas Builder section without --json.
// An instructor reading doctor in a terminal is the person who most needs it:
// the JSON is for the agent, and "which course is this and are the checks set
// up" is the question doctor exists to answer.
func renderSandboxReport(w io.Writer, report map[string]any) {
	value, ok := report["sandbox"]
	if !ok {
		return
	}
	section, ok := value.(map[string]any)
	if !ok {
		// "off", or anything else a future caller stores here.
		_, _ = fmt.Fprintf(w, "  Canvas Builder: %v\n", value)
		return
	}
	_, _ = fmt.Fprintf(w, "  Canvas Builder: %v\n", section["mode"])
	// A fixed order, because a map's own order changes between runs and this
	// is read by a person.
	for _, key := range []string{"gateway", "workspace", "course_id", "checks"} {
		if v, ok := section[key]; ok {
			_, _ = fmt.Fprintf(w, "    %s: %v\n", key, v)
		}
	}
}

// checksState reports whether Canvas Builder's checks can be run from here.
// "ready" means the command's first element is on disk and executable; it says
// nothing about whether the checks pass.
func checksState(command []string) string {
	if len(command) == 0 {
		return "not set up"
	}
	if _, err := exec.LookPath(command[0]); err != nil {
		return "not set up"
	}
	return "ready"
}
