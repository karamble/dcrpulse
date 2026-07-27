// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/websocket"

	"dcrpulse/internal/services"
	"dcrpulse/internal/types"
)

// The Bison Relay gaming section's confinement policy (Bison Relay > Gaming).
// Games are untrusted plugins running beside a wallet, dcrlnd and a BR
// identity, so they never hold wallet credentials: they reach one account,
// under caps, through this policy. Storage speaks atoms; the frontend speaks
// DCR, converted here the same way the BR-MCP handlers do.

// gamingSettingsView is the DCR-denominated frontend shape.
type gamingSettingsView struct {
	Enabled             bool     `json:"enabled"`
	Account             string   `json:"account"`
	Mode                string   `json:"mode"`
	PerTableCapDcr      float64  `json:"perTableCapDcr"`
	PerDayCapDcr        float64  `json:"perDayCapDcr"`
	MaxOpenTables       int      `json:"maxOpenTables"`
	ApprovalTimeoutSecs int      `json:"approvalTimeoutSecs"`
	InstalledGames      []string `json:"installedGames"`

	// GameTokens is what a game authenticates with. It is shown so the user
	// can copy it into a game's configuration; it is the game's identity,
	// so anything holding it is that game as far as this host is concerned.
	GameTokens map[string]string `json:"gameTokens"`
}

func gamingToView(s types.GamingSettings) gamingSettingsView {
	if s.InstalledGames == nil {
		s.InstalledGames = []string{}
	}
	if s.GameTokens == nil {
		s.GameTokens = map[string]string{}
	}
	return gamingSettingsView{
		GameTokens:          s.GameTokens,
		Enabled:             s.Enabled,
		Account:             s.Account,
		Mode:                s.Mode,
		PerTableCapDcr:      dcrutil.Amount(s.PerTableCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(s.PerDayCapAtoms).ToCoin(),
		MaxOpenTables:       s.MaxOpenTables,
		ApprovalTimeoutSecs: s.ApprovalTimeoutSecs,
		InstalledGames:      s.InstalledGames,
	}
}

func gamingFromView(v gamingSettingsView) (types.GamingSettings, error) {
	perTable, err := dcrutil.NewAmount(v.PerTableCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	perDay, err := dcrutil.NewAmount(v.PerDayCapDcr)
	if err != nil {
		return types.GamingSettings{}, err
	}
	return types.GamingSettings{
		Enabled:             v.Enabled,
		Account:             v.Account,
		Mode:                v.Mode,
		PerTableCapAtoms:    int64(perTable),
		PerDayCapAtoms:      int64(perDay),
		MaxOpenTables:       v.MaxOpenTables,
		ApprovalTimeoutSecs: v.ApprovalTimeoutSecs,
		InstalledGames:      v.InstalledGames,
	}, nil
}

func gamingJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// BisonrelayGamingSettingsHandler round-trips the gaming confinement policy.
func BisonrelayGamingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		gamingJSON(w, gamingToView(services.ReadGamingSettings()))
	case http.MethodPost:
		var in gamingSettingsView
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		next, err := gamingFromView(in)
		if err != nil {
			http.Error(w, "bad amount: "+err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := services.WriteGamingSettings(next)
		if err != nil {
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gamingJSON(w, gamingToView(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayGamingGamesHandler lists the games this build can route, marked
// with whether the user installed them.
func BisonrelayGamingGamesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"games": services.GamingCatalogue()})
}

// ---- The tunnel ----
//
// A game sends and receives its own protocol frames through these two routes.
// The host carries them and reads only the routing key: what a frame means is
// between the players, who sign their own traffic and check each other's.
//
// A game authenticates with a bearer token, and that token is its identity.
// It never states which game it is - it presents a token and the host decides -
// so a game cannot send another game's traffic even by asking to. The same
// identity is what later spend limits attach to; a shared secret would make
// every game one principal, and a cap on a principal nobody can tell apart is
// not a cap.
//
// These routes sit outside the browser API on purpose. That API is guarded by
// same-origin and a dashboard session, neither of which a separate process has,
// and same-origin is a defence against a browser being tricked into using
// someone's cookies - which has no bearing on a caller that presents a token.

// gamingTunnelGameKey carries the authenticated game down to the handlers.
type gamingTunnelGameKey struct{}

var gamingGCIDRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// GamingTunnelAuth resolves the bearer token to the game it identifies.
//
// A gaming section that is switched off answers as though the routes are not
// there, rather than admitting they exist and refusing - the same way the
// BR-MCP bridge does. There is nothing to authenticate against when no game is
// installed, and saying so would only describe the host to a stranger.
func GamingTunnelAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		game, ok := services.GamingGameForToken(strings.TrimSpace(token))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gamingTunnelGameKey{}, game)))
	})
}

// gamingCaller returns the game the request authenticated as.
func gamingCaller(r *http.Request) string {
	game, _ := r.Context().Value(gamingTunnelGameKey{}).(string)
	return game
}

// BisonrelayGamingSendHandler sends one frame to a table's group chat.
func BisonrelayGamingSendHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)

	var req struct {
		GCID  string `json:"gcid"`
		Frame string `json:"frame"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !gamingGCIDRe.MatchString(req.GCID) {
		http.Error(w, "gcid must be 64 hex characters", http.StatusBadRequest)
		return
	}
	if req.Frame == "" {
		http.Error(w, "frame is required", http.StatusBadRequest)
		return
	}

	switch err := services.SendGamingFrame(r.Context(), game, req.GCID, req.Frame); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, services.ErrGamingGameNotInstalled),
		errors.Is(err, services.ErrGamingNotAFrame),
		errors.Is(err, services.ErrGamingWrongGame):
		// A game is told it was refused, and nothing about what else the
		// host is carrying.
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, "send frame: "+err.Error(), http.StatusBadGateway)
	}
}

// BisonrelayGamingEventsHandler streams the calling game's inbound frames.
//
// The subscription follows the token, so a game receives its own traffic and no
// more. That is containment rather than secrecy - frames cross a group chat any
// member can read - but nothing is served to a game it was not addressed to.
func BisonrelayGamingEventsHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)

	// The bearer token is what authenticates here, so an origin check would
	// add nothing: it guards against a browser being made to spend cookies
	// it already holds, and there are no cookies in play.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("BisonrelayGamingEventsHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	frames, unsubscribe := services.Gaming().Subscribe(game, 64)
	defer unsubscribe()

	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	pinger := time.NewTicker(30 * time.Second)
	defer pinger.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pinger.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case ev, ok := <-frames:
			if !ok {
				return
			}
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}
}

// BisonrelayGamingBundleHandler serves a game's binary, or its signature, to
// the sandbox.
//
// The sandbox has no route off the host, so it cannot fetch its own binaries.
// The host does the fetching, from a URL in its own catalogue rather than one
// the caller supplies - a game able to name a URL could ask the host to reach
// anything reachable from here, which is exactly what the sandbox gives up.
//
// The bytes are passed through unverified. The portal checks the signature,
// because the portal is what executes them.
func BisonrelayGamingBundleHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)
	signature := r.URL.Query().Get("part") == "sig"
	arch := r.URL.Query().Get("arch")

	body, err := services.FetchGamingBundle(r.Context(), game, arch, signature)
	if err != nil {
		if errors.Is(err, services.ErrGamingGameNotInstalled) {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.Error(w, "fetch bundle: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := io.Copy(w, body); err != nil {
		// The response is already streaming, so there is no status left
		// to change; the portal sees a short read and refuses it.
		log.Printf("gaming bundle for %s: %v", game, err)
	}
}
