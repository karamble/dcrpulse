package gamingcore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"dcrpulse/internal/gamingfunds"
)

var gamingKeyMu sync.Mutex

// importedXpubAccountBase is the first account number dcrwallet gives an
// imported xpub account.
const importedXpubAccountBase = uint32(1) << 31

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
	owner, err := hostWallet().ValidateAddress(ctx, key.Address)
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
	return hostWallet().CheckSigning(ctx)
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
	// Never wrap the address gap and silently reuse an earlier financial key.
	addr, err := hostWallet().NextInternalAddress(ctx, scope.Account)
	if err != nil {
		return "", err
	}
	owner, err := hostWallet().ValidateAddress(ctx, addr)
	if err != nil {
		return "", err
	}
	key = gamingfunds.WalletKey{Scope: scope, Table: table, Address: addr, Public: hex.EncodeToString(owner.PubKey)}
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
	defer clear(passphrase)
	if err := recoveryWalletMatches(ctx, scope); err != nil {
		return nil, err
	}
	var out []byte
	err := hostWallet().WithUnlockedAccount(ctx, scope.Account, passphrase, func() error {
		var err error
		out, err = fn(func(key gamingfunds.WalletKey, hash []byte) ([]byte, error) {
			if key.Scope != scope || len(hash) != 32 {
				return nil, fmt.Errorf("invalid wallet signing scope")
			}
			if err := verifyGamingWalletKey(ctx, key); err != nil {
				return nil, err
			}
			pub, sig, err := hostWallet().SignHash(ctx, key.Address, hash)
			if err != nil {
				return nil, err
			}
			if hex.EncodeToString(pub) != key.Public || len(sig) == 0 {
				return nil, fmt.Errorf("wallet returned an unexpected signature")
			}
			return sig, nil
		})
		return err
	})
	return out, err
}
