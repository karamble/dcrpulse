// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/karamble/dcrgaming-sdk/pkg/gaming/bridge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/config"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/types"
	"dcrpulse/internal/utils"
)

// GamingHost is this dashboard as the gaming bridge's host: its dcrd, its
// wallet, its Bison Relay client, and its operator, who is protected while
// protected reports true.
func GamingHost(protected func() bool) bridge.Host {
	return bridge.Host{
		Node: func() bridge.Chain {
			if c := rpc.DcrdClient; c != nil {
				return c
			}
			return nil
		},
		Wallet:   gamingWallet{},
		Relay:    gamingRelay{},
		Operator: gamingOperator{protected: protected},
	}
}

type gamingWallet struct{}

// Missing wallet metadata is not evidence of private-key ownership. Unlike the
// informational wallet-status display, payment authorization must fail closed.
func (gamingWallet) CheckSigning(ctx context.Context) error {
	name := ActiveWalletName()
	if name == "" {
		return fmt.Errorf("active signing wallet unavailable")
	}
	network, err := CurrentNetwork(ctx)
	if err != nil {
		return err
	}
	cfg, err := config.LoadWalletCfg(network, name)
	if err != nil {
		return err
	}
	return requireGamingSigningMetadata(cfg)
}

func requireGamingSigningMetadata(cfg interface {
	Get(string, any) (bool, error)
}) error {
	var watching *bool
	present, err := cfg.Get(config.KeyIsWatchOnly, &watching)
	if err != nil || !present || watching == nil {
		return fmt.Errorf("wallet signing capability is unknown; reopen the wallet in dcrpulse")
	}
	if *watching {
		return fmt.Errorf("gaming funds require a signing wallet")
	}
	return nil
}

func (gamingWallet) Accounts(ctx context.Context) ([]bridge.Account, error) {
	accounts, err := FetchAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]bridge.Account, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, bridge.Account{Name: a.AccountName, Number: a.AccountNumber})
	}
	return out, nil
}

func (gamingWallet) ReservedAccount(name string) bool { return IsReservedAccountName(name) }

func (gamingWallet) AccountXPub(ctx context.Context, account uint32) (string, error) {
	return GetAccountExtendedPubKey(ctx, account)
}

func (gamingWallet) ValidateAddress(ctx context.Context, address string) (bridge.AddressInfo, error) {
	r, err := ValidateAddress(ctx, address)
	if err != nil {
		return bridge.AddressInfo{}, err
	}
	return bridge.AddressInfo{IsValid: r.IsValid, IsMine: r.IsMine, IsScript: r.IsScript, AccountNumber: r.AccountNumber, PubKey: r.PubKey}, nil
}

func (gamingWallet) NextInternalAddress(ctx context.Context, account uint32) (string, error) {
	if err := spendGuard(); err != nil {
		return "", err
	}
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet unavailable")
	}
	addr, err := rpc.WalletGrpcClient.NextAddress(ctx, &pb.NextAddressRequest{Account: account, Kind: pb.NextAddressRequest_BIP0044_INTERNAL, GapPolicy: pb.NextAddressRequest_GAP_POLICY_ERROR})
	if err != nil {
		return "", err
	}
	return addr.Address, nil
}

func (gamingWallet) NextExternalAddress(ctx context.Context, account uint32) (string, error) {
	return GetNextAddress(ctx, account)
}

func (gamingWallet) ImportScript(ctx context.Context, scriptHex string) error {
	return ImportMsigScript(ctx, scriptHex, false, 0)
}

func (gamingWallet) Construct(ctx context.Context, account uint32, address string, atoms int64) ([]byte, error) {
	tx, err := ConstructTransaction(ctx, account, []types.TxRecipient{{Address: address, AmountAtoms: atoms}}, false)
	if err != nil {
		return nil, err
	}
	return tx.UnsignedTransaction, nil
}

func (gamingWallet) SignTransaction(ctx context.Context, account uint32, unsigned, passphrase []byte) ([]byte, error) {
	return signTransactionForSpend(ctx, account, unsigned, passphrase)
}

// WithUnlockedAccount clears passphrase as soon as the account is unlocked.
func (gamingWallet) WithUnlockedAccount(ctx context.Context, account uint32, passphrase []byte, fn func() error) error {
	if err := spendGuard(); err != nil {
		return err
	}
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet unavailable")
	}
	beginUnlockedOp()
	defer endUnlockedOp()
	unlocked, err := unlockAccountForSpend(ctx, account, passphrase)
	utils.Zero(passphrase)
	if err != nil {
		return err
	}
	if unlocked {
		defer func() {
			lockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := rpc.WalletGrpcClient.LockAccount(lockCtx, &pb.LockAccountRequest{AccountNumber: account}); err != nil {
				gameLog.Errorf("locking gaming account: %v", err)
			}
		}()
	}
	return fn()
}

