// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Legitimate identifiers: 64 hex digits, all characters unreserved in a URL
// path, so no correct implementation alters one.
const (
	testGCID = "1111111111111111111111111111111111111111111111111111111111111111"
	testRV   = "2222222222222222222222222222222222222222222222222222222222222222"
	testUID  = "3333333333333333333333333333333333333333333333333333333333333333"
	testPID  = "4444444444444444444444444444444444444444444444444444444444444444"
	testFID  = "5555555555555555555555555555555555555555555555555555555555555555"
)

// mustID fails the test rather than returning an error: a malformed constant
// here is a broken fixture, not a case under test.
func mustID(t *testing.T, s string) ShortIDHex {
	t.Helper()
	id, err := ParseShortIDHex(s)
	if err != nil {
		t.Fatalf("fixture %q is not an identifier: %v", s, err)
	}
	return id
}

// recordingTransport captures the request line instead of dialling. The
// response is uninteresting; these tests assert only what went out.
type recordingTransport struct {
	seen []string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.seen = append(rt.seen, req.Method+" "+req.URL.Scheme+"://"+req.URL.Host+req.URL.RequestURI())
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader("{}")),
		Request:    req,
	}, nil
}

// installRecorder points all four cached clients at a recorder and restores
// them afterwards. brclientdBuild returns a non-nil cache untouched, so this
// also avoids needing cert files.
func installRecorder(t *testing.T) *recordingTransport {
	t.Helper()

	savedCfg := BrclientdCfg
	savedHTTP, savedStream := brclientdHTTPClient, brclientdStreamHTTPClient
	savedBackup, savedPages := brclientdBackupHTTPClient, brclientdPagesHTTPClient
	t.Cleanup(func() {
		brclientdClientMu.Lock()
		defer brclientdClientMu.Unlock()
		BrclientdCfg = savedCfg
		brclientdHTTPClient, brclientdStreamHTTPClient = savedHTTP, savedStream
		brclientdBackupHTTPClient, brclientdPagesHTTPClient = savedBackup, savedPages
	})

	// Init first: it drops the cached clients, which would discard the
	// recorder if it were installed the other way round.
	InitBrclientdConfig(BrclientdConfig{
		Host:       "brclientd",
		Port:       "7676",
		StatusPort: "7677",
	})

	rt := &recordingTransport{}
	cli := &http.Client{Transport: rt}
	brclientdClientMu.Lock()
	brclientdHTTPClient, brclientdStreamHTTPClient = cli, cli
	brclientdBackupHTTPClient, brclientdPagesHTTPClient = cli, cli
	brclientdClientMu.Unlock()
	return rt
}

