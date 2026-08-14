// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"dcrpulse/internal/gamingpb"
)

// gamingPayoutTimeout bounds the wait. Shorter than a reclaim: this records an
// address and announces it, and builds no transaction.
const gamingPayoutTimeout = 20 * time.Second

// payoutAddress derives where a game's winnings land. Settable for the same
// reason as the reclaim and spend seams. Production does not set it.
var payoutAddress = gamingReceiveAddress

// tried is the games this process has already pinned an address for, so a game
// that reports itself every few seconds is not asked again on every report.
//
// A failure is remembered too. Deriving an address touches the wallet, and a
// game that cannot take one - no account bound, wrong network - would otherwise
// have the wallet asked for a fresh address forever. The operator can force
// another attempt through the route.
var tried = struct {
	sync.Mutex
	m map[string]bool
}{m: map[string]bool{}}

// GamingGameArrived is called when a game opens its stream.
//
// It asks the game to report itself, because a game volunteers nothing and the
// console would otherwise know only that something connected. Whatever depends
// on knowing follows from the answer.
func GamingGameArrived(game string) {
	if err := RefreshGamingState(context.Background(), game); err != nil {
		gameLog.Warnf("%s did not say what it holds: %v", game, err)
		return
	}
	PinGamingPayoutOnce(game)
}

// PinGamingPayoutOnce pins an address the first time a game reports itself
// without one, and does nothing every time after that.
//
// Hooked to the state report because that is when the host learns a game has no
// address. Nobody can be paid out of a table until every seat has said where
// its share goes, and a seat that never says holds up everybody's money - which
// is exactly what happened the first time a hand was played here.
func PinGamingPayoutOnce(game string) {
	if s := GamingReportedState(game); s == nil || s.GetPayoutAddress() != "" {
		return
	}
	tried.Lock()
	if tried.m[game] {
		tried.Unlock()
		return
	}
	tried.m[game] = true
	tried.Unlock()

	addr, err := PinGamingPayout(context.Background(), game)
	if err != nil {
		gameLog.Warnf("could not tell %s where to be paid, so a table it plays cannot pay out: %v",
			game, err)
		return
	}
	gameLog.Infof("%s will be paid at %s", game, addr)
}

// PinGamingPayout tells a game where its winnings are to be paid, and reports
// the address it pinned.
//
// A table cannot pay anybody out until every seat has said where its share
// goes: the settlement spends the table's escrow and needs every signature, so
// one seat that has not answered holds up everyone's money.
//
// The address is derived here from the game's bound account and never taken
// from the game, for the same reason a reclaim's destination is not: a game
// that chose where its winnings landed could simply choose itself.
//
// Pinning is skipped when the game already has one. The old build re-pinned a
// fresh address every time a panel opened, and changing it under a live table
// makes the seats build different settlement drafts and all refuse - a table
// wedged rather than paid.
func PinGamingPayout(ctx context.Context, game string) (string, error) {
	if !gamingGameRegistered(game) {
		return "", ErrGamingGameNotRegistered
	}
	if gamingRequest == nil {
		return "", ErrGamingGameNotConnected
	}
	if have := GamingReportedState(game).GetPayoutAddress(); have != "" {
		return have, nil
	}

	addr, err := payoutAddress(ctx, game)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, gamingPayoutTimeout)
	defer cancel()

	reply, err := gamingRequest(ctx, game, &gamingpb.BridgeRequest{
		Req: &gamingpb.BridgeRequest_SetPayout{
			SetPayout: &gamingpb.SetPayoutAddress{Address: addr},
		},
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return "", fmt.Errorf("%s did not answer within %s", game, gamingPayoutTimeout)
	}
	if err != nil {
		return "", err
	}
	if !reply.GetOk() {
		// The game refuses an address it cannot decode on this network,
		// which is the answer worth showing: every claim built to pay it
		// would fail, and it would fail at the other seats.
		return "", fmt.Errorf("%s would not take the payout address: %s", game, reply.GetError())
	}
	return addr, nil
}
