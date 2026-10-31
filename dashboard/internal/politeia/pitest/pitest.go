// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package pitest builds Politeia proposals signed with test keys, for tests.
// Every signed message is spelled out here as Politeia signs it, apart from
// the verify package, so a mistake there cannot also hide here.
package pitest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"dcrpulse/internal/merkle"

	"dcrpulse/internal/politeia/verify"
)

// Key is a test signer.
type Key struct {
	Pub  string
	priv ed25519.PrivateKey
}

// NewKey returns a fresh signer.
func NewKey() Key {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return Key{hex.EncodeToString(pub), priv}
}

// Sign returns the hex signature of msg.
func (k Key) Sign(msg string) string {
	return hex.EncodeToString(ed25519.Sign(k.priv, []byte(msg)))
}

// File returns a record file holding content.
func File(name, mime, content string) verify.File {
	sum := sha256.Sum256([]byte(content))
	return verify.File{
		Name:    name,
		MIME:    mime,
		Digest:  hex.EncodeToString(sum[:]),
		Payload: base64.StdEncoding.EncodeToString([]byte(content)),
	}
}

// Proposal is a proposal signed by its server, author and admin.
type Proposal struct {
	Server, Author, Admin Key
	Record                verify.Record
	Comments              []verify.Comment
	Auths                 []verify.AuthDetails
	Vote                  *verify.VoteDetails
	Billing               []verify.BillingStatusChange
}

// Options shape a proposal.
type Options struct {
	Name        string
	LegacyToken string
	Comments    int
}

// NewProposal returns a finished, approved proposal with its text, an image,
// author metadata, comments, a vote authorization, a vote start and a
// billing status change, each signed as Politeia signs it.
func NewProposal(server Key, token string, o Options) *Proposal {
	p := &Proposal{Server: server, Author: NewKey(), Admin: NewKey()}
	created := time.Now().AddDate(0, -2, 0).Unix()
	md, _ := json.Marshal(verify.ProposalMetadata{Name: o.Name, Amount: 1000000, Domain: "development", LegacyToken: o.LegacyToken})
	files := []verify.File{
		File("index.md", "text/plain; charset=utf-8", "# "+o.Name+"\n\nThe proposal text."),
		File("proposalmetadata.json", "text/plain; charset=utf-8", string(md)),
		File("diagram.png", "image/png", "\x89PNG "+token),
	}
	digests := make([]*[sha256.Size]byte, 0, len(files))
	for _, f := range files {
		d, _ := hex.DecodeString(f.Digest)
		var a [sha256.Size]byte
		copy(a[:], d)
		digests = append(digests, &a)
	}
	root := hex.EncodeToString(merkle.Root(digests)[:])
	authorSig := p.Author.Sign(root)
	if o.LegacyToken != "" {
		authorSig = ""
	}
	um, _ := json.Marshal(verify.UserMetadata{UserID: "4a1f6c3e-0000-4000-8000-000000000001", PublicKey: p.Author.Pub, Signature: authorSig})
	p.Record = verify.Record{
		State: 2, Status: 2, Version: 1, Timestamp: created, Username: "author",
		Metadata: []verify.MetadataStream{{PluginID: "usermd", StreamID: 1, Payload: string(um)}},
		Files:    files,
		CensorshipRecord: verify.CensorshipRecord{
			Token: token, Merkle: root, Signature: server.Sign(root + token),
		},
	}
	if o.LegacyToken != "" {
		return p
	}

	for i := 1; i <= o.Comments; i++ {
		c := verify.Comment{
			UserID: "u", Username: "commenter", State: 2, Token: token,
			Comment: fmt.Sprintf("Comment %d.", i), PublicKey: p.Author.Pub,
			CommentID: uint32(i), Version: 1, CreatedAt: created + int64(i), Upvotes: 2,
		}
		c.Signature = p.Author.Sign("2" + token + "0" + c.Comment)
		c.Receipt = server.Sign(c.Signature)
		p.Comments = append(p.Comments, c)
	}

	auth := verify.AuthDetails{Token: token, Version: 1, Action: "authorize", PublicKey: p.Author.Pub}
	auth.Signature = p.Author.Sign(token + "1" + "authorize")
	auth.Receipt = server.Sign(auth.Signature)
	p.Auths = []verify.AuthDetails{auth}

	params := verify.VoteParams{
		Token: token, Version: 1, Type: 1, Mask: 3, Duration: 2016,
		QuorumPercentage: 20, PassPercentage: 60,
		Options: []verify.VoteOption{
			{ID: "no", Description: "Don't approve proposal", Bit: 1},
			{ID: "yes", Description: "Approve proposal", Bit: 2},
		},
	}
	pj := `{"token":"` + token + `","version":1,"type":1,"mask":3,"duration":2016,` +
		`"quorumpercentage":20,"passpercentage":60,"options":[` +
		`{"id":"no","description":"Don't approve proposal","bit":1},` +
		`{"id":"yes","description":"Approve proposal","bit":2}]}`
	sum := sha256.Sum256([]byte(pj))
	vd := &verify.VoteDetails{
		Params: params, PublicKey: p.Admin.Pub, StartBlockHeight: 800000,
		StartBlockHash: "000000000000000012ab", EndBlockHeight: 802016,
		EligibleTickets: []string{"aa", "bb"},
	}
	vd.Signature = p.Admin.Sign(hex.EncodeToString(sum[:]))
	vd.Receipt = server.Sign(vd.Signature + vd.StartBlockHash)
	p.Vote = vd

	b := verify.BillingStatusChange{Token: token, Status: 3, PublicKey: p.Admin.Pub}
	b.Signature = p.Admin.Sign(token + strconv.Itoa(3))
	b.Receipt = server.Sign(b.Signature)
	p.Billing = []verify.BillingStatusChange{b}
	return p
}
