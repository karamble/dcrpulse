package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/utils"
	pb "decred.org/dcrwallet/v5/rpc/walletrpc"
)

var gamingKeyMu sync.Mutex

func verifyGamingWalletKey(ctx context.Context, key gamingfunds.WalletKey) error {
	// Only seed-derived accounts are eligible. An imported xpub can report
	// IsMine without providing any private signing authority.
	if key.Scope.Account >= importedXpubAccountBase-1 {
		return fmt.Errorf("gaming funds require a seed-derived signing account")
	}

	if err := recoveryWalletMatches(ctx, key.Scope); err != nil {
		return err
	}
	if err := requireGamingSigningWallet(ctx); err != nil {
		return err
	}
	owner, err := ValidateAddress(ctx, key.Address)
	if err != nil {
		return err
	}
	if !owner.IsValid || !owner.IsMine || owner.IsScript || owner.AccountNumber != key.Scope.Account || hex.EncodeToString(owner.PubKey) != key.Public {
		return fmt.Errorf("financial key is not owned by the original wallet account")
	}
	return nil
}

// Missing wallet metadata is not evidence of private-key ownership. Unlike the
// informational wallet-status display, payment authorization must fail closed.
func requireGamingSigningWallet(ctx context.Context) error {
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

func ensureGamingWalletKey(ctx context.Context, store *gamingfunds.Store, scope gamingfunds.Scope, table string) (string, error) {
	gamingKeyMu.Lock()
	defer gamingKeyMu.Unlock()
	if _, err := store.AuthorizedTable(scope, table); err != nil {
		return "", err
	}
	key, err := store.WalletKey(scope, table)
	if err == nil {
		if err = verifyGamingWalletKey(ctx, key); err != nil {
			return "", err
		}
		return key.Public, nil
	}
	if !errors.Is(err, gamingfunds.ErrKeyNotRegistered) {
		return "", err
	}
	if err = spendGuard(); err != nil {
		return "", err
	}
	if rpc.WalletGrpcClient == nil {
		return "", fmt.Errorf("wallet unavailable")
	}
	// Never wrap the address gap and silently reuse an earlier financial key.
	addr, err := rpc.WalletGrpcClient.NextAddress(ctx, &pb.NextAddressRequest{Account: scope.Account, Kind: pb.NextAddressRequest_BIP0044_INTERNAL, GapPolicy: pb.NextAddressRequest_GAP_POLICY_ERROR})
	if err != nil {
		return "", err
	}
	owner, err := ValidateAddress(ctx, addr.Address)
	if err != nil {
		return "", err
	}
	key = gamingfunds.WalletKey{Scope: scope, Table: table, Address: addr.Address, Public: hex.EncodeToString(owner.PubKey)}
	if err = verifyGamingWalletKey(ctx, key); err != nil {
		return "", err
	}
	if err = store.RegisterKey(key); err != nil {
		return "", err
	}
	return key.Public, nil
}

// withGamingWalletSigner is reachable from dashboard approval handlers only.
// Hashes are computed by the authority from an already validated transaction.
func withGamingWalletSigner(ctx context.Context, scope gamingfunds.Scope, passphrase []byte, fn func(gamingfunds.WalletSigner) ([]byte, error)) ([]byte, error) {
	defer utils.Zero(passphrase)
	if err := spendGuard(); err != nil {
		return nil, err
	}
	if err := recoveryWalletMatches(ctx, scope); err != nil {
		return nil, err
	}
	if rpc.WalletGrpcClient == nil {
		return nil, fmt.Errorf("wallet unavailable")
	}
	beginUnlockedOp()
	defer endUnlockedOp()
	unlocked, err := unlockAccountForSpend(ctx, scope.Account, passphrase)
	if err != nil {
		return nil, err
	}
	if unlocked {
		defer func() {
			lockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := rpc.WalletGrpcClient.LockAccount(lockCtx, &pb.LockAccountRequest{AccountNumber: scope.Account}); err != nil {
				gameLog.Errorf("locking gaming account: %v", err)
			}
		}()
	}
	return fn(func(key gamingfunds.WalletKey, hash []byte) ([]byte, error) {
		if key.Scope != scope || len(hash) != 32 {
			return nil, fmt.Errorf("invalid wallet signing scope")
		}
		if err := verifyGamingWalletKey(ctx, key); err != nil {
			return nil, err
		}
		reply, err := rpc.WalletGrpcClient.SignHashes(ctx, &pb.SignHashesRequest{Address: key.Address, Hashes: [][]byte{hash}})
		if err != nil {
			return nil, err
		}
		if hex.EncodeToString(reply.PublicKey) != key.Public || len(reply.Signatures) != 1 {
			return nil, fmt.Errorf("wallet returned an unexpected signature")
		}
		return reply.Signatures[0], nil
	})
}
