// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Survives `generate --force` regen-merge as a whole unit.

// pp:data-source live

package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/johnnyrobot/canvas-pp-cli/internal/client"
	"github.com/johnnyrobot/canvas-pp-cli/internal/cliutil"
)

// A file-backed content migration is a three-step exchange with Canvas:
//
//  1. POST content_migrations with pre_attachment[...] — creates the migration
//     in pre_processing and returns a one-time upload_url + upload_params.
//  2. POST the file (multipart) to upload_url.
//  3. If step 2 answers with a redirect, GET the Location with auth. That
//     create_success call is what attaches the file and queues the import.
//
// Skipping step 3 (e.g. curl without -L) leaves the migration in
// pre_processing with no file. Canvas runs one migration per course at a time,
// so that orphan silently blocks every later import on the course for hours.
// The generated `content_migrations create-2` command only performs step 1;
// this command performs all three and checks the result.

// Canvas treats these API states as "a migration is still going" for the
// purposes of blocking newer migrations on the same course.
var activeMigrationStates = map[string]bool{
	"pre_processing": true,
	"pre_processed":  true,
	"queued":         true,
	"running":        true,
}

// Canvas stops letting an active migration block others once it started this
// long ago (Setting content_migration_job_block_hours, default 4).
const migrationBlockWindow = 4 * time.Hour

// releaseSize is sent as the pre_attachment size when abandoning a migration
// whose upload failed. It is far over any course quota, so Canvas's preflight
// rejects it and marks the migration pre_process_error ("failed"), which stops
// it blocking the course. Canvas has no cancel endpoint for migrations.
const releaseSize = int64(1) << 50

