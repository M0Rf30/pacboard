// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/M0Rf30/pacboard/internal/github"
)

const testKey = "pacboard-unit-test-session-key-0123456789abcdef"

// fakeGitHub answers the OAuth and REST calls pacboard makes.
type fakeGitHub struct {
	srv *httptest.Server

	mu          sync.Mutex
	members     map[string]bool // login -> active member of "acme"
	revoked     bool            // memberships answer 401
	status      int             // when set, memberships answer this status
	membership  int             // membership calls seen
	gate        chan struct{}   // when set, membership answers wait until it is closed
	lastVerifer string
}

func newFakeGitHub(t *testing.T, members map[string]bool) *fakeGitHub {
	t.Helper()

	f := &fakeGitHub{members: members}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		f.mu.Lock()
		f.lastVerifer = r.PostForm.Get("code_verifier")
		f.mu.Unlock()

		if r.PostForm.Get("client_secret") != "shh" || r.PostForm.Get("code") == "bad" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_verification_code"})

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-" + r.PostForm.Get("code")})
	})
	mux.HandleFunc("GET /api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		login := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok-")
		_ = json.NewEncoder(w).Encode(map[string]string{"login": login, "avatar_url": "https://avatars.example/" + login})
	})
	mux.HandleFunc("GET /api/v3/user/memberships/orgs/{org}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.membership++
		revoked, gate, status := f.revoked, f.gate, f.status
		f.mu.Unlock()

		if gate != nil {
			<-gate
		}

		login := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok-")

		switch {
		case status != 0:
			w.WriteHeader(status)
		case revoked:
			w.WriteHeader(http.StatusUnauthorized)
		case r.PathValue("org") == "acme" && f.members[login]:
			_ = json.NewEncoder(w).Encode(map[string]string{"state": "active"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	return f
}

func newService(t *testing.T, f *fakeGitHub) *Service {
	t.Helper()

	gh, err := github.New(f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	s, err := New(context.Background(), Config{
		Mode: ModeGitHub, ClientID: "cid", ClientSecret: "shh", AllowedOrgs: []string{"other", "acme"},
		SessionKey: testKey, PublicURL: "https://pacboard.example.com", GitHub: gh,
		Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}

	return s
}

// app is a guarded API plus the auth routes, as the server wires them.
func app(s *Service) http.Handler {
	mux := http.NewServeMux()
	s.Routes(mux)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := s.Actor(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		_, _ = io.WriteString(w, a.Login)
	})

	mux.Handle("/api/", s.Authenticate(s.RequireCSRF(ok)))
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(s.Me(r))
	})

	return s.EnsureCSRFCookie(mux)
}

func do(h http.Handler, method, target string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}

	return nil
}

// loginAs runs the whole web flow and returns the session and CSRF cookies.
func loginAs(t *testing.T, h http.Handler, f *fakeGitHub, login, next string) (*httptest.ResponseRecorder, []*http.Cookie) {
	t.Helper()

	rec := do(h, http.MethodGet, "/auth/login?next="+url.QueryEscape(next), nil, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d", rec.Code)
	}

	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}

	q := loc.Query()
	if !strings.HasPrefix(rec.Header().Get("Location"), f.srv.URL+"/login/oauth/authorize?") ||
		q.Get("client_id") != "cid" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		q.Get("redirect_uri") != "https://pacboard.example.com/auth/callback" || q.Get("scope") != "read:org repo" {
		t.Fatalf("authorize URL = %s", loc)
	}

	oauth := cookie(rec, cookieOAuth)
	if oauth == nil || !oauth.HttpOnly || !oauth.Secure {
		t.Fatalf("oauth cookie = %+v", oauth)
	}

	cb := do(h, http.MethodGet, "/auth/callback?code="+login+"&state="+url.QueryEscape(q.Get("state")), []*http.Cookie{oauth}, nil)

	var cookies []*http.Cookie
	for _, name := range []string{CookieSession, CookieCSRF} {
		if c := cookie(cb, name); c != nil {
			cookies = append(cookies, c)
		}
	}

	return cb, cookies
}

