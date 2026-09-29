// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"dcrpulse/internal/services"
)

const (
	tailsValidID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tailsBadID   = "not-an-id-7f3a"
)

func TestDecodeBody(t *testing.T) {
	var v struct {
		N int `json:"n"`
	}
	rec := httptest.NewRecorder()
	if decodeBody(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"n":`)), &v) {
		t.Fatal("a truncated body decoded")
	}
	if rec.Code != http.StatusBadRequest || !strings.HasPrefix(rec.Body.String(), "decode body:") {
		t.Errorf("answer = %d %q, want 400 decode body", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	if !decodeBody(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"n":3}`)), &v) || v.N != 3 {
		t.Errorf("a good body did not decode: %+v", v)
	}
}

// A body id is held to exactly what brclientd will parse: 64 hex digits as
// sent, either case; an empty one is left to the handler.
func TestBrHexID(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"hex", `{"id":"` + tailsValidID + `"}`, true},
		{"upper case", `{"id":"` + strings.ToUpper(tailsValidID) + `"}`, true},
		{"empty", `{"id":""}`, true},
		{"absent", `{}`, true},
		{"short", `{"id":"` + tailsValidID[:63] + `"}`, false},
		{"padded", `{"id":" ` + tailsValidID + `"}`, false},
		{"not hex", `{"id":"` + strings.Repeat("g", 64) + `"}`, false},
		{"fragment", `{"id":"` + tailsValidID[:60] + `#foo"}`, false},
		{"path", `{"id":"` + tailsValidID[:60] + `/../"}`, false},
		{"not a string", `{"id":7}`, false},
		{"list", `{"ids":["` + tailsValidID + `","` + strings.ToUpper(tailsValidID) + `"]}`, true},
		{"empty list", `{"ids":[]}`, true},
		{"list with one bad entry", `{"ids":["` + tailsValidID + `","` + tailsBadID + `"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v struct {
				ID  brHexID  `json:"id"`
				IDs brHexIDs `json:"ids"`
			}
			err := json.Unmarshal([]byte(tc.body), &v)
			if (err == nil) != tc.ok {
				t.Fatalf("Unmarshal(%s) error = %v, want ok=%v", tc.body, err, tc.ok)
			}
			if err != nil && strings.Contains(err.Error(), tailsBadID) {
				t.Errorf("the error echoes the input: %v", err)
			}
		})
	}
}

// Every body id that goes to brclientd is checked here, so a malformed one is a
// 400 from the dashboard. A handler that skipped the check would forward it to
// the absent daemon and answer 502 instead.
func TestMalformedBRIDsAreRefusedBeforeBrclientd(t *testing.T) {
	gcid := map[string]string{"gcid": tailsValidID}
	rv := map[string]string{"rv": tailsValidID}
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		vars    map[string]string
		body    string
	}{
		{"kx reset", BisonrelayContactKXResetHandler, nil, `{"uid":"B"}`},
		{"rename", BisonrelayContactRenameHandler, nil, `{"uid":"B","new_nick":"n"}`},
		{"group assign", BisonrelayContactGroupAssignHandler, nil, `{"uid":"B"}`},
		{"ignore", BisonrelayContactIgnoreHandler, nil, `{"uid":"B","ignore":true}`},
		{"suggest kx", BisonrelayContactSuggestKXHandler, nil, `{"invitee":"V","target":"B"}`},
		{"trans reset", BisonrelayContactTransResetHandler, nil, `{"mediator":"V","target":"B"}`},
		{"mediate ids", BisonrelayMediateIDsHandler, nil, `{"mediator":"B","target":"V"}`},
		{"fetch post", BisonrelayContactFetchPostHandler, nil, `{"uid":"V","pid":"B"}`},
		{"post comment", BisonrelayPostCommentHandler, nil, `{"uid":"V","pid":"V","comment":"c","parent":"B"}`},
		{"post relay", BisonrelayPostRelayHandler, nil, `{"uid":"V","pid":"V","toUid":"B"}`},
		{"post heart", BisonrelayPostHeartHandler, nil, `{"uid":"B","pid":"V"}`},
		{"unshare", BisonrelayManageUnshareHandler, nil, `{"fid":"V","target_uid":"B"}`},
		{"cancel download", BisonrelayManageCancelDownloadHandler, nil, `{"fid":"B"}`},
		{"content get", BisonrelayContentGetHandler, nil, `{"uid":"V","fid":"B"}`},
		{"order status", BisonrelayStoreOrderStatusHandler, nil, `{"uid":"B","id":1,"status":"shipped"}`},
		{"order comment", BisonrelayStoreOrderCommentHandler, nil, `{"uid":"B","id":1,"comment":"c"}`},
		{"rtdt instant", BisonrelayRTDTCreateInstantHandler, nil, `{"uids":["V","B"]}`},
		{"rtdt invite", BisonrelayRTDTInviteHandler, rv, `{"uids":["B"]}`},
		{"rtdt accept", BisonrelayRTDTAcceptHandler, rv, `{"inviter":"B"}`},
		{"rtdt remove", BisonrelayRTDTRemoveHandler, rv, `{"uid":"B"}`},
		{"gc invite", BisonrelayGCInviteHandler, gcid, `{"uid":"B"}`},
		{"gc block", BisonrelayGCBlockHandler, gcid, `{"uid":"B"}`},
		{"gc kick", BisonrelayGCKickHandler, gcid, `{"uid":"B"}`},
		{"gc owner", BisonrelayGCOwnerHandler, gcid, `{"new_owner":"B"}`},
		{"gc admins", BisonrelayGCAdminsHandler, gcid, `{"extra_admins":["V","B"]}`},
		{"gc resend list", BisonrelayGCResendListHandler, gcid, `{"uid":"B"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.NewReplacer(`"B"`, `"`+tailsBadID+`"`, `"V"`, `"`+tailsValidID+`"`).Replace(tc.body)
			req := httptest.NewRequest("POST", "/", strings.NewReader(body))
			if tc.vars != nil {
				req = mux.SetURLVars(req, tc.vars)
			}
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("answer = %d %q, want 400", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), tailsBadID) {
				t.Errorf("the answer echoes the input: %q", rec.Body.String())
			}
		})
	}

	t.Run("history contact", func(t *testing.T) {
		rec := httptest.NewRecorder()
		BisonrelayMessagesHandler(rec, httptest.NewRequest("GET", "/?contact="+tailsBadID, nil))
		if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid contact\n" {
			t.Fatalf("answer = %d %q, want 400 invalid contact", rec.Code, rec.Body.String())
		}
	})

	t.Run("share target", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("cost_atoms", "0")
		_ = mw.WriteField("target_uid", tailsBadID)
		fw, _ := mw.CreateFormFile("file", "a.txt")
		_, _ = fw.Write([]byte("x"))
		_ = mw.Close()
		req := httptest.NewRequest("POST", "/", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		BisonrelayManageAddHandler(rec, req)
		if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid target_uid\n" {
			t.Fatalf("answer = %d %q, want 400 invalid target_uid", rec.Code, rec.Body.String())
		}
	})
}

