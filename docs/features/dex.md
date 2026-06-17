# DEX (DCRDEX)

The **DEX** page brings the Decred decentralized exchange (DCRDEX) into the dashboard as a full-width trading terminal. You trade peer-to-peer through atomic swaps, with no exchange custody of your funds: orders match on a DEX server, but every trade settles directly between the two wallets on-chain.

## Overview

DCRDEX is a non-custodial exchange protocol. Instead of depositing coins with an exchange, you register with a DEX server by locking a small, refundable **fidelity bond**, then place orders. When orders match, the two parties perform an atomic cross-chain swap; the server never touches the coins.

In dcrpulse the DCRDEX client runs as a backend daemon (**bisonw**) with no web UI of its own. The dashboard talks to it over its local RPC and websocket interfaces and renders a native trading interface at `/dex`. Markets, order book, charts, your orders, your asset wallets, your account/bonds, and an optional market-maker bot all live behind one in-page sub-navigation, so switching tabs never tears down the live trading grid.

**Access**: Click the **"DEX"** button in the header navigation.

**Backend**: The dashboard exposes the DCRDEX feature under the `/api/dcrdex/...` route group. Those handlers connect to the bisonw daemon over a TLS-pinned RPC connection (default port 5757) plus its websocket/webserver interface (default port 5758) for the live feed and the market-maker controls. The bisonw **app password** is held in dashboard memory only for the unlocked session and is never stored, so you re-enter it after a restart.

**Canonical server**: The dashboard targets the mainnet DEX server **`dex.decred.org:7232`**.

---

## DEX States and Onboarding

When you open the DEX page it reads the backend status and shows the right stage. The status is polled every 10 seconds, so the page advances and recovers on its own.

**Stages:**

- **DCRDEX is starting** - The bisonw backend is not reachable yet. The page shows a notice and recovers automatically once the daemon is ready.
- **Set up DCRDEX (needs-init)** - The client has never been initialized. You set an app password (and optionally restore from a seed).
- **Connect your Decred wallet (needs-wallet)** - The client is initialized but has no Decred wallet configured yet.
- **Unlock DCRDEX (needs-unlock)** - The client is initialized but locked for this session. You enter the app password.
- **Ready** - Unlocked. Shows registration (if not yet registered with a server) or the trading terminal.

**Wallet-sync gate**: First-time setup that touches your wallet (initializing the client, creating the dex account) is gated on a synced, responsive wallet. Until the wallet is ready, the page shows a sync gate with progress. Simply unlocking an already-initialized client is not gated.

---

### Step 1: Set up DCRDEX (app password)

The first time you open the DEX page, you set an **app password** for the bisonw backend.

- **App password** encrypts your DEX account data and is required to trade.
- It is held only for the current session and is never stored. After a dashboard restart, you unlock again with the same password.
- **Confirm password** must match.
- **Restore from a backup seed** (optional checkbox): paste your 15-word DCRDEX recovery seed to restore an existing DEX identity instead of creating a fresh one.

If you create a new identity, a fresh DCRDEX seed is generated and you are prompted to back it up (see **Seed Backup** below).

---

### Step 2: Connect your Decred wallet

DCRDEX trades Decred from a dedicated **`dex`** account inside the dashboard's own dcrwallet.

- Enter your **wallet passphrase**. The backend creates the `dex` account (if it does not exist yet) and connects it.
- The passphrase is used only for this request and is not stored.

The DEX runs against the dashboard's existing wallet and its `dex` account; DEX-side wallet recovery/reconfiguration is out of scope here - wallet administration lives in the dashboard's Wallet pages.

---

### Step 3: Back up your DCRDEX seed

After creating a new identity, a backup reminder appears over the unlocked view. Backing up is also available any time from **DEX > Settings**.

The guided backup flow:

1. **Reveal the seed** - Re-enter your app password to display the 15 words.
2. **Write it down** - Copy or transcribe the seed and store it offline.
3. **Verify** - The seed is hidden and you re-type a few random words from it to prove you recorded it (mirrors the Decred wallet setup wizard).
4. **Confirm backup** - The dashboard records that the backup is done and stops reminding you.

