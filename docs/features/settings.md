# Settings

The **Settings** area gathers the dashboard's global, app-wide controls into a
single tabbed page. It covers wallet maintenance, privacy and external-request
preferences, Tor connectivity, log inspection, theming, the optional app
password that protects the whole dashboard, alert categories, AI agent access,
and version information.

## Overview

Settings is organized as a horizontal tab bar with nine tabs. Each tab is its
own route under `/wallet/settings`, so the active tab is reflected in the URL and
can be bookmarked or linked directly. The tab bar scrolls horizontally on narrow
screens.

**Access**: Click the **"Settings"** entry in the wallet navigation, then choose
a tab. Settings sits inside the wallet section, so it is reachable once a wallet
is open.

**Tabs:**

- **Wallet** - passphrase rotation, address discovery, close wallet
- **Privacy & Security** - mixer debug logging, external-request preferences, invite-bot URL
- **Tor** - route daemons through Tor, monitor the network, host an onion service
- **Logs** - read-only tail of each daemon's log file
- **Themes** - light/dark/custom themes with a live editor and import/export
- **Security** - the optional app password that gates the whole dashboard
- **Alerts** - which categories the alert center raises (node, wallet, staking,
  Lightning, DEX, Bison Relay, system)
- **AI Agents** - the MCP server and per-agent access, covered in
  [AI Agents (MCP)](ai-agents-mcp.md)
- **About** - dcrpulse and daemon version information, plus reference links

---

## Wallet Tab

Maintenance actions for the active wallet.

### Private Passphrase

Rotate the wallet's private (signing) passphrase. This passphrase is required
for ticket purchases, sending transactions, and unlock operations, so the new
value applies to all future signing.

Clicking **Change** opens a modal that asks for:

- **Current passphrase**
- **New passphrase** (must be at least 8 characters)
- **Confirm new passphrase**
- **DCRDEX app password** - shown only while the DEX is locked. The DEX stores the
  wallet passphrase too, so it needs the app password chosen at DEX setup (not the
  wallet passphrase) to receive the new one; it stays locked afterwards.

The modal validates locally that the new passphrase is long enough and that the
two new entries match before the **Change passphrase** button enables; with a
locked DEX it also waits for the DCRDEX app password field above. Errors returned
by the wallet are shown in the modal.

When Lightning is set up, the modal continues after the change instead of closing:
dcrlnd is restarted to re-key its macaroons, then unlocked with the new passphrase.
If the automatic unlock does not take, the modal offers a **Wallet passphrase**
field to unlock it by hand, and lets you close it and unlock later.

### Address Discovery

Scan the chain for previously-used addresses under a chosen gap limit. Run this
after importing an xpub or restoring from a seed that previously used a high
gap. A successful discovery is followed by a rescan so the found addresses' history
appears; the gap used by the last discovery is shown inline.

Clicking **Discover** opens a modal that asks for:

- **Gap limit** - a number from 20 to 10000 (default 20; increase if the
  restored wallet previously used a higher gap)
- **Wallet passphrase**

The wallet is briefly unlocked for the scan and re-locked afterward. The scan
can take several minutes; the modal warns not to close the tab while it runs.

### Close Wallet

Close the active wallet and return to the wallet list. Other wallets stay
available to open. After closing, the dashboard reloads to the wallet list view.

---

## Privacy & Security Tab

Privacy-related toggles and preferences for outbound (external) requests.

### Mixer Debug Logging

A single toggle that turns MIXC and TKBY debug logging on or off in the
dcrwallet container. This is useful for diagnosing mixer issues but is quite
chatty in the logs. The setting is applied to the running dcrwallet immediately.

### External Requests

Three toggles control whether the dashboard reaches out to external services for
optional, convenience-only data. Each is on by default:

- **VSP registry** - fetch the VSP list from api.decred.org for the Staking
  page's VSP picker.
- **Politeia** - fetch off-chain proposals from proposals.decred.org for the
  Governance > Proposals tab.
