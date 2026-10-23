// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored for Canvas Builder: the six jobs this CLI exists for, written
// out as the commands that do them, in the order that works.
//
// An agent can find any single command from `which` or `--help`. What it can't
// find there is the order: that the course dates move before the assignment
// dates, that a page body has to be read before it is written back because the
// write replaces all of it, that an upload hands back the address a page must
// embed rather than a path on disk, and that nothing is finished until the
// checks have been run again. Every recipe here was wrong in one of those ways
// the first time it was written, which is why they are tested against the
// command tree rather than kept in a document.
//
// Two rules run through all of them. Writes carry --job, because Canvas
// Builder groups a job's changes for the instructor to review and undo as one
// thing, and an unlabelled write is reviewed alone with no idea what it was
// for. And a fact the recipe can't know — a course id, a page's url, what the
// term is called — is left as a "[Check: …]" placeholder rather than invented,
// so an agent that pastes the line unchanged gets an obvious hole instead of a
// plausible wrong id.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// recipeStep is one instruction. Say is what to do and why; Line is the
// command to run, and is empty for a step that is thinking rather than running
// — deciding alt text, or writing the JSON a later step sends.
type recipeStep struct {
	Say string `json:"say"`
	// Command is the command path, such as "assignments bulk-update". It is
	// what the tests resolve against the live command tree, and it is left out
	// of the JSON because a reader wants the whole line, not its first half.
	Command string `json:"-"`
	Line    string `json:"line"`
}

type recipe struct {
	Name    string       `json:"name"`
	Title   string       `json:"title"`
	Summary string       `json:"summary"`
	Steps   []recipeStep `json:"steps"`
}

