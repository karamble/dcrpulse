package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telling a game where to pay this user, rather than asking it.
//
// A game builds transactions that pay people - a settlement pays the winners, a
// forfeited bond is split among the seats that stayed - and every one of them
// names an address. If the page could supply that address, the page could
// redirect the winnings, and a compromised or merely dishonest game would have
// a straightforward way to take everything anybody won.
//
// So the host derives it, from the wallet account the user bound to gaming, and
// pushes it in with the game's own token before the page has any credential at
// all. The game's own pre-signing then refuses any branch that pays anywhere
// else - which is a check that only means something because of what happens
// here.

// PinGamingPayoutAddress gives a game a fresh address from the bound account.
//
// Fresh each time rather than reused, because addresses are cheap and a fixed
// one links every table this user ever sat at, to everybody they sat with.
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