func (gamingWallet) SignHash(ctx context.Context, address string, hash []byte) ([]byte, []byte, error) {
	if rpc.WalletGrpcClient == nil {
		return nil, nil, fmt.Errorf("wallet unavailable")
	}
	reply, err := rpc.WalletGrpcClient.SignHashes(ctx, &pb.SignHashesRequest{Address: address, Hashes: [][]byte{hash}})
	if err != nil {
		return nil, nil, err
	}
	if len(reply.Signatures) != 1 {
		return nil, nil, fmt.Errorf("wallet returned an unexpected signature")
	}
	return reply.PublicKey, reply.Signatures[0], nil
}

func (gamingWallet) Publish(ctx context.Context, signed []byte) (string, error) {
	return publishSignedTransaction(ctx, signed)
}

func (gamingWallet) Broadcast(ctx context.Context, signed []byte) (string, error) {
	return BroadcastSignedTransaction(ctx, signed)
}

func (gamingWallet) Transaction(ctx context.Context, hash chainhash.Hash) (bridge.WalletTx, bool, error) {
	if rpc.WalletGrpcClient == nil {
		return bridge.WalletTx{}, false, nil
	}
	r, err := rpc.WalletGrpcClient.GetTransaction(ctx, &pb.GetTransactionRequest{TransactionHash: hash[:]})
	if status.Code(err) == codes.NotFound {
		return bridge.WalletTx{}, false, nil
	}
	if err != nil {
		return bridge.WalletTx{}, false, err
	}
	if r.GetTransaction() == nil {
		return bridge.WalletTx{}, false, nil
	}
	tx := bridge.WalletTx{Raw: r.GetTransaction().GetTransaction(), Confirmations: r.GetConfirmations()}
	if b := r.GetBlockHash(); len(b) == chainhash.HashSize {
		var h chainhash.Hash
		copy(h[:], b)
		tx.BlockHash = &h
	}
	return tx, true, nil
}

func (gamingWallet) MinedTransactions(ctx context.Context, start int32, fn func(int32, [][]byte) error) error {
	if rpc.WalletGrpcClient == nil {
		return fmt.Errorf("wallet unavailable")
	}
	stream, err := rpc.WalletGrpcClient.GetTransactions(ctx, &pb.GetTransactionsRequest{StartingBlockHeight: start, EndingBlockHeight: -1})
	if err != nil {
		return err
	}
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		block := response.GetMinedTransactions()
		if block == nil {
			continue
		}
		txs := make([][]byte, 0, len(block.GetTransactions()))
		for _, details := range block.GetTransactions() {
			txs = append(txs, details.GetTransaction())
		}
		if err := fn(block.GetHeight(), txs); err != nil {
			return err
		}
	}
}

type gamingRelay struct{}

// The brclientd calls the relay makes. Settable for tests.
var (
	gamingBRIdentity = rpc.BrclientdUserPublicIdentity
	gamingBRSend     = rpc.BrclientdGCMessage
	gamingBRHistory  = rpc.BrclientdGCHistory
)

func (gamingRelay) Identity(ctx context.Context) ([32]byte, string, error) {
	var uid [32]byte
	raw, err := gamingBRIdentity(ctx)
	if err != nil {
		return uid, "", err
	}
	var public struct {
		Identity []byte `json:"identity"`
		Nick     string `json:"nick"`
	}
	if err := json.Unmarshal(raw, &public); err != nil {
		return uid, "", fmt.Errorf("BR identity unavailable")
	}
	if len(public.Identity) == len(uid) {
		copy(uid[:], public.Identity)
	}
	return uid, public.Nick, nil
}

// SendGroupMessage reports a send brclientd refused as not sent: a status
// reply means it answered without queueing the message.
func (gamingRelay) SendGroupMessage(ctx context.Context, gcid [32]byte, text string) error {
	id, err := rpc.ParseShortIDHex(hex.EncodeToString(gcid[:]))
	if err != nil {
		return err
	}
	err = gamingBRSend(ctx, id, text, 0)
	var refused *rpc.BrclientdStatusError
	if errors.As(err, &refused) {
		return fmt.Errorf("%w: %w", bridge.ErrNotSent, err)
	}
	return err
}

func (gamingRelay) GroupHistory(ctx context.Context, gcid [32]byte, page, pageSize int) ([]bridge.GroupEntry, error) {
	id, err := rpc.ParseShortIDHex(hex.EncodeToString(gcid[:]))
	if err != nil {
		return nil, err
	}
	raw, err := gamingBRHistory(ctx, id, page, pageSize)
	if err != nil {
		return nil, err
	}
	var got struct {
		Entries []struct {
			Message string `json:"message"`
			From    string `json:"from"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		return nil, err
	}
	out := make([]bridge.GroupEntry, 0, len(got.Entries))
	for _, e := range got.Entries {
		out = append(out, bridge.GroupEntry{From: e.From, Message: e.Message})
	}
	return out, nil
}

type gamingOperator struct{ protected func() bool }

func (o gamingOperator) Protected() bool { return o.protected != nil && o.protected() }

// PresenceChanged carries an invalidation, not a potentially reordered ready
// flag. Browsers reread the current registry, including any other stream still
// connected for this game. No credential or wallet data leaves here.
func (gamingOperator) PresenceChanged(game string) {
	payload, _ := json.Marshal(struct {
		Game string `json:"game"`
	}{game})
	PublishBisonrelayEvent("gaming-presence", payload)
}
