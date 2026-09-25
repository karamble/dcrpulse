// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/fsutil"
	"dcrpulse/internal/rpc"
)

// CommunityJoin is a server-owned target for one explicit join request. It
// contains no redeemable invitation secret. Names never establish membership.
type CommunityJoin struct {
	ID       string `json:"id"`
	BotUID   string `json:"botUID,omitempty"`
	GCID     string `json:"gcid,omitempty"`
	Status   string `json:"status"`
	Joined   bool   `json:"joined"`
	Insecure bool   `json:"insecure,omitempty"`
	URL      string `json:"-"`
	LocalUID string `json:"-"`
}
type communityJoinDisk struct {
	Join     CommunityJoin `json:"join"`
	URL      string        `json:"url"`
	LocalUID string        `json:"localUID"`
}
type communityInvites struct {
	Invites []struct {
		ID       uint64 `json:"id"`
		From     string `json:"from"`
		GCID     string `json:"gcid"`
		Accepted bool   `json:"accepted"`
		Expires  int64  `json:"expires"`
	} `json:"invites"`
}
type communityGroup struct {
	ID          string   `json:"id"`
	Members     []string `json:"members"`
	LocalMember *bool    `json:"local_is_member"`
}

var errCommunityJoinChanged = errors.New("community join belongs to a different bot or local identity; start a new join")

type communityJoinManager struct {
	mu       sync.Mutex
	path     string
	url      func() string
	identity func(context.Context) (json.RawMessage, error)
	invite   func(context.Context, string, string) (DecredPulseInvite, error)
	redeem   func(context.Context, string) error
	groups   func(context.Context) (json.RawMessage, error)
	invites  func(context.Context) (json.RawMessage, error)
	accept   func(context.Context, uint64) error
}

var communityJoins = communityJoinManager{
	path: filepath.Join(config.AppDataDir, "community-join.json"),
	url:  decredPulseBotURL, identity: rpc.BrclientdUserPublicIdentity,
	invite: requestDecredPulseInviteAt, redeem: rpc.BrclientdRedeemPaidInviteKey,
	groups: rpc.BrclientdGCList, invites: rpc.BrclientdGCInvitesList,
	accept: rpc.BrclientdGCInvitesAccept,
}

