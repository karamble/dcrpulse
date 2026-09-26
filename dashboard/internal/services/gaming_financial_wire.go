package services

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
	"dcrpulse/internal/utils"

	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
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
	// Proof is the key's signature over keyProofHash, binding the key to the
	// announcer's own Bison Relay identity and this table.
	Proof      []byte
	Settlement string
	Signatures [][]byte
}

const (
	financialWireVersion  = byte(4)
	financialParticipant  = byte(1)
	financialSettlement   = byte(2)
	financialRoster       = byte(8)
	financialVersionShift = 4
)

func financialPart(raw string) (*gamingFrame, error) {
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
	part, ok := parseGamingFrame(raw)
	if !ok || part.Total != 1 || part.Seq != 1 || (part.Expiry > 0 && time.Now().Unix() > part.Expiry) {
		return nil, fmt.Errorf("invalid financial envelope")
	}
	return &part, nil
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

// financialFrame is the exact envelope a financial message is sent as; the
// same message always yields the same bytes.
func financialFrame(game, table string, msg financialMessage) (gamingFrame, string, error) {
	raw, err := encodeFinancialMessage(msg)
	if err != nil {
		return gamingFrame{}, "", err
	}
	encoded, err := buildGamingFrame(game, 2, table, raw, time.Time{})
	if err != nil || len(encoded) > 32768 {
		return gamingFrame{}, "", fmt.Errorf("financial message exceeds single-message limit")
	}
	frame := strings.Replace(encoded, "--gaming[", "--gaming[authority=3,", 1)
	parsed, ok := parseGamingFrame(frame)
	if !ok {
		return gamingFrame{}, "", fmt.Errorf("financial message produced an invalid gaming envelope")
	}
	return parsed, frame, nil
}

func sendFinancialMessage(ctx context.Context, game, group, table string, msg financialMessage) error {
	parsed, frame, err := financialFrame(game, table, msg)
	if err != nil {
		return err
	}
	return sendGamingFrameOnce(ctx, game, group, parsed, frame)
}

func announceGamingAuthority(ctx context.Context, scope gamingfunds.Scope, table string) error {
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
	uid, err := gamingSelfUID(ctx)
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
	if !known {
		err = store.RecordParticipant(scope, table, accepted.Group, uid, peer)
	}
	if err != nil {
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
	proof, err := store.KeyProof(scope, table)
	if err != nil {
		return err
	}
	if proof == "" {
		// The key is proven when the seat bond is approved; it is announced
		// from then on.
		return nil
	}
	raw, err := hex.DecodeString(proof)
	if err != nil {
		return err
	}
	return sendFinancialMessage(ctx, scope.Game, accepted.Group, table, financialMessage{Key: peer.Key, RosterHash: hash, Proof: raw})
}

// keyProofHash is what a table's financial key signs to announce itself for
// its owner's own Bison Relay identity at one table.
func keyProofHash(uid, game, sid, gcid, termsHash string, key []byte) ([32]byte, error) {
	id, err := hex.DecodeString(uid)
	if err != nil || len(id) != 32 || len(key) != 33 {
		return [32]byte{}, fmt.Errorf("invalid key proof inputs")
	}
	var b bytes.Buffer
	b.WriteString("dcrpulse/gaming/financial-key/v1\x00")
	b.Write(id)
	for _, s := range []string{game, sid, gcid, termsHash} {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(s)))
		b.WriteString(s)
	}
	b.Write(key)
	return blake256.Sum256(b.Bytes()), nil
}

// verifyKeyProof checks that key signed the proof for uid at this table.
func verifyKeyProof(proof []byte, uid, game, sid, gcid, termsHash string, key []byte) error {
	hash, err := keyProofHash(uid, game, sid, gcid, termsHash, key)
	if err != nil {
		return err
	}
	pub, err := secp256k1.ParsePubKey(key)
	if err != nil {
		return err
	}
	sig, err := ecdsa.ParseDERSignature(proof)
	if err != nil {
		return fmt.Errorf("invalid key proof: %w", err)
	}
	if !sig.Verify(hash[:], pub) {
		return fmt.Errorf("key proof does not verify")
	}
	return nil
}

