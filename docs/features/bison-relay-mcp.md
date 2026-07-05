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
4. Optionally set the approval and tip-wait timeouts.

The listener is loopback by default. To reach it from another device set
`MCP_BRIDGE_HOST=0.0.0.0` (and, if needed, `MCP_BRIDGE_PORT`) in your .env -
but it speaks plain HTTP behind the bearer token, so prefer an SSH tunnel or
a TLS reverse proxy. Nothing binds or answers until you enable it here.

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
