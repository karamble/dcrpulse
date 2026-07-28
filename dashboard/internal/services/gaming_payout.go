// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PinGamingPayoutAddress gives a game a fresh address from the bound gaming
// account, using the game's own token.
//
// The host derives it so a game's page cannot name where winnings go. Fresh
// each time, so a fixed address does not link every table the user sits at.
func PinGamingPayoutAddress(ctx context.Context, game string) (string, error) {
	base, err := GamingGameURL(game)
	if err != nil {
		return "", err
	}
	token, ok := GamingGameToken(game)
	if !ok {
		return "", ErrGamingGameNotInstalled
	}

	account, err := gamingAccountNumber(ctx)
	if err != nil {
		return "", fmt.Errorf("no gaming account to be paid into: %w", err)
	}
	address, err := GetNextAddress(ctx, account)
	if err != nil {
		return "", fmt.Errorf("could not derive an address to be paid at: %w", err)
	}

	body, err := json.Marshal(map[string]string{"address": address})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/payout/set", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not tell %s where to pay you: %w", game, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s refused the payout address (%d)", game, resp.StatusCode)
	}
	return address, nil
}
