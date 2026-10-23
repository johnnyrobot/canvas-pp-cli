// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Reading a course out of Canvas into a snapshot.
//
// Everything here is read from inside the one course: seven lists, all under
// /api/v1/courses/<id>. That is what Canvas Builder allows, and asking for
// anything else would only spend an agent's turn being refused.

package coursesnapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// standIn matches the assignment the quiz builder leaves behind in place of a
// New Quiz, whose questions nothing here can read.
var standIn = regexp.MustCompile(`^New Quiz: .+ \(questions kept unchanged\)$`)

// Build reads a whole course and returns the snapshot the checks run on.
func Build(ctx context.Context, c *client.Client, courseID int64) (Snapshot, error) {
	in := fmt.Sprintf("/api/v1/courses/%d", courseID)

	// The course itself, with the syllabus: Canvas leaves syllabus_body out
	// unless it is asked for by name.
	raw, err := c.GetNoCache(ctx, in, map[string]string{"include[]": "syllabus_body"})
	if err != nil {
		return Snapshot{}, err
	}
	var course struct {
		ID           int64   `json:"id"`
		Name         string  `json:"name"`
		StartAt      *string `json:"start_at"`
		EndAt        *string `json:"end_at"`
		TimeZone     string  `json:"time_zone"`
		SyllabusBody *string `json:"syllabus_body"`
	}
	if err := json.Unmarshal(raw, &course); err != nil {
		return Snapshot{}, fmt.Errorf("reading what Canvas said about this course: %w", err)
	}
	// Every date in a course is written in the course's own time zone, so a
	// course that doesn't name one can't be checked at all.
	if course.TimeZone == "" {
		return Snapshot{}, sandbox.NewError(sandbox.MsgNoTimeZone)
	}

	s := Snapshot{
		Course: Course{
			ID:       course.ID,
			Name:     course.Name,
			StartAt:  course.StartAt,
			EndAt:    course.EndAt,
			TimeZone: course.TimeZone,
		},
		SyllabusBody:     course.SyllabusBody,
		Pages:            []Page{},
		Assignments:      []Assignment{},
		Quizzes:          []Quiz{},
		DiscussionTopics: []Discussion{},
		Announcements:    []Announcement{},
		Files:            []File{},
	}

	// Pages. Canvas lists them without their bodies unless asked.
	pages, err := readList[struct {
		URL       string  `json:"url"`
		Title     string  `json:"title"`
		Body      *string `json:"body"`
		Published bool    `json:"published"`
	}](ctx, c, in+"/pages", map[string]string{"include[]": "body"})
	if err != nil {
		return Snapshot{}, err
	}
	for _, p := range pages {
		s.Pages = append(s.Pages, Page{URL: p.URL, Title: p.Title, Body: p.Body, Published: p.Published})
	}

	// Assignments. Canvas lists a classic quiz and a graded discussion here as
	// well as in their own lists, so those two are left out: the checks would
	// otherwise report the same item twice. The unfiltered list is kept,
	// because a graded discussion's dates live on the assignment behind it.
	assignments, err := readList[struct {
		ID              int64    `json:"id"`
		Name            string   `json:"name"`
		Description     *string  `json:"description"`
		DueAt           *string  `json:"due_at"`
		UnlockAt        *string  `json:"unlock_at"`
		LockAt          *string  `json:"lock_at"`
		SubmissionTypes []string `json:"submission_types"`
	}](ctx, c, in+"/assignments", nil)
	if err != nil {
		return Snapshot{}, err
	}
	byID := make(map[int64]int, len(assignments))
	for i, a := range assignments {
		byID[a.ID] = i
		if hasAny(a.SubmissionTypes, "online_quiz", "discussion_topic") {
			continue
		}
		s.Assignments = append(s.Assignments, Assignment{
			ID:             a.ID,
			Name:           a.Name,
			Description:    a.Description,
			DueAt:          a.DueAt,
			UnlockAt:       a.UnlockAt,
			LockAt:         a.LockAt,
			NewQuizStandin: standIn.MatchString(a.Name),
		})
	}

	// Classic quizzes.
	quizzes, err := readList[struct {
		ID          int64   `json:"id"`
		Title       string  `json:"title"`
		Description *string `json:"description"`
		DueAt       *string `json:"due_at"`
		UnlockAt    *string `json:"unlock_at"`
		LockAt      *string `json:"lock_at"`
	}](ctx, c, in+"/quizzes", nil)
	if err != nil {
		return Snapshot{}, err
	}
	for _, q := range quizzes {
		s.Quizzes = append(s.Quizzes, Quiz{
			ID: q.ID, Title: q.Title, Description: q.Description,
			DueAt: q.DueAt, UnlockAt: q.UnlockAt, LockAt: q.LockAt,
		})
	}

	// Discussion topics. An ungraded topic has no due date; what Canvas calls
	// delayed_post_at is when it opens.
	type topic struct {
		ID            int64   `json:"id"`
		Title         string  `json:"title"`
		Message       *string `json:"message"`
		AssignmentID  *int64  `json:"assignment_id"`
		DelayedPostAt *string `json:"delayed_post_at"`
		LockAt        *string `json:"lock_at"`
	}
	topics, err := readList[topic](ctx, c, in+"/discussion_topics", nil)
	if err != nil {
		return Snapshot{}, err
	}
	for _, t := range topics {
		d := Discussion{ID: t.ID, Title: t.Title, Message: t.Message}
		if t.AssignmentID != nil {
			d.Graded = true
			if i, ok := byID[*t.AssignmentID]; ok {
				d.DueAt = assignments[i].DueAt
				d.UnlockAt = assignments[i].UnlockAt
				d.LockAt = assignments[i].LockAt
			}
		} else {
			d.UnlockAt = t.DelayedPostAt
			d.LockAt = t.LockAt
		}
		s.DiscussionTopics = append(s.DiscussionTopics, d)
	}

	// Announcements. Canvas keeps them in the discussion topics it leaves out
	// of the list above, and hands them over only when asked for by name.
	announcements, err := readList[topic](ctx, c, in+"/discussion_topics", map[string]string{"only_announcements": "true"})
	if err != nil {
		return Snapshot{}, err
	}
	for _, a := range announcements {
		s.Announcements = append(s.Announcements, Announcement{ID: a.ID, Title: a.Title, Message: a.Message})
	}

	// Files. Only the name is checked; nothing here downloads them.
	files, err := readList[struct {
		ID          int64  `json:"id"`
		DisplayName string `json:"display_name"`
	}](ctx, c, in+"/files", nil)
	if err != nil {
		return Snapshot{}, err
	}
	for _, f := range files {
		s.Files = append(s.Files, File{ID: f.ID, DisplayName: f.DisplayName})
	}

	return s, nil
}

// readList reads every page of one Canvas list and decodes each item.
func readList[T any](ctx context.Context, c *client.Client, path string, params map[string]string) ([]T, error) {
	items, err := c.GetAll(ctx, path, params)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(items))
	for i, raw := range items {
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("reading item %d of %s: %w", i+1, path, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func hasAny(have []string, want ...string) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}
