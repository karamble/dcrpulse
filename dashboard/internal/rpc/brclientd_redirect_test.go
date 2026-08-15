// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeThrowawayCerts produces a self-signed pair and points the config at it
// so brclientdBuild can construct a real client. Nothing is dialled with it.
func writeThrowawayCerts(t *testing.T) {
	t.Helper()
	dir := t.TempDir()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "brclientd-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	write := func(path, blockType string, b []byte) {
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: b}), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(certPath, "CERTIFICATE", der)
	write(keyPath, "EC PRIVATE KEY", keyDER)

	saved := BrclientdCfg
	t.Cleanup(func() { InitBrclientdConfig(saved) })
	InitBrclientdConfig(BrclientdConfig{
		Host:           "brclientd",
		Port:           "7676",
		StatusPort:     "7677",
		ServerCertPath: certPath,
		ClientCertPath: certPath,
		ClientKeyPath:  keyPath,
	})
}

// TestTheBrclientdClientsRefuseRedirects checks all four cached clients.
//
// Kills: deleting CheckRedirect from brclientdBuild, naming which client.
func TestTheBrclientdClientsRefuseRedirects(t *testing.T) {
	writeThrowawayCerts(t)

	builders := map[string]func() (*http.Client, error){
		"shared": brclientdClient,
		"stream": brclientdStreamClient,
		"backup": brclientdBackupClient,
		"pages":  brclientdPagesClient,
	}
	if len(builders) != 4 {
		t.Fatalf("covering %d clients, want all 4", len(builders))
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			cli, err := build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if cli.CheckRedirect == nil {
				t.Fatal("follows redirects: ServeMux's path-cleaning 307 would be " +
					"replayed, method and body intact, at another route")
			}
			if got := cli.CheckRedirect(nil, nil); got != http.ErrUseLastResponse {
				t.Fatalf("CheckRedirect returned %v, want http.ErrUseLastResponse; "+
					"an error would read as an unreachable daemon", got)
			}
		})
	}
}

// TestAFollowedRedirectReplaysThePostBody demonstrates, against a mux
// registered the way brclientd's is, that a followed path-cleaning redirect
// re-sends the POST body outside the subtree.
//
// The first half pins net/http behaviour rather than ours, so no local mutation
// turns it red.
func TestAFollowedRedirectReplaysThePostBody(t *testing.T) {
	var reached []string
	var bodies []string

	mux := http.NewServeMux()
	// Only that /gc/ is registered as a subtree matters here.
	mux.HandleFunc("/gc/", func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, "gc")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		reached = append(reached, r.Method+" /contacts")
		bodies = append(bodies, string(b))
		w.Write([]byte(`{"contacts":["alice"]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	post := func(cli *http.Client) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/gc/../contacts", strings.NewReader(`{"age_days":0}`))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := cli.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	// Default policy: ServeMux cleans the path, answers 307, and the client
	// replays the POST outside /gc/.
	post(&http.Client{})
	if len(bodies) != 1 || bodies[0] != `{"age_days":0}` {
		t.Fatalf("expected the redirect to replay the POST body, got reached=%v bodies=%v", reached, bodies)
	}
	if reached[len(reached)-1] != "POST /contacts" {
		t.Fatalf("expected the replay to arrive as a POST, got %v", reached)
	}

	// Our policy: the 307 is returned and nothing is replayed.
	reached, bodies = nil, nil
	resp := post(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}})
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("got HTTP %d, want the 307 surfaced to the caller", resp.StatusCode)
	}
	if len(reached) != 0 {
		t.Fatalf("the redirect was followed anyway: %v", reached)
	}
}
