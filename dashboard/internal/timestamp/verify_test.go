// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
)

func TestVerifyAuthPathValidBranch(t *testing.T) {
	leaves := make([]*[sha256.Size]byte, 5)
	for i := range leaves {
		h := sha256.Sum256([]byte{byte(i)})
		leaves[i] = &h
	}
	br := AuthPath(leaves, leaves[3])
	want, err := VerifyAuthPath(br)
	if err != nil {
		t.Fatalf("upstream: %v", err)
	}
	got, err := verifyAuthPath(br)
	if err != nil || *got != *want {
		t.Fatalf("got %x, %v; want %x", got, err, want)
	}
}

func TestVerifyAuthPathMalformedBranch(t *testing.T) {
	h := sha256.Sum256([]byte("leaf"))
	for _, br := range []*Branch{
		{NumLeaves: 4, Hashes: [][sha256.Size]byte{h}},
		{NumLeaves: 8, Hashes: [][sha256.Size]byte{h}, Flags: []byte{0xff}},
	} {
		root, err := verifyAuthPath(br)
		if root != nil || err == nil || err.Error() != "malformed merkle path" {
			t.Fatalf("leaves %d: got %x, %v", br.NumLeaves, root, err)
		}
	}
}

func TestValidateProofMalformedPath(t *testing.T) {
	h := sha256.Sum256([]byte("leaf"))
	path, err := json.Marshal(Branch{NumLeaves: 4, Hashes: [][sha256.Size]byte{h}})
	if err != nil {
		t.Fatal(err)
	}
	v := ValidateProof(context.Background(), "ab", "cd", path, "ef")
	if v.MerklePathValid || !strings.Contains(v.Note, "malformed merkle path") {
		t.Fatalf("validation = %+v", v)
	}
}
