// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"dcrpulse/internal/services"
	"dcrpulse/internal/timestamp"
)

// reqCtx bounds a handler's dcrtime/dcrd work so the response returns well within
// the frontend's request timeout even when dcrtime is slow; the background worker
// reconciles anything still in flight.
func reqCtx(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

// timestampArchive returns the shared store or writes a 500 and reports false.
func timestampArchive(w http.ResponseWriter) (*timestamp.Store, bool) {
	store, err := timestamp.Archive()
	if err != nil {
		log.Printf("timestamp: open archive: %v", err)
		http.Error(w, "failed to open timestamp archive", http.StatusInternalServerError)
		return nil, false
	}
	return store, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

type createTimestampRequest struct {
	Digest      string   `json:"digest"`
	Filename    string   `json:"filename"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	FileSize    int64    `json:"fileSize"`
	MimeType    string   `json:"mimeType"`
	FileMtime   string   `json:"fileMtime"`
	Tags        []string `json:"tags"`
}

// CreateTimestampHandler records a digest locally and submits it to dcrtime. The
// file itself is never sent; the browser hashes it and posts only the digest.
func CreateTimestampHandler(w http.ResponseWriter, r *http.Request) {
	var req createTimestampRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	digest := strings.ToLower(strings.TrimSpace(req.Digest))
	if !timestamp.ValidDigest(digest) {
		http.Error(w, "digest must be a 64-character hex sha256", http.StatusBadRequest)
		return
	}
	store, ok := timestampArchive(w)
	if !ok {
		return
	}

	rec := &timestamp.Record{
		Digest:      digest,
		Filename:    req.Filename,
		Title:       req.Title,
		Description: req.Description,
		FileSize:    req.FileSize,
		MimeType:    req.MimeType,
		FileMtime:   req.FileMtime,
		Tags:        req.Tags,
		Status:      timestamp.StatusSubmitted,
	}
	if err := store.Create(rec); err != nil {
		if errors.Is(err, timestamp.ErrDuplicate) {
			existing, _ := store.Get(digest)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error":  "this file is already in your archive",
				"record": existing,
			})
			return
		}
		log.Printf("timestamp: create %s: %v", digest, err)
		http.Error(w, "failed to save record", http.StatusInternalServerError)
		return
	}

	ctx, cancel := reqCtx(r, 15*time.Second)
	defer cancel()
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
		// Accepted (OK or already Exists). Verify once immediately so a digest
		// that was anchored earlier (by anyone) reflects its real anchor now.
		if vr, verr := timestamp.Verify(ctx, id, []string{digest}); verr == nil {
			if res, ok := vr[digest]; ok {
				timestamp.ApplyResult(store, digest, res)
			}
		}
	}

	final, _ := store.Get(digest)
	writeJSON(w, final)
}

// ListTimestampsHandler returns the archive, filtered/sorted by query params.
func ListTimestampsHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	records := store.List(timestamp.Query{
		Text:   q.Get("q"),
		Status: timestamp.Status(q.Get("status")),
		Tag:    q.Get("tag"),
		Sort:   q.Get("sort"),
	})
	writeJSON(w, records)
}

// GetTimestampHandler returns one record.
func GetTimestampHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	rec, err := store.Get(digestVar(r))
	if err != nil {
		http.Error(w, "record not found", http.StatusNotFound)
		return
	}
	writeJSON(w, rec)
}

type updateTimestampRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// UpdateTimestampHandler edits the user-supplied metadata only.
func UpdateTimestampHandler(w http.ResponseWriter, r *http.Request) {
	var req updateTimestampRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	digest := digestVar(r)
	err := store.Update(digest, func(rr *timestamp.Record) error {
		rr.Title = req.Title
		rr.Description = req.Description
		rr.Tags = req.Tags
		return nil
	})
	if errors.Is(err, timestamp.ErrNotFound) {
		http.Error(w, "record not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to update record", http.StatusInternalServerError)
		return
	}
	rec, _ := store.Get(digest)
	writeJSON(w, rec)
}

// DeleteTimestampHandler removes a record and its proof from the archive.
func DeleteTimestampHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	err := store.Delete(digestVar(r))
	if errors.Is(err, timestamp.ErrNotFound) {
		http.Error(w, "record not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to delete record", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RetryTimestampHandler re-submits a previously failed record.
func RetryTimestampHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	digest := digestVar(r)
	rec, err := store.Get(digest)
	if err != nil {
		http.Error(w, "record not found", http.StatusNotFound)
		return
	}
	ctx, cancel := reqCtx(r, 15*time.Second)
	defer cancel()
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
	writeJSON(w, final)
}

// TimestampProofHandler downloads the self-contained proof JSON for a record.
func TimestampProofHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	rec, err := store.Get(digestVar(r))
	if err != nil {
		http.Error(w, "record not found", http.StatusNotFound)
		return
	}
	if rec.Status != timestamp.StatusAnchored {
		http.Error(w, "record is not anchored yet", http.StatusConflict)
		return
	}
	ctx := r.Context()
	proof := timestamp.NewProof(rec, timestamp.ChainName(ctx), timestamp.APIHost(ctx))
	name := fmt.Sprintf("timestamp-proof-%s.json", short(rec.Digest))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	json.NewEncoder(w).Encode(proof)
}

type verifyTimestampRequest struct {
	Digest string `json:"digest"`
}

// VerifyTimestampHandler checks a digest against the local archive, dcrtime, and
// (when anchored) the Decred chain via dcrpulse's own dcrd.
func VerifyTimestampHandler(w http.ResponseWriter, r *http.Request) {
	var req verifyTimestampRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	digest := strings.ToLower(strings.TrimSpace(req.Digest))
	if !timestamp.ValidDigest(digest) {
		http.Error(w, "digest must be a 64-character hex sha256", http.StatusBadRequest)
		return
	}
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	ctx, cancel := reqCtx(r, 18*time.Second)
	defer cancel()
	resp := map[string]any{"digest": digest, "inArchive": false}
	if rec, err := store.Get(digest); err == nil {
		resp["inArchive"] = true
		resp["record"] = rec
	}

	vr, err := timestamp.Verify(ctx, store.ClientID(), []string{digest})
	if err != nil {
		resp["dcrtimeError"] = err.Error()
		writeJSON(w, resp)
		return
	}
	res, found := vr[digest]
	resp["dcrtime"] = res
	if found && res.TxID != "" {
		resp["validation"] = timestamp.ValidateProof(ctx, digest, res.MerkleRoot, res.MerklePath, res.TxID)
	}
	// Keep a local record's status in step with what dcrtime just reported.
	if resp["inArchive"] == true && found {
		timestamp.ApplyResult(store, digest, res)
	}
	writeJSON(w, resp)
}

type validateTimestampRequest struct {
	Digest     string          `json:"digest"`
	MerkleRoot string          `json:"merkleRoot"`
	MerklePath json.RawMessage `json:"merklePath"`
	TxID       string          `json:"txId"`
}

// ValidateTimestampHandler validates a proof on-chain via dcrd. The proof may be
// supplied in the body (Explorer tool / imported proof JSON) or, when only a
// digest is given, looked up from the archive.
func ValidateTimestampHandler(w http.ResponseWriter, r *http.Request) {
	var req validateTimestampRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	digest := strings.ToLower(strings.TrimSpace(req.Digest))
	if (req.MerkleRoot == "" || req.TxID == "" || len(req.MerklePath) == 0) && timestamp.ValidDigest(digest) {
		if store, err := timestamp.Archive(); err == nil {
			if rec, err := store.Get(digest); err == nil {
				req.MerkleRoot, req.TxID, req.MerklePath = rec.MerkleRoot, rec.TxID, rec.MerklePath
			}
		}
	}
	ctx, cancel := reqCtx(r, 15*time.Second)
	defer cancel()
	val := timestamp.ValidateProof(ctx, digest, req.MerkleRoot, req.MerklePath, req.TxID)
	writeJSON(w, val)
}

// RefreshTimestampsHandler triggers an immediate anchor poll, then returns the
// refreshed archive. Used by the tab's manual refresh.
func RefreshTimestampsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r, 20*time.Second)
	defer cancel()
	timestamp.RefreshAnchors(ctx)
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	writeJSON(w, store.List(timestamp.Query{Sort: r.URL.Query().Get("sort")}))
}

// TimestampStatusHandler reports feature health for the tab header. Pass ?ping=1
// to additionally probe dcrtime reachability (a network round-trip).
func TimestampStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r, 12*time.Second)
	defer cancel()
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
	if r.URL.Query().Get("ping") == "1" && timestamp.Enabled() {
		if err := timestamp.Reachable(ctx); err != nil {
			resp["reachable"] = false
			resp["reachableError"] = err.Error()
		} else {
			resp["reachable"] = true
		}
	}
	writeJSON(w, resp)
}

// ExportTimestampsHandler downloads the full archive as a JSON array.
func ExportTimestampsHandler(w http.ResponseWriter, r *http.Request) {
	store, ok := timestampArchive(w)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dcrpulse-timestamps.json\"")
	json.NewEncoder(w).Encode(store.All())
}

func digestVar(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(mux.Vars(r)["digest"]))
}

func short(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