// recipes are the jobs from the product's own list, plus putting an image in a
// page, which is the small job that comes up inside most of the others.
var recipes = []recipe{
	{
		Name:    "move-to-new-term",
		Title:   "Move a course to a new term",
		Summary: "Shift every date in a course — the course's own, the assignments', the discussions', and the ones written into pages — then check what moved.",
		Steps: []recipeStep{
			{
				Say:     "Read the schedule as it stands. The report names every dated item and what its dates are now, so it is both the list you are about to move and your record of what it was.",
				Command: "check",
				Line:    "canvas-pp-cli check dates --agent",
			},
			{
				Say:     "Move the course's own start and end dates first. Canvas shows them on the syllabus and dates everything relative to them. Both are instants, in UTC, so a term that starts at 8am on 11 January in a Pacific course is 2027-01-11T16:00:00Z — work them out from the term the instructor asked for, and ask if they only gave you dates.",
				Command: "courses update",
				Line:    `canvas-pp-cli courses update [Check: course id] --course-start-at [Check: new start, UTC, like 2027-01-11T16:00:00Z] --course-end-at [Check: new end, UTC] --agent --job "Move to [Check: term name]"`,
			},
			{
				Say:     "Read the assignments with all of their dates. all_dates is the one that matters: an assignment given a different due date for one section carries an override, and the override's dates are in there and nowhere else. Every id in the list you build next has to be an id this course actually has — one unknown id fails the whole list, and nothing moves.",
				Command: "assignments index",
				Line:    "canvas-pp-cli assignments index [Check: course id] --include all_dates --agent --select id,name,due_at,unlock_at,lock_at,all_dates",
			},
			{
				Say: "Write the new dates to a file as a JSON list: one object per assignment, each with its id and an all_dates list. The assignment's own dates go in an entry marked base true. An override's dates go in its own entry, named by the override's id as you read it — an override you leave out keeps its old-term dates, and a section whose due date never moved is the failure nobody notices until a student asks. A date you leave out of an entry stays as it is; a date you send as null is cleared. Graded quizzes and graded discussions are assignments here, so their ids belong in this same list. An ungraded discussion has no assignment and gets its own step below.",
			},
			{
				Say:     "Send the list. One list is one change: the instructor approves it once and can undo it once, which is the whole reason not to send a term's worth of dates one assignment at a time. 500 assignments is the most a single list may name, and nothing splits a longer one for you: a longer list is refused, and the refusal says so and names the limit. Count first, and for a course with more than 500 write the lists yourself — at most 500 each, the same job label on every one, and the instructor approves each one.",
				Command: "assignments bulk-update",
				Line:    `canvas-pp-cli assignments bulk-update [Check: course id] --stdin --agent --job "Move to [Check: term name]" < new-dates.json`,
			},
			{
				Say:     "Move the ungraded discussions, which carry their own dates. Each is its own change, so give them the same job label and the instructor reviews them with the rest.",
				Command: "discussion-topics update",
				Line:    `canvas-pp-cli discussion-topics update [Check: course id] [Check: topic id] --delayed-post-at [Check: new post date, UTC] --lock-at [Check: new lock date, UTC] --agent --job "Move to [Check: term name]"`,
			},
			{
				Say:     "Read the pages. A term's dates are usually written into the text of a schedule page or a weekly overview as well, and no date field in Canvas moves those. The --select is not optional: --agent shortens lists, and a page body is one of the things it leaves out, so without it every page comes back with no text in it.",
				Command: "pages index",
				Line:    "canvas-pp-cli pages index [Check: course id] --include body --agent --select url,title,body",
			},
			{
				Say:     "Rewrite the dates you found in a page's text and leave the rest of the words alone. The body you send replaces the whole page. Write it to a file as {\"wiki_page\": {\"body\": \"...\"}} and send that file in on stdin: a page body is HTML full of quotation marks, and handing it to the shell as a quoted argument is how an agent silently mangles a page.",
				Command: "pages update",
				Line:    `canvas-pp-cli pages update [Check: course id] [Check: page url] --stdin --agent --job "Move to [Check: term name]" < page.json`,
			},
			{
				Say:     "Check everything. The dates check is the one that matters here: it reports a date that now lands on a closure day, or outside the term you just set. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
	{
		Name:    "accessibility-remediation",
		Title:   "Fix what the accessibility check reports",
		Summary: "Work through the accessibility issues in a course's pages, fixing the markup and leaving the instructor's words alone.",
		Steps: []recipeStep{
			{
				Say:     "Run the accessibility check and read what it reports. Each issue names the item it is in and the markup that caused it, so you do not have to guess which page to open.",
				Command: "check",
				Line:    "canvas-pp-cli check a11y --agent",
			},
			{
				Say:     "Read the page as it is now. The update replaces the whole body, so you have to start from the body Canvas holds and not from one you rebuilt out of the report.",
				Command: "pages show",
				Line:    "canvas-pp-cli pages show [Check: course id] [Check: page url] --agent",
			},
			{
				Say: "Fix the markup, not the writing. Give an image alt text that says what the image is there to tell the reader; make a heading a real heading instead of bold text; give a table a header row; replace \"click here\" with the name of the thing it links to. A decorative image takes an empty alt, and the check accepts that. Do not reword the instructor's sentences — that is their course, and a reworded paragraph is a change nobody asked for.",
			},
			{
				Say:     "Write the page back, body and fixes together. Put it in a file as {\"wiki_page\": {\"body\": \"...\"}} and send the file in on stdin — the body you are writing is markup full of quotation marks, and passing it as a quoted shell argument is how the fix itself breaks the page.",
				Command: "pages update",
				Line:    `canvas-pp-cli pages update [Check: course id] [Check: page url] --stdin --agent --job "Accessibility fixes for [Check: page title]" < page.json`,
			},
			{
				Say:     "Check everything, not just accessibility. Replacing a body can break a link or leave a placeholder behind, and those are different checks. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
	{
		Name:    "design-review",
		Title:   "Review how a course is put together",
		Summary: "Read a whole course and write the instructor a review of its structure. Changes nothing.",
		Steps: []recipeStep{
			{
				Say:     "Read the course and its syllabus. The syllabus is where a course says what it expects of a student, and the rest of the review is read against it.",
				Command: "courses show",
				Line:    "canvas-pp-cli courses show [Check: course id] --include syllabus_body --agent",
			},
			{
				Say:     "Read the modules with their items. The order a student meets the course in is the modules, not the assignment list, and a review that follows the assignment list reviews a course nobody takes.",
				Command: "modules index",
				Line:    "canvas-pp-cli modules index [Check: course id] --include items --agent",
			},
			{
				Say:     "Read the assignments: what is graded, what it is worth, and when it is due.",
				Command: "assignments index",
				Line:    "canvas-pp-cli assignments index [Check: course id] --agent",
			},
			{
				Say:     "Read the pages with their bodies. The pages are where the course does its teaching, and where most of what you will have to say about it is. The --select is what keeps the bodies: --agent shortens lists and drops them, so without it you would review a list of page titles.",
				Command: "pages index",
				Line:    "canvas-pp-cli pages index [Check: course id] --include body --agent --select url,title,body",
			},
			{
				Say: "Write the review. Say what you saw and where you saw it, and keep what you think separate from what the course says — the instructor can judge an opinion they can see is yours. This recipe changes nothing: if they want a change made, that is a new job with its own job label.",
			},
			{
				Say:     "Run the checks and include what they report. They catch the mechanical problems a reader skims past, which leaves your review free to be about the design. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
	{
		Name:    "ai-use-map",
		Title:   "Map where AI is allowed, and write it up",
		Summary: "Work out what each graded item says about using AI, then publish one page students can actually find.",
		Steps: []recipeStep{
			{
				Say:     "Read the assignments. The rule for a piece of work is normally written in its own description, which is why students miss it.",
				Command: "assignments index",
				Line:    "canvas-pp-cli assignments index [Check: course id] --agent",
			},
			{
				Say:     "Read the discussions, graded and not. A discussion is where \"in your own words\" usually turns up.",
				Command: "discussion-topics index",
				Line:    "canvas-pp-cli discussion-topics index [Check: course id] --agent",
			},
			{
				Say:     "Read the quizzes. A quiz is usually closed to AI, and saying so plainly beats leaving a student to assume it.",
				Command: "quizzes get-quizzes",
				Line:    "canvas-pp-cli quizzes get-quizzes [Check: course id] --agent",
			},
			{
				Say: "Build the map: for each graded item, what it says about AI and where you read it. Where an item says nothing, write that it says nothing rather than filling the gap with what the rest of the course implies. An unstated rule is the thing the instructor most needs to see.",
			},
			{
				Say:     "Publish the map as one page students can read. One page they can find beats the same rule written into twenty descriptions. Write the page to a file as {\"wiki_page\": {\"title\": \"AI Use Guide\", \"body\": \"...\", \"published\": true}} and send the file in on stdin — a table full of quotation marks does not survive being handed to the shell as a quoted argument.",
				Command: "pages create",
				Line:    "canvas-pp-cli pages create [Check: course id] --stdin --agent --job \"Add the AI use guide\" < page.json",
			},
			{
				Say:     "Check everything. A new page is new markup, and the accessibility check is what tells you whether the table you just wrote has a header row. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
	{
		Name:    "build-from-materials",
		Title:   "Build a week from a folder of materials",
		Summary: "Turn files in the course workspace into course files, a page that uses them, and a module that holds it.",
		Steps: []recipeStep{
			{
				Say:     "Read the modules first, so you add to the structure the instructor has instead of building a second one beside it. Match their naming while you are there.",
				Command: "modules index",
				Line:    "canvas-pp-cli modules index [Check: course id] --include items --agent",
			},
			{
				Say:     "Upload each file under the name it already has. Renaming breaks a link someone else wrote to it, and the display name is what a student sees anyway. The answer carries embed_src, the address a page has to use.",
				Command: "upload",
				Line:    `canvas-pp-cli upload materials/[Check: file name] --agent --job "Add [Check: what this week is called]"`,
			},
			{
				Say:     "Write the page that holds the week's work, embedding each file by the embed_src the upload gave back. Give every image its alt text as you write it: doing it now costs a sentence, and doing it after the check costs reading the page back. Put the page in a file as {\"wiki_page\": {\"title\": \"...\", \"body\": \"...\", \"published\": true}} and send the file in on stdin; embed markup is full of quotation marks and a quoted shell argument mangles it.",
				Command: "pages create",
				Line:    "canvas-pp-cli pages create [Check: course id] --stdin --agent --job \"Add [Check: what this week is called]\" < page.json",
			},
			{
				Say:     "Make the module last, once the pages and files it points at exist. The answer carries the new module's id, which the next step needs.",
				Command: "modules create",
				Line:    `canvas-pp-cli modules create [Check: course id] --module-name "[Check: what this week is called]" --agent --job "Add [Check: what this week is called]"`,
			},
			{
				Say:     "Put the page in the module. Creating a module does not put anything in it, and a module with nothing in it is the one mistake here a student sees: the week appears in the course and opens empty. The command for this is `modules create-2` — Canvas calls it \"create a module item\", and the generated name is unhelpful. A file goes in the same way, with --module-item-type File and --module-item-content-id.",
				Command: "modules create-2",
				Line:    `canvas-pp-cli modules create-2 [Check: course id] [Check: new module id] --module-item-type Page --module-item-page-url [Check: page url] --module-item-title "[Check: what this week is called]" --agent --job "Add [Check: what this week is called]"`,
			},
			{
				Say:     "Check what you built. The links check is what tells you an embed address is wrong; the accessibility check is what tells you an image went up without alt text. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
	{
		Name:    "insert-image",
		Title:   "Put an image into a page",
		Summary: "Upload an image from the course workspace and embed it in a page, with alt text, so both checks pass.",
		Steps: []recipeStep{
			{
				Say:     "Read the page first. The update replaces the whole body, so you need the body as it is now — not the paragraph you are adding to.",
				Command: "pages show",
				Line:    "canvas-pp-cli pages show [Check: course id] [Check: page url] --agent",
			},
			{
				Say:     "Upload the image. The answer carries embed_src, which is the address the page has to use; a path to the file on your disk is not reachable from Canvas, and a page that holds one shows a broken image to every student.",
				Command: "upload",
				Line:    `canvas-pp-cli upload materials/[Check: file name] --folder "course files/images" --agent --job "Add [Check: what the image shows]"`,
			},
			{
				Say: "Write the alt text before you write the tag. Say what the image is there to tell the reader, not what it is a picture of: \"the three stages of the water cycle\" rather than \"a diagram\". If the image is decorative, give it an empty alt and the check accepts it.",
			},
			{
				Say:     "Put the body back with the image in it: an img tag whose src is the embed_src from the upload and whose alt is the text you wrote. Write the whole body to a file as {\"wiki_page\": {\"body\": \"...\"}} and send the file in on stdin — an img tag is nothing but quoted attributes, and passing it as a quoted shell argument is how the tag arrives broken.",
				Command: "pages update",
				Line:    `canvas-pp-cli pages update [Check: course id] [Check: page url] --stdin --agent --job "Add [Check: what the image shows]" < page.json`,
			},
			{
				Say:     "Check everything. The accessibility check reads the alt text and the links check follows the src, so between them they tell you whether this worked. If the course's closure days have not been entered yet, the report says the closure-day part of the date check was not checked and checks everything else; pass that line on to the instructor rather than reporting the dates as clean.",
				Command: "check",
				Line:    "canvas-pp-cli check all --agent",
			},
		},
	},
}

// recipeNames lists the recipe names in the order they are declared, for the
// error an agent sees when it asks for one that doesn't exist.
func recipeNames() []string {
	out := make([]string, 0, len(recipes))
	for _, r := range recipes {
		out = append(out, r.Name)
	}
	return out
}

const recipesLong = `A recipe is the order the commands go in.

Any single command is findable from 'which' or '--help'. What isn't findable
there is the sequence: that a course's own dates move before its assignments',
that a page body has to be read before it is written back because the write
replaces all of it, that an upload hands back the address a page must embed
rather than a path on disk, and that no job is done until the checks have been
run again.

Where a recipe can't know something — a course id, a page's url, what the term
is called — it leaves a [Check: ...] placeholder. Fill those in; don't guess
them.`

// newRecipesCmd builds `recipes` and one subcommand per recipe.
//
// Each recipe is a real subcommand rather than an argument to one, because the
// capability index is checked against the command tree: an entry that doesn't
// resolve to a command fails TestWhichIndex_ExistsAndIsWellFormed, which is
// what keeps `which` from ever sending an agent somewhere that isn't there.
func newRecipesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipes",
		Short: "The order the commands go in, for each of the jobs this CLI is for",
		Long:  recipesLong,
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:typed-exit-codes": "0,2",
		},
		Example: `  # Which jobs have a recipe
  canvas-pp-cli recipes

  # The steps for one job
  canvas-pp-cli recipes move-to-new-term --agent`,
		// A name that isn't a recipe reaches here as a leftover argument.
		// Answering with the names that do exist is what lets an agent fix its
		// own mistake instead of asking.
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return usageErr(fmt.Errorf("there is no %q recipe, so choose %s",
				args[0], strings.Join(recipeNames(), ", ")))
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderRecipeList(cmd, flags)
		},
	}
	for _, r := range recipes {
		cmd.AddCommand(newRecipeCmd(flags, r))
	}
	return cmd
}

func newRecipeCmd(flags *rootFlags, r recipe) *cobra.Command {
	return &cobra.Command{
		Use:   r.Name,
		Short: r.Title,
		Long:  r.Title + ".\n\n" + r.Summary,
		Args:  cobra.NoArgs,
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderRecipe(cmd, flags, r)
		},
	}
}

// recipeSummary is a recipe without its steps, for the list.
type recipeSummary struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func renderRecipeList(cmd *cobra.Command, flags *rootFlags) error {
	w := cmd.OutOrStdout()
	if flags.asJSON || !isTerminal(w) {
		out := make([]recipeSummary, 0, len(recipes))
		for _, r := range recipes {
			out = append(out, recipeSummary{Name: r.Name, Title: r.Title, Summary: r.Summary})
		}
		// Wrapped in an object for the same reason `which` wraps its matches:
		// --compact filters a top-level array through an allowlist, which
		// would strip every field here.
		return printJSONFiltered(w, map[string]any{"recipes": out}, flags)
	}
	for _, r := range recipes {
		_, _ = fmt.Fprintf(w, "%-28s  %s\n", r.Name, r.Title)
	}
	_, _ = fmt.Fprint(w, "\nRun 'canvas-pp-cli recipes <name>' for the steps.\n")
	return nil
}

func renderRecipe(cmd *cobra.Command, flags *rootFlags, r recipe) error {
	w := cmd.OutOrStdout()
	if flags.asJSON || !isTerminal(w) {
		return printJSONFiltered(w, r, flags)
	}
	_, _ = fmt.Fprintf(w, "%s\n\n%s\n\n", r.Title, r.Summary)
	for i, s := range r.Steps {
		_, _ = fmt.Fprintf(w, "%d. %s\n", i+1, s.Say)
		if s.Line != "" {
			_, _ = fmt.Fprintf(w, "\n   %s\n", s.Line)
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}
