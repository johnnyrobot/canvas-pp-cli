// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: putting a file into a course. Canvas does this in three
// requests rather than one — ask where to put the file, put it there, then ask
// what was kept — and each of the three has to stay inside Canvas Builder. The
// second one carries both the file and the token, so the address Canvas names
// for it is checked before anything is sent.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// maxFileJSON bounds what is read from an answer that should be one file's
// JSON, so a redirect to something enormous can't be read into memory.
const maxFileJSON = 1 << 20

// UploadRequest is one file on its way into a course.
type UploadRequest struct {
	// Path is the file to send. Whoever built this request has already decided
	// it may be read; in Canvas Builder that means sandbox.Inside said so.
	Path string
	// Name is the display name in Canvas. Empty means the file's own name.
	Name string
	// ContentType is empty for "whatever the file's extension says".
	ContentType string
	// Folder is a course folder path, such as "course files/images". Empty
	// leaves the file where Canvas puts it by default.
	Folder string
	// OnDuplicate is "rename" (the default) or "overwrite".
	OnDuplicate string
}

// FileInfo is a Canvas file: as much of one as the commands here need.
type FileInfo struct {
	ID          int64
	DisplayName string
	Size        int64
	ContentType string
	FolderID    int64
	URL         string
}

// UnmarshalJSON reads a Canvas file object. Canvas spells a file's type
// "content-type", with a hyphen, in the files API and "content_type" in some
// other places, so both are read and the hyphenated one wins when both are
// there.
func (f *FileInfo) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          int64  `json:"id"`
		DisplayName string `json:"display_name"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
		HyphenType  string `json:"content-type"`
		FolderID    int64  `json:"folder_id"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = FileInfo{
		ID:          raw.ID,
		DisplayName: raw.DisplayName,
		Size:        raw.Size,
		ContentType: raw.ContentType,
		FolderID:    raw.FolderID,
		URL:         raw.URL,
	}
	if raw.HyphenType != "" {
		f.ContentType = raw.HyphenType
	}
	return nil
}

// RefusedError is a refusal this CLI made itself, before the request went out:
// an address one of the steps pointed at was not Canvas Builder's. A refusal
// the gateway answered with arrives differently — as an APIError carrying the
// gateway's own sentence — and the CLI layer turns both into exit 9.
type RefusedError struct{ Cause error }

func (e *RefusedError) Error() string { return e.Cause.Error() }

func (e *RefusedError) Unwrap() error { return e.Cause }

// formBody is a flat parameter map whose caller has already decided it must be
// form-encoded, even though none of its keys carry Rails brackets. Canvas's
// file-create endpoint reads flat parameters (name, size, content_type), and a
// JSON body of the same names is at the mercy of Rails' parameter wrapping.
// See formencode.go.
//
// It also keeps the post-write check off step 1, which is right: step 1's
// answer is a ticket, not the file, so none of the names sent come back in it.
type formBody map[string]any

// uploadTicket is what step 1 hands back: where to put the file, what to send
// alongside it, and what to call the file's own part.
type uploadTicket struct {
	url       string
	params    map[string]string
	fileParam string
}

// UploadFile puts a local file into a course and returns the file Canvas kept.
func (c *Client) UploadFile(ctx context.Context, courseID int64, r UploadRequest) (FileInfo, error) {
	file, err := os.Open(r.Path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("opening %s: %w", r.Path, err)
	}
	defer func() { _ = file.Close() }()
	stat, err := file.Stat()
	if err != nil {
		return FileInfo{}, fmt.Errorf("reading %s: %w", r.Path, err)
	}
	if stat.IsDir() {
		return FileInfo{}, fmt.Errorf("%s is a folder, and only one file goes up at a time", r.Path)
	}

	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = filepath.Base(r.Path)
	}
	contentType := strings.TrimSpace(r.ContentType)
	if contentType == "" {
		contentType = guessContentType(name)
	}
	onDuplicate := strings.TrimSpace(r.OnDuplicate)
	if onDuplicate == "" {
		onDuplicate = "rename"
	}

	ticket, err := c.requestUpload(ctx, courseID, name, stat.Size(), contentType, r.Folder, onDuplicate)
	if err != nil {
		return FileInfo{}, err
	}
	return c.sendUpload(ctx, ticket, file, stat.Size(), name, contentType)
}

