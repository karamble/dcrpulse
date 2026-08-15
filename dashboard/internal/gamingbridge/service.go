// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

// contractVersion is the version of the wire contract this bridge answers.
const contractVersion = 1

// errNoChain is what a game is told when the node is not reachable.
//
// Unavailable rather than Internal, because it is a condition that passes: a
// game that retries later will get an answer, and one told Internal would be
// right to give up.
var errNoChain = status.Error(codes.Unavailable, "the chain is not reachable from this bridge")

// Hello is where a game finds out what it is.
//
// The id in the request is advisory and is checked rather than believed. A game
// that has been handed the wrong credential is a configuration mistake the
// operator can fix, and refusing loudly is what makes it visible - where taking
// the game's word would let it quietly act as something else, spending against
// caps set for another game.
func (s *Server) Hello(ctx context.Context, req *gamingpb.HelloRequest) (*gamingpb.HelloReply, error) {
	game := callerGame(ctx)
	if claimed := req.GetGameId(); claimed != "" && claimed != game {
		return nil, status.Errorf(codes.FailedPrecondition,
			"this credential is registered to a different game than %q; it has been copied to the wrong place", claimed)
	}

	reply := &gamingpb.HelloReply{
		Game:                  game,
		BridgeContractVersion: contractVersion,
	}
	if s.cfg.Network != nil {
		network, ok := s.cfg.Network()
		if !ok {
			// A game must refuse to proceed without knowing the network, so
			// answering with an empty one would be worse than not answering.
			return nil, errNoChain
		}
		reply.Network = network
		reply.ChainAvailable = true
	}
	if s.cfg.Policy != nil {
		perTable, perDay, bound := s.cfg.Policy(game)
		reply.Policy = &gamingpb.PolicySummary{
			PerTableCapAtoms: perTable,
			PerDayCapAtoms:   perDay,
			AccountBound:     bound,
		}
	}

	gameLog.Infof("%s connected (%s, protocol %d)", game, req.GetClientVersion(), req.GetGameProtocolVersion())
	return reply, nil
}

