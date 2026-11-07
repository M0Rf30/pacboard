// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package auth implements pacboard's access control: the GitHub OAuth web flow with an
// encrypted cookie session, the organization membership gate, CSRF protection for POSTs,
// and the "none" modes (read-only, or acting as a developer token).
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/M0Rf30/pacboard/internal/github"
	"github.com/M0Rf30/pacboard/internal/httpx"
	"github.com/M0Rf30/pacboard/internal/model"
)

// Auth modes.
const (
	ModeGitHub = "github"
	ModeNone   = "none"
)

// Cookie and header names of the contract.
const (
	CookieSession = "pacboard_session"
	CookieCSRF    = "pacboard_csrf"
	HeaderCSRF    = "X-Pacboard-CSRF"
	cookieOAuth   = "pacboard_oauth"
)

const (
	// OAuth App scopes: read:org for the membership check, repo to comment on private
	// repositories. A GitHub App's user authorization ignores scopes.
	oauthScope = "read:org repo"

	sessionTTL    = 24 * time.Hour
	oauthTTL      = 10 * time.Minute
	membershipTTL = 10 * time.Minute
	pruneAbove    = 256
)

// ErrUnauthenticated and ErrForbidden are the two reasons Actor refuses.
var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrForbidden       = errors.New("not allowed")
)

// Config configures the Service.
type Config struct {
	Mode         string
	ClientID     string
	ClientSecret string
	AllowedOrgs  []string
	SessionKey   string
	// PublicURL is the externally visible base URL; the OAuth callback is PublicURL + /auth/callback.
	PublicURL string
	// DevToken, in mode none, makes pacboard act as the token's user for reruns.
	DevToken string
	GitHub   *github.Client
	Logger   *slog.Logger
	Now      func() time.Time
}

// Actor is who a request acts as: a GitHub login and the token comments are posted with.
type Actor struct {
	Login     string
	AvatarURL string
	Token     string
}

type actorKey struct{}

// Service is the authentication and authorization layer.
type Service struct {
	cfg    Config
	log    *slog.Logger
	sealer *sealer
	secure bool
	dev    *Actor
	now    func() time.Time

	mu      sync.Mutex
	members map[string]time.Time // login -> membership verified until
}

// New validates cfg. In mode none with a DevToken it resolves the token's user from GitHub.
func New(ctx context.Context, cfg Config) (*Service, error) {
	s := &Service{cfg: cfg, log: cfg.Logger, now: cfg.Now, members: map[string]time.Time{}}
	if s.log == nil {
		s.log = slog.Default()
	}

	if s.now == nil {
		s.now = time.Now
	}

	switch cfg.Mode {
	case ModeNone:
		if cfg.DevToken != "" {
			u, err := cfg.GitHub.User(ctx, cfg.DevToken)
			if err != nil {
				return nil, fmt.Errorf("dev GitHub token rejected: %w", err)
			}

			s.dev = &Actor{Login: u.Login, AvatarURL: u.AvatarURL, Token: cfg.DevToken}
		}

		return s, nil
	case ModeGitHub:
		if err := s.initGitHub(); err != nil {
			return nil, err
		}

		return s, nil
	default:
		return nil, fmt.Errorf("unknown auth mode %q (want %q or %q)", cfg.Mode, ModeGitHub, ModeNone)
	}
}

func (s *Service) initGitHub() error {
	cfg := s.cfg

	switch {
	case cfg.ClientID == "" || cfg.ClientSecret == "":
		return errors.New("auth mode github needs a GitHub client ID and client secret")
	case len(cfg.AllowedOrgs) == 0:
		return errors.New("auth mode github needs at least one allowed organization")
	case cfg.GitHub == nil:
		return errors.New("auth mode github needs a GitHub client")
	}

	u, err := url.Parse(cfg.PublicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("auth mode github needs a valid public URL, got %q", cfg.PublicURL)
	}

	s.secure = u.Scheme == "https"

	s.sealer, err = newSealer(cfg.SessionKey)

	return err
}

// Mode returns the auth mode.
func (s *Service) Mode() string { return s.cfg.Mode }

// DevLogin returns the login rerun comments are posted as in dev token mode, "" otherwise.
func (s *Service) DevLogin() string {
	if s.dev == nil {
		return ""
	}

	return s.dev.Login
}

func (s *Service) callbackURL() string {
	return strings.TrimSuffix(s.cfg.PublicURL, "/") + "/auth/callback"
}

func (s *Service) setCookie(w http.ResponseWriter, c *http.Cookie) {
	c.Secure = s.secure
	c.SameSite = http.SameSiteLaxMode
	http.SetCookie(w, c)
}

func (s *Service) clearCookie(w http.ResponseWriter, name, path string) {
	s.setCookie(w, &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true})
}

