package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	gamingOutbox.dir = GamingStateDir
	gamingOutbox.claims = make(map[string]gamingSendState)
	f, err := os.Open(filepath.Join(GamingStateDir, gamingOutboxFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for line := 1; scan.Scan(); line++ {
		var claim gamingSendClaim
		if err := json.Unmarshal(scan.Bytes(), &claim); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		key := gamingSendKey(claim)
		if claim.State != "claimed" && claim.State != "sent" {
			return fmt.Errorf("line %d: invalid send state", line)
		}
		if old, exists := gamingOutbox.claims[key]; exists {
			if old.digest != claim.Digest {
				return fmt.Errorf("line %d: message identity collision", line)
			}
			if old.state == "sent" && claim.State != "sent" {
				return fmt.Errorf("line %d: send state went backwards", line)
			}
		}
		gamingOutbox.claims[key] = gamingSendState{digest: claim.Digest, state: claim.State}
	}
	return scan.Err()
}

// claimGamingFrameSend durably consumes one physical-send opportunity. The
// claim is synced before the BR RPC begins, so a game retry or bridge restart
// can never publish the same immutable wire part again. An RPC failure is
// returned to the original caller and remains final; recovery comes from BR
// history and bridge inbox replay, never peer retransmission.
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
		return false, errGamingSendUncertain
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
