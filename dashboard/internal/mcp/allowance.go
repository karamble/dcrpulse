// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"errors"

	"dcrpulse/internal/middleware"
)

// allow refuses a call while the job's shared bucket is empty. The buckets are
// the ones main.go puts in front of the browser routes for the same jobs, so an
// agent and the operator draw one allowance between them.
func allow(a middleware.Allowance) error {
	if !a.Limiter().Allow() {
		return errors.New("rate limit exceeded, retry later")
	}
	return nil
}
