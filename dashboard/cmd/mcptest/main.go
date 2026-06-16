// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command mcptest exercises the dcrpulse in-process MCP server end to end the
// way a real agent would: it connects with a bearer token, reports the agent's
// capabilities, calls every read tool, and verifies that every spend/write tool
// is correctly refused without a grant. With -live it can additionally execute
// real spends, one user-confirmed step at a time.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var stdin = bufio.NewReader(os.Stdin)

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:8090/", "MCP server endpoint")
	token := flag.String("token", os.Getenv("MCP_TEST_TOKEN"), "bearer token (or set MCP_TEST_TOKEN)")
	live := flag.Bool("live", false, "enable the live spend phase (executes real, user-confirmed spends)")
	only := flag.String("only", "", "comma-separated domains or tool names to test (default all)")
	invoice := flag.String("invoice", "", "bolt11 invoice so ln_pay reaches its grant check")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	flag.Parse()

	optInvoice = *invoice
	initColor(*noColor)

	if *token == "" {
		fmt.Fprintln(os.Stderr, "error: a bearer token is required (-token or MCP_TEST_TOKEN)")
		os.Exit(2)
	}
	filter := parseOnly(*only)

	ctx := context.Background()
	failed := false

	// Phase 0: connect + negative auth.
	section("Connect")
	connCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	session, err := connect(connCtx, *endpoint, *token)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s connect failed: %v\n", red("FAIL"), err)
		os.Exit(1)
	}
	defer session.Close()
	if init := session.InitializeResult(); init != nil && init.ServerInfo != nil {
		fmt.Printf("  connected to %s %s (protocol %s)\n",
			bold(init.ServerInfo.Name), init.ServerInfo.Version, init.ProtocolVersion)
	}
	if ok := negativeAuth(ctx, *endpoint, *token); ok {
		fmt.Printf("  %s bad token rejected\n", green("PASS"))
	} else {
		fmt.Printf("  %s bad token was NOT rejected\n", red("FAIL"))
		failed = true
	}

	// Phase 1: capabilities.
	section("Capabilities")
	caps, ok := fetchCap(ctx, session)
	if !ok {
		fmt.Printf("  %s capabilities call failed\n", red("FAIL"))
		failed = true
	} else {
		printCap(caps)
	}

	// Phase 2: tools/list.
	section("Tools exposed")
	exposed := map[string]bool{}
	if tl, err := session.ListTools(ctx, nil); err != nil {
		fmt.Printf("  %s tools/list failed: %v\n", red("FAIL"), err)
		failed = true
	} else {
		for _, t := range tl.Tools {
			exposed[t.Name] = true
		}
		printExposed(tl.Tools)
	}

	// Phase 3: read phase (safe, automatic).
	section("Read tools")
	d := &discovered{}
	for _, sp := range catalog {
		if sp.kind != kindRead || !filter(sp) {
			continue
		}
		if !exposed[sp.name] {
			add(sp, "SKIP", "domain not granted")
			continue
		}
		args, ready := sp.read(d)
		if !ready {
			add(sp, "SKIP", "no live identifier discovered")
			continue
		}
		res, err := call(ctx, session, sp.name, args)
		switch {
		case err != nil:
			add(sp, "ERR", err.Error())
		case res.isError && subsystemDown(res.text):
			add(sp, "DOWN", preview(res.text, 90))
		case res.isError:
			add(sp, "ERR", preview(res.text, 90))
		default:
			add(sp, "OK", preview(res.text, 90))
			absorb(d, sp.name, res.data)
		}
	}

	// Phase 4/5: spends. A live grant changes everything: firing placeholder
	// spends against a real grant can move funds, and an over-cap call (e.g. a
	// ticket purchase priced far above a tiny per-tx cap) trips the tripwire,
	// which revokes the grant and blocks the agent. So placeholder gating runs
	// only when there is NO grant; with a grant, spends require -live.
	hasGrant := ok && caps.Spend.Granted
	switch {
	case *live && hasGrant:
		section("Live spends (user-confirmed)")
		if runLive(ctx, session, &caps, d, exposed, filter) {
			failed = true
		}
	case *live && !hasGrant:
		fmt.Printf("\n%s -live requested but the agent has no spend grant; nothing to spend. Running the safe gating checks instead.\n", yellow("note:"))
		runGating(ctx, session, exposed, filter, &failed)
	case hasGrant:
		section("Spends (skipped: grant active)")
		fmt.Printf("  %s the agent has an ACTIVE spend grant (perTx %.8f, daily %.8f DCR).\n",
			yellow("!"), caps.Spend.PerTxDcr, caps.Spend.DailyDcr)
		fmt.Println("  Placeholder gating is skipped to avoid moving funds or tripping the tripwire.")
		fmt.Printf("  Re-run with %s to test spends interactively, or revoke the grant to run the safe gating assertions.\n", bold("-live"))
		for _, sp := range catalog {
			if sp.kind == kindSpend && filter(sp) {
				add(sp, "SKIP", "grant active; use -live")
			}
		}
	default:
		section("Spend gating (no funds moved)")
		runGating(ctx, session, exposed, filter, &failed)
	}

	summary()
	if failed {
		os.Exit(1)
	}
}

