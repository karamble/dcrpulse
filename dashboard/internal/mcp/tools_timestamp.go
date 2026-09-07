// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"dcrpulse/internal/middleware"
	"dcrpulse/internal/services"
	"dcrpulse/internal/timestamp"
)

type timestampDigestInput struct {
	Digest string `json:"digest" jsonschema:"the file's sha256 digest as 64 hex characters"`
}

type timestampCreateInput struct {
	Digest      string   `json:"digest" jsonschema:"the file's sha256 digest as 64 hex characters; the file itself is never sent"`
	Filename    string   `json:"filename,omitempty" jsonschema:"optional original file name for the record"`
	Title       string   `json:"title,omitempty" jsonschema:"optional title for the record"`
	Description string   `json:"description,omitempty" jsonschema:"optional description for the record"`
	FileSize    int64    `json:"fileSize,omitempty" jsonschema:"optional file size in bytes"`
	MimeType    string   `json:"mimeType,omitempty" jsonschema:"optional file MIME type"`
	FileMtime   string   `json:"fileMtime,omitempty" jsonschema:"optional file modification time"`
	Tags        []string `json:"tags,omitempty" jsonschema:"optional tags for the record"`
}

type timestampUpdateInput struct {
	Digest      string   `json:"digest" jsonschema:"the digest of the record to update"`
	Title       string   `json:"title,omitempty" jsonschema:"new title"`
	Description string   `json:"description,omitempty" jsonschema:"new description"`
	Tags        []string `json:"tags,omitempty" jsonschema:"new tags"`
}

