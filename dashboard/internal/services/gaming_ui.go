package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

// A credential for the page, which is not the credential for the game.
//
// A game's own token (GameTokens, minted by carryGameTokens) authorizes
// /gaming/spend, /gaming/send and /gaming/chain/broadcast. It is the game's
// identity as far as this host is concerned; it never expires; and the only way
// to revoke it is to uninstall the game, which is deliberate - saving an
// unrelated policy edit must not cut off every running game. Every one of those
// properties is right for a process the user installed and wrong for a page.
//
// So a page gets its own: short, sliding, capped, revocable, and good for
// nothing but reaching one game's own HTTP API through the proxy, on an
// allowlist of paths. It is minted only by a same-origin, session-authenticated
// request from the dashboard app itself.
//
// It is never written to disk. A restart revokes every one of them, which is
// correct: the games restarted too.

const (
	// uiSessionIdle is how long a session survives without being refreshed.
	uiSessionIdle = 15 * time.Minute
	// uiSessionMax is the hard ceiling, refreshed or not. A panel left open
	// overnight is a token left lying around overnight.
	uiSessionMax = 8 * time.Hour
	// uiSessionsMax bounds how many can be live at once, so nothing can be
	// made to hold an unbounded number of them.
	uiSessionsMax = 8
)

// GamingUISession is one open panel.
type GamingUISession struct {
	Game string
	// TableID is the table the panel was opened for, if it was opened from
	// an invitation. It is carried so the page can be pointed at one table
	// among several; it grants nothing.
	TableID string
	// PayoutAddress is where this host will have the game pay this user.
	// Derived here, from the bound wallet account, and injected into the
	// page - never accepted from it.
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

// GamingUISessionFor resolves a token, and slides its idle deadline forward.
//
// Constant time, mirroring gamingGameForToken: a caller must not be able to
// learn a token a character at a time. It is a linear scan for the same reason
// - a map lookup on the token would compare in whatever time the runtime feels
// like.
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

// RotateGamingUISession replaces a live session's token with a new one.
//
// Rotation rather than extension, so a token that leaked has a bounded life
// even while the panel it came from stays open. Only the parent can call this,
// because only the parent holds the dashboard session.
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

// RevokeGamingUISession ends one session, which is what closing a panel does.
func RevokeGamingUISession(token string) {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	delete(uiSessions, token)
}

// RevokeGamingUISessionsFor ends every session for one game, which is what
// uninstalling it does. Uninstalling revokes rather than hides.
func RevokeGamingUISessionsFor(game string) {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	for token, s := range uiSessions {
		if s.Game == game {
			delete(uiSessions, token)
		}
	}
}

// RevokeAllGamingUISessions ends every session. Called on logout and when the
// gaming section is switched off: a page still holding a token after the user
// locked the dashboard would be a way around the lock.
func RevokeAllGamingUISessions() {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	uiSessions = map[string]GamingUISession{}
}

// CountGamingUISessions is how many are live, for tests and for saying so.
func CountGamingUISessions() int {
	uiSessionsMu.Lock()
	defer uiSessionsMu.Unlock()
	sweepUISessions(time.Now())
	return len(uiSessions)
}

// sweepUISessions drops what has expired. Lazily, on every lookup, rather than
// from a goroutine: there is nothing here worth a timer, and a session that is
// never looked at again is one nobody is using.
//
// Requires uiSessionsMu.
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