// runGating fires placeholder calls at every spend tool and asserts each is
// refused. Only safe to call when the agent has no spend grant.
func runGating(ctx context.Context, s *mcp.ClientSession, exposed map[string]bool, filter func(spec) bool, failed *bool) {
	for _, sp := range catalog {
		if sp.kind != kindSpend || !filter(sp) {
			continue
		}
		if !exposed[sp.name] {
			add(sp, "SKIP", "domain not granted")
			continue
		}
		if !gate(ctx, s, sp) {
			*failed = true
		}
	}
}

// gate calls a spend tool with placeholder args and asserts it is refused.
// Returns false if the gate did not hold (a real failure). With no grant every
// spend tool fails the grant lookup first, so the denial is "no spend grant".
func gate(ctx context.Context, s *mcp.ClientSession, sp spec) bool {
	res, err := call(ctx, s, sp.name, sp.spend(d2(sp)))
	if err != nil {
		add(sp, "FAIL", "transport error: "+err.Error())
		return false
	}
	// ln_pay decodes the invoice before checking the grant: without a real
	// invoice it fails at decode, which is still a safe no-op.
	if sp.name == "ln_pay" && optInvoice == "" {
		if res.isError {
			add(sp, "GATED", "decode-first no-op; no funds moved")
			return true
		}
		add(sp, "FAIL", "ln_pay succeeded without a grant")
		return false
	}
	switch {
	case res.isError && (strings.Contains(res.text, "no spend grant") || strings.Contains(res.text, sp.denial)):
		add(sp, "GATED", preview(res.text, 90))
		return true
	case !res.isError:
		add(sp, "FAIL", "tool succeeded without a grant")
		return false
	default:
		add(sp, "FAIL", "unexpected error: "+preview(res.text, 80))
		return false
	}
}

// d2 returns an empty discovered for gating arg builders (they ignore it except
// staking_purchase, which uses a placeholder VSP when none was discovered).
func d2(spec) *discovered { return &discovered{} }

// negativeAuth confirms a corrupted token is rejected at connect time.
func negativeAuth(ctx context.Context, endpoint, token string) bool {
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	sess, err := connect(c, endpoint, token+"x")
	if err != nil {
		return true
	}
	sess.Close()
	return false
}

// capReport mirrors mcp.capabilityReport (decoded from the capabilities tool).
type capReport struct {
	Agent   string   `json:"agent"`
	Domains []string `json:"domains"`
	Spend   struct {
		Granted           bool     `json:"granted"`
		Accounts          []uint32 `json:"accounts"`
		PerTxDcr          float64  `json:"perTxDcr"`
		DailyDcr          float64  `json:"dailyDcr"`
		SpentTodayDcr     float64  `json:"spentTodayDcr"`
		RemainingTodayDcr float64  `json:"remainingTodayDcr"`
		Allowlist         []string `json:"allowlist"`
		Expiry            string   `json:"expiry"`
		AllowVoting       bool     `json:"allowVoting"`
		AllowLightning    bool     `json:"allowLightning"`
		AllowDex          bool     `json:"allowDex"`
		AllowBrWrite      bool     `json:"allowBrWrite"`
		Note              string   `json:"note"`
	} `json:"spend"`
	Note string `json:"note"`
}

