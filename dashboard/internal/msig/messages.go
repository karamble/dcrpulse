// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Message types. Handshake messages carry tempId (the invite frame's mid);
// everything after activation carries walletId (the shared P2SH address).
const (
	TypeInvite       = "invite"
	TypeAccept       = "accept"
	TypeDecline      = "decline"
	TypeRoster       = "roster"
	TypeReady        = "ready"
	TypeInviteCancel = "invite_cancel"
	TypeSignReq      = "sign_req"
	TypeSig          = "sig"
	TypeSigDecline   = "sig_decline"
	TypeBroadcast    = "broadcast"
	// TypeAttestSet closes the handshake: the initiator fans out every
	// cosigner's signature over the settled roster, and no cosigner activates
	// until it holds a valid one for each key. See attest.go.
	TypeAttestSet = "attest_set"
)

// Payload field caps.
const (
	MaxLabelLen  = 64
	MaxReasonLen = 200
	MaxNoteLen   = 200
	// MaxRawTxHex comfortably fits a 30-input 15-of-15 transaction.
	MaxRawTxHex     = 400_000
	maxScriptHexLen = 2048
	maxAddressLen   = 100
	pubKeyHexLen    = 66
	txidHexLen      = 64
	// maxAttestLen bounds a base64 65-byte compact signature with room to
	// spare; anything longer is not one.
	maxAttestLen = 120
)

// ErrUnknownType marks a structurally valid message of a type this build
// does not know. Receivers journal the mid and ignore it so the protocol
// can grow without breaking older nodes.
var ErrUnknownType = errors.New("unknown msig message type")

// ProtoHD is the handshake protocol version for HD (xpub ladder) shared
// wallets. Version 0 (the field absent) is the original single-address
// form. The two forms are mutually exclusive on the wire: a handshake
// frame carrying fields of both is invalid, and HD frames never include
// the v1 fields, so pre-HD builds fail validation and drop them without
// journaling — the post-upgrade history replay then completes the round.
const ProtoHD = 2

