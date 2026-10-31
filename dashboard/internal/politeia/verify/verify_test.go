// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package verify

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"dcrpulse/internal/merkle"
)

// key is a test signer standing in for the server, an author or an admin.
type key struct {
	pub  string
	priv ed25519.PrivateKey
}

func newKey(t *testing.T) key {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return key{hex.EncodeToString(pub), priv}
}

func (k key) sign(msg string) string {
	return hex.EncodeToString(ed25519.Sign(k.priv, []byte(msg)))
}

func file(name, mime, content string) File {
	sum := sha256.Sum256([]byte(content))
	return File{
		Name:    name,
		MIME:    mime,
		Digest:  hex.EncodeToString(sum[:]),
		Payload: base64.StdEncoding.EncodeToString([]byte(content)),
	}
}

// record builds a public record whose files, censorship record and author
// metadata are signed as Politeia signs them.
func record(t *testing.T, server, author key, files ...File) Record {
	t.Helper()
	digests := make([]*[sha256.Size]byte, 0, len(files))
	for _, f := range files {
		d, _ := hex.DecodeString(f.Digest)
		var a [sha256.Size]byte
		copy(a[:], d)
		digests = append(digests, &a)
	}
	root := hex.EncodeToString(merkle.Root(digests)[:])
	const token = "0123456789abcdef"
	um, _ := json.Marshal(UserMetadata{UserID: "u", PublicKey: author.pub, Signature: author.sign(root)})
	return Record{
		State:    2,
		Status:   2,
		Version:  1,
		Metadata: []MetadataStream{{PluginID: "usermd", StreamID: 1, Payload: string(um)}},
		Files:    files,
		CensorshipRecord: CensorshipRecord{
			Token:     token,
			Merkle:    root,
			Signature: server.sign(root + token),
		},
	}
}

func proposalFiles() []File {
	return []File{
		file("index.md", "text/plain; charset=utf-8", "# A proposal\n\nBody."),
		file("proposalmetadata.json", "text/plain; charset=utf-8", `{"name":"A proposal","amount":100000,"legacytoken":"abc"}`),
		file("screenshot.png", "image/png", "\x89PNG not really"),
	}
}

func TestCensorshipRecordVerify(t *testing.T) {
	server, author := newKey(t), newKey(t)
	r := record(t, server, author, proposalFiles()...)
	if err := CensorshipRecordVerify(r, server.pub); err != nil {
		t.Fatalf("signed record: %v", err)
	}

	// An image kept by its digest alone still verifies.
	stripped := r
	stripped.Files = append([]File(nil), r.Files...)
	stripped.Files[2].Payload = ""
	if err := CensorshipRecordVerify(stripped, server.pub); err != nil {
		t.Fatalf("image kept by digest: %v", err)
	}

	tampered := r
	tampered.Files = append([]File(nil), r.Files...)
	tampered.Files[0].Payload = base64.StdEncoding.EncodeToString([]byte("# Another proposal"))
	if CensorshipRecordVerify(tampered, server.pub) == nil {
		t.Fatal("a changed file verified")
	}
	swapped := r
	swapped.Files = append([]File(nil), r.Files...)
	swapped.Files[0].Digest = swapped.Files[1].Digest
	swapped.Files[0].Payload = swapped.Files[1].Payload
	if CensorshipRecordVerify(swapped, server.pub) == nil {
		t.Fatal("a record with a replaced file verified")
	}
	if CensorshipRecordVerify(r, newKey(t).pub) == nil {
		t.Fatal("a record signed by another server verified")
	}
	reversed := r
	reversed.CensorshipRecord.Signature = server.sign(r.CensorshipRecord.Token + r.CensorshipRecord.Merkle)
	if CensorshipRecordVerify(reversed, server.pub) == nil {
		t.Fatal("a signature over Token+Merkle verified")
	}
	censored := r
	censored.Status = 3
	if CensorshipRecordVerify(censored, server.pub) == nil {
		t.Fatal("a censored record verified")
	}
}