// Routes registers /auth/login, /auth/callback and /auth/logout.
func (s *Service) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", s.login)
	mux.HandleFunc("GET /auth/callback", s.callback)
	mux.Handle("POST /auth/logout", s.RequireCSRF(http.HandlerFunc(s.logout)))
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Mode != ModeGitHub {
		httpx.Error(w, http.StatusNotFound, "login is disabled")

		return
	}

	state, err1 := randomToken()
	verifier, err2 := randomToken()

	if err1 != nil || err2 != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")

		return
	}

	value, err := s.sealer.seal(purposeOAuth, oauthState{
		State:    state,
		Verifier: verifier,
		Next:     SafeNext(r.URL.Query().Get("next")),
		Expires:  s.now().Add(oauthTTL).Unix(),
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")

		return
	}

	s.setCookie(w, &http.Cookie{Name: cookieOAuth, Value: value, Path: "/auth", MaxAge: int(oauthTTL.Seconds()), HttpOnly: true})

	challenge := sha256.Sum256([]byte(verifier))
	http.Redirect(w, r, s.cfg.GitHub.AuthorizeURL(s.cfg.ClientID, s.callbackURL(), oauthScope, state,
		base64.RawURLEncoding.EncodeToString(challenge[:])), http.StatusFound)
}

func (s *Service) callback(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Mode != ModeGitHub {
		httpx.Error(w, http.StatusNotFound, "login is disabled")

		return
	}

	st, ok := s.loginState(r)
	s.clearCookie(w, cookieOAuth, "/auth")

	q := r.URL.Query()

	switch {
	case !ok || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1:
		httpx.Error(w, http.StatusBadRequest, "invalid or expired login state, try again")

		return
	case q.Get("error") != "":
		httpx.Error(w, http.StatusForbidden, "login was denied")

		return
	case q.Get("code") == "":
		httpx.Error(w, http.StatusBadRequest, "missing authorization code")

		return
	}

	sess, status, msg := s.completeLogin(r.Context(), q.Get("code"), st.Verifier)
	if status != 0 {
		httpx.Error(w, status, msg)

		return
	}

	value, err := s.sealer.seal(purposeSession, sess)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")

		return
	}

	s.setCookie(w, &http.Cookie{Name: CookieSession, Value: value, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true})

	if csrf, err := randomToken(); err == nil {
		s.setCookie(w, &http.Cookie{Name: CookieCSRF, Value: csrf, Path: "/"})
	}

	http.Redirect(w, r, st.Next, http.StatusFound)
}

func (s *Service) loginState(r *http.Request) (oauthState, bool) {
	var st oauthState

	c, err := r.Cookie(cookieOAuth)
	if err != nil || s.sealer.open(purposeOAuth, c.Value, &st) != nil || s.now().Unix() > st.Expires {
		return oauthState{}, false
	}

	st.Next = SafeNext(st.Next)

	return st, true
}

// completeLogin exchanges the code and checks membership. A non-zero status is the error to send.
func (s *Service) completeLogin(ctx context.Context, code, verifier string) (session, int, string) {
	token, err := s.cfg.GitHub.ExchangeCode(ctx, s.cfg.ClientID, s.cfg.ClientSecret, code, s.callbackURL(), verifier)
	if err != nil {
		s.log.Warn("github token exchange failed", "err", err)

		return session{}, http.StatusBadGateway, "GitHub login failed"
	}

	user, err := s.cfg.GitHub.User(ctx, token)
	if err != nil {
		s.log.Warn("github user lookup failed", "err", err)

		return session{}, http.StatusBadGateway, "GitHub login failed"
	}

	sess := session{Login: user.Login, AvatarURL: user.AvatarURL, Token: token, Expires: s.now().Add(sessionTTL).Unix()}

	member, err := s.checkMembership(ctx, sess.Token)
	switch {
	case err != nil:
		s.log.Warn("membership check failed", "login", user.Login, "err", err)

		return session{}, http.StatusBadGateway, "could not verify organization membership"
	case !member:
		s.log.Info("login refused: not a member of an allowed organization", "login", user.Login)

		return session{}, http.StatusForbidden, "not a member of an allowed organization"
	}

	s.remember(sess.Login)

	return sess, 0, ""
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := s.readSession(r); ok {
		s.forget(sess.Login)
	}

	s.clearCookie(w, CookieSession, "/")
	w.WriteHeader(http.StatusNoContent)
}

// checkMembership asks GitHub whether the token's user is an active member of an allowed org.
func (s *Service) checkMembership(ctx context.Context, token string) (bool, error) {
	var firstErr error

	for _, org := range s.cfg.AllowedOrgs {
		ok, err := s.cfg.GitHub.IsActiveMember(ctx, token, org)

		switch {
		case ok:
			return true, nil
		case err != nil && firstErr == nil:
			firstErr = err
		}
	}

	return false, firstErr
}

