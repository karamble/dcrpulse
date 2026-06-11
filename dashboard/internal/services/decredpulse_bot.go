// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"dcrpulse/internal/config"
)

// The Decred Pulse invite bot (brulse) is an external service that issues a
// Bison Relay invite into the community "Decred Pulse" group chat. It defaults
// to the community-hosted instance; set BRULSE_API_URL to override (for example
// to a local instance during development or a TLS-fronted endpoint).
const (
	defaultDecredPulseBotURL = "http://brulse.decredcommunity.org:8080"
	decredPulseBotTimeout    = 20 * time.Second
	// powSolveCap bounds the proof-of-work search so a misbehaving or
	// overly-difficult challenge cannot spin forever.
	powSolveCap = 1 << 28
)

// DecredPulseBotEnabled reports whether the user has the Decred Pulse bot
// external-request toggle on. Defaults to true when no global config is present.
func DecredPulseBotEnabled() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	if allowed == nil {
		return true
	}
	v, ok := allowed[config.ExternalRequestDecredPulseBot]
	if !ok {
		return true
	}
	return v
}

// decredPulseBotURL returns the brulse base URL with any trailing slash
// removed, honoring the BRULSE_API_URL override and otherwise falling back to
// the community-hosted instance.
func decredPulseBotURL() string {
	if v := strings.TrimRight(os.Getenv("BRULSE_API_URL"), "/"); v != "" {
		return v
	}
	return defaultDecredPulseBotURL
}

type botChallenge struct {
	Nonce       string `json:"nonce"`
	Bits        uint   `json:"bits"`
	Expires     int64  `json:"expires"`
	PoWRequired bool   `json:"powRequired"`
}

// RequestDecredPulseInvite asks the bot for an invite for the given Bison Relay
// public identity (hex), solving the proof-of-work challenge if one is
// required, and returns the redeemable invite key (brpik1...).
func RequestDecredPulseInvite(ctx context.Context, pubkeyHex string) (string, error) {
	if !DecredPulseBotEnabled() {
		return "", fmt.Errorf("Decred Pulse bot requests are disabled in settings")
	}
	base := decredPulseBotURL()

	ctx, cancel := context.WithTimeout(ctx, decredPulseBotTimeout)
	defer cancel()

	ch, err := fetchBotChallenge(ctx, base)
	if err != nil {
		return "", err
	}

	var nonce, solution string
	if ch.PoWRequired {
		nonce = ch.Nonce
		solution, err = solvePoW(ch.Nonce, ch.Bits)
		if err != nil {
			return "", err
		}
	}

	return requestBotInvite(ctx, base, pubkeyHex, nonce, solution)
}

func fetchBotChallenge(ctx context.Context, base string) (*botChallenge, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/challenge", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("invite bot challenge: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var ch botChallenge
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&ch); err != nil {
		return nil, fmt.Errorf("decode challenge: %w", err)
	}
	return &ch, nil
}

func requestBotInvite(ctx context.Context, base, pubkeyHex, nonce, solution string) (string, error) {
	payload := map[string]string{
		"pubkey":      pubkeyHex,
		"powNonce":    nonce,
		"powSolution": solution,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/invite", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("invite bot: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		InviteKey string `json:"inviteKey"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&out); err != nil {
		return "", fmt.Errorf("decode invite response: %w", err)
	}
	if out.InviteKey == "" {
		return "", fmt.Errorf("invite bot returned an empty invite key")
	}
	return out.InviteKey, nil
}

// solvePoW finds a solution string such that sha256(nonce ":" solution) has at
// least bits leading zero bits. It mirrors the verification in the brulse bot.
func solvePoW(nonce string, bits uint) (string, error) {
	prefix := nonce + ":"
	for i := 0; i < powSolveCap; i++ {
		sol := strconv.Itoa(i)
		sum := sha256.Sum256([]byte(prefix + sol))
		if leadingZeroBits(sum[:]) >= bits {
			return sol, nil
		}
	}
	return "", fmt.Errorf("could not solve proof-of-work (difficulty too high)")
}

func leadingZeroBits(b []byte) uint {
	var n uint
	for _, c := range b {
		if c == 0 {
			n += 8
			continue
		}
		for mask := byte(0x80); mask != 0; mask >>= 1 {
			if c&mask != 0 {
				return n
			}
			n++
		}
		break
	}
	return n
}