func TestUserMetadataVerify(t *testing.T) {
	server, author := newKey(t), newKey(t)
	r := record(t, server, author, proposalFiles()...)
	um, err := UserMetadataVerify(r)
	if err != nil || um.PublicKey != author.pub {
		t.Fatalf("author: %+v %v", um, err)
	}

	forged := r
	payload, _ := json.Marshal(UserMetadata{UserID: "u", PublicKey: author.pub, Signature: newKey(t).sign(r.CensorshipRecord.Merkle)})
	forged.Metadata = []MetadataStream{{PluginID: "usermd", StreamID: 1, Payload: string(payload)}}
	if _, err := UserMetadataVerify(forged); err == nil {
		t.Fatal("an author signature by another key verified")
	}
	blank := r
	payload, _ = json.Marshal(UserMetadata{UserID: "u"})
	blank.Metadata = []MetadataStream{{PluginID: "usermd", StreamID: 1, Payload: string(payload)}}
	if _, err := UserMetadataVerify(blank); err == nil {
		t.Fatal("a blank author signature verified")
	}
	none := r
	none.Metadata = nil
	if _, err := UserMetadataVerify(none); !errors.Is(err, ErrNoAuthor) {
		t.Fatalf("no author metadata: %v", err)
	}
}

func TestStatusChangesVerify(t *testing.T) {
	server, author, admin := newKey(t), newKey(t), newKey(t)
	r := record(t, server, author, proposalFiles()...)
	public := StatusChange{Token: "0123456789abcdef", Version: 1, Status: 2, PublicKey: admin.pub}
	public.Signature = admin.sign("0123456789abcdef" + "1" + "2")
	archived := StatusChange{Token: "0123456789abcdef", Version: 1, Status: 4, Reason: "abandoned", PublicKey: author.pub}
	archived.Signature = author.sign("0123456789abcdef" + "1" + "4" + "abandoned")
	a, _ := json.Marshal(public)
	b, _ := json.Marshal(archived)
	r.Metadata = append(r.Metadata, MetadataStream{PluginID: "usermd", StreamID: 2, Payload: string(a) + string(b)})

	got, err := StatusChangesVerify(r)
	if err != nil || len(got) != 2 || got[1].Reason != "abandoned" {
		t.Fatalf("status changes: %+v %v", got, err)
	}

	archived.Signature = author.sign("0123456789abcdef" + "4" + "1" + "abandoned")
	b, _ = json.Marshal(archived)
	r.Metadata[1].Payload = string(a) + string(b)
	if _, err := StatusChangesVerify(r); err == nil {
		t.Fatal("a status change signed over Token+Status+Version verified")
	}
}

func TestProposalMetadataDecode(t *testing.T) {
	r := record(t, newKey(t), newKey(t), proposalFiles()...)
	pm, err := ProposalMetadataDecode(r)
	if err != nil || pm.Name != "A proposal" || pm.Amount != 100000 || pm.LegacyToken != "abc" {
		t.Fatalf("metadata: %+v %v", pm, err)
	}
}

func TestCommentVerify(t *testing.T) {
	server, user, admin := newKey(t), newKey(t), newKey(t)
	c := Comment{
		State: 2, Token: "0123456789abcdef", ParentID: 3, Comment: "Looks good.",
		PublicKey: user.pub, CommentID: 7, Version: 1,
		ExtraData: `{"title":"Update 1"}`, ExtraDataHint: "proposalupdate",
	}
	c.Signature = user.sign("2" + "0123456789abcdef" + "3" + "Looks good." + `{"title":"Update 1"}` + "proposalupdate")
	c.Receipt = server.sign(c.Signature)
	if err := CommentVerify(c, server.pub); err != nil {
		t.Fatalf("new comment: %v", err)
	}
	if CommentVerify(c, newKey(t).pub) == nil {
		t.Fatal("a receipt by another server verified")
	}

	// Politeia takes no comment edits, so an edit's signature form is refused.
	edited := c
	edited.Version = 2
	edited.Signature = user.sign("2" + "0123456789abcdef" + "3" + "7" + "Looks good." + `{"title":"Update 1"}` + "proposalupdate")
	edited.Receipt = server.sign(edited.Signature)
	if CommentVerify(edited, server.pub) == nil {
		t.Fatal("an edit's signature verified")
	}

	deleted := Comment{
		State: 2, Token: "0123456789abcdef", ParentID: 3, PublicKey: admin.pub,
		CommentID: 7, Version: 1, Deleted: true, Reason: "spam",
	}
	deleted.Signature = admin.sign("2" + "0123456789abcdef" + "7" + "spam")
	deleted.Receipt = server.sign(deleted.Signature)
	if err := CommentVerify(deleted, server.pub); err != nil {
		t.Fatalf("deleted comment: %v", err)
	}
	deleted.Reason = "off topic"
	if CommentVerify(deleted, server.pub) == nil {
		t.Fatal("a deletion with a changed reason verified")
	}
}

