// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dcrpulse/pkg/bisonw"
)

// sendWith posts body to the send handler and returns the subtract flag the
// send was made with.
func sendWith(t *testing.T, body string) bool {
	t.Helper()
	prevSession, prevTx := dexSendSession, sendDexWalletTx
	t.Cleanup(func() { dexSendSession, sendDexWalletTx = prevSession, prevTx })
	dexSendSession = func(http.ResponseWriter) (*bisonw.WebClient, bool) { return new(bisonw.WebClient), true }
	var got *bool
	sendDexWalletTx = func(_ context.Context, _ *bisonw.WebClient, _ string, _ uint32, _ uint64, _ string, subtract bool) (string, error) {
		got = &subtract
		return "coin", nil
	}
	w := httptest.NewRecorder()
	SendDcrdexWalletHandler(w, httptest.NewRequest(http.MethodPost, "/api/dcrdex/wallet/send", strings.NewReader(body)))
	if w.Code != http.StatusOK || got == nil {
		t.Fatalf("send: status %d, body %q, sent %v", w.Code, w.Body.String(), got != nil)
	}
	return *got
}

func TestDexSendPassesSubtract(t *testing.T) {
	const base = `"assetID":0,"value":0.5,"address":"bc1qexample","appPass":"pw"`
	if !sendWith(t, `{`+base+`,"subtract":true}`) {
		t.Error("subtract:true was sent without subtract")
	}
	if sendWith(t, `{`+base+`}`) {
		t.Error("a send without subtract was sent with it")
	}
}
