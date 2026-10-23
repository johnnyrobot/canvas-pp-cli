// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored: taking a file out of a course. Like an upload this is two
// requests rather than one — ask the course where the bytes are, then fetch
// them — and the address for the second request comes from Canvas, which an
// agent can write to. So it is checked twice: it has to be Canvas Builder's,
// and it has to be a file download.
//
// The one failure this has that a request carrying JSON doesn't is a body that
// stops partway. Half a file on disk reads like a whole one, so what arrived is
// counted against what the course said the file weighs.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// fileDownloadPath is what a course file's own address looks like: Canvas hands
// out /files/<id>/download and /courses/<id>/files/<id>/download, with the
// verifier in the query. Anything else on the gateway — an API path, a page —
// is not a file, and this CLI's token doesn't go there on Canvas's word.
var fileDownloadPath = regexp.MustCompile(`^/(courses/\d+/)?files/\d+/download$`)

// ErrDryRun says the request was shown instead of sent, so there is no file to
// write. A download can't be previewed past its first request: the address the
// bytes come from is in the answer to that request, which a dry run never gets.
var ErrDryRun = errors.New("the request was shown and not sent")

// DownloadFile writes one course file to dst and returns the file's record and
// how many bytes arrived. Nothing is written unless the address the course gave
// passes both checks, and a body that stops short is an error, not a file.
func (c *Client) DownloadFile(ctx context.Context, courseID, fileID int64, dst io.Writer) (FileInfo, int64, error) {
	path := "/api/v1/courses/" + strconv.FormatInt(courseID, 10) + "/files/" + strconv.FormatInt(fileID, 10)
	// Not the cache: a file's address carries a verifier that expires, so a
	// remembered record sends the fetch at a dead address.
	raw, err := c.GetNoCache(ctx, path, nil)
	if err != nil {
		return FileInfo{}, 0, err
	}
	if c.DryRun {
		return FileInfo{}, 0, ErrDryRun
	}
	var info FileInfo
	if err := json.Unmarshal(raw, &info); err != nil || info.ID == 0 {
		return FileInfo{}, 0, &APIError{
			Method: http.MethodGet, Path: path, StatusCode: http.StatusOK,
			Body: "the course answered without naming the file",
		}
	}
	if info.URL == "" {
		return info, 0, &APIError{
			Method: http.MethodGet, Path: path, StatusCode: http.StatusOK,
			Body: "the file record doesn't say where its contents are",
		}
	}
	if err := c.checkDownloadURL(info.URL); err != nil {
		return info, 0, err
	}

	n, err := c.fetchInto(ctx, info, dst)
	if err != nil {
		return info, n, err
	}
	// Canvas states a file's size in its record, which makes a short body
	// something the CLI can see rather than something the next command trips
	// over. A body with no length at all can't be checked this way, and
	// io.Copy's own error is the only signal there is.
	if info.Size > 0 && n != info.Size {
		if n < info.Size {
			return info, n, fmt.Errorf("%s (%d of %d bytes)", sandbox.MsgDownloadShort, n, info.Size)
		}
		return info, n, fmt.Errorf("%s (%d bytes, and the file is %d)",
			"Canvas sent more of this file than its record says it holds, so nothing was saved.", n, info.Size)
	}
	return info, n, nil
}

// checkDownloadURL holds the address Canvas named to the same rule as every
// other request, and then to one of its own.
func (c *Client) checkDownloadURL(raw string) error {
	if c.Sandbox == nil {
		return nil
	}
	if err := c.CheckURL(raw); err != nil {
		return &RefusedError{Cause: err}
	}
	parsed, err := url.Parse(raw)
	if err != nil || !fileDownloadPath.MatchString(parsed.Path) {
		return &RefusedError{Cause: sandbox.NewError(sandbox.MsgNotAFileAddress)}
	}
	return nil
}

// fetchInto streams the file. Redirects follow the client's own rule, which in
// Canvas Builder refuses a hop off the gateway before it is dialed, so a file
// address that answers with somewhere else gets no further than the check.
func (c *Client) fetchInto(ctx context.Context, info FileInfo, dst io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating the request that fetches the file: %w", err)
	}
	auth, err := c.authHeader(ctx)
	if err != nil {
		return 0, err
	}
	// The address came out of the file's record, and outside Canvas Builder
	// nothing holds it to the gateway. A file store's address carries its own
	// verifier, so the token goes only to the Canvas this CLI was pointed at.
	if auth != "" && c.authAllowed(info.URL) {
		req.Header.Set("Authorization", auth)
	}
	c.applyJobLabel(req, http.MethodGet)
	req.Header.Set("User-Agent", UserAgent())

	// The client's timeout was set for a request whose body is a line of JSON,
	// and it covers the whole exchange. A file big enough to outrun it would be
	// cut off mid-stream and reported as a short download, which is wrong
	// twice: the file is whole, and the reason given isn't the real one.
	fetchClient := *c.HTTPClient
	fetchClient.Timeout = transferTimeout(fetchClient.Timeout, info.Size)

	resp, err := fetchClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if refused, ok := refusal(err); ok {
			return 0, &RefusedError{Cause: refused}
		}
		// Nothing was written and nothing changed in the course: a download
		// that doesn't arrive can simply be asked for again.
		return 0, fmt.Errorf("fetching %s: %w", urlPath(info.URL), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxFileJSON))
		return 0, &APIError{
			Method: http.MethodGet, Path: urlPath(info.URL), StatusCode: resp.StatusCode, Body: string(body),
		}
	}

	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		if refused, ok := refusal(err); ok {
			return n, &RefusedError{Cause: refused}
		}
		return n, fmt.Errorf("%s (%d bytes arrived): %w", sandbox.MsgDownloadShort, n, err)
	}
	return n, nil
}
