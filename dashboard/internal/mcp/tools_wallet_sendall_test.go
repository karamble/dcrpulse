// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The dashboard route collapses a send-all request to one recipient before the
// service ever sees it, so the agent surface is the only one that reaches the
// guard. This pins that the tool actually surfaces the refusal.
func TestConstructToolRejectsSendAllWithManyOutputs(t *testing.T) {
	cs := connectTo(t, testAgent("sendall-agent", "sendall", map[string]bool{"wallet": true}))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "wallet_construct_transaction",
		Arguments: map[string]any{
			"account": 0,
			"sendAll": true,
			"outputs": []map[string]any{
				{"address": "DsAlpha", "amountAtoms": 100000000},
				{"address": "DsBeta", "amountAtoms": 0},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}
	if txt := resultText(res); !strings.Contains(txt, "single recipient") {
		t.Fatalf("the tool did not surface the refusal: %q", txt)
	}
}
