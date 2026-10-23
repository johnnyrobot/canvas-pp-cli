// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: the upload command. An agent writing a page needs somewhere
// to point an <img> at, and this is it — one local file into the course, with
// the embed address the page should use handed back.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

func newUploadCmd(flags *rootFlags) *cobra.Command {
	var courseID int64
	var folder, displayName, onDuplicate string

	cmd := &cobra.Command{
		Use:   "upload <local-file>",
		Short: "Put a file into a course",
		Long: `Put one local file into a course's files and print the address a page
should embed it from.

In Canvas Builder the file has to be in the course folder, the course is the
one that folder belongs to, and the file goes to Canvas Builder and nowhere
else.`,
		Example: `  # Add an image to the course it belongs to
  canvas-pp-cli upload materials/diagram.png --job "Add the week 3 diagram"

  # Put it in a folder, under a different name
  canvas-pp-cli upload materials/diagram.png --folder "course files/images" --name "Week 3 diagram.png"

  # Replace a file of the same name instead of keeping both
  canvas-pp-cli upload materials/syllabus.pdf --on-duplicate overwrite`,
		Args: cobra.ExactArgs(1),
		// The route this command ends up at, so Canvas Builder keeps it
		// visible by the same course-traceable rule as every other command
		// that works on one course's content.
		Annotations: map[string]string{"pp:path": "/api/v1/courses/{course_id}/files"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.TrimSpace(onDuplicate) {
			case "", "rename", "overwrite":
			default:
				return usageErr(fmt.Errorf("--on-duplicate is \"rename\" or \"overwrite\", not %q", onDuplicate))
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// c.Sandbox, not flags.sandboxProfile: the client's copy comes from
			// the config that was actually loaded, so it is never half there.
			profile := c.Sandbox

			path := args[0]
			if profile != nil {
				// The course folder is the whole of what this command may read.
				// A path outside it, or a link inside it pointing out, is the
				// one way an agent could put someone else's file into Canvas.
				resolved, err := sandbox.Inside(profile.Workspace, path)
				if err != nil {
					refused := refusedErr(err.Error(), 0)
					writeRefusalEnvelope(flags, refused)
					return refused
				}
				path = resolved

				if courseID == 0 {
					ws, err := sandbox.ReadWorkspace(profile.Workspace)
					if err != nil {
						return configErr(err)
					}
					courseID = ws.CourseID
				}
			}
			if courseID == 0 {
				return usageErr(errors.New("--course says which course the file goes into"))
			}

			info, err := c.UploadFile(cmd.Context(), courseID, client.UploadRequest{
				Path:        path,
				Name:        displayName,
				Folder:      folder,
				OnDuplicate: onDuplicate,
			})
			if err != nil {
				// A refusal this CLI made before sending: the message is the
				// gateway's own sentence and the status is 0, because nothing
				// was sent for a gateway to answer.
				var refused *client.RefusedError
				if errors.As(err, &refused) {
					classified := refusedErr(refused.Error(), 0)
					writeRefusalEnvelope(flags, classified)
					return classified
				}
				return classifyAPIError(err, flags)
			}

			out, err := json.Marshal(map[string]any{
				"action":    "upload",
				"course_id": courseID,
				"file": map[string]any{
					"id":           info.ID,
					"display_name": info.DisplayName,
					"size":         info.Size,
					"content_type": info.ContentType,
					"folder_id":    info.FolderID,
				},
				// What a page's <img src> or <a href> should say. A file's own
				// url carries a verifier that expires, so it is no use in a
				// page; this address works for anyone who can see the course.
				"embed_src": fmt.Sprintf("/courses/%d/files/%d/preview", courseID, info.ID),
			})
			if err != nil {
				return fmt.Errorf("building the result: %w", err)
			}
			return printOutputWithFlags(cmd.OutOrStdout(), out, flags)
		},
	}

	cmd.Flags().Int64Var(&courseID, "course", 0, "Course ID (defaults to this course in Canvas Builder)")
	cmd.Flags().StringVar(&folder, "folder", "", "Course folder to put the file in, such as \"course files/images\"")
	cmd.Flags().StringVar(&displayName, "name", "", "Display name in Canvas (defaults to the file's own name)")
	cmd.Flags().StringVar(&onDuplicate, "on-duplicate", "rename", "What to do when a file of that name is already there: rename or overwrite")
	return cmd
}
