// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCheckBRPageFields(t *testing.T) {
	got := CheckBRPageFields([]BRPageFieldCheck{
		{Pattern: ".+", Value: ""},
		{Pattern: ".+", Value: "x"},
		{Pattern: `^\d{3}$`, Value: "123"},
		{Pattern: `^\d{3}$`, Value: "12a"},
		{Pattern: "", Value: "anything"},
		{Pattern: "(?=a)a", Value: "b"},
		{Pattern: "(", Value: "b"},
		{Pattern: "^" + strings.Repeat("a", 200) + "$", Value: "b"},
	})
	want := []bool{false, true, true, false, true, true, true, true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A page picks the pattern; a nested quantifier that backtracks for minutes in
// a browser must be decided quickly.
func TestCheckBRPageFieldsIsLinear(t *testing.T) {
	start := time.Now()
	got := CheckBRPageFields([]BRPageFieldCheck{{Pattern: "^(a+)+$", Value: strings.Repeat("a", 4000) + "!"}})
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("check took %s", elapsed)
	}
	if got[0] {
		t.Error("value that does not match was accepted")
	}
}