type courseImportAPI interface {
	GetNoCache(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
	Post(ctx context.Context, path string, body any) (json.RawMessage, int, error)
	Put(ctx context.Context, path string, body any) (json.RawMessage, int, error)
	RequestBaseURL() string
}

type cmPreAttachment struct {
	UploadURL    string         `json:"upload_url"`
	UploadParams map[string]any `json:"upload_params"`
	FileParam    string         `json:"file_param"`
	Error        bool           `json:"error"`
	Message      string         `json:"message"`
}

type cmMigration struct {
	ID            int64            `json:"id"`
	WorkflowState string           `json:"workflow_state"`
	MigrationType string           `json:"migration_type"`
	StartedAt     *time.Time       `json:"started_at"`
	ProgressURL   string           `json:"progress_url"`
	IssuesCount   int              `json:"migration_issues_count"`
	PreAttachment *cmPreAttachment `json:"pre_attachment"`
	Attachment    *struct {
		ID int64 `json:"id"`
	} `json:"attachment"`
}

type courseImportResult struct {
	CourseID        string `json:"course_id"`
	MigrationID     int64  `json:"migration_id,omitempty"`
	MigrationType   string `json:"migration_type"`
	File            string `json:"file"`
	Size            int64  `json:"size"`
	AttachmentID    int64  `json:"attachment_id,omitempty"`
	UploadAttempts  int    `json:"upload_attempts"`
	WorkflowState   string `json:"workflow_state,omitempty"`
	MigrationIssues int    `json:"migration_issues_count"`
	ProgressURL     string `json:"progress_url,omitempty"`
	Released        bool   `json:"released,omitempty"`
	Note            string `json:"note,omitempty"`
}

type courseImporter struct {
	api    courseImportAPI
	upload *http.Client
	log    io.Writer

	courseID      string
	filePath      string
	migrationType string
	force         bool
	wait          bool
	waitTimeout   time.Duration
	pollInterval  time.Duration
	startTimeout  time.Duration
	maxAttempts   int
	now           func() time.Time

	lastState string // last state logged, so polls only log changes
}

func newCourseImportCmd(flags *rootFlags) *cobra.Command {
	var migrationType string
	var force bool
	var noWait bool
	var waitTimeout time.Duration
	var attempts int

	cmd := &cobra.Command{
		Use:   "course-import <course_id> <file>",
		Short: "Import a course package (.imscc / .zip) into a course, including the file upload",
		Long: "Creates a content migration, uploads the file, confirms the upload, and waits for the import to finish.\n\n" +
			"Use this instead of `content_migrations create-2` plus a hand-rolled upload: a missed confirm step leaves the\n" +
			"migration stuck in pre_processing, which blocks every later import on the course for hours.\n\n" +
			"The migration type is detected from the package (Canvas export -> canvas_cartridge_importer, other Common\n" +
			"Cartridge -> common_cartridge_importer); pass --type to override. Refuses to start while another import on the\n" +
			"course is still active (it would only queue behind it) unless --force is given.",
		Example: "  canvas-pp-cli course-import 259 ./AILiteracy.imscc\n" +
			"  canvas-pp-cli course-import 259 ./package.zip --type common_cartridge_importer --no-wait --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) != 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("expected <course_id> <file>"))
			}
			courseID, filePath := args[0], args[1]
			info, err := os.Stat(filePath)
			if err != nil {
				return usageErr(fmt.Errorf("reading %s: %w", filePath, err))
			}
			if info.IsDir() || info.Size() == 0 {
				return usageErr(fmt.Errorf("%s is not a non-empty file", filePath))
			}
			if migrationType == "" {
				migrationType, err = detectMigrationType(filePath)
				if err != nil {
					return usageErr(err)
				}
			}
			if attempts < 1 {
				return usageErr(fmt.Errorf("--attempts must be at least 1"))
			}
			if dryRunOK(flags) || (cliutil.IsVerifyEnv() && !cliutil.IsVerifyLiveHTTPEnv()) {
				fmt.Fprintf(cmd.OutOrStdout(), "would import %s (%d bytes) into course %s as %s: create migration, upload, confirm, wait=%t\n",
					filepath.Base(filePath), info.Size(), courseID, migrationType, !noWait)
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			imp := &courseImporter{
				api:           c,
				upload:        uploadHTTPClient(c),
				log:           cmd.ErrOrStderr(),
				courseID:      courseID,
				filePath:      filePath,
				migrationType: migrationType,
				force:         force,
				wait:          !noWait,
				waitTimeout:   waitTimeout,
				pollInterval:  3 * time.Second,
				startTimeout:  30 * time.Second,
				maxAttempts:   attempts,
				now:           time.Now,
			}
			if flags.quiet {
				imp.log = io.Discard
			}
			res, runErr := imp.run(cmd.Context())
			if res != nil {
				if perr := printJSONFiltered(cmd.OutOrStdout(), res, flags); perr != nil && runErr == nil {
					runErr = perr
				}
			}
			if runErr != nil {
				return classifyAPIError(runErr, flags)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&migrationType, "type", "", "Migration type (default: detected from the package)")
	cmd.Flags().BoolVar(&force, "force", false, "Start even if another import on the course is still active")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Return once the import is queued instead of waiting for it to finish")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Minute, "How long to wait for the import to finish")
	cmd.Flags().IntVar(&attempts, "attempts", 3, "Upload attempts before giving up (each retry gets a fresh upload token)")
	return cmd
}

// uploadHTTPClient reuses the API client's transport but never follows
// redirects (the redirect target must be fetched with auth, by the API client)
// and has no fixed timeout, since packages can be hundreds of MB. Cancellation
// comes from the command context.
func uploadHTTPClient(c *client.Client) *http.Client {
	var transport http.RoundTripper
	if c.HTTPClient != nil {
		transport = c.HTTPClient.Transport
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// detectMigrationType mirrors how Canvas tells its own exports apart from
// generic Common Cartridge packages.
func detectMigrationType(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("%s is not a zip/imscc package (%v); pass --type to choose a migration type", filepath.Base(path), err)
	}
	defer zr.Close()
	hasManifest := false
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if name == "course_settings/canvas_export.txt" {
			return "canvas_cartridge_importer", nil
		}
		if name == "imsmanifest.xml" {
			hasManifest = true
		}
	}
	if hasManifest {
		return "common_cartridge_importer", nil
	}
	return "", fmt.Errorf("%s has no imsmanifest.xml, so it is not a course package; pass --type to choose a migration type", filepath.Base(path))
}

func (r *courseImporter) migrationsPath() string {
	return "/api/v1/courses/" + url.PathEscape(r.courseID) + "/content_migrations"
}

func (r *courseImporter) logf(format string, a ...any) {
	if r.log != nil {
		fmt.Fprintf(r.log, format+"\n", a...)
	}
}

func (r *courseImporter) run(ctx context.Context) (*courseImportResult, error) {
	info, err := os.Stat(r.filePath)
	if err != nil {
		return nil, err
	}
	res := &courseImportResult{
		CourseID:      r.courseID,
		MigrationType: r.migrationType,
		File:          filepath.Base(r.filePath),
		Size:          info.Size(),
	}

	if !r.force {
		if err := r.checkNoActiveMigration(ctx); err != nil {
			return nil, err
		}
	}

	body := map[string]any{
		"migration_type":       r.migrationType,
		"pre_attachment[name]": res.File,
		"pre_attachment[size]": res.Size,
	}
	data, _, err := r.api.Post(ctx, r.migrationsPath(), body)
	if err != nil {
		return nil, fmt.Errorf("creating content migration: %w", err)
	}
	var m cmMigration
	if err := json.Unmarshal(data, &m); err != nil || m.ID == 0 {
		return nil, fmt.Errorf("creating content migration: unexpected response: %s", truncateForError(data))
	}
	res.MigrationID = m.ID
	res.WorkflowState = m.WorkflowState
	r.logf("created migration %d (%s)", m.ID, r.migrationType)

	pre := m.PreAttachment
	var uploadErr error
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		res.UploadAttempts = attempt
		if attempt > 1 {
			r.logf("upload attempt %d failed: %v; requesting a fresh upload token", attempt-1, uploadErr)
			pre, err = r.refreshUpload(ctx, m.ID, res.Size)
			if err != nil {
				uploadErr = err
				break
			}
		}
		var attachmentID int64
		attachmentID, uploadErr = r.uploadOnce(ctx, pre)
		if uploadErr == nil {
			res.AttachmentID = attachmentID
			break
		}
	}
	if uploadErr != nil {
		return res, r.abandon(ctx, res, uploadErr)
	}
	r.logf("uploaded %s (%d bytes), attachment %d", res.File, res.Size, res.AttachmentID)

	// create_success attaches the file and queues the import in the same
	// request, so the migration should leave pre_processing almost at once.
	m2, err := r.waitFor(ctx, m.ID, r.startTimeout, func(s string) bool { return s != "pre_processing" })
	if m2 != nil {
		r.fill(res, m2)
	}
	if err != nil {
		if errors.Is(err, errWaitTimeout) {
			return res, r.abandon(ctx, res, fmt.Errorf("upload confirmed but the migration never left pre_processing"))
		}
		return res, err
	}

	if !r.wait {
		res.Note = "import queued; not waiting (--no-wait)"
		return res, nil
	}
	m3, err := r.waitFor(ctx, m.ID, r.waitTimeout, func(s string) bool { return s == "completed" || s == "failed" })
	if m3 != nil {
		r.fill(res, m3)
	}
	if err != nil {
		if errors.Is(err, errWaitTimeout) {
			res.Note = fmt.Sprintf("still %s after %s; check later with `content_migrations show-6 %s %d`", res.WorkflowState, r.waitTimeout, r.courseID, m.ID)
			return res, apiErr(fmt.Errorf("timed out waiting for migration %d (state %s)", m.ID, res.WorkflowState))
		}
		return res, err
	}
	if res.WorkflowState == "failed" {
		res.Note = fmt.Sprintf("see `content_migrations index-2 %s %d` for migration issues", r.courseID, m.ID)
		return res, apiErr(fmt.Errorf("migration %d failed", m.ID))
	}
	if res.MigrationIssues > 0 {
		res.Note = fmt.Sprintf("completed with %d migration issue(s); see `content_migrations index-2 %s %d`", res.MigrationIssues, r.courseID, m.ID)
	}
	return res, nil
}

func (r *courseImporter) fill(res *courseImportResult, m *cmMigration) {
	res.WorkflowState = m.WorkflowState
	res.MigrationIssues = m.IssuesCount
	if m.ProgressURL != "" {
		res.ProgressURL = m.ProgressURL
	}
}

// checkNoActiveMigration refuses to start when Canvas would only queue the new
// import behind one that is still active. Listing migrations also makes Canvas
// time out uploads abandoned for over an hour, so stale orphans clear here.
func (r *courseImporter) checkNoActiveMigration(ctx context.Context) error {
	data, err := r.api.GetNoCache(ctx, r.migrationsPath(), map[string]string{"per_page": "50"})
	if err != nil {
		return fmt.Errorf("listing content migrations: %w", err)
	}
	var list []cmMigration
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("listing content migrations: unexpected response: %s", truncateForError(data))
	}
	cutoff := r.now().Add(-migrationBlockWindow)
	var blocking []string
	for _, m := range list {
		if !activeMigrationStates[m.WorkflowState] {
			continue
		}
		if m.StartedAt != nil && m.StartedAt.Before(cutoff) {
			continue
		}
		blocking = append(blocking, fmt.Sprintf("%d (%s)", m.ID, m.WorkflowState))
	}
	if len(blocking) == 0 {
		return nil
	}
	sort.Strings(blocking)
	return &cliError{code: 2, err: fmt.Errorf(
		"course %s already has an active import: migration %s. A new import would wait behind it. "+
			"Wait for it to finish, or pass --force to queue anyway", r.courseID, strings.Join(blocking, ", "))}
}

