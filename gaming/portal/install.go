package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// signingKeyHex is the public key game binaries must be signed with, set at
// build time:
//
//	go build -ldflags "-X main.signingKeyHex=<64 hex chars>"
//
// It is empty by default, and an empty key installs nothing. That is the
// important half: a build that forgot to set it refuses to install anything at
// all, rather than installing whatever it is handed. The failure is a game that
// never starts, which is visible, instead of unsigned code that runs, which is
// not.
var signingKeyHex string

// bundleTimeout bounds a fetch. A game binary is tens of megabytes over a link
// the host is proxying, so this is generous rather than tight.
const bundleTimeout = 5 * time.Minute

// verifyKey returns the configured signing key.
func verifyKey() (ed25519.PublicKey, error) {
	if signingKeyHex == "" {
		return nil, fmt.Errorf("no signing key was built into this portal, so nothing can be installed")
	}
	raw, err := hex.DecodeString(signingKeyHex)
	if err != nil {
		return nil, fmt.Errorf("signing key is not hex: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("signing key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// install fetches a game's binary through the host, verifies it, and writes it.
//
// Nothing is written until the signature verifies. The binary is held in memory
// and checked whole, so a partially downloaded or wrong-keyed bundle never
// reaches the disk under the name the portal will later execute - an install
// that fails leaves the previous state exactly as it was.
func (p *portal) install(id, token string) error {
	key, err := verifyKey()
	if err != nil {
		return err
	}

	bundle, err := p.fetch(id, token, "bin")
	if err != nil {
		return fmt.Errorf("fetch %s: %w", id, err)
	}
	sig, err := p.fetch(id, token, "sig")
	if err != nil {
		return fmt.Errorf("fetch signature for %s: %w", id, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signature for %s is %d bytes, want %d", id, len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(key, bundle, sig) {
		// The interesting failure. Something was published, or served,
		// that this portal was not built to trust.
		return fmt.Errorf("signature for %s does not verify; refusing to install it", id)
	}

	// Write to a temporary name and rename, so the executable path is
	// either the old binary or the new one and never a half-written file
	// that happens to be executable.
	dst := p.binaryFor(id)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, bundle, 0o500); err != nil {
		return fmt.Errorf("write %s: %w", id, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install %s: %w", id, err)
	}
	log.Printf("portal: installed %s (%d bytes, signature verified)", id, len(bundle))
	return nil
}

// fetch pulls one part of a bundle through the host, which is the only route
// the sandbox has.
func (p *portal) fetch(id, token, part string) ([]byte, error) {
	client := &http.Client{Timeout: bundleTimeout}
	req, err := http.NewRequest(http.MethodGet, p.bridgeURL+"/bundle?part="+part, nil)
	if err != nil {
		return nil, err
	}
	// The token is this game's identity: the host resolves it to decide
	// which game is asking, so the request never names one.
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("host returned %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBundleBytes))
}

// maxBundleBytes bounds what the sandbox will hold in memory for one game.
const maxBundleBytes = 128 << 20

// have reports whether a game's binary is already present.
func (p *portal) have(id string) bool {
	_, err := os.Stat(filepath.Join(p.gamesDir, id))
	return err == nil
}
