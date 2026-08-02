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
)

// ErrUnknownType marks a structurally valid message of a type this build
// does not know. Receivers journal the mid and ignore it so the protocol
// can grow without breaking older nodes.
var ErrUnknownType = errors.New("unknown msig message type")

// Message is the JSON payload of one coordination frame. One flat struct
// covers every type; ValidateMessage enforces the per-type requirements
// and unknown JSON fields are ignored on decode.
type Message struct {
	Type     string   `json:"type"`
	TempID   string   `json:"tempId,omitempty"`
	WalletID string   `json:"walletId,omitempty"`
	Label    string   `json:"label,omitempty"`
	M        int      `json:"m,omitempty"`
	N        int      `json:"n,omitempty"`
	Network  string   `json:"network,omitempty"`
	PubKey   string   `json:"pubkey,omitempty"`
	PubKeys  []string `json:"pubkeys,omitempty"`
	Script   string   `json:"scriptHex,omitempty"`
	Address  string   `json:"address,omitempty"`
	TxID     string   `json:"txid,omitempty"`
	RawTx    string   `json:"rawTx,omitempty"`
	Note     string   `json:"note,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	SigsHave int      `json:"sigsHave,omitempty"`
}

// TTLFor returns the envelope lifetime for a message type. Everything
// matches the BR server's queued-PM horizon except sign requests, which
// stay short so a stale signing prompt dies after the hub re-routes.
func TTLFor(msgType string) time.Duration {
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
		if !validPubKey(m.PubKey) {
			return fmt.Errorf("invite: malformed pubkey")
		}
	case TypeAccept:
		if !validTempID(m.TempID) {
			return fmt.Errorf("accept: malformed tempId")
		}
		if !validPubKey(m.PubKey) {
			return fmt.Errorf("accept: malformed pubkey")
		}
	case TypeDecline, TypeInviteCancel:
		if !validTempID(m.TempID) {
			return fmt.Errorf("%s: malformed tempId", m.Type)
		}
	case TypeRoster:
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
