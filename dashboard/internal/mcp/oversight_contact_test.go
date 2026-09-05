// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import "testing"

const overseer = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"

func contactEntry(uid, nick, alias, name string) map[string]any {
	return map[string]any{
		"id":         map[string]any{"identity": uid, "nick": nick, "name": name},
		"nick_alias": alias,
	}
}

// The BR tools take a nick, alias or hex uid and let brclientd resolve it, so a
// refusal that only knows the stored hex is walked around by passing the nick.
func TestNamesOversightContactCoversEveryName(t *testing.T) {
	entries := []map[string]any{
		contactEntry(overseer, "phone", "myphone", "Operator Phone"),
		contactEntry("ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111", "someoneelse", "friend", "A Friend"),
	}
	for _, tc := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "hex uid", target: overseer, want: true},
		{name: "hex uid, different case", target: "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F90", want: true},
		{name: "nick", target: "phone", want: true},
		{name: "local alias", target: "myphone", want: true},
		{name: "display name", target: "Operator Phone", want: true},
		{name: "nick with padding", target: "  phone  ", want: true},
		{name: "another contact's uid", target: "ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111ffff1111"},
		{name: "another contact's nick", target: "someoneelse"},
		{name: "another contact's alias", target: "friend"},
		{name: "unknown name", target: "nobody"},
		{name: "empty", target: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := namesOversightContact(tc.target, overseer, entries); got != tc.want {
				t.Errorf("namesOversightContact(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

// A contact whose entry carries no nick or alias must still be matched by uid,
// and empty fields must never match an empty-ish target.
func TestNamesOversightContactHandlesSparseEntries(t *testing.T) {
	entries := []map[string]any{contactEntry(overseer, "", "", "")}
	if !namesOversightContact(overseer, overseer, entries) {
		t.Error("the uid must match even when the contact has no names")
	}
	if namesOversightContact("", overseer, entries) {
		t.Error("an empty target must not match empty name fields")
	}
	if namesOversightContact("   ", overseer, entries) {
		t.Error("a blank target must not match empty name fields")
	}
}

func TestNamesOversightContactIgnoresMalformedEntries(t *testing.T) {
	entries := []map[string]any{
		{"nick_alias": "orphan"},
		{"id": "not a map"},
		contactEntry(overseer, "phone", "", ""),
	}
	if !namesOversightContact("phone", overseer, entries) {
		t.Error("a well-formed entry must still match past malformed ones")
	}
	if namesOversightContact("orphan", overseer, entries) {
		t.Error("an entry with no uid must not match the contact")
	}
}
