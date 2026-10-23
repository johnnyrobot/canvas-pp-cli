// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the download command. An agent remediating a document or
// reading a file it is about to replace needs the file itself, and this is the
// only way it gets one.
//
// It writes to a temporary name in the target folder and renames it once the
// whole file has arrived, so a download that stops partway leaves nothing
// behind. Half a file under the right name is worse than no file: the next
// command reads it and reports what isn't there.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

func newDownloadCmd(flags *rootFlags) *cobra.Command {
	var courseID int64
	var output string
	var force bool

	cmd := &cobra.Command{
		Use:   "download <file_id>",
		Short: "Save a course file to a local file",
		Long: `Save one of a course's files to a local file and print where it went.

--output takes either a folder, in which case the file keeps the name it has in
Canvas, or the name to save it under. In Canvas Builder it has to be inside the
course folder.`,
		Example: `  # Save a file under the name it has in Canvas
  canvas-pp-cli download 31 --output materials/

  # Save it under a name of your own, replacing what is there
  canvas-pp-cli download 31 --output materials/week3-diagram.png --force`,
		Args: cobra.ExactArgs(1),
		// The route this command reads from, so Canvas Builder keeps it
		// visible by the same course-traceable rule as every other command
		// that works on one course's content.
		Annotations: map[string]string{"pp:path": "/api/v1/courses/{course_id}/files/{file_id}"},
		RunE: func(cmd *cobra.Command, args []string) error {
			fileID, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || fileID <= 0 {
				return usageErr(fmt.Errorf("a file ID is a number, not %q", args[0]))
			}
			target := strings.TrimSpace(output)
			switch filepath.Clean(target) {
			case ".", "..":
				// "." and ".." name a folder by where the command happens to
				// be standing, which isn't a place to put a course's file.
				target = ""
			}
			if target == "" {
				return usageErr(errors.New("--output says where to save the file, such as --output materials/ or --output materials/diagram.png"))
			}
			// A trailing separator is how "this is a folder" is spelled, so a
			// folder that isn't there is reported as missing rather than
			// saved as a file of that name; without one, an existing folder is
			// still a folder, and anything else is the name to save under.
			asFolder := strings.HasSuffix(target, string(filepath.Separator))

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// c.Sandbox, not flags.sandboxProfile: the client's copy comes from
			// the config that was actually loaded, so it is never half there.
			profile := c.Sandbox

			if profile != nil {
				// The course folder is the whole of what this command may write
				// to. Where the file would land is checked before anything is
				// asked of Canvas, because a file that may not be saved
				// shouldn't be fetched. A relative path is read against the
				// course folder, which is also how the agent is told to write
				// one.
				resolved, err := sandbox.Inside(profile.Workspace, target)
				if err != nil {
					refused := refusedErr(err.Error(), 0)
					writeRefusalEnvelope(flags, refused)
					return refused
				}
				target = resolved

				if courseID == 0 {
					ws, err := sandbox.ReadWorkspace(profile.Workspace)
					if err != nil {
						return configErr(err)
					}
					courseID = ws.CourseID
				}
			} else if target, err = filepath.Abs(target); err != nil {
				return usageErr(fmt.Errorf("--output %q isn't a path this can save to: %w", output, err))
			}
			if courseID == 0 {
				return usageErr(errors.New("--course says which course the file is in"))
			}

			dir, name := target, ""
			if !asFolder {
				if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
					dir, name = filepath.Dir(target), filepath.Base(target)
					if !isFileName(name) {
						return usageErr(fmt.Errorf("--output %q doesn't name a folder or a file to save", output))
					}
				}
			}
			// The folder has to be there already. Creating one would mean a
			// typo in a path becomes a folder in the instructor's course
			// workspace, named after the typo.
			if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
				return usageErr(fmt.Errorf("there's no folder %s to save into", dir))
			}
			// When --output named the file, the refusal to overwrite comes
			// before the download rather than after it.
			if name != "" {
				if err := free(filepath.Join(dir, name), force); err != nil {
					return err
				}
			}

			if flags.dryRun {
				// The preview itself comes from the client, which prints the
				// request it would have sent. Only the first of the two is
				// shown, and no file is created.
				if _, _, err := c.DownloadFile(cmd.Context(), courseID, fileID, io.Discard); err != nil && !errors.Is(err, client.ErrDryRun) {
					return classifyAPIError(err, flags)
				}
				out, err := json.Marshal(map[string]any{
					"action":  "download",
					"dry_run": true,
					"status":  0,
					"success": false,
					"file_id": fileID,
					// Where it would have gone. When --output named a folder,
					// the name itself is in the answer a dry run doesn't get.
					"output": target,
				})
				if err != nil {
					return fmt.Errorf("building the result: %w", err)
				}
				return printOutputWithFlags(cmd.OutOrStdout(), out, flags)
			}

			tmp, err := os.CreateTemp(dir, ".canvas-download-*")
			if err != nil {
				return fmt.Errorf("creating the file to save into: %w", err)
			}
			tmpName := tmp.Name()
			saved := false
			defer func() {
				if !saved {
					_ = os.Remove(tmpName)
				}
			}()

			info, n, err := c.DownloadFile(cmd.Context(), courseID, fileID, tmp)
			if closeErr := tmp.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("finishing the saved file: %w", closeErr)
			}
			if err != nil {
				// A refusal this CLI made before fetching: the message is the
				// whole sentence and the status is 0, because nothing was sent
				// for Canvas Builder to answer.
				var refused *client.RefusedError
				if errors.As(err, &refused) {
					classified := refusedErr(refused.Error(), 0)
					writeRefusalEnvelope(flags, classified)
					return classified
				}
				return classifyAPIError(err, flags)
			}

			if name == "" {
				// The name comes from Canvas, which an agent can write to, so
				// it is a file name and nothing else: "../../evil.sh" saves as
				// "evil.sh" in the folder that was asked for.
				name = filepath.Base(filepath.Clean(strings.TrimSpace(info.DisplayName)))
				if !isFileName(name) {
					return usageErr(fmt.Errorf("this file's name in Canvas (%q) isn't a file name, so --output has to give one", info.DisplayName))
				}
				if err := free(filepath.Join(dir, name), force); err != nil {
					return err
				}
			}
			path := filepath.Join(dir, name)
			if err := os.Rename(tmpName, path); err != nil {
				return fmt.Errorf("saving the file as %s: %w", path, err)
			}
			saved = true

			out, err := json.Marshal(map[string]any{
				"action": "download",
				"file": map[string]any{
					"id":           info.ID,
					"display_name": info.DisplayName,
					"size":         info.Size,
					"content_type": info.ContentType,
					"folder_id":    info.FolderID,
				},
				"saved_to": path,
				"bytes":    n,
			})
			if err != nil {
				return fmt.Errorf("building the result: %w", err)
			}
			return printOutputWithFlags(cmd.OutOrStdout(), out, flags)
		},
	}

	cmd.Flags().Int64Var(&courseID, "course", 0, "Course ID (defaults to this course in Canvas Builder)")
	cmd.Flags().StringVar(&output, "output", "", "Folder to save the file in, or the name to save it under")
	cmd.Flags().BoolVar(&force, "force", false, "Replace a file that is already there")
	return cmd
}

// isFileName reports whether a name can be a file inside a folder. The three
// that can't are the ones that name a folder instead: nothing, here, and up.
func isFileName(name string) bool {
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return false
	}
	return true
}

// free reports that nothing is in the way, or says what is. Replacing a file
// is a decision the agent makes out loud, with --force.
func free(path string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Lstat(path); err == nil {
		return usageErr(fmt.Errorf("%s is already there; pass --force to replace it", path))
	}
	return nil
}