// Message is the JSON payload of one coordination frame. One flat struct
// covers every type; ValidateMessage enforces the per-type requirements
// and unknown JSON fields are ignored on decode.
type Message struct {
	Type     string   `json:"type"`
	Ver      int      `json:"ver,omitempty"`
	TempID   string   `json:"tempId,omitempty"`
	WalletID string   `json:"walletId,omitempty"`
	Label    string   `json:"label,omitempty"`
	M        int      `json:"m,omitempty"`
	N        int      `json:"n,omitempty"`
	Network  string   `json:"network,omitempty"`
	PubKey   string   `json:"pubkey,omitempty"`
	PubKeys  []string `json:"pubkeys,omitempty"`
	Xpub     string   `json:"xpub,omitempty"`
	Xpubs    []string `json:"xpubs,omitempty"`
	Script   string   `json:"scriptHex,omitempty"`
	Address  string   `json:"address,omitempty"`
	TxID     string   `json:"txid,omitempty"`
	RawTx    string   `json:"rawTx,omitempty"`
	Note     string   `json:"note,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	SigsHave int      `json:"sigsHave,omitempty"`

	// Peers rides the invite and the roster so every cosigner learns who
	// is said to hold which key, not just the initiator. Routing hints
	// only: the mapping is initiator-asserted and never guards funds,
	// which stay bound to the xpubs. On an invite the xpubs are absent -
	// nobody has accepted yet - and it is purely what the invitee is
	// shown before deciding.
	Peers []RosterPeer `json:"peers,omitempty"`

	// Attest carries one signature: proof of possession on an accept, the
	// initiator's roster commitment on a roster, a cosigner's on a ready.
	Attest string `json:"attest,omitempty"`

	// Attests carries the full set on an attest_set.
	Attests []RosterAttest `json:"attests,omitempty"`
}

// RosterPeer names one participant of a settled roster.
type RosterPeer struct {
	UID  string `json:"uid,omitempty"`
	Nick string `json:"nick,omitempty"`
	Xpub string `json:"xpub"`
}

// RosterAttest is one participant's signature over the roster, bound to the
// key that made it rather than to any claim about who holds that key.
type RosterAttest struct {
	Xpub string `json:"xpub"`
	Sig  string `json:"sig"`
}

// ManualTTL is the envelope lifetime for hand-carried frames: long
// enough for sneakernet, equal to the receiver journals' horizon, and
// refreshed at export time anyway.
const ManualTTL = 30 * 24 * time.Hour

// TTLFor returns the envelope lifetime for a message type. On Bison
// Relay everything matches the server's queued-PM horizon except sign
// requests, which stay short so a stale signing prompt dies after the
// hub re-routes. Manual frames all get the long lifetime: the human is
// the relay and nothing re-routes behind their back.
func TTLFor(msgType string, manual bool) time.Duration {
	if manual {
		return ManualTTL
	}
	if msgType == TypeSignReq {
		return 24 * time.Hour
	}
	return 7 * 24 * time.Hour
}

// EncodeMessage validates and marshals a message for framing.
func EncodeMessage(m *Message) ([]byte, error) {
	if err := ValidateMessage(m); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// DecodeMessage unmarshals and validates a frame payload. A message of an
// unknown type returns ErrUnknownType together with the parsed message so
// the caller can journal its mid.
func DecodeMessage(payload []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("malformed message: %v", err)
	}
	if err := ValidateMessage(&m); err != nil {
		if errors.Is(err, ErrUnknownType) {
			return &m, err
		}
		return nil, err
	}
	return &m, nil
}

func isLowerHex(s string) bool {
	if len(s)%2 != 0 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func validNetwork(n string) bool {
	switch n {
	case "mainnet", "testnet", "simnet":
		return true
	}
	return false
}

func validTempID(s string) bool { return midRE.MatchString(s) }

func validWalletID(s string) bool { return s != "" && len(s) <= maxAddressLen }

func validTxID(s string) bool { return len(s) == txidHexLen && isLowerHex(s) }

func validPubKey(s string) bool { return len(s) == pubKeyHexLen && isLowerHex(s) }

func validParams(m, n int) bool {
	return m >= MinRequired && n >= m && n <= MaxPubKeys
}

// handshakeVer resolves and bounds the protocol version of a handshake
// message, rejecting frames that mix the v1 (pubkey/script) and HD
// (xpub) field sets. Guessing between mixed forms would let one frame
// mean different wallets to different builds.
func handshakeVer(m *Message) (int, error) {
	switch m.Ver {
	case 0, ProtoHD:
	default:
		return 0, fmt.Errorf("%s: unsupported protocol version %d", m.Type, m.Ver)
	}
	v1Fields := m.PubKey != "" || len(m.PubKeys) > 0 || m.Script != ""
	hdFields := m.Xpub != "" || len(m.Xpubs) > 0
	if v1Fields && hdFields {
		return 0, fmt.Errorf("%s: mixed protocol forms", m.Type)
	}
	if m.Ver == ProtoHD && v1Fields {
		return 0, fmt.Errorf("%s: v1 fields in an HD frame", m.Type)
	}
	if m.Ver == 0 && hdFields {
		return 0, fmt.Errorf("%s: xpub fields without ver", m.Type)
	}
	return m.Ver, nil
}

// validXpubField bounds and parses one extended public key on the wire.
// The network cannot be pinned here (accept frames carry none); handlers
// re-parse strictly against the record's network.
func validXpubField(s string) error {
	if s == "" || len(s) > maxXpubLen {
		return fmt.Errorf("malformed xpub")
	}
	return ParseXpubAnyNet(s)
}

// validAttestField bounds an attestation signature. Empty is rejected by the
// callers that require one; the shape check lives here.
func validAttestField(s string) error {
	if len(s) > maxAttestLen {
		return fmt.Errorf("malformed attestation")
	}
	return nil
}

// validPeerTuples checks the identity hints on an invite or a roster. withXpubs
// is false on an invite, where no key has been offered yet.
func validPeerTuples(peers []RosterPeer, withXpubs bool) error {
	if len(peers) > MaxPubKeys {
		return fmt.Errorf("too many peer entries")
	}
	for i, p := range peers {
		if withXpubs && p.Xpub == "" {
			return fmt.Errorf("peer %d missing xpub", i)
		}
		if !withXpubs && p.Xpub != "" {
			return fmt.Errorf("peer %d carries a key before anyone accepted", i)
		}
		if len(p.Nick) > MaxLabelLen {
			return fmt.Errorf("peer %d nick too long", i)
		}
		if len(p.UID) > 64 || (p.UID != "" && !isLowerHex(p.UID)) {
			return fmt.Errorf("peer %d malformed uid", i)
		}
	}
	return nil
}

// ValidateMessage enforces the per-type field requirements.
func ValidateMessage(m *Message) error {
	if m.Type == "" {
		return fmt.Errorf("missing message type")
	}
	if len(m.Label) > MaxLabelLen {
		return fmt.Errorf("label exceeds %d characters", MaxLabelLen)
	}
	if len(m.Reason) > MaxReasonLen {
		return fmt.Errorf("reason exceeds %d characters", MaxReasonLen)
	}
	if len(m.Note) > MaxNoteLen {
		return fmt.Errorf("note exceeds %d characters", MaxNoteLen)
	}
	switch m.Type {
	case TypeInvite:
		ver, err := handshakeVer(m)
		if err != nil {
			return err
		}
		if !validTempID(m.TempID) {
			return fmt.Errorf("invite: malformed tempId")
		}
		if m.Label == "" {
			return fmt.Errorf("invite: missing label")
		}
		if !validParams(m.M, m.N) {
			return fmt.Errorf("invite: invalid m-of-n")
		}
		if !validNetwork(m.Network) {
			return fmt.Errorf("invite: invalid network")
		}
		if ver == ProtoHD {
			if err := validXpubField(m.Xpub); err != nil {
				return fmt.Errorf("invite: %v", err)
			}
			// The invite's peer list is who the initiator says it is
			// inviting. No key has been offered yet, so the entries carry
			// no xpub; they exist so the invitee can see the membership it
			// is being asked to join.
			if err := validPeerTuples(m.Peers, false); err != nil {
				return fmt.Errorf("invite: %v", err)
			}
		} else if !validPubKey(m.PubKey) {
			return fmt.Errorf("invite: malformed pubkey")
		}
	case TypeAccept:
		ver, err := handshakeVer(m)
		if err != nil {
			return err
		}
		if !validTempID(m.TempID) {
			return fmt.Errorf("accept: malformed tempId")
		}
		if ver == ProtoHD {
			if err := validXpubField(m.Xpub); err != nil {
				return fmt.Errorf("accept: %v", err)
			}
			// Proof of possession: the key is proven held before it ever
			// reaches a roster, so a round can never settle on a key nobody
			// has. Mandatory, so there is no unattested path to fall back to.
			if m.Attest == "" {
				return fmt.Errorf("accept: missing proof of possession")
			}
			if err := validAttestField(m.Attest); err != nil {
				return fmt.Errorf("accept: %v", err)
			}
		} else if !validPubKey(m.PubKey) {
			return fmt.Errorf("accept: malformed pubkey")
		}
	case TypeDecline, TypeInviteCancel:
		if !validTempID(m.TempID) {
			return fmt.Errorf("%s: malformed tempId", m.Type)
		}
	case TypeRoster:
		ver, err := handshakeVer(m)
		if err != nil {
			return err
		}
		if !validTempID(m.TempID) {
			return fmt.Errorf("roster: malformed tempId")
		}
		if m.Label == "" {
			return fmt.Errorf("roster: missing label")
		}
		if !validParams(m.M, m.N) {
			return fmt.Errorf("roster: invalid m-of-n")
		}
		if !validNetwork(m.Network) {
			return fmt.Errorf("roster: invalid network")
		}
		if ver == ProtoHD {
			if err := ValidateXpubRoster(m.Xpubs, m.N); err != nil {
				return fmt.Errorf("roster: %v", err)
			}
			if err := validPeerTuples(m.Peers, true); err != nil {
				return fmt.Errorf("roster: %v", err)
			}
			if err := validAttestField(m.Attest); err != nil {
				return fmt.Errorf("roster: %v", err)
			}
		} else {
			if len(m.PubKeys) != m.N {
				return fmt.Errorf("roster: expected %d pubkeys, got %d", m.N, len(m.PubKeys))
			}
			for i, pk := range m.PubKeys {
				if !validPubKey(pk) {
					return fmt.Errorf("roster: malformed pubkey %d", i)
				}
				// The roster is canonical: strictly ascending order also
				// forbids duplicate keys.
				if i > 0 && m.PubKeys[i-1] >= pk {
					return fmt.Errorf("roster: pubkeys not in canonical sorted order")
				}
			}
			if m.Script == "" || len(m.Script) > maxScriptHexLen || !isLowerHex(m.Script) {
				return fmt.Errorf("roster: malformed scriptHex")
			}
		}
		if !validWalletID(m.Address) {
			return fmt.Errorf("roster: missing address")
		}
	case TypeReady:
		if !validTempID(m.TempID) {
			return fmt.Errorf("ready: malformed tempId")
		}
		if !validWalletID(m.WalletID) {
			return fmt.Errorf("ready: missing walletId")
		}
		if err := validAttestField(m.Attest); err != nil {
			return fmt.Errorf("ready: %v", err)
		}
	case TypeAttestSet:
		if !validTempID(m.TempID) {
			return fmt.Errorf("attest_set: malformed tempId")
		}
		if !validWalletID(m.WalletID) {
			return fmt.Errorf("attest_set: missing walletId")
		}
		if len(m.Attests) == 0 || len(m.Attests) > MaxPubKeys {
			return fmt.Errorf("attest_set: expected between 1 and %d attestations", MaxPubKeys)
		}
		for i, a := range m.Attests {
			if err := validXpubField(a.Xpub); err != nil {
				return fmt.Errorf("attest_set: entry %d: %v", i, err)
			}
			if a.Sig == "" {
				return fmt.Errorf("attest_set: entry %d missing signature", i)
			}
			if err := validAttestField(a.Sig); err != nil {
				return fmt.Errorf("attest_set: entry %d: %v", i, err)
			}
		}
	case TypeSignReq, TypeSig:
		if !validWalletID(m.WalletID) {
			return fmt.Errorf("%s: missing walletId", m.Type)
		}
		if !validTxID(m.TxID) {
			return fmt.Errorf("%s: malformed txid", m.Type)
		}
		if m.RawTx == "" || len(m.RawTx) > MaxRawTxHex || !isLowerHex(m.RawTx) {
			return fmt.Errorf("%s: malformed rawTx", m.Type)
		}
		if m.SigsHave < 0 || m.SigsHave > MaxPubKeys {
			return fmt.Errorf("%s: sigsHave out of range", m.Type)
		}
	case TypeSigDecline, TypeBroadcast:
		if !validWalletID(m.WalletID) {
			return fmt.Errorf("%s: missing walletId", m.Type)
		}
		if !validTxID(m.TxID) {
			return fmt.Errorf("%s: malformed txid", m.Type)
		}
	default:
		return ErrUnknownType
	}
	return nil
}
