// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package services

import "testing"

// withHooks isolates the hook list and the active wallet name, restoring both
// so one test cannot leave a hook installed for the next.
func withHooks(t *testing.T, start string) {
	t.Helper()
	activeWalletHooksMu.Lock()
	prevHooks := activeWalletHooks
	activeWalletHooks = nil
	activeWalletHooksMu.Unlock()

	activeWalletMu.Lock()
	prevName := activeWallet
	activeWallet = start
	activeWalletMu.Unlock()

	t.Cleanup(func() {
		activeWalletHooksMu.Lock()
		activeWalletHooks = prevHooks
		activeWalletHooksMu.Unlock()
		activeWalletMu.Lock()
		activeWallet = prevName
		activeWalletMu.Unlock()
	})
}

// The hook exists so a subscriber can drop authority that belonged to the old
// wallet. It must therefore see the transition, and must be able to read the
// active wallet without deadlocking on the lock the writer just released. A
// hook that ran before the write would read the wallet it is meant to abandon.
func TestSetActiveWalletNameNotifiesHooks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to string
	}{
		{name: "switch to another wallet", from: "alpha", to: "beta"},
		{name: "deselect", from: "alpha", to: ""},
		{name: "select from none", from: "", to: "alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withHooks(t, tc.from)

			var got []ActiveWalletChange
			var seen []string
			OnActiveWalletChange(func(ch ActiveWalletChange) {
				got = append(got, ch)
				seen = append(seen, ActiveWalletName())
			})

			setActiveWalletName(tc.to)

			if len(got) != 1 {
				t.Fatalf("hook ran %d times, want exactly 1", len(got))
			}
			if got[0].Old != tc.from || got[0].New != tc.to {
				t.Errorf("hook got %+v, want {Old:%q New:%q}", got[0], tc.from, tc.to)
			}
			if seen[0] != tc.to {
				t.Errorf("hook read the active wallet as %q, want the new name %q; "+
					"a hook that runs before the write cannot drop the old wallet's state",
					seen[0], tc.to)
			}
		})
	}
}

// SwitchWallet falls through to SetActiveWallet when the name is already active
// but the wallet is not loaded, which relaunches the daemon against whatever
// database now sits at that path. Skipping the hook on an unchanged name would
// carry a grant across that.
func TestSetActiveWalletNameNotifiesWhenUnchanged(t *testing.T) {
	withHooks(t, "alpha")

	var got []ActiveWalletChange
	OnActiveWalletChange(func(ch ActiveWalletChange) { got = append(got, ch) })

	setActiveWalletName("alpha")

	if len(got) != 1 {
		t.Fatalf("hook ran %d times on an unchanged name, want exactly 1", len(got))
	}
	if got[0].Old != "alpha" || got[0].New != "alpha" {
		t.Errorf("hook got %+v, want both names alpha", got[0])
	}
}

// Registration appends rather than replaces: a later registrant must not be
// able to silently displace a hook that drops spend authority.
func TestOnActiveWalletChangeAppends(t *testing.T) {
	withHooks(t, "alpha")

	var order []string
	OnActiveWalletChange(func(ActiveWalletChange) { order = append(order, "first") })
	OnActiveWalletChange(func(ActiveWalletChange) { order = append(order, "second") })

	setActiveWalletName("beta")

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hooks ran as %v, want [first second]", order)
	}
}
