// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for `recipes`: the six jobs Canvas Builder exists for,
// written out as the commands that do them.
//
// A recipe is instructions an agent follows without checking them first, so a
// wrong one is worse than no recipe at all — it spends the agent's turn on a
// command that doesn't exist, or on a write with no job label, which costs the
// instructor's review and Undo. These tests hold every recipe to the command
// tree as it actually is: every command resolves, every flag exists, every
// write is labelled, and every recipe ends by checking its own work.

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// nonGeneratedWrites are the writing commands with no pp:method annotation to
// read: upload is hand-authored and makes three requests, so the generator
// never wrote a method for it.
var nonGeneratedWrites = map[string]bool{"upload": true}

// stepWrites reports whether a step's command changes the course, and so must
// carry --job.
func stepWrites(root *cobra.Command, command string) bool {
	if nonGeneratedWrites[command] {
		return true
	}
	cmd := findCommandByRoute(root, command)
	if cmd == nil {
		return false
	}
	method, ok := cmd.Annotations["pp:method"]
	return ok && method != "GET"
}

// lineFlags returns the --flags written in an example line. Only whitespace-
// separated tokens count, so a flag name has to stand on its own.
func lineFlags(line string) []string {
	var out []string
	for _, tok := range strings.Fields(line) {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		name, _, _ := strings.Cut(tok, "=")
		out = append(out, strings.TrimPrefix(name, "--"))
	}
	return out
}

// TestRecipeStepsResolve is the contract that matters most: every command a
// recipe names exists, and every flag it passes exists on that command.
func TestRecipeStepsResolve(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	if len(recipes) == 0 {
		t.Fatal("no recipes are defined")
	}
	for _, r := range recipes {
		t.Run(r.Name, func(t *testing.T) {
			if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Summary) == "" {
				t.Errorf("recipe %q has no title or no summary", r.Name)
			}
			if len(r.Steps) < 2 {
				t.Errorf("recipe %q has %d steps; a job takes more than one", r.Name, len(r.Steps))
			}
			for i, s := range r.Steps {
				if strings.TrimSpace(s.Say) == "" {
					t.Errorf("step %d has no instruction", i+1)
				}
				if s.Command == "" {
					if s.Line != "" {
						t.Errorf("step %d has an example line but no command: %q", i+1, s.Line)
					}
					continue
				}
				found, remaining, err := root.Find(strings.Fields(s.Command))
				if err != nil || len(remaining) > 0 {
					t.Errorf("step %d command %q does not resolve (found=%v, remaining=%v, err=%v)",
						i+1, s.Command, found, remaining, err)
					continue
				}
				want := "canvas-pp-cli " + s.Command
				if !strings.HasPrefix(s.Line, want) {
					t.Errorf("step %d line %q does not start with %q", i+1, s.Line, want)
				}
				for _, name := range lineFlags(s.Line) {
					if found.Flags().Lookup(name) == nil && root.PersistentFlags().Lookup(name) == nil {
						t.Errorf("step %d passes --%s, which %q does not have", i+1, name, s.Command)
					}
				}
			}
		})
	}
}

// TestRecipesEndWithChecksAndLabelWrites holds the two house rules an agent
// can't be trusted to remember: a write carries a job label so the instructor
// can review and undo it as one change, and no job is called done until the
// checks have been run again.
func TestRecipesEndWithChecksAndLabelWrites(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	for _, r := range recipes {
		t.Run(r.Name, func(t *testing.T) {
			last := r.Steps[len(r.Steps)-1]
			if last.Command != "check" {
				t.Errorf("last step is %q; every recipe ends by checking its own work", last.Command)
			}
			if !strings.Contains(last.Line, "check all") {
				t.Errorf("last line is %q; the closing check is all four, not one of them", last.Line)
			}
			for i, s := range r.Steps {
				if s.Command == "" || !stepWrites(root, s.Command) {
					continue
				}
				if !strings.Contains(s.Line, "--job ") {
					t.Errorf("step %d writes with %q but carries no --job: %s", i+1, s.Command, s.Line)
				}
			}
		})
	}
}

