// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"dcrpulse/internal/gamingpb"
)

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
	start := s.reg.streamStart(game, req, missed)
	if err := stream.Send(&gamingpb.BridgeEvent{
		Event: &gamingpb.BridgeEvent_Start{Start: start},
	}); err != nil {
		return err
	}
	if start.GetGap() {
		gameLog.Infof("%s reconnected with a gap (%s), %d table(s) named",
			game, start.GetGapScope(), len(start.GetGapGcids()))
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
