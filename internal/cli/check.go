// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the check command. It is how an agent finds out what is wrong
// with a course before it changes anything, and how it shows afterwards that
// the course is better than it was.
//
// This command does not check anything itself. It takes a snapshot of the
// course as it is right now, hands it to cb-checks, and passes the report back
// word for word. Keeping the judgement in one place matters: the instructor
// sees the same report in Canvas Builder that the agent saw, and a report this
// CLI had reshaped on the way past would be a second, quieter opinion nobody
// asked for.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/johnnyrobot/canvas-pp-cli/internal/coursesnapshot"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// checkNames are the checks cb-checks runs. "all" runs every one of them and
// reports them together.
var checkNames = []string{"a11y", "dates", "placeholders", "links"}

// checksTimeLimit is how long one run of the checks gets. A test shortens it;
// nothing else changes it.
var checksTimeLimit = 10 * time.Minute

// msgNoChecksCommand is for the plain CLI, where there is no Canvas Builder to
// have set the checks up. It names the setting rather than explaining the
// checks: whoever is running this outside Canvas Builder has cb-checks or
// doesn't.
const msgNoChecksCommand = "Set checks_command in config.toml to the cb-checks command."

func newCheckCmd(flags *rootFlags) *cobra.Command {
	var courseID int64
	var closures string

	cmd := &cobra.Command{
		Use:   "check <a11y|dates|placeholders|links|all>",
		Short: "Check a course and print the report",
		Long: `Read a course as it is now and check it.

  a11y          the content students read, against WCAG 2.2 AA
  dates         every due, available-from and available-until date
  placeholders  text left behind from a previous term
  links         links that go nowhere, or back to the old term's course
  all           all four, reported together

The report is cb-checks' own, printed as it comes. In Canvas Builder the course
and its closure days come from the course workspace, so neither has to be
given. A workspace whose closure days the instructor hasn't entered yet costs
one part of the date check: the report says that part wasn't checked, and the
rest of it ran.`,
		Example: `  # Check everything
  canvas-pp-cli check all

  # Check the dates, outside Canvas Builder
  canvas-pp-cli check dates --course 101 --closures 2027-05-06,2027-05-31`,
		Args: cobra.ExactArgs(1),
		// The course this reads. Canvas Builder keeps a command visible only
		// when it can tell which course the command is for.
		Annotations: map[string]string{"pp:path": "/api/v1/courses/{course_id}"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// The name is read before anything else: a misspelled check is a
			// typo, and a course shouldn't be read to find that out.
			name := strings.TrimSpace(args[0])
			if name != "all" && !slices.Contains(checkNames, name) {
				return usageErr(fmt.Errorf("there is no %q check, so choose %s or all",
					name, strings.Join(checkNames, ", ")))
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// c.Sandbox, not flags.sandboxProfile: the client's copy comes from
			// the config that was actually loaded, so it is never half there.
			profile := c.Sandbox

			var command []string
			var localHost string
			// days is nil when nobody has said what the course's closure days
			// are, which is not the same as saying it has none. cb-checks then
			// reports the closure-day rule as not checked and checks the rest
			// of the dates anyway (JR-59).
			var days *string
			if cmd.Flags().Changed("closures") {
				given := strings.TrimSpace(closures)
				days = &given
			}

			if profile != nil {
				if days != nil && *days != "" {
					return usageErr(sandbox.NewError(sandbox.MsgClosuresFromWorkspace))
				}
				command = profile.ChecksCommand
				if len(command) == 0 {
					return configErr(sandbox.NewError(sandbox.MsgChecksMissing))
				}
				localHost = profile.HostPort()

				ws, err := sandbox.ReadWorkspace(profile.Workspace)
				if err != nil {
					return configErr(err)
				}
				if courseID == 0 {
					courseID = ws.CourseID
				}
				// An empty list is an answer — this course closes on no days
				// — and a missing one isn't, so it is passed on as nothing
				// said rather than as none. Closure days are one rule of the
				// date check, and a course whose instructor hasn't entered
				// them yet is still worth checking: holding up the whole run
				// over them taught agents to skip the check instead.
				days = nil
				if ws.ClosureDays != nil {
					joined := strings.Join(ws.ClosureDays, ",")
					days = &joined
				}
			} else {
				command = c.Config.ChecksCommand
				if len(command) == 0 {
					return configErr(sandbox.NewError(msgNoChecksCommand))
				}
				base, err := url.Parse(c.BaseURL)
				if err != nil {
					return configErr(fmt.Errorf("the Canvas address this CLI is pointed at isn't an address: %w", err))
				}
				localHost = base.Host
			}
			if courseID == 0 {
				return usageErr(errors.New("--course says which course to check"))
			}

			if flags.dryRun {
				// The checks only read, so there is nothing for a dry run to
				// hold back. It says what it would have checked rather than
				// building a snapshot, because --dry-run makes every read
				// answer "not sent" and a snapshot of those is a snapshot of
				// nothing.
				out, err := json.Marshal(map[string]any{
					"action":    "check",
					"dry_run":   true,
					"check":     name,
					"course_id": courseID,
				})
				if err != nil {
					return fmt.Errorf("building the result: %w", err)
				}
				return printOutputWithFlags(cmd.OutOrStdout(), out, flags)
			}

			snapshot, err := coursesnapshot.Build(cmd.Context(), c, courseID)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			body, err := coursesnapshot.Marshal(snapshot)
			if err != nil {
				return fmt.Errorf("writing the course snapshot: %w", err)
			}

			report, err := runChecks(cmd.Context(), command, name, days, localHost, body)
			if err != nil {
				return err
			}
			// The report goes out as it came in, newline and all.
			_, err = cmd.OutOrStdout().Write(report)
			return err
		},
	}
	cmd.Flags().Int64Var(&courseID, "course", 0, "Course to check (the course workspace's own, in Canvas Builder)")
	cmd.Flags().StringVar(&closures, "closures", "", "Days the college is closed, as YYYY-MM-DD separated by commas (from the course workspace in Canvas Builder)")
	return cmd
}

// runChecks runs cb-checks over one course snapshot and returns its report.
// The snapshot goes in on stdin: it is the whole course's text, which is too
// much for a command line and has no business in a process listing.
//
// A nil days leaves --closures off altogether, which is how cb-checks is told
// that nobody has said; --closures with nothing after it says the course
// closes on no days. The two answers are different and only one of them is a
// reason for the date check to report anything.
func runChecks(ctx context.Context, command []string, name string, days *string, localHost string, snapshot []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, checksTimeLimit)
	defer cancel()

	args := make([]string, 0, len(command)+6)
	args = append(args, command[1:]...)
	args = append(args, name, "--snapshot", "-")
	if days != nil {
		args = append(args, "--closures", *days)
	}
	args = append(args, "--local-host", localHost)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	helper := exec.CommandContext(ctx, command[0], args...)
	helper.Stdin = bytes.NewReader(snapshot)
	helper.Stdout = stdout
	helper.Stderr = stderr
	// The limit has to hold even when checks_command is a launcher rather than
	// cb-checks itself. Killing the launcher doesn't kill what it started, and
	// a grandchild still holding the pipes would keep Run waiting past the
	// limit; WaitDelay closes them and returns instead.
	helper.WaitDelay = 5 * time.Second
	runErr := helper.Run()

	// The limit comes first: a stopped run reports being stopped, not the
	// "killed" its own exit looks like from here.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, sandbox.NewError(sandbox.MsgChecksTimeout)
	}
	if runErr != nil {
		return nil, sandbox.ChecksFailed(firstLine(stderr.String()))
	}
	// Checks that end well and print something that isn't a report have failed
	// in a way they didn't notice. Passing it on would hand the agent a report
	// it can't read, with nothing saying why.
	if !json.Valid(bytes.TrimSpace(stdout.Bytes())) {
		return nil, sandbox.ChecksFailed(firstLine(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// firstLine is the first line of what the checks said, with nothing around it.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
