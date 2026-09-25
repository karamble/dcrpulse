// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package mcp

import (
	"context"
	"strings"
	"testing"

	"dcrpulse/internal/services"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRateToolsRefuseWhileExchangeRatesAreOff(t *testing.T) {
	old := services.ExchangeRatesEnabled
	services.ExchangeRatesEnabled = func() bool { return false }
	t.Cleanup(func() { services.ExchangeRatesEnabled = old })

	cs := connectTo(t, testAgent("rates-off", "reader", map[string]bool{"dex": true, "bisonrelay": true}))
	for _, name := range []string{"dex_rates", "br_rates"} {
		res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || !strings.Contains(resultText(res), services.ErrExchangeRatesOff.Error()) {
			t.Fatalf("%s: %s", name, resultText(res))
		}
	}
}
