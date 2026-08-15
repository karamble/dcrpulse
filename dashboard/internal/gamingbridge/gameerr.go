// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingbridge

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gameSafeError marks an error whose text was written to be read by a game.
type gameSafeError struct{ err error }

func (e gameSafeError) Error() string { return e.err.Error() }
func (e gameSafeError) Unwrap() error { return e.err }

// GameSafe marks err as one a game may be shown verbatim. Errors reaching a
// game are opaque by default; a daemon writes for the operator, and a game is
// not the operator.
func GameSafe(err error) error {
	if err == nil {
		return nil
	}
	return gameSafeError{err: err}
}

// gameErr renders err for a game at the given code, or, when it was not
// written for one, logs it in full and returns a fixed message.
//
// Unavailable rather than a caller error for the opaque case: the game cannot
// act on it, and a code that says otherwise would stop it retrying something
// that may well succeed later.
func gameErr(game, method string, code codes.Code, err error) error {
	var safe gameSafeError
	if errors.As(err, &safe) {
		// The marked error's own text, not the outer wrapper's, which may have
		// been added by a caller that never vetted it.
		return status.Error(code, safe.Error())
	}
	gameLog.Warnf("%s: %s: %v", game, method, err)
	return status.Error(codes.Unavailable, "the bridge could not complete that")
}