// guessContentType names a file's type from its extension, which is all the
// agent gives us. Anything unrecognized is bytes.
func guessContentType(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// requestUpload is step 1: it asks the course for somewhere to put the file.
func (c *Client) requestUpload(ctx context.Context, courseID int64, name string, size int64, contentType, folder, onDuplicate string) (uploadTicket, error) {
	body := formBody{
		"name":         name,
		"size":         strconv.FormatInt(size, 10),
		"content_type": contentType,
		"on_duplicate": onDuplicate,
	}
	if folder = strings.TrimSpace(folder); folder != "" {
		body["parent_folder_path"] = folder
	}
	raw, _, err := c.Post(ctx, fmt.Sprintf("/api/v1/courses/%d/files", courseID), body)
	if err != nil {
		return uploadTicket{}, err
	}

	var answer struct {
		UploadURL    string                     `json:"upload_url"`
		UploadParams map[string]json.RawMessage `json:"upload_params"`
		FileParam    string                     `json:"file_param"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return uploadTicket{}, fmt.Errorf("reading the answer to the upload request: %w", err)
	}
	if strings.TrimSpace(answer.UploadURL) == "" {
		return uploadTicket{}, errors.New("the upload request came back without an address to put the file at")
	}
	ticket := uploadTicket{url: answer.UploadURL, fileParam: answer.FileParam, params: map[string]string{}}
	if ticket.fileParam == "" {
		ticket.fileParam = "file"
	}
	for key, value := range answer.UploadParams {
		ticket.params[key] = scalarString(value)
	}
	return ticket, nil
}

// scalarString renders one upload_params value as the string it has to be on
// the wire. Canvas puts numbers and booleans in there as well as strings, and
// a multipart field is text whichever it was.
func scalarString(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	literal := strings.TrimSpace(string(raw))
	if literal == "null" {
		return ""
	}
	return literal
}

// sendUpload is step 2: the file itself. It is the one request that carries
// both the file and the token, so the address comes from Canvas and is checked
// here before anything is built or dialed. The answer is read here too, by the
// function that opened it.
func (c *Client) sendUpload(ctx context.Context, ticket uploadTicket, file *os.File, size int64, name, contentType string) (FileInfo, error) {
	if err := c.checkUploadURL(ticket.url); err != nil {
		return FileInfo{}, err
	}
	prefix, suffix, formType, err := multipartFraming(ticket, name, contentType)
	if err != nil {
		return FileInfo{}, err
	}

	// The file is never read into memory: it is the middle of the body, and
	// Content-Length is the exact sum of the three pieces.
	body := io.MultiReader(bytes.NewReader(prefix), file, bytes.NewReader(suffix))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticket.url, body)
	if err != nil {
		return FileInfo{}, fmt.Errorf("creating the upload request: %w", err)
	}
	req.ContentLength = int64(len(prefix)) + size + int64(len(suffix))
	req.Header.Set("Content-Type", formType)
	auth, err := c.authHeader(ctx)
	if err != nil {
		return FileInfo{}, err
	}
	if auth != "" {
		// The gateway ties what it captures to the caller, so this request is
		// the same caller as the two around it.
		req.Header.Set("Authorization", auth)
	}
	c.applyJobLabel(req, http.MethodPost)
	req.Header.Set("User-Agent", UserAgent())

	// A redirect here is the answer, not a hop: Canvas's local storage answers
	// the upload with a 302 to create_success, and following it automatically
	// would re-send the whole file.
	uploadClient := *c.HTTPClient
	uploadClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	// The client's timeout covers the whole exchange, body included, and it was
	// set for a request whose body is a line of JSON. A file big enough to
	// outrun it would be cut off mid-stream and reported as an outcome nobody
	// knows, which is the most expensive answer this command has, for a file
	// that was probably on its way in fine.
	uploadClient.Timeout = transferTimeout(uploadClient.Timeout, size)

	resp, err := uploadClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return FileInfo{}, ctx.Err()
		}
		if refused, ok := refusal(err); ok {
			return FileInfo{}, &RefusedError{Cause: refused}
		}
		// The file went out and no answer came back. Whether Canvas kept it
		// isn't knowable from here, and sending it again isn't the answer.
		return FileInfo{}, &OutcomeUnknownError{Method: http.MethodPost, Path: urlPath(ticket.url), Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	return c.uploadResult(ctx, resp, ticket.url)
}

// minTransferRate is the slowest a stalled transfer can be told apart from a
// slow one. Canvas Builder's Canvas is on this machine, where a megabyte a second
// is many times slower than the real thing, so a transfer below it has stopped
// rather than merely being large.
const minTransferRate = 1 << 20

// transferTimeout is how long a file's own bytes are given, in either
// direction: the base timeout, which is what waiting for an answer is worth,
// plus the time the bytes need at a rate only a stalled transfer falls below. Zero stays zero — a caller that
// asked for no bound gets none.
func transferTimeout(base time.Duration, size int64) time.Duration {
	if base <= 0 {
		return 0
	}
	return base + time.Duration(size/minTransferRate)*time.Second
}

// checkUploadURL is step 2's own rule, and it is stricter than the client's
// general one: the only address Canvas Builder takes a file at is its
// /files_api, which is also the only multipart body its gateway accepts from an
// agent. A ticket pointing anywhere else is a ticket to hand the file, and the
// token carrying it, to something outside Canvas Builder.
func (c *Client) checkUploadURL(raw string) error {
	if c.Sandbox == nil {
		return nil
	}
	if err := c.CheckURL(raw); err != nil {
		return &RefusedError{Cause: err}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path != "/files_api" {
		return &RefusedError{Cause: sandbox.NotGateway(c.Sandbox.Gateway, raw)}
	}
	return nil
}

// multipartFraming builds everything that goes before the file's bytes and the
// closing boundary that goes after them. The upload_params come first, in
// sorted key order, because S3-style storage rejects a body whose policy
// fields arrive after the file.
func multipartFraming(ticket uploadTicket, name, contentType string) (prefix, suffix []byte, formType string, err error) {
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)
	keys := make([]string, 0, len(ticket.params))
	for key := range ticket.params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := mw.WriteField(key, ticket.params[key]); err != nil {
			return nil, nil, "", fmt.Errorf("building the upload body: %w", err)
		}
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf("form-data; name=%s; filename=%s", quoteMIME(ticket.fileParam), quoteMIME(name)))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	if _, err := mw.CreatePart(header); err != nil {
		return nil, nil, "", fmt.Errorf("building the upload body: %w", err)
	}
	// Not mw.Close(): that would put the closing boundary in the prefix, ahead
	// of the file. It goes after the file instead.
	return head.Bytes(), []byte("\r\n--" + mw.Boundary() + "--\r\n"), mw.FormDataContentType(), nil
}

// quoteMIME quotes a multipart field name the way mime/multipart does. Its own
// escaper isn't exported.
func quoteMIME(s string) string {
	return `"` + strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s) + `"`
}

// uploadResult is step 3: what Canvas kept. Local storage answers the upload
// with a redirect to create_success, which is the request that returns the
// file; S3-style storage answers 201 with a Location that does the same. The
// answer's body is closed by sendUpload, which opened it.
func (c *Client) uploadResult(ctx context.Context, resp *http.Response, uploadURL string) (FileInfo, error) {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxFileJSON))
	location := resp.Header.Get("Location")

	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400,
		resp.StatusCode == http.StatusCreated && location != "":
		if location == "" {
			return FileInfo{}, &APIError{
				Method: http.MethodPost, Path: urlPath(uploadURL), StatusCode: resp.StatusCode,
				Body: "the upload was redirected without saying where",
			}
		}
		return c.confirmUpload(ctx, uploadURL, location)
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated:
		if readErr != nil {
			return FileInfo{}, fmt.Errorf("reading the answer to the upload: %w", readErr)
		}
		var info FileInfo
		if err := json.Unmarshal(body, &info); err == nil && info.ID != 0 {
			return info, nil
		}
		return FileInfo{}, &APIError{
			Method: http.MethodPost, Path: urlPath(uploadURL), StatusCode: resp.StatusCode,
			Body: "the upload was accepted without naming the file",
		}
	default:
		return FileInfo{}, &APIError{
			Method: http.MethodPost, Path: urlPath(uploadURL), StatusCode: resp.StatusCode, Body: string(body),
		}
	}
}

// confirmUpload follows the address the upload was answered with, which is the
// request that turns an accepted upload into a file.
func (c *Client) confirmUpload(ctx context.Context, uploadURL, location string) (FileInfo, error) {
	target := location
	if base, err := url.Parse(uploadURL); err == nil {
		if resolved, err := base.Parse(location); err == nil {
			target = resolved.String()
		}
	}
	if err := c.CheckURL(target); err != nil {
		return FileInfo{}, &RefusedError{Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return FileInfo{}, fmt.Errorf("creating the request that confirms the upload: %w", err)
	}
	auth, err := c.authHeader(ctx)
	if err != nil {
		return FileInfo{}, err
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	c.applyJobLabel(req, http.MethodGet)
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return FileInfo{}, ctx.Err()
		}
		if refused, ok := refusal(err); ok {
			return FileInfo{}, &RefusedError{Cause: refused}
		}
		// The file is already with Canvas; it is the confirmation that was
		// lost, so the course is still the only place that says what happened.
		return FileInfo{}, &OutcomeUnknownError{Method: http.MethodGet, Path: urlPath(target), Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFileJSON))
	if err != nil {
		return FileInfo{}, fmt.Errorf("reading the file the upload produced: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return FileInfo{}, &APIError{
			Method: http.MethodGet, Path: urlPath(target), StatusCode: resp.StatusCode, Body: string(body),
		}
	}
	var info FileInfo
	if err := json.Unmarshal(body, &info); err != nil || info.ID == 0 {
		return FileInfo{}, &APIError{
			Method: http.MethodGet, Path: urlPath(target), StatusCode: resp.StatusCode,
			Body: "the upload was confirmed without naming the file",
		}
	}
	return info, nil
}

// urlPath is an address's path on its own. A file address carries a verifier in
// its query, which is a credential and doesn't belong in a message.
func urlPath(raw string) string {
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return raw
}