// Each shape below carries a valid signature over the same bytes as a
// comment Politeia did sign, split into its fields another way.
func TestCommentShapesPoliteiaNeverProduces(t *testing.T) {
	server, user := newKey(t), newKey(t)
	signed := func(c Comment) Comment {
		c.State, c.Token, c.PublicKey = 2, "0123456789abcdef", user.pub
		msg := "2" + c.Token + strconv.FormatUint(uint64(c.ParentID), 10) + c.Comment + c.ExtraData + c.ExtraDataHint
		if c.Deleted {
			msg = "2" + c.Token + strconv.FormatUint(uint64(c.CommentID), 10) + c.Reason
		}
		c.Signature = user.sign(msg)
		c.Receipt = server.sign(c.Signature)
		return c
	}
	for _, tc := range []struct {
		name string
		c    Comment
		ok   bool
	}{
		{"plain comment", Comment{CommentID: 4, Comment: "Looks good. Really."}, true},
		{"proposal update", Comment{CommentID: 4, Comment: "Done.", ExtraData: `{"title":"Update 1"}`, ExtraDataHint: "proposalupdate"}, true},
		{"deleted comment", Comment{CommentID: 4, ParentID: 1, Deleted: true, Reason: "spam"}, true},
		{"text moved into extra data", Comment{CommentID: 4, Comment: "Looks good.", ExtraData: " Really."}, false},
		{"text moved into the hint", Comment{CommentID: 4, Comment: "Looks good.", ExtraDataHint: " Really."}, false},
		{"update without JSON", Comment{CommentID: 4, ExtraData: "Done.", ExtraDataHint: "proposalupdate"}, false},
		{"update without a title", Comment{CommentID: 4, ExtraData: `{"title":""}`, ExtraDataHint: "proposalupdate"}, false},
		{"update with a malformed title", Comment{CommentID: 4, ExtraData: `{"title":"Update 1","title":5}`, ExtraDataHint: "proposalupdate"}, false},
		{"update under another hint", Comment{CommentID: 4, ExtraData: `{"title":"Update 1"}`, ExtraDataHint: "update"}, false},
		{"top-level comment shown as deleted", Comment{Deleted: true, Reason: "Looks good."}, false},
		{"deleted with text", Comment{CommentID: 4, Deleted: true, Comment: "Looks good."}, false},
		{"deleted with extra data", Comment{CommentID: 4, Deleted: true, ExtraData: "x"}, false},
		{"reason on a comment not deleted", Comment{CommentID: 4, Comment: "Looks good.", Reason: "spam"}, false},
		{"no comment ID", Comment{Comment: "Looks good."}, false},
	} {
		if err := CommentVerify(signed(tc.c), server.pub); (err == nil) != tc.ok {
			t.Errorf("%s: err %v, want ok %v", tc.name, err, tc.ok)
		}
	}
}

func TestAuthDetailsVerify(t *testing.T) {
	server, author := newKey(t), newKey(t)
	a := AuthDetails{Token: "0123456789abcdef", Version: 2, Action: "authorize", PublicKey: author.pub}
	a.Signature = author.sign("0123456789abcdef" + "2" + "authorize")
	a.Receipt = server.sign(a.Signature)
	if err := AuthDetailsVerify(a, server.pub); err != nil {
		t.Fatalf("authorization: %v", err)
	}
	a.Action = "revoke"
	if AuthDetailsVerify(a, server.pub) == nil {
		t.Fatal("an authorization verified as a revocation")
	}
	a.Action = "approve"
	a.Signature = author.sign("0123456789abcdef" + "2" + "approve")
	a.Receipt = server.sign(a.Signature)
	if AuthDetailsVerify(a, server.pub) == nil {
		t.Fatal("a signed unknown action verified")
	}
}