func (s *Service) remember(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if len(s.members) > pruneAbove {
		for l, until := range s.members {
			if now.After(until) {
				delete(s.members, l)
			}
		}
	}

	s.members[login] = now.Add(membershipTTL)
}

func (s *Service) forget(login string) {
	s.mu.Lock()
	delete(s.members, login)
	s.mu.Unlock()
}

func (s *Service) cachedMember(login string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	until, ok := s.members[login]

	return ok && s.now().Before(until)
}

func (s *Service) readSession(r *http.Request) (session, bool) {
	var sess session

	if s.sealer == nil {
		return session{}, false
	}

	c, err := r.Cookie(CookieSession)
	if err != nil || s.sealer.open(purposeSession, c.Value, &sess) != nil || s.now().Unix() > sess.Expires {
		return session{}, false
	}

	return sess, true
}

type outcome int

const (
	outcomeOK outcome = iota
	outcomeAnonymous
	outcomeForbidden
	outcomeError
)

// identify resolves the session of a github-mode request and checks (cached) membership.
func (s *Service) identify(r *http.Request) (Actor, outcome) {
	sess, ok := s.readSession(r)
	if !ok {
		return Actor{}, outcomeAnonymous
	}

	actor := Actor{Login: sess.Login, AvatarURL: sess.AvatarURL, Token: sess.Token}

	if s.cachedMember(sess.Login) {
		return actor, outcomeOK
	}

	member, err := s.checkMembership(r.Context(), sess.Token)

	var apiErr *github.APIError

	switch {
	case errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized:
		// The token was revoked: the session is worth nothing.
		return Actor{}, outcomeAnonymous
	case err != nil:
		s.log.Warn("membership check failed", "login", sess.Login, "err", err)

		return Actor{}, outcomeError
	case !member:
		s.forget(sess.Login)

		return actor, outcomeForbidden
	}

	s.remember(sess.Login)

	return actor, outcomeOK
}

// Authenticate guards the API: in github mode a request needs a valid session of an
// organization member (401 / 403); in mode none everything passes. The actor is put in the context.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	if s.cfg.Mode != ModeGitHub {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, res := s.identify(r)

		switch res {
		case outcomeOK:
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
		case outcomeAnonymous:
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
		case outcomeForbidden:
			httpx.Error(w, http.StatusForbidden, "not a member of an allowed organization")
		case outcomeError:
			httpx.Error(w, http.StatusBadGateway, "could not verify organization membership")
		}
	})
}

// Me describes the caller for GET /api/v1/me; it never fails.
func (s *Service) Me(r *http.Request) model.Me {
	me := model.Me{AuthMode: s.cfg.Mode}

	var actor *Actor

	if s.cfg.Mode == ModeGitHub {
		if a, res := s.identify(r); res == outcomeOK {
			actor = &a
		}
	} else {
		actor = s.dev
	}

	if actor != nil {
		me.Authenticated = true
		me.Login = actor.Login
		me.AvatarURL = actor.AvatarURL
		me.CanRerun = true
	}

	return me
}

// Actor returns who the request's reruns are posted as. Behind Authenticate (github mode) it
// is the logged-in member; in mode none it is the dev token's user, else ErrForbidden.
func (s *Service) Actor(r *http.Request) (Actor, error) {
	if s.cfg.Mode != ModeGitHub {
		if s.dev == nil {
			return Actor{}, fmt.Errorf("%w: reruns are disabled without login", ErrForbidden)
		}

		return *s.dev, nil
	}

	if a, ok := r.Context().Value(actorKey{}).(Actor); ok {
		return a, nil
	}

	return Actor{}, ErrUnauthenticated
}

// EnsureCSRFCookie issues the readable pacboard_csrf cookie when the client has none.
func (s *Service) EnsureCSRFCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(CookieCSRF); err != nil {
			if token, err := randomToken(); err == nil {
				// Readable by the SPA on purpose: it echoes the value in X-Pacboard-CSRF.
				s.setCookie(w, &http.Cookie{Name: CookieCSRF, Value: token, Path: "/"})
			}
		}

		next.ServeHTTP(w, r)
	})
}

// RequireCSRF enforces the double-submit check on unsafe methods: the X-Pacboard-CSRF header
// must equal the pacboard_csrf cookie.
func (s *Service) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)

			return
		}

		c, err := r.Cookie(CookieCSRF)
		h := r.Header.Get(HeaderCSRF) //nolint:canonicalheader // spelling fixed by docs/api.md; lookup is case-insensitive

		if err != nil || c.Value == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(h)) != 1 {
			httpx.Error(w, http.StatusForbidden, "missing or invalid CSRF token")

			return
		}

		next.ServeHTTP(w, r)
	})
}
