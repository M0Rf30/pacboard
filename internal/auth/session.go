// SPDX-FileCopyrightText: The pacboard Authors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Cookie purposes; each is bound into the AEAD so one cookie cannot stand in for another.
const (
	purposeSession = "pacboard_session"
	purposeOAuth   = "pacboard_oauth"
)

// MinKeyLen is the shortest accepted session key.
const MinKeyLen = 32

var errBadCookie = errors.New("invalid cookie")

// session is what the encrypted session cookie holds.
type session struct {
	Login     string `json:"l"`
	AvatarURL string `json:"a"`
	Token     string `json:"t"`
	Expires   int64  `json:"e"`
}

// oauthState is what the short-lived login cookie holds between /auth/login and /auth/callback.
type oauthState struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
	Expires  int64  `json:"e"`
}

// sealer encrypts cookie payloads with AES-256-GCM under a key derived from the session key.
type sealer struct{ aead cipher.AEAD }

func newSealer(secret string) (*sealer, error) {
	if len(secret) < MinKeyLen {
		return nil, fmt.Errorf("session key must be at least %d bytes", MinKeyLen)
	}

	key, err := hkdf.Key(sha256.New, []byte(secret), nil, "pacboard cookie encryption v1", 32)
	if err != nil {
		return nil, fmt.Errorf("derive cookie key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cookie cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cookie cipher: %w", err)
	}

	return &sealer{aead: aead}, nil
}

func (s *sealer) seal(purpose string, v any) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode cookie: %w", err)
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("cookie nonce: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plain, []byte(purpose))), nil
}

func (s *sealer) open(purpose, value string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return errBadCookie
	}

	nonce, ct := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]

	plain, err := s.aead.Open(nil, nonce, ct, []byte(purpose))
	if err != nil {
		return errBadCookie
	}

	if err := json.Unmarshal(plain, v); err != nil {
		return errBadCookie
	}

	return nil
}

// SafeNext returns next when it is a same-origin path, else "/". It rejects absolute URLs,
// protocol-relative ("//host") and backslash forms browsers treat as such, and control characters.
func SafeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}

	for _, r := range next {
		if r == '\\' || r < 0x20 || r == 0x7f {
			return "/"
		}
	}

	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}

	return next
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
