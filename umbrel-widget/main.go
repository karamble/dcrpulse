// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command dcrpulse-widget is a small standalone server that serves umbrelOS
// desktop widgets for Decred Pulse. It talks directly to dcrd over JSON-RPC and
// exposes only public network/chain data on /widgets/<name>; it never reads
// wallet data, so the endpoints are safe to serve unauthenticated (which is how
// umbreld polls them).
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type server struct {
	dcrd *rpcClient
}

type widgetDef struct {
	typ     string
	refresh string
	build   func() (any, error)
}

func (s *server) registry() map[string]widgetDef {
	return map[string]widgetDef{
		"sync":              {"text-with-progress", "10s", s.widgetSync},
		"node-stats":        {"four-stats", "10s", s.widgetNodeStats},
		"ticket-price":      {"text-with-progress", "30s", s.widgetTicketPrice},
		"ticket-pool":       {"four-stats", "30s", s.widgetTicketPool},
		"staking":           {"three-stats", "30s", s.widgetStaking},
		"price-gauges":      {"two-stats-with-guage", "30s", s.widgetPriceGauges},
		"supply":            {"four-stats", "60s", s.widgetSupply},
		"subsidy-countdown": {"text-with-progress", "60s", s.widgetSubsidyCountdown},
		"network":           {"four-stats", "60s", s.widgetNetwork},
		"votes":             {"list", "5m", s.widgetVotes},
		"status":            {"list-emoji", "30s", s.widgetStatus},
		"launch":            {"text-with-buttons", "30s", s.widgetLaunch},
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	dcrd, err := newRPCClient(
		env("DCRD_RPC_HOST", "dcrd"),
		env("DCRD_RPC_PORT", "9109"),
		env("DCRD_RPC_USER", ""),
		env("DCRD_RPC_PASS", ""),
		env("DCRD_RPC_CERT", "/app-data/dcrd/rpc.cert"),
	)
	if err != nil {
		log.Fatalf("widget: dcrd client: %v", err)
	}
	s := &server{dcrd: dcrd}
	reg := s.registry()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/widgets/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/widgets/")
		def, ok := reg[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		payload, err := def.build()
		if err != nil {
			log.Printf("widget %s: %v", name, err)
			payload = fallbackFor(def.typ)
		}
		_ = json.NewEncoder(w).Encode(withRefresh(payload, def.refresh))
	})

	addr := env("LISTEN", ":3000")
	log.Printf("dcrpulse widget server listening on %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// fallbackFor returns a minimal valid payload for a widget type so the endpoint
// always renders something when an RPC call fails.
func fallbackFor(typ string) any {
	const ph = "-"
	switch typ {
	case "four-stats":
		return statsWidget{Type: typ, Items: []statItem{{Text: ph}, {Text: ph}, {Text: ph}, {Text: ph}}}
	case "three-stats":
		return statsWidget{Type: typ, Items: []statItem{{Text: ph}, {Text: ph}, {Text: ph}}}
	case "text-with-progress":
		return textWithProgress{Type: typ, Text: ph}
	case "two-stats-with-guage":
		return twoStatsGauge{Type: typ, Items: []gaugeItem{{Text: ph}, {Text: ph}}}
	case "list":
		return listWidget{Type: typ, NoItemsText: "Unavailable"}
	case "list-emoji":
		return listEmojiWidget{Type: typ, Items: []emojiItem{{Text: ph}}}
	case "text-with-buttons":
		return textWithButtons{Type: typ, Text: ph, Buttons: []buttonItem{}}
	default:
		return map[string]any{"type": typ}
	}
}

// withRefresh adds the poll interval to a widget payload. umbrelOS reads the
// refresh interval from the response body (it runs ms() on it to schedule
// polling), so every widget response must carry a refresh field alongside its
// type-specific fields.
func withRefresh(payload any, refresh string) any {
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]any{"refresh": refresh}
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return map[string]any{"refresh": refresh}
	}
	fields["refresh"], _ = json.Marshal(refresh)
	return fields
}
