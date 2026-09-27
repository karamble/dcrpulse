// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	chainjson "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// Host is the wallet app a bridge runs inside: the node it reads the chain
// from, custody of the operator's coins, the Bison Relay client it speaks
// through, and the operator it answers to. A part left nil reads as
// unavailable.
type Host struct {
	// Node returns the node's client, or nil while there is none.
	Node     func() Chain
	Wallet   Wallet
	Relay    Relay
	Operator Operator
}

// Chain is the node the bridge reads the chain from. Its methods are those of
// *rpcclient.Client, so a host can hand one over as it is.
type Chain interface {
	GetBestBlock(ctx context.Context) (*chainhash.Hash, int64, error)
	GetBlockChainInfo(ctx context.Context) (*chainjson.GetBlockChainInfoResult, error)
	GetBlockCount(ctx context.Context) (int64, error)
	GetBlockHash(ctx context.Context, blockHeight int64) (*chainhash.Hash, error)
	GetBlockHeaderVerbose(ctx context.Context, hash *chainhash.Hash) (*chainjson.GetBlockHeaderVerboseResult, error)
	GetInfo(ctx context.Context) (*chainjson.InfoChainResult, error)
	GetRawMempool(ctx context.Context, txType chainjson.GetRawMempoolTxTypeCmd) ([]*chainhash.Hash, error)
	GetRawTransaction(ctx context.Context, txHash *chainhash.Hash) (*dcrutil.Tx, error)
	GetRawTransactionVerbose(ctx context.Context, txHash *chainhash.Hash) (*chainjson.TxRawResult, error)
	GetTxOut(ctx context.Context, txHash *chainhash.Hash, index uint32, tree int8, mempool bool) (*chainjson.GetTxOutResult, error)
}

// Wallet is custody of the operator's coins. Each method applies the host's
// own guards, and fails rather than act on a wallet it cannot vouch for.
type Wallet interface {
	// CheckSigning fails unless the active wallet can sign: a watching-only
	// wallet, or one whose kind is unknown, holds no gaming funds.
	CheckSigning(ctx context.Context) error
	// Accounts lists the wallet's accounts.
	Accounts(ctx context.Context) ([]Account, error)
	// ReservedAccount reports an account name the host keeps for itself.
	ReservedAccount(name string) bool
	// AccountXPub is an account's extended public key.
	AccountXPub(ctx context.Context, account uint32) (string, error)
	// ValidateAddress says whether an address is the wallet's, and where.
	ValidateAddress(ctx context.Context, address string) (AddressInfo, error)
	// NextInternalAddress hands out an account's next internal address, and
	// fails rather than wrap around to one handed out before.
	NextInternalAddress(ctx context.Context, account uint32) (string, error)
	// NextExternalAddress hands out an account's next receiving address.
	NextExternalAddress(ctx context.Context, account uint32) (string, error)
	// ImportScript makes the wallet watch a pay-to-script-hash script.
	ImportScript(ctx context.Context, scriptHex string) error
	// Construct builds an unsigned transaction paying atoms to address from
	// account, with the host's own change policy.
	Construct(ctx context.Context, account uint32, address string, atoms int64) ([]byte, error)
	// SignTransaction signs an unsigned transaction from account.
	SignTransaction(ctx context.Context, account uint32, unsigned, passphrase []byte) ([]byte, error)
	// WithUnlockedAccount runs fn with account unlocked by passphrase, and
	// locks it again afterwards only if it was locked before.
	WithUnlockedAccount(ctx context.Context, account uint32, passphrase []byte, fn func() error) error
	// SignHash signs a 32-byte hash with the key behind address, and returns
	// that key and the signature. Its account must be unlocked.
	SignHash(ctx context.Context, address string, hash []byte) (pubKey, sig []byte, err error)
	// Publish relays a transaction the wallet signed. It may have reached the
	// network even when Publish returns an error.
	Publish(ctx context.Context, signed []byte) (string, error)
	// Broadcast relays any signed transaction, and returns the wallet's own
	// error so a transient failure can be told from a refusal.
	Broadcast(ctx context.Context, signed []byte) (string, error)
	// Transaction is one of the wallet's own transactions. ok is false for one
	// it has no record of, or while it cannot be asked.
	Transaction(ctx context.Context, hash chainhash.Hash) (tx WalletTx, ok bool, err error)
	// MinedTransactions calls fn for each block from height start on with the
	// wallet's transactions mined in it, in order.
	MinedTransactions(ctx context.Context, start int32, fn func(height int32, txs [][]byte) error) error
}

// Account is one of the wallet's accounts.
type Account struct {
	Name   string
	Number uint32
}

// AddressInfo is what the wallet knows of an address.
type AddressInfo struct {
	IsValid, IsMine, IsScript bool
	AccountNumber             uint32
	PubKey                    []byte
}

// WalletTx is a transaction the wallet holds. BlockHash is nil while it is
// unmined.
type WalletTx struct {
	Raw           []byte
	Confirmations int32
	BlockHash     *chainhash.Hash
}

// Relay is the Bison Relay client the bridge speaks through.
type Relay interface {
	// Identity is this client's user id and the nick Bison Relay logs its own
	// messages under.
	Identity(ctx context.Context) (uid [32]byte, nick string, err error)
	// SendGroupMessage posts text to a group chat. An error matching
	// ErrNotSent means Bison Relay certainly did not take it; any other error
	// leaves that unknown.
	SendGroupMessage(ctx context.Context, gcid [32]byte, text string) error
	// GroupHistory is one page of Bison Relay's own log of a group chat.
	GroupHistory(ctx context.Context, gcid [32]byte, page, pageSize int) ([]GroupEntry, error)
}

