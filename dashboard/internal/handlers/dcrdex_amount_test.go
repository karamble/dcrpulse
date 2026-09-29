// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A DEX amount that does not fit the asset is refused before anything reaches
// bisonw; these handlers would otherwise need a session and a daemon.
func TestDexAmountsAreRefusedBeforeBisonw(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{"negative bond cap", SetDcrdexBondOptionsHandler, `{"host":"dex.example","maxBondedDcr":-1}`},
		{"oversized fee estimate", EstimateDcrdexSendFeeHandler, `{"assetID":42,"value":1e300,"address":"Dsaddr"}`},
		{"oversized send", SendDcrdexWalletHandler, `{"assetID":42,"value":1e300,"address":"Dsaddr","appPass":"x"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "range") {
				t.Fatalf("got %d %q, want 400 naming the range", rec.Code, rec.Body.String())
			}
		})
	}
}