// refreshUpload asks Canvas for a new upload token on the same migration.
// Upload tokens are single-use, so a retry cannot reuse the first one.
func (r *courseImporter) refreshUpload(ctx context.Context, id, size int64) (*cmPreAttachment, error) {
	body := map[string]any{
		"migration_type":       r.migrationType,
		"pre_attachment[name]": filepath.Base(r.filePath),
		"pre_attachment[size]": size,
	}
	data, _, err := r.api.Put(ctx, fmt.Sprintf("%s/%d", r.migrationsPath(), id), body)
	if err != nil {
		return nil, fmt.Errorf("refreshing upload token: %w", err)
	}
	var m cmMigration
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("refreshing upload token: unexpected response: %s", truncateForError(data))
	}
	return m.PreAttachment, nil
}

// uploadOnce performs steps 2 and 3 and returns the confirmed attachment id.
func (r *courseImporter) uploadOnce(ctx context.Context, pre *cmPreAttachment) (int64, error) {
	if pre == nil || pre.UploadURL == "" {
		return 0, fmt.Errorf("Canvas returned no upload_url")
	}
	if pre.Error {
		return 0, fmt.Errorf("Canvas rejected the upload: %s", pre.Message)
	}
	req, err := r.buildUploadRequest(ctx, pre)
	if err != nil {
		return 0, err
	}
	resp, err := r.upload.Do(req)
	if err != nil {
		return 0, fmt.Errorf("uploading file: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc := resp.Header.Get("Location")
		if loc == "" {
			return 0, fmt.Errorf("upload returned HTTP %d without a Location to confirm", resp.StatusCode)
		}
		return r.confirm(ctx, loc)
	case resp.StatusCode == http.StatusCreated:
		if loc := resp.Header.Get("Location"); loc != "" && r.isCanvasURL(loc) {
			return r.confirm(ctx, loc)
		}
		return attachmentIDFrom(respBody)
	case resp.StatusCode == http.StatusOK:
		return attachmentIDFrom(respBody)
	default:
		return 0, fmt.Errorf("upload returned HTTP %d: %s", resp.StatusCode, truncateForError(respBody))
	}
}

// buildUploadRequest streams the file as the last multipart field (S3-style
// targets require that) with an exact Content-Length (they reject chunked).
func (r *courseImporter) buildUploadRequest(ctx context.Context, pre *cmPreAttachment) (*http.Request, error) {
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	keys := make([]string, 0, len(pre.UploadParams))
	for k := range pre.UploadParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := pre.UploadParams[k]
		if v == nil {
			continue
		}
		if err := mw.WriteField(k, fmt.Sprint(v)); err != nil {
			return nil, err
		}
	}
	fileParam := pre.FileParam
	if fileParam == "" {
		fileParam = "file"
	}
	if _, err := mw.CreateFormFile(fileParam, filepath.Base(r.filePath)); err != nil {
		return nil, err
	}
	headBytes := append([]byte(nil), head.Bytes()...)
	head.Reset()
	if err := mw.Close(); err != nil {
		return nil, err
	}
	tail := "\r\n" + strings.TrimPrefix(head.String(), "\r\n")

	f, err := os.Open(r.filePath)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	body := io.MultiReader(bytes.NewReader(headBytes), f, strings.NewReader(tail))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pre.UploadURL, readCloser{body, f})
	if err != nil {
		f.Close()
		return nil, err
	}
	req.ContentLength = int64(len(headBytes)) + info.Size() + int64(len(tail))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("User-Agent", client.UserAgent())
	return req, nil
}