- **Bison Relay LN seeder** - fetch Lightning peer suggestions from
  bisonrelay.org for the Channels tab's open-channel form. When disabled, the
  form shows no presets and you must type a peer URI manually.

These preferences are persisted to the global config.

### Decred Pulse Bot URL

The endpoint used by the "Join Decred chat networks" invite bot. Leave it at the
default unless you run your own brulse instance. The field accepts a URL that
must start with `http://` or `https://`; clearing it resets the value to the
default. Saving confirms the bot is reachable.

---

## Tor Tab

Route the node, wallet, Lightning, and DEX connections through a bundled Tor
proxy to hide your IP address. Changes apply within a few seconds without
restarting the app. The tab refreshes its status every five seconds. It is
presented as three cards: settings, network health, and per-daemon routing.

### Settings

- **Route connections through Tor** - the master switch. Send outbound traffic
  for every daemon through the Tor network.
- **Stream isolation** - use a separate Tor circuit per connection
  (recommended). Available only when Tor is enabled.
- **Host an inbound onion service (dcrd)** - advertise a `.onion` address so
  other nodes can reach yours anonymously. Available only when Tor is enabled.
- **Circuit limit** - the maximum number of concurrent Tor circuits when stream
  isolation is on (1 to 1000).

Changing any setting here relaunches every daemon. The wallet, Lightning, and
the DEX will lock and need their passphrases entered again, and any running DEX
bots stop.

### Tor Network

A health panel for the bundled Tor process, with a **New identity** button that
builds fresh Tor circuits (enabled only while the control port is reachable).
When reachable, it shows:

- **Bootstrap** - bootstrap percentage and current phase tag
- **Circuits** - number of open circuits
- **Proxy** - whether the Tor SOCKS proxy is reachable
- **Version** - the running Tor version
- **Sent** / **Received** - bytes written and read through Tor

If the control port is not reachable, the panel shows that state (with the error
text when available) instead of the stats.

### dcrd Onion Address

When an onion address is available it is displayed here, labeled **(active)**
when the onion toggle is on or **(available)** when it exists but is not yet
advertised.

### Daemon Routing

A per-daemon list showing how each daemon is currently reaching the network.
Each row shows a status badge:

- **Not running** - the daemon is not up
- **Applying...** - the daemon is still being relaunched to pick up the latest
  Tor setting
- **Via Tor** - the daemon's traffic is routed through Tor
- **Clearnet** - the daemon is connecting directly

Daemons covered include the node (dcrd), wallet (dcrwallet), Lightning (dcrlnd),
DEX (bisonw), Bison Relay (brclientd), and the dashboard itself.

---

## Logs Tab

A read-only viewer that tails one log file at a time. It does not interpret the
logs; it tails the file written by the selected container under
`/app-data/<component>/logs/`. The exceptions are `tor`, which writes straight
to `/app-data/tor/tor.log`, and `dcrpulse`, the dashboard's own log at
`/dashboard-data/logs/dcrpulse.log` (10 MB rotation, 3 compressed rolls). It carries the MCP agent-activity log when that is enabled under
Settings -> AI Agents.

### Controls

- **Component** - choose which log to read: dcrwallet, dcrd, dcrlnd, brclientd,
  dcrdex, tor, or dcrpulse.
- **Line count** - tail the last 200, 500, 1000, or 2000 lines (default 500).
- **Refresh** - re-fetch the current selection.

The viewer reloads automatically whenever the component or line count changes
and auto-scrolls to the newest line. Lines are color-coded by level: errors are
highlighted, warnings are dimmer, and debug lines are dimmest.

---

## Themes Tab

CSS-variable theming for the whole dashboard. Pick a built-in theme to apply it
everywhere, or build your own. Custom themes are saved on this instance (server
side) and follow you across browsers.

### Theme List

Each theme is shown as a card with its name, a **Built-in** or **Custom** badge,
its base appearance (dark or light), and a row of color swatches. Card actions:

