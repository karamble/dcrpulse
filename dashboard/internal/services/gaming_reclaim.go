// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"dcrpulse/internal/gamingpb"
)

// gamingReclaimTimeout bounds how long the console waits for a reclaim.
//
// Longer than a join: this builds a transaction and waits for the network to
// take it. A reclaim that outruns it is still completed by the game, which is
// why the caller is told the coin may be on its way rather than that it failed.
const gamingReclaimTimeout = 2 * time.Minute

// gamingReclaimKinds maps what the operator asked for onto the wire.
var gamingReclaimKinds = map[string]gamingpb.Reclaim_Kind{
	"bond":      gamingpb.Reclaim_BOND,
	"stake":     gamingpb.Reclaim_STAKE,
	"tablebond": gamingpb.Reclaim_TABLE_BOND,
}

// ErrGamingReclaimKind is a reclaim naming something that is not locked coin.
var ErrGamingReclaimKind = errors.New("kind must be bond, stake or tablebond")

// reclaimAddress derives where reclaimed coin lands. Settable for the same
// reason as the spend seams: the rule that a game never chooses its own
// destination is only exercised if it can be exercised without a wallet.
// Production does not set it.
var reclaimAddress = gamingReceiveAddress

// ErrGamingReclaimInFlight is a reclaim that outran the wait.
//
// Its own error because it is not a failure: the game completes a reclaim
// regardless of the deadline, so telling the operator it failed would invite a
// second attempt at coin that is already moving.
var ErrGamingReclaimInFlight = errors.New("the reclaim did not answer in time and may already have been broadcast")

// ReclaimGamingCoin takes back coin a game locked, into the account it is bound
// to.
//
// The destination is derived here and never taken from the caller: a game that
// chose where its refunds landed could simply choose itself.
func ReclaimGamingCoin(ctx context.Context, game, kind, sid, outpoint string) (string, error) {
	k, ok := gamingReclaimKinds[strings.ToLower(strings.TrimSpace(kind))]
	if !ok {
		return "", ErrGamingReclaimKind
	}
	if !gamingGameRegistered(game) {
		return "", ErrGamingGameNotRegistered
	}
	if gamingRequest == nil {
		return "", ErrGamingGameNotConnected
	}
	dest, err := reclaimAddress(ctx, game)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, gamingReclaimTimeout)
	defer cancel()

	reply, err := gamingRequest(ctx, game, &gamingpb.BridgeRequest{
		Req: &gamingpb.BridgeRequest_Reclaim{
			Reclaim: &gamingpb.Reclaim{
				Kind:     k,
				Sid:      sid,
				DestAddr: dest,
				Outpoint: outpoint,
			},
		},
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return "", ErrGamingReclaimInFlight
	}
	if err != nil {
		return "", err
	}
	if !reply.GetOk() {
		// The game's own refusal says how many blocks are left on the
		// lock, which is the only useful thing to show.
		return "", fmt.Errorf("%s did not reclaim it: %s", game, reply.GetError())
	}
	txid := reply.GetReclaim().GetTxid()
	if txid == "" {
		return "", fmt.Errorf("%s did not say what it sent", game)
	}
	return txid, nil
}

// gamingReceiveAddress is where reclaimed coin lands: a fresh address in the
// account this game is bound to, never one a game supplied.
func gamingReceiveAddress(ctx context.Context, game string) (string, error) {
	account, err := gamingAccountNumber(ctx, game)
	if err != nil {
		return "", fmt.Errorf("no account to pay %q's coin into: %w", game, err)
	}
	addr, err := GetNextAddress(ctx, account)
	if err != nil {
		return "", fmt.Errorf("could not derive an address to pay into: %w", err)
	}
	return addr, nil
}

// GamingReportedState is the last thing a game told the bridge about itself.
//
// Read from the bridge's cache rather than asked for: a game reports when
// something moves, and the console renders a game that is not connected right
// now as it last was.
func GamingReportedState(game string) *gamingpb.GameState {
	if gamingState == nil {
		return nil
	}
	return gamingState(game)
}

// RefreshGamingState asks a game to report itself now.
func RefreshGamingState(ctx context.Context, game string) error {
	if !gamingGameRegistered(game) {
		return ErrGamingGameNotRegistered
	}
	if gamingRequest == nil {
		return ErrGamingGameNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, gamingJoinTimeout)
	defer cancel()

	reply, err := gamingRequest(ctx, game, &gamingpb.BridgeRequest{
		Req: &gamingpb.BridgeRequest_RefreshState{RefreshState: &gamingpb.RefreshState{}},
	})
	if err != nil {
		return err
	}
	if !reply.GetOk() {
		return fmt.Errorf("%s could not report itself: %s", game, reply.GetError())
	}
	return nil
}
