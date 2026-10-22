// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the course snapshot: that what Canvas says about a
// course becomes exactly the shape the checks read, and that the snapshot a
// real cb-checks run accepts is the one this builds.

package coursesnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/config"
	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// courseClient builds the client a Canvas Builder run has, from a real config
// pointing at the fake gateway.
func courseClient(t *testing.T, f *gatewaytest.Fake) *client.Client {
	t.Helper()
	gatewaytest.Workspace(t, f, gatewaytest.WorkspaceOpts{})
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("loading the workspace config: %v", err)
	}
	c := client.New(cfg, 10*time.Second, 0)
	if c.Sandbox == nil {
		t.Fatal("the client has no Canvas Builder profile, so this isn't testing a course workspace")
	}
	c.WriteVerify = client.WriteVerifyOff
	c.Notices = &bytes.Buffer{}
	return c
}

// courseJSON is what Canvas answers for the course itself.
const courseJSON = `{"id":7,"name":"Intro to Canvas","start_at":"2027-04-12T07:00:00Z","end_at":"2027-06-12T06:59:00Z",` +
	`"time_zone":"America/Los_Angeles","syllabus_body":"<p>Read the book.</p>","workflow_state":"available"}`

// aCourse answers every list the snapshot reads. The course has one page; a
// plain assignment, a New Quiz stand-in and an assignment that is really a
// quiz; one classic quiz; a graded discussion and an ungraded one; one
// announcement; one file. The two assignments that are really something else
// are the interesting part: Canvas lists them as assignments, the checks must
// not see them twice, and the graded discussion's dates live on one of them.
func aCourse(f *gatewaytest.Fake) {
	f.Handle(http.MethodGet, "/api/v1/courses/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(courseJSON))
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"url":"week-1","title":"Week 1","body":"<p>Hello</p>","published":true,"editing_roles":"teachers"}]`))
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/assignments", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"id":11,"name":"Essay","description":"<p>Write 500 words.</p>","due_at":"2027-04-20T06:59:00Z","unlock_at":null,"lock_at":null,"submission_types":["online_text_entry"]},
			{"id":12,"name":"New Quiz: Week 3 (questions kept unchanged)","description":null,"due_at":"2027-04-27T06:59:00Z","unlock_at":null,"lock_at":null,"submission_types":["external_tool"]},
			{"id":13,"name":"Week 2 Quiz","description":null,"due_at":"2027-04-22T06:59:00Z","unlock_at":null,"lock_at":null,"submission_types":["online_quiz"]},
			{"id":14,"name":"Intro discussion","description":"<p>Say hello.</p>","due_at":"2027-04-15T06:59:00Z","unlock_at":"2027-04-12T07:00:00Z","lock_at":"2027-04-16T06:59:00Z","submission_types":["discussion_topic"]}
		]`))
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/quizzes", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":21,"title":"Week 2 Quiz","description":"<p>Ten questions.</p>","due_at":"2027-04-22T06:59:00Z","unlock_at":null,"lock_at":null,"quiz_type":"assignment"}]`))
	})
	// Canvas answers announcements on the discussion_topics path too, so one
	// handler has to tell the two lists apart the way Canvas does.
	f.Handle(http.MethodGet, "/api/v1/courses/7/discussion_topics", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("only_announcements") == "true" {
			_, _ = w.Write([]byte(`[{"id":41,"title":"Welcome","message":"<p>Welcome to the course.</p>","assignment_id":null,"delayed_post_at":null,"lock_at":null}]`))
			return
		}
		_, _ = w.Write([]byte(`[
			{"id":31,"title":"Intro discussion","message":"<p>Say hello.</p>","assignment_id":14,"delayed_post_at":null,"lock_at":null},
			{"id":32,"title":"Open questions","message":"<p>Ask anything here.</p>","assignment_id":null,"delayed_post_at":"2027-04-13T07:00:00Z","lock_at":"2027-06-01T06:59:00Z"}
		]`))
	})
	f.Handle(http.MethodGet, "/api/v1/courses/7/files", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":51,"display_name":"syllabus.pdf","folder_id":9}]`))
	})
}

