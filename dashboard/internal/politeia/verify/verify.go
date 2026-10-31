// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package verify checks the signatures Politeia puts on proposal data, the
// way politeiawww's own client does (politeiawww/client). Server signatures
// are checked against PinnedServerKey; every other signature against the key
// carried in the signed object.
package verify

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"dcrpulse/internal/merkle"
)

// PinnedServerKey is the ed25519 identity of the mainnet Politeia server,
// as its /api version reply publishes it.
const PinnedServerKey = "a3a3b37951a1dd5de6ab2a35e19e266b6df2daad39aad4c2da04034ca95672f5"

// RecordStatusCensored is a record whose files Politeia deleted.
const RecordStatusCensored = 3

const (
	usermdPluginID         = "usermd"
	streamIDUserMetadata   = 1
	streamIDStatusChanges  = 2
	fileNameProposalMeta   = "proposalmetadata.json"
	authActionAuthorize    = "authorize"
	authActionRevoke       = "revoke"
	proposalUpdateHint     = "proposalupdate"
	billingStatusActive    = 1
	billingStatusCompleted = 3
)

// ErrNoAuthor is returned for a record without author metadata.
var ErrNoAuthor = errors.New("author metadata not found")

// VerifySignature checks a hex ed25519 signature of msg by the hex public key.
func VerifySignature(sigHex, pubKeyHex, msg string) error {
	sig, err := hex.DecodeString(sigHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("signature is not a hex ed25519 signature")
	}
	pk, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return errors.New("public key is not a hex ed25519 key")
	}
	if !ed25519.Verify(pk, []byte(msg), sig) {
		return errors.New("signature does not match")
	}
	return nil
}

// CensorshipRecordVerify checks that the record's files hash to its merkle
// root and that the server signed Merkle+Token. A file kept without its
// payload counts by its digest, which the signed root still covers.
func CensorshipRecordVerify(r Record, serverKey string) error {
	if r.Status == RecordStatusCensored {
		return errors.New("censored record: its files were deleted")
	}
	if len(r.Files) == 0 {
		return errors.New("record has no files")
	}
	digests := make([]*[sha256.Size]byte, 0, len(r.Files))
	for _, f := range r.Files {
		d, err := hex.DecodeString(f.Digest)
		if err != nil || len(d) != sha256.Size {
			return fmt.Errorf("file %v: invalid digest", f.Name)
		}
		if f.Payload != "" {
			b, err := base64.StdEncoding.DecodeString(f.Payload)
			if err != nil {
				return fmt.Errorf("file %v: payload is not base64", f.Name)
			}
			sum := sha256.Sum256(b)
			if !bytes.Equal(sum[:], d) {
				return fmt.Errorf("file %v: payload does not match its digest", f.Name)
			}
		}
		var a [sha256.Size]byte
		copy(a[:], d)
		digests = append(digests, &a)
	}
	if hex.EncodeToString(merkle.Root(digests)[:]) != r.CensorshipRecord.Merkle {
		return errors.New("files do not match the merkle root")
	}
	cr := r.CensorshipRecord
	if err := VerifySignature(cr.Signature, serverKey, cr.Merkle+cr.Token); err != nil {
		return fmt.Errorf("censorship record: %w", err)
	}
	return nil
}

// UserMetadataVerify checks the author's signature over the record's merkle
// root and returns the author metadata.
func UserMetadataVerify(r Record) (UserMetadata, error) {
	var um UserMetadata
	found := false
	for _, ms := range r.Metadata {
		if ms.PluginID == usermdPluginID && ms.StreamID == streamIDUserMetadata {
			if err := json.Unmarshal([]byte(ms.Payload), &um); err != nil {
				return um, fmt.Errorf("author metadata: %w", err)
			}
			found = true
			break
		}
	}
	if !found {
		return um, ErrNoAuthor
	}
	if err := VerifySignature(um.Signature, um.PublicKey, r.CensorshipRecord.Merkle); err != nil {
		return um, fmt.Errorf("author: %w", err)
	}
	return um, nil
}

// StatusChangesVerify checks every signed status change of the record. The
// stream holds one JSON object after another.
func StatusChangesVerify(r Record) ([]StatusChange, error) {
	var out []StatusChange
	for _, ms := range r.Metadata {
		if ms.PluginID != usermdPluginID || ms.StreamID != streamIDStatusChanges {
			continue
		}
		d := json.NewDecoder(bytes.NewReader([]byte(ms.Payload)))
		for d.More() {
			var sc StatusChange
			if err := d.Decode(&sc); err != nil {
				return nil, fmt.Errorf("status change: %w", err)
			}
			msg := sc.Token + strconv.FormatUint(uint64(sc.Version), 10) +
				strconv.FormatUint(uint64(sc.Status), 10) + sc.Reason
			if err := VerifySignature(sc.Signature, sc.PublicKey, msg); err != nil {
				return nil, fmt.Errorf("status change to %d: %w", sc.Status, err)
			}
			out = append(out, sc)
		}
	}
	return out, nil
}

