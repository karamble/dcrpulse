// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
	"dcrpulse/internal/types"

	"github.com/gorilla/websocket"
)

// LightningStatusHandler — high-level stage the UI uses to choose
// between wizard, unlock screen, and Overview tab.
func LightningStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status := services.LightningStatus(ctx)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// LightningSetupHandler — creates the dedicated lightning dcrwallet
// account, unblocks the dcrlnd container, and runs the first-time
// InitWallet on dcrlnd. Used once per wallet lifetime.
func LightningSetupHandler(w http.ResponseWriter, r *http.Request) {
	if ready, reason := services.WalletReady(r.Context()); !ready {
		http.Error(w, reason, http.StatusServiceUnavailable)
		return
	}
	var req types.LightningSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) < 8 {
		http.Error(w, "passphrase must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) > 1024 {
		http.Error(w, "passphrase too long", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	setupCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if _, err := services.SetupLightningAccount(setupCtx, passphrase); err != nil {
		lightningWriteErr(w, "SetupLightningAccount", err)
		return
	}

	// dcrlnd entrypoint may take a few seconds to notice the sentinel,
	// start the daemon, and begin listening. Once it is up, InitWallet
	// responds promptly — short retry window is enough.
	// ReinitDcrlndClient inside the service rebuilds the gRPC client
	// when the dcrlnd TLS cert first appears.
	initCtx, cancel2 := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel2()
	var lastErr error
	for i := 0; i < 8; i++ {
		if err := services.InitLightningWallet(initCtx, passphrase); err != nil {
			lastErr = err
			lower := strings.ToLower(err.Error())
			// Wallet already initialised: treat as success on a wizard
			// re-run after a dashboard restart with the dcrlnd volume
			// intact.
			if strings.Contains(lower, "already exists") {
				lastErr = nil
				break
			}
			// dcrlnd's own length check — should never trigger because
			// we gate it above, but surface clearly if it does.
			if strings.Contains(lower, "at least 8 characters") {
				break
			}
			time.Sleep(2 * time.Second)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		lightningWriteErr(w, "InitLightningWallet", lastErr)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LightningUnlockHandler — UnlockWallet on subsequent starts when the
// dcrlnd wallet is already initialised.
func LightningUnlockHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningUnlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Passphrase == "" {
		http.Error(w, "passphrase required", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) < 8 {
		http.Error(w, "passphrase must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if len(req.Passphrase) > 1024 {
		http.Error(w, "passphrase too long", http.StatusBadRequest)
		return
	}
	passphrase := []byte(req.Passphrase)
	req.Passphrase = ""
	defer func() {
		for i := range passphrase {
			passphrase[i] = 0
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := services.UnlockLightningWallet(ctx, passphrase); err != nil {
		lightningWriteErr(w, "UnlockLightningWallet", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LightningInfoHandler — GetInfo proxy for the Overview tab.
func LightningInfoHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	info, err := services.GetLightningInfo(ctx)
	if err != nil {
		lightningWriteErr(w, "GetLightningInfo", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

// LightningBalanceHandler — merged wallet + channel balance for the
// 6-card Overview grid.
func LightningBalanceHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	bal, err := services.GetLightningBalance(ctx)
	if err != nil {
		lightningWriteErr(w, "GetLightningBalance", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(bal)
}

// LightningActivityHandler — recent invoices + payments merged into
// one feed for the Overview tab's activity list.
func LightningActivityHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	act, err := services.GetLightningActivity(ctx)
	if err != nil {
		lightningWriteErr(w, "GetLightningActivity", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(act)
}

func lightningWriteErr(w http.ResponseWriter, label string, err error) {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "passphrase"), strings.Contains(lower, "decrypt"):
		http.Error(w, "Wrong passphrase", http.StatusUnauthorized)
	case strings.Contains(lower, "not available"), strings.Contains(lower, "unreachable"),
		strings.Contains(lower, "unavailable"):
		http.Error(w, "Lightning daemon not available", http.StatusServiceUnavailable)
	default:
		log.Printf("%s failed: %v", label, err)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

// ---- Channels (Phase 4: Decrediton parity) --------------------------------

// LightningChannelsHandler — merged list of open/pending/closed channels.
func LightningChannelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp, err := services.ListLightningChannels(ctx)
	if err != nil {
		lightningWriteErr(w, "ListLightningChannels", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningOpenChannelHandler — ConnectPeer + OpenChannelSync.
func LightningOpenChannelHandler(w http.ResponseWriter, r *http.Request) {
	var req types.OpenChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.PeerURI == "" {
		http.Error(w, "peerUri required", http.StatusBadRequest)
		return
	}
	if req.LocalAtoms <= 0 {
		http.Error(w, "localAtoms must be positive", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	resp, err := services.OpenLightningChannel(ctx, &req)
	if err != nil {
		lightningWriteErr(w, "OpenLightningChannel", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningCloseChannelHandler — streaming CloseChannel, returns when
// closePending is received.
func LightningCloseChannelHandler(w http.ResponseWriter, r *http.Request) {
	var req types.CloseChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ChannelPoint == "" {
		http.Error(w, "channelPoint required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	resp, err := services.CloseLightningChannel(ctx, req.ChannelPoint, req.Force)
	if err != nil {
		lightningWriteErr(w, "CloseLightningChannel", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningPeerPresetsHandler — cached brseeder list (always non-empty
// thanks to the hardcoded hub0 fallback).
func LightningPeerPresetsHandler(w http.ResponseWriter, r *http.Request) {
	presets := services.LightningPeerPresets(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"presets": presets})
}

// LightningAutopilotStatusHandler — current autopilot active flag.
func LightningAutopilotStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resp, err := services.GetLightningAutopilotStatus(ctx)
	if err != nil {
		lightningWriteErr(w, "GetLightningAutopilotStatus", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningAutopilotSetHandler — toggle autopilot.
func LightningAutopilotSetHandler(w http.ResponseWriter, r *http.Request) {
	var req types.AutopilotStatus
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := services.SetLightningAutopilotStatus(ctx, req.Active); err != nil {
		lightningWriteErr(w, "SetLightningAutopilotStatus", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LightningGraphSearchHandler — substring search of DescribeGraph nodes.
func LightningGraphSearchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := services.SearchLightningNodes(ctx, q)
	if err != nil {
		lightningWriteErr(w, "SearchLightningNodes", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningChannelEventsHandler — WebSocket; fans out dcrlnd's
// SubscribeChannelEvents stream. Origin-checked via the existing
// middleware.SameOriginWS upgrader.
func LightningChannelEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("LightningChannelEventsHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	events, err := services.SubscribeLightningChannelEvents(ctx)
	if err != nil {
		log.Printf("SubscribeLightningChannelEvents: %v", err)
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}

	// Reader goroutine — close the stream if the client disconnects.
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	for ev := range events {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}
}

// LightningNetworkHandler — global network statistics for the Overview
// tab's "Network statistics" section. Returns the GetNetworkInfo
// aggregate + top-10 nodes by capacity. Best-effort on top-nodes —
// network info is essential, the top list can be empty if the graph
// walk fails.
func LightningNetworkHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	info, err := services.GetLightningNetworkInfo(ctx)
	if err != nil {
		lightningWriteErr(w, "GetLightningNetworkInfo", err)
		return
	}

	out := types.LightningNetworkPanel{
		Info:     *info,
		TopNodes: []types.TopLightningNode{},
	}
	if top, terr := services.GetTopLightningNodes(ctx, 10); terr == nil {
		out.TopNodes = top
	} else {
		log.Printf("GetTopLightningNodes (best-effort): %v", terr)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// ---- Send tab --------------------------------------------------------------

// LightningDecodePayReqHandler validates a BOLT-11 invoice and returns
// its decoded fields for the Send tab's preview. Mirrors Decrediton's
// decodePayRequest action.
func LightningDecodePayReqHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PayReq string `json:"payReq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PayReq) == "" {
		http.Error(w, "payReq required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := services.DecodeLightningInvoice(ctx, strings.TrimSpace(req.PayReq))
	if err != nil {
		lightningWriteErr(w, "DecodeLightningInvoice", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningSendPaymentHandler is a WebSocket endpoint that forwards
// Router.SendPaymentV2 snapshots to the browser. Mirrors Decrediton's
// handlePaymentStream (LNActions.js:697-732): the user sees
// IN_FLIGHT -> SUCCEEDED|FAILED transitions live.
//
// Protocol: client sends a single text frame with the
// LightningSendPaymentRequest JSON, then receives LightningPayment
// snapshots until the server closes. On non-transport errors a final
// JSON frame `{"error": "..."}` is written before the socket closes.
func LightningSendPaymentHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("LightningSendPaymentHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	// Read the first text frame as the send request.
	_ = conn.SetReadDeadline(timeFrom(r.Context(), 30*time.Second))
	mt, raw, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": "no request received"})
		return
	}
	if mt != websocket.TextMessage {
		_ = conn.WriteJSON(map[string]string{"error": "request must be a text frame"})
		return
	}
	var req types.LightningSendPaymentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = conn.WriteJSON(map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.PayReq) == "" {
		_ = conn.WriteJSON(map[string]string{"error": "payReq required"})
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Reader goroutine: bail out if the client disconnects (closes tab,
	// navigates away). Cancels the gRPC stream context so dcrlnd stops
	// processing.
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	snaps, err := services.StreamLightningPayment(ctx, &req)
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	for snap := range snaps {
		if err := conn.WriteJSON(snap); err != nil {
			return
		}
	}
}

// timeFrom returns ctx's deadline if it is sooner than now+d, otherwise
// now+d. Avoids hanging reads when the parent request is closing.
func timeFrom(ctx context.Context, d time.Duration) time.Time {
	deadline := time.Now().Add(d)
	if cd, ok := ctx.Deadline(); ok && cd.Before(deadline) {
		return cd
	}
	return deadline
}

// LightningPaymentsHandler returns the wallet's payment history for the
// Send tab's lower list. Mirrors Decrediton's listLatestPayments.
func LightningPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp, err := services.ListLightningPayments(ctx)
	if err != nil {
		lightningWriteErr(w, "ListLightningPayments", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---- Receive tab -----------------------------------------------------------

// LightningAddInvoiceHandler mints a new invoice via lnrpc.AddInvoice and
// returns the canonical record.
func LightningAddInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningAddInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ValueAtoms < 0 {
		http.Error(w, "valueAtoms must be >= 0", http.StatusBadRequest)
		return
	}
	if len(req.Memo) > 639 {
		http.Error(w, "memo too long (max 639 chars)", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	inv, err := services.AddLightningInvoice(ctx, &req)
	if err != nil {
		lightningWriteErr(w, "AddLightningInvoice", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(inv)
}

// LightningInvoicesHandler returns the wallet's invoice history for the
// Receive tab's lower list.
func LightningInvoicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp, err := services.ListLightningInvoices(ctx)
	if err != nil {
		lightningWriteErr(w, "ListLightningInvoices", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningInvoiceEventsHandler is a WebSocket endpoint that forwards
// SubscribeInvoices snapshots so the Receive tab updates live as
// invoices settle, expire, or are canceled.
func LightningInvoiceEventsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: middleware.SameOriginWS,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("LightningInvoiceEventsHandler upgrade: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Reader goroutine: bail out if the client disconnects.
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				cancel()
				return
			}
		}
	}()

	events, err := services.StreamLightningInvoiceEvents(ctx)
	if err != nil {
		_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	for ev := range events {
		if err := conn.WriteJSON(ev); err != nil {
			return
		}
	}
}

// LightningCancelInvoiceHandler cancels an OPEN invoice.
func LightningCancelInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningCancelInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PaymentHash) == "" {
		http.Error(w, "paymentHash required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := services.CancelLightningInvoice(ctx, req.PaymentHash); err != nil {
		lightningWriteErr(w, "CancelLightningInvoice", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Advanced tab ---------------------------------------------------------

// LightningBackupExportHandler returns the latest Static Channel Backup
// as a base64 blob plus the channel count. The frontend turns this into
// a browser file download.
func LightningBackupExportHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := services.ExportLightningChannelBackup(ctx)
	if err != nil {
		lightningWriteErr(w, "ExportLightningChannelBackup", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// LightningBackupVerifyHandler validates a user-uploaded backup blob.
func LightningBackupVerifyHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningVerifyBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.BackupBase64) == "" {
		http.Error(w, "backupBase64 required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp := services.VerifyLightningChannelBackup(ctx, req.BackupBase64)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningWatchtowersHandler lists registered watchtowers.
func LightningWatchtowersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := services.ListLightningWatchtowers(ctx)
	if err != nil {
		lightningWriteErr(w, "ListLightningWatchtowers", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LightningWatchtowerAddHandler registers a new watchtower.
func LightningWatchtowerAddHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningAddTowerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PubKeyHex) == "" || strings.TrimSpace(req.Address) == "" {
		http.Error(w, "pubKeyHex and address required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := services.AddLightningWatchtower(ctx, req.PubKeyHex, req.Address); err != nil {
		lightningWriteErr(w, "AddLightningWatchtower", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LightningWatchtowerRemoveHandler deregisters a watchtower.
func LightningWatchtowerRemoveHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningRemoveTowerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PubKeyHex) == "" {
		http.Error(w, "pubKeyHex required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := services.RemoveLightningWatchtower(ctx, req.PubKeyHex); err != nil {
		lightningWriteErr(w, "RemoveLightningWatchtower", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LightningGraphNodeHandler queries one node from dcrlnd's channel graph.
func LightningGraphNodeHandler(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.TrimSpace(r.URL.Query().Get("pubkey"))
	if pubkey == "" {
		http.Error(w, "pubkey query param required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := services.QueryLightningNodeInfo(ctx, pubkey)
	if err != nil {
		lightningWriteErr(w, "QueryLightningNodeInfo", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// LightningGraphRoutesHandler queries candidate payment routes.
func LightningGraphRoutesHandler(w http.ResponseWriter, r *http.Request) {
	var req types.LightningQueryRoutesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.PubKey) == "" || req.AmtAtoms <= 0 {
		http.Error(w, "pubKey + positive amtAtoms required", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := services.QueryLightningRoutes(ctx, req.PubKey, req.AmtAtoms)
	if err != nil {
		lightningWriteErr(w, "QueryLightningRoutes", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
