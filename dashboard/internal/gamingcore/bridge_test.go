// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gamingcore

import (
	"reflect"
	"testing"
)

// newTestBridge is a bridge with no host, keeping its files in a directory of
// its own.
func newTestBridge(t *testing.T) *Bridge {
	t.Helper()
	return New(t.TempDir(), Host{})
}

// Tests stub the staged calls, so only this sees one New leaves unset. The
// listener's hooks wait for Start.
func TestNewWiresEveryStagedCall(t *testing.T) {
	v := reflect.ValueOf(newTestBridge(t)).Elem()
	for i := range v.NumField() {
		f := v.Type().Field(i)
		switch f.Name {
		case "gamingConnected", "gamingRequest", "gamingState", "gamingLockTerms":
			continue
		}
		if f.Type.Kind() == reflect.Func && v.Field(i).IsNil() {
			t.Errorf("New leaves %s unset", f.Name)
		}
	}
}
