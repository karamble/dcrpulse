# AI Agents (MCP)

dcrpulse can expose your node, wallet, and the rest of the dashboard to AI agents
over the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). An agent
connects with its own bearer token and, by default, can read only node and
blockchain status. Everything else (wallet, staking, Lightning, DEX, Bison Relay,
and any ability to move funds) is granted by you, per agent, and is bounded by hard
limits you set. The agent never receives your wallet passphrase.

## Overview

The MCP server runs in-process inside the dashboard and reuses the same code paths
as the web UI, so an agent sees the same data and can perform the same actions you
can, subject to the access you grant it.

The security model has four layers:

- **Per-agent tokens.** Each agent has its own named bearer token. Only a SHA-256
  hash is stored; the plaintext is shown once.
- **Capability domains.** A new agent can use only the `node` domain. You grant
  other domains (wallet, staking, etc.) per agent in the dashboard.
- **Spend grants.** Moving funds additionally requires an account-scoped grant with
  literal per-transaction and daily caps. You enter the passphrase once; it is held
  in memory and used on the agent's behalf, never sent to the agent.
- **Tripwire, freeze, and optional Bison Relay approvals.** A cap violation blocks
  the offending agent; a one-click freeze blocks all agents; and you can require a
  Bison Relay approval before every fund move.

The listener is bound to localhost by default and is never exposed by the Umbrel or
CasaOS app definitions.

## Enabling the server

Open **Settings -> AI Agents** and toggle the MCP server on. The toggle is persisted
and survives restarts.

The listener address is configured by environment variables on the `dashboard`
service:

| Variable | Default | Meaning |
| --- | --- | --- |
| `MCP_ENABLE` | `false` | First-run default for the on/off state (the persisted toggle wins after that). |
| `MCP_BIND` | `127.0.0.1` | Interface the listener binds to. |
| `MCP_PORT` | `8090` | Listener port. |

The bundled `docker-compose.yml` sets `MCP_ENABLE=true` and `MCP_BIND=0.0.0.0` on the
dashboard container, and publishes the port to the host loopback only:

```yaml
ports:
  - "127.0.0.1:8090:8090"   # MCP listener, localhost only
```

Security: the MCP port grants programmatic control of your wallet (within the limits
you set). Keep it on localhost. Expose it beyond this machine only behind an
authenticated, TLS-terminating reverse proxy, and remember that each agent still
needs its own token and starts limited to node status.

## Creating an agent token

In **Settings -> AI Agents -> Create an agent token**, enter a name. The dashboard
returns the plaintext token exactly once, in the form `mcp_<random>`. Copy it then;
only its hash is stored and it cannot be recovered later.

A newly created agent starts with the `node` domain only.

## Granting access: domains and scopes

Access has two independent axes.

### Capability domains (visibility)

A domain controls which tools and resources an agent can see at all. Grant them per
agent under **Settings -> AI Agents** (toggle the domains for an agent). A node-only
agent literally cannot see wallet tools.

Domains: `node` (always granted), `wallet`, `staking`, `governance`, `treasury`,
`lightning`, `privacy`, `explorer`, `timestamp`, `tor`, `dex`, `bisonrelay`, and
`audit` (the cross-agent spend feed).

### Write scopes (authorization)

Reading is covered by the domain alone. A tool that changes state or moves funds
follows a two-key model: it needs **both** its read domain (to be visible) **and** a
matching write scope in the agent's grant.

Write scopes: `governance`, `lightning`, `dex`, `dex.spend`, `bisonrelay`,
`bisonrelay.admin`, `timestamp`, `tor`, `staking`, `privacy`, `wallet.broadcast`.
Some carry extra risk or move funds and are separate tiers you enable deliberately:

- `dex.spend` - withdrawals and bond posting on the DEX (moves funds).
- `bisonrelay.admin` - destructive group-chat and room administration.
- `wallet.broadcast` - publish an already-signed transaction. It needs no spend grant
  or passphrase: a human signs the transaction on a hardware wallet first, so the
  signature is the authorization and the agent only relays the bytes.

