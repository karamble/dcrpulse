// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"fmt"
	stdslog "log/slog"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/decred/slog"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"dcrpulse/internal/config"
	dcrlog "dcrpulse/internal/log"
)

// mcpLog is the package's subsystem logger. A variable so tests can capture
// output with a buffer-backed logger.
var mcpLog slog.Logger = dcrlog.MCPS

// logGate gates agent-activity logging (the SDK adapter and the per-call
// middleware). The MCP lifecycle lines log unconditionally; only the
// per-request activity is optional. An atomic mirror of KeyMCPLogEnabled so
// the hot path never touches the config file.
var logGate atomic.Bool

// applyPersistedLogging seeds the gate from the persisted toggle.
func applyPersistedLogging() {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return
	}
	var enabled bool
	_, _ = gc.Get(config.KeyMCPLogEnabled, &enabled)
	logGate.Store(enabled)
}

// SetLogging flips agent-activity logging and persists the choice. The state
// change itself is always logged so the file documents its own gaps.
func SetLogging(enabled bool) error {
	logGate.Store(enabled)
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	mcpLog.Infof("Agent activity logging %s", state)
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return err
	}
	if err := gc.Set(config.KeyMCPLogEnabled, enabled); err != nil {
		return err
	}
	return gc.Save()
}

// LoggingConfig is the dashboard-facing view of the activity-log settings.
type LoggingConfig struct {
	Enabled bool `json:"enabled"`
}

// Logging returns the current activity-log settings for the Settings UI.
func Logging() LoggingConfig {
	return LoggingConfig{Enabled: logGate.Load()}
}

// unsafeLogRune reports whether r must not reach the log as itself: anything a
// terminal or a log reader acts on rather than displays. A newline would forge a
// second line, the control runes move the cursor and repaint, and the separators
// and bidi overrides re-order what the operator sees.
func unsafeLogRune(r rune) bool {
	switch {
	case unicode.IsControl(r):
		return true
	case r == '\u2028' || r == '\u2029': // line and paragraph separators
		return true
	case r >= '\u202a' && r <= '\u202e': // bidi embedding and override
		return true
	case r >= '\u2066' && r <= '\u2069': // bidi isolates
		return true
	}
	return false
}

// sanitizeLogField renders an agent-supplied string safe to write to the log.
// The dangerous runes are escaped rather than dropped: what the agent actually
// sent is what an investigation wants to see, and an escaped byte can neither
// forge a line nor move the operator's cursor.
func sanitizeLogField(s string) string {
	if !strings.ContainsFunc(s, unsafeLogRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case !unsafeLogRune(r):
			b.WriteRune(r)
		case r < 0x100:
			fmt.Fprintf(&b, "\\x%02x", r)
		default:
			fmt.Fprintf(&b, "\\u%04x", r)
		}
	}
	return b.String()
}

// truncateLogField bounds error text to one readable line.
func truncateLogField(s string) string {
	const max = 160
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// toolResultError extracts the message of an in-band tool refusal: gated and
// failed tool calls answer with an IsError result, not a JSON-RPC error, and
// must not read as "ok" in the activity log.
func toolResultError(res mcp.Result) string {
	ctr, ok := res.(*mcp.CallToolResult)
	if !ok || ctr == nil || !ctr.IsError {
		return ""
	}
	for _, c := range ctr.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			return truncateLogField(sanitizeLogField(tc.Text))
		}
	}
	return "tool error"
}

