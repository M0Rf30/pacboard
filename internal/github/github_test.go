// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type seen struct {
	method, escapedPath, auth, body string
}

func newTestClient(t *testing.T, status int, reply string, got *seen) *Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = seen{r.Method, r.URL.EscapedPath(), r.Header.Get("Authorization"), string(b)}

		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestPostCommentEscapesPathAndSendsBody(t *testing.T) {
	var got seen

	c := newTestClient(t, http.StatusCreated, `{"html_url":"https://github.com/o/r/pull/7#issuecomment-1"}`, &got)

	url, err := c.PostComment(context.Background(), "tok", "or g/..", "re%po?x#y", 7, "/retest p")
	if err != nil {
		t.Fatal(err)
	}

	if url != "https://github.com/o/r/pull/7#issuecomment-1" {
		t.Fatalf("url = %q", url)
	}

	if got.method != http.MethodPost || got.auth != "Bearer tok" {
		t.Fatalf("request = %+v", got)
	}

	// Slashes, spaces and URL metacharacters stay inside their own path segment.
	if want := "/api/v3/repos/or%20g%2F../re%25po%3Fx%23y/issues/7/comments"; got.escapedPath != want {
		t.Fatalf("path = %q, want %q", got.escapedPath, want)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(got.body), &body); err != nil || body["body"] != "/retest p" {
		t.Fatalf("body = %q (%v)", got.body, err)
	}
}

func TestPostCommentErrorMapping(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusBadGateway} {
		var got seen

		c := newTestClient(t, status, `{"message":"nope"}`, &got)

		url, err := c.PostComment(context.Background(), "tok", "o", "r", 1, "x")

		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != status || apiErr.Message != "nope" || url != "" {
			t.Errorf("status %d: url %q err %v", status, url, err)
		}
	}

	// A non-JSON error body still maps to the status.
	var got seen

	c := newTestClient(t, http.StatusServiceUnavailable, `<html>down</html>`, &got)

	var apiErr *APIError
	if _, err := c.PostComment(context.Background(), "tok", "o", "r", 1, "x"); !errors.As(err, &apiErr) ||
		apiErr.Status != http.StatusServiceUnavailable || apiErr.Message != "" {
		t.Errorf("non-JSON error: %v", err)
	}

	// A transport failure is not an APIError.
	c.APIURL = "http://127.0.0.1:1"

	if _, err := c.PostComment(context.Background(), "tok", "o", "r", 1, "x"); err == nil || errors.As(err, &apiErr) && apiErr.Status == 0 {
		t.Errorf("transport error = %v", err)
	}
}
