// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package github is the small slice of the GitHub REST and OAuth API pacboard uses: the web
// login, the organization membership check and posting rerun comments as the user.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultWebURL is public GitHub.
	DefaultWebURL = "https://github.com"

	maxBody       = 1 << 20
	clientTimeout = 15 * time.Second
)

// APIError is a non-success answer from GitHub. Message is for logs, not for clients.
type APIError struct {
	Status  int
	Message string
	// RateLimited is set when the answer carries a rate-limit signal (Retry-After, or
	// X-RateLimit-Remaining: 0): the request was throttled, not refused.
	RateLimited bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github: HTTP %d: %s", e.Status, e.Message)
}

// Client talks to one GitHub instance (github.com or Enterprise Server / Cloud).
type Client struct {
	WebURL string // https://github.com, login and token endpoints
	APIURL string // https://api.github.com, REST API
	HTTP   *http.Client
}

// APIURLFor derives the REST API base from the web URL: api.github.com for github.com,
// api.<sub>.ghe.com for GHE Cloud with data residency, <url>/api/v3 for Enterprise Server.
func APIURLFor(webURL string) (string, error) {
	u, err := url.Parse(strings.TrimSuffix(webURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid GitHub URL %q", webURL)
	}

	switch host := strings.ToLower(u.Hostname()); {
	case host == "github.com":
		return "https://api.github.com", nil
	case strings.HasSuffix(host, ".ghe.com"):
		return u.Scheme + "://api." + u.Host, nil
	default:
		return strings.TrimSuffix(u.String(), "/") + "/api/v3", nil
	}
}

// New returns a client for webURL (default github.com).
func New(webURL string) (*Client, error) {
	if webURL == "" {
		webURL = DefaultWebURL
	}

	webURL = strings.TrimSuffix(webURL, "/")

	api, err := APIURLFor(webURL)
	if err != nil {
		return nil, err
	}

	return &Client{WebURL: webURL, APIURL: api, HTTP: &http.Client{Timeout: clientTimeout}}, nil
}

// User is the authenticated GitHub user.
type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("github response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Message string `json:"message"`
		}

		_ = json.Unmarshal(body, &e)

		limited := resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-Ratelimit-Remaining") == "0"

		return &APIError{Status: resp.StatusCode, Message: e.Message, RateLimited: limited}
	}

	if out == nil {
		return nil
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("github response: %w", err)
	}

	return nil
}

func (c *Client) api(ctx context.Context, token, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.APIURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Github-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "pacboard")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// AuthorizeURL is where the browser is sent to log in.
func (c *Client) AuthorizeURL(clientID, redirectURI, scope, state, challenge string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	return c.WebURL + "/login/oauth/authorize?" + q.Encode()
}

// ExchangeCode trades the authorization code (and PKCE verifier) for a user token.
func (c *Client) ExchangeCode(ctx context.Context, clientID, clientSecret, code, redirectURI, verifier string) (string, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebURL+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("github request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "pacboard")

	// GitHub answers 200 with {"error": ...} for a bad code.
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}

	if err := c.do(req, &out); err != nil {
		return "", err
	}

	if out.AccessToken == "" {
		return "", fmt.Errorf("github token exchange: %s: %s", out.Error, out.Description) //nolint:err113 // message from GitHub
	}

	return out.AccessToken, nil
}

// User returns the user the token belongs to.
func (c *Client) User(ctx context.Context, token string) (User, error) {
	req, err := c.api(ctx, token, http.MethodGet, "/user", nil)
	if err != nil {
		return User{}, err
	}

	var u User
	if err := c.do(req, &u); err != nil {
		return User{}, err
	}

	return u, nil
}

// IsActiveMember reports whether the token's user is an active member of org. A user
// outside the organization (404) or one the token cannot see into (403) is not a member;
// 401 (bad token), rate-limited answers (403/429 with Retry-After or X-RateLimit-Remaining: 0)
// and transport errors are returned.
func (c *Client) IsActiveMember(ctx context.Context, token, org string) (bool, error) {
	req, err := c.api(ctx, token, http.MethodGet, "/user/memberships/orgs/"+url.PathEscape(org), nil)
	if err != nil {
		return false, err
	}

	var m struct {
		State string `json:"state"`
	}

	if err := c.do(req, &m); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && !apiErr.RateLimited &&
			(apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusForbidden) {
			return false, nil
		}

		return false, err
	}

	return m.State == "active", nil
}

// PostComment adds an issue (pull request) comment as the token's user and returns its URL.
func (c *Client) PostComment(ctx context.Context, token, org, repo string, number int, body string) (string, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return "", fmt.Errorf("encode comment: %w", err)
	}

	path := "/repos/" + url.PathEscape(org) + "/" + url.PathEscape(repo) + "/issues/" + strconv.Itoa(number) + "/comments"

	req, err := c.api(ctx, token, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}

	var out struct {
		HTMLURL string `json:"html_url"`
	}

	if err := c.do(req, &out); err != nil {
		return "", err
	}

	return out.HTMLURL, nil
}
