// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// bisonw's reconfigurewallet handler unmarshals newWalletPW into a byte slice
// and a null value fails outright, so the key has to be absent rather than null
// when no password change is intended. These assert on the raw JSON, since
// unmarshalling into a struct cannot tell an absent key from a null one.
func newTestWebClient(t *testing.T, h http.HandlerFunc) *WebClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := &WebClient{baseURL: srv.URL, http: srv.Client()}
	c.loggedIn = true // skip the login round trip; not what these test
	return c
}

func captureReconfigureBody(t *testing.T, newWalletPW string) map[string]json.RawMessage {
	t.Helper()
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	cfg := map[string]string{"username": "dcrwallet", "rpclisten": "127.0.0.1:9110"}
	err := c.ReconfigureWallet(context.Background(), AssetDCR, WalletTypeDcrwalletRPC, cfg, newWalletPW)
	if err != nil {
		t.Fatalf("ReconfigureWallet: %v", err)
	}
	return got
}

func TestReconfigureWalletOmitsEmptyPassword(t *testing.T) {
	got := captureReconfigureBody(t, "")

	if raw, ok := got["newWalletPW"]; ok {
		t.Errorf("newWalletPW was sent as %s; the key must be absent, not null", raw)
	}
	if raw, ok := got["appPW"]; ok {
		t.Errorf("appPW was sent as %s; the session cache supplies the password", raw)
	}

	// The rest of the request must be unaffected.
	for _, k := range []string{"assetID", "walletType", "config"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q missing from the request body", k)
		}
	}
}

func TestReconfigureWalletSendsPassword(t *testing.T) {
	got := captureReconfigureBody(t, "s3cret")

	raw, ok := got["newWalletPW"]
	if !ok {
		t.Fatal("newWalletPW missing when a password was supplied")
	}
	var pw string
	if err := json.Unmarshal(raw, &pw); err != nil {
		t.Fatalf("newWalletPW is not a JSON string: %s", raw)
	}
	if pw != "s3cret" {
		t.Errorf("newWalletPW = %q, want %q", pw, "s3cret")
	}
}

