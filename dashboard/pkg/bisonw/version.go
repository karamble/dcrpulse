// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import (
	"context"
	"encoding/json"
)

// VersionResult is the result of the "version" route. The fields are kept as
// raw JSON because the underlying dex.Semver / SemVersion shapes are an upstream
// detail; callers can decode them as needed. The version route does not require
// the app to be initialized, so it doubles as a connectivity/health check.
type VersionResult struct {
	RPCServerVersion json.RawMessage `json:"rpcServerVersion"`
	BisonwVersion    json.RawMessage `json:"dexcVersion"`
}

// Version returns the bisonw and RPC server versions.
func (c *Client) Version(ctx context.Context) (*VersionResult, error) {
	var res VersionResult
	if err := c.Call(ctx, "version", nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