## Spend grants

Moving DCR (on-chain sends, ticket purchases, Lightning payments, DEX spends, Bison
Relay tips) requires a spend grant in addition to the domain and scope. Grant it per
agent in **Settings -> AI Agents** (Spend & action grant):

- **Accounts** - which wallet accounts the agent may spend from.
- **Per-transaction cap** and **daily cap** (in DCR) - literal hard limits. `0` means
  zero: there is no "unlimited". Fund-moving access requires positive caps.
- **Allowlist** (optional) - restrict on-chain sends to specific addresses.
- **Expiry** (optional) - auto-revoke after N hours.

You enter the wallet passphrase when granting. It is verified once, then held in
memory and used on the agent's behalf. It is never persisted, never sent to the
agent, and is wiped on dashboard restart, so grants must be re-created after a
restart.

Fund-moving tools include `wallet_send`, `staking_purchase`, `ln_pay`,
`ln_open_channel`, the `dex.spend` send/post-bond tools, and `br_tip_user`.

## Safety controls

- **Tripwire.** A spend attempt that exceeds a cap revokes that agent's grant and
  blocks its token. The blast radius is the one offending agent; others are
  unaffected. Wrong-account or non-allowlisted denials do not trip it.
- **Freeze all agents.** A confirm-guarded button (Settings -> AI Agents) revokes
  every grant and blocks every token at once. Restore agents individually afterwards.
- **Audit trail.** Every spend attempt (allowed, denied, error, blocked) is recorded
  and shown in the dashboard, and is persisted append-only to
  `/dashboard-data/mcp_audit.jsonl`. Export the full trail from the UI.
- **Unblock + re-grant.** A blocked agent is restored by Unblocking it and granting a
  fresh spend capability.

### Bison Relay approvals (optional)

When enabled (Settings -> AI Agents -> Bison Relay oversight: toggle on and pick a
contact), every fund move must be approved by you over a Bison Relay direct message
before it executes. The dashboard DMs the selected contact:

```
dcrpulse approval [a1b2]: agent "trader" wants to send 0.50000000 DCR to Dsxxx.
Reply "yes a1b2" to approve, "no a1b2" to deny, or "no a1b2 freeze" to deny and
block this agent. Expires in 2 min.
```

You reply over Bison Relay:

- `yes <id>` - approve this one spend.
- `no <id>` - deny it (no funds move; the agent is not blocked).
- `no <id> freeze` (or `block`) - deny it and immediately block the agent (revoke its
  grant and block its token, the same effect as the tripwire).

The id is required, so a stale or late reply cannot resolve a different request.
Caps and the tripwire still apply on top of approvals. Successful spends and blocks
are also reported to the contact. If you do not reply within the window, the spend is
refused.

## Connecting an agent

- **Endpoint:** `http://127.0.0.1:8090/` (streamable HTTP; the whole listener is the
  root path).
- **Auth:** send `Authorization: Bearer mcp_<token>` on every request.
- **Protocol:** both MCP generations are served per request - 2026-07-28
  (`server/discover`, per-request `_meta`) and the legacy revisions through
  2025-11-25 (`initialize`). The endpoint is stateless: no `Mcp-Session-Id` is
  issued, `GET`/`DELETE` answer 405, and request bodies are capped at 16 MiB.

Point any MCP-capable client, or a custom agent built on an MCP SDK, at that endpoint
with the bearer token. The server advertises tools and resources scoped to the
agent's granted domains.

### Validate with the bundled harness

`dashboard/cmd/mcptest` is a Go client that exercises the server end to end the way an
agent would. Run it from the `dashboard` directory:

```bash
# full read sweep + assert every write tool is gated without a grant
go run ./cmd/mcptest -token mcp_xxx

# resources: list, read, subscribe, and confirm a pushed update
go run ./cmd/mcptest -token mcp_xxx -resources

# interactive, user-confirmed live spends (wallet domain only here)
go run ./cmd/mcptest -token mcp_xxx -only=wallet -live
```