// wantSnapshot is the snapshot aCourse should produce, written out in full:
// the shape is a contract with cb-checks, so it is pinned here rather than
// rebuilt from the same code that produces it.
const wantSnapshot = `{
	"course":{"id":7,"name":"Intro to Canvas","start_at":"2027-04-12T07:00:00Z","end_at":"2027-06-12T06:59:00Z","time_zone":"America/Los_Angeles"},
	"syllabus_body":"<p>Read the book.</p>",
	"pages":[{"url":"week-1","title":"Week 1","body":"<p>Hello</p>","published":true}],
	"assignments":[
		{"id":11,"name":"Essay","description":"<p>Write 500 words.</p>","due_at":"2027-04-20T06:59:00Z","unlock_at":null,"lock_at":null},
		{"id":12,"name":"New Quiz: Week 3 (questions kept unchanged)","description":null,"due_at":"2027-04-27T06:59:00Z","unlock_at":null,"lock_at":null,"new_quiz_standin":true}
	],
	"quizzes":[{"id":21,"title":"Week 2 Quiz","description":"<p>Ten questions.</p>","due_at":"2027-04-22T06:59:00Z","unlock_at":null,"lock_at":null}],
	"discussion_topics":[
		{"id":31,"title":"Intro discussion","message":"<p>Say hello.</p>","graded":true,"due_at":"2027-04-15T06:59:00Z","unlock_at":"2027-04-12T07:00:00Z","lock_at":"2027-04-16T06:59:00Z"},
		{"id":32,"title":"Open questions","message":"<p>Ask anything here.</p>","graded":false,"due_at":null,"unlock_at":"2027-04-13T07:00:00Z","lock_at":"2027-06-01T06:59:00Z"}
	],
	"announcements":[{"id":41,"title":"Welcome","message":"<p>Welcome to the course.</p>"}],
	"files":[{"id":51,"display_name":"syllabus.pdf"}]
}`

func compact(t *testing.T, s string) string {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := json.Compact(buf, []byte(s)); err != nil {
		t.Fatalf("compacting the expected snapshot: %v", err)
	}
	return buf.String()
}

func TestBuildMapsCanvasToSnapshot(t *testing.T) {
	f := gatewaytest.New(t)
	aCourse(f)
	c := courseClient(t, f)

	s, err := Build(context.Background(), c, 7)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(got) != compact(t, wantSnapshot) {
		t.Fatalf("snapshot mismatch\n got: %s\nwant: %s", got, compact(t, wantSnapshot))
	}

	// The two includes are the difference between a snapshot the checks can
	// read and one with no syllabus and no page bodies in it.
	for _, want := range []struct{ path, query string }{
		{"/api/v1/courses/7", "include%5B%5D=syllabus_body"},
		{"/api/v1/courses/7/pages", "include%5B%5D=body"},
	} {
		found := false
		for _, r := range f.Requests() {
			if r.Path == want.path && strings.Contains(r.RawQuery, want.query) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no request to %s carried %s", want.path, want.query)
		}
	}
}

// [Review Focus 6] A course taught in another language has to come through
// untouched. Go escapes HTML in JSON by default, which would rewrite every
// body in the snapshot.
func TestBuildKeepsNonEnglishText(t *testing.T) {
	const body = `<p lang="ar" dir="rtl">مرحبا</p>`
	const title = "Leçon 1"
	f := gatewaytest.New(t)
	emptyCourse(f)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		item, err := json.Marshal(map[string]any{"url": "lecon-1", "title": title, "body": body, "published": true})
		if err != nil {
			t.Errorf("building the page: %v", err)
			return
		}
		_, _ = w.Write([]byte("[" + string(item) + "]"))
	})
	c := courseClient(t, f)

	s, err := Build(context.Background(), c, 7)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(s.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(s.Pages))
	}
	if s.Pages[0].Title != title {
		t.Fatalf("title = %q, want %q", s.Pages[0].Title, title)
	}
	if s.Pages[0].Body == nil || *s.Pages[0].Body != body {
		t.Fatalf("body = %v, want %q", s.Pages[0].Body, body)
	}
	out, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// JSON has to escape the quotes around an attribute, and nothing else:
	// Go's default encoder would also rewrite every "<" as "\u003c", which
	// changes the HTML the checks read and makes a real course's snapshot
	// unreadable to anyone opening it.
	for _, want := range []string{`<p lang=`, `dir=`, "مرحبا", "</p>", "Leçon 1"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("the marshalled snapshot has no %s in it:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{`\u003c`, `\u003e`, `\u0026`, `\u00e7`} {
		if strings.Contains(string(out), unwanted) {
			t.Fatalf("the marshalled snapshot escaped text as %s:\n%s", unwanted, out)
		}
	}
}

