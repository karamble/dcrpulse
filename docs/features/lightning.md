# Lightning Network

The **Lightning Network** integration lets your dcrpulse wallet open payment channels and send or receive fast, low-fee Decred payments off-chain. It is powered by a dedicated **dcrlnd** daemon that runs alongside your dcrd and dcrwallet stack.

## Overview

Lightning is a layer-2 payment network. Instead of broadcasting every payment to the Decred blockchain, you open a funded channel to a counterparty once, then exchange many payments instantly within that channel. Settlement back to the chain happens only when a channel is closed.

In dcrpulse, Lightning is managed by **dcrlnd**, which keeps its own internal wallet seed and its own funds, isolated in a dedicated dcrwallet account named `lightning`. The dashboard talks to dcrlnd over gRPC and surfaces everything through a tabbed Lightning area.

**Access**: Open the **Wallet** section, then click **Lightning** in the wallet sub-navigation. The Lightning area itself has five tabs: **Overview**, **Channels**, **Send**, **Receive**, and **Advanced**.

**What you need before you can pay or get paid:**
- The main wallet must be **synced and responsive** before Lightning can be enabled.
- The Lightning wallet must be **unlocked** (this happens during setup, and again after a dashboard restart).
- dcrlnd needs at least one **peer** connected so it can learn the channel graph.
- You need at least one **open channel** with **outbound** capacity to send, and **inbound** capacity to receive.

---

## Lightning Lifecycle

The Lightning page chooses what to show based on a high-level **stage** reported by the backend (`GET /api/wallet/ln/status`). The page polls this stage every 10 seconds and moves between screens automatically.

### Stages

**needs-setup**
- Lightning has never been enabled on this wallet.
- If the main wallet is not yet synced, a sync gate is shown instead of the wizard.
- Otherwise the setup wizard is shown.

**needs-unlock**
- Lightning is set up and dcrlnd is running, but the Lightning wallet is locked (typical after a dashboard restart).
- The unlock form is shown.

**unavailable**
- dcrlnd is starting or temporarily unreachable.
- A friendly status message is shown and the page recovers automatically once dcrlnd is ready.

**syncing / ready**
- The full tabbed Lightning layout is rendered, starting on the Overview tab.

---

## First-Time Setup

When Lightning has never been enabled, a **setup wizard** walks you through activation.

### Step 1: Disclaimer

The wizard opens with an **"Enable Lightning Network"** disclaimer that you must acknowledge. It explains:

- Lightning is **experimental**.
- Funds in channels can be **partially or fully lost** if a counterparty misbehaves and you do not have a recent channel backup.
- You **must** back up your **Static Channel Backup (SCB)** after opening any channel; without it, channel-state recovery is impossible.
- **Force-closing** a channel ties up funds for a multi-day timelock.
- The Lightning wallet uses its **own internal seed** managed by dcrlnd. The dashboard does not display this seed; recovery happens through the SCB mechanism.
- Continuing creates a dedicated dcrwallet account named `lightning` that isolates Lightning funds from your main on-chain balance.

Click **"I Understand, Continue"** to proceed.

### Step 2: Passphrase

Enter your **wallet passphrase** (minimum 8 characters). This is used to:

1. Create the dedicated `lightning` dcrwallet account.
2. Unblock and start the dcrlnd daemon.
3. Run dcrlnd's first-time `InitWallet`.

Click **"Enable Lightning"**.

### Step 3: Daemon Boot

The dcrlnd daemon takes a short while to notice it has been unblocked, start, and begin listening. The wizard shows a live progress message with an elapsed-seconds counter:

```
Connecting to Lightning daemon (12s)... this can take 1-2 minutes.
```

During this window the dashboard repeatedly attempts to unlock the Lightning wallet until dcrlnd reports it is ready. If the daemon does not come up after **3 minutes**, an error is shown and you can retry from the passphrase step.

Once dcrlnd is ready, the wizard shows **"Lightning is ready"** and loads the Overview tab.

---

## Unlocking After a Restart