// GroupEntry is one message in a group chat's log.
type GroupEntry struct {
	From, Message string
}

// ErrNotSent is a message Bison Relay certainly did not take.
var ErrNotSent = errors.New("bison relay did not take the message")

// Operator is the person the bridge answers to.
type Operator interface {
	// Protected reports that the operator's console is behind a password.
	// The bridge serves no game while it is not.
	Protected() bool
	// PresenceChanged says a game connected or went away.
	PresenceChanged(game string)
}

var host Host

// Configure hands the bridge its data directory and its host. It is called
// once, before anything else in this package runs.
func Configure(dataDir string, h Host) {
	GamingStateDir = dataDir
	host = h
}

// hostNode is the node's client, nil while there is none.
func hostNode() Chain {
	if host.Node == nil {
		return nil
	}
	return host.Node()
}

func hostWallet() Wallet {
	if host.Wallet == nil {
		return noWallet{}
	}
	return host.Wallet
}

func hostRelay() Relay {
	if host.Relay == nil {
		return noRelay{}
	}
	return host.Relay
}

func hostOperator() Operator {
	if host.Operator == nil {
		return noOperator{}
	}
	return host.Operator
}

var (
	networkMu   sync.Mutex
	networkName string
)

// currentNetwork is "mainnet", "testnet" or "simnet", as the node says. Only
// an answer is kept; a failed lookup is asked again next time.
func currentNetwork(ctx context.Context) (string, error) {
	networkMu.Lock()
	defer networkMu.Unlock()
	if networkName != "" {
		return networkName, nil
	}
	node := hostNode()
	if node == nil {
		return "", ErrGamingChainUnavailable
	}
	info, err := node.GetBlockChainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("get blockchain info: %w", err)
	}
	chain := strings.ToLower(strings.TrimSpace(info.Chain))
	switch {
	case strings.Contains(chain, "main"):
		networkName = "mainnet"
	case strings.Contains(chain, "test"):
		networkName = "testnet"
	case strings.Contains(chain, "sim"):
		networkName = "simnet"
	default:
		networkName = chain
	}
	return networkName, nil
}

// chainParams is the connected network's consensus parameters.
func chainParams(ctx context.Context) (*chaincfg.Params, error) {
	net, err := currentNetwork(ctx)
	if err != nil {
		return nil, err
	}
	switch net {
	case "mainnet":
		return chaincfg.MainNetParams(), nil
	case "testnet":
		return chaincfg.TestNet3Params(), nil
	case "simnet":
		return chaincfg.SimNetParams(), nil
	case "regnet":
		return chaincfg.RegNetParams(), nil
	}
	return nil, fmt.Errorf("unknown network %q", net)
}

var (
	errWalletUnavailable = errors.New("wallet unavailable")
	errRelayUnavailable  = errors.New("bison relay unavailable")
)

// noWallet, noRelay and noOperator stand in for a part the host left out.
type noWallet struct{}

func (noWallet) CheckSigning(context.Context) error          { return errWalletUnavailable }
func (noWallet) Accounts(context.Context) ([]Account, error) { return nil, errWalletUnavailable }
func (noWallet) ReservedAccount(string) bool                 { return false }
func (noWallet) AccountXPub(context.Context, uint32) (string, error) {
	return "", errWalletUnavailable
}
func (noWallet) ValidateAddress(context.Context, string) (AddressInfo, error) {
	return AddressInfo{}, errWalletUnavailable
}
func (noWallet) NextInternalAddress(context.Context, uint32) (string, error) {
	return "", errWalletUnavailable
}
func (noWallet) NextExternalAddress(context.Context, uint32) (string, error) {
	return "", errWalletUnavailable
}
func (noWallet) ImportScript(context.Context, string) error { return errWalletUnavailable }
func (noWallet) Construct(context.Context, uint32, string, int64) ([]byte, error) {
	return nil, errWalletUnavailable
}
func (noWallet) SignTransaction(context.Context, uint32, []byte, []byte) ([]byte, error) {
	return nil, errWalletUnavailable
}
func (noWallet) WithUnlockedAccount(context.Context, uint32, []byte, func() error) error {
	return errWalletUnavailable
}
func (noWallet) SignHash(context.Context, string, []byte) ([]byte, []byte, error) {
	return nil, nil, errWalletUnavailable
}
func (noWallet) Publish(context.Context, []byte) (string, error)   { return "", errWalletUnavailable }
func (noWallet) Broadcast(context.Context, []byte) (string, error) { return "", errWalletUnavailable }
func (noWallet) Transaction(context.Context, chainhash.Hash) (WalletTx, bool, error) {
	return WalletTx{}, false, nil
}
func (noWallet) MinedTransactions(context.Context, int32, func(int32, [][]byte) error) error {
	return errWalletUnavailable
}

type noRelay struct{}

func (noRelay) Identity(context.Context) ([32]byte, string, error) {
	return [32]byte{}, "", errRelayUnavailable
}
func (noRelay) SendGroupMessage(context.Context, [32]byte, string) error {
	return fmt.Errorf("%w: %w", ErrNotSent, errRelayUnavailable)
}
func (noRelay) GroupHistory(context.Context, [32]byte, int, int) ([]GroupEntry, error) {
	return nil, errRelayUnavailable
}

type noOperator struct{}

func (noOperator) Protected() bool        { return false }
func (noOperator) PresenceChanged(string) {}
