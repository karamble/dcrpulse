// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testBotUID = "1111111111111111111111111111111111111111111111111111111111111111"
const testGCID = "2222222222222222222222222222222222222222222222222222222222222222"
const testLocalUID = "0000000000000000000000000000000000000000000000000000000000000000"

func joinFixture(t *testing.T) (*communityJoinManager, *[]uint64) {
	t.Helper()
	accepted := []uint64{}
	m := &communityJoinManager{
		path: filepath.Join(t.TempDir(), "join.json"), url: func() string { return "https://custom.example" },
		identity: func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(`{"identity":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `"}`), nil
		},
		invite: func(context.Context, string, string) (DecredPulseInvite, error) {
			return DecredPulseInvite{InviteKey: "key", BotUID: testBotUID, GCID: testGCID}, nil
		},
		redeem:  func(context.Context, string) error { return nil },
		groups:  func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"gcs":[]}`), nil },
		invites: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"invites":[]}`), nil },
		accept:  func(_ context.Context, id uint64) error { accepted = append(accepted, id); return nil },
	}
	return m, &accepted
}
func TestCommunityJoinBindsBothIdentities(t *testing.T) {
	m, accepted := joinFixture(t)
	ctx := t.Context()
	j, err := m.begin(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	m.invites = func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"invites":[
 {"id":1,"name":"Decred Pulse","from":"%s","gcid":"%s","expires":%d},
 {"id":2,"name":"Decred Pulse","from":"%s","gcid":"%s","expires":%d},
 {"id":3,"name":"Renamed community","from":"%s","gcid":"%s","expires":%d}]}`,
			testLocalUID, testGCID, time.Now().Unix()+60, testBotUID, testLocalUID, time.Now().Unix()+60, testBotUID, testGCID, time.Now().Unix()+60)), nil
	}
	if err := m.acceptMatching(ctx, "forged join ID"); err == nil {
		t.Fatal("accepted forged join")
	}
	if err := m.acceptMatching(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if len(*accepted) != 1 || (*accepted)[0] != 3 {
		t.Fatalf("accepted %v", *accepted)
	}
	// Name never establishes success; actual local membership is required.
	for _, tc := range []struct {
		id     string
		member bool
		uid    string
		want   bool
	}{
		{testLocalUID, true, testLocalUID, false}, {testGCID, false, testLocalUID, false}, {testGCID, true, testBotUID, false}, {testGCID, true, testLocalUID, true},
	} {
		m.groups = func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(fmt.Sprintf(`{"gcs":[{"id":%q,"name":"Decred Pulse","local_is_member":%t,"members":[%q]}]}`, tc.id, tc.member, tc.uid)), nil
		}
		got, err := m.status(ctx)
		if err != nil || got.Joined != tc.want {
			t.Fatalf("status %+v %v", got, err)
		}
	}
}
func TestCommunityJoinResumeAndChanges(t *testing.T) {
	m, accepted := joinFixture(t)
	ctx := t.Context()
	calls := 0
	original := m.invite
	m.invite = func(c context.Context, b, u string) (DecredPulseInvite, error) { calls++; return original(c, b, u) }
	j, err := m.begin(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	// Independent coordinator instance reads only durable state after restart.
	restored, _ := joinFixture(t)
	restored.path = m.path
	restored.invite = m.invite
	resumed, err := restored.begin(ctx, false)
	if err != nil || resumed.ID != j.ID || calls != 1 {
		t.Fatalf("resume %+v %v calls %d", resumed, err, calls)
	}
	m.url = func() string { return "https://different.example" }
	if err := m.acceptMatching(ctx, j.ID); err == nil {
		t.Fatal("changed URL accepted old binding")
	}
	if got, err := m.status(ctx); err != nil || got != nil {
		t.Fatal("stale target reported")
	}
	m.url = restored.url
	m.identity = func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"identity":"` + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))) + `"}`), nil
	}
	if err := m.acceptMatching(ctx, j.ID); err == nil {
		t.Fatal("changed identity accepted old binding")
	}
	if len(*accepted) != 0 {
		t.Fatal("unexpected acceptance")
	}
}
func TestCommunityJoinDelayedExpiredAndUncertain(t *testing.T) {
	m, accepted := joinFixture(t)
	ctx := t.Context()
	m.redeem = func(context.Context, string) error { return errors.New("timeout after possible delivery") }
	if _, err := m.begin(ctx, false); err == nil {
		t.Fatal("redemption failure hidden")
	}
	j, err := m.status(ctx)
	if err != nil || j.Status != "uncertain" {
		t.Fatalf("lost uncertain attempt %+v %v", j, err)
	}
	if err := m.acceptMatching(ctx, j.ID); err != nil {
		t.Fatal(err)
	} // no invite yet
	for _, item := range []struct {
		accepted bool
		expires  int64
	}{{false, 1}, {true, time.Now().Unix() + 100}} {
		m.invites = func(context.Context) (json.RawMessage, error) {
			return json.RawMessage(fmt.Sprintf(`{"invites":[{"id":4,"from":%q,"gcid":%q,"accepted":%t,"expires":%d}]}`, testBotUID, testGCID, item.accepted, item.expires)), nil
		}
		if err := m.acceptMatching(ctx, j.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(*accepted) != 0 {
		t.Fatal("accepted expired/already accepted invite")
	}
	m.invites = func(context.Context) (json.RawMessage, error) {
		m.url = func() string { return "changed" }
		return json.RawMessage(fmt.Sprintf(`{"invites":[{"id":5,"from":%q,"gcid":%q,"expires":%d}]}`, testBotUID, testGCID, time.Now().Unix()+100)), nil
	}
	if err := m.acceptMatching(ctx, j.ID); err == nil {
		t.Fatal("configuration change during lookup ignored")
	}
}
func TestCommunityJoinLegacyAndPersistenceFailure(t *testing.T) {
	m, accepted := joinFixture(t)
	ctx := t.Context()
	m.invite = func(context.Context, string, string) (DecredPulseInvite, error) {
		return DecredPulseInvite{InviteKey: "legacy"}, nil
	}
	j, err := m.begin(ctx, false)
	if err != nil || j.Status != "manual" {
		t.Fatalf("legacy %+v %v", j, err)
	}
	if err := m.acceptMatching(ctx, j.ID); err == nil || len(*accepted) != 0 {
		t.Fatal("legacy autoaccepted")
	}
	m, _ = joinFixture(t)
	m.path = filepath.Join(m.path, "missing", "join.json")
	// Force a read/write failure using a parent that is an ordinary file.
	if err := os.WriteFile(filepath.Dir(filepath.Dir(m.path)), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	redeems := 0
	m.redeem = func(context.Context, string) error { redeems++; return nil }
	if _, err := m.begin(ctx, false); err == nil || redeems != 0 {
		t.Fatal("redeemed despite inaccessible state")
	}
}

type communityRoundTrip func(*http.Request) (*http.Response, error)

func (f communityRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCommunityBotResponseContract(t *testing.T) {
	old := externalHTTPClient
	defer func() { externalHTTPClient = old }()
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"inviteKey":"key","botUID":"` + testBotUID + `","gcid":"` + testGCID + `"}`, true},
		{`{"inviteKey":"legacy"}`, true}, {`{"inviteKey":"key","botUID":"` + testBotUID + `"}`, false}, {`{"inviteKey":"key","botUID":"nick","gcid":"` + testGCID + `"}`, false},
	} {
		externalHTTPClient = &http.Client{Transport: communityRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "custom.example" {
				t.Fatal("configured URL not used")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})}
		_, err := requestBotInvite(t.Context(), "https://custom.example", testLocalUID, "", "")
		if (err == nil) != tc.valid {
			t.Fatalf("response %s: %v", tc.body, err)
		}
	}
}

func TestCommunityBotURLSnapshot(t *testing.T) {
	old := externalHTTPClient
	defer func() { externalHTTPClient = old }()
	t.Setenv("BRULSE_API_URL", "https://custom.example/")
	if decredPulseBotURL() != "https://custom.example" {
		t.Fatal("environment override not selected")
	}
	var paths []string
	externalHTTPClient = &http.Client{Transport: communityRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "custom.example" {
			t.Fatal("request changed bots during join")
		}
		paths = append(paths, r.URL.Path)
		body := `{"powRequired":false}`
		if r.URL.Path == "/challenge" {
			t.Setenv("BRULSE_API_URL", "https://other.example")
		} else {
			body = `{"inviteKey":"key","botUID":"` + testBotUID + `","gcid":"` + testGCID + `"}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	got, err := RequestDecredPulseInvite(t.Context(), testLocalUID)
	if err != nil || got.GCID != testGCID || len(paths) != 2 || paths[0] != "/challenge" || paths[1] != "/invite" {
		t.Fatalf("snapshot %+v %v paths %v", got, err, paths)
	}
}
func TestCommunityMalformedReplacementPreservesBinding(t *testing.T) {
	m, _ := joinFixture(t)
	j, err := m.begin(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	m.invite = func(context.Context, string, string) (DecredPulseInvite, error) {
		return DecredPulseInvite{InviteKey: "key", BotUID: "nickname", GCID: testGCID}, nil
	}
	if _, err := m.begin(t.Context(), true); err == nil {
		t.Fatal("malformed replacement accepted")
	}
	got, err := m.status(t.Context())
	if err != nil || got.ID != j.ID {
		t.Fatal("valid binding lost")
	}
}