type readCloser struct {
	io.Reader
	c io.Closer
}

func (rc readCloser) Close() error { return rc.c.Close() }

func (r *courseImporter) isCanvasURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if !u.IsAbs() {
		return true
	}
	base, err := url.Parse(r.api.RequestBaseURL())
	return err == nil && strings.EqualFold(u.Hostname(), base.Hostname())
}

// confirm is step 3: GET the redirect target with the API token. This is the
// call that attaches the file to the migration and queues the import.
func (r *courseImporter) confirm(ctx context.Context, loc string) (int64, error) {
	if !r.isCanvasURL(loc) {
		return 0, fmt.Errorf("upload redirected to %s, which is not this Canvas instance", loc)
	}
	u, err := url.Parse(loc)
	if err != nil {
		return 0, err
	}
	params := map[string]string{}
	for k, vs := range u.Query() {
		if len(vs) > 0 {
			params[k] = vs[0]
		}
	}
	path := u.Path
	if basePath := strings.TrimRight(mustURLPath(r.api.RequestBaseURL()), "/"); basePath != "" {
		path = strings.TrimPrefix(path, basePath)
	}
	data, err := r.api.GetNoCache(ctx, path, params)
	if err != nil {
		return 0, fmt.Errorf("confirming upload: %w", err)
	}
	return attachmentIDFrom(data)
}

func mustURLPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Path
}

func attachmentIDFrom(data []byte) (int64, error) {
	var a struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &a); err != nil || a.ID == 0 {
		return 0, fmt.Errorf("upload finished but Canvas returned no file id: %s", truncateForError(data))
	}
	return a.ID, nil
}

var errWaitTimeout = errors.New("timed out")

func (r *courseImporter) getMigration(ctx context.Context, id int64) (*cmMigration, error) {
	data, err := r.api.GetNoCache(ctx, fmt.Sprintf("%s/%d", r.migrationsPath(), id), nil)
	if err != nil {
		return nil, err
	}
	var m cmMigration
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("reading migration %d: unexpected response: %s", id, truncateForError(data))
	}
	return &m, nil
}

func (r *courseImporter) waitFor(ctx context.Context, id int64, timeout time.Duration, done func(string) bool) (*cmMigration, error) {
	deadline := r.now().Add(timeout)
	for {
		m, err := r.getMigration(ctx, id)
		if err != nil {
			return nil, err
		}
		if m.WorkflowState != r.lastState {
			r.logf("migration %d: %s", id, m.WorkflowState)
			r.lastState = m.WorkflowState
		}
		if done(m.WorkflowState) {
			return m, nil
		}
		if !r.now().Before(deadline) {
			return m, errWaitTimeout
		}
		select {
		case <-ctx.Done():
			return m, ctx.Err()
		case <-time.After(r.pollInterval):
		}
	}
}

// abandon releases a migration that will never get its file, so it does not
// block later imports on the course, and reports what happened.
func (r *courseImporter) abandon(ctx context.Context, res *courseImportResult, cause error) error {
	body := map[string]any{
		"migration_type":       r.migrationType,
		"pre_attachment[name]": res.File,
		"pre_attachment[size]": releaseSize,
	}
	data, _, err := r.api.Put(ctx, fmt.Sprintf("%s/%d", r.migrationsPath(), res.MigrationID), body)
	var m cmMigration
	if err == nil && json.Unmarshal(data, &m) == nil && m.WorkflowState == "failed" {
		res.Released = true
		res.WorkflowState = "failed"
		res.Note = "upload failed; the migration was marked failed so it does not block other imports. Re-run course-import to try again"
		return apiErr(fmt.Errorf("migration %d: %w", res.MigrationID, cause))
	}
	res.Note = fmt.Sprintf("upload failed and the migration could not be released. Canvas will block other imports on course %s "+
		"until it times the upload out (about an hour after its last update, applied when the import list is viewed)", r.courseID)
	return apiErr(fmt.Errorf("migration %d: %w", res.MigrationID, cause))
}

func truncateForError(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	if s == "" {
		s = "(empty body)"
	}
	return s
}
