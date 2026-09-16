package services

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	RosterHash  string                   `json:"rosterHash,omitempty"`
	Version     uint32                   `json:"version"`
	Participant *gamingfunds.Participant `json:"participant,omitempty"`
	Settlement  string                   `json:"settlement,omitempty"`
	Signatures  [][]byte                 `json:"signatures,omitempty"`
	Want        bool                     `json:"want,omitempty"`
}

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
		if key == "authority" && value != "2" {
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
	msg.Version = 2
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	parts, err := gw.Encode(game, 2, table, raw, time.Now().Add(10*time.Minute), 24000)
	if err != nil || len(parts) != 1 {
		return fmt.Errorf("financial message exceeds single-message limit")
	}
	frame := strings.Replace(parts[0], "--gaming[", "--gaming[authority=2,", 1)
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
	return sendFinancialMessage(ctx, scope.Game, accepted.Group, table, financialMessage{Participant: &peer, Want: want, RosterHash: hash})
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
	if msg.Participant != nil {
		if msg.Settlement != "" || len(msg.Signatures) > 0 {
			return fmt.Errorf("ambiguous financial message")
		}
		addr, err := stdaddr.DecodeAddress(msg.Participant.Destination, params)
		if err != nil {
			return err
		}
		// Payout destination is the announced wallet key's P2PKH address.
		key, err := hex.DecodeString(msg.Participant.Key)
		if err != nil {
			return err
		}
		expected, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(stdaddr.Hash160(key), params)
		if err != nil || expected.String() != addr.String() {
			return fmt.Errorf("financial destination is not bound to participant key")
		}
		if err = store.RecordParticipant(scope, part.SID, event.GCID, event.From, *msg.Participant); err != nil {
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
		if msg.Want || changed {
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&msg); err != nil || msg.Version != 2 {
		return msg, fmt.Errorf("invalid financial message")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return msg, fmt.Errorf("trailing financial message data")
	}
	return msg, nil
}
