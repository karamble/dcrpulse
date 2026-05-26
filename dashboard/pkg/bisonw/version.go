// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bisonw

import "context"

// Semver is the bisonw RPC server version. Note the JSON keys are capitalized:
// it is the default marshaling of decred.org/dcrdex/dex.Semver (no struct tags).
type Semver struct {
	Major uint32 `json:"Major"`
	Minor uint32 `json:"Minor"`
	Patch uint32 `json:"Patch"`
}

// BisonwVersion is the bisonw application version (the "dexcVersion" field).
type BisonwVersion struct {
	VersionString string `json:"versionString"`
	Major         uint32 `json:"major"`
	Minor         uint32 `json:"minor"`
	Patch         uint32 `json:"patch"`
	Prerelease    string `json:"prerelease,omitempty"`
	BuildMetadata string `json:"buildMetadata,omitempty"`
}

// VersionResult is the result of the "version" route. The route does not
// require the app to be initialized, so it doubles as a connectivity/health
// check.
type VersionResult struct {
	RPCServerVersion *Semver        `json:"rpcServerVersion"`
	Bisonw           *BisonwVersion `json:"dexcVersion"`
}

// Version returns the bisonw and RPC server versions.
func (c *Client) Version(ctx context.Context) (*VersionResult, error) {
	var res VersionResult
	if err := c.Call(ctx, "version", nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