// gamingKeyProofSign proves this bridge's key for a table. Settable for tests.
var gamingKeyProofSign = signGamingKeyProof

// signGamingKeyProof has the wallet sign this table's key proof, with the
// passphrase of the seat-bond approval, and records it. Once per table.
func signGamingKeyProof(ctx context.Context, scope gamingfunds.Scope, table string, passphrase []byte) error {
	defer utils.Zero(passphrase)
	store, err := gamingFundsStore()
	if err != nil {
		return err
	}
	if proof, err := store.KeyProof(scope, table); err != nil || proof != "" {
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
	uid, err := gamingSelfUID(ctx)
	if err != nil {
		return err
	}
	pub, err := hex.DecodeString(key.Public)
	if err != nil {
		return err
	}
	hash, err := keyProofHash(uid, scope.Game, table, accepted.Group, accepted.TermsHash(), pub)
	if err != nil {
		return err
	}
	sig, err := withGamingWalletSigner(ctx, scope, passphrase, func(sign gamingfunds.WalletSigner) ([]byte, error) {
		return sign(key, hash[:])
	})
	if err != nil {
		return err
	}
	if err := verifyKeyProof(sig, uid, scope.Game, table, accepted.Group, accepted.TermsHash(), pub); err != nil {
		return err
	}
	return store.SaveKeyProof(scope, table, hex.EncodeToString(sig))
}

// The wallet scope and chain a received frame is judged against. Settable
// for tests; production never sets them.
var (
	receiveScope  = gamingFinancialScope
	receiveParams = chainParams
)

// receiveFinancialFrame runs only on the authenticated BR inbound path.
func receiveFinancialFrame(ctx context.Context, event GamingFrameEvent) error {
	part, err := financialPart(event.Frame)
	if err != nil {
		return err
	}
	if part.Game != event.Game {
		return fmt.Errorf("financial game mismatch")
	}
	msg, err := decodeFinancialMessage(part.Payload)
	if err != nil {
		return err
	}
	scope, err := receiveScope(ctx, event.Game)
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
	params, err := receiveParams(ctx)
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
		// The key must have signed for exactly this sender, table and group,
		// so nobody can announce another player's key as their own.
		if err := verifyKeyProof(msg.Proof, event.From, event.Game, part.SID, event.GCID, accepted.TermsHash(), key); err != nil {
			return err
		}
		// UID comes from authenticated BR delivery. Payout destination and
		// terms are deterministic local facts, so neither belongs on the wire.
		expected, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(stdaddr.Hash160(key), params)
		if err != nil {
			return err
		}
		participant := gamingfunds.Participant{UID: event.From, Key: msg.Key, Destination: expected.String(), TermsHash: accepted.TermsHash()}
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
		if changed {
			return announceGamingAuthority(ctx, scope, part.SID)
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
		if flags & ^(financialParticipant|financialRoster) != 0 {
			return msg, fmt.Errorf("invalid participant flags")
		}
		fixed := 33
		if flags&financialRoster != 0 {
			fixed += 32
		}
		if len(payload) < fixed+2 || int(payload[fixed]) != len(payload)-fixed-1 || payload[fixed] == 0 {
			return msg, fmt.Errorf("invalid participant message")
		}
		msg.Key = hex.EncodeToString(payload[:33])
		if flags&financialRoster != 0 {
			msg.RosterHash = hex.EncodeToString(payload[33:fixed])
		}
		msg.Proof = append([]byte(nil), payload[fixed+1:]...)
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
		out := append([]byte{financialWireVersion<<financialVersionShift | flags}, key...)
		if msg.RosterHash != "" {
			hash, err := hex.DecodeString(msg.RosterHash)
			if err != nil || len(hash) != 32 {
				return nil, fmt.Errorf("invalid financial roster hash")
			}
			out[0] |= financialRoster
			out = append(out, hash...)
		}
		if len(msg.Proof) == 0 || len(msg.Proof) > 255 {
			return nil, fmt.Errorf("financial key announcement needs its proof")
		}
		out = append(out, byte(len(msg.Proof)))
		return append(out, msg.Proof...), nil
	}
	if msg.Settlement == "" || len(msg.Signatures) == 0 || len(msg.Signatures) > 255 || msg.RosterHash != "" {
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
