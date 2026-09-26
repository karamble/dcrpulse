package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dcrpulse/internal/rpc"
)

const gamingOutboxFile = "gaming-wire-outbox-v2.jsonl"

type gamingSendClaim struct {
	Game      string `json:"game"`
	GCID      string `json:"gcid"`
	MID       string `json:"mid"`
	Part      string `json:"part"`
	Digest    string `json:"digest"`
	ClaimedAt int64  `json:"claimed_at"`
	State     string `json:"state"`
}

type gamingSendState struct {
	digest string
	state  string
}

var errGamingSendUncertain = errors.New("gaming message publication is uncertain; reconcile local BR history")

var gamingOutbox = struct {
	sync.Mutex
	dir    string
	claims map[string]gamingSendState
}{}

func gamingSendKey(c gamingSendClaim) string {
	return c.Game + "\x00" + c.GCID + "\x00" + c.MID + "\x00" + c.Part
}

func loadGamingSendClaimsLocked() error {
	if gamingOutbox.dir == GamingStateDir && gamingOutbox.claims != nil {
		return nil
	}
	path := filepath.Join(GamingStateDir, gamingOutboxFile)
	claims, err := readGamingSendClaims(path)
	if err != nil {
		return fmt.Errorf("gaming outbox %s: %w", path, err)
	}
	// Installed only once the whole file read cleanly.
	gamingOutbox.dir, gamingOutbox.claims = GamingStateDir, claims
	return nil
}

// readGamingSendClaims reads the outbox file. A missing file has no claims.
func readGamingSendClaims(path string) (map[string]gamingSendState, error) {
	claims := make(map[string]gamingSendState)
	dropped, err := healTornTail(path)
	if errors.Is(err, os.ErrNotExist) {
		return claims, nil
	}
	if err != nil {
		return nil, err
	}
	if dropped > 0 {
		gameLog.Warnf("gaming outbox %s: dropped an unfinished last record (%d bytes)", path, dropped)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), gamingWireMaxLine)
	for line := 1; scan.Scan(); line++ {
		var claim gamingSendClaim
		if err := json.Unmarshal(scan.Bytes(), &claim); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		key := gamingSendKey(claim)
		if claim.State != "claimed" && claim.State != "sent" && claim.State != "released" {
			return nil, fmt.Errorf("line %d: invalid send state", line)
		}
		if old, exists := claims[key]; exists {
			if old.digest != claim.Digest {
				return nil, fmt.Errorf("line %d: message identity collision", line)
			}
			if old.state == "sent" && claim.State != "sent" {
				return nil, fmt.Errorf("line %d: send state went backwards", line)
			}
		}
		claims[key] = gamingSendState{digest: claim.Digest, state: claim.State}
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return claims, nil
}

// claimGamingFrameSend durably consumes one physical-send opportunity. The
// claim is synced before the BR RPC begins, so a game retry or bridge restart
// can never publish the same immutable wire part again. Only a claim released
// because brclientd refused the send can be claimed again; recovery otherwise
// comes from BR history and bridge inbox replay, never peer retransmission.
func claimGamingFrameSend(game, gcid string, frame gamingFrame, text string) (bool, error) {
	gamingOutbox.Lock()
	defer gamingOutbox.Unlock()
	if err := loadGamingSendClaimsLocked(); err != nil {
		return false, err
	}
	digest := sha256.Sum256([]byte(text))
	claim := gamingSendClaim{
		Game: game, GCID: gcid, MID: frame.MID, Part: frame.Part,
		Digest: hex.EncodeToString(digest[:]), ClaimedAt: time.Now().Unix(), State: "claimed",
	}
	key := gamingSendKey(claim)
	if old, exists := gamingOutbox.claims[key]; exists {
		if old.digest != claim.Digest {
			return false, fmt.Errorf("gaming message identity collision")
		}
		if old.state == "sent" {
			return false, nil
		}
		if old.state != "released" {
			return false, errGamingSendUncertain
		}
	}
	if err := appendGamingSendClaimLocked(claim); err != nil {
		return false, err
	}
	gamingOutbox.claims[key] = gamingSendState{digest: claim.Digest, state: claim.State}
	return true, nil
}

