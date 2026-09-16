package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	gw "github.com/karamble/dcrgaming-sdk/pkg/gaming/wire"
)

// authority is a bridge-reserved extension to the async --gaming envelope.
// Game SendFrame calls may never send it, including malformed variants.
func isFinancialFrame(raw string) bool {
	match := gamingFrameRE.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return false
	}
	for _, field := range strings.Split(match[1], ",") {
		key, _, _ := strings.Cut(field, "=")
		if key == "authority" {
			return true
		}
	}
	return false
}

type financialMessage struct {
	RosterHash string
	Key        string
	Settlement string
	Signatures [][]byte
	Want       bool
}

const (
	financialWireVersion  = byte(3)
	financialParticipant  = byte(1)
	financialSettlement   = byte(2)
	financialWant         = byte(4)
	financialRoster       = byte(8)
	financialVersionShift = 4
)

func financialPart(raw string) (*gw.Part, error) {
	if len(raw) > 32768 || !isFinancialFrame(raw) {
		return nil, fmt.Errorf("invalid financial frame")
	}
	match := gamingFrameRE.FindStringSubmatch(strings.TrimSpace(raw))
	seen := map[string]bool{}
	for _, field := range strings.Split(match[1], ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok || seen[key] {
			return nil, fmt.Errorf("ambiguous financial envelope")
		}
		seen[key] = true
		if key == "authority" && value != "3" {
			return nil, fmt.Errorf("unsupported financial wire version")
		}
	}
	part, ok := gw.Parse(raw)
	if !ok || part.Total != 1 || part.Seq != 1 || part.Expired(time.Now()) {
		return nil, fmt.Errorf("invalid financial envelope")
	}
	return part, nil
}

func localGamingUID(ctx context.Context) (string, error) {
	raw, err := rpc.BrclientdUserPublicIdentity(ctx)
	if err != nil {
		return "", err
	}
	var public struct {
		Identity []byte `json:"identity"`
	}
	if err = json.Unmarshal(raw, &public); err != nil || len(public.Identity) != 32 {
		return "", fmt.Errorf("BR identity unavailable")
	}
	return hex.EncodeToString(public.Identity), nil
}

func sendFinancialMessage(ctx context.Context, game, group, table string, msg financialMessage) error {
	id, err := parseGamingGCID(group)
	if err != nil {
		return err
	}
	raw, err := encodeFinancialMessage(msg)
	if err != nil {
		return err
	}
	parts, err := gw.Encode(game, 2, table, raw, time.Now().Add(10*time.Minute), 24000)
	if err != nil || len(parts) != 1 {
		return fmt.Errorf("financial message exceeds single-message limit")
	}
	frame := strings.Replace(parts[0], "--gaming[", "--gaming[authority=3,", 1)
	return rpc.BrclientdGCMessage(ctx, id, frame, 0)
}

func announceGamingAuthority(ctx context.Context, scope gamingfunds.Scope, table string, want bool) error {
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	accepted, err := store.AuthorizedTable(scope, table)
	if err != nil {
		return err
	}
	key, err := store.WalletKey(scope, table)
	if err != nil {
		return err
	}
	uid, err := localGamingUID(ctx)
	if err != nil {
		return err
	}
	peer := gamingfunds.Participant{UID: uid, Key: key.Public, Destination: key.Address, TermsHash: accepted.TermsHash()}
	participants, err := store.Participants(scope, table)
	if err != nil {
		return err
	}
	known := false
	for _, participant := range participants {
		if participant.UID == uid {
			if participant != peer {
				return fmt.Errorf("local financial authority changed")
			}
			known = true
			break
		}
	}
	if want {
		if known {
			return nil
		}
		// BR group-chat history is durable. Send the state once, then record
		// it locally so repeated game requests do not rebroadcast it.
		if err = sendFinancialMessage(ctx, scope.Game, accepted.Group, table, financialMessage{Key: peer.Key, Want: true}); err != nil {
			return err
		}
		return store.RecordParticipant(scope, table, accepted.Group, uid, peer)
	}
	if err = store.RecordParticipant(scope, table, accepted.Group, uid, peer); err != nil {
		return err
	}
	hash, err := store.RosterHash(scope, table)
	if err != nil {
		return err
	}
	if hash != "" {
		if _, err = store.CommitRoster(scope, table, uid, hash); err != nil {
			return err
		}
	}
	return sendFinancialMessage(ctx, scope.Game, accepted.Group, table, financialMessage{Key: peer.Key, RosterHash: hash})
}

