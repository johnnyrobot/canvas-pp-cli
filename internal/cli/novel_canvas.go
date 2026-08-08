// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-built Canvas transcendence commands (roster,
// at-risk, to-grade, since, standings, audit-enrollments). These commands fetch
// several Canvas REST endpoints and join the results locally — a view no single
// /api/v1 endpoint returns. They are read-only and emit agent-native JSON.
//
// Hand-authored file (no generated header) so it survives `generate --force`
// regen-merge as a whole unit.

package cli

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"canvas-pp-cli/internal/client"
	"canvas-pp-cli/internal/cliutil"
)

// canvasObj is a decoded Canvas JSON object kept as raw values for lazy,
// type-tolerant field access (Canvas mixes string/number/null shapes).
type canvasObj map[string]json.RawMessage

func (o canvasObj) str(key string) string {
	raw, ok := o[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strings.Trim(string(raw), `"`)
}

func (o canvasObj) num(key string) (float64, bool) {
	raw, ok := o[key]
	if !ok || string(raw) == "null" {
		return 0, false
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

func (o canvasObj) boolv(key string) bool {
	raw, ok := o[key]
	if !ok {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

func (o canvasObj) obj(key string) canvasObj {
	raw, ok := o[key]
	if !ok {
		return nil
	}
	var m canvasObj
	if json.Unmarshal(raw, &m) == nil {
		return m
	}
	return nil
}

// present reports whether key exists and is not JSON null.
func (o canvasObj) present(key string) bool {
	raw, ok := o[key]
	return ok && string(raw) != "null"
}

// list decodes a nested JSON array value into []canvasObj (nil on absence or
// non-array, e.g. verify-mode synthetic envelopes).
func (o canvasObj) list(key string) []canvasObj {
	raw, ok := o[key]
	if !ok {
		return nil
	}
	var items []canvasObj
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	return items
}

// canvasFetchList GETs a Canvas collection endpoint with bounded page-based
// pagination, returning accumulated items and the number of pages actually
// scanned. It is tolerant of verify-mode synthetic envelopes: a non-array body
// ends pagination with the items gathered so far and no error, so the command
// still produces a valid (empty) result under PRINTING_PRESS_VERIFY=1.
func canvasFetchList(ctx context.Context, c *client.Client, path string, params map[string]string, maxPages int) ([]canvasObj, int, error) {
	if maxPages < 1 {
		maxPages = 1
	}
	if cliutil.IsDogfoodEnv() && maxPages > 1 {
		maxPages = 1
	}
	p := map[string]string{}
	for k, v := range params {
		p[k] = v
	}
	if p["per_page"] == "" {
		p["per_page"] = "100"
	}
	out := []canvasObj{}
	pages := 0
	for page := 1; page <= maxPages; page++ {
		p["page"] = strconv.Itoa(page)
		data, err := c.Get(ctx, path, p)
		if err != nil {
			if pages == 0 {
				return out, pages, err
			}
			// A later page failed; return what we have rather than discarding it.
			return out, pages, nil
		}
		pages++
		var items []canvasObj
		if uerr := json.Unmarshal(data, &items); uerr != nil || len(items) == 0 {
			break // non-array (verify synthetic), empty page, or end of data
		}
		out = append(out, items...)
		if len(items) < 100 {
			break
		}
	}
	return out, pages, nil
}

// anonSaltEnv lets an operator pin the label salt, so two machines (or a CI
// job and a laptop) can produce comparable anonymized reports on purpose.
const anonSaltEnv = "CANVAS_ANON_SALT"

var (
	anonSaltOnce  sync.Once
	anonSaltValue []byte
)

// anonSalt returns the machine-local salt for anonymized labels.
//
// The salt is what makes a label non-reversible. Canvas user ids are small
// sequential integers, so an unsalted digest of one is recoverable by trying
// every plausible id — a few tens of thousands of hashes, i.e. milliseconds.
// Salting removes that shortcut without changing what the label is for.
//
// It is generated once and persisted 0600 under the state dir rather than
// randomised per run, because successive reports must stay correlatable.
// If the salt cannot be persisted the label still works for the current
// process — a fresh random salt is used, trading cross-run stability for
// never emitting a guessable label.
func anonSalt() []byte {
	anonSaltOnce.Do(func() {
		if env := strings.TrimSpace(os.Getenv(anonSaltEnv)); env != "" {
			anonSaltValue = []byte(env)
			return
		}
		fresh := make([]byte, 32)
		if _, err := rand.Read(fresh); err != nil {
			// Never fall back to "no salt" — that is the reversible case.
			anonSaltValue = []byte("canvas-pp-cli/anon/fallback")
			return
		}
		dir, err := cliutil.StateDir()
		if err != nil {
			anonSaltValue = fresh
			return
		}
		path := filepath.Join(dir, "anon-salt")
		if existing, rerr := os.ReadFile(path); rerr == nil && len(existing) > 0 {
			anonSaltValue = existing
			return
		}
		if mkerr := os.MkdirAll(dir, 0o700); mkerr != nil {
			anonSaltValue = fresh
			return
		}
		if werr := os.WriteFile(path, fresh, 0o600); werr != nil {
			anonSaltValue = fresh
			return
		}
		anonSaltValue = fresh
	})
	return anonSaltValue
}

// anonLabel returns a stable, non-reversible label for a PII string, e.g.
// "student-1a2b3c4d5e6f7a8b". Empty input yields empty output.
//
// Stable for a given salt, so the same student carries the same label across
// runs and across commands; not reversible by someone holding only the output,
// because they do not have the salt. See anonSalt.
func anonLabel(prefix, s string) string {
	if s == "" {
		return ""
	}
	m := hmac.New(sha256.New, anonSalt())
	m.Write([]byte(s))
	return prefix + "-" + hex.EncodeToString(m.Sum(nil)[:8])
}

// courseRef is a minimal course reference for fan-out commands.
type courseRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// teacherCourses returns the active courses where the authenticated user is a
// teacher. Used by the --all-my-courses fan-out commands.
func teacherCourses(ctx context.Context, c *client.Client, maxPages int) ([]courseRef, error) {
	items, _, err := canvasFetchList(ctx, c, "/api/v1/courses", map[string]string{
		"enrollment_type":  "teacher",
		"enrollment_state": "active",
	}, maxPages)
	if err != nil {
		return nil, err
	}
	out := make([]courseRef, 0, len(items))
	for _, it := range items {
		id := it.str("id")
		if id == "" {
			continue
		}
		out = append(out, courseRef{ID: id, Name: it.str("name")})
	}
	return out, nil
}

// emitNovel writes a transcendence command result. Machine consumers
// (--json/--agent/--select/--compact/--csv or a piped stdout) get the full JSON
// view; an interactive human terminal gets a compact auto-table built from rows
// (pass nil to force JSON for nested/aggregate views).
func emitNovel(cmd *cobra.Command, flags *rootFlags, view any, rows []map[string]any) error {
	w := cmd.OutOrStdout()
	if rows != nil && wantsHumanTable(w, flags) {
		return printAutoTable(w, rows)
	}
	return printJSONFiltered(w, view, flags)
}

// fetchFailure records a per-source fetch error for partial-failure accounting.
type fetchFailure struct {
	Scope string `json:"scope"`
	Error string `json:"error"`
}

// verifyEmpty short-circuits a transcendence command under
// PRINTING_PRESS_VERIFY=1 so verification/dogfood never dials the live Canvas
// host (these commands call client.Get directly, which — unlike generated
// endpoint commands — does not synthesize verify responses). It emits a valid
// empty envelope and returns true when it handled the call.
func verifyEmpty(cmd *cobra.Command, flags *rootFlags, listField string) (bool, error) {
	if !cliutil.IsVerifyEnv() {
		return false, nil
	}
	view := map[string]any{
		listField: []any{},
		"note":    "verify mode: live Canvas fetch skipped",
	}
	return true, emitNovel(cmd, flags, view, nil)
}
