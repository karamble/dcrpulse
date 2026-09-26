package services

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/crypto/blake256"
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
var (
	gamingFrameRE   = regexp.MustCompile(`^--gaming\[([^\]]*)\]--([A-Za-z0-9+/=\s]*)$`)
	gamingGameRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	gamingSessionRE = regexp.MustCompile(`^[0-9a-f]{1,32}$`)
	gamingMessageRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// gamingFrame is a frame the bridge has decided it can route.
type gamingFrame struct {
	Version     string
	Game        string
	GameVersion int
	SID         string
	MID         string
	Part        string
	Seq         int
	Total       int
	Expiry      int64
	Payload     []byte
	Text        string
}

// parseGamingFrame reads a group chat message as a game frame.
//
// Shape alone is not enough to claim one. The payload class admits letters and
// whitespace, so a chat message that opens with something frame-shaped and
// continues in prose matches the pattern; requiring the payload to base64-decode
// separates the two. Wire version 2 and every bounded routing field are checked
// here. The game identifier need not be registered: an unknown game's valid
// protocol traffic is still consumed rather than mistaken for chat.
func parseGamingFrame(text string) (gamingFrame, bool) {
	if len(text) > 1<<20 {
		return gamingFrame{}, false
	}
	m := gamingFrameRE.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return gamingFrame{}, false
	}
	payload := strings.Join(strings.Fields(m[2]), "")
	if payload == "" {
		return gamingFrame{}, false
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 {
		return gamingFrame{}, false
	}

	seen := map[string]bool{}
	frame := gamingFrame{Text: strings.TrimSpace(text)}
	gameVersion, session, expiry := "", "", ""
	for _, tok := range strings.Split(m[1], ",") {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || k == "" || seen[k] || strings.TrimSpace(k) != k {
			return gamingFrame{}, false
		}
		seen[k] = true
		switch k {
		case "v":
			frame.Version = v
		case "game":
			frame.Game = v
		case "gv":
			gameVersion = v
		case "sid":
			session = v
		case "mid":
			frame.MID = v
		case "seq":
			frame.Part = v
		case "exp":
			expiry = v
		}
	}
	if frame.Version != "2" || !gamingGameRE.MatchString(frame.Game) ||
		!gamingSessionRE.MatchString(session) || !gamingMessageRE.MatchString(frame.MID) {
		return gamingFrame{}, false
	}
	gv, err := strconv.Atoi(gameVersion)
	if err != nil || gv <= 0 {
		return gamingFrame{}, false
	}
	part, total, ok := strings.Cut(frame.Part, "/")
	if !ok {
		return gamingFrame{}, false
	}
	partN, partErr := strconv.Atoi(part)
	totalN, totalErr := strconv.Atoi(total)
	if partErr != nil || totalErr != nil || partN < 1 || totalN < 1 || partN > totalN || totalN > 64 {
		return gamingFrame{}, false
	}
	exp, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || exp < 0 {
		return gamingFrame{}, false
	}
	frame.GameVersion = gv
	frame.SID = session
	frame.Seq = partN
	frame.Total = totalN
	frame.Expiry = exp
	frame.Payload = raw
	return frame, true
}

// buildGamingFrame is dcrpulse's independent v2 encoder for bridge-authored
// authority state. It intentionally matches the SDK's pinned canonical MID.
func buildGamingFrame(game string, gameVersion int, sid string, payload []byte, expiry time.Time) (string, error) {
	if !gamingGameRE.MatchString(game) || !gamingSessionRE.MatchString(sid) || gameVersion <= 0 || len(payload) == 0 {
		return "", fmt.Errorf("invalid gaming frame fields")
	}
	mid := gamingMessageID(game, gameVersion, sid, payload)
	var exp int64
	if !expiry.IsZero() {
		exp = expiry.Unix()
	}
	return canonicalGamingFrame(game, gameVersion, sid, mid, exp, payload), nil
}

// canonicalGamingFrame is the one-part v2 envelope exactly as the SDK's
// wire.Encode writes it: fixed key order, no other keys, standard base64.
func canonicalGamingFrame(game string, gameVersion int, sid, mid string, exp int64, payload []byte) string {
	return fmt.Sprintf("--gaming[v=2,game=%s,gv=%d,sid=%s,mid=%s,seq=1/1,exp=%d]--%s",
		game, gameVersion, sid, mid, exp, base64.StdEncoding.EncodeToString(payload))
}

func gamingMessageID(game string, gameVersion int, sid string, payload []byte) string {
	var pre bytes.Buffer
	pre.WriteString("dcrgaming/event/v2\x00")
	writeBytes := func(b []byte) {
		_ = binary.Write(&pre, binary.BigEndian, uint32(len(b)))
		_, _ = pre.Write(b)
	}
	writeBytes([]byte(game))
	_ = binary.Write(&pre, binary.BigEndian, uint32(gameVersion))
	writeBytes([]byte(sid))
	writeBytes(payload)
	mid := blake256.Sum256(pre.Bytes())
	return fmt.Sprintf("%x", mid)
}