// normDigest lowercases and trims a digest argument, matching the handlers.
func normDigest(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// timestampTools are the "timestamp" domain tools, backed by the same dcrtime
// proof archive the timestamp page uses. The write tools are gated on the
// timestamp write scope.
var timestampTools = []toolDef{
	readTool("timestamp", "timestamp_records",
		"List dcrtime timestamp records and their anchoring status.",
		func(_ context.Context, _ emptyInput) (any, error) {
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			return store.List(timestamp.Query{}), nil
		}),
	readTool("timestamp", "timestamp_verify",
		"Verify a digest against the local archive, dcrtime, and (when anchored) the Decred chain.",
		func(ctx context.Context, in timestampDigestInput) (any, error) {
			digest := normDigest(in.Digest)
			if !timestamp.ValidDigest(digest) {
				return nil, errors.New("digest must be a 64-character hex sha256")
			}
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			resp := map[string]any{"digest": digest, "inArchive": false}
			if rec, err := store.Get(digest); err == nil {
				resp["inArchive"] = true
				resp["record"] = rec
			}
			vr, err := timestamp.Verify(ctx, store.ClientID(), []string{digest})
			if err != nil {
				resp["dcrtimeError"] = err.Error()
				return resp, nil
			}
			res, found := vr[digest]
			resp["dcrtime"] = res
			if found && res.TxID != "" {
				resp["validation"] = timestamp.ValidateProof(ctx, digest, res.MerkleRoot, res.MerklePath, res.TxID)
			}
			if resp["inArchive"] == true && found {
				timestamp.ApplyResult(store, digest, res)
			}
			return resp, nil
		}),
	readTool("timestamp", "timestamp_validate",
		"Validate a digest's proof on-chain via dcrd, looking the proof up from the local archive.",
		func(ctx context.Context, in timestampDigestInput) (any, error) {
			digest := normDigest(in.Digest)
			if !timestamp.ValidDigest(digest) {
				return nil, errors.New("digest must be a 64-character hex sha256")
			}
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			rec, err := store.Get(digest)
			if err != nil {
				return nil, errors.New("record not found")
			}
			return timestamp.ValidateProof(ctx, digest, rec.MerkleRoot, rec.MerklePath, rec.TxID), nil
		}),
	readTool("timestamp", "timestamp_status",
		"Report dcrtime feature health: whether enabled, the active network and host, and pending/total record counts.",
		func(ctx context.Context, _ emptyInput) (any, error) {
			network, _ := services.CurrentNetwork(ctx)
			if network == "" {
				network = "mainnet"
			}
			resp := map[string]any{
				"enabled": timestamp.Enabled(),
				"network": network,
				"host":    timestamp.APIHost(ctx),
			}
			if store, err := timestamp.Archive(); err == nil {
				resp["pending"] = len(store.PendingDigests())
				resp["total"] = len(store.All())
			}
			if timestamp.Enabled() {
				if err := timestamp.Reachable(ctx); err != nil {
					resp["reachable"] = false
					resp["reachableError"] = err.Error()
				} else {
					resp["reachable"] = true
				}
			}
			return resp, nil
		}),
	readTool("timestamp", "timestamp_proof",
		"Get the self-contained, exportable proof JSON for an anchored record.",
		func(ctx context.Context, in timestampDigestInput) (any, error) {
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			rec, err := store.Get(normDigest(in.Digest))
			if err != nil {
				return nil, errors.New("record not found")
			}
			if rec.Status != timestamp.StatusAnchored {
				return nil, errors.New("record is not anchored yet")
			}
			return timestamp.NewProof(rec, timestamp.ChainName(ctx), timestamp.APIHost(ctx)), nil
		}),
	readTool("timestamp", "timestamp_export",
		"Export every dcrtime timestamp record in the archive.",
		func(_ context.Context, _ emptyInput) (any, error) {
			store, err := timestamp.Archive()
			if err != nil {
				return nil, err
			}
			return store.All(), nil
		}),
	agentTool("timestamp", "timestamp_create",
		"Record a digest locally and submit it to dcrtime for anchoring. The file itself is never sent; pass its sha256 digest. Requires a grant with timestamp write enabled.",
		func(ctx context.Context, a *agent, in timestampCreateInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTimestamp, time.Now()); err != nil {
				recordSpend(a, "timestamp_create", 0, 0, in.Digest, "denied", err.Error())
				return nil, err
			}
			digest := normDigest(in.Digest)
			if !timestamp.ValidDigest(digest) {
				err := errors.New("digest must be a 64-character hex sha256")
				recordSpend(a, "timestamp_create", 0, 0, in.Digest, "error", err.Error())
				return nil, err
			}
			store, err := timestamp.Archive()
			if err != nil {
				recordSpend(a, "timestamp_create", 0, 0, digest, "error", err.Error())
				return nil, err
			}
			rec := &timestamp.Record{
				Digest:      digest,
				Filename:    in.Filename,
				Title:       in.Title,
				Description: in.Description,
				FileSize:    in.FileSize,
				MimeType:    in.MimeType,
				FileMtime:   in.FileMtime,
				Tags:        in.Tags,
				Status:      timestamp.StatusSubmitted,
			}
			if err := store.Create(rec); err != nil {
				if errors.Is(err, timestamp.ErrDuplicate) {
					existing, _ := store.Get(digest)
					recordSpend(a, "timestamp_create", 0, 0, digest, "error", "digest already in archive")
					return map[string]any{"error": "this file is already in your archive", "record": existing}, nil
				}
				recordSpend(a, "timestamp_create", 0, 0, digest, "error", err.Error())
				return nil, err
			}
			id := store.ClientID()
			results, err := timestamp.Submit(ctx, id, []string{digest})
			if err != nil {
				_ = store.Update(digest, func(rr *timestamp.Record) error {
					rr.Status = timestamp.StatusFailed
					rr.FailReason = err.Error()
					return nil
				})
			} else if code, found := results[digest]; found &&
				code != timestamp.ResultOK && code != timestamp.ResultExistsError {
				_ = store.Update(digest, func(rr *timestamp.Record) error {
					rr.Status = timestamp.StatusFailed
					rr.FailReason = "dcrtime: " + code.String()
					return nil
				})
			} else {
				if vr, verr := timestamp.Verify(ctx, id, []string{digest}); verr == nil {
					if res, ok := vr[digest]; ok {
						timestamp.ApplyResult(store, digest, res)
					}
				}
			}
			final, _ := store.Get(digest)
			recordSpend(a, "timestamp_create", 0, 0, digest, "ok", string(final.Status))
			return final, nil
		}),
	agentTool("timestamp", "timestamp_retry",
		"Re-submit a previously failed timestamp record to dcrtime, by digest. Requires a grant with timestamp write enabled.",
		func(ctx context.Context, a *agent, in timestampDigestInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTimestamp, time.Now()); err != nil {
				recordSpend(a, "timestamp_retry", 0, 0, in.Digest, "denied", err.Error())
				return nil, err
			}
			store, err := timestamp.Archive()
			if err != nil {
				recordSpend(a, "timestamp_retry", 0, 0, in.Digest, "error", err.Error())
				return nil, err
			}
			digest := normDigest(in.Digest)
			rec, err := store.Get(digest)
			if err != nil {
				recordSpend(a, "timestamp_retry", 0, 0, digest, "error", "record not found")
				return nil, errors.New("record not found")
			}
			id := store.ClientID()
			results, err := timestamp.Submit(ctx, id, []string{rec.Digest})
			if err != nil {
				_ = store.Update(digest, func(rr *timestamp.Record) error {
					rr.Status = timestamp.StatusFailed
					rr.FailReason = err.Error()
					return nil
				})
			} else if code, found := results[rec.Digest]; found &&
				code != timestamp.ResultOK && code != timestamp.ResultExistsError {
				_ = store.Update(digest, func(rr *timestamp.Record) error {
					rr.Status = timestamp.StatusFailed
					rr.FailReason = "dcrtime: " + code.String()
					return nil
				})
			} else {
				_ = store.Update(digest, func(rr *timestamp.Record) error {
					if rr.Status == timestamp.StatusFailed {
						rr.Status = timestamp.StatusSubmitted
						rr.FailReason = ""
					}
					return nil
				})
				if vr, verr := timestamp.Verify(ctx, id, []string{rec.Digest}); verr == nil {
					if res, ok := vr[rec.Digest]; ok {
						timestamp.ApplyResult(store, digest, res)
					}
				}
			}
			final, _ := store.Get(digest)
			recordSpend(a, "timestamp_retry", 0, 0, digest, "ok", string(final.Status))
			return final, nil
		}),
	agentTool("timestamp", "timestamp_delete",
		"Delete a timestamp record and its proof from the archive, by digest. Requires a grant with timestamp write enabled.",
		func(_ context.Context, a *agent, in timestampDigestInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTimestamp, time.Now()); err != nil {
				recordSpend(a, "timestamp_delete", 0, 0, in.Digest, "denied", err.Error())
				return nil, err
			}
			store, err := timestamp.Archive()
			if err != nil {
				recordSpend(a, "timestamp_delete", 0, 0, in.Digest, "error", err.Error())
				return nil, err
			}
			digest := normDigest(in.Digest)
			if err := store.Delete(digest); err != nil {
				if errors.Is(err, timestamp.ErrNotFound) {
					recordSpend(a, "timestamp_delete", 0, 0, digest, "error", "record not found")
					return nil, errors.New("record not found")
				}
				recordSpend(a, "timestamp_delete", 0, 0, digest, "error", err.Error())
				return nil, err
			}
			recordSpend(a, "timestamp_delete", 0, 0, digest, "ok", "")
			return map[string]any{"digest": digest, "deleted": true}, nil
		}),
	agentTool("timestamp", "timestamp_update",
		"Edit a timestamp record's user metadata (title, description, tags), by digest. Requires a grant with timestamp write enabled.",
		func(_ context.Context, a *agent, in timestampUpdateInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTimestamp, time.Now()); err != nil {
				recordSpend(a, "timestamp_update", 0, 0, in.Digest, "denied", err.Error())
				return nil, err
			}
			store, err := timestamp.Archive()
			if err != nil {
				recordSpend(a, "timestamp_update", 0, 0, in.Digest, "error", err.Error())
				return nil, err
			}
			digest := normDigest(in.Digest)
			err = store.Update(digest, func(rr *timestamp.Record) error {
				rr.Title = in.Title
				rr.Description = in.Description
				rr.Tags = in.Tags
				return nil
			})
			if errors.Is(err, timestamp.ErrNotFound) {
				recordSpend(a, "timestamp_update", 0, 0, digest, "error", "record not found")
				return nil, errors.New("record not found")
			}
			if err != nil {
				recordSpend(a, "timestamp_update", 0, 0, digest, "error", err.Error())
				return nil, err
			}
			rec, _ := store.Get(digest)
			recordSpend(a, "timestamp_update", 0, 0, digest, "ok", "")
			return rec, nil
		}),
	agentTool("timestamp", "timestamp_refresh",
		"Trigger an immediate dcrtime anchor poll, advancing not-yet-anchored records, then return the refreshed archive. Requires a grant with timestamp write enabled. One poll per 30 seconds, shared with the dashboard.",
		func(ctx context.Context, a *agent, _ emptyInput) (any, error) {
			if err := grants.authorizeAction(a.id, scopeTimestamp, time.Now()); err != nil {
				recordSpend(a, "timestamp_refresh", 0, 0, "", "denied", err.Error())
				return nil, err
			}
			if err := allow(middleware.TimestampRefresh); err != nil {
				return nil, err
			}
			timestamp.RefreshAnchors(ctx)
			store, err := timestamp.Archive()
			if err != nil {
				recordSpend(a, "timestamp_refresh", 0, 0, "", "error", err.Error())
				return nil, err
			}
			recordSpend(a, "timestamp_refresh", 0, 0, "", "ok", "")
			return store.List(timestamp.Query{}), nil
		}),
}
