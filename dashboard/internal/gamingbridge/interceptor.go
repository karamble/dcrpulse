// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// callerKey carries the resolved game down to the handlers. Unexported, so
// nothing outside this package can put a game into a context.
type callerKey struct{}

// callerGame is the game this call authenticated as.
func callerGame(ctx context.Context) string {
	game, _ := ctx.Value(callerKey{}).(string)
	return game
}

// errNotHere is what an unresolved caller is told.
//
// Unimplemented, and empty. A bridge that is switched off, a credential that
// was revoked and a build with no bridge in it all answer the same way, because
// the alternative is describing this host to a stranger who has just been
// refused. It is the same posture the browser tunnel took with 404.
var errNotHere = status.Error(codes.Unimplemented, "")

// resolveCaller names the game behind a connection, from its certificate.
//
// Read from the verified chain rather than from the certificates the peer
// merely presented. They are the same bytes while the listener demands a
// verified client certificate, and reading the verified one means that if that
// ever loosened, this would stop resolving rather than quietly start trusting
// whatever arrived.
func (s *Server) resolveCaller(ctx context.Context) (string, bool) {
	if !s.live() {
		return "", false
	}
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", false
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", false
	}
	if len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 {
		return "", false
	}
	return s.allow.Resolve(fingerprintOf(tlsInfo.State.VerifiedChains[0][0]))
}

// unaryIdentity resolves the caller before any handler runs.
func (s *Server) unaryIdentity(
	ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	game, ok := s.resolveCaller(ctx)
	if !ok {
		return nil, errNotHere
	}
	return handler(context.WithValue(ctx, callerKey{}, game), req)
}

// streamIdentity does the same for the one stream.
//
// The resolution is repeated per call rather than settled once when the
// connection opened, so a credential withdrawn mid-session stops working on the
// next thing it tries rather than at the next reconnect.
func (s *Server) streamIdentity(
	srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	game, ok := s.resolveCaller(ss.Context())
	if !ok {
		return errNotHere
	}
	return handler(srv, &identifiedStream{ServerStream: ss, ctx: context.WithValue(ss.Context(), callerKey{}, game)})
}

// identifiedStream carries the resolved game, because a ServerStream's context
// cannot be replaced.
type identifiedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *identifiedStream) Context() context.Context { return s.ctx }