// TestRecipesReadBeforeTheyWrite checks the ordering rule: the first command a
// recipe runs is a read. An agent that writes before it has looked is working
// from the request rather than from the course, which is how a date lands on
// an assignment that was already moved.
func TestRecipesReadBeforeTheyWrite(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	for _, r := range recipes {
		t.Run(r.Name, func(t *testing.T) {
			for _, s := range r.Steps {
				if s.Command == "" {
					continue
				}
				if stepWrites(root, s.Command) {
					t.Errorf("the first command is %q, a write; read the course first", s.Command)
				}
				return
			}
		})
	}
}

// TestRecipePlaceholdersAreClosed guards the one thing a reader can't recover
// from: a "[Check: …]" left half-written reads as part of the command, and an
// agent pastes it.
func TestRecipePlaceholdersAreClosed(t *testing.T) {
	for _, r := range recipes {
		for i, s := range r.Steps {
			for _, text := range []string{s.Say, s.Line} {
				// Each placeholder has to close before the next one opens.
				// Counting brackets across the whole string would not do:
				// an example line can hold a JSON list as well.
				parts := strings.Split(text, "[Check:")
				for _, after := range parts[1:] {
					if !strings.Contains(after, "]") {
						t.Errorf("%s step %d has an unclosed placeholder: %q", r.Name, i+1, text)
					}
				}
			}
		}
	}
}

// TestRecipesListsEveryRecipe: `recipes` with no name is how an agent finds
// out which jobs have a recipe at all.
func TestRecipesListsEveryRecipe(t *testing.T) {
	stdout, _, code := runCLI(t, "recipes", "--json")
	if code != 0 {
		t.Fatalf("recipes exited %d, want 0", code)
	}
	var out struct {
		Recipes []struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
		} `json:"recipes"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decoding the recipe list: %v\n%s", err, stdout)
	}
	if len(out.Recipes) != len(recipes) {
		t.Fatalf("listed %d recipes, want %d", len(out.Recipes), len(recipes))
	}
	for _, r := range out.Recipes {
		if r.Name == "" || r.Title == "" || r.Summary == "" {
			t.Errorf("listed recipe is missing a field: %+v", r)
		}
	}
}

// TestRecipeShowsSteps: the steps an agent follows, in order, with the say and
// the line for each.
func TestRecipeShowsSteps(t *testing.T) {
	stdout, _, code := runCLI(t, "recipes", "move-to-new-term", "--agent")
	if code != 0 {
		t.Fatalf("recipes move-to-new-term exited %d, want 0", code)
	}
	var out struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Steps   []struct {
			Say  string `json:"say"`
			Line string `json:"line"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decoding the recipe: %v\n%s", err, stdout)
	}
	if out.Name != "move-to-new-term" {
		t.Errorf("name = %q, want move-to-new-term", out.Name)
	}
	if out.Title == "" || out.Summary == "" {
		t.Errorf("title or summary missing: %+v", out)
	}
	if len(out.Steps) == 0 {
		t.Fatal("the recipe came back with no steps")
	}
	for i, s := range out.Steps {
		if s.Say == "" {
			t.Errorf("step %d has no instruction", i+1)
		}
	}
	if last := out.Steps[len(out.Steps)-1]; !strings.Contains(last.Line, "check all") {
		t.Errorf("last line is %q; the recipe should end by checking everything", last.Line)
	}
}

// TestRecipeUnknownNameIsUsage: a misspelled name is the agent's mistake, and
// exit 2 with the list of real names is what lets it fix the mistake itself.
func TestRecipeUnknownNameIsUsage(t *testing.T) {
	_, stderr, code := runCLI(t, "recipes", "move-the-dates", "--agent")
	if code != 2 {
		t.Fatalf("unknown recipe exited %d, want 2", code)
	}
	if !strings.Contains(stderr, "move-to-new-term") {
		t.Errorf("the error does not name the recipes that do exist: %s", stderr)
	}
}
