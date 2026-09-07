// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package utils

import "strings"

// SafeBRPath reports whether p is a safe relative name to hand to brclientd.
// Rejects an empty or over-long name, NUL and backslash, an absolute path, and
// any ".." segment. Shared by the dashboard routes and the agent tools so the
// two surfaces cannot drift apart.
func SafeBRPath(p string) bool {
	if p == "" || len(p) > 255 {
		return false
	}
	if strings.ContainsRune(p, 0) || strings.ContainsRune(p, '\\') || strings.HasPrefix(p, "/") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// SafeStoreMediaPath is SafeBRPath plus a denylist for the store's file
// endpoints: .tmpl/.tmp must never be created or served as "media" because the
// store parses and executes *.tmpl as Go templates. The template routes
// deliberately use SafeBRPath instead, since a template legitimately ends in
// .tmpl.
func SafeStoreMediaPath(p string) bool {
	if !SafeBRPath(p) {
		return false
	}
	lower := strings.ToLower(strings.TrimRight(p, ". "))
	return !strings.HasSuffix(lower, ".tmpl") && !strings.HasSuffix(lower, ".tmp")
}
