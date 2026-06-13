// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package timestamp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"dcrpulse/internal/config"
	"dcrpulse/internal/services"
)

// httpClient reuses the dashboard's shared, Tor-aware transport so dcrtime
// calls follow the same routing as every other outbound request. Per-call
// timeouts come from the request context.
var httpClient = &http.Client{Transport: services.ExternalTransport()}

const (
	dcrtimeTimeout = 10 * time.Second
	maxReplyBytes  = 4 << 20
	maxAttempts    = 3
)

// ErrDisabled is returned when the user has turned off dcrtime external requests.
var ErrDisabled = errors.New("dcrtime requests are disabled")

// Enabled reports whether the dcrtime external-request toggle is on. Defaults to
// true when no global config or no explicit entry is present, matching the other
// external-request gates (see services.BrseederEnabled).
func Enabled() bool {
	gc, err := config.LoadGlobalCfg()
	if err != nil {
		return true
	}
	allowed, _ := gc.AllowedExternalRequests()
	if allowed == nil {
		return true
	}
	v, ok := allowed[config.ExternalRequestDcrtime]
	if !ok {
		return true
	}
	return v
}

// apiBaseURL selects the dcrtime host for the stack's active network.
func apiBaseURL(ctx context.Context) string {
	if n, err := services.CurrentNetwork(ctx); err == nil && n == "testnet" {
		return testnetAPIURL
	}
	return mainnetAPIURL
}

// APIHost returns the dcrtime API base URL for the active network. Exposed for
// status reporting and proof export.
func APIHost(ctx context.Context) string { return apiBaseURL(ctx) }

// ChainName returns the Decred chain label for the active network (e.g.
// "decred-mainnet"), used in exported proofs.
func ChainName(ctx context.Context) string {
	if n, err := services.CurrentNetwork(ctx); err == nil && n == "testnet" {
		return "decred-testnet"
	}
	return "decred-mainnet"
}

// DigestResult is the processed status of one digest from a Verify call.
type DigestResult struct {
	Digest           string          `json:"digest"`
	Found            bool            `json:"found"`
	State            ChainState      `json:"state"`
	AnchorTime       int64           `json:"anchorTime"` // chaintimestamp (unix seconds); 0 until anchored
	MerkleRoot       string          `json:"merkleRoot,omitempty"`
	MerklePath       json.RawMessage `json:"merklePath,omitempty"` // verbatim dcrtime proof
	TxID             string          `json:"txId,omitempty"`       // anchor tx; "" while awaiting (zero hash)
	Confirmations    int32           `json:"confirmations"`
	MinConfirmations int32           `json:"minConfirmations"`
}

// Submit anchors digests with dcrtime. The returned map is keyed by digest with
// its per-digest result code. ResultExistsError is a normal outcome (the digest
// was submitted before, by anyone) and is not treated as an error here.
func Submit(ctx context.Context, id string, digests []string) (map[string]ResultT, error) {
	if !Enabled() {
		return nil, ErrDisabled
	}
	var reply timestampBatchReply
	err := postJSON(ctx, routeTimestampBatch, timestampBatch{ID: id, Digests: digests}, &reply)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ResultT, len(reply.Digests))
	for i, d := range reply.Digests {
		if i < len(reply.Results) {
			out[d] = reply.Results[i]
		}
	}
	return out, nil
}

// Verify fetches the current status and proof for digests. Digests dcrtime does
// not know are returned with Found=false / StateNotFound.
func Verify(ctx context.Context, id string, digests []string) (map[string]DigestResult, error) {
	if !Enabled() {
		return nil, ErrDisabled
	}
	var reply verifyBatchReply
	err := postJSON(ctx, routeVerifyBatch, verifyBatch{ID: id, Digests: digests}, &reply)
	if err != nil {
		return nil, err
	}
	out := make(map[string]DigestResult, len(reply.Digests))
	for _, vd := range reply.Digests {
		ci := vd.ChainInformation
		r := DigestResult{
			Digest:           vd.Digest,
			Found:            vd.Result == ResultOK,
			State:            vd.state(),
			AnchorTime:       ci.ChainTimestamp,
			MerkleRoot:       ci.MerkleRoot,
			MerklePath:       ci.MerklePath,
			Confirmations:    derefInt32(ci.Confirmations),
			MinConfirmations: ci.MinConfirmations,
		}
		if ci.Transaction != "" && ci.Transaction != zeroHash {
			r.TxID = ci.Transaction
		}
		out[vd.Digest] = r
	}
	return out, nil
}

// Reachable posts a status ping and returns nil if dcrtime answers.
func Reachable(ctx context.Context) error {
	if !Enabled() {
		return ErrDisabled
	}
	return postJSON(ctx, routeStatus, statusPing{ID: "dcrpulse"}, nil)
}

type statusPing struct {
	ID string `json:"id"`
}

// postJSON POSTs reqBody as JSON to the active dcrtime host + route, decoding the
// reply into out (out may be nil). Transient failures are retried with backoff;
// dcrtime submit/verify are idempotent so retrying never duplicates work.
func postJSON(ctx context.Context, route string, reqBody, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	base := apiBaseURL(ctx)
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		cctx, cancel := context.WithTimeout(ctx, dcrtimeTimeout)
		req, rerr := http.NewRequestWithContext(cctx, http.MethodPost, base+route, bytes.NewReader(payload))
		if rerr != nil {
			cancel()
			return rerr
		}
		req.Header.Set("Content-Type", "application/json")
		resp, derr := httpClient.Do(req)
		if derr != nil {
			cancel()
			lastErr = derr
			continue
		}
		body, berr := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
		resp.Body.Close()
		cancel()
		if berr != nil {
			lastErr = berr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("dcrtime %s: status %d: %s", route, resp.StatusCode, snippet(body))
			continue
		}
		if out != nil {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decode dcrtime %s reply: %w", route, err)
			}
		}
		return nil
	}
	return lastErr
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// snippet trims a response body for inclusion in an error message.
func snippet(b []byte) string {
	const max = 200
	s := string(bytes.TrimSpace(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