After a dashboard restart with the dcrlnd volume intact, Lightning enters the **needs-unlock** stage. The wizard renders an **"Unlock Lightning Wallet"** form.

- Enter your **wallet passphrase** and click **Unlock**.
- The dashboard performs a single auto-unlock attempt, then surfaces any error in the form.
- A wrong passphrase returns a **"Wrong passphrase"** message.

You do not re-run setup; your `lightning` account and channels are preserved.

---

## Overview Tab

The Overview tab is the Lightning home screen. It auto-refreshes every **15 seconds**.

### Node Summary Bar

A header bar shows your node identity and live sync state:

- **Alias** and **version** of your Lightning node.
- **Identity pubkey** (the node's public key).
- **Chain**: synced or syncing.
- **Graph**: synced or syncing.
- **Peers**: number of connected peers (highlighted when zero).

### Sync Hints

When the node shows as still "syncing", a contextual hint explains why:

- **No peers connected**: dcrlnd cannot receive the gossiped channel graph until at least one peer is reachable, so the graph never finishes syncing.
- **Peers but no channels**: the node is idle. A link is offered to **open a channel** so you can start routing.

### Balance Cards

A grid of six balance cards summarizes your funds:

- **On-chain confirmed** - spendable on-chain funds in the Lightning account.
- **On-chain unconfirmed** - on-chain funds awaiting confirmation.
- **On-chain total** - confirmed plus unconfirmed.
- **Channel local** - your spendable balance inside channels (outbound capacity), with the active channel count.
- **Channel remote** - the counterparty side of your channels (inbound capacity).
- **Pending channels** - funds in channels that are still opening or closing, with the pending channel count.

### Recent Activity

A merged feed of recent **invoices** and **payments**, each showing kind, state, optional memo, timestamp, and amount.

### Network Statistics

A panel of global Lightning network metrics sourced from dcrlnd's view of the graph:

- **Nodes** and **Channels** counts.
- **Total capacity**, **Avg channel size**, **Median channel size**.
- **Graph diameter** (hops), **Smallest** and **Largest** channel.
- **Avg out-degree**.
- A **Top 10 nodes by capacity** table (alias, pubkey, channel count, capacity). Until peers gossip channel data, this table is empty.

---

## Channels Tab

The Channels tab is where you fund and manage your payment channels.

### Autopilot

At the top is an **Autopilot** toggle. When enabled, dcrlnd automatically opens channels using **up to 60% of the lightning account's spendable funds**. Click the toggle to switch it **Enabled** or **Disabled**.

### Funding Balance

A compact balance row shows what is available to fund new channels:

- **Spendable** (on-chain confirmed)
- **Unconfirmed**
- **In channels** (channel local)
- **Pending**

### Request an Inbound Channel

To receive payments you need **inbound** capacity, which an outbound-only node lacks. The **"Request Inbound Channel"** button opens a wizard that asks a liquidity provider to open a channel back to your node for a small fee.

**Step 1 - Form:**
- Shows your current **Outbound**, **Inbound**, **Pending**, and **Channels** figures.
- Warns if you have **no outbound capacity**, since you need outbound funds to pay the provider's fee.
- Enter **Add Inbound Capacity (DCR)** (minimum 0.00001 DCR).
- An **Advanced** section lets you override the **LP Server Address** and **LP Server Cert (PEM)**. By default these are pre-filled from the built-in provider for the active network.
- Click **Continue** to fetch the provider's policy and a fee estimate.

**Step 2 - Confirm:**
- Shows the **requested channel size**, **estimated fee**, **minimum channel lifetime**, **max channels per node**, and the **server node** and addresses.
- Warns that the provider may close the channel after the minimum lifetime if not enough payments flow through it, and that the channel becomes active after up to 6 confirmations.
- Click **Pay** to pay the provider's invoice and request the channel.

**Step 3 - Progress:**
- The dashboard pays the invoice and waits for the provider's channel to appear pending.
- On success it shows the new **channel point** and capacity. Click **Done**.

### Create a Channel

The **"Create a channel"** form opens an outbound channel to a counterparty you choose.

**Counterparty node** - enter either a bare node **pubkey** (66 hex characters) or `pubkey@host:port`. Three helpers make this easier:

- **Presets** dropdown - a list of peer presets fetched from **Bison Relay's seeder** (bisonrelay.org). Presets can be disabled under **Settings > Privacy** if you prefer manual entry only. A built-in fallback peer keeps the list non-empty.
- A **datalist** autocomplete on the input itself.
- **Search graph** - opens a node search modal so you can pick a known node from dcrlnd's channel graph by alias or pubkey.

> Note: the Bison Relay setup flow can deep-link into this form with the hub peer pre-filled (via a `?peer=` link), so you land directly on the funding action.

**Funding amounts:**
- **Local funding (DCR)** - how much you commit to the channel; this is your starting outbound balance.
- **Push amount (DCR, optional)** - an amount to immediately push to the counterparty (must be less than the local funding).

Click **Open channel**. The backend connects to the peer and opens the channel; on success it reports the **pending funding outpoint** (`fundingTxid:outputIndex`). The new channel then appears in the list as **Pending open** until it gains the required confirmations.

### Channel List

Below the form, the **Channels** list shows all of your channels. It refreshes live via a WebSocket channel-events subscription and also has a manual **Refresh** button.

**Filter** by **All**, **Open**, **Pending**, or **Closed**.

Each channel row shows:
- Counterparty **alias** (or truncated pubkey) and truncated **channel point**.
- A **status badge**: Active, Inactive, Pending open (with `confs/required`), Closing, Force-closing, Waiting close, or Closed.
- **Local**, **Capacity**, and **Remote** balances, with a bar visualizing the local-vs-remote split.

Click a row to open its detail page.

### Channel Details

The detail page shows a field set that depends on the channel's status:

- **All channels**: funding tx (linked to the block explorer), channel point, remote pubkey, capacity.
- **Open channels**: channel ID, local/remote balance, commit fee, CSV delay, unsettled balance, total sent/received, update count, initiator, private, active flags.
- **Pending channels**: local/remote balance, limbo balance, closing tx, and for pending-open, a confirmations counter.
- **Closed channels**: close type, closing tx, settled balance, time-locked balance.

For **open** and **pending-open** channels, a **Close channel** button is available.

### Closing a Channel

The close modal adapts to the channel's state:

- **Cooperative close** (counterparty reachable): negotiates a mutual close. Funds become spendable on-chain after the close transaction confirms.
- **Force-close** (counterparty offline): publishes the latest commitment transaction and ties your funds in a multi-day **CSV timelock** before they become spendable. The modal warns that force-close is only appropriate when the peer is unreachable; the confirm button is styled as destructive.

The dashboard chooses cooperative vs force automatically based on whether the channel is active, and the request returns once the close is pending.

---

## Send Tab

The Send tab pays a **BOLT-11** Lightning invoice.

### Paying an Invoice

1. Paste a Lightning **payment request** (`lnbc...`) into the text area. A **Paste** button reads from the clipboard; a **Clear** button resets the form.
2. The invoice is **decoded on the fly** (after a short debounce) and a preview card appears. If the invoice is invalid, a decode error is shown.

The decoded preview shows:
- **Amount** - the invoice amount. If the invoice has **no fixed amount**, an input lets you choose how much to send.
- **Destination** node pubkey (copyable).
- **Expiration** - a live countdown; an expired invoice is flagged and cannot be paid.
- **Description** (if present).
- **Payment hash** (copyable).
- A **Show invoice details** toggle reveals CLTV expiry, fallback address, and payment address.

3. Click **Send Payment**. The payment streams its progress from dcrlnd over a WebSocket, so the in-flight row updates live from **pending** to **succeeded** or **failed**. The row appears immediately at the top of the history while it is in flight.

### Payment History

Below the form is your **Lightning payments** history:

- **Filter** by **All**, **Confirmed**, **Pending**, or **Failed**.
- Toggle sort between **Newest first** and **Oldest first**.
- **Search** by payment hash or description.
- The list polls every 10 seconds. Click any payment for a details modal.

---

## Receive Tab

The Receive tab creates a **BOLT-11** invoice for someone to pay you.

### Creating an Invoice

1. Optionally enter an **Amount** in DCR. Leave it blank for an **open-amount** invoice that the payer fills in.
2. Optionally enter a **Description** (up to 639 characters) shown to the payer.
3. Click **Create invoice**.

### Current Invoice Card

The newly created invoice appears in a card showing:

- The amount (or "Open amount"), description, and **status**.
- A live **Expires in ...** countdown (or "Settled ..." once paid).
- The full **payment request** string with a **Copy payment request** button.

Open invoices update **live** via a WebSocket subscription as they settle, expire, or are canceled.

### Invoice History

Below is your **Lightning invoices** history:

- **Filter** by **All**, **Open**, **Settled**, **Expired**, or **Canceled**.
- Toggle sort between **Newest first** and **Oldest first**.
- **Search** by hash or memo.
- Click any invoice for a details modal, where an **open** invoice can be **canceled**.

---

## Advanced Tab

The Advanced tab groups node info, backups, watchtowers, and channel-graph queries.

### Node Info

Read-only node identity:
- **Alias**
- **Identity pubkey** (copyable)
- **Block height**

### Channel Backup

The most important safety feature for a Lightning node.

- **Download backup** - exports the **Static Channel Backup (SCB)** of all your channels as a `.scb` file. The result reports how many channels were backed up. Store this file somewhere safe; without it, channel-state recovery is impossible if dcrlnd's local state is lost.
- **Verify backup** - choose a previously saved `.scb` file and the backend validates it, reporting whether the backup is valid.

> Back up the SCB again after every channel open or close, since the file reflects your current channel set.

### Watchtowers

Register **watchtower** clients. A watchtower monitors your channels for breach attempts while your node is offline.

- **Add a watchtower** by entering its **pubkey** (66 hex) and **address** (`host:port`).
- The **Registered** list shows each tower's pubkey (copyable), addresses, session count, and an **Active/Inactive** status pill, with a **Remove** action.

### Network (Graph Queries)

Two graph-inspection tools:

- **Query node** - paste a 66-hex node pubkey to look up its graph entry: alias, total capacity, last update, pubkey, and its channels (channel point, capacity, the two endpoint pubkeys).
- **Query routes** - enter a destination **pubkey** and an **amount** to find candidate payment routes. Results show an overall **success probability**, the number of routes, and per-route totals and per-hop fees. This does not send anything; it only probes the graph.

---

## API Reference

All Lightning endpoints live under the `/api/wallet/ln/` route group. The dashboard backend proxies them to dcrlnd over gRPC.

### Lifecycle and Status
- `GET  /api/wallet/ln/status` - high-level stage (needs-setup, needs-unlock, unavailable, syncing, ready)
- `POST /api/wallet/ln/setup` - create the lightning account, start dcrlnd, run first-time InitWallet
- `POST /api/wallet/ln/unlock` - unlock the Lightning wallet on subsequent starts

### Overview
- `GET  /api/wallet/ln/info` - node GetInfo (alias, pubkey, sync flags, peer/channel counts)
- `GET  /api/wallet/ln/balance` - merged wallet and channel balances
- `GET  /api/wallet/ln/activity` - merged recent invoices and payments
- `GET  /api/wallet/ln/network` - global network statistics plus top nodes

### Channels
- `GET  /api/wallet/ln/channels` - open, pending, and closed channels
- `POST /api/wallet/ln/channels/open` - connect to a peer and open a channel
- `POST /api/wallet/ln/channels/close` - cooperative or force close
- `GET  /api/wallet/ln/channel-events` - WebSocket stream of channel events
- `GET  /api/wallet/ln/peer-presets` - peer presets from the Bison Relay seeder
- `GET  /api/wallet/ln/autopilot` - read autopilot status
- `POST /api/wallet/ln/autopilot` - toggle autopilot
- `GET  /api/wallet/ln/graph/search` - substring search of graph nodes

### Inbound Liquidity
- `GET  /api/wallet/ln/liquidity/defaults` - built-in liquidity provider for the active network
- `POST /api/wallet/ln/liquidity/estimate` - fetch the provider policy and fee estimate
- `POST /api/wallet/ln/liquidity/request` - pay the provider and request the inbound channel

### Send
- `POST /api/wallet/ln/send/decode` - decode a BOLT-11 invoice
- `GET  /api/wallet/ln/send` - WebSocket that streams payment progress
- `GET  /api/wallet/ln/payments` - payment history

### Receive
- `POST /api/wallet/ln/invoices/add` - create an invoice
- `GET  /api/wallet/ln/invoices` - invoice history
- `POST /api/wallet/ln/invoices/cancel` - cancel an open invoice
- `GET  /api/wallet/ln/invoice-events` - WebSocket stream of invoice updates

### Advanced
- `GET  /api/wallet/ln/backup` - export the Static Channel Backup
- `POST /api/wallet/ln/backup/verify` - validate an uploaded backup blob
- `GET  /api/wallet/ln/watchtowers` - list registered watchtowers
- `POST /api/wallet/ln/watchtowers/add` - register a watchtower
- `POST /api/wallet/ln/watchtowers/remove` - deregister a watchtower
- `GET  /api/wallet/ln/graph/node` - query one node from the channel graph
- `POST /api/wallet/ln/graph/routes` - query candidate payment routes

---

## Tips and Best Practices

### Getting Started
1. Make sure your main wallet is fully synced before enabling Lightning.
2. Fund the `lightning` account with on-chain DCR before opening channels.
3. Open at least one channel to a well-connected peer to start routing.
4. Use **Request Inbound Channel** if you intend to receive payments.

### Protecting Your Funds
1. Download the **SCB** after every channel open or close, and keep copies off the host.
2. Prefer **cooperative** closes; only **force-close** when a peer is truly unreachable.
3. Consider registering a **watchtower** so your channels are protected while offline.
4. Remember that channel funds are isolated in the dedicated `lightning` account.

### Sending and Receiving
1. Check an invoice's **expiration** before paying; expired invoices cannot be paid.
2. For open-amount invoices, set the amount you intend to send in the decode preview.
3. Leave the amount blank when creating an invoice if you want the payer to choose.
4. Watch the live activity and history lists to confirm settlement.

---

## Troubleshooting

### Lightning Will Not Enable
**Problem**: The setup wizard shows a sync gate instead of the passphrase form.

**Solutions:**
1. Wait for the main wallet to finish syncing and become responsive.
2. Verify dcrwallet is running and reachable.

### Daemon Does Not Come Up
**Problem**: Setup is stuck on "Connecting to Lightning daemon ...".

**Solutions:**
1. Allow 1-2 minutes; dcrlnd boot and unlock can race on first start.
2. If it fails after 3 minutes, retry from the passphrase step.
3. Check the dashboard and dcrlnd logs.

### Node Stuck on "syncing"
**Problem**: Chain or graph never shows synced.

**Solutions:**
1. **No peers**: dcrlnd cannot fetch the channel graph without a peer. Open a channel or connect to a peer.
2. **No channels**: the node is idle. Open a channel to begin routing.

### Wrong Passphrase on Unlock
**Problem**: Unlock returns "Wrong passphrase".

**Solutions:**
1. Re-enter the same passphrase you use for the main wallet.
2. Confirm the Lightning wallet was initialized with that passphrase during setup.

### Cannot Receive Payments
**Problem**: Payers cannot pay your invoice.

**Solutions:**
1. You likely lack **inbound** capacity. Use **Request Inbound Channel**.
2. Confirm at least one channel is **Active** (not just pending).

### Cannot Send Payments
**Problem**: Payments fail.

**Solutions:**
1. Confirm you have enough **outbound** (channel local) balance.
2. Check the invoice has not **expired**.
3. Use **Advanced > Network > Query routes** to see whether a route exists.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - balances, transactions, and accounts
- **[Wallet Setup](../guides/wallet-operations.md)** - initial wallet configuration

---

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)