// ProposalMetadataDecode decodes the record's proposalmetadata.json. Its
// payload is covered by CensorshipRecordVerify, which must have passed first.
func ProposalMetadataDecode(r Record) (ProposalMetadata, error) {
	var pm ProposalMetadata
	for _, f := range r.Files {
		if f.Name != fileNameProposalMeta {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(f.Payload)
		if err != nil {
			return pm, fmt.Errorf("proposal metadata: %w", err)
		}
		if err := json.Unmarshal(b, &pm); err != nil {
			return pm, fmt.Errorf("proposal metadata: %w", err)
		}
		return pm, nil
	}
	return pm, errors.New("proposal metadata not found")
}

// CommentVerify checks a comment's shape, its signature and the server's
// receipt. A deleted comment carries the deletion's signature.
func CommentVerify(c Comment, serverKey string) error {
	if err := commentShape(c); err != nil {
		return err
	}
	state := strconv.FormatUint(uint64(c.State), 10)
	msg := state + c.Token + strconv.FormatUint(uint64(c.ParentID), 10) + c.Comment + c.ExtraData + c.ExtraDataHint
	if c.Deleted {
		msg = state + c.Token + strconv.FormatUint(uint64(c.CommentID), 10) + c.Reason
	}
	if err := VerifySignature(c.Signature, c.PublicKey, msg); err != nil {
		return fmt.Errorf("comment %d: %w", c.CommentID, err)
	}
	if err := VerifySignature(c.Receipt, serverKey, c.Signature); err != nil {
		return fmt.Errorf("comment %d receipt: %w", c.CommentID, err)
	}
	return nil
}

// commentShape refuses fields Politeia never sets together: the signature
// covers their concatenation, not where one field ends and the next begins.
func commentShape(c Comment) error {
	switch {
	case c.CommentID == 0:
		return errors.New("comment without an ID")
	case c.Deleted && (c.Comment != "" || c.ExtraData != "" || c.ExtraDataHint != ""):
		return fmt.Errorf("comment %d: deleted but carries content", c.CommentID)
	case !c.Deleted && c.Reason != "":
		return fmt.Errorf("comment %d: deletion reason on a comment not deleted", c.CommentID)
	case c.ExtraData == "" && c.ExtraDataHint == "":
		return nil
	case c.ExtraDataHint != proposalUpdateHint:
		return fmt.Errorf("comment %d: extra data hint %q", c.CommentID, c.ExtraDataHint)
	}
	var u struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(c.ExtraData), &u); err != nil || u.Title == "" {
		return fmt.Errorf("comment %d: proposal update without a title", c.CommentID)
	}
	return nil
}

// AuthDetailsVerify checks a vote authorization or revocation and its receipt.
func AuthDetailsVerify(a AuthDetails, serverKey string) error {
	if a.Action != authActionAuthorize && a.Action != authActionRevoke {
		return fmt.Errorf("vote authorization: unknown action %q", a.Action)
	}
	msg := a.Token + strconv.FormatUint(uint64(a.Version), 10) + a.Action
	if err := VerifySignature(a.Signature, a.PublicKey, msg); err != nil {
		return fmt.Errorf("vote authorization: %w", err)
	}
	if err := VerifySignature(a.Receipt, serverKey, a.Signature); err != nil {
		return fmt.Errorf("vote authorization receipt: %w", err)
	}
	return nil
}

// VoteDetailsVerify checks the admin's signature over the hex SHA256 of the
// vote parameters' JSON, and the server's receipt over the signature and the
// start block hash.
func VoteDetailsVerify(vd VoteDetails, serverKey string) error {
	b, err := json.Marshal(vd.Params)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if err := VerifySignature(vd.Signature, vd.PublicKey, hex.EncodeToString(sum[:])); err != nil {
		return fmt.Errorf("vote start: %w", err)
	}
	switch {
	case vd.Params.Token == "":
		return errors.New("vote start: token not found")
	case vd.Params.Mask == 0:
		return errors.New("vote start: mask not found")
	case len(vd.Params.Options) == 0:
		return errors.New("vote start: options not found")
	}
	if err := VerifySignature(vd.Receipt, serverKey, vd.Signature+vd.StartBlockHash); err != nil {
		return fmt.Errorf("vote start receipt: %w", err)
	}
	return nil
}

// BillingStatusChangeVerify checks an admin's billing status change and its
// receipt.
func BillingStatusChangeVerify(b BillingStatusChange, serverKey string) error {
	if b.Status < billingStatusActive || b.Status > billingStatusCompleted {
		return fmt.Errorf("billing status: unknown status %d", b.Status)
	}
	msg := b.Token + strconv.FormatUint(uint64(b.Status), 10) + b.Reason
	if err := VerifySignature(b.Signature, b.PublicKey, msg); err != nil {
		return fmt.Errorf("billing status: %w", err)
	}
	if err := VerifySignature(b.Receipt, serverKey, b.Signature); err != nil {
		return fmt.Errorf("billing status receipt: %w", err)
	}
	return nil
}
