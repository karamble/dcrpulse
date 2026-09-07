// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// A tool description is built when an agent's scoped server is assembled, which
// happens on the request path, so the wallet is read at most once per TTL and
// every caller reuses the result. A wallet that is down yields an empty profile
// and the tools fall back to describing their fields plainly.
const stakingProfileTTL = 30 * time.Minute

// fetchStakingProfile is the wallet read behind the hints; a test parks it to
// stand in for a wedged wallet.
var fetchStakingProfile = services.GetStakingProfile

var profileCache struct {
	mu   sync.Mutex
	at   time.Time
	prof types.StakingProfile
	have bool
}

func cachedStakingProfile() types.StakingProfile {
	profileCache.mu.Lock()
	defer profileCache.mu.Unlock()
	if profileCache.have && time.Since(profileCache.at) < stakingProfileTTL {
		return profileCache.prof
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profileCache.prof = fetchStakingProfile(ctx)
	profileCache.at = time.Now()
	profileCache.have = true
	return profileCache.prof
}

// InvalidateStakingProfile drops the cached wallet profile so the next tool
// listing rebuilds it. Called when the active wallet changes, since the accounts
// and tickets it describes belong to that wallet.
func InvalidateStakingProfile() {
	profileCache.mu.Lock()
	profileCache.have = false
	profileCache.mu.Unlock()
}

func accountList(nums []uint32) string {
	if len(nums) == 0 {
		return ""
	}
	sorted := append([]uint32(nil), nums...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, len(sorted))
	for i, n := range sorted {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ", ")
}

// grantedAccounts reports the accounts this agent's grant covers, for a tool
// that takes an account number.
func grantedAccounts(a *agent) string {
	info, ok := SpendGrantInfo(a.id)
	if !ok || len(info.Accounts) == 0 {
		return "No spend grant covers an account yet, so any account will be refused."
	}
	noun := "account"
	if len(info.Accounts) > 1 {
		noun = "accounts"
	}
	return "Your grant covers " + noun + " " + accountList(info.Accounts) + "."
}

// stakingHint describes the accounts and VSPs this wallet actually stakes with,
// so the caller can fill the fields correctly on the first attempt.
func stakingHint(a *agent) string {
	p := cachedStakingProfile()
	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(grantedAccounts(a))
	if len(p.Accounts) > 0 {
		noun := "account"
		if len(p.Accounts) > 1 {
			noun = "accounts"
		}
		fmt.Fprintf(&b, " This wallet already holds ticket value in %s %s.", noun, accountList(p.Accounts))
	}
	if p.MixedAccount != nil && p.ChangeAccount != nil {
		fmt.Fprintf(&b, " Privacy is configured, so a purchase is funded from account %d and its change goes to %d whatever this call asks for - both must be covered by the grant.",
			*p.MixedAccount, *p.ChangeAccount)
	}
	if len(p.VSPs) > 0 {
		top := p.VSPs[0]
		fmt.Fprintf(&b, " Most of this wallet's tickets (%d) are registered with %s.", top.Tickets, top.Host)
	}
	return b.String()
}
