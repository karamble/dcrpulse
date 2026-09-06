// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// goldenToolScopes pins the write scope each write tool actually enforces, read
// from the denial a scopeless grant produces. The gating test only proves a tool
// refuses *something* without a grant, so on its own it cannot see a tool
// re-pointed at a different scope; this map can.
//
// Three rows look surprising and are correct:
//   - br_tip_user is a Bison Relay tool gated on the Lightning scope, because it
//     is a Lightning payment and is counted against the Lightning cap.
//   - br_content_get gates on bisonrelay first and only then reserves against the
//     Lightning cap, so the first gate is what a scopeless grant reveals.
//   - dex_place_order uses dex rather than dex.spend. Both are fund scopes, so the
//     caps bind either way; do not "correct" this without checking the grant UI.
var goldenToolScopes = map[string]string{
	"br_content_get":                        scopeBR,
	"br_download_cancel":                    scopeBR,
	"br_download_delete":                    scopeBR,
	"br_file_add":                           scopeBR,
	"br_file_send":                          scopeBR,
	"br_file_send_path":                     scopeBR,
	"br_gc_admins":                          scopeBRAdmin,
	"br_gc_block":                           scopeBRAdmin,
	"br_gc_create":                          scopeBR,
	"br_gc_invite":                          scopeBR,
	"br_gc_invites_accept":                  scopeBR,
	"br_gc_kick":                            scopeBRAdmin,
	"br_gc_kill":                            scopeBRAdmin,
	"br_gc_owner":                           scopeBRAdmin,
	"br_gc_part":                            scopeBR,
	"br_gc_unblock":                         scopeBRAdmin,
	"br_notification_delete":                scopeBR,
	"br_notifications_clear":                scopeBR,
	"br_page_delete":                        scopeBR,
	"br_page_import_embed":                  scopeBR,
	"br_page_save":                          scopeBR,
	"br_page_submit":                        scopeBR,
	"br_post_comment":                       scopeBR,
	"br_post_create":                        scopeBR,
	"br_post_heart":                         scopeBR,
	"br_post_relay":                         scopeBR,
	"br_rtdt_accept":                        scopeBR,
	"br_rtdt_chat":                          scopeBR,
	"br_rtdt_create":                        scopeBR,
	"br_rtdt_create_instant":                scopeBR,
	"br_rtdt_dissolve":                      scopeBR,
	"br_rtdt_invite":                        scopeBR,
	"br_rtdt_join":                          scopeBR,
	"br_rtdt_kick":                          scopeBRAdmin,
	"br_rtdt_leave":                         scopeBR,
	"br_rtdt_remove":                        scopeBRAdmin,
	"br_send_groupchat_image":               scopeBR,
	"br_send_groupchat_message":             scopeBR,
	"br_send_message":                       scopeBR,
	"br_send_message_image":                 scopeBR,
	"br_shop_add_to_cart":                   scopeBR,
	"br_shop_clear_cart":                    scopeBR,
	"br_shop_order_comment":                 scopeBR,
	"br_shop_place_order":                   scopeBR,
	"br_store_delete_product":               scopeBR,
	"br_store_file_delete":                  scopeBR,
	"br_store_file_upload":                  scopeBR,
	"br_store_order_comment":                scopeBR,
	"br_store_order_status":                 scopeBR,
	"br_store_save_product":                 scopeBR,
	"br_store_template_delete":              scopeBR,
	"br_store_template_save":                scopeBR,
	"br_tip_user":                           scopeLightning,
	"br_unshare_file":                       scopeBR,
	"dex_add_peer":                          scopeDex,
	"dex_cancel_order":                      scopeDex,
	"dex_discover_account":                  scopeDex,
	"dex_mm_remove_config":                  scopeDex,
	"dex_mm_stop":                           scopeDex,
	"dex_mm_update_cex_config":              scopeDex,
	"dex_mm_update_config":                  scopeDex,
	"dex_place_order":                       scopeDex,
	"dex_post_bond":                         scopeDexSpend,
	"dex_remove_peer":                       scopeDex,
	"dex_set_bond_options":                  scopeDexSpend,
	"dex_wallet_close":                      scopeDex,
	"dex_wallet_open":                       scopeDex,
	"dex_wallet_rescan":                     scopeDex,
	"dex_wallet_toggle":                     scopeDex,
	"governance_cast_proposal_vote":         scopeGovernance,
	"governance_set_treasury_policy":        scopeGovernance,
	"governance_set_tspend_policy":          scopeGovernance,
	"governance_set_vote_choice":            scopeGovernance,
	"governance_vote_trickle_start":         scopeGovernance,
	"governance_vote_trickle_stop":          scopeGovernance,
	"ln_add_invoice":                        scopeLightning,
	"ln_autopilot_set":                      scopeLightning,
	"ln_cancel_invoice":                     scopeLightning,
	"ln_close_channel":                      scopeLightning,
	"ln_liquidity_request":                  scopeLightning,
	"ln_open_channel":                       scopeLightning,
	"ln_pay":                                scopeLightning,
	"ln_watchtower_add":                     scopeLightning,
	"ln_watchtower_remove":                  scopeLightning,
	"privacy_mixer_start":                   scopePrivacy,
	"privacy_mixer_stop":                    scopePrivacy,
	"staking_autobuyer_save_settings":       scopeStaking,
	"staking_autobuyer_stop":                scopeStaking,
	"staking_process_unmanaged_vsp_tickets": scopeStaking,
	"staking_sync_failed_vsp_tickets":       scopeStaking,
	"timestamp_create":                      scopeTimestamp,
	"timestamp_delete":                      scopeTimestamp,
	"timestamp_refresh":                     scopeTimestamp,
	"timestamp_retry":                       scopeTimestamp,
	"timestamp_update":                      scopeTimestamp,
	"tor_new_identity":                      scopeTor,
	"tor_set_settings":                      scopeTor,
	"wallet_broadcast_signed_transaction":   scopeWalletBroadcast,
}

