package gamingbridge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAPanickingCallFailsWithoutTakingTheProcessDown(t *testing.T) {
	s := &Server{}
	_, err := s.recoverUnary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/t/Unary"},
		func(context.Context, any) (any, error) { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("unary panic returned %v", err)
	}
	got, err := s.recoverUnary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/t/Unary"},
		func(context.Context, any) (any, error) { return "reply", nil })
	if err != nil || got != "reply" {
		t.Fatalf("a calm call came back as %v, %v", got, err)
	}
	err = s.recoverStream(nil, nil, &grpc.StreamServerInfo{FullMethod: "/t/Stream"},
		func(any, grpc.ServerStream) error { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Fatalf("stream panic returned %v", err)
	}
	if err = s.recoverStream(nil, nil, &grpc.StreamServerInfo{FullMethod: "/t/Stream"},
		func(any, grpc.ServerStream) error { return nil }); err != nil {
		t.Fatalf("a calm stream came back as %v", err)
	}
}

func TestARetryableErrorReachesTheGameAsUnavailable(t *testing.T) {
	err := gameErr("poker", "SendFrame", codes.InvalidArgument, fmt.Errorf("wrapped: %w", GameRetry(errors.New("publication is uncertain"))))
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != "publication is uncertain" {
		t.Fatalf("retryable error rendered as %v", err)
	}
	err = gameErr("poker", "SendFrame", codes.InvalidArgument, GameSafe(errors.New("identity collision")))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("game-safe error rendered as %v", err)
	}
}