func (m *communityJoinManager) localIdentity(ctx context.Context) (string, error) {
	raw, err := m.identity(ctx)
	if err != nil {
		return "", err
	}
	var id struct {
		Identity string `json:"identity"`
	}
	if err := json.Unmarshal(raw, &id); err != nil {
		return "", err
	}
	b, err := base64.StdEncoding.DecodeString(id.Identity)
	if err != nil || len(b) != 32 {
		return "", fmt.Errorf("malformed local BR identity")
	}
	return hex.EncodeToString(b), nil
}
func (m *communityJoinManager) load() (*CommunityJoin, error) {
	raw, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var disk communityJoinDisk
	if err := json.Unmarshal(raw, &disk); err != nil {
		return nil, err
	}
	j := disk.Join
	j.URL, j.LocalUID = disk.URL, disk.LocalUID
	if j.ID == "" || !validCommunityID(j.LocalUID) || j.URL == "" ||
		(j.Status != "manual" && (!validCommunityID(j.BotUID) || !validCommunityID(j.GCID))) {
		return nil, fmt.Errorf("invalid saved community join")
	}
	return &j, nil
}
func (m *communityJoinManager) save(j *CommunityJoin) error {
	raw, err := json.Marshal(communityJoinDisk{Join: *j, URL: j.URL, LocalUID: j.LocalUID})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	if err := fsutil.AtomicWriteJSON(m.path, raw); err != nil {
		return err
	}
	// Redemption is an external side effect: make the saved binding durable
	// before issuing it, including the rename into the parent directory.
	for _, path := range []string{m.path, filepath.Dir(m.path)} {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func (m *communityJoinManager) current(ctx context.Context, j *CommunityJoin) error {
	uid, err := m.localIdentity(ctx)
	if err != nil {
		return err
	}
	if j.URL != m.url() || j.LocalUID != uid {
		return errCommunityJoinChanged
	}
	return nil
}
func (m *communityJoinManager) membership(ctx context.Context, j *CommunityJoin) error {
	j.Joined = false
	if j.GCID == "" {
		return nil
	}
	raw, err := m.groups(ctx)
	if err != nil {
		return err
	}
	var result struct {
		Groups []communityGroup `json:"gcs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	for _, g := range result.Groups {
		if g.ID != j.GCID || (g.LocalMember != nil && !*g.LocalMember) {
			continue
		}
		for _, uid := range g.Members {
			if uid == j.LocalUID {
				j.Joined = true
			}
		}
	}
	return nil
}

// begin resumes a pending request rather than issuing another external invite
// on browser reload. Only an explicit restart replaces the bounded saved slot.
func (m *communityJoinManager) begin(ctx context.Context, restart bool) (*CommunityJoin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uid, err := m.localIdentity(ctx)
	if err != nil {
		return nil, err
	}
	base := m.url()
	old, err := m.load()
	if err != nil {
		return nil, err
	}
	if !restart && old != nil && old.URL == base && old.LocalUID == uid {
		if err := m.membership(ctx, old); err != nil {
			return nil, err
		}
		return old, nil
	}
	invite, err := m.invite(ctx, base, uid)
	if err != nil {
		return nil, err
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	j := &CommunityJoin{ID: hex.EncodeToString(token), URL: base, LocalUID: uid, BotUID: invite.BotUID, GCID: invite.GCID, Status: "waiting"}
	if j.BotUID == "" && j.GCID == "" {
		j.Status = "manual"
	} else if !validCommunityID(j.BotUID) || !validCommunityID(j.GCID) {
		return nil, fmt.Errorf("invalid community identity metadata")
	}
	// Over a URL that does not authenticate the bot, anyone on the path could
	// have supplied the bot and group, so nothing is accepted automatically.
	if !botURLAuthenticated(base) {
		j.Status, j.Insecure = "manual", true
	}
	if err := m.current(ctx, j); err != nil {
		return nil, err
	}
	// Persist before redemption: an uncertain RPC result must not cause a fresh
	// external request or forget the identity of an invitation already in flight.
	if err := m.save(j); err != nil {
		return nil, err
	}
	if err := m.redeem(ctx, invite.InviteKey); err != nil {
		if j.Status != "manual" {
			j.Status = "uncertain"
		}
		if saveErr := m.save(j); saveErr != nil {
			return nil, saveErr
		}
		return nil, fmt.Errorf("invite redemption could not be confirmed; resume this join to wait for delivery: %w", err)
	}
	return j, nil
}
func (m *communityJoinManager) status(ctx context.Context) (*CommunityJoin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, err := m.load()
	if err != nil || j == nil {
		return j, err
	}
	if err := m.current(ctx, j); err != nil {
		if errors.Is(err, errCommunityJoinChanged) {
			return nil, nil
		}
		return nil, err
	}
	if err := m.membership(ctx, j); err != nil {
		return nil, err
	}
	return j, nil
}
func (m *communityJoinManager) acceptMatching(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, err := m.load()
	if err != nil {
		return err
	}
	if j == nil || id == "" || j.ID != id || j.Status == "manual" {
		return fmt.Errorf("no matching automatic community join")
	}
	if err := m.current(ctx, j); err != nil {
		return err
	}
	if err := m.membership(ctx, j); err != nil {
		return err
	}
	if j.Joined {
		return nil
	}
	raw, err := m.invites(ctx)
	if err != nil {
		return err
	}
	var pending communityInvites
	if err := json.Unmarshal(raw, &pending); err != nil {
		return err
	}
	for _, inv := range pending.Invites {
		if inv.ID == 0 || inv.Accepted || inv.Expires <= time.Now().Unix() || inv.From != j.BotUID || inv.GCID != j.GCID {
			continue
		}
		// Recheck the current configuration/identity after the asynchronous reads.
		if err := m.current(ctx, j); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return m.accept(ctx, inv.ID)
	}
	return nil // Delayed delivery is normal; keep waiting for a matching invite.
}
func BeginCommunityJoin(ctx context.Context, restart bool) (*CommunityJoin, error) {
	return communityJoins.begin(ctx, restart)
}
func CommunityJoinStatus(ctx context.Context) (*CommunityJoin, error) {
	return communityJoins.status(ctx)
}
func AcceptCommunityJoin(ctx context.Context, id string) error {
	return communityJoins.acceptMatching(ctx, id)
}

// botURLAuthenticated reports whether a reply from base comes from the host it
// names: TLS, an onion address, or this machine.
func botURLAuthenticated(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme == "https" || strings.HasSuffix(host, ".onion") || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
