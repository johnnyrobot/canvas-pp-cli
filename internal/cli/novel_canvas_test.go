// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for the Canvas transcendence command logic.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func obj(t *testing.T, jsonStr string) canvasObj {
	t.Helper()
	var o canvasObj
	if err := json.Unmarshal([]byte(jsonStr), &o); err != nil {
		t.Fatalf("bad fixture json: %v", err)
	}
	return o
}

func TestCanvasObjAccessors(t *testing.T) {
	o := obj(t, `{"id":42,"name":"Ada","ratio":"1.5","active":true,"score":null,"user":{"login_id":"ada1"},"tags":[{"k":"v"}]}`)
	if got := o.str("id"); got != "42" {
		t.Errorf("str(id numeric) = %q, want 42", got)
	}
	if got := o.str("name"); got != "Ada" {
		t.Errorf("str(name) = %q, want Ada", got)
	}
	if v, ok := o.num("id"); !ok || v != 42 {
		t.Errorf("num(id) = %v,%v want 42,true", v, ok)
	}
	if v, ok := o.num("ratio"); !ok || v != 1.5 {
		t.Errorf("num(numeric-string ratio) = %v,%v want 1.5,true", v, ok)
	}
	if _, ok := o.num("score"); ok {
		t.Errorf("num(null) should be ok=false")
	}
	if !o.boolv("active") {
		t.Errorf("boolv(active) = false, want true")
	}
	if o.obj("user").str("login_id") != "ada1" {
		t.Errorf("obj(user).login_id mismatch")
	}
	if o.present("score") {
		t.Errorf("present(null score) should be false")
	}
	if !o.present("name") {
		t.Errorf("present(name) should be true")
	}
	if len(o.list("tags")) != 1 {
		t.Errorf("list(tags) len = %d, want 1", len(o.list("tags")))
	}
	if o.list("missing") != nil {
		t.Errorf("list(missing) should be nil")
	}
}

func TestLetterBucket(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{95, "A"}, {90, "A"}, {89.9, "B"}, {80, "B"}, {75, "C"}, {70, "C"},
		{65, "D"}, {60, "D"}, {59.9, "F"}, {0, "F"},
	}
	for _, tc := range cases {
		if got := letterBucket(tc.score); got != tc.want {
			t.Errorf("letterBucket(%v) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestClassifyAtRisk(t *testing.T) {
	future := time.Now().Add(48 * time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	cases := []struct {
		name      string
		sub       string
		useCutoff bool
		cutoff    time.Time
		want      string
	}{
		{"missing", `{"missing":true,"assignment":{}}`, false, time.Time{}, "missing"},
		{"late not missing", `{"missing":false,"late":true,"assignment":{}}`, false, time.Time{}, "late"},
		{"excused suppressed", `{"missing":true,"excused":true,"assignment":{}}`, false, time.Time{}, ""},
		{"unsubmitted past due", `{"workflow_state":"unsubmitted","assignment":{"due_at":"` + past + `"}}`, false, time.Time{}, "unsubmitted"},
		{"unsubmitted future due", `{"workflow_state":"unsubmitted","assignment":{"due_at":"` + future + `"}}`, false, time.Time{}, ""},
		{"graded clean", `{"workflow_state":"graded","missing":false,"late":false,"assignment":{}}`, false, time.Time{}, ""},
		{"cutoff filters old", `{"missing":true,"assignment":{"due_at":"` + past + `"}}`, true, time.Now().Add(-24 * time.Hour), ""},
		{"cutoff keeps recent", `{"missing":true,"assignment":{"due_at":"` + future + `"}}`, true, time.Now().Add(-24 * time.Hour), "missing"},
		{"cutoff skips no-due", `{"missing":true,"assignment":{}}`, true, time.Now().Add(-24 * time.Hour), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAtRisk(obj(t, tc.sub), tc.cutoff, tc.useCutoff)
			if got != tc.want {
				t.Errorf("classifyAtRisk = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNeedsGrading(t *testing.T) {
	cases := []struct {
		name string
		sub  string
		want bool
	}{
		{"submitted ungraded", `{"workflow_state":"submitted","submitted_at":"2024-01-01T00:00:00Z"}`, true},
		{"pending_review", `{"workflow_state":"pending_review","submitted_at":"2024-01-01T00:00:00Z"}`, true},
		{"already graded", `{"workflow_state":"submitted","submitted_at":"2024-01-01T00:00:00Z","graded_at":"2024-01-02T00:00:00Z"}`, false},
		{"scored", `{"workflow_state":"submitted","submitted_at":"2024-01-01T00:00:00Z","score":88}`, false},
		{"excused", `{"workflow_state":"submitted","submitted_at":"2024-01-01T00:00:00Z","excused":true}`, false},
		{"unsubmitted", `{"workflow_state":"unsubmitted"}`, false},
		{"no submitted_at", `{"workflow_state":"submitted"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsGrading(obj(t, tc.sub)); got != tc.want {
				t.Errorf("needsGrading = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAfterCutoff(t *testing.T) {
	cutoff := time.Now().Add(-24 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	old := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	if !afterCutoff(recent, cutoff) {
		t.Errorf("recent ts should be after cutoff")
	}
	if afterCutoff(old, cutoff) {
		t.Errorf("old ts should be before cutoff")
	}
	if afterCutoff("", cutoff) {
		t.Errorf("empty ts should be false")
	}
	if !afterCutoff("not-a-date", cutoff) {
		t.Errorf("unparseable ts should be kept (true)")
	}
}

func TestAnonLabel(t *testing.T) {
	a := anonLabel("student", "Ada Lovelace")
	b := anonLabel("student", "Ada Lovelace")
	c := anonLabel("student", "Grace Hopper")
	if a != b {
		t.Errorf("anonLabel not stable: %q != %q", a, b)
	}
	if a == c {
		t.Errorf("anonLabel collision for distinct inputs")
	}
	if anonLabel("student", "") != "" {
		t.Errorf("anonLabel(empty) should be empty")
	}
	if len(a) <= len("student-") {
		t.Errorf("anonLabel too short: %q", a)
	}
}

func TestStandingsAccGroup(t *testing.T) {
	acc := &standingsAcc{name: "Bio 101"}
	acc.add(95, true) // A
	acc.add(85, true) // B
	acc.add(72, true) // C
	acc.add(55, true) // F
	acc.add(0, false) // ungraded
	g := acc.group("c1")
	if g.Students != 5 {
		t.Errorf("students = %d, want 5", g.Students)
	}
	if g.Graded != 4 {
		t.Errorf("graded = %d, want 4", g.Graded)
	}
	if g.Dist.A != 1 || g.Dist.B != 1 || g.Dist.C != 1 || g.Dist.F != 1 || g.Dist.Ungraded != 1 {
		t.Errorf("distribution mismatch: %+v", g.Dist)
	}
	// pass = A+B+C over graded = 3/4
	if g.PassRate == nil || *g.PassRate != 0.75 {
		t.Errorf("pass_rate = %v, want 0.75", g.PassRate)
	}
	if g.DFWRate == nil || *g.DFWRate != 0.25 {
		t.Errorf("dfw_rate = %v, want 0.25", g.DFWRate)
	}
}

func TestContainsStr(t *testing.T) {
	s := []string{"a", "b"}
	if !containsStr(s, "a") || containsStr(s, "z") {
		t.Errorf("containsStr logic wrong")
	}
}