func TestSafeNext(t *testing.T) {
	tests := map[string]string{
		"":                          "/",
		"/":                         "/",
		"/runs/ci/x?tab=graph#a":    "/runs/ci/x?tab=graph#a",
		"//evil.com":                "/",
		"//evil.com/path":           "/",
		"/\\evil.com":               "/",
		"https://evil.com":          "/",
		"http://evil.com/x":         "/",
		"javascript:alert(1)":       "/",
		"evil.com":                  "/",
		"/ok\r\nSet-Cookie: a=b":    "/",
		"/%2f%2fevil.com":           "/%2f%2fevil.com", // stays a path: browsers do not decode it
		"/a\\b":                     "/",
		"/\x00":                     "/",
		"/https://evil.com":         "/https://evil.com",
		"/@evil.com":                "/@evil.com",
		"/path with space":          "/path with space",
		"\\\\evil.com":              "/",
		"/\t/evil.com":              "/",
		"/ /evil.com":               "/ /evil.com",
		"/runs?next=//evil.com":     "/runs?next=//evil.com",
		"/runs#//evil.com":          "/runs#//evil.com",
		"/\u202e//evil.com":         "/\u202e//evil.com",
		"%2f%2fevil.com":            "/",
		"/../../etc/passwd":         "/../../etc/passwd",
		"/api/v1/me":                "/api/v1/me",
		"/auth/logout":              "/auth/logout",
		"/runs/ci/x?y=%0d%0aSet-Co": "/runs/ci/x?y=%0d%0aSet-Co",
	}

	for in, want := range tests {
		if got := SafeNext(in); got != want {
			t.Errorf("SafeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoginFlowSetsSessionAndRejectsOpenRedirect(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	h := app(newService(t, f))

	for next, want := range map[string]string{
		"https://evil.com":     "/",
		"//evil.com":           "/",
		"/runs/ci/abc?tab=dag": "/runs/ci/abc?tab=dag",
	} {
		cb, cookies := loginAs(t, h, f, "alice", next)
		if cb.Code != http.StatusFound || cb.Header().Get("Location") != want {
			t.Fatalf("next=%q: status %d, location %q, want %q (%s)", next, cb.Code, cb.Header().Get("Location"), want, cb.Body)
		}

		if len(cookies) != 2 {
			t.Fatalf("cookies = %v", cookies)
		}

		sess := cookies[0]
		if sess.Name != CookieSession || !sess.HttpOnly || !sess.Secure || sess.SameSite != http.SameSiteLaxMode {
			t.Fatalf("session cookie = %+v", sess)
		}

		if strings.Contains(sess.Value, "tok-alice") {
			t.Fatal("session cookie leaks the token in clear")
		}

		if rec := do(h, http.MethodGet, "/api/v1/runs", cookies, nil); rec.Code != http.StatusOK || rec.Body.String() != "alice" {
			t.Fatalf("authenticated request = %d %q", rec.Code, rec.Body)
		}
	}

	if f.lastVerifer == "" {
		t.Fatal("PKCE verifier was not sent in the token exchange")
	}
}

func TestCallbackRejections(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	h := app(newService(t, f))

	login := do(h, http.MethodGet, "/auth/login", nil, nil)
	oauth := cookie(login, cookieOAuth)
	state := mustQuery(t, login.Header().Get("Location")).Get("state")

	tests := []struct {
		name    string
		target  string
		cookies []*http.Cookie
		want    int
	}{
		{"no login cookie", "/auth/callback?code=alice&state=" + state, nil, http.StatusBadRequest},
		{"wrong state", "/auth/callback?code=alice&state=nope", []*http.Cookie{oauth}, http.StatusBadRequest},
		{"missing state", "/auth/callback?code=alice", []*http.Cookie{oauth}, http.StatusBadRequest},
		{"denied", "/auth/callback?error=access_denied&state=" + state, []*http.Cookie{oauth}, http.StatusForbidden},
		{"no code", "/auth/callback?state=" + state, []*http.Cookie{oauth}, http.StatusBadRequest},
		{"bad code", "/auth/callback?code=bad&state=" + state, []*http.Cookie{oauth}, http.StatusBadGateway},
		{"not a member", "/auth/callback?code=mallory&state=" + state, []*http.Cookie{oauth}, http.StatusForbidden},
		{
			"forged login cookie",
			"/auth/callback?code=alice&state=" + state,
			[]*http.Cookie{{Name: cookieOAuth, Value: "AAAA"}},
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		rec := do(h, http.MethodGet, tt.target, tt.cookies, nil)
		if rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (%s)", tt.name, rec.Code, tt.want, rec.Body)
		}

		if cookie(rec, CookieSession) != nil && tt.want != http.StatusFound {
			t.Errorf("%s: a session was issued", tt.name)
		}
	}
}

func mustQuery(t *testing.T, location string) url.Values {
	t.Helper()

	u, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}

	return u.Query()
}

func TestAuthenticateGate(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	s := newService(t, f)
	h := app(s)

	// Anonymous.
	if rec := do(h, http.MethodGet, "/api/v1/runs", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", rec.Code)
	}

	// Garbage and wrong-purpose cookies are anonymous too.
	oauthValue, err := s.sealer.seal(purposeOAuth, session{Login: "alice", Token: "tok-alice", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range []string{"garbage", oauthValue} {
		if rec := do(
			h,
			http.MethodGet,
			"/api/v1/runs",
			[]*http.Cookie{{Name: CookieSession, Value: v}},
			nil,
		); rec.Code != http.StatusUnauthorized {
			t.Fatalf("bad cookie %q = %d", v, rec.Code)
		}
	}

	// A valid session of a non-member.
	forged, err := s.sealer.seal(purposeSession, session{Login: "mallory", Token: "tok-mallory", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}

	rec := do(h, http.MethodGet, "/api/v1/runs", []*http.Cookie{{Name: CookieSession, Value: forged}}, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member = %d", rec.Code)
	}

	// Expired session.
	expired, err := s.sealer.seal(purposeSession, session{Login: "alice", Token: "tok-alice", Expires: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}

	if rec := do(
		h,
		http.MethodGet,
		"/api/v1/runs",
		[]*http.Cookie{{Name: CookieSession, Value: expired}},
		nil,
	); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired = %d", rec.Code)
	}

	// /me is always 200 and anonymous for all of the above.
	for _, v := range []string{"", forged, expired} {
		var cs []*http.Cookie
		if v != "" {
			cs = []*http.Cookie{{Name: CookieSession, Value: v}}
		}

		rec := do(h, http.MethodGet, "/api/v1/me", cs, nil)

		var me struct {
			Authenticated bool   `json:"authenticated"`
			AuthMode      string `json:"authMode"`
			CanRerun      bool   `json:"canRerun"`
		}

		if err := json.Unmarshal(
			rec.Body.Bytes(),
			&me,
		); err != nil || rec.Code != 200 || me.Authenticated || me.CanRerun ||
			me.AuthMode != "github" {
			t.Fatalf("me = %d %s", rec.Code, rec.Body)
		}
	}
}

func TestMembershipIsCachedAndRevocationEndsSession(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	s := newService(t, f)
	now := time.Now()
	s.now = func() time.Time { return now }
	h := app(s)

	_, cookies := loginAs(t, h, f, "alice", "/")
	afterLogin := f.membership

	for range 3 {
		if rec := do(h, http.MethodGet, "/api/v1/runs", cookies, nil); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	}

	if f.membership != afterLogin {
		t.Fatalf("membership re-checked within the cache window: %d -> %d", afterLogin, f.membership)
	}

	// After the 10 minute window GitHub is asked again; a revoked token ends the session.
	now = now.Add(membershipTTL + time.Second)
	f.mu.Lock()
	f.revoked = true
	f.mu.Unlock()

	if rec := do(h, http.MethodGet, "/api/v1/runs", cookies, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d", rec.Code)
	}

	// The failure is remembered for a few seconds, not asked again per request.
	calls := f.membership

	if rec := do(h, http.MethodGet, "/api/v1/runs", cookies, nil); rec.Code != http.StatusUnauthorized || f.membership != calls {
		t.Fatalf("repeat within error window = %d, GitHub calls %d -> %d", rec.Code, calls, f.membership)
	}

	// And a removed member is refused after the window.
	now = now.Add(membershipErrorTTL + time.Second)

	f.mu.Lock()
	f.revoked = false
	f.members["alice"] = false
	f.mu.Unlock()

	if rec := do(h, http.MethodGet, "/api/v1/runs", cookies, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("removed member = %d", rec.Code)
	}
}

func TestExpiredMembershipIsCheckedOncePerSession(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	s := newService(t, f)
	now := time.Now()
	s.now = func() time.Time { return now }
	h := app(s)

	_, cookies := loginAs(t, h, f, "alice", "/")
	afterLogin := f.membership
	now = now.Add(membershipTTL + time.Second)

	f.mu.Lock()
	f.gate = make(chan struct{})
	gate := f.gate
	f.mu.Unlock()

	const requests = 8

	codes := make(chan int, requests)

	for range requests {
		go func() { codes <- do(h, http.MethodGet, "/api/v1/runs", cookies, nil).Code }()
	}

	// One request is at GitHub; give the others time to pile up behind it.
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := f.membership
		f.mu.Unlock()

		if n > afterLogin {
			break
		}

		time.Sleep(time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	close(gate)

	for range requests {
		if code := <-codes; code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
	}

	// A single re-check walks the allowed orgs once ("other" says no, "acme" yes).
	if got := f.membership - afterLogin; got != 2 {
		t.Fatalf("GitHub membership calls for %d concurrent requests = %d, want 2", requests, got)
	}
}

func TestCSRFDoubleSubmit(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	h := app(newService(t, f))

	_, cookies := loginAs(t, h, f, "alice", "/")
	csrf := cookies[1].Value

	tests := []struct {
		name    string
		method  string
		target  string
		headers map[string]string
		want    int
	}{
		{"POST without header", http.MethodPost, "/api/v1/runs/ci/a/rerun", nil, http.StatusForbidden},
		{"POST with wrong header", http.MethodPost, "/api/v1/runs/ci/a/rerun", map[string]string{HeaderCSRF: "nope"}, http.StatusForbidden},
		{"POST with matching header", http.MethodPost, "/api/v1/runs/ci/a/rerun", map[string]string{HeaderCSRF: csrf}, http.StatusOK},
		{"GET needs no header", http.MethodGet, "/api/v1/runs", nil, http.StatusOK},
		{"logout without header", http.MethodPost, "/auth/logout", nil, http.StatusForbidden},
		{"logout with header", http.MethodPost, "/auth/logout", map[string]string{HeaderCSRF: csrf}, http.StatusNoContent},
	}

	for _, tt := range tests {
		if rec := do(h, tt.method, tt.target, cookies, tt.headers); rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d", tt.name, rec.Code, tt.want)
		}
	}

	// A header without the cookie is no proof either.
	if rec := do(h, http.MethodPost, "/auth/logout", nil, map[string]string{HeaderCSRF: "x"}); rec.Code != http.StatusForbidden {
		t.Errorf("header without cookie = %d", rec.Code)
	}

	// The cookie is handed out to clients that have none.
	rec := do(h, http.MethodGet, "/api/v1/me", nil, nil)
	if c := cookie(rec, CookieCSRF); c == nil || c.HttpOnly || c.Value == "" {
		t.Fatalf("csrf cookie = %+v", c)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	h := app(newService(t, f))

	_, cookies := loginAs(t, h, f, "alice", "/")
	rec := do(h, http.MethodPost, "/auth/logout", cookies, map[string]string{HeaderCSRF: cookies[1].Value})

	if c := cookie(rec, CookieSession); c == nil || c.MaxAge >= 0 || c.Value != "" {
		t.Fatalf("session cookie after logout = %+v", c)
	}

	if c := cookie(rec, CookieCSRF); c == nil || c.MaxAge >= 0 || c.Value != "" {
		t.Fatalf("csrf cookie after logout = %+v", c)
	}
}

func TestNewValidation(t *testing.T) {
	gh, _ := github.New("")
	base := Config{
		Mode:         ModeGitHub,
		ClientID:     "a",
		ClientSecret: "b",
		AllowedOrgs:  []string{"o"},
		SessionKey:   testKey,
		PublicURL:    "https://x",
		GitHub:       gh,
	}

	tests := map[string]func(*Config){
		"short key":      func(c *Config) { c.SessionKey = "short" },
		"no orgs":        func(c *Config) { c.AllowedOrgs = nil },
		"no client id":   func(c *Config) { c.ClientID = "" },
		"no secret":      func(c *Config) { c.ClientSecret = "" },
		"bad public url": func(c *Config) { c.PublicURL = "pacboard.example.com" },
		"unknown mode":   func(c *Config) { c.Mode = "magic" },
	}

	for name, mutate := range tests {
		cfg := base
		mutate(&cfg)

		if _, err := New(context.Background(), cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	if _, err := New(context.Background(), base); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}

func TestNoneAndDevTokenModes(t *testing.T) {
	f := newFakeGitHub(t, nil)
	gh, err := github.New(f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	none, err := New(context.Background(), Config{Mode: ModeNone, GitHub: gh})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)

	if me := none.Me(req); me.Authenticated || me.CanRerun || me.AuthMode != "none" {
		t.Fatalf("none me = %+v", me)
	}

	if _, err := none.Actor(req); err == nil {
		t.Fatal("none mode must not allow reruns")
	}

	// The gate is a pass-through; login is disabled; logout works without a sealer.
	h := app(none)
	if rec := do(h, http.MethodGet, "/api/v1/runs", nil, nil); rec.Code != http.StatusInternalServerError {
		t.Fatalf("passes through to the handler, which has no actor: %d", rec.Code)
	}

	if rec := do(h, http.MethodGet, "/auth/login", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("login in none mode = %d", rec.Code)
	}

	dev, err := New(context.Background(), Config{Mode: ModeNone, GitHub: gh, DevToken: "tok-devuser"})
	if err != nil {
		t.Fatal(err)
	}

	me := dev.Me(req)
	if !me.Authenticated || me.Login != "devuser" || !me.CanRerun || me.AuthMode != "none" {
		t.Fatalf("dev me = %+v", me)
	}

	if a, err := dev.Actor(req); err != nil || a.Token != "tok-devuser" || a.Login != "devuser" {
		t.Fatalf("dev actor = %+v, %v", a, err)
	}
}

func TestStreamRevalidationAndSessionRemaining(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	s := newService(t, f)
	now := time.Now()
	s.now = func() time.Time { return now }
	h := app(s)

	_, cookies := loginAs(t, h, f, "alice", "/")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	remaining, ok := s.SessionRemaining(req)
	if !ok || remaining < sessionTTL-time.Second || remaining > sessionTTL+time.Second {
		t.Fatalf("remaining = %v, %v", remaining, ok)
	}

	// Within the membership window the stream is re-checked from the cache.
	calls := f.membership

	if !s.Revalidate(req) || f.membership != calls {
		t.Fatalf("revalidate within the cache window: GitHub calls %d -> %d", calls, f.membership)
	}

	// After the window GitHub is asked; a failing check does not end the stream, a removal does.
	now = now.Add(membershipTTL + time.Second)

	f.mu.Lock()
	f.members["alice"] = false
	f.mu.Unlock()

	if s.Revalidate(req) {
		t.Fatal("removed member kept the stream")
	}

	// An expired session ends it, too.
	now = now.Add(sessionTTL + 2*time.Second)

	if s.Revalidate(req) {
		t.Fatal("expired session kept the stream")
	}

	if _, ok := s.SessionRemaining(req); ok {
		t.Fatal("expired session has remaining time")
	}
}

func TestStreamRevalidationSurvivesGitHubErrors(t *testing.T) {
	f := newFakeGitHub(t, map[string]bool{"alice": true})
	s := newService(t, f)
	now := time.Now()
	s.now = func() time.Time { return now }
	h := app(s)

	_, cookies := loginAs(t, h, f, "alice", "/")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}

	now = now.Add(membershipTTL + time.Second)

	f.mu.Lock()
	f.status = http.StatusBadGateway
	f.mu.Unlock()

	if !s.Revalidate(req) {
		t.Fatal("a GitHub outage ended the stream")
	}
}

func TestModeNoneStreamsNeverExpire(t *testing.T) {
	s, err := New(context.Background(), Config{Mode: ModeNone})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)

	if _, ok := s.SessionRemaining(req); ok || !s.Revalidate(req) {
		t.Fatal("mode none has no sessions to expire")
	}
}