func fetchCap(ctx context.Context, s *mcp.ClientSession) (capReport, bool) {
	res, err := call(ctx, s, "capabilities", map[string]any{})
	if err != nil || res.isError {
		return capReport{}, false
	}
	var wrap struct {
		Data capReport `json:"data"`
	}
	if json.Unmarshal([]byte(res.text), &wrap) != nil {
		return capReport{}, false
	}
	return wrap.Data, true
}

func printCap(c capReport) {
	fmt.Printf("  agent:   %s\n", bold(c.Agent))
	fmt.Printf("  domains: %s\n", strings.Join(c.Domains, ", "))
	if !c.Spend.Granted {
		fmt.Printf("  spend:   %s (read-only / gated)\n", yellow("no grant"))
		return
	}
	fmt.Printf("  spend:   %s  accounts=%v perTx=%.8f daily=%.8f spentToday=%.8f remaining=%.8f\n",
		green("granted"), c.Spend.Accounts, c.Spend.PerTxDcr, c.Spend.DailyDcr,
		c.Spend.SpentTodayDcr, c.Spend.RemainingTodayDcr)
	fmt.Printf("           voting=%v lightning=%v dex=%v brWrite=%v allowlist=%v expiry=%s\n",
		c.Spend.AllowVoting, c.Spend.AllowLightning, c.Spend.AllowDex, c.Spend.AllowBrWrite,
		c.Spend.Allowlist, orNone(c.Spend.Expiry))
}

func printExposed(tools []*mcp.Tool) {
	byDomain := map[string]int{}
	exposed := map[string]bool{}
	for _, t := range tools {
		exposed[t.Name] = true
	}
	for _, sp := range catalog {
		if exposed[sp.name] {
			byDomain[sp.domain]++
		}
	}
	fmt.Printf("  server exposes %d tools\n", len(tools))
	seen := map[string]bool{}
	for _, sp := range catalog {
		if seen[sp.domain] {
			continue
		}
		seen[sp.domain] = true
		if n := byDomain[sp.domain]; n > 0 {
			fmt.Printf("    %-12s %d\n", sp.domain, n)
		}
	}
}

