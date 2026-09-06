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

// Everything a terminal or a log reader would act on rather than display has to
// be escaped, not dropped: the escaped form still tells an investigation what the
// agent actually sent.
func TestSanitizeLogField(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{name: "plain text is untouched", in: "wallet_send", want: "wallet_send"},
		{name: "unicode is untouched", in: "Grüße 世界", want: "Grüße 世界"},
		{name: "newlines cannot forge a line", in: "a\r\nb", want: `a\x0d\x0ab`},
		{name: "ansi escape", in: "a\x1b[31mred", want: `a\x1b[31mred`},
		{name: "nul", in: "a\x00b", want: `a\x00b`},
		{name: "tab", in: "a\tb", want: `a\x09b`},
		{name: "delete", in: "a\x7fb", want: `a\x7fb`},
		{name: "line separator", in: "a\u2028b", want: `a\u2028b`},
		{name: "bidi override", in: "a\u202eb", want: `a\u202eb`},
		{name: "bidi isolate", in: "a\u2066b", want: `a\u2066b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLogField(tc.in); got != tc.want {
				t.Errorf("sanitizeLogField(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A tool name is agent-supplied and is read before the SDK knows whether it
// exists, so an invented one must not be echoed into the field the operator
// reads. It is flagged instead; the attempted name still reaches the line through
// err=, which is length-bounded.
func TestActivityLogFlagsUnregisteredTool(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)

	junk := strings.Repeat("A", 4096)
	cs := connectTo(t, testAgent("flag-agent", "flag-agent", map[string]bool{"node": true}))
	_, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: junk})

	line := sb.String()
	if !strings.Contains(line, "tool=<unregistered>") {
		t.Errorf("an invented tool name was not flagged: %q", firstLine(line))
	}
	if strings.Contains(line, junk) {
		t.Error("the invented name was echoed into the log in full")
	}
	if !strings.Contains(line, "err=") {
		t.Error("the refusal was not recorded at all, so the operator learns nothing")
	}
	if len(line) > 4096 {
		t.Errorf("the log line is %d bytes; an agent-chosen field is still unbounded", len(line))
	}
}

// The flag must not swallow real names: those are our own constants and the
// operator needs to see which tool ran.
func TestActivityLogKeepsKnownToolNames(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)

	cs := connectTo(t, testAgent("known-agent", "known-agent", map[string]bool{"node": true}))
	if _, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "capabilities"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := sb.String(); !strings.Contains(got, "tool=capabilities") {
		t.Errorf("a registered tool name was not logged: %q", firstLine(got))
	}
}

// Same for a resource URI, which is also read before the catalog is consulted.
func TestActivityLogFlagsUnregisteredURI(t *testing.T) {
	sb := captureMCPLog(t)
	logGate.Store(true)

	junk := "dcrpulse://" + strings.Repeat("B", 2048)
	cs := connectTo(t, testAgent("uri-agent", "uri-agent", map[string]bool{"node": true}))
	_, _ = cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: junk})

	line := sb.String()
	if !strings.Contains(line, "uri=<unregistered>") {
		t.Errorf("an invented URI was not flagged: %q", firstLine(line))
	}
	if strings.Contains(line, junk) {
		t.Error("the invented URI was echoed into the log in full")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
