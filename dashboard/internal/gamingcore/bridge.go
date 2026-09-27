// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"dcrpulse/internal/gamingbridge"
	"dcrpulse/internal/gamingfunds"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
	"github.com/karamble/dcrgaming-sdk/pkg/gaming/gamingpb"
)

// Bridge is a gaming bridge running inside its host.
type Bridge struct {
	// dataDir is where the bridge keeps its files: the policy saying which
	// account games may spend from and under what caps, the log of every
	// spend one has asked for, and the ledger.
	dataDir string
	host    Host

	networkMu   sync.Mutex
	networkName string

	// subs fans inbound frames out to the games that are listening.
	//
	// Subscribers are per game, so one game never sees another's traffic. That
	// is containment rather than privacy - frames travel over a group chat
	// every member can read - but a game has no business seeing traffic
	// addressed to a different one.
	subsMu sync.RWMutex
	subs   map[*gamingSubscriber]struct{}

	// wireMu serializes durable inbox append with replay subscription. Holding
	// it until a replaying subscriber is registered closes the replay/live gap.
	wireMu      sync.Mutex
	wireDir     string
	wireNext    map[string]uint64
	wireRecords map[string][]GamingFrameEvent
	wireSeen    map[string]struct{}
	// financialDone are financial frames this process already applied.
	financialDone map[string]struct{}

	gamingHistoryRecovery sync.Mutex
	// prunedGroups are the settled groups whose history this run already
	// dropped; guarded by gamingHistoryRecovery.
	prunedGroups map[string]struct{}

	// gamingFrameJournal is loaded for one directory at a time. A file that
	// does not read cleanly keeps it unloaded, so every call fails until it is
	// repaired.
	gamingFrameJournal struct {
		sync.Mutex
		dir  string
		next uint64
		seen map[string]struct{}
	}

	gamingOutbox struct {
		sync.Mutex
		dir    string
		claims map[string]gamingSendState
	}

	financeStores struct {
		sync.Mutex
		stores map[string]*gamingfunds.Store
	}

	gamingFinancialWorker struct {
		sync.Mutex
		running bool
	}
	gamingFinancialInbox chan GamingFrameEvent

	// gamingIndexComplained keeps the reconcile pass from saying the same
	// thing every thirty seconds, without it going unsaid.
	gamingIndexComplained atomic.Bool

	// gamingSettingsMu serialises the read-modify-write of gaming.json.
	//
	// There are two writers now: saving the section, and issuing or revoking a
	// credential. Both read the whole file, change part of it and write it
	// back, so without this one could land between the other's read and write
	// and lose a registration or a credential.
	gamingSettingsMu sync.Mutex

	bridgeKeypairMu sync.Mutex
	gamingKeyMu     sync.Mutex

	// spendMu serialises the read-modify-write of the log. Two games asking at
	// once must not each see the other's spend as not yet counted.
	spendMu sync.Mutex

	// spendApproving is the requests a person is approving right now. The
	// spend itself runs outside spendMu - signing and broadcasting take as
	// long as they take - and a request that stays pending while it runs would
	// let a second approval pass the Pending check and pay twice. Guarded by
	// spendMu.
	spendApproving map[string]bool

	// gamingAllow is the live allowlist the listener verifies against.
	//
	// One per bridge, held here rather than in the handler that mutates it,
	// because issuing a credential has to change what the running listener
	// accepts in the same act that writes it to disk. Two copies would mean a
	// credential that works only after a restart, or one that outlives being
	// revoked.
	gamingAllow *gamingbridge.Allowlist

	// gamingBridgePort is the port the listener came up on, recorded so the
	// console can tell the operator what to type into a game's wizard.
	//
	// Only the port. The address a game dials is the operator's own, which
	// this process cannot know - it sees a container's interfaces, not the
	// route from wherever the game happens to be running.
	gamingBridgePort string

	// gamingConnected reports how many streams a game is holding. Start sets
	// it and the three below from the listener, which is a leaf: it depends on
	// nothing here, which is what lets this package use its allowlist without
	// the two importing each other.
	gamingConnected func(game string) int

	// gamingRequest asks a connected game to do something and waits for its
	// answer.
	gamingRequest func(ctx context.Context, game string, req *gamingpb.BridgeRequest) (*gamingpb.RespondRequest, error)

	// gamingState is the last state a game reported, cached by the listener.
	gamingState func(game string) *gamingpb.GameState

	// gamingLockTerms is the refund and bond timelocks a game advertised on
	// Hello, cached by the listener.
	gamingLockTerms func(game string) (minRefund, bondLock uint32)

	// The staged calls a game's money passes through: the account it resolves
	// to, the transaction built for it, the signature a person authorises, the
	// relay that puts it on the network, and what the node says an input was.
	//
	// They are settable because the rules around them are the only thing
	// between an untrusted game and this wallet, and a rule that can only be
	// exercised against a live wallet and a live node is a rule nobody has
	// exercised. Production sets none of them.
	spendAccount   func(ctx context.Context, game string) (uint32, error)
	spendConstruct func(ctx context.Context, account uint32, address string, amountAtoms int64) ([]byte, error)
	spendSign      func(ctx context.Context, account uint32, unsigned, passphrase []byte) ([]byte, error)
	spendPublish   func(ctx context.Context, signed []byte) (string, error)
	spendPrevout   func(ctx context.Context, op wire.OutPoint) (GamingPrevout, error)

	// spendDecodeAddress checks an address against the network this node is
	// actually on. Staged like the wallet calls above and for the same reason;
	// the rule itself is checkSpendAddress, which needs no chain.
	spendDecodeAddress func(ctx context.Context, address string) error

	// The staged calls creating a table passes through: the height its
	// deadline is set from, and the chat the invitation is posted to. Settable
	// for the same reason as the spend seams - the order they run in is what
	// stops a table being announced that its creator never took a seat at,
	// and a rule only exercisable against a live node is one nobody has
	// exercised. Production sets neither.
	tableChainTip  func(ctx context.Context) (GamingChainTip, error)
	tableGCMessage func(ctx context.Context, gcid [32]byte, text string) error
	tableAuthorize func(ctx context.Context, game, invite, gcid string) error

	// The wallet scope and chain a received frame is judged against. Settable
	// for tests; production never sets them.
	receiveScope  func(ctx context.Context, game string) (gamingfunds.Scope, error)
	receiveParams func(ctx context.Context) (*chaincfg.Params, error)

	// receiveFinancial applies one financial frame. Settable for tests.
	receiveFinancial func(ctx context.Context, event GamingFrameEvent) error

	// gamingKeyProofSign proves this bridge's key for a table. Settable for
	// tests.
	gamingKeyProofSign func(ctx context.Context, scope gamingfunds.Scope, table string, passphrase []byte) error

	// gamingRecoverAfterAccept reads back a newly accepted table's earlier
	// frames. Settable for tests; production never sets it.
	gamingRecoverAfterAccept func()

	// Settable so these rules run without a live brclientd. Production sets
	// none of them.
	gamingGCHistoryFetch func(ctx context.Context, gcid [32]byte, page, pageSize int) ([]GroupEntry, error)
	gamingSelfNick       func(ctx context.Context) (string, error)
	gamingSelfUID        func(ctx context.Context) (string, error)

	// gamingGCSend posts to a group chat. Settable for tests; production never
	// sets it.
	gamingGCSend func(ctx context.Context, gcid [32]byte, text string) error

	// Wallet checks the restore runs; tests replace them.
	restoreWalletMatches func(ctx context.Context, scope gamingfunds.Scope) error
	restoreKeyOwned      func(ctx context.Context, key gamingfunds.WalletKey) error
}