// The admin signs the hash of Go's JSON encoding of the vote parameters, so
// the field order and names must be upstream's exactly.
func TestVoteParamsEncodeAsUpstream(t *testing.T) {
	p := VoteParams{
		Token: "0123456789abcdef", Version: 1, Type: 1, Mask: 3, Duration: 2016,
		QuorumPercentage: 20, PassPercentage: 60,
		Options: []VoteOption{
			{ID: "no", Description: "Don't approve proposal", Bit: 1},
			{ID: "yes", Description: "Approve proposal", Bit: 2},
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"token":"0123456789abcdef","version":1,"type":1,"mask":3,"duration":2016,` +
		`"quorumpercentage":20,"passpercentage":60,"options":[` +
		`{"id":"no","description":"Don't approve proposal","bit":1},` +
		`{"id":"yes","description":"Approve proposal","bit":2}]}`
	if string(b) != want {
		t.Fatalf("vote params JSON\n got %s\nwant %s", b, want)
	}
	p.Parent = "fedcba9876543210"
	b, _ = json.Marshal(p)
	if string(b) != want[:len(want)-1]+`,"parent":"fedcba9876543210"}` {
		t.Fatalf("parent is not last: %s", b)
	}
}

func TestVoteDetailsVerify(t *testing.T) {
	server, admin := newKey(t), newKey(t)
	params := `{"token":"0123456789abcdef","version":1,"type":1,"mask":3,"duration":2016,` +
		`"quorumpercentage":20,"passpercentage":60,"options":[{"id":"no","description":"No","bit":1},` +
		`{"id":"yes","description":"Yes","bit":2}]}`
	var vd VoteDetails
	if err := json.Unmarshal([]byte(params), &vd.Params); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(params))
	vd.PublicKey = admin.pub
	vd.Signature = admin.sign(hex.EncodeToString(sum[:]))
	vd.StartBlockHash = "00000000000000001a2b"
	vd.Receipt = server.sign(vd.Signature + vd.StartBlockHash)
	if err := VoteDetailsVerify(vd, server.pub); err != nil {
		t.Fatalf("vote start: %v", err)
	}

	moved := vd
	moved.Params.PassPercentage = 50
	if VoteDetailsVerify(moved, server.pub) == nil {
		t.Fatal("a vote start with a changed pass percentage verified")
	}
	otherBlock := vd
	otherBlock.StartBlockHash = "00000000000000003c4d"
	if VoteDetailsVerify(otherBlock, server.pub) == nil {
		t.Fatal("a vote start receipt over another start block verified")
	}
}

func TestBillingStatusChangeVerify(t *testing.T) {
	server, admin := newKey(t), newKey(t)
	b := BillingStatusChange{Token: "0123456789abcdef", Status: 2, Reason: "work stopped", PublicKey: admin.pub}
	b.Signature = admin.sign("0123456789abcdef" + "2" + "work stopped")
	b.Receipt = server.sign(b.Signature)
	if err := BillingStatusChangeVerify(b, server.pub); err != nil {
		t.Fatalf("billing status: %v", err)
	}
	b.Status = 3
	if BillingStatusChangeVerify(b, server.pub) == nil {
		t.Fatal("a closure verified as a completion")
	}
	b.Status = 4
	b.Signature = admin.sign("0123456789abcdef" + "4" + "work stopped")
	b.Receipt = server.sign(b.Signature)
	if BillingStatusChangeVerify(b, server.pub) == nil {
		t.Fatal("a signed unknown billing status verified")
	}
}

func TestVerifySignatureRefusesMalformedInput(t *testing.T) {
	k := newKey(t)
	sig := k.sign("m")
	for name, tc := range map[string][2]string{
		"short signature": {sig[:20], k.pub},
		"not hex":         {"zz" + sig[2:], k.pub},
		"short key":       {sig, k.pub[:20]},
	} {
		if VerifySignature(tc[0], tc[1], "m") == nil {
			t.Errorf("%s verified", name)
		}
	}
}
