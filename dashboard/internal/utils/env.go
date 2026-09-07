// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "os"

// EnvOr returns the environment value for key, or def when it is unset or
// empty. Shared so the dashboard and the agent surface read their defaults the
// same way.
func EnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
