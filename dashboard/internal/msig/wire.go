// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package msig

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Wire envelope: --msig[v=1,mid=<hex>,exp=<unixsecs>]--<base64(payload)>.
// The grammar is cloned from the brmcp wire: anchored whole-body match,
// restricted base64 alphabet so a body can never smuggle a second frame,
// and unknown k=v header keys tolerated for forward compatibility. Frames
// are single part; every payload is far below the Bison Relay PM floor.

const (
	// WireVersion is the envelope version this codec emits.
	WireVersion = 1

	// MaxPayload bounds the raw payload of one frame.
	MaxPayload = 256 * 1024

	// maxBody bounds a framed body before it reaches the regexp.
	maxBody = 512_000

	// ClockSkewGrace tolerates cross-host clock drift on expiry checks.
	ClockSkewGrace = 300 * time.Second
)

// SampleEnvelope mirrors brclientd's content-filter guard sample.
const SampleEnvelope = "--msig[v=1,mid=0011223344556677,exp=1700000000]--aGVsbG8="

var (
	envelopeRE = regexp.MustCompile(`^--msig\[([^\]]*)\]--([A-Za-z0-9+/=\s]*)$`)
	midRE      = regexp.MustCompile(`^[0-9a-f]{1,32}$`)
)

var (
	// ErrNotEnvelope reports a body that is not msig traffic at all.
	ErrNotEnvelope = errors.New("not an msig envelope")
	// ErrUnsupportedVersion reports msig traffic this codec cannot read.
	ErrUnsupportedVersion = errors.New("unsupported msig envelope version")
	// ErrExpired reports a frame past its deadline; expired frames are
	// dropped without being journaled so a later replay can never act on
	// stale protocol state.
	ErrExpired = errors.New("msig envelope expired")
)

// Frame is one decoded coordination envelope.
type Frame struct {
	MID     string
	Exp     time.Time
	Payload []byte
}

// NewID returns a fresh 16-hex message id. The invite's id doubles as the
// handshake tempId.
func NewID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Encode frames a payload for sending as a private message body.
func Encode(payload []byte, mid string, exp time.Time) (string, error) {
	if len(payload) == 0 {
		return "", fmt.Errorf("empty payload")
	}
	if len(payload) > MaxPayload {
		return "", fmt.Errorf("payload exceeds %d bytes", MaxPayload)
	}
	if !midRE.MatchString(mid) {
		return "", fmt.Errorf("malformed mid")
	}
	if exp.IsZero() {
		return "", fmt.Errorf("missing expiry")
	}
	return fmt.Sprintf("--msig[v=%d,mid=%s,exp=%d]--%s",
		WireVersion, mid, exp.Unix(), base64.StdEncoding.EncodeToString(payload)), nil
}

// IsEnvelope is the coarse classifier shared with brclientd: anything
// matching is protocol traffic (of any version), even if a strict Parse
// would reject it.
func IsEnvelope(text string) bool {
	return len(text) <= maxBody && envelopeRE.MatchString(text)
}

// Parse strictly decodes one frame. Non-envelopes return ErrNotEnvelope,
// other-version traffic returns ErrUnsupportedVersion, deadline misses
// return ErrExpired (with ClockSkewGrace applied), and anything else
// malformed returns a descriptive error.
func Parse(text string, now time.Time) (*Frame, error) {
	if len(text) > maxBody {
		return nil, ErrNotEnvelope
	}
	m := envelopeRE.FindStringSubmatch(text)
	if m == nil {
		return nil, ErrNotEnvelope
	}
	var mid string
	var exp int64
	var expSet, verOK bool
	for kv := range strings.SplitSeq(m[1], ",") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch k {
		case "v":
			verOK = v == strconv.Itoa(WireVersion)
		case "mid":
			mid = v
		case "exp":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("malformed exp: %v", err)
			}
			exp = n
			expSet = true
		}
	}
	if !verOK {
		return nil, ErrUnsupportedVersion
	}
	if !midRE.MatchString(mid) {
		return nil, fmt.Errorf("missing or malformed mid")
	}
	if !expSet {
		return nil, fmt.Errorf("missing exp")
	}
	expT := time.Unix(exp, 0)
	if now.After(expT.Add(ClockSkewGrace)) {
		return nil, ErrExpired
	}
	b64 := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		}
		return r
	}, m[2])
	payload, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("malformed payload encoding: %v", err)
	}
	if len(payload) > MaxPayload {
		return nil, fmt.Errorf("payload exceeds %d bytes", MaxPayload)
	}
	return &Frame{MID: mid, Exp: expT, Payload: payload}, nil
} // Reencode re-frames an existing envelope with a fresh expiry, keeping
// the mid and payload byte-identical. Manual exports use it so a blob is
// always valid for its full lifetime from the moment it is handed over;
// the unchanged mid keeps receiver journals deduplicating correctly no
// matter which copy arrives.
func Reencode(body string, exp time.Time) (string, error) {
	m := envelopeRE.FindStringSubmatch(body)
	if m == nil {
		return "", ErrNotEnvelope
	}
	mid, payload := "", ""
	header := m[1]
	for _, kv := range strings.Split(header, ",") {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 && parts[0] == "mid" {
			mid = parts[1]
		}
	}
	payload = m[2]
	if mid == "" {
		return "", fmt.Errorf("envelope carries no mid")
	}
	return fmt.Sprintf("--msig[v=%d,mid=%s,exp=%d]--%s", WireVersion, mid, exp.Unix(), payload), nil
}
