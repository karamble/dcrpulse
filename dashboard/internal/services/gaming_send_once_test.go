package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dcrpulse/internal/gamingfunds"
	"dcrpulse/internal/rpc"
)

// withGCSend replaces the group-chat send with one that answers from errs in
// turn (nil once they run out) and counts every call.
func withGCSend(t *testing.T, errs ...error) *int {
	t.Helper()
	calls := 0
	old := gamingGCSend
	gamingGCSend = func(context.Context, rpc.ShortIDHex, string, int) error {
		calls++
		if len(errs) == 0 {
			return nil
		}
		err := errs[0]
		errs = errs[1:]
		return err
	}
	t.Cleanup(func() { gamingGCSend = old })
	return &calls
}

func testParsedFrame(t *testing.T) gamingFrame {
	t.Helper()
	parsed, ok := parseGamingFrame(testFrame)
	if !ok {
		t.Fatal("test frame did not parse")
	}
	return parsed
}

func freshOutboxLoad() { gamingOutbox.claims = nil }

func sendState(t *testing.T, parsed gamingFrame) string {
	t.Helper()
	state, err := gamingFrameSendState("poker", pruneGCA, parsed, testFrame)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestSendOnceRetriesOnlyARefusedSend(t *testing.T) {
	withGamingWireDir(t)
	parsed := testParsedFrame(t)
	calls := withGCSend(t, &rpc.BrclientdStatusError{Path: "/gc", Code: 503, Body: "BR client not yet running"})

	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err == nil {
		t.Fatal("refused send reported success")
	}
	if got := sendState(t, parsed); got != "released" {
		t.Fatalf("state after refusal = %q", got)
	}
	freshOutboxLoad()
	if got := sendState(t, parsed); got != "released" {
		t.Fatalf("state after reload = %q", got)
	}
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err != nil {
		t.Fatal(err)
	}
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err != nil {
		t.Fatal(err)
	}
	freshOutboxLoad()
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 || sendState(t, parsed) != "sent" {
		t.Fatalf("calls = %d, state %q", *calls, sendState(t, parsed))
	}
}

func TestSendOnceNeverRepeatsAnUnknownOutcome(t *testing.T) {
	withGamingWireDir(t)
	parsed := testParsedFrame(t)
	calls := withGCSend(t, errors.New("brclientd /gc: context deadline exceeded"))
	withHistorySeams(t, historyPages())

	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err == nil {
		t.Fatal("lost send reported success")
	}
	if got := sendState(t, parsed); got != "claimed" {
		t.Fatalf("state after lost answer = %q", got)
	}
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); !errors.Is(err, errGamingSendUncertain) {
		t.Fatalf("second attempt = %v", err)
	}
	// brclientd's own record of the send settles it, still without sending.
	withHistorySeams(t, historyPages(map[string]any{"message": testFrame, "from": pruneSelf, "sent": true}))
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err != nil {
		t.Fatal(err)
	}
	if *calls != 1 || sendState(t, parsed) != "sent" {
		t.Fatalf("calls = %d, state %q", *calls, sendState(t, parsed))
	}
}

