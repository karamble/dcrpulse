package handlers

import (
	"dcrpulse/internal/services"
	"encoding/json"
	"net/http"
)

func BisonrelayGamingPayoutsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		payouts, err := services.GamingPayouts(r.Context())
		if err != nil {
			http.Error(w, "Payout ledger unavailable", http.StatusServiceUnavailable)
			return
		}
		gamingJSON(w, map[string]any{"payouts": payouts})
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID         string `json:"id"`
		Action     string `json:"action"`
		Passphrase string `json:"passphrase"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.ID == "" || (req.Action != "approve" && req.Action != "reject") {
		http.Error(w, "Invalid payout decision", http.StatusBadRequest)
		return
	}
	var (
		reply any
		err   error
	)
	if req.Action == "reject" {
		reply, err = services.RejectGamingPayout(r.Context(), req.ID)
	} else {
		reply, err = services.ApproveGamingPayout(r.Context(), req.ID, []byte(req.Passphrase))
	}
	req.Passphrase = ""
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	gamingJSON(w, reply)
}