// emptyCourse answers every list with nothing in it, for the tests that care
// about one list only.
func emptyCourse(f *gatewaytest.Fake) {
	f.Handle(http.MethodGet, "/api/v1/courses/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"name":"Intro to Canvas","start_at":null,"end_at":null,"time_zone":"America/Los_Angeles","syllabus_body":null}`))
	})
	for _, path := range []string{"pages", "assignments", "quizzes", "discussion_topics", "files"} {
		f.Handle(http.MethodGet, "/api/v1/courses/7/"+path, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[]`))
		})
	}
}

// An empty list has to arrive as [] and not as null: the checks treat a missing
// list as a snapshot they can't read, and would refuse a course that is merely
// empty.
func TestBuildEmptyCourseUsesEmptyLists(t *testing.T) {
	f := gatewaytest.New(t)
	emptyCourse(f)
	c := courseClient(t, f)

	s, err := Build(context.Background(), c, 7)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"pages":[]`, `"assignments":[]`, `"quizzes":[]`, `"discussion_topics":[]`, `"announcements":[]`, `"files":[]`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("snapshot has no %s:\n%s", want, out)
		}
	}
}

// Every date in the snapshot is read in the course's own time zone, so a course
// that doesn't name one can't be checked at all.
func TestBuildNeedsTimeZone(t *testing.T) {
	f := gatewaytest.New(t)
	emptyCourse(f)
	f.Handle(http.MethodGet, "/api/v1/courses/7", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"name":"Intro to Canvas","start_at":null,"end_at":null,"time_zone":"","syllabus_body":null}`))
	})
	c := courseClient(t, f)

	_, err := Build(context.Background(), c, 7)
	if err == nil {
		t.Fatal("want an error for a course with no time zone")
	}
	if err.Error() != sandbox.MsgNoTimeZone {
		t.Fatalf("error = %q, want %q", err, sandbox.MsgNoTimeZone)
	}
}

// Everything the snapshot reads is inside the one course. A path outside it
// would be refused by the gateway, which is the guardrail; not asking is what
// keeps an agent from spending a turn finding that out.
func TestBuildUsesCourseScopedPathsOnly(t *testing.T) {
	f := gatewaytest.New(t)
	aCourse(f)
	c := courseClient(t, f)

	if _, err := Build(context.Background(), c, 7); err != nil {
		t.Fatalf("Build: %v", err)
	}
	reqs := f.Requests()
	if len(reqs) == 0 {
		t.Fatal("no requests were made")
	}
	for _, r := range reqs {
		if !strings.HasPrefix(r.Path, "/api/v1/courses/7") {
			t.Fatalf("request to %s, which is outside this course", r.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("%s %s: building a snapshot only reads", r.Method, r.Path)
		}
	}
}

// The snapshot's whole purpose is being read by cb-checks, so one test runs the
// real thing. It needs the v2 repo and node, and skips without them.
func TestSnapshotAcceptedByCbChecks(t *testing.T) {
	repo := os.Getenv("CANVAS_BUILDER_V2")
	if repo == "" {
		t.Skip("set CANVAS_BUILDER_V2 to the canvas-builder-v2 repo to run the checks against this snapshot")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node isn't on PATH, so cb-checks can't run")
	}
	cli := filepath.Join(repo, "checks", "src", "cli.ts")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("no cb-checks at %s", cli)
	}

	f := gatewaytest.New(t)
	aCourse(f)
	c := courseClient(t, f)
	s, err := Build(context.Background(), c, 7)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out, err := Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	cmd := exec.CommandContext(context.Background(), node, cli, "all", "--snapshot", "-")
	cmd.Stdin = bytes.NewReader(out)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cb-checks refused this snapshot: %v\n%s\nsnapshot: %s", err, stderr.String(), out)
	}
	if !strings.Contains(stdout.String(), `"check":"dates"`) {
		t.Fatalf("cb-checks produced no dates report:\n%s", stdout.String())
	}
}
