// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

// A panel session is deliberately not a game token: GameTokens authorize
// /gaming/spend, never expire, and are revocable only by uninstalling the game.

const (
	uiSessionIdle = 15 * time.Minute
	uiSessionMax  = 8 * time.Hour
	uiSessionsMax = 8
)

// GamingUISession is one open game panel.
type GamingUISession struct {
	Game    string
	TableID string
	// PayoutAddress is derived from the bound gaming account and pushed to
	// the game; it is never accepted from the page.
	PayoutAddress string

	Expires  time.Time
	Absolute time.Time
}

var (
	uiSessionsMu sync.Mutex
	uiSessions   = map[string]GamingUISession{}
)

// MintGamingUISession issues a token for one open panel.
func MintGamingUISession(game, tableID, payout string) (string, GamingUISession, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", GamingUISession{}, err
	}
	token := hex.EncodeToString(raw)

	now := time.Now()
	s := GamingUISession{
		Game:          game,
		TableID:       tableID,
		PayoutAddress: payout,
		Expires:       now.Add(uiSessionIdle),
		Absolute:      now.Add(uiSessionMax),
	}

	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	sweepUISessions(now)
	for len(uiSessions) >= uiSessionsMax {
		evictOldestUISession()
	}
	uiSessions[token] = s
	return token, s, nil
}

// GamingUISessionFor resolves a token and slides its idle deadline forward.
// Compared in constant time, so a linear scan rather than a map lookup.
func GamingUISessionFor(token string) (GamingUISession, bool) {
	if token == "" {
		return GamingUISession{}, false
	}
	now := time.Now()

	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	sweepUISessions(now)

	want := []byte(token)
	for have, s := range uiSessions {
		if subtle.ConstantTimeCompare([]byte(have), want) != 1 {
			continue
		}
		s.Expires = now.Add(uiSessionIdle)
		if s.Expires.After(s.Absolute) {
			s.Expires = s.Absolute
		}
		uiSessions[have] = s
		return s, true
	}
	return GamingUISession{}, false
}

// RotateGamingUISession replaces a live session's token with a new one, so a
// leaked token has a bounded life even while its panel stays open.
func RotateGamingUISession(old string) (string, GamingUISession, bool) {
	s, ok := GamingUISessionFor(old)
	if !ok {
		return "", GamingUISession{}, false
	}
	RevokeGamingUISession(old)

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", GamingUISession{}, false
	}
	token := hex.EncodeToString(raw)

	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	s.Expires = time.Now().Add(uiSessionIdle)
	if s.Expires.After(s.Absolute) {
		s.Expires = s.Absolute
	}
	uiSessions[token] = s
	return token, s, true
}

// RevokeGamingUISession ends one session.
func RevokeGamingUISession(token string) {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	delete(uiSessions, token)
}

// RevokeGamingUISessionsFor ends every session for one game.
func RevokeGamingUISessionsFor(game string) {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	for token, s := range uiSessions {
		if s.Game == game {
			delete(uiSessions, token)
		}
	}
}

// RevokeAllGamingUISessions ends every session.
func RevokeAllGamingUISessions() {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	uiSessions = map[string]GamingUISession{}
}

// CountGamingUISessions reports how many sessions are live.
func CountGamingUISessions() int {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	sweepUISessions(time.Now())
	return len(uiSessions)
}

// sweepUISessions drops expired sessions. Requires uiSessionsMu.
func sweepUISessions(now time.Time) {
	for token, s := range uiSessions {
		if now.After(s.Expires) || now.After(s.Absolute) {
			delete(uiSessions, token)
		}
	}
}

// evictOldestUISession makes room. Requires uiSessionsMu.
func evictOldestUISession() {
	var oldest string
	var at time.Time
	for token, s := range uiSessions {
		if oldest == "" || s.Absolute.Before(at) {
			oldest, at = token, s.Absolute
		}
	}
	if oldest != "" {
		delete(uiSessions, oldest)
	}
}