// TestBrclientdWireBytes pins the exact request line every brclientd route
// emits for legitimate input.
//
// Kills: escaping the identifier or the "/history/clear" suffix, sending a
// route to the wrong port, and any change to a query string's contents.
func TestBrclientdWireBytes(t *testing.T) {
	const status = "https://brclientd:7677"
	const setup = "https://brclientd:7676"

	gcID, rvID := mustID(t, testGCID), mustID(t, testRV)

	cases := []struct {
		name string
		call func(context.Context) error
		want string
	}{
		// Constant paths, on the status port.
		{"version", func(c context.Context) error {
			_, err := BrclientdVersion(c)
			return err
		}, "GET " + status + "/version"},
		{"status", func(c context.Context) error {
			_, err := BrclientdStatus(c)
			return err
		}, "GET " + status + "/status"},
		{"backup", func(c context.Context) error {
			_, err := BrclientdBackup(c)
			return err
		}, "GET " + status + "/backup"},

		// Constant paths, on the setup port.
		{"create-identity", func(c context.Context) error {
			return BrclientdCreateIdentity(c, "nick", "name")
		}, "POST " + setup + "/create-identity"},
		{"restore-backup", func(c context.Context) error {
			return BrclientdRestoreBackup(c, strings.NewReader("x"))
		}, "POST " + setup + "/restore-backup"},

		// The group-chat surface: every route that carries an id.
		{"gc list", func(c context.Context) error {
			_, err := BrclientdGCList(c)
			return err
		}, "GET " + status + "/gc"},
		{"gc create", func(c context.Context) error {
			_, err := BrclientdGCCreate(c, "table")
			return err
		}, "POST " + status + "/gc/create"},
		{"gc invites", func(c context.Context) error {
			_, err := BrclientdGCInvitesList(c)
			return err
		}, "GET " + status + "/gc/invites"},
		{"gc invites accept", func(c context.Context) error {
			return BrclientdGCInvitesAccept(c, 7)
		}, "POST " + status + "/gc/invites/accept"},
		{"gc detail", func(c context.Context) error {
			_, err := BrclientdGCDetail(c, gcID)
			return err
		}, "GET " + status + "/gc/" + testGCID},
		{"gc invite", func(c context.Context) error {
			return BrclientdGCInvite(c, gcID, testUID)
		}, "POST " + status + "/gc/" + testGCID + "/invite"},
		{"gc message", func(c context.Context) error {
			return BrclientdGCMessage(c, gcID, "hello", 0)
		}, "POST " + status + "/gc/" + testGCID + "/message"},
		{"gc history", func(c context.Context) error {
			_, err := BrclientdGCHistory(c, gcID, 2, 50)
			return err
		}, "GET " + status + "/gc/" + testGCID + "/history?page=2&page_size=50"},
		// Two-segment action: brclientd matches it as one string, so the
		// slash must survive.
		{"gc history clear", func(c context.Context) error {
			return BrclientdGCClearHistory(c, gcID)
		}, "POST " + status + "/gc/" + testGCID + "/history/clear"},
		{"gc part", func(c context.Context) error {
			return BrclientdGCPart(c, gcID, "bye")
		}, "POST " + status + "/gc/" + testGCID + "/part"},
		{"gc kill", func(c context.Context) error {
			return BrclientdGCKill(c, gcID, "over")
		}, "POST " + status + "/gc/" + testGCID + "/kill"},
		{"gc kick", func(c context.Context) error {
			return BrclientdGCKick(c, gcID, testUID, "rude")
		}, "POST " + status + "/gc/" + testGCID + "/kick"},
		{"gc block", func(c context.Context) error {
			return BrclientdGCBlock(c, gcID, testUID)
		}, "POST " + status + "/gc/" + testGCID + "/block"},
		{"gc unblock", func(c context.Context) error {
			return BrclientdGCUnblock(c, gcID, testUID)
		}, "POST " + status + "/gc/" + testGCID + "/unblock"},
		{"gc admins", func(c context.Context) error {
			return BrclientdGCModifyAdmins(c, gcID, []string{testUID}, "why")
		}, "POST " + status + "/gc/" + testGCID + "/admins"},
		{"gc owner", func(c context.Context) error {
			return BrclientdGCModifyOwner(c, gcID, testUID, "why")
		}, "POST " + status + "/gc/" + testGCID + "/owner"},
		{"gc upgrade", func(c context.Context) error {
			return BrclientdGCUpgrade(c, gcID, 2)
		}, "POST " + status + "/gc/" + testGCID + "/upgrade"},
		{"gc alias", func(c context.Context) error {
			return BrclientdGCAlias(c, gcID, "table")
		}, "POST " + status + "/gc/" + testGCID + "/alias"},
		{"gc resend-list", func(c context.Context) error {
			return BrclientdGCResendList(c, gcID, testUID)
		}, "POST " + status + "/gc/" + testGCID + "/resend-list"},

		// The RTDT surface.
		{"rtdt create", func(c context.Context) error {
			_, err := BrclientdRTDTCreate(c, 4, "call")
			return err
		}, "POST " + status + "/rtdt/sessions/create"},
		{"rtdt create-instant", func(c context.Context) error {
			_, err := BrclientdRTDTCreateInstant(c, []string{testUID})
			return err
		}, "POST " + status + "/rtdt/sessions/create-instant"},
		{"rtdt messages", func(c context.Context) error {
			_, err := BrclientdRTDTMessages(c, rvID)
			return err
		}, "GET " + status + "/rtdt/sessions/" + testRV + "/messages"},
		{"rtdt chat", func(c context.Context) error {
			return BrclientdRTDTChat(c, rvID, "hi")
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/chat"},
		{"rtdt invite", func(c context.Context) error {
			return BrclientdRTDTInvite(c, rvID, []string{testUID}, true)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/invite"},
		{"rtdt accept", func(c context.Context) error {
			return BrclientdRTDTAccept(c, rvID, testUID, false)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/accept"},
		{"rtdt join", func(c context.Context) error {
			return BrclientdRTDTJoin(c, rvID)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/join"},
		{"rtdt leave", func(c context.Context) error {
			return BrclientdRTDTLeave(c, rvID)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/leave"},
		{"rtdt dissolve", func(c context.Context) error {
			return BrclientdRTDTDissolve(c, rvID)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/dissolve"},
		{"rtdt kick", func(c context.Context) error {
			return BrclientdRTDTKick(c, rvID, 7, 60)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/kick"},
		{"rtdt remove", func(c context.Context) error {
			return BrclientdRTDTRemove(c, rvID, testUID, "why")
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/remove"},
		{"rtdt rotate-cookies", func(c context.Context) error {
			return BrclientdRTDTRotateCookies(c, rvID)
		}, "POST " + status + "/rtdt/sessions/" + testRV + "/rotate-cookies"},

		// The three routes whose query strings used to be hand-rolled.
		{"content file", func(c context.Context) error {
			_, err := BrclientdContentFile(c, testUID, testFID)
			return err
		}, "GET " + status + "/content/file?fid=" + testFID + "&uid=" + testUID},
		// The absent uid must stay absent rather than becoming uid=.
		{"content file, no uid", func(c context.Context) error {
			_, err := BrclientdContentFile(c, "", testFID)
			return err
		}, "GET " + status + "/content/file?fid=" + testFID},
		// The one route whose bytes moved: url.Values.Encode sorts keys, so
		// uid,pid,index became index,pid,uid. brclientd reads these with
		// r.URL.Query().Get, which ignores order.
		{"embed data", func(c context.Context) error {
			_, err := BrclientdPostEmbedData(c, testUID, testPID, 3)
			return err
		}, "GET " + status + "/posts/embed-data?index=3&pid=" + testPID + "&uid=" + testUID},
		// The one query value that is arbitrary text.
		{"store file", func(c context.Context) error {
			_, _, err := BrclientdGetStoreFile(c, "a b/c&d.png")
			return err
		}, "GET " + status + "/store/files/get?path=a+b%2Fc%26d.png"},
	}

	if len(cases) < 38 {
		t.Fatalf("only %d routes are pinned; this test is the record of the wire "+
			"and a short one silently permits a change", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := installRecorder(t)
			// The recorder answers 200 with "{}", which some callers treat as
			// an error; only the request matters here.
			_ = tc.call(context.Background())
			if len(rt.seen) != 1 {
				t.Fatalf("made %d requests, want exactly 1: %q", len(rt.seen), rt.seen)
			}
			if rt.seen[0] != tc.want {
				t.Errorf("wire mismatch\n got %s\nwant %s", rt.seen[0], tc.want)
			}
		})
	}
}

// TestParseShortIDHexRejectsInjection feeds ParseShortIDHex values that are an
// identifier followed by something else, plus the near-misses.
//
// Kills: dropping the ^ anchor, which readmits "../contacts"; dropping the $
// anchor, which readmits "<hex>/kill"; {64} widened to {64,}; MatchString
// swapped for FindString.
func TestParseShortIDHexRejectsInjection(t *testing.T) {
	good := []string{
		testGCID,
		strings.ToUpper(testGCID), // brclientd hex-decodes either case.
	}
	bad := []string{
		// An identifier, or nothing, followed by another path segment.
		testGCID + "/kill",
		testGCID + "/kill#",
		testGCID + "/kill?x=1",
		testGCID + "/history",
		testGCID + "/history/clear",
		"../contacts",
		"../contacts?",
		"../contacts/reset-all",
		testGCID + "/../../contacts/reset-all",
		testGCID + "#",
		testGCID + "?",
		// Pre-escaped.
		testGCID + "%2fkill",
		// Near-misses.
		"",
		testGCID[:63],
		testGCID + "a",
		" " + testGCID,
		testGCID + " ",
		testGCID + "\n",
		strings.Repeat("g", 64),
	}

	if len(good) < 2 || len(bad) < 19 {
		t.Fatalf("the table shrank to %d good and %d bad; it is the record of "+
			"what this gate refuses", len(good), len(bad))
	}

	for _, s := range good {
		if _, err := ParseShortIDHex(s); err != nil {
			t.Errorf("ParseShortIDHex(%q) rejected a legitimate identifier: %v", s, err)
		}
	}
	for _, s := range bad {
		id, err := ParseShortIDHex(s)
		if err == nil {
			t.Errorf("ParseShortIDHex(%q) accepted it", s)
			continue
		}
		if id != (ShortIDHex{}) {
			t.Errorf("ParseShortIDHex(%q) returned %q alongside its error", s, id)
		}
		// The text reaches logs and untrusted callers, so it must not
		// quote its input.
		if s != "" && strings.Contains(err.Error(), s) {
			t.Errorf("the error for %q quotes the value back: %v", s, err)
		}
	}
}

// TestEveryIdentifierRouteRefusesTheZeroID drives every wrapper that takes an
// identifier with ShortIDHex{}, the only one obtainable without
// ParseShortIDHex.
//
// Kills: removing brclientdRoute's re-check, after which "/gc" + "/" + "" is
// "/gc/" - the group list - or dropping validation from any single wrapper.
func TestEveryIdentifierRouteRefusesTheZeroID(t *testing.T) {
	var zero ShortIDHex

	routes := map[string]func(context.Context) error{
		"GCDetail":          func(c context.Context) error { _, e := BrclientdGCDetail(c, zero); return e },
		"GCInvite":          func(c context.Context) error { return BrclientdGCInvite(c, zero, testUID) },
		"GCMessage":         func(c context.Context) error { return BrclientdGCMessage(c, zero, "x", 0) },
		"GCHistory":         func(c context.Context) error { _, e := BrclientdGCHistory(c, zero, 1, 1); return e },
		"GCClearHistory":    func(c context.Context) error { return BrclientdGCClearHistory(c, zero) },
		"GCPart":            func(c context.Context) error { return BrclientdGCPart(c, zero, "x") },
		"GCKill":            func(c context.Context) error { return BrclientdGCKill(c, zero, "x") },
		"GCKick":            func(c context.Context) error { return BrclientdGCKick(c, zero, testUID, "x") },
		"GCBlock":           func(c context.Context) error { return BrclientdGCBlock(c, zero, testUID) },
		"GCUnblock":         func(c context.Context) error { return BrclientdGCUnblock(c, zero, testUID) },
		"GCModifyAdmins":    func(c context.Context) error { return BrclientdGCModifyAdmins(c, zero, nil, "x") },
		"GCModifyOwner":     func(c context.Context) error { return BrclientdGCModifyOwner(c, zero, testUID, "x") },
		"GCUpgrade":         func(c context.Context) error { return BrclientdGCUpgrade(c, zero, 1) },
		"GCAlias":           func(c context.Context) error { return BrclientdGCAlias(c, zero, "x") },
		"GCResendList":      func(c context.Context) error { return BrclientdGCResendList(c, zero, testUID) },
		"RTDTMessages":      func(c context.Context) error { _, e := BrclientdRTDTMessages(c, zero); return e },
		"RTDTChat":          func(c context.Context) error { return BrclientdRTDTChat(c, zero, "x") },
		"RTDTInvite":        func(c context.Context) error { return BrclientdRTDTInvite(c, zero, nil, false) },
		"RTDTAccept":        func(c context.Context) error { return BrclientdRTDTAccept(c, zero, testUID, false) },
		"RTDTJoin":          func(c context.Context) error { return BrclientdRTDTJoin(c, zero) },
		"RTDTLeave":         func(c context.Context) error { return BrclientdRTDTLeave(c, zero) },
		"RTDTDissolve":      func(c context.Context) error { return BrclientdRTDTDissolve(c, zero) },
		"RTDTKick":          func(c context.Context) error { return BrclientdRTDTKick(c, zero, 1, 1) },
		"RTDTRemove":        func(c context.Context) error { return BrclientdRTDTRemove(c, zero, testUID, "x") },
		"RTDTRotateCookies": func(c context.Context) error { return BrclientdRTDTRotateCookies(c, zero) },
		"RTDTAudioDial":     func(context.Context) error { _, _, e := BrclientdRTDTAudioDial(zero); return e },
	}

	// 25 wrappers plus the audio dialler.
	if len(routes) != 26 {
		t.Fatalf("covering %d identifier routes, want 26", len(routes))
	}

	for name, call := range routes {
		t.Run(name, func(t *testing.T) {
			rt := installRecorder(t)
			err := call(context.Background())
			if !errors.Is(err, ErrBadShortID) {
				t.Fatalf("returned %v, want ErrBadShortID", err)
			}
			if len(rt.seen) != 0 {
				t.Fatalf("reached the network anyway: %q", rt.seen)
			}
		})
	}
}

// TestBrclientdEndpointRefusesUnconfigured covers the guard the twelve URL
// sites each used to carry.
//
// Kills: dropping the host/port check, which sends requests to https://:/…
func TestBrclientdEndpointRefusesUnconfigured(t *testing.T) {
	saved := BrclientdCfg
	t.Cleanup(func() { InitBrclientdConfig(saved) })

	cases := []struct {
		name string
		cfg  BrclientdConfig
		port brclientdPort
	}{
		{"no host", BrclientdConfig{Port: "7676", StatusPort: "7677"}, statusPort},
		{"no status port", BrclientdConfig{Host: "brclientd", Port: "7676"}, statusPort},
		// A different listener: an unset setup port must not fall back.
		{"no setup port", BrclientdConfig{Host: "brclientd", StatusPort: "7677"}, setupPort},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			InitBrclientdConfig(tc.cfg)
			if got, err := brclientdEndpoint(tc.port, "/version", nil); err == nil {
				t.Fatalf("built %q from an incomplete config", got)
			}
		})
	}
}

// TestAnIPv6HostIsBracketed pins net.JoinHostPort.
//
// Kills: host + ":" + port, which yields https://::1:7677.
func TestAnIPv6HostIsBracketed(t *testing.T) {
	saved := BrclientdCfg
	t.Cleanup(func() { InitBrclientdConfig(saved) })

	InitBrclientdConfig(BrclientdConfig{Host: "::1", Port: "7676", StatusPort: "7677"})
	got, err := brclientdEndpoint(statusPort, "/version", nil)
	if err != nil {
		t.Fatalf("brclientdEndpoint: %v", err)
	}
	if want := "https://[::1]:7677/version"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestARouteMustBeAbsolute rejects a path with no leading slash.
func TestARouteMustBeAbsolute(t *testing.T) {
	installRecorder(t)
	for _, p := range []brPath{"", "version", "gc/x"} {
		if got, err := brclientdEndpoint(statusPort, p, nil); err == nil {
			t.Errorf("brclientdEndpoint(%q) built %q", string(p), got)
		}
	}
}
