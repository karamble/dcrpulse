// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"bytes"
	"context"
	stdslog "log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/decred/slog"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureMCPLog swaps the package logger for a buffer-backed one and restores
// it (and the activity gate) when the test ends.
func captureMCPLog(t *testing.T) *syncBuf {
	t.Helper()
	sb := &syncBuf{}
	old := mcpLog
	oldGate := logGate.Load()
	mcpLog = slog.NewBackend(sb).Logger("MCPS")
	t.Cleanup(func() {
		mcpLog = old
		logGate.Store(oldGate)
	})
	return sb
}

func TestActivityLoggingDefaultOff(t *testing.T) {
	sb := captureMCPLog(t)
	if Logging().Enabled {
		t.Fatal("activity logging must default to disabled")
	}
	a := testAgent("quiet", "quiet", map[string]bool{"node": true})
	cs := connectTo(t, a)
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "capabilities", Arguments: map[string]any{}}); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if out := sb.String(); out != "" {
		t.Fatalf("disabled activity log emitted output: %q", out)
	}
}

func TestActivityLoggingEmitsToolCall(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)
	a := testAgent("loud", "loud-agent", map[string]bool{"node": true})
	cs := connectTo(t, a)
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "capabilities", Arguments: map[string]any{}}); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"agent=loud-agent", "method=tools/call", "tool=capabilities", " ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("activity line missing %q: %q", want, out)
		}
	}
	if !strings.Contains(out, "[INF]") {
		t.Fatalf("tools/call must log at info: %q", out)
	}
}

func TestActivityLoggingMarksRefusals(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)
	a := testAgent("gated", "gated-agent", map[string]bool{"node": true, "tor": true})
	cs := connectTo(t, a)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "tor_new_identity", Arguments: map[string]any{}})
	if err != nil || !res.IsError {
		t.Fatalf("ungranted write must refuse in-band: res=%+v err=%v", res, err)
	}
	out := sb.String()
	if !strings.Contains(out, `tool=tor_new_identity dur=`) || !strings.Contains(out, `err="no write grant`) {
		t.Fatalf("refusal line missing err marker: %q", out)
	}
	if strings.Contains(out, "tool=tor_new_identity dur=0s ok") {
		t.Fatalf("refusal logged as ok: %q", out)
	}
}

func TestActivityLoggingResourceReadURI(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)
	a := testAgent("reader", "reader", map[string]bool{"node": true})
	cs := connectTo(t, a)
	if _, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: resNodeSync}); err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if out := sb.String(); !strings.Contains(out, "method=resources/read uri="+resNodeSync) {
		t.Fatalf("resource read line missing uri: %q", out)
	}
}

func TestSDKLogDemotesInfo(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)
	mcpLog.SetLevel(slog.LevelDebug)
	lg := stdslog.New(&sdkLogHandler{})
	lg.Info("session connected", "id", "abc")
	lg.Warn("something odd")
	out := sb.String()
	if !strings.Contains(out, "[DBG] MCPS: session connected id=abc") {
		t.Fatalf("std info must demote to debug: %q", out)
	}
	if !strings.Contains(out, "[WRN] MCPS: something odd") {
		t.Fatalf("std warn must stay warn: %q", out)
	}
}

func TestSDKLogGateOff(t *testing.T) {
	sb := captureMCPLog(t)
	mcpLog.SetLevel(slog.LevelDebug)
	lg := stdslog.New(&sdkLogHandler{})
	lg.Error("should not appear")
	if out := sb.String(); out != "" {
		t.Fatalf("gated-off adapter emitted: %q", out)
	}
}

func TestSanitizeLogField(t *testing.T) {
	if got := sanitizeLogField("a\r\nb"); got != "a  b" {
		t.Fatalf("sanitize: %q", got)
	}
	if got := sanitizeLogField("clean"); got != "clean" {
		t.Fatalf("sanitize clean: %q", got)
	}
}