func TestOutboxRejectsReleaseAfterSent(t *testing.T) {
	withGamingWireDir(t)
	parsed := testParsedFrame(t)
	withGCSend(t)
	if err := sendGamingFrameOnce(context.Background(), "poker", pruneGCA, parsed, testFrame); err != nil {
		t.Fatal(err)
	}
	if err := releaseGamingFrameClaim("poker", pruneGCA, parsed, testFrame); err == nil {
		t.Fatal("released a sent message")
	}
	path := filepath.Join(GamingStateDir, gamingOutboxFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	released := strings.Replace(lines[len(lines)-1], `"state":"sent"`, `"state":"released"`, 1)
	if err := os.WriteFile(path, []byte(string(raw)+released+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	freshOutboxLoad()
	if _, err := gamingFrameSendState("poker", pruneGCA, parsed, testFrame); err == nil {
		t.Fatal("loaded a release after a send")
	}
}

// payoutLedger writes a ledger holding one payout that awaits signatures,
// with ours already stored, and opens it as the bridge does.
func payoutLedger(t *testing.T, state string) (*gamingfunds.Store, string) {
	t.Helper()
	scope := gamingfunds.Scope{Game: "poker", Network: "mainnet", Wallet: "fp"}
	key, _ := json.Marshal(struct {
		Scope gamingfunds.Scope
		Table string
	}{scope, "0123456789abcdef"})
	id := strings.Repeat("ab", 32)
	public := "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	ledger := map[string]any{
		"version": gamingfunds.Version, "rosterCommits": map[string]any{}, "peers": map[string]any{},
		"deposits": map[string]any{}, "operations": map[string]any{}, "previews": map[string]any{}, "quotes": map[string]any{},
		"keys":   map[string]any{string(key): map[string]any{"scope": scope, "table": "0123456789abcdef", "address": "DsOurs", "public": public}},
		"tables": map[string]any{string(key): map[string]any{"scope": scope, "table": "0123456789abcdef", "group": pruneGCA, "seats": 2, "until": 1}},
		"settlements": map[string]any{id: map[string]any{
			"id": id, "scope": scope, "table": "0123456789abcdef", "state": state,
			"signatures": map[string][][]byte{public: {make([]byte, 64)}},
		}},
	}
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(GamingStateDir, "financial-authority")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "authority.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := gamingFundsStore()
	if err != nil {
		t.Fatal(err)
	}
	return store, id
}

func payoutView(t *testing.T) GamingPayoutView {
	t.Helper()
	views, err := GamingPayouts(context.Background())
	if err != nil || len(views) != 1 {
		t.Fatalf("payouts = %+v, %v", views, err)
	}
	return views[0]
}

func TestPayoutSignaturesSendOnlyWhatBRNeverTook(t *testing.T) {
	withGamingWireDir(t)
	_, id := payoutLedger(t, "awaiting_signatures")
	if got := payoutView(t).SignaturesSent; got != "unsent" {
		t.Fatalf("before any send = %q", got)
	}
	calls := withGCSend(t, &rpc.BrclientdStatusError{Path: "/gc", Code: 500, Body: "send: offline"})
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err == nil {
		t.Fatal("refused send reported success")
	}
	if got := payoutView(t).SignaturesSent; got != "unsent" {
		t.Fatalf("after refusal = %q", got)
	}
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := payoutView(t).SignaturesSent; got != "sent" {
		t.Fatalf("after send = %q", got)
	}
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if *calls != 2 {
		t.Fatalf("sends = %d", *calls)
	}
}

func TestPayoutSignaturesUncertainIsReported(t *testing.T) {
	withGamingWireDir(t)
	_, id := payoutLedger(t, "awaiting_signatures")
	withHistorySeams(t, historyPages())
	calls := withGCSend(t, errors.New("brclientd /gc: connection reset"))
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err == nil {
		t.Fatal("lost send reported success")
	}
	if got := payoutView(t).SignaturesSent; got != "uncertain" {
		t.Fatalf("after a lost answer = %q", got)
	}
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err == nil || *calls != 1 {
		t.Fatalf("second press = %v, sends %d", err, *calls)
	}
}

func TestPayoutSignaturesNothingOnceAssembled(t *testing.T) {
	withGamingWireDir(t)
	_, id := payoutLedger(t, "publishing")
	calls := withGCSend(t)
	if got := payoutView(t).SignaturesSent; got != "" {
		t.Fatalf("assembled payout = %q", got)
	}
	if _, err := SendGamingPayoutSignatures(context.Background(), id); err == nil || *calls != 0 {
		t.Fatalf("send on an assembled payout = %v, sends %d", err, *calls)
	}
}
