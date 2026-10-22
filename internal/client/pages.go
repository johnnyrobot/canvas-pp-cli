// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored. Reading a whole list out of Canvas.
//
// Canvas answers a collection a page at a time and says where the rest is in a
// Link header, so a caller that reads only the first answer silently gets a
// course with the first hundred pages in it. Everything built on top of this
// (the course snapshot, and the checks that read it) would then report a clean
// course it never fully saw, which is why a list that came back short is
// returned as an error and never as a list.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/johnnyrobot/canvas-pp-cli/internal/sandbox"
)

// maxPages is where a Canvas that keeps promising one more page is stopped.
// At the per_page=100 asked for below that is 10,000 items of one kind, far
// more than any real course holds.
const maxPages = 100

// ErrTooManyPages says the list was longer than the CLI will read.
var ErrTooManyPages = sandbox.NewError(sandbox.MsgTooManyPages)

// GetAll reads every page of a Canvas collection and returns the items in the
// order Canvas listed them. The error is all-or-nothing: on any failure, no
// items come back, because a caller cannot tell a short list from a complete
// one.
//
// It goes through the transport directly rather than through Get, because the
// response cache is keyed on path and params: paging through it could answer
// one page from an older read and mix two different courses together.
func (c *Client) GetAll(ctx context.Context, path string, params map[string]string) ([]json.RawMessage, error) {
	all := []json.RawMessage{}
	next := path
	nextParams := map[string]string{"per_page": "100"}
	for k, v := range params {
		nextParams[k] = v
	}
	for pages := 0; ; pages++ {
		if pages == maxPages {
			return nil, &RefusedError{Cause: ErrTooManyPages}
		}
		body, _, header, err := c.doWithHeader(ctx, http.MethodGet, next, nextParams, nil, nil)
		if err != nil {
			return nil, err
		}
		if c.DryRun {
			// The first request was printed, not sent, so there is no page to
			// follow and no list to return.
			return nil, ErrDryRun
		}
		var page []json.RawMessage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("reading the list Canvas sent for %s: %w", path, err)
		}
		all = append(all, page...)

		raw := nextLink(header)
		if raw == "" {
			return all, nil
		}
		next, err = c.pathOf(raw)
		if err != nil {
			return nil, err
		}
		// Params already live in the address Canvas gave, including repeated
		// include[] keys a map cannot hold.
		nextParams = nil
	}
}

// pathOf turns the address of the next page into a request path for this
// client, and refuses one that doesn't belong to the Canvas being read.
func (c *Client) pathOf(raw string) (string, error) {
	// Inside Canvas Builder this answers with the gateway's own wording, so an
	// agent reads the same sentence here as anywhere else it stepped outside.
	if err := c.CheckURL(raw); err != nil {
		return "", &RefusedError{Cause: err}
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", fmt.Errorf("the Canvas address this CLI is pointed at isn't an address: %w", err)
	}
	// Holds in every mode, Canvas Builder or not: a list is read from the
	// Canvas the CLI was pointed at and nowhere else. An address that doesn't
	// parse at all fails the same way, with the same sentence.
	next, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(next.Scheme, base.Scheme) || !strings.EqualFold(next.Host, base.Host) {
		return "", &RefusedError{Cause: sandbox.NotTheCanvasRead(raw)}
	}
	p := strings.TrimPrefix(next.Path, base.Path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if next.RawQuery != "" {
		p += "?" + next.RawQuery
	}
	return p, nil
}

// nextLink reads the address of the next page out of a Link header, ignoring
// the rel="last" and rel="prev" that sit beside it. A response may send the
// header more than once.
func nextLink(header http.Header) string {
	for _, h := range header.Values("Link") {
		for _, link := range splitLinks(h) {
			raw, rel := parseLink(link)
			if rel == "next" && raw != "" {
				return raw
			}
		}
	}
	return ""
}

// splitLinks splits a Link header on the commas between links, leaving the
// commas inside an <address> alone.
func splitLinks(h string) []string {
	var out []string
	depth, start := 0, 0
	for i, ch := range h {
		switch ch {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, h[start:i])
				start = i + 1
			}
		}
	}
	return append(out, h[start:])
}

// parseLink reads one link's address and its rel, which Canvas writes quoted.
func parseLink(link string) (raw, rel string) {
	for _, part := range strings.Split(link, ";") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">"):
			raw = part[1 : len(part)-1]
		case strings.HasPrefix(part, "rel="):
			rel = strings.Trim(strings.TrimPrefix(part, "rel="), `"`)
		}
	}
	return raw, rel
}