- **Apply** - make the theme active everywhere (the active theme is marked
  **Active**).
- **Customize** / **Edit** - open the editor. Built-in themes open as a new copy
  to customize; custom themes are edited in place.
- **Duplicate** - open the editor on a copy of the theme.
- **Export** - download the theme as a JSON file.
- **Delete** - remove a custom theme (with a confirmation prompt). Built-in
  themes cannot be deleted.

### Import

The **Import** button reveals an area to add a theme either by pasting theme
JSON or by choosing a JSON file. Imported themes are saved as custom themes.

### Theme Editor

Selecting **New theme**, **Customize**, **Edit**, or **Duplicate** opens the
editor, which previews changes live across the whole app while you work and
restores the real active theme if you discard. The editor offers:

- **Theme name** and **Base appearance** (dark or light).
- Color fields grouped into **Brand & accent**, **Backgrounds & surfaces**,
  **Text**, **Status**, and **Borders**. Text/background pairs that fall below
  the WCAG AA contrast ratio are flagged.
- **Typography** controls under the Text group: heading color, heading weight,
  and H1/H2/H3 sizes (weight and size apply to rendered content headings such as
  proposals and pages).
- An **Advanced** section to customize gradients (otherwise derived from the
  accent and card colors) and a reserved corner-radius value.

**Discard** closes the editor without saving; **Save theme** persists the theme
and applies it.

---

## Security Tab

An optional **app password** that gates the entire dashboard. It is off by
default. When enabled, a login is required to use the dashboard, and the gate
covers the whole API and all live connections, not just individual pages. The
card at the top of the tab shows whether the password is currently enabled.

### Enable App Password

While disabled, the tab shows a single form to enable protection. Enter a new
password and confirm it; the **Enable** button stays disabled until both fields
match. On success the gate turns on immediately.

### Change Password

While enabled, a **Change password** form takes the current password and a new
one. Changing the password keeps your existing session alive.

### Disable App Password

While enabled, a **Disable app password** form takes the current password and
turns the login requirement off. The form warns that, once disabled, anyone who
can reach the dashboard will be able to use it.

### How the Session Works

- Enabling or changing the password takes effect in the dashboard immediately
  (no page reload), including the header's logout button.
- After a successful login or setup, the server issues a signed, HttpOnly
  session cookie. The session lasts 30 days.
- While the app password is enabled, every API route requires a valid session
  except the login handshake itself. Only the password-status check and the
  login endpoint are reachable without a session.
- **Disabling** the password invalidates all existing sessions. **Changing** the
  password leaves your current session valid.
- A stray enabled flag cannot lock you out: the gate counts as enabled only when
  a stored password hash and a session secret both exist. A half-written or
  unreadable auth config is the other case and fails closed - the dashboard locks
  and answers `503 dashboard locked: the app-password config could not be loaded;
  repair it and restart the dashboard` on every `/api` route except the
  password-status check, which stays reachable so the setup screen can still say
  what happened. The lock is read at startup, so it clears only once the config is
  repaired and the dashboard restarted.

---

## Alerts Tab

Which categories the alert center is allowed to raise. Every category is on by
default and each one is an opt-out toggle:

| Category | Raises on |
|---|---|
| Node | dcrd unreachable, zero peers, stalled chain sync |
| Wallet | Incoming payments |
| Staking | Ticket purchases, votes, misses, expiries, revocations |
| Lightning | Force-closed channels, peer connectivity |
| DEX | Completed trades, bisonw connectivity |
| Bison Relay | brclientd connectivity |
| System | Disk space on the data volumes |

Turning a category off stops it at the source rather than hiding it: the engine
drops those alerts at raise time, and any of that category's active conditions
resolve as it goes off. Entries already recorded stay in the alert center until
you read them.

Alerts never leave the box. They are not delivered over Bison Relay, email, or
any other transport.

---

## AI Agents Tab

