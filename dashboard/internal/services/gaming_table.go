// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"

	"github.com/decred/dcrd/dcrutil/v4"

	"dcrpulse/internal/rpc"
)

// A game has no host: an invitation is terms plus a session id, and a table
// exists once enough peers join under the same ones. Creating one is composing
// that link and putting it in a group chat.

const (
	// gamingInviteScheme and gamingInviteKind must stay the shape a game's
	// own parser accepts.
	gamingInviteScheme = "gaming"
	gamingInviteKind   = "table"

	// gamingOpenBlocks is the default for how long registration stays open,
	// in blocks. A height rather than a time because every peer has to read
	// the same deadline and clocks disagree.
	//
	// One block, because seating cannot begin until a block past the close
	// and every block of waiting is paid by a table that has already agreed.
	// A peer who does not accept within the block misses the table, so a
	// caller expecting anybody it has not already spoken to should ask for
	// more.
	gamingOpenBlocks = 1

	// gamingMaxOpenBlocks bounds it. A day is already far longer than an
	// invitation nobody has taken up is worth keeping.
	gamingMaxOpenBlocks = 288

	// gamingRefundBlocks is the relative timelock on every seat's refund
	// branch. It has to outlast a hand by enough that nobody can pull their
	// stake mid-play.
	gamingRefundBlocks = 288

	gamingMinSeats = 2
	gamingMaxSeats = 6
)

// GamingTable is a table this host has just proposed.
type GamingTable struct {
	SID    string `json:"sid"`
	Invite string `json:"invite"`
	Until  uint32 `json:"until"`
	Height int64  `json:"height"`
	GCID   string `json:"gcid"`
}

// gamingSessionID mints the id that identifies one table. It becomes the
// routing key on the wire, which is 1 to 32 lowercase hex.
func gamingSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// gamingInviteLink renders the link that goes in a chat message.
func gamingInviteLink(game, sid string, buyinAtoms uint64, seats, csvBlocks, until uint32) string {
	q := url.Values{}
	q.Set("buyin", strconv.FormatUint(buyinAtoms, 10))
	q.Set("seats", strconv.FormatUint(uint64(seats), 10))
	q.Set("sid", sid)
	q.Set("csv", strconv.FormatUint(uint64(csvBlocks), 10))
	q.Set("until", strconv.FormatUint(uint64(until), 10))
	return fmt.Sprintf("%s://%s/%s?%s", gamingInviteScheme, game, gamingInviteKind, q.Encode())
}

// CreateGamingTable proposes a table and puts it in a group chat.
//
// The seat is taken before the invitation is sent. A join that fails leaves an
// invitation nobody is at; a send that fails leaves a table only this player
// knows about, which nobody can join and which expires on its own.
func CreateGamingTable(ctx context.Context, game, gcid string, buyinAtoms uint64, seats, openBlocks uint32) (GamingTable, error) {
	if openBlocks == 0 {
		openBlocks = gamingOpenBlocks
	}
	if openBlocks > gamingMaxOpenBlocks {
		return GamingTable{}, fmt.Errorf("registration can stay open for at most %d blocks, not %d",
			gamingMaxOpenBlocks, openBlocks)
	}
	if seats < gamingMinSeats || seats > gamingMaxSeats {
		return GamingTable{}, fmt.Errorf("a table holds %d to %d seats, not %d",
			gamingMinSeats, gamingMaxSeats, seats)
	}
	if buyinAtoms == 0 {
		return GamingTable{}, fmt.Errorf("a table needs a buy-in")
	}
	p, registered := ReadGamingSettings().Policies[game]
	if !registered {
		return GamingTable{}, ErrGamingGameNotRegistered
	}
	if p.PerTableCapAtoms > 0 && int64(buyinAtoms) > p.PerTableCapAtoms {
		return GamingTable{}, fmt.Errorf("a buy-in of %d atoms is over %q's per-table limit of %d",
			buyinAtoms, game, p.PerTableCapAtoms)
	}

	tip, err := GamingChainTipNow(ctx)
	if err != nil {
		// Without a height there is no deadline every peer can check, and
		// a table cannot be formed by guessing.
		return GamingTable{}, fmt.Errorf("cannot read the chain, so a deadline cannot be set: %w", err)
	}

	sid, err := gamingSessionID()
	if err != nil {
		return GamingTable{}, err
	}
	until := uint32(tip.Height) + openBlocks
	invite := gamingInviteLink(game, sid, buyinAtoms, seats, gamingRefundBlocks, until)

	// Prose and the link, not a bare URL: a client that knows nothing about
	// games must still show a person something they can act on.
	msg := fmt.Sprintf("Table for %d at %s DCR a seat. Registration closes at block %d.\n%s",
		seats, gamingAtomsText(buyinAtoms), until, invite)
	if err := rpc.BrclientdGCMessage(ctx, gcid, msg, 0); err != nil {
		return GamingTable{}, fmt.Errorf("the invitation could not be sent: %w", err)
	}
	return GamingTable{SID: sid, Invite: invite, Until: until, Height: tip.Height, GCID: gcid}, nil
}

// gamingAtomsText renders atoms for a chat message, trailing zeros trimmed.
func gamingAtomsText(atoms uint64) string {
	s := strconv.FormatFloat(dcrutil.Amount(atoms).ToCoin(), 'f', -1, 64)
	return s
}
