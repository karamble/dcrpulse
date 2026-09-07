// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
)

// The privacy status only needs which account is "mixed" and which "unmixed",
// which the wallet's account list answers on its own. It must not go through the
// balance listing, and an unreadable wallet is an error, not "not configured".
func TestFindPrivacyAccountsNeedsOnlyTheAccountList(t *testing.T) {
	withWalletClient(t, nil) // no JSON-RPC client: the balance path is closed
	withWallet(t, &privacyWallet{})

	mixed, change, configured, err := FindPrivacyAccounts(context.Background())
	if err != nil {
		t.Fatalf("FindPrivacyAccounts() = %v", err)
	}
	if !configured || mixed != 5 || change != 6 {
		t.Fatalf("configured=%v mixed=%d change=%d; want the pair at 5 and 6 from the account list alone", configured, mixed, change)
	}

	withWallet(t, &privacyWallet{accountsErr: errors.New("wallet is syncing")})
	if _, _, _, err := FindPrivacyAccounts(context.Background()); err == nil {
		t.Fatal("an unreadable wallet reads as a plain wallet")
	}
}
