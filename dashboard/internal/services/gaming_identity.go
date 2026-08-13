// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"crypto/elliptic"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/decred/dcrd/certgen"

	"dcrpulse/internal/gamingbridge"
	"dcrpulse/internal/types"
)

// bridgeCertLifetime is how long the bridge's own certificate is good for.
//
// Long, for the same reason a game's credential is: a game pins these bytes by
// hand, and an expiry would come due on a machine the operator may not be
// sitting at. Replacing it is an act they take, not a date that arrives.
const bridgeCertLifetime = 10 * 365 * 24 * time.Hour

// ErrGamingNoCredential is a game that has not been issued one.
var ErrGamingNoCredential = errors.New("that game has no credential yet")

// gamingAllow is the live allowlist the listener verifies against.
//
// One per process, held here rather than in the handler that mutates it,
// because issuing a credential has to change what the running listener accepts
// in the same act that writes it to disk. Two copies would mean a credential
// that works only after a restart, or one that outlives being revoked.
var gamingAllow = gamingbridge.NewAllowlist()

// LoadGamingAllowlist rebuilds the allowlist from what was stored, and is
// called once at startup. A credential the operator issued has to survive the
// appliance restarting, or every game would need reconfiguring after an update.
func LoadGamingAllowlist() {
	for game, c := range ReadGamingSettings().GameCredentials {
		if err := gamingAllow.Add(gamingbridge.Credential{
			Game:    game,
			CertPEM: []byte(c.CertPEM),
		}); err != nil {
			gameLog.Warnf("stored credential for %q will not load, so that game cannot connect: %v",
				game, err)
		}
	}
}

// GamingCredentialMaterial is what an operator carries to a game, once.
type GamingCredentialMaterial struct {
	Game string `json:"game"`

	// CertPEM and KeyPEM are the game's own credential. The key is in this
	// answer and in no file: it is shown once and then exists only wherever
	// the operator put it.
	CertPEM string `json:"certPem"`
	KeyPEM  string `json:"keyPem"`

	// BridgeCertPEM is what the game pins so it can tell this bridge from
	// anything else answering on that address.
	BridgeCertPEM string `json:"bridgeCertPem"`

	Fingerprint string `json:"fingerprint"`
	IssuedAt    int64  `json:"issuedAt"`
}

// IssueGamingCredential mints a game's credential, admits it, and stores what
// the bridge needs to recognise it again.
//
// Issuing to a game that already has one replaces it in the same act, which is
// what regenerating means: the old credential stops working immediately rather
// than leaving two ways in, one of them on a machine the operator no longer
// trusts.
func IssueGamingCredential(game string) (GamingCredentialMaterial, error) {
	gamingSettingsMu.Lock()
	defer gamingSettingsMu.Unlock()

	s := ReadGamingSettings()
	if !gamingRegisteredIn(s, game) {
		return GamingCredentialMaterial{}, ErrGamingGameNotRegistered
	}

	bridgeCert, _, err := gamingBridgeKeypairLocked()
	if err != nil {
		return GamingCredentialMaterial{}, err
	}

	cred, err := gamingbridge.GenerateGameCredential(game)
	if err != nil {
		return GamingCredentialMaterial{}, err
	}

	// Store before admitting. A credential the listener accepts but has not
	// written down would stop working at the next restart, with nothing to
	// say why.
	if s.GameCredentials == nil {
		s.GameCredentials = map[string]types.GameCredential{}
	}
	issued := time.Now().Unix()
	s.GameCredentials[game] = types.GameCredential{
		Fingerprint: cred.Fingerprint,
		CertPEM:     string(cred.CertPEM),
		IssuedAt:    issued,
	}
	if err := writeGamingSettingsLocked(s); err != nil {
		return GamingCredentialMaterial{}, err
	}
	if err := gamingAllow.Add(cred); err != nil {
		return GamingCredentialMaterial{}, err
	}

	gameLog.Infof("issued a credential for %q (%s)", game, cred.Fingerprint[:16])
	return GamingCredentialMaterial{
		Game:          game,
		CertPEM:       string(cred.CertPEM),
		KeyPEM:        string(cred.KeyPEM),
		BridgeCertPEM: string(bridgeCert),
		Fingerprint:   cred.Fingerprint,
		IssuedAt:      issued,
	}, nil
}

// RevokeGamingCredential withdraws a game's credential.
//
// The listener stops accepting it and any stream it is holding ends, before the
// write, because the point of revoking is that it takes effect now and the disk
// is only what makes it survive a restart.
func RevokeGamingCredential(game string) error {
	gamingSettingsMu.Lock()
	defer gamingSettingsMu.Unlock()

	s := ReadGamingSettings()
	if _, ok := s.GameCredentials[game]; !ok {
		return ErrGamingNoCredential
	}
	gamingAllow.Revoke(game)
	delete(s.GameCredentials, game)
	if err := writeGamingSettingsLocked(s); err != nil {
		return err
	}
	gameLog.Infof("revoked the credential for %q", game)
	return nil
}

var bridgeKeypairMu sync.Mutex

func gamingBridgeCertPath() string { return filepath.Join(GamingStateDir, "gaming-bridge.cert") }
func gamingBridgeKeyPath() string  { return filepath.Join(GamingStateDir, "gaming-bridge.key") }

// GamingBridgeKeypair is the bridge's own identity, minted on first use.
//
// Minted here rather than shipped, because a certificate shared across
// installations would be one every operator could impersonate. It persists so
// that a game which pinned it keeps trusting this bridge across restarts - a
// pair regenerated at every boot would break every configured game on every
// update.
func GamingBridgeKeypair() (certPEM, keyPEM []byte, err error) {
	bridgeKeypairMu.Lock()
	defer bridgeKeypairMu.Unlock()
	return gamingBridgeKeypairLocked()
}

func gamingBridgeKeypairLocked() (certPEM, keyPEM []byte, err error) {
	cert, certErr := os.ReadFile(gamingBridgeCertPath())
	key, keyErr := os.ReadFile(gamingBridgeKeyPath())
	if certErr == nil && keyErr == nil {
		return cert, key, nil
	}

	cert, key, err = certgen.NewTLSCertPair(
		elliptic.P256(), "dcrpulse gaming bridge", time.Now().Add(bridgeCertLifetime), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("mint the bridge's certificate: %w", err)
	}
	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return nil, nil, err
	}
	// The key first: a certificate on disk with no key beside it would be
	// read back as a usable pair on the next call and fail at the listener.
	if err := os.WriteFile(gamingBridgeKeyPath(), key, 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(gamingBridgeCertPath(), cert, 0o600); err != nil {
		return nil, nil, err
	}
	gameLog.Infof("minted the gaming bridge's own certificate")
	return cert, key, nil
}

// gamingConnected reports how many streams a game is holding.
//
// A function set from main rather than a call into the bridge package, because
// that package is a leaf: it depends on nothing here, which is what lets this
// one use its allowlist without the two importing each other.
var gamingConnected func(game string) int

// SetGamingConnected wires the listener's view of who is connected.
func SetGamingConnected(f func(game string) int) { gamingConnected = f }

func gamingGameConnected(game string) bool {
	// No listener means nothing is connected, which is the truthful answer
	// rather than an optimistic one.
	return gamingConnected != nil && gamingConnected(game) > 0
}
