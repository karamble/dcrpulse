// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dcrpulse/internal/services"
)

func TestGamingTableStatusHandlerChecksItsQuery(t *testing.T) {
	old := services.GamingStateDir
	services.GamingStateDir = t.TempDir()
	t.Cleanup(func() { services.GamingStateDir = old })

	for query, want := range map[string]int{
		"game=stakewars&sid=8be1b656de75b8d54a12b0ffa4f08ff6": http.StatusOK,
		"game=StakeWars&sid=8BE1B656DE75B8D54A12B0FFA4F08FF6": http.StatusOK,
		"sid=8be1b656de75b8d54a12b0ffa4f08ff6":                http.StatusBadRequest,
		"game=stakewars":                                      http.StatusBadRequest,
		"game=stakewars&sid=not-a-table":                      http.StatusBadRequest,
		"game=stakewars&sid=" + strings.Repeat("a", 33):       http.StatusBadRequest,
	} {
		rec := httptest.NewRecorder()
		BisonrelayGamingTableStatusHandler(rec, httptest.NewRequest(http.MethodGet, "/br/gaming/table/status?"+query, nil))
		if rec.Code != want {
			t.Errorf("%s: HTTP %d, want %d", query, rec.Code, want)
		}
		if want == http.StatusOK && !strings.Contains(rec.Body.String(), `"accepted":false`) {
			t.Errorf("%s: body %s", query, rec.Body.String())
		}
	}
}