// New is a bridge keeping its files in dataDir and running on host. It serves
// no game until Start.
func New(dataDir string, host Host) *Bridge {
	br := &Bridge{
		dataDir:              dataDir,
		host:                 host,
		subs:                 make(map[*gamingSubscriber]struct{}),
		gamingFinancialInbox: make(chan GamingFrameEvent, 128),
		spendApproving:       map[string]bool{},
		gamingAllow:          gamingbridge.NewAllowlist(),
	}
	br.spendAccount = br.gamingAccountNumber
	br.spendConstruct = func(ctx context.Context, account uint32, address string, amountAtoms int64) ([]byte, error) {
		return br.hostWallet().Construct(ctx, account, address, amountAtoms)
	}
	br.spendSign = func(ctx context.Context, account uint32, unsigned, passphrase []byte) ([]byte, error) {
		return br.hostWallet().SignTransaction(ctx, account, unsigned, passphrase)
	}
	br.spendPublish = func(ctx context.Context, signed []byte) (string, error) {
		return br.hostWallet().Publish(ctx, signed)
	}
	br.spendPrevout = br.lookupGamingPrevout
	br.spendDecodeAddress = func(ctx context.Context, address string) error {
		params, err := br.chainParams(ctx)
		if err != nil {
			return fmt.Errorf("cannot verify the address without the chain: %w", err)
		}
		return checkSpendAddress(address, params)
	}
	br.tableChainTip = br.GamingChainTipNow
	br.tableGCMessage = func(ctx context.Context, gcid [32]byte, text string) error {
		return br.hostRelay().SendGroupMessage(ctx, gcid, text)
	}
	br.tableAuthorize = br.authorizeGamingTable
	br.receiveScope = br.gamingFinancialScope
	br.receiveParams = br.chainParams
	br.receiveFinancial = br.receiveFinancialFrame
	br.gamingKeyProofSign = br.signGamingKeyProof
	br.gamingRecoverAfterAccept = func() { go br.RecoverHistory() }
	br.gamingGCHistoryFetch = func(ctx context.Context, gcid [32]byte, page, pageSize int) ([]GroupEntry, error) {
		return br.hostRelay().GroupHistory(ctx, gcid, page, pageSize)
	}
	br.gamingSelfNick = br.localGamingNick
	br.gamingSelfUID = br.localGamingUID
	br.gamingGCSend = func(ctx context.Context, gcid [32]byte, text string) error {
		return br.hostRelay().SendGroupMessage(ctx, gcid, text)
	}
	br.restoreWalletMatches = br.recoveryWalletMatches
	br.restoreKeyOwned = br.verifyGamingWalletKey
	return br
}