**Why it matters:** These 15 words are the only way to recover your DCRDEX account, your fidelity-bond reclaim keys, and every native coin wallet you create here (for example BTC and LTC). Your DCR itself is held by your Decred wallet and is recovered with your Decred wallet seed - not these 15 words. Keep both seeds safe.

You can choose **Remind me later** to dismiss the reminder for the session.

---

### Step 4: Register with a DEX server (post a fidelity bond)

Once unlocked and connected, the page shows the registration screen for `dex.decred.org:7232` until your account is registered. After a seed restore the client first runs account discovery against the server; if it finds a live bond, it skips straight to trading.

The registration screen shows the server's markets and bond requirements, then walks you through two steps:

**1. Fund your DEX account**

- A **deposit address** for the dex account is shown (with a copy button).
- Send at least the required bond amount in DCR (plus a little for network fees) to that address.
- Your **current balance** updates automatically as the deposit confirms; a sync indicator is shown while the wallet is still syncing.

**2. Post your bond**

- **Tiers** - Choose how many tiers to bond. Each tier locks a fixed amount of DCR for a set expiry period as a refundable bond, and raises your trading limit and reputation on the server.
- The screen shows the **total bond**, the expiry in days, and the required confirmations.
- Posting requires the dex account to be funded above the bond amount; the button stays disabled until it is.
- **Post bond and register** asks for an explicit confirmation, then spends the bond.

**About bonds:** A fidelity bond is what lets you trade without an account fee. It is time-locked DCR that deters spam and fake orders by holding you accountable for trades you start. The bond is refundable to your wallet after it expires, and auto-renews while maintained. If you back out of a trade during settlement you are penalized: your effective trading tier drops and you may need to post additional bond to restore it.

Posting spends real DCR on mainnet, so it is always behind a confirmation step.

---

## The Trading Terminal

Once registered, the **Trade** tab is the full trading terminal. It is laid out as a single grid on large screens and a one-pane-at-a-time switcher (Markets / Chart / Order book / Trade) on smaller screens.

A connection indicator reflects the real DEX server state (connected and authenticated) from the live feed; a banner appears if the server becomes unreachable, and the view keeps showing the last cached market list and recovers when the connection returns.

### Market stats bar

Shows the selected market and its summary statistics (last price, 24h figures) for the chosen market. The default market is **DCR/BTC**, falling back to the first market the server lists.

### Markets sidebar

Lists every market the server offers. Each row shows the pair (with coin icons) and its last/24h figures, streamed live. Click a market to switch the whole terminal to it.

### Price chart

A candlestick chart for the selected market with a duration selector (the candle durations the server supports, for example 1h). Candles update live.

### Order book

A right-hand panel with three tabs:

- **Order book** - Aggregated price levels for asks and sells, with a running depth total and a depth-shaded background bar per level. The spread (mid price, absolute spread, and spread percent) is shown in the middle. A small count badge marks a price made up of multiple orders, and a dot marks a price where you have an order. Levels flash on live changes. Clicking a level prefills the order form with that price, size, and side.
- **Depth** - A depth chart of the book.
- **Trades** - Recent matches (price, size, relative age), newest first.

### Order entry form

The order form sits under the order book. It places limit and market orders, sized in **lots** (whole units the market trades in).

**Side and type:**

- **Buy / Sell** the base asset.
- **Limit** or **Market**.

**Inputs:**

- **Price** (limit only) - The limit price, snapped to the market's rate step. Up/down steppers nudge it by one step.
- **Lots** - The primary quantity field; a whole number of lots. The lot size is shown.
- **Amount** - The same quantity expressed in the base coin, kept in sync with lots and snapped to a whole number of lots.
- **Spend** (market buy only) - A market buy is sized by how much quote asset you spend, not by lots (this mirrors bisonw).
- A **max** shortcut fills in the estimated maximum fundable lots.

**Spend / receive summary:** As you type, the form shows what you will spend and receive. The base side is always exact; the quote side is exact for a limit order and estimated from the best opposing book level (marked with `~`) for a market order.

