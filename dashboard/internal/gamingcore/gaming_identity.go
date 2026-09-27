// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"context"
	"crypto/elliptic"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/certgen"

	"dcrpulse/internal/gamingbridge"
	"github.com/karamble/dcrgaming-sdk/pkg/gaming/gamingpb"
)

// bridgeCertLifetime is how long the bridge's own certificate is good for.
//
// Long, for the same reason a game's credential is: a game pins these bytes by
// hand, and an expiry would come due on a machine the operator may not be
// sitting at. Replacing it is an act they take, not a date that arrives.
const bridgeCertLifetime = 10 * 365 * 24 * time.Hour

// ErrGamingNoCredential is a game that has not been issued one.
var ErrGamingNoCredential = errors.New("that game has no credential yet")

// LoadGamingAllowlist rebuilds the allowlist from what was stored, and is
// called once at startup. A credential the operator issued has to survive the
// appliance restarting, or every game would need reconfiguring after an update.
func (br *Bridge) LoadGamingAllowlist() {
	for game, c := range br.ReadGamingSettings().GameCredentials {
		if err := br.gamingAllow.Add(gamingbridge.Credential{
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
func (br *Bridge) IssueGamingCredential(game string) (GamingCredentialMaterial, error) {
	br.gamingSettingsMu.Lock()
	defer br.gamingSettingsMu.Unlock()

	s := br.ReadGamingSettings()
	if !gamingRegisteredIn(s, game) {
		return GamingCredentialMaterial{}, ErrGamingGameNotRegistered
	}

	bridgeCert, _, err := br.gamingBridgeKeypairLocked()
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
		s.GameCredentials = map[string]GameCredential{}
	}
	issued := time.Now().Unix()
	s.GameCredentials[game] = GameCredential{
		Fingerprint: cred.Fingerprint,
		CertPEM:     string(cred.CertPEM),
		IssuedAt:    issued,
	}
	if err := br.writeGamingSettingsLocked(s); err != nil {
		return GamingCredentialMaterial{}, err
	}
	if err := br.gamingAllow.Add(cred); err != nil {
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
func (br *Bridge) RevokeGamingCredential(game string) error {
	br.gamingSettingsMu.Lock()
	defer br.gamingSettingsMu.Unlock()

	s := br.ReadGamingSettings()
	if _, ok := s.GameCredentials[game]; !ok {
		return ErrGamingNoCredential
	}
	br.gamingAllow.Revoke(game)
	delete(s.GameCredentials, game)
	if err := br.writeGamingSettingsLocked(s); err != nil {
		return err
	}
	// Requests the revoked game left waiting are answered too; a warning is
	// all a failure earns, because the approval-time re-check keeps money
	// shut either way.
	if err := br.invalidatePendingSpends(func(g string) bool { return g == game }, spendInvalidatedText); err != nil {
		gameLog.Warnf("retire %q's pending requests: %v", game, err)
	}
	gameLog.Infof("revoked the credential for %q", game)
	return nil
}

func (br *Bridge) gamingBridgeCertPath() string {
	return filepath.Join(br.dataDir, "gaming-bridge.cert")
}

func (br *Bridge) gamingBridgeKeyPath() string {
	return filepath.Join(br.dataDir, "gaming-bridge.key")
}

// GamingBridgeKeypair is the bridge's own identity, minted on first use.
//
// Minted here rather than shipped, because a certificate shared across
// installations would be one every operator could impersonate. It persists so
// that a game which pinned it keeps trusting this bridge across restarts - a
// pair regenerated at every boot would break every configured game on every
// update.
func (br *Bridge) GamingBridgeKeypair() (certPEM, keyPEM []byte, err error) {
	br.bridgeKeypairMu.Lock()
	defer br.bridgeKeypairMu.Unlock()
	return br.gamingBridgeKeypairLocked()
}

func (br *Bridge) gamingBridgeKeypairLocked() (certPEM, keyPEM []byte, err error) {
	cert, certErr := os.ReadFile(br.gamingBridgeCertPath())
	key, keyErr := os.ReadFile(br.gamingBridgeKeyPath())
	if certErr == nil && keyErr == nil {
		return cert, key, nil
	}

	cert, key, err = certgen.NewTLSCertPair(
		elliptic.P256(), "dcrpulse gaming bridge", time.Now().Add(bridgeCertLifetime), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("mint the bridge's certificate: %w", err)
	}
	if err := os.MkdirAll(br.dataDir, 0o700); err != nil {
		return nil, nil, err
	}
	// The key first: a certificate on disk with no key beside it would be
	// read back as a usable pair on the next call and fail at the listener.
	if err := os.WriteFile(br.gamingBridgeKeyPath(), key, 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(br.gamingBridgeCertPath(), cert, 0o600); err != nil {
		return nil, nil, err
	}
	gameLog.Infof("minted the gaming bridge's own certificate")
	return cert, key, nil
}

// GamingBridgeConfig assembles what the listener needs from this package.
//
// Built here rather than in main so the two sides stay one decision: the bridge
// package depends on nothing, and everything it is handed is named in one
// place, where it can be seen that a game is given chain reads and frame
// carriage and no way to move money.
func (br *Bridge) GamingBridgeConfig(addr string) (gamingbridge.Config, error) {
	cert, key, err := br.GamingBridgeKeypair()
	if err != nil {
		return gamingbridge.Config{}, err
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		br.gamingBridgePort = port
	}
	return gamingbridge.Config{
		Addr:              addr,
		ServerCert:        cert,
		ServerKey:         key,
		AppPasswordActive: func() bool { return br.hostOperator().Protected() },
		Enabled:           func() bool { return br.ReadGamingSettings().Enabled },
		Allow:             br.gamingAllow,

		Frames: func(game string, after uint64, buf int) (<-chan gamingbridge.Frame, func()) {
			in, stop := br.SubscribeFrom(game, after, buf)
			return forwardGamingFrames(in, stop, buf)
		},
		Network: func() (string, bool) {
			net, err := br.currentNetwork(context.Background())
			return net, err == nil
		},
		Policy: func(game string) (int64, int64, bool) {
			p := br.ReadGamingSettings().Policies[game]
			return p.PerTableCapAtoms, p.PerDayCapAtoms, strings.TrimSpace(p.Account) != ""
		},

		FinancialKey:   br.GamingFinancialKeyReply,
		PrepareDeposit: br.PrepareGamingDeposit,
		FinancialState: br.GamingFinancialState,
		ProposePayout:  br.ProposeGamingPayout,
		BindRoster:     br.BindGamingRoster,
		PayoutStatus:   br.GamingPayoutStatus,
		VerifiedSpend: func(ctx context.Context, game string, req *gamingpb.RequestSpendRequest) (*gamingpb.Spend, error) {
			spend, err := br.RequestGamingDepositSpend(ctx, game, req)
			return spend, spendBridgeErr(err)
		},
		SpendStatus: func(game, id string) (*gamingpb.Spend, error) {
			spend, err := br.GamingSpendFor(game, id)
			return spendProto(spend), spendBridgeErr(err)
		},
		SendFrame: br.SendGamingFrame,
		ChainTip: func(ctx context.Context) (int64, string, error) {
			tip, err := br.GamingChainTipNow(ctx)
			return tip.Height, tip.Hash, err
		},
		BlockHash: br.GamingBlockHash,
		Outpoint: func(ctx context.Context, txid string, vout uint32, mempool bool) (gamingbridge.Outpoint, error) {
			o, err := br.GamingChainOutpoint(ctx, txid, vout, mempool)
			if err != nil {
				return gamingbridge.Outpoint{}, err
			}
			return gamingbridge.Outpoint{
				Found:         o.Found,
				ValueAtoms:    o.ValueAtoms,
				PkScriptHex:   o.PkScriptHex,
				Confirmations: o.Confirmations,
				Coinbase:      o.Coinbase,
			}, nil
		},
		OnConnect: func(game string) {
			if err := br.RefreshGamingState(context.Background(), game); err != nil {
				gameLog.Warnf("%s did not report its state: %v", game, err)
			}
		},
		OnPresence:           br.GamingPresenceChanged,
		StartFinancialWorker: br.StartGamingFinancialWorker,
	}, nil
}

// GamingBridgePort is the port games connect in on, or empty if the listener
// never started.
func (br *Bridge) GamingBridgePort() string { return br.gamingBridgePort }

// Start brings up the listener games connect to on addr and serves it. It is
// called once.
//
// A failure is returned, not fatal: gaming is one section of a wallet app, and
// refusing to run the rest because a game could not be served would be the
// wrong trade.
func (br *Bridge) Start(addr string) error {
	br.LoadGamingAllowlist()
	cfg, err := br.GamingBridgeConfig(addr)
	if err != nil {
		return fmt.Errorf("the gaming bridge has no certificate, so no game can connect: %w", err)
	}
	srv, err := gamingbridge.New(cfg)
	if err != nil {
		return fmt.Errorf("could not prepare the gaming bridge: %w", err)
	}
	// What the console reports as connected. A live stream is the only honest
	// answer: a game is registered here and run on a machine of the person's
	// choosing, so registered and connected are different questions.
	br.gamingConnected = srv.SubscriberCount
	br.gamingRequest = srv.Request
	br.gamingState = srv.State
	br.gamingLockTerms = srv.LockTerms
	// How a loss upstream of the bridge reaches the games. The bridge cannot
	// see that kind of gap for itself, so the notification stream tells it.
	br.SetGamingResync(srv.ResyncAll)
	go func() {
		if err := srv.Serve(); err != nil {
			gameLog.Errorf("the gaming bridge stopped: %v", err)
		}
	}()
	return nil
}

// gamingGameLockTerms is the advertised locks for a game, or zero when there is
// no listener or the game advertised none.
func (br *Bridge) gamingGameLockTerms(game string) (minRefund, bondLock uint32) {
	if br.gamingLockTerms == nil {
		return 0, 0
	}
	return br.gamingLockTerms(game)
}

func (br *Bridge) gamingGameConnected(game string) bool {
	// No listener means nothing is connected, which is the truthful answer
	// rather than an optimistic one.
	return br.gamingConnected != nil && br.gamingConnected(game) > 0
}

// spendProto is one spend on the wire.
func spendProto(s GamingSpend) *gamingpb.Spend {
	if s.ID == "" {
		return nil
	}
	// A payment being broadcast is told to its game as still pending. The
	// deployed game reads any state it does not know as a terminal refusal
	// and drops its own double-payment guard - and "keep waiting" is also
	// the truthful answer, since the outcome lands moments later.
	state := s.State
	if state == GamingSpendPublishing {
		state = GamingSpendPending
	}
	return &gamingpb.Spend{
		Id:          s.ID,
		Game:        s.Game,
		Address:     s.Address,
		AmountAtoms: s.AmountAtoms,
		Reason:      s.Reason,
		State:       string(state),
		Txid:        s.TxID,
		Error:       s.Error,
		RequestedAt: s.RequestedAt,
		DecidedAt:   s.DecidedAt,
		ExpiresAt:   s.ExpiresAt,
	}
}

// spendBridgeErr translates a refusal into the two the bridge can tell apart.
//
// Only these two, because they are the only two a game can do anything useful
// with: wait and ask for less, or stop asking about an id that is not its own.
// Everything else is the operator's to fix and reaches the game as its own
// words.
func spendBridgeErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrGamingSpendOverCap):
		return gamingbridge.GameSafe(fmt.Errorf("%w: %s", gamingbridge.ErrSpendOverCap, err))
	case errors.Is(err, ErrGamingSpendNotFound):
		return gamingbridge.GameSafe(fmt.Errorf("%w: %s", gamingbridge.ErrSpendNotFound, err))
	default:
		return err
	}
}

// forwardGamingFrames converts bus events for one bridge subscription. Stopping
// it releases both the bus subscription and this worker, even if nobody drains
// out. Only the worker closes out; durable inbox records remain for reconnect.
func forwardGamingFrames(in <-chan GamingFrameEvent, unsubscribe func(), buf int) (<-chan gamingbridge.Frame, func()) {
	out := make(chan gamingbridge.Frame, buf)
	cancelled := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		defer close(out)
		for {
			// Do not keep consuming a ready backlog after explicit cancellation.
			select {
			case <-cancelled:
				return
			default:
			}
			var ev GamingFrameEvent
			select {
			case <-cancelled:
				return
			case next, ok := <-in:
				if !ok {
					return
				}
				ev = next
			}
			select {
			case <-cancelled:
				return
			case out <- gamingbridge.Frame{Seq: ev.Seq, GCID: ev.GCID, From: ev.From, Frame: ev.Frame}:
			}
		}
	}()
	return out, func() {
		once.Do(func() {
			close(cancelled)
			unsubscribe()
		})
		// Unsubscribe has released the bus lock before waiting for the worker.
		<-finished
	}
}
