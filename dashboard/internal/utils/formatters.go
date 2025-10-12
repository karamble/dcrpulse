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

// FormatDCRAmount formats a DCR amount with commas and appropriate precision
func FormatDCRAmount(amount float64) string {
	// Format with 0 decimal places for large amounts
	intAmount := int64(amount)
	return AddCommas(intAmount)
}

// FormatDCRAmountWithDecimals formats a DCR amount with commas and specified decimal places
func FormatDCRAmountWithDecimals(amount float64, decimals int) string {
	// Split into integer and decimal parts
	intPart := int64(amount)
	decimalPart := amount - float64(intPart)

	// Format integer part with commas
	intStr := AddCommas(intPart)

	// Format decimal part
	format := fmt.Sprintf("%%.%df", decimals)
	fullStr := fmt.Sprintf(format, decimalPart)

	// Extract just the decimal part (after the "0.")
	decimalStr := fullStr[2:] // Skip "0."

	return fmt.Sprintf("%s.%s", intStr, decimalStr)
}

// FormatNumber formats a number with thousands separators
func FormatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return AddCommas(n)
}

// AddCommas adds comma separators to a number
func AddCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result string
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(digit)
	}
	return result
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