// okJSON answers any request with {"ok":true} plus the given extra fields.
func okJSON(extra string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if extra == "" {
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,` + extra + `}`))
	}
}

func TestCallSessionRefusesWithoutLogin(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	t.Cleanup(srv.Close)
	c := &WebClient{baseURL: srv.URL, http: srv.Client()}

	if err := c.OpenWallet(context.Background(), AssetDCR); !errors.Is(err, ErrDexLocked) {
		t.Fatalf("err = %v, want ErrDexLocked", err)
	}
	if hits != 0 {
		t.Fatalf("server reached %d times; a locked client must issue no requests and never attempt a login", hits)
	}
}

func TestCallSessionUnauthedLocks(t *testing.T) {
	c := newTestWebClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if err := c.OpenWallet(context.Background(), AssetDCR); !errors.Is(err, ErrDexLocked) {
		t.Fatalf("err = %v, want ErrDexLocked", err)
	}
	if c.LoggedIn() {
		t.Fatal("client still reports a session after a 401")
	}
}

func TestCallSessionStalePasswordCacheLocks(t *testing.T) {
	for _, msg := range []string{
		"app pass cannot be empty",
		"cached encrypted password not found for auth token: deadbeef",
	} {
		c := newTestWebClient(t, okJSONMsg(msg))
		if err := c.OpenWallet(context.Background(), AssetDCR); !errors.Is(err, ErrDexLocked) {
			t.Fatalf("msg %q: err = %v, want ErrDexLocked", msg, err)
		}
		if c.LoggedIn() {
			t.Fatalf("msg %q: client still reports a session", msg)
		}
	}
}

// okJSONMsg answers with an ok:false reply carrying msg.
func okJSONMsg(msg string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "msg": msg})
	}
}

func TestCallSessionDaemonErrorKeepsSession(t *testing.T) {
	c := newTestWebClient(t, okJSONMsg("cannot log out with active orders"))
	err := c.OpenWallet(context.Background(), AssetDCR)
	if err == nil || errors.Is(err, ErrDexLocked) {
		t.Fatalf("err = %v, want a plain daemon error", err)
	}
	if !strings.Contains(err.Error(), "cannot log out with active orders") {
		t.Fatalf("daemon message lost: %v", err)
	}
	if !c.LoggedIn() {
		t.Fatal("an ordinary daemon error must not drop the session")
	}
}

func TestLoginEstablishesSessionCookies(t *testing.T) {
	var loginBody map[string]string
	var walletCookies []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&loginBody); err != nil {
			t.Errorf("decode login body: %v", err)
		}
		http.SetCookie(w, &http.Cookie{Name: "dexauth", Value: "tok", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "sessionkey", Value: "6b6579", Path: "/"})
		okJSON("")(w, r)
	})
	mux.HandleFunc("/api/openwallet", func(w http.ResponseWriter, r *http.Request) {
		for _, ck := range r.Cookies() {
			walletCookies = append(walletCookies, ck.Name)
		}
		okJSON("")(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &WebClient{baseURL: srv.URL, http: &http.Client{Jar: jar}}

	if err := c.Login(context.Background(), "hunter2"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loginBody["pass"] != "hunter2" {
		t.Fatalf("login body pass = %q", loginBody["pass"])
	}
	if !c.LoggedIn() {
		t.Fatal("LoggedIn false after a successful login")
	}
	if err := c.OpenWallet(context.Background(), AssetDCR); err != nil {
		t.Fatalf("OpenWallet: %v", err)
	}
	joined := strings.Join(walletCookies, ",")
	if !strings.Contains(joined, "dexauth") || !strings.Contains(joined, "sessionkey") {
		t.Fatalf("session cookies not sent on the follow-up call: %v", walletCookies)
	}
}

// Concurrent logins must queue: each registers a daemon-side token that is
// only released on logout.
func TestLoginSerialized(t *testing.T) {
	var mu sync.Mutex
	inFlight, maxInFlight := 0, 0
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		okJSON("")(w, r)
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Login(context.Background(), "pw"); err != nil {
				t.Errorf("Login: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxInFlight != 1 {
		t.Fatalf("%d logins in flight at once, want 1", maxInFlight)
	}
}

func TestTradeBodyAndResult(t *testing.T) {
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON(`"order":{"id":"abc"}`)(w, r)
	})

	raw, err := c.Trade(context.Background(), "dex.example.org", true, false, 42, 0, 100, 200, false, map[string]string{"swapsplit": "true"})
	if err != nil {
		t.Fatalf("Trade: %v", err)
	}
	if string(raw) != `{"id":"abc"}` {
		t.Fatalf("order = %s", raw)
	}
	if pw, ok := got["pw"]; !ok || string(pw) != `""` {
		t.Fatalf("pw field = %s (present %v), want empty string for the session cache", got["pw"], ok)
	}
	var order map[string]json.RawMessage
	if err := json.Unmarshal(got["order"], &order); err != nil {
		t.Fatalf("order member: %v", err)
	}
	for _, k := range []string{"host", "isLimit", "sell", "base", "quote", "qty", "rate", "tifnow", "options"} {
		if _, ok := order[k]; !ok {
			t.Errorf("%q missing from the order form", k)
		}
	}
}

func TestSendBodyExplicitPassword(t *testing.T) {
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON(`"coin":"deadbeef:0"`)(w, r)
	})

	coin, err := c.Send(context.Background(), "hunter2", AssetDCR, 12345, "DsAddr", true)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if coin != "deadbeef:0" {
		t.Fatalf("coin = %q", coin)
	}
	if pw := string(got["pw"]); pw != `"hunter2"` {
		t.Fatalf("pw = %s, want the explicit password (bisonw refuses to resolve /api/send from its cache)", pw)
	}
	if sub := string(got["subtract"]); sub != "true" {
		t.Fatalf("subtract = %s", sub)
	}
}

func TestPostBondBody(t *testing.T) {
	var got map[string]json.RawMessage
	handler := func(w http.ResponseWriter, r *http.Request) {
		got = nil
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON("")(w, r)
	}

	c := newTestWebClient(t, handler)
	if err := c.PostBond(context.Background(), "dex.example.org", "", 100000000, AssetDCR, nil); err != nil {
		t.Fatalf("PostBond: %v", err)
	}
	if pass := string(got["pass"]); pass != `""` {
		t.Fatalf("pass = %s, want empty string", pass)
	}
	if a := string(got["asset"]); a != "42" {
		t.Fatalf("asset = %s, want 42", a)
	}
	for _, k := range []string{"cert", "maintain"} {
		if raw, ok := got[k]; ok {
			t.Errorf("%s sent as %s; the key must be absent when unset", k, raw)
		}
	}

	// Asset id 0 is Bitcoin's real BIP-44 id, not "unset"; it must go verbatim.
	if err := c.PostBond(context.Background(), "dex.example.org", "", 100000000, 0, nil); err != nil {
		t.Fatalf("PostBond(asset 0): %v", err)
	}
	if a := string(got["asset"]); a != "0" {
		t.Fatalf("asset = %s, want 0", a)
	}

	maintain := false
	if err := c.PostBond(context.Background(), "dex.example.org", "PEM", 100000000, AssetDCR, &maintain); err != nil {
		t.Fatalf("PostBond with options: %v", err)
	}
	if m := string(got["maintain"]); m != "false" {
		t.Fatalf("maintain = %s, want false", m)
	}
	if cert := string(got["cert"]); cert != `"PEM"` {
		t.Fatalf("cert = %s", cert)
	}
}

func TestNewWalletFlatBody(t *testing.T) {
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON("")(w, r)
	})

	cfg := map[string]string{"account": "dex"}
	if err := c.NewWallet(context.Background(), AssetDCR, WalletTypeDcrwalletRPC, cfg, "walletpw"); err != nil {
		t.Fatalf("NewWallet: %v", err)
	}
	for _, k := range []string{"assetID", "walletType", "config", "pass", "appPass"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q missing from the flat newwallet form", k)
		}
	}
	if pass := string(got["pass"]); pass != `"walletpw"` {
		t.Fatalf("pass = %s, want the wallet passphrase", pass)
	}
	if app := string(got["appPass"]); app != `""` {
		t.Fatalf("appPass = %s, want empty string for the session cache", app)
	}
}

func TestDiscoverAccountBody(t *testing.T) {
	var got map[string]json.RawMessage
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON(`"paid":true`)(w, r)
	})

	paid, err := c.DiscoverAccount(context.Background(), "dex.example.org", "")
	if err != nil {
		t.Fatalf("DiscoverAccount: %v", err)
	}
	if !paid {
		t.Fatal("paid = false, want true")
	}
	if pass := string(got["pass"]); pass != `""` {
		t.Fatalf("pass = %s, want empty string", pass)
	}
	if _, ok := got["cert"]; ok {
		t.Error("cert sent despite being empty")
	}
}

func TestInitAppLogsIn(t *testing.T) {
	var got map[string]string
	c := &WebClient{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		okJSON(`"mnemonic":"never inspected"`)(w, r)
	}))
	t.Cleanup(srv.Close)
	c.baseURL, c.http = srv.URL, srv.Client()

	if err := c.InitApp(context.Background(), "hunter2", ""); err != nil {
		t.Fatalf("InitApp: %v", err)
	}
	if got["pass"] != "hunter2" {
		t.Fatalf("pass = %q", got["pass"])
	}
	if _, ok := got["seed"]; ok {
		t.Error("seed sent despite being empty")
	}
	if !c.LoggedIn() {
		t.Fatal("init must leave the session established (bisonw logs in on init)")
	}
}

func TestSessionValidReconcilesFlag(t *testing.T) {
	user := `null`
	c := newTestWebClient(t, func(w http.ResponseWriter, r *http.Request) {
		okJSON(`"user":`+user)(w, r)
	})

	valid, err := c.SessionValid(context.Background())
	if err != nil || valid {
		t.Fatalf("valid = %v err = %v, want false nil for a null user", valid, err)
	}
	if c.LoggedIn() {
		t.Fatal("flag not dropped after the probe saw a dead session")
	}

	user = `{"inited":true}`
	valid, err = c.SessionValid(context.Background())
	if err != nil || !valid {
		t.Fatalf("valid = %v err = %v, want true nil", valid, err)
	}
	if !c.LoggedIn() {
		t.Fatal("flag not restored after the probe saw a live session")
	}
}

func TestLogout(t *testing.T) {
	c := newTestWebClient(t, okJSON(""))
	if err := c.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if c.LoggedIn() {
		t.Fatal("session flag still up after logout")
	}

	c = newTestWebClient(t, okJSONMsg("cannot log out with active orders"))
	err := c.Logout(context.Background())
	if err == nil || errors.Is(err, ErrDexLocked) {
		t.Fatalf("err = %v, want the daemon's refusal", err)
	}
	if !c.LoggedIn() {
		t.Fatal("a refused logout must keep the session")
	}
}