func appendGamingSendClaimLocked(claim gamingSendClaim) error {
	if err := os.MkdirAll(GamingStateDir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(claim)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(GamingStateDir, gamingOutboxFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(raw, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func markGamingFrameSent(game, gcid string, frame gamingFrame, text string) error {
	gamingOutbox.Lock()
	defer gamingOutbox.Unlock()
	if err := loadGamingSendClaimsLocked(); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(text))
	claim := gamingSendClaim{
		Game: game, GCID: gcid, MID: frame.MID, Part: frame.Part,
		Digest: hex.EncodeToString(digest[:]), ClaimedAt: time.Now().Unix(), State: "sent",
	}
	key := gamingSendKey(claim)
	old, exists := gamingOutbox.claims[key]
	if !exists || old.digest != claim.Digest {
		return fmt.Errorf("gaming message was not claimed before send")
	}
	if old.state == "sent" {
		return nil
	}
	if err := appendGamingSendClaimLocked(claim); err != nil {
		return err
	}
	gamingOutbox.claims[key] = gamingSendState{digest: claim.Digest, state: claim.State}
	return nil
}

// releaseGamingFrameClaim frees a claim whose send brclientd refused, so the
// message never reached Bison Relay and may be sent once later.
func releaseGamingFrameClaim(game, gcid string, frame gamingFrame, text string) error {
	gamingOutbox.Lock()
	defer gamingOutbox.Unlock()
	if err := loadGamingSendClaimsLocked(); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(text))
	claim := gamingSendClaim{
		Game: game, GCID: gcid, MID: frame.MID, Part: frame.Part,
		Digest: hex.EncodeToString(digest[:]), ClaimedAt: time.Now().Unix(), State: "released",
	}
	key := gamingSendKey(claim)
	old, exists := gamingOutbox.claims[key]
	if !exists || old.digest != claim.Digest || old.state != "claimed" {
		return fmt.Errorf("gaming message is not an open claim")
	}
	if err := appendGamingSendClaimLocked(claim); err != nil {
		return err
	}
	gamingOutbox.claims[key] = gamingSendState{digest: claim.Digest, state: claim.State}
	return nil
}

// gamingFrameSendState reports a message's send state: "sent", "claimed"
// (outcome unknown), "released" or "" for never claimed.
func gamingFrameSendState(game, gcid string, frame gamingFrame, text string) (string, error) {
	gamingOutbox.Lock()
	defer gamingOutbox.Unlock()
	if err := loadGamingSendClaimsLocked(); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(text))
	key := gamingSendKey(gamingSendClaim{Game: game, GCID: gcid, MID: frame.MID, Part: frame.Part})
	old, exists := gamingOutbox.claims[key]
	if !exists {
		return "", nil
	}
	if old.digest != hex.EncodeToString(digest[:]) {
		return "", fmt.Errorf("gaming message identity collision")
	}
	return old.state, nil
}

// gamingGCSend posts to a group chat. Settable for tests; production never sets it.
var gamingGCSend = rpc.BrclientdGCMessage

// sendGamingFrameOnce sends a frame to Bison Relay at most once. A send that
// brclientd refused releases the claim; any other failure leaves the outcome
// unknown, to be settled only from brclientd's own record of what it sent.
func sendGamingFrameOnce(ctx context.Context, game, gcid string, parsed gamingFrame, frame string) error {
	fresh, err := claimOrReconcileGamingFrame(ctx, game, gcid, parsed, frame)
	if err != nil || !fresh {
		return err
	}
	return sendClaimedGamingFrame(ctx, game, gcid, parsed, frame)
}

// sendClaimedGamingFrame performs the one send a fresh claim allows.
func sendClaimedGamingFrame(ctx context.Context, game, gcid string, parsed gamingFrame, frame string) error {
	id, err := parseGamingGCID(gcid)
	if err != nil {
		return err
	}
	if err := gamingGCSend(ctx, id, frame, 0); err != nil {
		var refused *rpc.BrclientdStatusError
		if errors.As(err, &refused) {
			if relErr := releaseGamingFrameClaim(game, gcid, parsed, frame); relErr != nil {
				gameLog.Errorf("release refused gaming send: %v", relErr)
			}
		}
		return err
	}
	return markGamingFrameSent(game, gcid, parsed, frame)
}
