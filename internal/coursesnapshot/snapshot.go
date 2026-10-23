// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. The course snapshot: one JSON document holding everything the
// checks read about a course, so they never talk to Canvas themselves.
//
// The shape here mirrors checks/src/types.ts in canvas-builder-v2 field for
// field. cb-checks names every field it can't read, so a snapshot that drifts
// from that file fails loudly rather than quietly skipping a check.

package coursesnapshot

import (
	"bytes"
	"encoding/json"
)

// Snapshot is the whole course as the checks see it. Every list is present even
// when empty: a missing list is a snapshot cb-checks refuses, and a course that
// is merely empty shouldn't look damaged.
type Snapshot struct {
	Course           Course         `json:"course"`
	SyllabusBody     *string        `json:"syllabus_body"`
	Pages            []Page         `json:"pages"`
	Assignments      []Assignment   `json:"assignments"`
	Quizzes          []Quiz         `json:"quizzes"`
	DiscussionTopics []Discussion   `json:"discussion_topics"`
	Announcements    []Announcement `json:"announcements"`
	Files            []File         `json:"files"`
}

// Course is the course itself. TimeZone is what every date in the snapshot is
// read in, so it is never empty in a snapshot that was built.
type Course struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	StartAt  *string `json:"start_at"`
	EndAt    *string `json:"end_at"`
	TimeZone string  `json:"time_zone"`
}

type Page struct {
	URL       string  `json:"url"`
	Title     string  `json:"title"`
	Body      *string `json:"body"`
	Published bool    `json:"published"`
}

// Assignment carries NewQuizStandin for the placeholder assignment the quiz
// builder leaves behind for a New Quiz: the dates check reports its schedule
// but says the questions weren't looked at. It is left out of the JSON
// entirely for an ordinary assignment, which is what cb-checks expects.
type Assignment struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	DueAt          *string `json:"due_at"`
	UnlockAt       *string `json:"unlock_at"`
	LockAt         *string `json:"lock_at"`
	NewQuizStandin bool    `json:"new_quiz_standin,omitempty"`
}

type Quiz struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	DueAt       *string `json:"due_at"`
	UnlockAt    *string `json:"unlock_at"`
	LockAt      *string `json:"lock_at"`
}

// Discussion is a discussion topic. A graded one takes its three dates from the
// assignment behind it, which Canvas keeps separately.
type Discussion struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Message  *string `json:"message"`
	Graded   bool    `json:"graded"`
	DueAt    *string `json:"due_at"`
	UnlockAt *string `json:"unlock_at"`
	LockAt   *string `json:"lock_at"`
}

// Announcement carries no schedule: Canvas dates announcements, but nothing in
// the course is due because of one, so no dated item comes from them.
type Announcement struct {
	ID      int64   `json:"id"`
	Title   string  `json:"title"`
	Message *string `json:"message"`
}

type File struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
}

// Marshal writes a snapshot as JSON with its text left as Canvas sent it.
//
// json.Marshal would rewrite every "<" in every page body as "<". The
// checks read HTML, so that would change what they see, triple the size of a
// real course's snapshot, and make a snapshot unreadable to anyone looking at
// it by hand.
func Marshal(s Snapshot) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