The MCP server toggle, the Bison Relay bridge, and per-agent tokens and scopes.
Both listeners ship off. See [AI Agents (MCP)](ai-agents-mcp.md) for the whole
surface.

---

## About Tab

Version information and reference links. There is no configuration here.

### Application

Version cells for the dashboard and the daemons it talks to, read live from each
service:

- **dcrpulse** - the dashboard version
- **dcrd** - the node version
- **dcrwallet** - the wallet version
- **dcrlnd** - the Lightning daemon version
- **brclientd** - the Bison Relay daemon version
- **bisonw** - the DEX daemon version (shown only when the DEX is running)

Cells whose source is unavailable show a dash.

### Sources and Communications

Two groups of outbound links to Decred resources: source repositories and sites
(dcrpulse and Decred on GitHub, decred.org, the documentation, the VSP list, and
the block explorer), and community channels (Matrix chat and Telegram). All open
in a new tab.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - account balances, transactions, and staking
- **[Wallet Setup](../guides/wallet-operations.md)** - initial wallet configuration
- **[Staking Guide](staking-guide.md)** - tickets, VSPs, and governance
- **[AI Agents (MCP)](ai-agents-mcp.md)** - agent tokens, capability grants, spend limits, and per-agent allowed IPs

---

## Tips & Best Practices

### Wallet Maintenance

1. Rotate the private passphrase to a strong value and keep a record of it; it is
   required for every signing operation.
2. Run address discovery after importing an xpub or restoring a wallet with high
   address activity, and pick a gap limit at least as large as the wallet used
   before.
3. Use **Close wallet** to return to the wallet list without affecting other
   wallets.

### Privacy

1. Disable any external request you do not want the dashboard making; the
   corresponding feature simply falls back to manual entry or no data.
2. Leave the bot URL at the default unless you run your own brulse instance.

### Tor

1. Toggle Tor on or off knowing that every daemon relaunches, so the wallet,
   Lightning, and DEX will need their passphrases re-entered.
2. Watch the **Daemon routing** card to confirm each daemon has finished
   switching (no **Applying...** badges) before relying on the new path.
3. Use **New identity** to rotate circuits without changing any setting.

### Security

1. Enable the app password if anyone else can reach the dashboard's network.
2. Remember that disabling the password makes the dashboard open to anyone who
   can reach it, and that doing so logs out all sessions.

---

## Troubleshooting

### Logs Not Loading

**Problem**: The Logs tab shows an error or no lines.

**Solutions:**

1. Confirm the selected daemon is running and writing to
   `/app-data/<component>/logs/`.
2. Try a different component or a smaller line count.
3. Click **Refresh** to re-fetch.

Set `DCRPULSE_LOG_LEVEL` to raise the dashboard's own verbosity, either as a
bare level (`debug`) or with per-subsystem overrides (`info,MSIG=debug`), then
restart the dashboard.

### Tor Stuck on "Applying..."

**Problem**: A daemon's routing badge stays on **Applying...**.

**Solutions:**

1. Give the daemons a few seconds; the page refreshes status every five seconds.
2. Check the daemon's log in the Logs tab for restart errors.
3. Re-enter the wallet, Lightning, or DEX passphrase, since changing a Tor
   setting relaunches and re-locks them.

### New Identity Disabled

**Problem**: The **New identity** button is greyed out.

**Solutions:**

1. Confirm Tor is enabled and the control port is reachable (the Tor network
   panel will show its state).
2. Wait for Tor to finish bootstrapping.

### Locked Out After Enabling the App Password

**Problem**: You forgot the app password.

**Solutions:**

1. A forgotten password cannot be recovered from the dashboard. The password hash
   is stored in the global config; clearing the auth fields there disables the
   gate. Clear them together - a hash left behind without its session secret
   locks the whole API with a 503 instead.
2. Choose a password you can recover and keep a record of it.

---

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or the
[Troubleshooting Guide](../guides/troubleshooting.md).
