// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import (
	"math"
	"testing"
)

// Expected strings are written out by hand. The DCR amount formatters that
// used to live here were removed rather than repaired: they split a value into
// integer and fractional parts and lost the carry when the fraction rounded up.
// Amounts now reach the browser as numbers converted with dcrutil, so what is
// left to guard is digit grouping.
func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"zero", 0, "0"},
		{"single digit", 7, "7"},
		{"three digits stay ungrouped", 999, "999"},
		{"first grouped value", 1000, "1,000"},
		{"four digits", 1234, "1,234"},
		{"five digits", 12345, "12,345"},
		{"six digits", 123456, "123,456"},
		{"seven digits", 1234567, "1,234,567"},
		{"exact million", 1000000, "1,000,000"},
		{"block height", 1104276, "1,104,276"},

		// The sign used to occupy a digit position, so a six-digit negative
		// gained a leading separator: -123456 rendered as "-,123,456".
		{"negative single digit", -7, "-7"},
		{"negative three digits", -999, "-999"},
		{"negative four digits", -1234, "-1,234"},
		{"negative five digits", -12345, "-12,345"},
		{"negative six digits", -123456, "-123,456"},
		{"negative seven digits", -1234567, "-1,234,567"},

		// Magnitude has no positive counterpart, so negating up front would
		// overflow. Digits are negated one at a time instead.
		{"max int64", math.MaxInt64, "9,223,372,036,854,775,807"},
		{"min int64", math.MinInt64, "-9,223,372,036,854,775,808"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatNumber(tc.n); got != tc.want {
				t.Errorf("FormatNumber(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}

// Every group after the first must hold exactly three digits, whatever the
// length or sign.
func TestFormatNumberGroupsInThrees(t *testing.T) {
	for _, n := range []int64{1000, -1000, 999999, -999999, 1000000000, -1000000000} {
		t.Run(FormatNumber(n), func(t *testing.T) {
			s := FormatNumber(n)
			if s[0] == ',' || (s[0] == '-' && s[1] == ',') {
				t.Fatalf("FormatNumber(%d) = %q starts with a separator", n, s)
			}

			digits := s
			if digits[0] == '-' {
				digits = digits[1:]
			}
			groups := []string{}
			start := 0
			for i := 0; i <= len(digits); i++ {
				if i == len(digits) || digits[i] == ',' {
					groups = append(groups, digits[start:i])
					start = i + 1
				}
			}
			if len(groups) < 2 {
				t.Fatalf("FormatNumber(%d) = %q was not grouped", n, s)
			}
			for i, g := range groups {
				if g == "" {
					t.Fatalf("FormatNumber(%d) = %q has an empty group", n, s)
				}
				if i > 0 && len(g) != 3 {
					t.Errorf("FormatNumber(%d) = %q: group %d is %q, want 3 digits", n, s, i, g)
				}
			}
		})
	}
}
