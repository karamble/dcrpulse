// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A recipient reaches brclientd only as a full uid. Nick lookup there also
// matches uid prefixes and nicks shaped like another contact's uid, so a name
// could land a message or a tip on the wrong contact.
func TestSendsNameTheirRecipientByUIDOnly(t *testing.T) {
	uid := strings.Repeat("ab", 32)
	bad := []string{"alice", uid[:12], uid[:63], uid + "​", ""}

	jsonRoutes := []struct {
		name    string
		handler http.HandlerFunc
		body    func(string) string
	}{
		{"pm", BisonrelayPMHandler, func(u string) string { return `{"user":"` + u + `","msg":"hi"}` }},
		{"audionote", BisonrelayAudioNoteHandler, func(u string) string { return `{"user":"` + u + `","packets_b64":"AAEA"}` }},
		{"tip", BisonrelayContactTipHandler, func(u string) string { return `{"uid":"` + u + `","dcrAmount":0.001}` }},
	}
	for _, rt := range jsonRoutes {
		for _, u := range bad {
			rec := httptest.NewRecorder()
			rt.handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(rt.body(u))))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s with recipient %q: status %d, want 400", rt.name, u, rec.Code)
			}
		}
	}

	for _, u := range bad {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("user", u)
		fw, _ := mw.CreateFormFile("file", "a.txt")
		_, _ = fw.Write([]byte("x"))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		BisonrelayFileSendHandler(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("file send with recipient %q: status %d, want 400", u, rec.Code)
		}
	}
}