**Pre-order estimate and checks:** Before you commit, the form requests a pre-order estimate from the backend. It surfaces:

- Estimated **swap fee** and **redeem fee** (worst case), in the relevant assets.
- A server-side validation failure (for example not enough to cover fees or reserves).
- An insufficient-funds warning by comparing the order's requirement against the funding wallet's available balance (a buy locks the quote asset, a sell the base asset).
- A warning if the order exceeds the estimated max lots (bisonw may reject it).

**Advanced options:** If the estimate reports order options (for example pre-paid redemptions), an **Advanced options** section lets you toggle them; they are seeded with the server's defaults.

**Placing the order:** Click **Buy/Sell**, then **Confirm** in the warning panel. Placing spends real funds. Every order settles by atomic swap with self-custody.

### Your open orders

Two panels below the grid show your orders:

- A per-market open-orders panel for the selected market.
- An all-markets open-orders panel.

While any order is still settling, the panels poll quickly so swap-status bars and statuses advance on their own; otherwise they fall back to a slow idle refresh, and live order/match notifications trigger an immediate update. Cancelling a standing order is available where the order is cancellable, behind a confirmation modal.

**Market-maker interaction:** When a market-maker bot is running on the selected market, manual trading on that market is blocked (mirroring bisonw): the order form is replaced by a running-bot card, and clicking book levels is disabled.

---

## Order Lifecycle and Settlement

DCRDEX trades settle by atomic swap. A single swap proceeds in four stages between the two parties (maker and taker):

1. **Maker Swap** - The maker broadcasts its swap contract.
2. **Taker Swap** - The taker broadcasts its swap contract.
3. **Maker Redemption** - The maker redeems, revealing the secret.
4. **Taker Redemption** - The taker redeems.

The dashboard renders this as a four-segment progress bar: green for completed stages, orange for the stage currently confirming, and muted for pending stages. An order's overall stage is driven by its least-progressed match (the slowest match dictates completion).

### Orders tab (full history)

The **Orders** tab is your complete order history for the server - including executed, canceled, and revoked orders - read from the order archive (the trade-view panels show only active/recent orders).

- **Filter** by market and by status (epoch, booked, executed, canceled, revoked), applied server-side.
- **Paginated** with a **Load more** button.
- Canceled orders are hidden under "All statuses" and appear only when you filter to "canceled".
- New orders and fills refresh the first page automatically.
- Click a row to open the in-tab **order detail** view.

### Order detail

The detail view shows:

- A summary: market, side, type, status, quantity, price, submit time, and order ID.
- **Filled** and **Settled** progress bars.
- The per-counterparty **matches**, each as a maker/taker swap negotiation with its four numbered stages.
- For each on-chain stage, the broadcast **coin/transaction id** as a clickable link, with live confirmation progress (for example `2/3 confs`, then `confirmed`). Decred coins link to the dashboard's own block explorer; other assets link to an external block explorer (with a leaving-site confirmation).
- If a swap is being refunded (or the match was revoked), a refund row with the refund coin or a refund-availability countdown.

While an order is still settling, the detail view polls the single-order route so the swap steps and confirmation counts advance even if a notification is missed. A cancellable order can be cancelled from here.

---

## Wallets Tab

The **Wallets** tab is a multi-asset, master-detail view of the coin wallets DCRDEX manages (Decred plus any other supported assets you add). A selector lists your wallets with status and balance; a detail pane shows the selected wallet.

### Wallet list

Each wallet shows a status dot (synced / syncing / off or disabled), its symbol, and its balance (with an approximate USD value when a rate is available). A wallet that is still syncing shows a sync progress bar. An **Add wallet** button starts wallet creation.

### Add a wallet

The add flow supports any asset DCRDEX offers (tokens included):

1. **Pick an asset** from the catalog of assets you do not already have a wallet for.
2. **Pick a wallet type** when the asset offers more than one (for example a built-in/native wallet vs. an external one).
3. **Fill the configuration form**, which is generated from the wallet type's schema, with a description and an optional setup-guide link.
4. **Wallet password** - Built-in (seeded) wallets are encrypted with your DCRDEX app password and need no separate password; external wallet types take their own passphrase.
5. **Create** the wallet.

