package services

import (
	"encoding/base64"
	"regexp"
	"strings"
)

// The bridge reads just enough of a game's wire envelope to route it.
//
//	--gaming[v=1,game=poker,gv=1,sid=<hex>,mid=<hex>,seq=<n>/<total>,exp=<unix>]--<base64>
//
// It reads the routing key and nothing else. What a frame means is the game's
// business, and the bridge is a tunnel: it decides which installed game a frame
// belongs to, hands it over whole, and forms no opinion about the contents.
// Games sign their own traffic and check each other's, so a host that inspected
// payloads would be adding a party nobody agreed to trust.
//
// This is deliberately not a copy of a game's protocol library. brclientd
// recognises frames to keep them out of chat, a game implements the protocol,
// and this routes them - three different jobs that happen to share one prefix.
var gamingFrameRE = regexp.MustCompile(`^--gaming\[([^\]]*)\]--([A-Za-z0-9+/=\s]*)$`)

// gamingFrame is a frame the bridge has decided it can route.
type gamingFrame struct {
	Game string
	Text string
}

// parseGamingFrame reads a group chat message as a game frame.
//
// Shape alone is not enough to claim one. The payload class admits letters and
// whitespace, so a chat message that opens with something frame-shaped and
// continues in prose matches the pattern; requiring the payload to base64-decode
// separates the two. The version and the game are not validated here - a game
// this host has never heard of must still be recognisable as protocol traffic,
// so that it can be dropped as unroutable rather than mistaken for chat.
func parseGamingFrame(text string) (gamingFrame, bool) {
	m := gamingFrameRE.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return gamingFrame{}, false
	}
	payload := strings.Join(strings.Fields(m[2]), "")
	if payload == "" {
		return gamingFrame{}, false
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		return gamingFrame{}, false
	}

	for _, tok := range strings.Split(m[1], ",") {
		k, v, ok := strings.Cut(tok, "=")
		if ok && k == "game" && v != "" {
			return gamingFrame{Game: v, Text: strings.TrimSpace(text)}, true
		}
	}
	return gamingFrame{}, false
}
