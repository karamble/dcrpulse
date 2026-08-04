# Bison Relay MCP (agent access to tool bots)

Bison Relay MCP lets an AI agent call MCP tool services offered by Bison
Relay bots, and pay for them per call with Lightning tips - all over Bison
Relay's encrypted, pay-per-use messaging, with no API keys and no public
endpoint on the bot side.

Your daemon runs the client half (the "bridge"): for each bot you allow, it
exposes a local streamable-HTTP MCP endpoint that an off-the-shelf MCP agent
(Claude Code, etc.) connects to. The bridge mirrors that bot's tools, relays
every call over Bison Relay, and settles paid calls by tipping the bot over
your Lightning node - under caps and an approval mode you control.

## Requirements

- Bison Relay connected (brclientd running and online).
- Lightning (dcrlnd) unlocked and funded with outbound liquidity - payments
  are Bison Relay tips over the Lightning Network.
- A key exchange (KX) completed with each bot you want to call; you allow it
  by its 64-hex uid.

## Enabling it

Settings -> Bison Relay -> AI Agent Access:

1. Toggle it on. A bearer token is minted (recycle it any time).
2. Add the uid of each bot you allow (default deny - no bot is callable
   until listed).
3. Set the spending policy:
   - Per-call cap and daily cap, in DCR. Both bind in every mode; zero means
     never pay.
   - Mode: approval (every payment waits for your yes/no) or autopay
     (payments under the caps settle unattended).
4. Optionally restrict where the token may be used from under **Allowed IP
   addresses** (single IPs or CIDR ranges; empty means any address).
5. Optionally set the approval and tip-wait timeouts.

The listener is loopback by default. To reach it from another device set
`MCP_BRIDGE_HOST=0.0.0.0` (and, if needed, `MCP_BRIDGE_PORT`) in your .env -
but it speaks plain HTTP behind the bearer token, so prefer an SSH tunnel or
a TLS reverse proxy, and pin the token to the caller's address with the
allowed-IP list. Nothing binds or answers until you enable it here.

### Allowed IP addresses (optional)

When the list is set, every request is checked against the connection's
source address after the token verifies; a request from anywhere else is
answered with a generic 401 indistinguishable from a bad token (deliberate:
it does not confirm token validity) and logged by brclientd. Forwarding
headers such as `X-Forwarded-For` are deliberately ignored - behind a
reverse proxy allowlist the proxy and restrict real clients there. Under
Docker an agent on the host usually appears as the bridge gateway (for
example `172.17.0.1`), not `127.0.0.1`: when a restricted agent is denied,
the section shows the most recent attempt ("An agent using this token was
denied from ...") with an **Allow this IP** button - the one-click fix when
you don't know which address the listener sees. The notice is in-memory
only and clears on the agent's next successful request.

## Connecting an agent

Point the agent at the per-bot endpoint with the bearer token:

    claude mcp add --transport http mybot http://127.0.0.1:8891/mcp/<BOT_UID> \
        --header "Authorization: Bearer <TOKEN>"

Then run `/mcp` in the agent to connect. The agent sees the bot's tools as
an ordinary MCP server; it needs no Bison Relay awareness, wallet, or keys.

## Paying for tools

When a tool costs money, the bot replies that payment is required. The
bridge checks your caps and mode, tips the exact amount to the bot over
Lightning, and retries the call. In approval mode the payment parks in this
page until you approve or deny it; the spend log below records every settled
payment and the rolling daily total.

Envelope frames the agent and bot exchange are hidden from your chat history
automatically, and Bison Relay content filters that would match them are
refused (filtering them would break the sessions).