// runLive walks the spend tools interactively, executing only on typed
// confirmation and refusing anything that would exceed the grant caps (which
// would trip the tripwire and block the agent). Returns true on any failure.
func runLive(ctx context.Context, s *mcp.ClientSession, caps *capReport, d *discovered, exposed map[string]bool, filter func(spec) bool) bool {
	fmt.Printf("%s live mode: each spend is shown, cap-checked, and requires you to type the tool name to send.\n", yellow("!"))
	failed := false
	for _, sp := range catalog {
		if sp.kind != kindSpend || !filter(sp) {
			continue
		}
		if !exposed[sp.name] {
			add(sp, "SKIP", "domain not granted")
			continue
		}
		args := sp.spend(d)
		if sp.name == "wallet_send" {
			if len(caps.Spend.Accounts) > 0 {
				args["account"] = caps.Spend.Accounts[0]
			}
			if addr := freshAddress(ctx, s, args["account"]); addr != "" {
				args["address"] = addr // self-send: funds return to this wallet
			}
			args["amountDcr"] = 0.0001
		}
		fmt.Printf("\n%s (%s)\n  args: %s\n", bold(sp.name), sp.domain, jsonStr(args))
		custom := promptLine("  custom JSON args (blank=defaults, 'skip' to skip): ")
		if custom == "skip" {
			add(sp, "SKIP", "skipped by user")
			continue
		}
		if custom != "" {
			var override map[string]any
			if err := json.Unmarshal([]byte(custom), &override); err != nil {
				add(sp, "SKIP", "invalid JSON args, skipped")
				continue
			}
			args = override
		}
		if amt, known := spendAmount(ctx, s, sp.name, args); known {
			if amt > caps.Spend.PerTxDcr {
				add(sp, "SKIP", fmt.Sprintf("refused: %.8f DCR over per-tx cap %.8f", amt, caps.Spend.PerTxDcr))
				continue
			}
			if amt > caps.Spend.RemainingTodayDcr {
				add(sp, "SKIP", fmt.Sprintf("refused: %.8f DCR over remaining daily %.8f", amt, caps.Spend.RemainingTodayDcr))
				continue
			}
			fmt.Printf("  cap check: %.8f DCR within per-tx %.8f and remaining %.8f\n", amt, caps.Spend.PerTxDcr, caps.Spend.RemainingTodayDcr)
		} else if amountRelevant(sp.name) {
			if promptLine("  amount unknown; exceeding caps will BLOCK the agent. Type 'I understand' to proceed: ") != "I understand" {
				add(sp, "SKIP", "declined unknown-amount risk")
				continue
			}
		}
		if promptLine(fmt.Sprintf("  type %q to SEND, anything else cancels: ", sp.name)) != sp.name {
			add(sp, "SKIP", "not confirmed")
			continue
		}
		res, err := call(ctx, s, sp.name, args)
		switch {
		case err != nil:
			add(sp, "ERR", err.Error())
		case res.isError:
			add(sp, "ERR", preview(res.text, 90))
		default:
			add(sp, "SENT", preview(res.text, 90))
		}
		if c, ok := fetchCap(ctx, s); ok {
			*caps = c
			fmt.Printf("  spentToday now %.8f DCR (remaining %.8f)\n", c.Spend.SpentTodayDcr, c.Spend.RemainingTodayDcr)
		}
	}
	return failed
}

// freshAddress derives a new receiving address for self-sends.
func freshAddress(ctx context.Context, s *mcp.ClientSession, account any) string {
	res, err := call(ctx, s, "wallet_new_address", map[string]any{"account": account})
	if err != nil || res.isError {
		return ""
	}
	return findString(res.data, isAddr, "address", "addr")
}

// spendAmount returns the DCR amount a live spend will count against the caps,
// when it can be determined locally. staking_purchase is priced from chain.
func spendAmount(ctx context.Context, s *mcp.ClientSession, name string, args map[string]any) (float64, bool) {
	switch name {
	case "wallet_send", "br_tip_user":
		return floatArg(args, "amountDcr")
	case "ln_open_channel":
		return floatArg(args, "localDcr")
	case "staking_purchase":
		res, err := call(ctx, s, "staking_info", map[string]any{})
		if err != nil || res.isError {
			return 0, false
		}
		price, ok := findFloat(res.data, "ticketprice", "ticketPrice", "sdiff", "stakedifficulty")
		n, _ := floatArg(args, "numTickets")
		if !ok || n == 0 {
			return 0, false
		}
		return price * n, true
	}
	return 0, false
}

func amountRelevant(name string) bool {
	return name == "ln_pay" // amount may be carried by the invoice
}

func floatArg(args map[string]any, key string) (float64, bool) {
	switch v := args[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func subsystemDown(msg string) bool {
	m := strings.ToLower(msg)
	for _, s := range []string{"not connected", "not running", "unavailable", "not enabled", "disabled", "no wallet", "not configured", "connection refused", "not initialized"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

func parseOnly(only string) func(spec) bool {
	if strings.TrimSpace(only) == "" {
		return func(spec) bool { return true }
	}
	want := map[string]bool{}
	for _, p := range strings.Split(only, ",") {
		if p = strings.TrimSpace(p); p != "" {
			want[p] = true
		}
	}
	return func(sp spec) bool { return want[sp.domain] || want[sp.name] }
}

func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func promptLine(label string) string {
	fmt.Print(label)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}
