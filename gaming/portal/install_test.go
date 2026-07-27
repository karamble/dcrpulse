package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bundleHost stands in for the dashboard, serving a binary and its signature.
func bundleHost(t *testing.T, bundle, sig []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("bundle fetched without the game's token: %q", got)
		}
		if r.URL.Query().Get("part") == "sig" {
			_, _ = w.Write(sig)
			return
		}
		_, _ = w.Write(bundle)
	}))
}

func newTestPortalReal(t *testing.T, bridge string) *portal {
	t.Helper()
	dir := t.TempDir()
	games := filepath.Join(dir, "games")
	if err := os.MkdirAll(games, 0o700); err != nil {
		t.Fatalf("games dir: %v", err)
	}
	return &portal{
		gamesDir:  games,
		bridgeURL: bridge,
		running:   map[string]*exec.Cmd{},
	}
}

// A build with no signing key installs nothing. A portal that forgot its key
// must refuse everything rather than accept anything.
func TestNoSigningKeyInstallsNothing(t *testing.T) {
	prev := signingKeyHex
	signingKeyHex = ""
	defer func() { signingKeyHex = prev }()

	srv := bundleHost(t, []byte("binary"), make([]byte, ed25519.SignatureSize))
	defer srv.Close()
	p := newTestPortalReal(t, srv.URL)

	err := p.install("poker", "tok")
	if err == nil {
		t.Fatal("a portal with no signing key must install nothing")
	}
	if !strings.Contains(err.Error(), "no signing key") {
		t.Fatalf("error should name the cause: %v", err)
	}
	if p.have("poker") {
		t.Fatal("nothing should have been written")
	}
}

// The whole point: a bundle signed by the wrong key never reaches the disk.
func TestWrongSignatureIsRefusedAndNothingIsWritten(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	prev := signingKeyHex
	signingKeyHex = hex.EncodeToString(pub)
	defer func() { signingKeyHex = prev }()

	bundle := []byte("a game binary")
	sig := ed25519.Sign(otherPriv, bundle) // signed by somebody else

	srv := bundleHost(t, bundle, sig)
	defer srv.Close()
	p := newTestPortalReal(t, srv.URL)

	err = p.install("poker", "tok")
	if err == nil {
		t.Fatal("a bundle signed by another key must be refused")
	}
	if !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("error should say the signature failed: %v", err)
	}
	if p.have("poker") {
		t.Fatal("a refused bundle must never be written")
	}
}

// Tampering after signing must fail too - this is what signing is for.
func TestAlteredBundleIsRefused(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	prev := signingKeyHex
	signingKeyHex = hex.EncodeToString(pub)
	defer func() { signingKeyHex = prev }()

	original := []byte("a game binary")
	sig := ed25519.Sign(priv, original)
	altered := []byte("a game binary, plus something else")

	srv := bundleHost(t, altered, sig)
	defer srv.Close()
	p := newTestPortalReal(t, srv.URL)

	if err := p.install("poker", "tok"); err == nil {
		t.Fatal("a bundle altered after signing must be refused")
	}
	if p.have("poker") {
		t.Fatal("an altered bundle must never be written")
	}
}

// A correctly signed bundle installs, and lands executable.
func TestSignedBundleInstalls(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	prev := signingKeyHex
	signingKeyHex = hex.EncodeToString(pub)
	defer func() { signingKeyHex = prev }()

	bundle := []byte("a game binary")
	srv := bundleHost(t, bundle, ed25519.Sign(priv, bundle))
	defer srv.Close()
	p := newTestPortalReal(t, srv.URL)

	if err := p.install("poker", "tok"); err != nil {
		t.Fatalf("a correctly signed bundle should install: %v", err)
	}
	if !p.have("poker") {
		t.Fatal("the binary was not written")
	}

	info, err := os.Stat(filepath.Join(p.gamesDir, "poker"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("installed binary is not executable: %v", info.Mode())
	}
	// Nothing else may write it, including a later compromised fetch.
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatalf("installed binary should not be writable: %v", info.Mode())
	}

	got, err := os.ReadFile(filepath.Join(p.gamesDir, "poker"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(bundle) {
		t.Fatal("the installed bytes are not the ones that were signed")
	}
}

// A signature of the wrong length is refused before any verification is
// attempted, so a truncated fetch cannot be mistaken for a bad key.
func TestShortSignatureIsRefused(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	prev := signingKeyHex
	signingKeyHex = hex.EncodeToString(pub)
	defer func() { signingKeyHex = prev }()

	bundle := []byte("a game binary")
	sig := ed25519.Sign(priv, bundle)[:10]

	srv := bundleHost(t, bundle, sig)
	defer srv.Close()
	p := newTestPortalReal(t, srv.URL)

	err = p.install("poker", "tok")
	if err == nil || !strings.Contains(err.Error(), "want 64") {
		t.Fatalf("a short signature should be refused as such: %v", err)
	}
}

func TestMalformedSigningKeyIsRefused(t *testing.T) {
	prev := signingKeyHex
	defer func() { signingKeyHex = prev }()

	for name, key := range map[string]string{
		"not hex":   "zzzz",
		"too short": hex.EncodeToString(make([]byte, 16)),
	} {
		signingKeyHex = key
		if _, err := verifyKey(); err == nil {
			t.Errorf("%s should be refused as a signing key", name)
		}
	}
}