// Subscribe is the one channel from this bridge to a game.
//
// Everything the operator decides reaches the game here, and so does every
// frame from its tables. It ends when the game hangs up, when the process does,
// or when the credential holding it open is withdrawn.
func (s *Server) Subscribe(req *gamingpb.SubscribeRequest, stream grpc.ServerStreamingServer[gamingpb.BridgeEvent]) error {
	ctx := stream.Context()
	game := callerGame(ctx)

	live := s.reg.add(game)
	defer s.reg.remove(game, live)

	var missed []string
	if s.cfg.TakeMissed != nil {
		missed = s.cfg.TakeMissed(game)
	}
	// StreamStart first, always. It is the only place a gap is declared, and
	// a game that inferred one from its own reconnect loop would resync every
	// table on every failed dial.
	missedAll := false
	if s.cfg.TookMissedAll != nil {
		missedAll = s.cfg.TookMissedAll(game)
	}
	start := s.reg.streamStart(game, req, missed, missedAll)
	if err := stream.Send(&gamingpb.BridgeEvent{
		Event: &gamingpb.BridgeEvent_Start{Start: start},
	}); err != nil {
		return err
	}
	if start.GetGap() {
		gameLog.Infof("%s reconnected with a gap (%s), %d table(s) named",
			game, start.GetGapScope(), len(start.GetGapGcids()))
	}
	// Now that there is a stream, the host can ask this game about itself.
	// A game volunteers nothing, so without this the console never learns
	// what it holds - and nothing that depends on knowing can happen.
	if s.cfg.OnConnect != nil {
		go s.cfg.OnConnect(game)
	}

	var frames <-chan Frame
	if s.cfg.Frames != nil {
		ch, stop := s.cfg.Frames(game, frameBuffer)
		defer stop()
		frames = ch
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-live.done:
			// The credential was withdrawn. Told the same way a stranger is
			// told, because that is now what this caller is.
			return errNotHere

		case f, ok := <-frames:
			if !ok {
				// The fan-out closed underneath us. Ending the stream is
				// honest: staying open would look like a game that is
				// connected and receiving when it is only connected.
				return status.Error(codes.Unavailable, "the frame feed closed")
			}
			if err := stream.Send(s.reg.frameEvent(game, f)); err != nil {
				return err
			}

		case ev := <-live.events:
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}

// SendFrame carries a frame the game built out to one of its tables.
//
// The bridge reads only the routing key: what a frame means is between the
// players, who sign their own traffic and check each other's. What it does
// check is that the frame is this game's own, so a connected game cannot send
// another's traffic or use the bridge to send chat.
func (s *Server) SendFrame(ctx context.Context, req *gamingpb.SendFrameRequest) (*gamingpb.SendFrameReply, error) {
	if s.cfg.SendFrame == nil {
		return nil, errNotHere
	}
	if err := s.cfg.SendFrame(ctx, callerGame(ctx), req.GetGcid(), req.GetFrame()); err != nil {
		return nil, gameErr(callerGame(ctx), "SendFrame", codes.InvalidArgument, err)
	}
	return &gamingpb.SendFrameReply{}, nil
}

// ChainTip reports where the chain is.
//
// A game needs this to agree a deadline with its peers: a height is a fact
// everyone can check, where a clock is each machine's opinion and nobody can be
// shown to have read it wrong.
func (s *Server) ChainTip(ctx context.Context, _ *gamingpb.ChainTipRequest) (*gamingpb.ChainTipReply, error) {
	if s.cfg.ChainTip == nil {
		return nil, errNoChain
	}
	height, hash, err := s.cfg.ChainTip(ctx)
	if err != nil {
		return nil, errNoChain
	}
	return &gamingpb.ChainTipReply{Height: height, Hash: hash}, nil
}

// BlockHash names a block by height.
func (s *Server) BlockHash(ctx context.Context, req *gamingpb.BlockHashRequest) (*gamingpb.BlockHashReply, error) {
	if s.cfg.BlockHash == nil {
		return nil, errNoChain
	}
	// The contract carries a height as unsigned, so the negative height the
	// old HTTP route had to reject cannot be expressed here.
	hash, err := s.cfg.BlockHash(ctx, int64(req.GetHeight()))
	if err != nil {
		return nil, errNoChain
	}
	return &gamingpb.BlockHashReply{Height: req.GetHeight(), Hash: hash}, nil
}

// Outpoint is how a game finds its own output among a transaction's vouts.
func (s *Server) Outpoint(ctx context.Context, req *gamingpb.OutpointRequest) (*gamingpb.OutpointReply, error) {
	if s.cfg.Outpoint == nil {
		return nil, errNoChain
	}
	out, err := s.cfg.Outpoint(ctx, req.GetTxid(), req.GetVout(), req.GetIncludeMempool())
	if err != nil {
		return nil, errNoChain
	}
	return &gamingpb.OutpointReply{
		Found:         out.Found,
		ValueAtoms:    out.ValueAtoms,
		PkScriptHex:   out.PkScriptHex,
		Confirmations: out.Confirmations,
		Coinbase:      out.Coinbase,
	}, nil
}

// ReportState records what a game says it is doing.
//
// Kept rather than acted on, and kept per game, so the console can render a
// game as it last was when it is not connected - which is most of the time, for
// something a person runs on their own machine.
func (s *Server) ReportState(ctx context.Context, state *gamingpb.GameState) (*gamingpb.ReportStateReply, error) {
	game := callerGame(ctx)
	s.reg.setState(game, state)
	if s.cfg.OnState != nil {
		go s.cfg.OnState(game)
	}
	return &gamingpb.ReportStateReply{}, nil
}

// Respond is a game answering something the operator asked for.
func (s *Server) Respond(ctx context.Context, req *gamingpb.RespondRequest) (*gamingpb.RespondReply, error) {
	game := callerGame(ctx)
	if state := req.GetState(); state != nil {
		s.reg.setState(game, state)
	}
	if state := req.GetState(); state != nil && s.cfg.OnState != nil {
		go s.cfg.OnState(game)
	}
	if req.GetOk() {
		gameLog.Infof("%s completed request %s", game, req.GetRequestId())
	} else {
		gameLog.Warnf("%s could not complete request %s: %s", game, req.GetRequestId(), req.GetError())
	}
	// An answer nobody is waiting for is normal: a push needs no reply, and a
	// caller that gave up has already stopped listening.
	s.pend.fulfil(req.GetRequestId(), game, req)
	return &gamingpb.RespondReply{}, nil
}

// State is the last thing a game reported, for the console to render.
func (s *Server) State(game string) *gamingpb.GameState { return s.reg.state(game) }

// RequestSpend asks a person to pay, and returns as soon as the request is
// recorded - never when it is paid.
//
// The game is the one this connection's certificate resolved to, so a game
// cannot ask for money as another even by trying. Nothing here decides anything
// about the amount: the caps and the account belong to the policy the operator
// wrote, and this only carries the answer back.
func (s *Server) RequestSpend(ctx context.Context, req *gamingpb.RequestSpendRequest) (*gamingpb.Spend, error) {
	if s.cfg.RequestSpend == nil {
		return nil, errNotHere
	}
	spend, err := s.cfg.RequestSpend(callerGame(ctx), req.GetAddress(), req.GetAmountAtoms(), req.GetReason())
	if err != nil {
		return nil, spendErr(callerGame(ctx), err)
	}
	return spend, nil
}

// SpendStatus reports what became of one of this game's own requests.
func (s *Server) SpendStatus(ctx context.Context, req *gamingpb.SpendStatusRequest) (*gamingpb.Spend, error) {
	if s.cfg.SpendStatus == nil {
		return nil, errNotHere
	}
	spend, err := s.cfg.SpendStatus(callerGame(ctx), req.GetId())
	if err != nil {
		return nil, spendErr(callerGame(ctx), err)
	}
	return spend, nil
}

// Broadcast relays a transaction the game signed itself.
//
// Bounded by shape rather than by intent, which is the only thing that can be
// checked: the bridge cannot know what a game meant, and does know what a
// transaction does.
func (s *Server) Broadcast(ctx context.Context, req *gamingpb.BroadcastRequest) (*gamingpb.BroadcastReply, error) {
	if s.cfg.Broadcast == nil {
		return nil, errNotHere
	}
	txid, err := s.cfg.Broadcast(ctx, callerGame(ctx), req.GetRawTxHex())
	if err != nil {
		return nil, gameErr(callerGame(ctx), "Broadcast", codes.FailedPrecondition, err)
	}
	return &gamingpb.BroadcastReply{Txid: txid}, nil
}

// spendErr says which kind of no this was.
//
// A cap is the operator's standing decision working as intended, and a game
// told about one can wait or ask for less. Anything else is the section not
// being set up to pay at all, which is a thing a person has to go and do - and
// a game told it was over a limit would send them to the wrong screen.
//
// Not found rather than forbidden for another game's request: forbidden would
// confirm the id is real to somebody who has no business knowing.
func spendErr(game string, err error) error {
	switch {
	case errors.Is(err, ErrSpendOverCap):
		return gameErr(game, "RequestSpend", codes.ResourceExhausted, err)
	case errors.Is(err, ErrSpendNotFound):
		return gameErr(game, "RequestSpend", codes.NotFound, err)
	default:
		return gameErr(game, "RequestSpend", codes.FailedPrecondition, err)
	}
}