// receiveFinancialFrame runs only on the authenticated BR inbound path.
func receiveFinancialFrame(ctx context.Context, event GamingFrameEvent) error {
	part, err := financialPart(event.Frame)
	if err != nil {
		return err
	}
	if part.Game != event.Game {
		return fmt.Errorf("financial game mismatch")
	}
	msg, err := decodeFinancialMessage(part.Chunk)
	if err != nil {
		return err
	}
	scope, err := gamingFinancialScope(ctx, event.Game)
	if err != nil {
		return err
	}
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	accepted, err := store.AuthorizedTable(scope, part.SID)
	if err != nil {
		return err
	}
	if accepted.Group != event.GCID {
		return fmt.Errorf("financial message arrived in wrong group")
	}
	params, err := chainParams(ctx)
	if err != nil {
		return err
	}
	if msg.Key != "" {
		if msg.Settlement != "" || len(msg.Signatures) > 0 {
			return fmt.Errorf("ambiguous financial message")
		}
		key, err := hex.DecodeString(msg.Key)
		if err != nil {
			return err
		}
		// UID comes from authenticated BR delivery. Payout destination and
		// terms are deterministic local facts, so neither belongs on the wire.
		expected, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(stdaddr.Hash160(key), params)
		if err != nil {
			return err
		}
		participant := gamingfunds.Participant{UID: event.From, Key: msg.Key, Destination: expected.String(), TermsHash: accepted.TermsHash()}
		participants, err := store.Participants(scope, part.SID)
		if err != nil {
			return err
		}
		isNew := true
		for _, known := range participants {
			if known.UID == event.From {
				isNew = false
				break
			}
		}
		if err = store.RecordParticipant(scope, part.SID, event.GCID, event.From, participant); err != nil {
			return err
		}
		if msg.RosterHash != "" {
			if _, err = store.CommitRoster(scope, part.SID, event.From, msg.RosterHash); err != nil {
				return err
			}
		}
		changed := false
		hash, err := store.RosterHash(scope, part.SID)
		if err != nil {
			return err
		}
		if hash != "" {
			uid, err := localGamingUID(ctx)
			if err != nil {
				return err
			}
			changed, err = store.CommitRoster(scope, part.SID, uid, hash)
			if err != nil {
				return err
			}
		}
		if (msg.Want && isNew) || changed {
			return announceGamingAuthority(ctx, scope, part.SID, false)
		}
		return nil
	}
	if msg.Settlement == "" || len(msg.Signatures) == 0 {
		return fmt.Errorf("empty financial message")
	}
	payout, err := store.Settlement(scope, msg.Settlement)
	if err != nil {
		return err
	}
	if payout.Table != part.SID {
		return fmt.Errorf("payout table mismatch")
	}
	_, err = store.AddSettlementSignatures(scope, msg.Settlement, event.From, msg.Signatures, params)
	return err
}

func decodeFinancialMessage(raw []byte) (financialMessage, error) {
	var msg financialMessage
	if len(raw) < 1 || raw[0]>>financialVersionShift != financialWireVersion {
		return msg, fmt.Errorf("invalid financial message")
	}
	flags := raw[0] & 0x0f
	payload := raw[1:]
	switch flags & (financialParticipant | financialSettlement) {
	case financialParticipant:
		if flags & ^(financialParticipant|financialWant|financialRoster) != 0 {
			return msg, fmt.Errorf("invalid participant flags")
		}
		want := 33
		if flags&financialRoster != 0 {
			want += 32
		}
		if len(payload) != want {
			return msg, fmt.Errorf("invalid participant message")
		}
		msg.Key = hex.EncodeToString(payload[:33])
		msg.Want = flags&financialWant != 0
		if flags&financialRoster != 0 {
			msg.RosterHash = hex.EncodeToString(payload[33:])
		}
		return msg, nil
	case financialSettlement:
		if flags != financialSettlement || len(payload) < 33 {
			return msg, fmt.Errorf("invalid settlement message")
		}
		msg.Settlement = hex.EncodeToString(payload[:32])
		count := int(payload[32])
		payload = payload[33:]
		if count == 0 {
			return financialMessage{}, fmt.Errorf("empty settlement signatures")
		}
		for range count {
			if len(payload) < 1 || int(payload[0]) > len(payload)-1 || payload[0] == 0 {
				return financialMessage{}, fmt.Errorf("invalid settlement signature")
			}
			n := int(payload[0])
			msg.Signatures = append(msg.Signatures, append([]byte(nil), payload[1:1+n]...))
			payload = payload[1+n:]
		}
		if len(payload) != 0 {
			return financialMessage{}, fmt.Errorf("trailing financial message data")
		}
		return msg, nil
	default:
		return msg, fmt.Errorf("ambiguous financial message")
	}
}

func encodeFinancialMessage(msg financialMessage) ([]byte, error) {
	if msg.Key != "" {
		if msg.Settlement != "" || len(msg.Signatures) > 0 {
			return nil, fmt.Errorf("ambiguous financial message")
		}
		key, err := hex.DecodeString(msg.Key)
		if err != nil || len(key) != 33 {
			return nil, fmt.Errorf("invalid financial key")
		}
		flags := financialParticipant
		if msg.Want {
			flags |= financialWant
		}
		out := append([]byte{financialWireVersion<<financialVersionShift | flags}, key...)
		if msg.RosterHash != "" {
			hash, err := hex.DecodeString(msg.RosterHash)
			if err != nil || len(hash) != 32 {
				return nil, fmt.Errorf("invalid financial roster hash")
			}
			out[0] |= financialRoster
			out = append(out, hash...)
		}
		return out, nil
	}
	if msg.Settlement == "" || len(msg.Signatures) == 0 || len(msg.Signatures) > 255 || msg.Want || msg.RosterHash != "" {
		return nil, fmt.Errorf("invalid settlement message")
	}
	id, err := hex.DecodeString(msg.Settlement)
	if err != nil || len(id) != 32 {
		return nil, fmt.Errorf("invalid settlement id")
	}
	out := append([]byte{financialWireVersion<<financialVersionShift | financialSettlement}, id...)
	out = append(out, byte(len(msg.Signatures)))
	for _, sig := range msg.Signatures {
		if len(sig) == 0 || len(sig) > 255 {
			return nil, fmt.Errorf("invalid settlement signature")
		}
		out = append(out, byte(len(sig)))
		out = append(out, sig...)
	}
	return out, nil
}
