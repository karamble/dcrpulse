// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "testing"

// An empty variable must fall back, not be treated as a set value: a compose
// file that declares a key with no value should behave as if it were unset.
func TestEnvOr(t *testing.T) {
	const key = "DCRPULSE_TEST_ENVOR"

	if got := EnvOr(key, "fallback"); got != "fallback" {
		t.Errorf("unset: EnvOr = %q, want fallback", got)
	}
	t.Setenv(key, "")
	if got := EnvOr(key, "fallback"); got != "fallback" {
		t.Errorf("set but empty: EnvOr = %q, want fallback", got)
	}
	t.Setenv(key, "value")
	if got := EnvOr(key, "fallback"); got != "value" {
		t.Errorf("set: EnvOr = %q, want value", got)
	}
}
