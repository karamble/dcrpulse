// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingpb

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"testing"

	"google.golang.org/protobuf/proto"
)

// The contract is a published wire format, and the game on the other side is a
// separate program built from a separate copy of this file.
//
// Nothing but agreement between the two makes a frame arrive, so changing it
// has to be a deliberate act with a visible diff rather than something that
// happens on the way past. Updating this hash is how that intent is stated.
//
// If this fails and the change was meant: regenerate (`make proto`), check the
// method set below still says what you want, and paste the new hash in.
const contractSHA256 = "b6c13c6b2dba35628f9206d4ac57f2cc026fb528deb8c7ce38b6d81c984bda70"

func TestTheWireContractHasNotDrifted(t *testing.T) {
	raw, err := os.ReadFile("gaming_bridge.proto")
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != contractSHA256 {
		t.Fatalf("the wire contract changed:\n  now %s\n  was %s\n"+
			"a game built from the old one will not agree with this bridge; "+
			"update the hash only if that is what you meant",
			hex.EncodeToString(sum[:]), contractSHA256)
	}
}

// The generated stubs have to match the contract they claim to come from.
//
// The hash above guards the source; this guards the output, which is what
// actually gets compiled. Together they catch stubs regenerated from a
// different file, or committed without regenerating at all.
func TestTheServiceOffersExactlyTheseCalls(t *testing.T) {
	want := []string{
		// handshake
		"Hello",
		// the bridge->game channel and its reply half
		"Respond", "ReportState",
		// money - the only verbs where the bridge forms an opinion
		"RequestSpend", "SpendStatus", "Broadcast",
		// frames and chain
		"SendFrame", "ChainTip", "BlockHash", "Outpoint",
	}
	got := make([]string, 0, len(BridgeService_ServiceDesc.Methods))
	for _, m := range BridgeService_ServiceDesc.Methods {
		got = append(got, m.MethodName)
	}
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("the service offers %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the service offers %v, want %v", got, want)
		}
	}

	// Subscribe is the one stream, and it is server-side only: the bridge
	// pushes down the connection the game opened, and never dials a game.
	if len(BridgeService_ServiceDesc.Streams) != 1 {
		t.Fatalf("the service has %d streams, want exactly Subscribe",
			len(BridgeService_ServiceDesc.Streams))
	}
	s := BridgeService_ServiceDesc.Streams[0]
	if s.StreamName != "Subscribe" {
		t.Fatalf("the stream is %q, want Subscribe", s.StreamName)
	}
	if s.ClientStreams {
		t.Error("Subscribe accepts a client stream; the game's calls are unary")
	}
	if !s.ServerStreams {
		t.Error("Subscribe does not stream from the bridge, so nothing can be pushed")
	}
}

// No message carries the caller's own name.
//
// A game presents a credential and the bridge decides which game it is. A field
// it could fill in would be a second answer to that question, and the two could
// disagree.
func TestNoRequestNamesItsOwnGame(t *testing.T) {
	for _, m := range []proto.Message{
		&RequestSpendRequest{},
		&SendFrameRequest{},
		&BroadcastRequest{},
		&SpendStatusRequest{},
		&SubscribeRequest{},
	} {
		d := m.ProtoReflect().Descriptor()
		for i := 0; i < d.Fields().Len(); i++ {
			switch name := string(d.Fields().Get(i).Name()); name {
			case "game", "game_id":
				t.Errorf("%s carries %q, so a game could name itself rather than "+
					"being told what it is", d.Name(), name)
			}
		}
	}
}
