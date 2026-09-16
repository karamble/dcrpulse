package services

import (
	"context"
	"fmt"

	"dcrpulse/internal/gamingpb"
)

// GamingReportedState is the game's cached, nonfinancial presentation state.
// Financial status always comes from the bridge authority ledger.
func GamingReportedState(game string) *gamingpb.GameState {
	if gamingState == nil {
		return nil
	}
	return gamingState(game)
}

// RefreshGamingState asks a connected game to update its presentation state.
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