Flags: `-endpoint` (default `http://127.0.0.1:8090/`), `-token` (or `MCP_TEST_TOKEN`),
`-only` (a domain or tool name), `-live`, `-resources`, `-call-timeout` (seconds; raise
it for the approval flow, which blocks until you reply), `-invoice`, `-no-color`.

### Low-level: connecting with curl

MCP streamable HTTP is JSON-RPC 2.0 and the endpoint is stateless, so every POST
stands alone - there is no session id to capture:

```bash
TOKEN=mcp_xxx
H='-H Authorization:Bearer '"$TOKEN"' -H Content-Type:application/json -H Accept:application/json,text/event-stream'

# call a tool (the JSON result arrives on the SSE "data:" line)
curl -sS $H http://127.0.0.1:8090/ \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"node_status","arguments":{}}}' \
  | sed -n 's/^data: //p' | jq .

# a legacy initialize still works and shows the negotiated version
curl -sS $H http://127.0.0.1:8090/ \
  -d '{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}' \
  | sed -n 's/^data: //p' | jq .result.protocolVersion

# 2026-07-28 callers probe with server/discover and mirror the two standard
# headers on every request
curl -sS $H -H Mcp-Protocol-Version:2026-07-28 -H Mcp-Method:server/discover \
  http://127.0.0.1:8090/ \
  -d '{"jsonrpc":"2.0","id":3,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}' \
  | sed -n 's/^data: //p' | jq .result.supportedVersions
```

## Capabilities and resources

### Tools

The server exposes roughly 200 tools across the domains above (node, wallet, staking,
governance, treasury, lightning, privacy, explorer, timestamp, tor, dex, bisonrelay).
Rather than list them all here, discover the live set from the client:

- `tools/list` returns the tools visible to your agent (filtered by granted domains).
- The `capabilities` tool (always available) reports the agent's domains, its spend
  grant, and the resources it can see.

Tools are annotated so a client can tell reads from writes: read-only tools carry the
read-only hint; state-changing and fund-moving tools carry the destructive hint.

Read tools return lean payloads by default and accept optional parameters to narrow or
shape the result:

- `governance_proposals` (and `governance_refresh_proposals`) accept an optional
  `status` bucket (`pre-vote`, `voting`, `finished`, `abandoned`); it defaults to
  `voting`.
- `governance_proposal_detail` (and `governance_refresh_proposal_detail`) omit the
  rendered HTML copies of the markdown body and comments by default; pass
  `includeHtml: true` to get them.
- `dex_trades` accepts an optional `limit` to cap how many recent matches are returned.
- `dex_market_summary` returns a market's last trade rate (conventional and USD where
  the quote can be priced) plus 24h stats in a single call; with no `host` it covers
  every market.

Some tools compose into multi-step workflows worth calling out:

- **Air-gapped spending.** `wallet_construct_transaction` builds an unsigned
  transaction (no keys; works on a watch-only wallet); a human signs it on a hardware
  wallet; `wallet_decode_signed_transaction` previews the signed result; and
  `wallet_broadcast_signed_transaction` (scope `wallet.broadcast`) publishes it.
- **Trickle voting.** `governance_vote_trickle_start` signs a proposal's ballots up
  front then submits them spread over time; follow it with
  `governance_vote_trickle_status` / `governance_vote_trickle_events` and stop it with
  `governance_vote_trickle_stop`.
- **Bison Relay housekeeping.** Manage downloads (`br_download_delete`), the
  notification bell (`br_notification_delete`, `br_notifications_clear`), and
  storefront media (`br_store_files`, `br_store_file_get`, `br_store_file_delete`).
- **Contact resolution.** `br_resolve_nick` maps a nick, alias, or name to its
  64-hex uid and returns ALL matches (nicks are not unique on Bison Relay);
  `br_resolve_uid` maps a uid back to its nick, alias, and name. `br_contacts`
  lists the address book without avatar bytes or key material (each entry
  carries `hasAvatar`), and `br_contact_avatar` fetches a single contact's
  avatar as base64 with a sniffed content type.

