// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored tests for reading a whole list out of Canvas: how the next page
// is found, where it is allowed to come from, and what a failure part-way
// through means for the items already gathered.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/johnnyrobot/canvas-pp-cli/internal/gatewaytest"
)

// listOf answers one Canvas collection a page at a time. Nothing in the body
// says there is more to come: the address of the next page is in a Link header,
// next to a rel="last" the client has to ignore.
func listOf(f *gatewaytest.Fake, path string, total int) {
	f.Handle(http.MethodGet, path, func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if v := r.URL.Query().Get("page"); v != "" {
			page, _ = strconv.Atoi(v)
		}
		perPage := 100
		if v := r.URL.Query().Get("per_page"); v != "" {
			perPage, _ = strconv.Atoi(v)
		}
		last := (total + perPage - 1) / perPage
		if last < 1 {
			last = 1
		}
		if page < last {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d&per_page=%d>; rel="next", <%s%s?page=%d&per_page=%d>; rel="last"`,
				f.URL, path, page+1, perPage, f.URL, path, last, perPage))
		}
		items := []string{}
		for i := (page-1)*perPage + 1; i <= page*perPage && i <= total; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d}`, i))
		}
		_, _ = fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	})
}

// ids decodes what GetAll gathered down to the one field these lists carry.
func ids(t *testing.T, items []json.RawMessage) []int64 {
	t.Helper()
	out := make([]int64, 0, len(items))
	for _, raw := range items {
		var item struct{ ID int64 }
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("decoding %s: %v", raw, err)
		}
		out = append(out, item.ID)
	}
	return out
}

// [Review Focus 4] A course with more items than fit in one answer has to come
// back whole. Canvas says where the rest is only in the Link header.
func TestGetAllFollowsLinks(t *testing.T) {
	f := gatewaytest.New(t)
	listOf(f, "/api/v1/courses/7/pages", 250)
	c := normalClient(t, f.URL, 5*time.Second)

	items, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil)
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	got := ids(t, items)
	if len(got) != 250 {
		t.Fatalf("got %d items, want 250", len(got))
	}
	if got[0] != 1 || got[249] != 250 {
		t.Fatalf("first and last ids are %d and %d, want 1 and 250", got[0], got[249])
	}
	reqs := f.Requests()
	if len(reqs) != 3 {
		t.Fatalf("%d requests, want 3 (100 + 100 + 50)", len(reqs))
	}
	for _, r := range reqs {
		if !strings.Contains(r.RawQuery, "per_page=100") {
			t.Fatalf("request %s carried %q, want per_page=100", r.Path, r.RawQuery)
		}
	}
}

// [Review Focus 4] A list that came back short is worse than no list: the
// checks would report a clean course because they never saw half of it.
func TestGetAllFailsOnAnyPage(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			gatewaytest.Refuse(w, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/courses/7/pages?page=2&per_page=100>; rel="next"`, f.URL))
		items := make([]string, 0, 100)
		for i := 1; i <= 100; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d}`, i))
		}
		_, _ = fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
	})
	c := normalClient(t, f.URL, 5*time.Second)

	items, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil)
	if err == nil {
		t.Fatal("want an error when a page in the middle of the list fails")
	}
	if items != nil {
		t.Fatalf("got %d items back with the error, want none", len(items))
	}
}

// [Review Focus 3] The next page's address comes out of an answer, so it is
// checked like any other address Canvas hands over.
func TestGetAllRefusesNextOffGateway(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/courses/7/pages?page=2&per_page=100>; rel="next"`, other))
		_, _ = w.Write([]byte(`[{"id":1}]`))
	})
	c := sandboxClient(t, f, gatewaytest.WorkspaceOpts{}, 5*time.Second)

	if _, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil); err == nil {
		t.Fatal("want a refusal for a next page off the gateway")
	} else if !strings.Contains(err.Error(), "only talks to Canvas Builder at") {
		t.Fatalf("error = %q, want the Canvas Builder wording", err)
	}
	if count() != 0 {
		t.Fatalf("the other server saw %d requests, want none", count())
	}
}

// The same rule holds outside Canvas Builder, where there is no gateway: a list
// is read from the Canvas the CLI was pointed at and nowhere else.
func TestGetAllRefusesNextOffBaseURLOutsideCanvasBuilder(t *testing.T) {
	f := gatewaytest.New(t)
	other, count := elsewhere(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/courses/7/pages?page=2&per_page=100>; rel="next"`, other))
		_, _ = w.Write([]byte(`[{"id":1}]`))
	})
	c := normalClient(t, f.URL, 5*time.Second)

	if _, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil); err == nil {
		t.Fatal("want an error for a next page pointing at another server")
	}
	if count() != 0 {
		t.Fatalf("the other server saw %d requests, want none", count())
	}
}

// A Canvas that keeps promising one more page has to stop somewhere, and it
// stops with an error rather than a list that is missing the rest.
func TestGetAllStopsAtTheCap(t *testing.T) {
	f := gatewaytest.New(t)
	f.Handle(http.MethodGet, "/api/v1/courses/7/pages", func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if v := r.URL.Query().Get("page"); v != "" {
			page, _ = strconv.Atoi(v)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/courses/7/pages?page=%d&per_page=100>; rel="next"`, f.URL, page+1))
		_, _ = fmt.Fprintf(w, `[{"id":%d}]`, page)
	})
	c := normalClient(t, f.URL, 5*time.Second)

	_, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil)
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("error = %v, want ErrTooManyPages", err)
	}
	if n := len(f.Requests()); n != 100 {
		t.Fatalf("%d requests, want the 100-page cap", n)
	}
}

// A dry run has nothing to page through: the first request was only printed.
func TestGetAllSaysSoOnADryRun(t *testing.T) {
	f := gatewaytest.New(t)
	listOf(f, "/api/v1/courses/7/pages", 10)
	c := normalClient(t, f.URL, 5*time.Second)
	c.DryRun = true

	if _, err := c.GetAll(context.Background(), "/api/v1/courses/7/pages", nil); !errors.Is(err, ErrDryRun) {
		t.Fatalf("error = %v, want ErrDryRun", err)
	}
}
