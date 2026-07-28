package services

import (
	"net/http"
	"testing"
	"time"
)

// The allowlist is the access control, not a convenience.
//
// A game plugin accepts one bearer token for every route it has, including the
// one that hands out the seed all its keys come from. It cannot tell a proxied
// browser request from one this host made, because the proxy authenticates as
// the game. So whether a page can reach a route is decided here and nowhere
// else, and this test is what stops somebody adding one back by accident.
func TestThePageCannotReachWhatOnlyTheHostMay(t *testing.T) {
	forbidden := []struct {
		method, path, why string
	}{
		{http.MethodGet, "/identity/backup",
			"hands out the seed every key is derived from"},
		{http.MethodPost, "/identity/restore",
			"replaces the seed, which would strand a bond forever"},
		{http.MethodPost, "/payout/set",
			"names where winnings go; the host injects that so a page cannot redirect it"},
		{http.MethodPost, "/table/deposit/set",
			"names where a stake already is"},
		{http.MethodPost, "/bond/set",
			"names where a bond already is"},
		{http.MethodPost, "/table/refund",
			"builds and broadcasts a self-signed reclaim"},
		{http.MethodPost, "/bond/sweep",
			"builds and broadcasts a self-signed reclaim"},
		{http.MethodPost, "/table/join",
			"joining is an invitation decision the dashboard already handles"},
		{http.MethodPost, "/cmd",
			"one entry point for a whole command vocabulary, now and later"},
	}

	for _, f := range forbidden {
		if _, ok := GamingUIRouteFor("poker", f.method, f.path); ok {
			t.Errorf("a page can reach %s %s, which %s", f.method, f.path, f.why)
		}
	}
}

// And the ones it must reach, because a table cannot be played without them.
func TestThePageCanReachWhatItNeeds(t *testing.T) {
	needed := []struct{ method, path string }{
		{http.MethodGet, "/tables"},
		{http.MethodGet, "/table/hand"},
		{http.MethodGet, "/table/ledger"},
		{http.MethodGet, "/events"},
		{http.MethodPost, "/table/act"},
		{http.MethodPost, "/table/leave"},
	}
	for _, n := range needed {
		if _, ok := GamingUIRouteFor("poker", n.method, n.path); !ok {
			t.Errorf("a page cannot reach %s %s, so a table cannot be played", n.method, n.path)
		}
	}
}

// A route is matched on both path and method. A GET where a POST is allowed is
// a different thing, and reaching it would mean the list is looser than it
// reads.
func TestMethodIsPartOfTheMatch(t *testing.T) {
	if _, ok := GamingUIRouteFor("poker", http.MethodGet, "/table/act"); ok {
		t.Error("GET /table/act is allowed, and only POST should be")
	}
	if _, ok := GamingUIRouteFor("poker", http.MethodPost, "/tables"); ok {
		t.Error("POST /tables is allowed, and only GET should be")
	}
}

// Anything not written down fails closed, including for a game nobody has
// written a list for.
func TestAnUnknownGameReachesNothing(t *testing.T) {
	if _, ok := GamingUIRouteFor("chess", http.MethodGet, "/tables"); ok {
		t.Error("a game with no allowlist reached a route anyway")
	}
	if GamingUIRoutesKnown("chess") {
		t.Error("a game with no allowlist is reported as known")
	}
	if _, ok := GamingUIRouteFor("poker", http.MethodGet, "/table/hand/../../identity/backup"); ok {
		t.Error("a traversal matched a route")
	}
	if _, ok := GamingUIRouteFor("poker", http.MethodGet, "/TABLES"); ok {
		t.Error("a route matched with different case")
	}
}

// The stream must not be given a deadline. A timeout on it would close a
// connection that is behaving correctly, and to a reader that looks exactly
// like a table where nothing is happening.
func TestTheStreamHasNoDeadline(t *testing.T) {
	route, ok := GamingUIRouteFor("poker", http.MethodGet, "/events")
	if !ok {
		t.Fatal("the stream is not reachable")
	}
	if !route.Stream {
		t.Error("the stream is not marked as one, so it may be buffered")
	}
	if route.Deadline != 0 {
		t.Errorf("the stream has a %s deadline", route.Deadline)
	}
}

// The funding calls block while a person decides in the dashboard. Cutting them
// short here would abandon the payment they were in the middle of approving.
func TestFundingIsGivenTimeForAPersonToDecide(t *testing.T) {
	for _, path := range []string{"/table/fund", "/table/bond", "/bond/fund"} {
		route, ok := GamingUIRouteFor("poker", http.MethodPost, path)
		if !ok {
			t.Fatalf("%s is not reachable", path)
		}
		if route.Deadline < 10*time.Minute {
			t.Errorf("%s is cut off after %s, which is less than somebody takes to decide",
				path, route.Deadline)
		}
	}
}
