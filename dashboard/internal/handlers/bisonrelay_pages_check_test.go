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

func TestPagesCheckHandler(t *testing.T) {
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		BisonrelayPagesCheckHandler(rec, httptest.NewRequest(http.MethodPost, "/api/br/pages/check", strings.NewReader(body)))
		return rec
	}
	rec := call(`{"fields":[{"pattern":".+","value":""},{"pattern":".+","value":"x"}]}`)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"valid":[false,true]}` {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	many := "{\"fields\":[" + strings.TrimSuffix(strings.Repeat(`{"pattern":"a","value":"a"},`, 65), ",") + "]}"
	if rec := call(many); rec.Code != http.StatusBadRequest {
		t.Fatalf("65 fields: status %d, want 400", rec.Code)
	}
}
