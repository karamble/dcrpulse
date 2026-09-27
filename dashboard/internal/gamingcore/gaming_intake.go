// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"encoding/hex"
	"time"

	gamingwire "github.com/karamble/dcrgaming-sdk/pkg/gaming/wire"
)

// GroupMessage is one message a group chat received, as the host's Bison Relay
// client delivered it: the chat, the sender's authenticated user id, the text,
// and when it was sent.
type GroupMessage struct {
	GCID, From [32]byte
	Text       string
	Time       time.Time
}

// ReceiveGroupMessage takes one group message from the host. A gaming frame is
// journaled and then delivered; anything else is ignored. The host
// acknowledges the message to its source only once this returns nil: an error
// means the frame is not kept yet and has to come again.
func ReceiveGroupMessage(m GroupMessage) error {
	return Gaming().receiveGroupMessage(m)
}

func (b *GamingBus) receiveGroupMessage(m GroupMessage) error {
	if !gamingwire.IsEnvelope(m.Text) {
		return nil
	}
	gcid := hex.EncodeToString(m.GCID[:])
	from := hex.EncodeToString(m.From[:])
	if _, err := appendGamingJournal(gcid, from, m.Text, m.Time); err != nil {
		return err
	}
	b.deliverGamingMessage(gcid, from, m.Text)
	return nil
}