// sdkLevel maps a std slog record level onto the subsystem logger. The SDK
// logs one session line per HTTP request in stateless mode, so its Info
// output is demoted to debug; warnings and errors keep their level.
func sdkLevel(l stdslog.Level) slog.Level {
	switch {
	case l >= stdslog.LevelError:
		return slog.LevelError
	case l >= stdslog.LevelWarn:
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}

// sdkLogHandler adapts the go-sdk's std log/slog output onto the MCPS
// subsystem logger, gated per record so the settings toggle applies to live
// sessions without rebuilding servers.
type sdkLogHandler struct {
	attrs []string
	group string
}

func (h *sdkLogHandler) Enabled(_ context.Context, l stdslog.Level) bool {
	return logGate.Load() && sdkLevel(l) >= mcpLog.Level()
}

func (h *sdkLogHandler) Handle(_ context.Context, r stdslog.Record) error {
	parts := make([]string, 0, 1+len(h.attrs)+r.NumAttrs())
	parts = append(parts, r.Message)
	parts = append(parts, h.attrs...)
	r.Attrs(func(a stdslog.Attr) bool {
		parts = append(parts, h.renderAttr(a))
		return true
	})
	line := sanitizeLogField(strings.Join(parts, " "))
	switch sdkLevel(r.Level) {
	case slog.LevelError:
		mcpLog.Error(line)
	case slog.LevelWarn:
		mcpLog.Warn(line)
	default:
		mcpLog.Debug(line)
	}
	return nil
}

func (h *sdkLogHandler) renderAttr(a stdslog.Attr) string {
	key := a.Key
	if h.group != "" {
		key = h.group + "." + key
	}
	return key + "=" + a.Value.String()
}

func (h *sdkLogHandler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler {
	nh := &sdkLogHandler{attrs: h.attrs, group: h.group}
	for _, a := range attrs {
		nh.attrs = append(nh.attrs, nh.renderAttr(a))
	}
	return nh
}

func (h *sdkLogHandler) WithGroup(name string) stdslog.Handler {
	group := name
	if h.group != "" {
		group = h.group + "." + name
	}
	return &sdkLogHandler{attrs: h.attrs, group: group}
}

// sdkLogger is the shared std-slog logger handed to the SDK server and
// streamable handler options.
var sdkLogger = stdslog.New(&sdkLogHandler{})

// activityInfoMethods are the agent actions worth an info-level line. Probes
// and list calls log at debug through the same path.
var activityInfoMethods = map[string]bool{
	"tools/call":            true,
	"resources/read":        true,
	"resources/subscribe":   true,
	"resources/unsubscribe": true,
	"subscriptions/listen":  true,
}

// knownToolNames indexes the tool catalog once, so the middleware can tell one of
// our own names from a string an agent invented.
var knownToolNames = func() map[string]bool {
	m := make(map[string]bool, len(toolCatalog))
	for _, t := range toolCatalog {
		m[t.name] = true
	}
	return m
}()

// logTarget renders an agent-supplied tool name or resource URI for the log. A
// value we recognise is one of our own constants and is safe to print as-is;
// anything else is flagged rather than echoed, so the field the operator reads
// never carries agent input. The attempted value still reaches the line through
// err=, which the SDK fills in and which is already length-bounded.
func logTarget(known bool, v string) string {
	if known {
		return sanitizeLogField(v)
	}
	return "<unregistered>"
}

// activityMiddleware logs one line per handled request for the agent's
// server. Arguments are never logged; only the method, the tool name, the
// duration, and the outcome.
func activityMiddleware(a *agent) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if !logGate.Load() {
				return next(ctx, method, req)
			}
			var target string
			switch p := req.GetParams().(type) {
			case *mcp.CallToolParamsRaw:
				if p != nil {
					target = " tool=" + logTarget(knownToolNames[p.Name], p.Name)
				}
			case *mcp.ReadResourceParams:
				if p != nil {
					_, known := domainForResourceURI(p.URI)
					target = " uri=" + logTarget(known, p.URI)
				}
			case *mcp.SubscribeParams:
				if p != nil {
					_, known := domainForResourceURI(p.URI)
					target = " uri=" + logTarget(known, p.URI)
				}
			case *mcp.UnsubscribeParams:
				if p != nil {
					_, known := domainForResourceURI(p.URI)
					target = " uri=" + logTarget(known, p.URI)
				}
			}
			prefix := "agent=" + sanitizeLogField(a.name) + " method=" + method + target
			// The listen stream blocks until the client goes away, so note the
			// entry; the completion line may be hours out.
			if method == "subscriptions/listen" {
				mcpLog.Info(prefix + " start")
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			line := prefix + " dur=" + time.Since(start).Round(time.Millisecond).String()
			switch {
			case err != nil:
				line += " err=\"" + truncateLogField(sanitizeLogField(err.Error())) + "\""
			default:
				if msg := toolResultError(res); msg != "" {
					line += " err=\"" + msg + "\""
				} else {
					line += " ok"
				}
			}
			if activityInfoMethods[method] {
				mcpLog.Info(line)
			} else {
				mcpLog.Debug(line)
			}
			return res, err
		}
	}
}