### Resources (live state)

The server also exposes subscribable MCP resources, so an agent can react to events
instead of polling. Each is gated by the same domain as the matching tools.

| URI | Domain | Contents |
| --- | --- | --- |
| `dcrpulse://node/sync` | node | Node sync state and chain tip |
| `dcrpulse://wallet/sync` | wallet | Wallet sync progress |
| `dcrpulse://wallet/balance` | wallet | Per-account balances |
| `dcrpulse://bisonrelay/messages` | bisonrelay | Recent incoming PMs and group-chat messages |
| `dcrpulse://lightning/events` | lightning | Recent channel and invoice events |
| `dcrpulse://staking/activity` | staking | Recent ticket-purchase and autobuyer events |
| `dcrpulse://privacy/mixer` | privacy | Recent mixer events |
| `dcrpulse://mcp/audit` | audit | Recent agent spend attempts (all agents) |

How updates are delivered depends on the client's MCP revision. Clients on the
2026-07-28 wire (e.g. go-sdk >= v1.7.0 `ClientSession.Subscribe`) subscribe via a
long-lived `subscriptions/listen` call and receive `resources/updated`
notifications (the URI only) on that stream. The legacy `resources/subscribe` is
still accepted and gated the same way, but the stateless endpoint has no legacy
standalone SSE stream (`GET` answers 405), so receiving pushes requires a
2026-07-28 client. Read the new value with `resources/read` either way.

## Examples

Read the node status (needs only the default `node` domain):

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"node_status","arguments":{}}}
```

Send 0.01 DCR (needs the `wallet` domain and a spend grant covering the account and
amount; if Bison Relay oversight is on, you approve it over a DM first):

```json
{"jsonrpc":"2.0","id":3,"method":"tools/call",
 "params":{"name":"wallet_send","arguments":{"account":0,"address":"Dsxxx","amountDcr":0.01}}}
```

Subscribe to the agent audit feed (needs the `audit` domain; legacy-wire frame -
on the 2026-07-28 wire subscriptions ride `subscriptions/listen`):

```json
{"jsonrpc":"2.0","id":4,"method":"resources/subscribe","params":{"uri":"dcrpulse://mcp/audit"}}
```

## Management API

The dashboard manages agents through authenticated endpoints under
`/api/settings/mcp` (these back the Settings UI; use the UI unless you are
automating):

- `GET /api/settings/mcp` - server state, domains, agents, sessions, grants, audit,
  and oversight config.
- `POST /api/settings/mcp/enable` - start/stop the listener.
- `POST /api/settings/mcp/tokens`, `DELETE /api/settings/mcp/tokens/{id}` - create
  (returns the plaintext once) / revoke an agent.
- `POST /api/settings/mcp/agents/{id}/domains` - set granted domains.
- `POST` / `DELETE /api/settings/mcp/agents/{id}/grant` - set / clear a spend grant.
- `POST /api/settings/mcp/agents/{id}/unblock` - clear a tripwire/freeze block.
- `POST /api/settings/mcp/freeze-all` - revoke all grants and block all tokens.
- `POST /api/settings/mcp/notify` - configure Bison Relay oversight (on/off + contact).
- `GET /api/settings/mcp/audit/export` - download the full audit trail.

## Troubleshooting

- **HTTP 401** - missing or wrong bearer token. Check the `Authorization: Bearer
  mcp_...` header.
- **HTTP 403, "agent blocked"** - the agent tripped a cap or was frozen. Unblock it in
  Settings -> AI Agents and grant a fresh spend capability.
- **A tool returns "no spend grant"** - the action moves funds but the agent has no
  spend grant. Grant one (account + caps) in the dashboard.
- **A tool is not in `tools/list`** - its domain is not granted to this agent. Grant
  the domain (and, for writes, the matching write scope).
- **A spend hangs, then is refused with "approval timed out"** - Bison Relay oversight
  is on. Reply to the approval DM with `yes <id>` within the window.
