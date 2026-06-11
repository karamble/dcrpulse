// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package auth implements the dashboard's optional "app password" protection:
// a single-password gate over the whole HTTP/WebSocket API, persisted in the
// global config and carried by a signed, HttpOnly session cookie. It is off by
// default; while disabled the RequireAuth middleware is a pass-through.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"dcrpulse/internal/config"
)

const (
	sessionCookieName = "dcrpulse_session"
	sessionTTL        = 30 * 24 * time.Hour
)

var (
	mu        sync.RWMutex
	enabled   bool
	hash      []byte // bcrypt hash of the app password
	secret    []byte // HMAC key for signed session cookies
	dismissed bool   // user declined the first-run setup prompt
)

// Init loads the persisted auth state from the global config. Call once at
// startup before the router is built. A load error leaves auth disabled (fail
// open) rather than locking the user out of a dashboard with a broken config.
func Init() error {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	var (
		en      bool
		hashStr string
		secStr  string
		dis     bool
	)
	if _, err := cfg.Get(config.KeyAuthEnabled, &en); err != nil {
		return err
	}
	if _, err := cfg.Get(config.KeyAuthPasswordHash, &hashStr); err != nil {
		return err
	}
	if _, err := cfg.Get(config.KeyAuthSessionSecret, &secStr); err != nil {
		return err
	}
	if _, err := cfg.Get(config.KeyAuthSetupDismissed, &dis); err != nil {
		return err
	}
	sec, _ := base64.StdEncoding.DecodeString(secStr)

	mu.Lock()
	defer mu.Unlock()
	hash = []byte(hashStr)
	secret = sec
	// Only treat auth as enabled when a hash and secret actually exist, so a
	// stray enabled flag can never lock the user out.
	enabled = en && len(hash) > 0 && len(secret) > 0
	dismissed = dis
	return nil
}

// Enabled reports whether the app-password gate is active.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// Configured reports whether a password has been set (regardless of enabled).
func Configured() bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(hash) > 0
}

// SetupDismissed reports whether the user declined the first-run setup prompt.
func SetupDismissed() bool {
	mu.RLock()
	defer mu.RUnlock()
	return dismissed
}

// Setup performs first-time configuration: hash the password, generate the
// session secret, enable the gate, and persist. Errors if already configured.
func Setup(password string) error {
	if password == "" {
		return errors.New("password must not be empty")
	}
	if Configured() {
		return errors.New("a password is already configured")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	sec := make([]byte, 32)
	if _, err := rand.Read(sec); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hash) > 0 {
		return errors.New("a password is already configured")
	}
	if err := persistLocked(true, h, sec, true); err != nil {
		return err
	}
	hash, secret, enabled, dismissed = h, sec, true, true
	return nil
}

// Verify reports whether password matches the configured hash.
func Verify(password string) bool {
	mu.RLock()
	h := append([]byte(nil), hash...)
	mu.RUnlock()
	if len(h) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(h, []byte(password)) == nil
}

// Change replaces the password after verifying the current one. The session
// secret is kept, so the caller's existing session stays valid.
func Change(current, next string) error {
	if next == "" {
		return errors.New("new password must not be empty")
	}
	if !Verify(current) {
		return errors.New("current password is incorrect")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if err := persistLocked(enabled, h, secret, dismissed); err != nil {
		return err
	}
	hash = h
	return nil
}

// Disable turns the gate off after verifying the current password. The hash
// and secret are cleared so all sessions become invalid.
func Disable(current string) error {
	if !Verify(current) {
		return errors.New("current password is incorrect")
	}
	mu.Lock()
	defer mu.Unlock()
	if err := persistLocked(false, nil, nil, dismissed); err != nil {
		return err
	}
	enabled, hash, secret = false, nil, nil
	return nil
}

// MarkSetupDismissed records that the user declined the first-run prompt.
func MarkSetupDismissed() error {
	mu.Lock()
	defer mu.Unlock()
	if dismissed {
		return nil
	}
	if err := persistLocked(enabled, hash, secret, true); err != nil {
		return err
	}
	dismissed = true
	return nil
}

// persistLocked writes the four auth fields into the global config, preserving
// every other key. Caller holds mu; values are passed in, not read from state.
func persistLocked(en bool, h, sec []byte, dis bool) error {
	cfg, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := cfg.Set(config.KeyAuthEnabled, en); err != nil {
		return err
	}
	if err := cfg.Set(config.KeyAuthPasswordHash, string(h)); err != nil {
		return err
	}
	if err := cfg.Set(config.KeyAuthSessionSecret, base64.StdEncoding.EncodeToString(sec)); err != nil {
		return err
	}
	if err := cfg.Set(config.KeyAuthSetupDismissed, dis); err != nil {
		return err
	}
	return cfg.Save()
}

// MintSession returns a signed token (expiry|HMAC) valid for sessionTTL.
func MintSession() (string, error) {
	mu.RLock()
	sec := append([]byte(nil), secret...)
	mu.RUnlock()
	if len(sec) == 0 {
		return "", errors.New("auth not configured")
	}
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	mac := hmac.New(sha256.New, sec)
	mac.Write([]byte(exp))
	return exp + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// ValidSession reports whether token is a non-expired, correctly-signed session.
func ValidSession(token string) bool {
	mu.RLock()
	sec := append([]byte(nil), secret...)
	en := enabled
	mu.RUnlock()
	if !en || len(sec) == 0 {
		return false
	}
	dot := strings.LastIndexByte(token, '.')
	if dot <= 0 {
		return false
	}
	expStr, sigStr := token[:dot], token[dot+1:]
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, sec)
	mac.Write([]byte(expStr))
	return hmac.Equal(mac.Sum(nil), got)
}

// Authenticated reports whether r carries a valid session cookie.
func Authenticated(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return ValidSession(c.Value)
}

// SetSessionCookie issues a fresh signed session cookie on w.
func SetSessionCookie(w http.ResponseWriter, r *http.Request) error {
	val, err := MintSession()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure(r),
		MaxAge:   int(sessionTTL / time.Second),
	})
	return nil
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   isSecure(r),
		MaxAge:   -1,
	})
}

func isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// RequireAuth gates the /api subrouter. While the app password is disabled it
// is a pass-through. When enabled, only the login handshake (/api/auth/login,
// /api/auth/status) is exempt; every other route needs a valid session cookie.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/api/auth/login", "/api/auth/status":
			next.ServeHTTP(w, r)
			return
		}
		if !Authenticated(r) {
			// Tag the gate's own 401 so the frontend can tell it apart from a
			// downstream daemon/wallet 401 (e.g. a wallet's public-passphrase
			// prompt after a wallet switch) and re-lock only on a genuine
			// app-password session failure.
			w.Header().Set("X-Dashboard-Auth", "required")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
