// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"dcrpulse/internal/rpc"
	"dcrpulse/internal/services"
)

const hidingFrame = `--gaming[v=2,game=poker,gv=1,sid=0123456789abcdef,mid=5736684151c34f0a17823de6822769dfafeb3170477c2079dec9d72e35aa5c5f,seq=1/1,exp=0]--eyJhY3Rpb24iOiJmb2xkIn0=`

func TestGCHistoryHandlerServesChatOnly(t *testing.T) {
	t.Cleanup(services.SetGCHistoryFetch(func(context.Context, rpc.ShortIDHex, int, int) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"entries": []map[string]string{
			{"message": hidingFrame}, {"message": "hello"}, {"message": hidingFrame},
		}})
	}))
	req := httptest.NewRequest(http.MethodGet, "/br/gc/x/history?page=0&page_size=10", nil)
	req = mux.SetURLVars(req, map[string]string{"gcid": strings.Repeat("ab", 32)})
	rec := httptest.NewRecorder()
	BisonrelayGCHistoryHandler(rec, req)
	var got struct {
		Entries []struct {
			Message string `json:"message"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d %s: %v", rec.Code, rec.Body.String(), err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Message != "hello" {
		t.Fatalf("entries = %+v", got.Entries)
	}
}

func TestContentFilterMayNotReachGamingFrames(t *testing.T) {
	for body, want := range map[string]bool{
		`{"regexp":"gaming"}`:                  true,
		`{"regexp":".*"}`:                      true,
		`{"regexp":"gaming","skip_gcms":true}`: false,
		`{"regexp":"^hello$"}`:                 false,
		`{"regexp":"("}`:                       false,
		`not json`:                             false,
	} {
		if got := filterMatchesGamingFrames([]byte(body)); got != want {
			t.Errorf("%s: matches gaming frames = %v, want %v", body, got, want)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/br/filters", strings.NewReader(`{"regexp":"--gaming"}`))
	rec := httptest.NewRecorder()
	BisonrelayFiltersHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("filter over gaming frames: HTTP %d", rec.Code)
	}
}