// goldenAccountGatedTools are the write tools that key off the grant's accounts
// and caps instead of a named write scope, so they refuse on the account. Listed
// explicitly so the exception is declared rather than silently tolerated.
var goldenAccountGatedTools = []string{"staking_purchase", "wallet_send"}

// TestWriteToolScopesAreDeclared pins every write tool to the scope it enforces.
// The probe grant exists but carries no write scope and no account, so nothing
// can reach a real spend: a scope-gated tool names its scope and an
// account-gated one refuses on the account.
func TestWriteToolScopesAreDeclared(t *testing.T) {
	const agentID = "scope-census"
	grants.set(agentID, GrantSpec{PerTxAtoms: 1, DailyAtoms: 1}, time.Now())
	t.Cleanup(func() { grants.revoke(agentID) })

	accountGated := map[string]bool{}
	for _, n := range goldenAccountGatedTools {
		accountGated[n] = true
	}
	domains := map[string]bool{}
	for _, d := range catalogDomains() {
		domains[d] = true
	}
	cs := connectTo(t, testAgent(agentID, "scopes", domains))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	seen := map[string]bool{}
	writes := 0
	for _, tl := range res.Tools {
		if tl.Annotations != nil && tl.Annotations.ReadOnlyHint {
			continue
		}
		writes++
		seen[tl.Name] = true
		out, cerr := cs.CallTool(context.Background(), &mcp.CallToolParams{
			Name: tl.Name, Arguments: minimalArgs(tl.InputSchema),
		})
		if cerr != nil {
			t.Errorf("%s: unexpected transport error: %v", tl.Name, cerr)
			continue
		}
		txt := resultText(out)
		if accountGated[tl.Name] {
			if !strings.Contains(txt, errAccountNotGranted.Error()) {
				t.Errorf("%s is listed as account-gated but did not refuse on the account: %q", tl.Name, txt)
			}
			continue
		}
		want, ok := goldenToolScopes[tl.Name]
		if !ok {
			t.Errorf("%s is a write tool with no entry in goldenToolScopes. Add it with the scope it gates on, or list it in goldenAccountGatedTools if it keys off accounts.", tl.Name)
			continue
		}
		if !strings.Contains(txt, `"`+want+`" write scope`) {
			t.Errorf("%s does not gate on the %q scope any more: %q", tl.Name, want, txt)
		}
	}

	for name := range goldenToolScopes {
		if !seen[name] {
			t.Errorf("goldenToolScopes lists %q, which is no longer a write tool in the catalog; remove it.", name)
		}
	}
	for _, name := range goldenAccountGatedTools {
		if !seen[name] {
			t.Errorf("goldenAccountGatedTools lists %q, which is no longer a write tool in the catalog; remove it.", name)
		}
	}
	if want := len(goldenToolScopes) + len(goldenAccountGatedTools); writes != want {
		t.Errorf("catalog has %d write tools but the golden lists cover %d", writes, want)
	}
}
