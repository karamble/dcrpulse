// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"dcrpulse/internal/rpc"
)

// Telling a game what people are called.
//
// A game's page shows seats, and a seat is a throwaway session key - a number,
// deliberately. The people holding those keys arrived through a group chat the
// host can see, and only the host can see it: the game reaches an allowlist of
// its own routes and cannot ask Bison Relay anything. So the host resolves the
// names and pushes them down, and the game may use them for exactly one thing,
// which is labelling chairs.
//
// This is a deliberate, narrow widening of what the game is told. It already
// sees the members' identities - every frame arrives with its sender - so what
// crosses here is only what those identities are called by this user's own
// contact list. Display strings, pushed at panel mint, never identity.

// PushGamingNames resolves the members of every conversation the game is
// playing in and tells it what they are called. Best-effort by design: a table
// with unnamed seats is a table with numbered seats, not a fault.
func PushGamingNames(ctx context.Context, game string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// The conversations, from the game's own tables. The host asking the
	// game what it is playing in is the ordinary direction of trust.
	body, err := gamingCall(ctx, game, http.MethodGet, "/tables", nil, 15*time.Second)
	if err != nil {
		return err
	}
	var tables struct {
		Tables []struct {
			GCID string `json:"gcid"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body, &tables); err != nil {
		return err
	}
	gcids := map[string]bool{}
	for _, t := range tables.Tables {
		if t.GCID != "" {
			gcids[t.GCID] = true
		}
	}
	if len(gcids) == 0 {
		return nil
	}

	// Who is in them.
	members := map[string]bool{}
	for gcid := range gcids {
		detail, err := rpc.BrclientdGCDetail(ctx, gcid)
		if err != nil {
			// One unreadable conversation must not unname the rest.
			log.Printf("gaming names: gc %s: %v", gcid, err)
			continue
		}
		var gc struct {
			Members []string `json:"members"`
		}
		if err := json.Unmarshal(detail, &gc); err != nil {
			continue
		}
		for _, uid := range gc.Members {
			members[uid] = true
		}
	}
	if len(members) == 0 {
		return nil
	}

	// What they are called, by this user's own contact list. An alias the
	// user chose beats the nick the contact chose, because the whole point
	// is showing the user the name they know.
	raw, err := rpc.BrclientdContacts(ctx)
	if err != nil {
		return err
	}
	var contacts struct {
		Entries []struct {
			ID struct {
				Identity string `json:"identity"`
				Nick     string `json:"nick"`
			} `json:"id"`
			NickAlias string `json:"nick_alias"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &contacts); err != nil {
		return err
	}
	names := map[string]string{}
	for _, c := range contacts.Entries {
		if !members[c.ID.Identity] {
			continue
		}
		name := c.ID.Nick
		if c.NickAlias != "" {
			name = c.NickAlias
		}
		if name != "" {
			names[c.ID.Identity] = name
		}
	}
	if len(names) == 0 {
		return nil
	}

	blob, err := json.Marshal(map[string]any{"names": names})
	if err != nil {
		return err
	}
	_, err = gamingCall(ctx, game, http.MethodPost, "/names/set", blob, 15*time.Second)
	return err
}
