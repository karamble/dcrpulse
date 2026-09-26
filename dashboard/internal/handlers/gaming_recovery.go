package handlers

import (
	"encoding/json"
	"net/http"

	"dcrpulse/internal/services"
)

func BisonrelayGamingRecoveryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := services.GamingRecoveryList(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		gamingJSON(w, map[string]any{"deposits": rows})
		return
	}
	var req struct {
		Action     string `json:"action"`
		ID         string `json:"id"`
		Quote      string `json:"quote"`
		Passphrase string `json:"passphrase"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "invalid recovery request", http.StatusBadRequest)
		return
	}
	switch req.Action {
	case "close":
		if err := services.CloseGamingRecoveryTable(r.Context(), req.ID); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		gamingJSON(w, map[string]bool{"closed": true})
	case "quote":
		q, err := services.QuoteGamingRecovery(r.Context(), req.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		gamingJSON(w, q)
	case "confirm":
		if req.Quote == "" {
			http.Error(w, "quote required", http.StatusBadRequest)
			return
		}
		id, err := services.ConfirmGamingRecovery(r.Context(), req.ID, req.Quote, []byte(req.Passphrase))
		if err != nil {
			gamingJSON(w, map[string]any{"txid": id, "pending": id != "", "error": err.Error()})
			return
		}
		gamingJSON(w, map[string]any{"txid": id, "pending": true})
	default:
		http.Error(w, "unknown recovery action", http.StatusBadRequest)
	}
}

// BisonrelayGamingLedgerBackupHandler serves the financial ledger backup as a
// file, byte for byte, so its checksum still verifies on restore.
func BisonrelayGamingLedgerBackupHandler(w http.ResponseWriter, r *http.Request) {
	raw, err := services.GamingLedgerBackup()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}