### Wallet detail

For the selected wallet:

- **Status** and **actions** gated on the wallet's capabilities: **Unlock**/**Lock** (encrypted wallets), **Rescan** (rescannable wallets), and **Enable**/**Disable**.
- **Balance** breakdown: spendable (with approximate USD), plus Locked, Immature, In orders, In bonds, and Total.
- **Deposit address** with a copy button and a QR code. For UTXO chains (DCR, BTC, LTC) you can **Generate new address**, and the panel warns if the current address has already been used (to avoid address reuse). Account-based chains reuse one static address.
- **Send** (withdraw): enter a recipient address and an amount (the **max** shortcut fills the spendable balance). A debounced estimate shows the network fee and flags an invalid address. For account-based chains, gas fees are paid from the parent asset's balance separately from the amount. Sending is behind a two-step confirmation and reports the resulting coin id. Spending real funds cannot be undone.
- **Peers** (for wallets that manage peers): view connected peers and add or remove peer addresses.
- **Transactions**: the wallet's transaction history (for wallets that report it).

---

## Account Tab

The **Account** tab is your per-server account view for `dex.decred.org:7232`: connection state, trading tier, reputation, and bonds, with controls to maintain them.

**Cards:**

- **Trading tier** - Your effective tier, your target tier, and the tier from your bonds. Your tier sets how much you can hold across active orders and settling matches per market (tier x the server's parcel size for that market). The effective tier can be lower than the bonded tier if you have penalties.
- **Reputation** - A reputation meter (penalty zone vs. positive zone) with your current score and the resulting trading-limit bonus, plus penalty count and penalty threshold.
- **Bonds** - Bond cost per tier, expiry in days, pending bonds, and bonds pending refund. A **Pending bonds** card lists pending bonds with their confirmation counts.

**Controls:**

- **Bond options** - Toggle **auto-renew** to keep your target tier as bonds expire, set the **target tier**, an optional **max bonded** cap (blank leaves it unchanged, 0 resets to default), **penalty compensation** (auto-tops-up tiers lost to penalties), and the **bond asset**. Save to apply.
- **Post additional bond** - Bond more tiers from the Account tab. Shows the total cost and posts behind a confirmation; this spends DCR from the dex account, locked until expiry.

---

## Market Maker Tab

The **Market Maker** tab runs automated market-making bots through the bisonw market-maker engine. Bot and exchange (CEX) state are read from a shared, live market-making status feed; the market-maker controls go through the `/api/dcrdex/mm/...` routes.

### Exchanges (CEX credentials)

A section for centralized-exchange API keys, used by arbitrage bot types. Each supported exchange shows whether it is connected, configured, or not set, and (when connected) its balances. **Configure keys** opens the credential form.

### Bots

Lists your configured bots. For each bot:

- The market (with coin icons) and the bot kind: **Basic market maker**, **Arb market maker**, or **Simple arb** (and the CEX it uses, if any).
- **Start** / **Stop**, **Edit**, and **Delete** controls (edit and delete are available when the bot is stopped). Deleting asks for confirmation.
- **Logs** while running.
- A live activity summary while running.

A **Run history** view shows archived bot runs.

### New bot wizard

Creating a bot is a three-step flow (mirroring the bisonw market-maker settings page):

1. **Market** - Pick the market to make.
2. **Bot type** - Choose the strategy (basic market maker, arb market maker, simple arb) and, for arb types, the CEX.
3. **Configure** - Fill the strategy configuration. A market report feeds the placements chart, an oracle table, and lots-to-USD hints.

Editing an existing bot jumps straight to the config step with the market and type locked.

**Starting a bot** opens a funding dialog where you set the allocation (and, for arb types, auto-rebalance transfer thresholds). Allocation is collected at start time, not stored in the saved config. Starting spends funds and is gated behind an explicit confirmation.

---

## Settings Tab

The **Settings** tab holds DEX-view preferences:

- **Notifications** - Toggle desktop (OS) notifications for new DEX activity and choose which categories fire them. The in-app notification bell always shows activity regardless of this setting.
- **Back up app seed** - The same guided seed-backup flow described above, with a "Backed up" / "Not backed up yet" indicator. Available any time.

The notification bell lives in the top sub-nav next to the **Lock** control, and surfaces live DEX activity.

---

## Locking the DEX

A **Lock** control in the trading view's sub-nav locks the bisonw backend for the session.

- Lock only succeeds when bisonw actually locks. bisonw refuses to lock while any order is still active, so the daemon's message is surfaced and the DEX stays unlocked (the dashboard state always matches the daemon).
- To lock cleanly: stop all market-maker bots, cancel all standing orders, and let matched trades finish settling, then lock again.
- After a lock (or a dashboard restart), you return to the **Unlock DCRDEX** screen and re-enter your app password.

---

## Cross-Chain Bridge

DCRDEX bridge transactions (cross-chain bridge notifications) are ingested into the dashboard's live DEX state for a future bridge view. There is no bridge UI yet; the data is consumed by no panel at this time.

---

## API Route Groups

The DEX feature is served under `/api/dcrdex/...`. The handlers proxy to the bisonw daemon. Grouped by area:

**Status and session**
- `GET /api/dcrdex/status` - current onboarding stage and flags
- `POST /api/dcrdex/init` - initialize the client (optionally from a restore seed)
- `POST /api/dcrdex/unlock` - unlock for the session
- `POST /api/dcrdex/lock` - lock
- `POST /api/dcrdex/discover-account` - discover an existing account on a server (used after a restore)

**Wallet setup and seed**
- `POST /api/dcrdex/wallet` - create/connect the Decred `dex` account
- `GET /api/dcrdex/wallet` - the dex account wallet (deposit address, balance, sync)
- `POST /api/dcrdex/seed` - reveal the recovery seed (app-password gated)
- `POST /api/dcrdex/seed/backed-up` - record that the seed is backed up

**Registration and bonds**
- `GET /api/dcrdex/account` - per-server account (tier, reputation, bonds)
- `POST /api/dcrdex/postbond` - post a fidelity bond
- `POST /api/dcrdex/bondopts` - set auto-renew / target tier / bond options

**Markets and config**
- `GET /api/dcrdex/exchanges` - registered exchanges
- `GET /api/dcrdex/dexconfig` - a server's markets and bond config
- `GET /api/dcrdex/assets` - the supported-asset catalog
- `GET /api/dcrdex/rates` - fiat rates for value display

**Orders and trading**
- `GET /api/dcrdex/myorders` - active/recent orders (trade-view feed)
- `POST /api/dcrdex/orders` - filtered, paginated order history
- `POST /api/dcrdex/order` - a single order with live swap confirmations
- `POST /api/dcrdex/trade` - place an order
- `POST /api/dcrdex/preorder` - pre-order fee/option estimate
- `POST /api/dcrdex/maxbuy`, `POST /api/dcrdex/maxsell` - max fundable lots
- `POST /api/dcrdex/cancel` - cancel an order

**Live feeds (websocket)**
- `GET /api/dcrdex/ws` - order book / market live feed
- `GET /api/dcrdex/notify` - notification relay
- `GET /api/dcrdex/notifications` - recent notifications

**Asset wallets**
- `GET /api/dcrdex/wallets` - all managed wallets
- `POST /api/dcrdex/wallet/create` - create a wallet for an asset
- `POST /api/dcrdex/wallet/open`, `POST /api/dcrdex/wallet/close` - unlock/lock
- `POST /api/dcrdex/wallet/toggle` - enable/disable
- `POST /api/dcrdex/wallet/rescan` - rescan
- `POST /api/dcrdex/wallet/send`, `POST /api/dcrdex/wallet/txfee` - send and fee estimate
- `POST /api/dcrdex/wallet/new-address`, `GET /api/dcrdex/wallet/address-used` - deposit addresses
- `GET /api/dcrdex/wallet/txs`, `GET /api/dcrdex/wallet/tx` - transaction history
- `GET|POST|DELETE /api/dcrdex/wallet/peers` - list/add/remove peers

**Market maker**
- `GET /api/dcrdex/mm/status` - live bot and CEX status
- `GET /api/dcrdex/mm/marketreport` - market report for config/funding
- `GET /api/dcrdex/mm/runlogs`, `GET /api/dcrdex/mm/archivedruns` - logs and archived runs
- `POST /api/dcrdex/mm/config`, `POST /api/dcrdex/mm/config/remove` - add/remove a bot config
- `POST /api/dcrdex/mm/cexconfig` - set CEX credentials
- `POST /api/dcrdex/mm/start`, `POST /api/dcrdex/mm/stop` - start/stop a bot

---

## Tips and Best Practices

### Before you trade
1. Finish wallet sync - first-time setup is gated on a synced wallet.
2. Back up your DCRDEX seed immediately after creating an identity.
3. Fund the dex account before trying to register; the bond button stays disabled until it is funded.
4. Remember the app password is session-only; you will re-enter it after a restart.

### Placing orders
1. Size orders in lots; the form snaps quantities and prices to the market's increments.
2. Read the spend/receive summary; a `~` means the quote side is an estimate (market orders).
3. Watch the pre-order fee estimate and any insufficient-funds or max-lots warning before confirming.
4. A market buy is sized by spend (quote asset), not by lots.

### Managing your account
1. Enable auto-renew so bonds keep your target tier as they expire.
2. Higher tiers raise your per-market trading limit; reputation adds a limit bonus.
3. Penalties (backing out during settlement) lower your effective tier; penalty compensation can auto-top-up.

### Wallets
1. Generate a fresh deposit address for UTXO chains to avoid address reuse.
2. For account-based chains, leave headroom for gas fees, which are paid from the parent asset.
3. Built-in wallets are encrypted with the app password; external wallets keep their own passphrase.

---

## Troubleshooting

### DCRDEX page shows "starting"
**Problem**: The page is stuck on "DCRDEX is starting".

**Solutions:**
1. Wait - the page polls and recovers when the bisonw daemon is up.
2. Check the daemon is running: `docker compose ps`.
3. Check the daemon logs: `docker compose logs -f dcrdex`.

### Cannot register / "could not reach" the server
**Problem**: The registration screen cannot load the server config.

**Solutions:**
1. Check the server-connection banner; the check resumes automatically once the connection is restored.
2. Confirm outbound connectivity to `dex.decred.org:7232`.
3. After a seed restore, account discovery runs first; give it a moment to complete.

### Register button is disabled
**Problem**: "Post bond and register" stays greyed out.

**Solutions:**
1. Fund the dex account above the total bond amount (deposit DCR to the shown address).
2. Wait for the deposit to confirm; the balance updates automatically.
3. Lower the number of tiers if you do not have enough to bond.

### Order rejected or estimate error
**Problem**: An order fails to place or the pre-order estimate shows an error.

**Solutions:**
1. Ensure the funding wallet has enough available balance (a buy locks quote, a sell locks base).
2. Leave room for swap/redeem fees and reserves shown in the estimate.
3. If you exceed the estimated max lots, reduce the size.
4. The raw daemon error is shown as-is; read it for the specific cause.

### Lock fails
**Problem**: Clicking Lock shows an error and the DEX stays unlocked.

**Solutions:**
1. Stop all market-maker bots.
2. Cancel all standing orders.
3. Let any matched trades finish settling, then lock again.

### A wallet will not unlock or sync
**Problem**: An asset wallet is off, locked, or stuck syncing.

**Solutions:**
1. Use **Unlock** in the wallet detail (encrypted wallets).
2. For peer-managed wallets with no peers, add a peer address.
3. Use **Rescan** for rescannable wallets if balances look wrong.
4. Confirm the asset's daemon/backend (for external wallet types) is reachable.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - The Decred wallet the DEX trades from
- **[Node Dashboard](node-dashboard.md)** - Underlying node and daemon status
- **[Staking Guide](staking-guide.md)** - Decred staking and governance

---

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)
