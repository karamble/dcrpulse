// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"strings"
)

// FormatDuration formats a duration in seconds to a human-readable string
func FormatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	remainingMinutes := minutes % 60
	if hours < 24 {
		if remainingMinutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, remainingMinutes)
	}
	days := hours / 24
	remainingHours := hours % 24
	if remainingHours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, remainingHours)
}

// FormatTraffic formats bytes to a human-readable traffic string
func FormatTraffic(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.2f MB", mb)
	}
	gb := mb / 1024
	return fmt.Sprintf("%.2f GB", gb)
}

// ExtractDcrdVersion extracts the dcrd version from subver string
func ExtractDcrdVersion(subver string) string {
	// subver format: "/dcrwire:1.0.0/dcrd:2.0.5/"
	if idx := strings.Index(subver, "dcrd:"); idx != -1 {
		start := idx + 5
		end := strings.Index(subver[start:], "/")
		if end != -1 {
			return subver[start : start+end]
		}
		return subver[start:]
	}
	return "unknown"
}

// FormatNumber formats a number with thousands separators
func FormatNumber(n int64) string {
	return addCommas(n)
}

// addCommas groups a number's digits in threes. The sign is held aside so it
// does not occupy a digit position and push a separator to the front.
func addCommas(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
	}

	// Build from the least significant digit so grouping needs no length math.
	// Negation is done per digit to stay correct at math.MinInt64, whose
	// magnitude has no positive counterpart.
	var digits []byte
	for {
		d := n % 10
		if d < 0 {
			d = -d
		}
		digits = append(digits, byte('0'+d))
		n /= 10
		if n == 0 {
			break
		}
	}

	var b strings.Builder
	b.WriteString(sign)
	for i := len(digits) - 1; i >= 0; i-- {
		b.WriteByte(digits[i])
		if i > 0 && i%3 == 0 {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// FormatHashrate formats hashrate to a human-readable string
func FormatHashrate(hashrate float64) string {
	units := []string{"H/s", "KH/s", "MH/s", "GH/s", "TH/s", "PH/s", "EH/s"}
	unitIndex := 0

	for hashrate >= 1000 && unitIndex < len(units)-1 {
		hashrate /= 1000
		unitIndex++
	}

	return fmt.Sprintf("%.2f %s", hashrate, units[unitIndex])
}