// Optional ids may be empty: the check is on the shape, so these reach the
// (absent) daemon rather than being refused.
func TestEmptyOptionalBRIDsPass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		vars    map[string]string
		body    string
	}{
		{"comment parent", BisonrelayPostCommentHandler, nil, `{"uid":"V","pid":"V","comment":"c","parent":""}`},
		{"relay to all", BisonrelayPostRelayHandler, nil, `{"uid":"V","pid":"V","toUid":""}`},
		{"unshare for all", BisonrelayManageUnshareHandler, nil, `{"fid":"V","target_uid":""}`},
		{"resend to all", BisonrelayGCResendListHandler, map[string]string{"gcid": tailsValidID}, `{"uid":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(tc.body, `"V"`, `"`+tailsValidID+`"`)
			req := httptest.NewRequest("POST", "/", strings.NewReader(body))
			if tc.vars != nil {
				req = mux.SetURLVars(req, tc.vars)
			}
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code == http.StatusBadRequest {
				t.Fatalf("an empty optional id was refused: %q", rec.Body.String())
			}
		})
	}
}

// The path id is checked before the body is read.
func TestPathIDIsCheckedFirst(t *testing.T) {
	for _, tc := range []struct {
		name, key, want string
		handler         http.HandlerFunc
	}{
		{"gc kick", "gcid", "invalid gcid\n", BisonrelayGCKickHandler},
		{"gc member", "gcid", "invalid gcid\n", BisonrelayGCBlockHandler},
		{"gc clear history", "gcid", "invalid gcid\n", BisonrelayGCClearHistoryHandler},
		{"rtdt kick", "rv", "invalid rv\n", BisonrelayRTDTKickHandler},
		{"rtdt join", "rv", "invalid rv\n", BisonrelayRTDTJoinHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := mux.SetURLVars(httptest.NewRequest("POST", "/", strings.NewReader(`{"uid":`)),
				map[string]string{tc.key: tailsBadID})
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != tc.want {
				t.Fatalf("answer = %d %q, want 400 %q", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

func TestBrMessageBody(t *testing.T) {
	img := &brEmbed{Name: "a.png", Mime: "image/png", DataB64: base64.StdEncoding.EncodeToString([]byte("png"))}
	tag := services.BuildEmbedTag(img.Name, img.Mime, img.DataB64)
	for _, tc := range []struct {
		name, msg string
		embed     *brEmbed
		want      string
	}{
		{"text only", "hi", nil, "hi"},
		{"text and embed", "hi", img, "hi\n" + tag},
		{"embed only", "", img, tag},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := brMessageBody(httptest.NewRecorder(), tc.msg, tc.embed)
			if !ok || got != tc.want {
				t.Fatalf("brMessageBody = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}

	rec := httptest.NewRecorder()
	if _, ok := brMessageBody(rec, "hi", &brEmbed{DataB64: "!!"}); ok || rec.Code != http.StatusBadRequest ||
		!strings.HasPrefix(rec.Body.String(), "embed data_b64:") {
		t.Errorf("bad base64 answered %d %q, want 400 embed data_b64", rec.Code, rec.Body.String())
	}
	big := base64.StdEncoding.EncodeToString(make([]byte, services.MaxInlineEmbedBytes+1))
	rec = httptest.NewRecorder()
	if _, ok := brMessageBody(rec, "hi", &brEmbed{DataB64: big}); ok || rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("an oversized embed answered %d %q, want 413", rec.Code, rec.Body.String())
	}
}

func TestNamedDeletesRefuseTraversal(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"page":     BisonrelayPagesLocalDeleteHandler,
		"template": BisonrelayStoreTemplateDeleteHandler,
	} {
		for _, n := range []string{"../x", " ", "/etc/passwd"} {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+n+`"}`)))
			if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid name\n" {
				t.Errorf("%s delete of %q answered %d %q, want 400 invalid name", name, n, rec.Code, rec.Body.String())
			}
		}
	}
}
