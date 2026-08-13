// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/gorilla/websocket"

	"dcrpulse/internal/auth"
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
	PerTableCapDcr      float64  `json:"perTableCapDcr"`
	PerDayCapDcr        float64  `json:"perDayCapDcr"`
	ApprovalTimeoutSecs int      `json:"approvalTimeoutSecs"`
	InstalledGames      []string `json:"installedGames"`

	// GameNames is the label the operator gave each game, for the interface
	// to show. Decoration: nothing routes or authorises by it.
	GameNames map[string]string `json:"gameNames"`

	// GameTokens is what a game authenticates with. It is shown so the user
	// can copy it into a game's configuration; it is the game's identity,
	// so anything holding it is that game as far as this host is concerned.
	//
	// It travels outward only. A token is issued here and never accepted
	// back, because a game that could name its own identity could name
	// another game's.
	GameTokens map[string]string `json:"gameTokens"`
}

func gamingToView(s types.GamingSettings) gamingSettingsView {
	if s.InstalledGames == nil {
		s.InstalledGames = []string{}
	}
	if s.GameTokens == nil {
		s.GameTokens = map[string]string{}
	}
	if s.GameNames == nil {
		s.GameNames = map[string]string{}
	}
	return gamingSettingsView{
		GameTokens:          s.GameTokens,
		GameNames:           s.GameNames,
		Enabled:             s.Enabled,
		Account:             s.Account,
		PerTableCapDcr:      dcrutil.Amount(s.PerTableCapAtoms).ToCoin(),
		PerDayCapDcr:        dcrutil.Amount(s.PerDayCapAtoms).ToCoin(),
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
		PerTableCapAtoms:    int64(perTable),
		PerDayCapAtoms:      int64(perDay),
		ApprovalTimeoutSecs: v.ApprovalTimeoutSecs,
		InstalledGames:      v.InstalledGames,
		GameNames:           v.GameNames,
		// GameTokens is deliberately not read back: a token is issued
		// here, and a caller that could set one could name another
		// game's identity.
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
		// The bridge is only ever live behind the App Password: the caps
		// below and the approval a spend waits on are worth nothing if the
		// person approving cannot be told from anybody who reached the port.
		saved, err := services.WriteGamingSettings(next, auth.Enabled())
		switch {
		case err == nil:
		case errors.Is(err, services.ErrGamingNeedsAppPassword):
			http.Error(w, err.Error(), http.StatusConflict)
			return
		case errors.Is(err, services.ErrGamingBadGameID):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
			http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gamingJSON(w, gamingToView(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// BisonrelayGamingGamesHandler lists the games the operator registered.
func BisonrelayGamingGamesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"games": services.GamingGames()})
}

// BisonrelayGamingSpendsHandler lists what games have asked to spend, and what
// was decided, for a person to read.
func BisonrelayGamingSpendsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	gamingJSON(w, map[string]any{"spends": services.GamingSpends()})
}

// BisonrelayGamingSpendDecideHandler is a person answering a game's request.
//
// Approving takes the passphrase, because that is the only thing that can move
// money here and it belongs to them. What is paid is the amount and address
// recorded when the request was made, never anything passed in now - so what
// is signed is what was shown.
func BisonrelayGamingSpendDecideHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID         string `json:"id"`
		Approve    bool   `json:"approve"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""

	var (
		spend services.GamingSpend
		err   error
	)
	if req.Approve {
		if len(passphrase) == 0 {
			http.Error(w, "passphrase is required to approve a spend", http.StatusBadRequest)
			return
		}
		spend, err = services.ApproveGamingSpend(r.Context(), strings.TrimSpace(req.ID), passphrase)
	} else {
		spend, err = services.DenyGamingSpend(strings.TrimSpace(req.ID))
	}

	switch {
	case err == nil:
		gamingJSON(w, spend)
	case errors.Is(err, services.ErrGamingSpendNotFound):
		http.Error(w, "no such spend request", http.StatusNotFound)
	case errors.Is(err, services.ErrGamingSpendNotPending):
		http.Error(w, "that request was already decided", http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

// A game's seed is deliberately not reachable from here. It is the one secret
// this host does not hold a copy of, and a game now runs on a machine of the
// person's choosing rather than in a volume beside the wallet - so fetching it
// would carry it across a network to a page a browser session can read, to
// solve a problem the person is already standing in front of. The game offers
// its own backup; this only reports whether they have taken it.

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

// BisonrelayGamingChainTipHandler reports where the chain is.
//
// A game needs this to agree a deadline with its peers: a height is a fact
// everyone can check, where a clock is each machine's opinion and nobody can
// be shown to have read it wrong.
func BisonrelayGamingChainTipHandler(w http.ResponseWriter, r *http.Request) {
	tip, err := services.GamingChainTipNow(r.Context())
	if err != nil {
		gamingChainError(w, err)
		return
	}
	gamingJSON(w, tip)
}

// BisonrelayGamingBlockHashHandler reports the hash of a block.
//
// A table seats itself from one, so it asks by height rather than for the tip:
// the tip moves, and two peers reading it a second apart would seat the same
// table differently.
func BisonrelayGamingBlockHashHandler(w http.ResponseWriter, r *http.Request) {
	height, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("height")), 10, 64)
	if err != nil || height < 0 {
		http.Error(w, "height must be a block number", http.StatusBadRequest)
		return
	}
	hash, err := services.GamingBlockHash(r.Context(), height)
	if err != nil {
		gamingChainError(w, err)
		return
	}
	gamingJSON(w, map[string]any{"height": height, "hash": hash})
}

// BisonrelayGamingOutpointHandler reports what is at an outpoint.
//
// This is how a game checks somebody else's bond for itself. The host says what
// the chain contains; what that means for a table is the game's own business,
// and a host that decided it would be a party nobody agreed to trust.
func BisonrelayGamingOutpointHandler(w http.ResponseWriter, r *http.Request) {
	txid := strings.TrimSpace(r.URL.Query().Get("txid"))
	if !gamingTxidRe.MatchString(txid) {
		http.Error(w, "txid must be 64 hex characters", http.StatusBadRequest)
		return
	}
	vout, err := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("vout")), 10, 32)
	if err != nil {
		http.Error(w, "vout must be a number", http.StatusBadRequest)
		return
	}

	// Confirmed only unless asked otherwise: staking against somebody else's
	// deposit needs a buried one, but locating an output of a payment just
	// made needs the mempool, because it is in no block yet.
	includeMempool := r.URL.Query().Get("mempool") == "1"

	out, err := services.GamingChainOutpoint(r.Context(), txid, uint32(vout), includeMempool)
	if err != nil {
		gamingChainError(w, err)
		return
	}
	gamingJSON(w, out)
}

var gamingTxidRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// gamingChainError separates "the node is not there" from "the question was
// wrong", because a game can wait out the first and never the second.
func gamingChainError(w http.ResponseWriter, err error) {
	if errors.Is(err, services.ErrGamingChainUnavailable) {
		http.Error(w, "chain is not available", http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

// ---- Spending ----
//
// The one place a game can move money, and the one place the bridge forms an
// opinion about anything. Everywhere else it carries frames whole; here it
// decides which account, how much, how often, and whether a person said yes.
// Policy belongs where money moves, not where messages do.

// BisonrelayGamingBroadcastHandler relays a transaction a game signed itself.
//
// The other money route, /spend, has the host build the transaction and a
// person approve it. That covers paying money in and cannot cover taking it
// back out: a bond, or a stake behind its refund timelock, sits under a script
// only the game holds the key for. The game signs it; this puts the bytes on
// the network.
//
// Nobody is asked, because there is nobody to ask about - the coin is already
// the game's to move and the wallet's passphrase has nothing to do with it. So
// this is bounded instead of approved, and what bounds it (services.
// GamingBroadcast) is about shape rather than intent: every input a script hash
// the wallet does not own, every output paying an address it does. The host is
// not going to be taught what a table is.
func BisonrelayGamingBroadcastHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)

	var req struct {
		RawTxHex string `json:"rawTxHex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	txid, err := services.GamingBroadcast(r.Context(), game, req.RawTxHex)
	if err != nil {
		// The refusal is the game's to read and act on, so it says why.
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	gamingJSON(w, map[string]any{"txid": txid})
}

// BisonrelayGamingSpendHandler records a game's request to spend, or refuses it.
//
// It never spends. The dashboard holds no wallet passphrase - every send route
// takes one from the user and wipes it - so all this can do is ask a person.
func BisonrelayGamingSpendHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)

	var req struct {
		Address     string `json:"address"`
		AmountAtoms int64  `json:"amountAtoms"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}

	spend, err := services.RequestGamingSpend(game, req.Address, req.AmountAtoms, req.Reason)
	switch {
	case err == nil:
		gamingJSON(w, spend)
	case errors.Is(err, services.ErrGamingSpendRefused):
		// The reason matters: a game told only "no" cannot tell a cap
		// it will never get under from one it could wait out.
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, services.ErrGamingGameNotInstalled):
		http.Error(w, "game is not installed", http.StatusForbidden)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// BisonrelayGamingSpendStatusHandler reports what became of a request.
//
// Scoped to the game that made it: what another game has been spending is a
// question this one has no business having answered.
func BisonrelayGamingSpendStatusHandler(w http.ResponseWriter, r *http.Request) {
	game := gamingCaller(r)

	spend, err := services.GamingSpendFor(game, strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil {
		http.Error(w, "no such spend request", http.StatusNotFound)
		return
	}
	gamingJSON(w, spend)
}

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
