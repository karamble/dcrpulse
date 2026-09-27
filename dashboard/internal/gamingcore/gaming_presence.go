// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

// GamingPresenceChanged tells the operator a game connected or went away.
func (br *Bridge) GamingPresenceChanged(game string) {
	br.hostOperator().PresenceChanged(game)
}
