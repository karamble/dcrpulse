# Privacy and the Mixer

The **Privacy** page lets you run Decred's built-in CoinShuffle++ (CSPP) mixer against your own wallet. Mixing breaks the on-chain link between the coins you receive and the coins you later spend, so an observer cannot trivially trace your funds back to their source.

## Overview

Decred mixing is account based. Instead of mixing individual coins one at a time, the wallet keeps two accounts and moves value from one to the other through repeated peer-to-peer mix cycles. Each cycle joins your outputs with outputs from other participants on the network, producing uniform mixed outputs that are difficult to distinguish from one another.

The mixer runs inside the dashboard process and drives dcrwallet over gRPC. Mix cycles are coordinated peer-to-peer over the Decred network through dcrd; the dashboard does not relay any of your coins to a third party.

**Access**: Click the **"Privacy"** button in the header navigation.

**Requires a full wallet.** Mixing needs to sign transactions and spend coins, so it only works with an RPC-connected wallet that holds private keys. Watch-only (xpub) wallets cannot mix.

---

## The Mixed and Unmixed Accounts

Mixing uses two dedicated wallet accounts:

### Unmixed (source)

- Named **`unmixed`**
- Holds funds that are **waiting to be mixed**
- This is the account the mixer spends from; in dcrwallet terms it is the **change** account
- New, un-mixed deposits should land here

### Mixed

- Named **`mixed`**
- Receives the **privacy-improved outputs** produced by each mix cycle
- This is where you should keep funds you intend to spend privately

### How funds flow

```
   unmixed account            mixed account
   (source / change)   --->   (mixed outputs)
```

The mixer repeatedly takes value from the unmixed account, runs it through a peer-to-peer mix cycle with other participants, and deposits the mixed result into the mixed account. Over time the unmixed balance falls and the mixed balance rises.

**Branch 0 (external)** is used for the mixed account's addresses.

**Important:** After mixing, spend only from the **mixed** account. Spending from the unmixed account can recombine mixed and un-mixed coins and undo the privacy you just gained.

> Both accounts are **reserved**. They cannot be renamed once created, and `mixed` / `unmixed` cannot be used as names for any other account.

---

## Enabling Privacy (Setup)

The first time you open the Privacy page, if the two accounts do not yet exist you are shown a **Set up privacy** card.

### What setup does

Clicking **Set up privacy** prompts for your wallet passphrase and then creates whichever of the `mixed` and `unmixed` accounts is missing. Setup is **idempotent**: if both accounts already exist it simply reports their account numbers and changes nothing.

### Preconditions

- The wallet must be **ready** (synced and responsive). If the wallet is still syncing, the page shows a sync gate instead of the setup card, and setup is rejected by the backend until the wallet is ready.
- The wallet must be a full RPC wallet (gRPC available).
- The correct wallet **passphrase** is required; a wrong passphrase returns a "Wrong passphrase" error.

**API**: `POST /api/wallet/privacy/setup` with the wallet passphrase. On success it returns the `mixedAccount` and `changeAccount` (unmixed) numbers. Account state is read with `GET /api/wallet/privacy/status`, which reports whether privacy is `configured`, the two account numbers, whether the mixer is currently running, and the last mixer error if any.

---

## The Privacy Page

Once the two accounts exist, the Privacy page shows the full mixer surface.

### Mixer status badge

A pill in the top-right corner shows the live mixer state:

- **Mixer running** (green) - the mixer goroutine is active and connected
- **Mixer stopped** (gray) - no mixer is running

### Balance cards

Two cards show the accounts side by side, with animated arrows between them while mixing is active:

- **Unmixed (source)** - the unmixed account's total balance, the pool of funds awaiting mixing
- **Mixed** - the mixed account's spendable plus unconfirmed balance, the privacy-improved funds

Balances are shown with **8 decimal places** and refresh every 5 seconds.

### Configuration card

A read-only summary of how the mixer is wired:

- **Mixed account** - the `mixed` account name and number
- **Unmixed account** - the `unmixed` account name and number
- **Branch** - `0 (external)`
- **Network** - peer-to-peer via dcrd

### Send to unmixed

A small send form to **move funds into the unmixed account** so the mixer has something to work on. You choose a source account (the reserved accounts and the imported account are excluded), enter an amount or check **Send all**, and the dashboard builds, previews (network fee and total debited), and then signs and publishes the transfer to a freshly derived unmixed address after you enter your passphrase.

> This send is subject to the same constraint as any other send: it is blocked while the mixer or the ticket autobuyer is running (see Constraints below). Use it to top up the unmixed account before you start mixing.

---

## Starting and Stopping Mixing

### Start the mixer

Click **Start mixer**, enter your wallet passphrase, and confirm. The dashboard launches a background mixer goroutine that:

1. Unlocks the unmixed (change) account for signing, for the lifetime of the run
2. Opens a `RunAccountMixer` gRPC stream against dcrwallet
3. Waits for peers and processes mix-cycle events until you stop it

**API**: `POST /api/wallet/privacy/start` with the wallet passphrase.

**The dashboard container must stay running** for mixing to continue. If the dashboard stops, the mixer stops with it. Mix cycles only complete when enough peers are paired on the network, so it is normal for the mixer to sit at "awaiting peers" for a while.

### Stop the mixer

Click **Stop mixer**. The mixer goroutine is cancelled and the unmixed (change) account is re-locked.

**API**: `POST /api/wallet/privacy/stop`. Calling stop when nothing is running is a safe no-op.

### Watching mixer progress and events

The page includes a collapsible **Mixer events** log that streams structured events live over a WebSocket (`GET /api/wallet/privacy/events`). On connect it replays up to the last **200** events, then appends new ones in real time. Events include lines such as:

