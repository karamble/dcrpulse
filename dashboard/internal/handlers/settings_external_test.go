// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"encoding/json"
	"reflect"
	"testing"

	"dcrpulse/internal/types"
)

// WALLET-15: a client that omits a switch, such as a tab from before the
// exchange-rates switch existed, must not turn it off by saving.
func TestSaveKeepsExternalRequestSwitchesItOmits(t *testing.T) {
	var env types.SettingsEnvelope
	if err := json.Unmarshal([]byte(`{"global":{"externalRequests":{"vspListing":true,"politeia":false}}}`), &env); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"exchange_rates": true, "brseeder": false, "politeia": true}
	mergeExternalRequests(allowed, env.Global.ExternalRequests)
	want := map[string]bool{"exchange_rates": true, "brseeder": false, "politeia": false, "stakepool_listing": true}
	if !reflect.DeepEqual(allowed, want) {
		t.Fatalf("allowed = %v, want %v", allowed, want)
	}
}
