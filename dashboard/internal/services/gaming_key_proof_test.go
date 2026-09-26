package services

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

const (
	proofSID   = "0123456789abcdef"
	proofTerms = "terms-hash"
)

// signedProof signs a key proof exactly as dcrwallet's SignHashes does.
func signedProof(t *testing.T, keyN byte, uid, sid, gcid, terms string) ([]byte, []byte) {
	t.Helper()
	priv := testPriv(keyN)
	key := priv.PubKey().SerializeCompressed()
	hash, err := keyProofHash(uid, "poker", sid, gcid, terms, key)
	if err != nil {
		t.Fatal(err)
	}
	return ecdsa.Sign(priv, hash[:]).Serialize(), key
}

func TestKeyProofBindsKeySenderAndTable(t *testing.T) {
	proof, key := signedProof(t, 5, prunePeer, proofSID, pruneGCA, proofTerms)
	if err := verifyKeyProof(proof, prunePeer, "poker", proofSID, pruneGCA, proofTerms, key); err != nil {
		t.Fatalf("valid proof refused: %v", err)
	}
	_, otherKey := signedProof(t, 6, prunePeer, proofSID, pruneGCA, proofTerms)
	for name, check := range map[string]func() error{
		"relayed by another account": func() error {
			return verifyKeyProof(proof, pruneSelf, "poker", proofSID, pruneGCA, proofTerms, key)
		},
		"another table": func() error {
			return verifyKeyProof(proof, prunePeer, "poker", "0123456789abcdee", pruneGCA, proofTerms, key)
		},
		"another group": func() error {
			return verifyKeyProof(proof, prunePeer, "poker", proofSID, pruneGCB, proofTerms, key)
		},
		"other terms": func() error {
			return verifyKeyProof(proof, prunePeer, "poker", proofSID, pruneGCA, "other", key)
		},
		"another game": func() error {
			return verifyKeyProof(proof, prunePeer, "chess", proofSID, pruneGCA, proofTerms, key)
		},
		"someone else's key": func() error {
			return verifyKeyProof(proof, prunePeer, "poker", proofSID, pruneGCA, proofTerms, otherKey)
		},
		"not a signature": func() error {
			return verifyKeyProof([]byte{1, 2, 3}, prunePeer, "poker", proofSID, pruneGCA, proofTerms, key)
		},
	} {
		if check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestKeyIsAnnouncedOnlyOnceProven(t *testing.T) {
	withGamingWireDir(t)
	registerPoker(t)
	store, _ := payoutLedger(t, "awaiting_signatures")
	withHistorySeams(t, historyPages())
	var sent []string
	old := gamingGCSend
	gamingGCSend = func(_ context.Context, _ rpc.ShortIDHex, frame string, _ int) error {
		sent = append(sent, frame)
		return nil
	}
	t.Cleanup(func() { gamingGCSend = old })
	scope := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	if err := announceGamingAuthority(context.Background(), scope, proofSID); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 0 {
		t.Fatalf("an unproven key was announced: %v", sent)
	}
	proof := hex.EncodeToString([]byte{0x30, 0x06, 0x02, 0x01, 0x01, 0x02, 0x01, 0x01})
	if err := store.SaveKeyProof(scope, proofSID, proof); err != nil {
		t.Fatal(err)
	}
	if err := announceGamingAuthority(context.Background(), scope, proofSID); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 {
		t.Fatalf("announcements = %d", len(sent))
	}
	part, err := financialPart(sent[0])
	if err != nil {
		t.Fatal(err)
	}
	msg, err := decodeFinancialMessage(part.Payload)
	if err != nil || hex.EncodeToString(msg.Proof) != proof {
		t.Fatalf("announced = %+v, %v", msg, err)
	}
}

func TestOnlyTheSeatBondProvesTheKey(t *testing.T) {
	calls := 0
	fail := false
	old := gamingKeyProofSign
	gamingKeyProofSign = func(_ context.Context, _ gamingfunds.Scope, table string, pass []byte) error {
		calls++
		if table != proofSID || string(pass) != "pass" {
			t.Errorf("proof asked for %q with %q", table, pass)
		}
		for i := range pass {
			pass[i] = 0 // as the wallet signer does
		}
		if fail {
			return errors.New("wrong passphrase")
		}
		return nil
	}
	t.Cleanup(func() { gamingKeyProofSign = old })
	bond := gamingfunds.Deposit{Terms: gamingfunds.Terms{Kind: "seatbond", Table: proofSID}}
	stake := gamingfunds.Deposit{Terms: gamingfunds.Terms{Kind: "stake", Table: proofSID}}
	pass := []byte("pass")
	if proven, err := proveSeatBondKey(context.Background(), stake, pass); err != nil || proven || calls != 0 {
		t.Fatalf("stake = %v, %v, calls %d", proven, err, calls)
	}
	if proven, err := proveSeatBondKey(context.Background(), bond, pass); err != nil || !proven || calls != 1 {
		t.Fatalf("seat bond = %v, %v, calls %d", proven, err, calls)
	}
	if string(pass) != "pass" {
		t.Fatal("the approval's own passphrase was consumed")
	}
	fail = true
	if _, err := proveSeatBondKey(context.Background(), bond, pass); err == nil {
		t.Fatal("a failed proof let the approval continue")
	}
}

func TestReceivedKeyAnnouncementsMustBeProven(t *testing.T) {
	withGamingWireDir(t)
	store, _ := payoutLedger(t, "awaiting_signatures")
	scope := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	oldScope, oldParams := receiveScope, receiveParams
	receiveScope = func(context.Context, string) (gamingfunds.Scope, error) { return scope, nil }
	receiveParams = func(context.Context) (*chaincfg.Params, error) { return chaincfg.MainNetParams(), nil }
	t.Cleanup(func() { receiveScope, receiveParams = oldScope, oldParams })
	accepted, err := store.AuthorizedTable(scope, proofSID)
	if err != nil {
		t.Fatal(err)
	}
	proof, key := signedProof(t, 5, prunePeer, proofSID, pruneGCA, accepted.TermsHash())
	event := func(from string, proof []byte) GamingFrameEvent {
		_, frame, err := financialFrame("poker", proofSID, financialMessage{Key: hex.EncodeToString(key), Proof: proof})
		if err != nil {
			t.Fatal(err)
		}
		return GamingFrameEvent{Game: "poker", GCID: pruneGCA, From: from, Frame: frame, Financial: true}
	}
	copier := strings.Repeat("3", 64)
	if err := receiveFinancialFrame(context.Background(), event(copier, proof)); err == nil {
		t.Fatal("another account's copy of a key announcement was accepted")
	}
	forged, _ := signedProof(t, 6, prunePeer, proofSID, pruneGCA, accepted.TermsHash())
	if err := receiveFinancialFrame(context.Background(), event(prunePeer, forged)); err == nil {
		t.Fatal("a proof by another key was accepted")
	}
	if err := receiveFinancialFrame(context.Background(), event(prunePeer, proof)); err != nil {
		t.Fatalf("a proven announcement was refused: %v", err)
	}
	own, err := store.WalletKey(scope, proofSID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BindSeats(scope, proofSID, []string{hex.EncodeToString(key), own.Public}); err != nil {
		t.Fatal(err)
	}
	peers, err := store.Participants(scope, proofSID)
	if err != nil || len(peers) != 1 || peers[0].UID != prunePeer {
		t.Fatalf("roster = %+v, %v", peers, err)
	}
}