- `Mixer starting (mixed=... branch=... change=...)`
- `Mixer connected; awaiting peers`
- `Mix cycle event` (emitted as cycles progress)
- `Mixer stopped` / `Mixer stream closed by daemon`
- error lines (for example, an unlock failure or a stream error)

Each entry shows a timestamp, a level (info / warn / error), and the message. The most recent terminal error is also surfaced on the page as a **Last error** banner when the mixer is not running.

> Mixer events are intentionally coarse. The log tells you that the mixer started, connected, ran cycles, and stopped; it is a status feed, not a per-coin accounting of every mix.

### Mixer debug logging

For deeper diagnostics, **Settings > Privacy & Security** has a **Mixer debug logging** toggle. It flips dcrwallet's `MIXC` and `TKBY` subsystems between `info` and `debug` so detailed mixer and ticket-buyer logs appear in the dcrwallet container. It is verbose, so leave it off unless you are troubleshooting.

**API**: `GET /api/wallet/mixer/debug` reads the current state; `POST /api/wallet/mixer/debug` with `{ "enabled": true|false }` toggles it.

---

## How Mixing Interacts with Staking and Sending

The mixer, the ticket autobuyer, and any manual ticket purchase or send all spend the same wallet UTXOs, so the dashboard enforces that they do not run at the same time. The constraints below mirror Decrediton's behavior.

### Sending is blocked while mixing

A regular send (including the **Send to unmixed** form and the on-chain Send tab) is refused while the **mixer** or the **autobuyer** is running. The backend returns a conflict with a message to stop the privacy mixer or ticket autobuyer first. Stop the mixer, send, then start it again.

### Starting the mixer is blocked during staking activity

You cannot start the mixer while either of these is active:

- The **ticket autobuyer** is running (it mixes its ticket buys inline as it runs). The Start button is disabled and a warning explains you must stop the autobuyer first.
- A **manual ticket purchase** is in progress. Starting the mixer returns a conflict; try again once the purchase finishes.

### Ticket purchases automatically pause and restart the mixer

When privacy is configured, ticket purchases route through the mixed and unmixed accounts (the mixed account funds and splits the buy; the unmixed account receives change). If you start a ticket purchase while the mixer is running, the dashboard:

1. Stops the running mixer and waits for it to fully stop
2. Performs the purchase, mixing the ticket inline
3. Restarts the mixer afterward

You do not need to stop the mixer yourself before buying tickets; the purchase handles it.

### Spending from mixed funds

With privacy configured, both ticket purchasing and the recommended spend path use the **mixed** account as the funding source, so the coins you commit to tickets or send are mixed coins. Keep spendable balances in the mixed account and treat the unmixed account purely as the mixer's input.

---

## API Routes

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/wallet/privacy/status` | Whether privacy is configured, the mixed/unmixed account numbers, mixer running state, last error |
| POST | `/api/wallet/privacy/setup` | Create the missing `mixed` / `unmixed` accounts (idempotent) |
| POST | `/api/wallet/privacy/start` | Start the mixer (requires passphrase) |
| POST | `/api/wallet/privacy/stop` | Stop the mixer (no-op if not running) |
| GET | `/api/wallet/privacy/events` | WebSocket stream of mixer events (replays last 200) |
| GET / POST | `/api/wallet/mixer/debug` | Read or toggle MIXC + TKBY debug logging |

---

## Tips and Best Practices

### Before you mix
1. Make sure the wallet is fully synced (the page gates setup until it is ready)
2. Move the coins you want to mix into the **unmixed** account, using **Send to unmixed**
3. Start the mixer and leave the dashboard running

### While mixing
1. Expect to wait for peers; mix cycles only complete when enough participants are paired
2. Watch the **Mixer events** log to confirm the mixer connected and is running cycles
3. Do not start the autobuyer or attempt a manual send while you want the mixer to keep running

### After mixing
1. Spend only from the **mixed** account to preserve the privacy you gained
2. Do not send from the **unmixed** account, which holds un-mixed coins
3. Stop the mixer when you no longer need it; it will re-lock the unmixed account

---

## Troubleshooting

### Set up privacy card keeps showing / setup is rejected
**Problem**: The page shows a sync gate or setup returns 503

**Solutions:**
1. Wait for the wallet to finish syncing; setup is gated on a ready wallet
2. Confirm you are on a full RPC wallet, not a watch-only xpub wallet
3. Re-enter the passphrase carefully; a wrong passphrase returns "Wrong passphrase"

### Mixer will not start
**Problem**: Start mixer is disabled or returns a conflict

**Solutions:**
1. Stop the **ticket autobuyer** if it is running
2. Wait for any in-progress **ticket purchase** to finish, then try again
3. Check the **Last error** banner and the **Mixer events** log for the reason

### Mixer is running but balances are not moving
**Problem**: Unmixed balance is not decreasing

**Solutions:**
1. Confirm the unmixed account actually holds funds (use **Send to unmixed**)
2. Give it time; cycles complete only when enough peers are paired
3. Enable **Mixer debug logging** in Settings and check the dcrwallet container logs

### "Stop the privacy mixer or ticket autobuyer before sending"
**Problem**: A send is refused

**Solutions:**
1. Stop the mixer (or the autobuyer), send the transaction, then restart it
2. Remember the **Send to unmixed** form is also a send and follows the same rule

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - Accounts, balances, and transaction history
- **[Staking Guide](staking-guide.md)** - Ticket purchasing and the autobuyer
- **[Wallet Setup](../guides/wallet-operations.md)** - Initial wallet configuration

---

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)
