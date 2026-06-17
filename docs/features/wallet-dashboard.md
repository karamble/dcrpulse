# Wallet Dashboard

The **Wallet Dashboard** provides comprehensive monitoring and management of your Decred wallet, including account balances, transaction history, staking information, and ticket management.

## Overview

The Wallet Dashboard displays real-time information about your wallet's financial status and staking activities. It's designed to work with both full wallets (via RPC) and watch-only wallets (via imported xpub keys).

**Access**: Open `http://localhost:8080` and click the **"Wallet"** button in the header navigation.

> Decred Pulse runs as a single `dashboard` service on port `8080`; it serves both the API and the bundled web interface from the same address.

### The Wallet area is a multi-section hub

The Wallet area is organized as a hub with a left-hand sidebar. The **Overview** page (this document) is the landing view; the other sections each have their own dedicated guide:

- **Overview** - balances, recent transactions, ticket summary, accounts (this page)
- **On-Chain Transactions** - send, receive, full transaction history, export
- **Privacy** - see [Privacy Mixer](privacy-mixer.md)
- **Staking** - see [Staking Guide](staking-guide.md)
- **Governance** - see [Governance](governance.md)
- **Lightning** - see [Lightning](lightning.md)
- **Accounts** - account management
- **Timestamp** - see [Timestamp](timestamp.md)
- **Settings** - wallet, privacy, logs, themes, security, and Tor settings
- **Switch Wallet** - see [Multiple Wallets](multi-wallet.md)

The rest of this page describes the **Overview** landing view.

---

## Dashboard Components

### 1. Account Balance Card

Displays wallet-wide balance information:

#### Cumulative Total
- **Total balance** across all accounts
- Large display showing the first **2 decimal places** at full size, with the remaining decimals in a smaller font

#### Total Spendable
- **Available balance** for transactions
- Excludes locked and immature funds
- Can be used for sending or ticket purchases

#### Conditional rows
The following rows appear only when the corresponding balance is non-zero:

- **Locked by Tickets** - funds locked in active tickets; automatically unlocked when tickets vote or expire
- **Immature** - mining and stake rewards awaiting maturity
- **Unconfirmed** - amounts from transactions still awaiting confirmation

A **Watch-Only Mode** note is shown at the bottom of the card, explaining that the wallet can monitor balances and transactions but cannot spend funds.

**Visual Features:**
- Large, prominent balance display
- Color-coded for quick scanning
- Real-time updates every 10 seconds

---

### 2. Recent Transactions Card

The Overview shows a compact **Recent Transactions** card:

#### Features
- **Latest Activity**: Shows the **5 most recent** transactions
- **View All**: A "View all" link opens the full transaction history under **On-Chain Transactions** (`/wallet/transactions/history`)
- **Clickable Transactions**: Click any transaction to open it in the built-in Decred Pulse explorer (`/explorer/tx/<txid>`)

#### Transaction Information
Each row shows:
- **Type Icon**: Visual indicator (send, receive, ticket, vote, revocation, CoinJoin, VSP fee, mined)
- **Label**: Sent, Received, Ticket Purchase, Vote, Revocation, CoinJoin, VSP Fee, Mined
- **Amount**: Signed transaction value in DCR
- **Time**: Relative time (e.g., "2h ago") or formatted date

#### Transaction Types

**Regular Transactions:**
- **Received** - Green arrow down
- **Sent** - Red arrow up

**Privacy:**
- **CoinJoin** - Purple shuffle icon (mixed transaction)
- **VSP Fee** - Orange badge icon (voting service provider fee)

**Staking Transactions:**
- **Ticket Purchase** - Yellow ticket icon
- **Vote** - Green checkmark (ticket voted)
- **Revocation** - Red X (ticket revoked)

**Mining/Generation:**
- **Mined** - Coins icon (generated/coinbase)

> The full transaction history (with filters by category, address/txid search, date range, a CoinJoin statistics card, and progressive "Load More" loading) lives under **On-Chain Transactions > History**. See the section below.

---

### 2a. Full Transaction History (On-Chain Transactions > History)

The dedicated history view is reached from the **On-Chain Transactions** sidebar item (or the "View all" link on the Overview).

#### Features
- **Initial fetch**: Loads up to **200** transactions
- **Show Details**: Expand to reveal the filterable list
- **Filters**: All, Send, Receive, CoinJoin, Tickets, Votes
- **Search**: Filter by address or txid; filter by start/end date
- **Progressive Loading**: "Load More" reveals more from the loaded set, then fetches additional batches of **50** from the server
- **CoinJoin Statistics**: A summary card shows count, min/max/avg CoinJoin fees

#### Transaction Information
Each row shows:
- **Type Icon** and **Label** (same set as above, plus Immature)
- **Account** badge (the wallet account the transaction belongs to)
- **Amount**: Signed value in DCR, with the network **Fee** below when applicable
- **Truncated txid**, relative time, and truncated address
- Clicking a row opens it in the built-in explorer

#### Status Badges
- **Pending**: Warning-colored badge for 0 confirmations
- **N conf**: Muted badge while a transaction has 1 to 5 confirmations (no badge once it reaches 6+)

---

### 3. Accounts Card

Lists all wallet accounts with a detailed balance breakdown. A "Manage accounts" link opens the full **Accounts** section.

#### Account Information
For each account, displays:
- **Account Name**: default, mixed, unmixed, imported, etc.
- **Total Balance**: Overall account balance

#### Granular Balance Details

Each account always shows its **Spendable** balance, plus any of the following that are non-zero:

**Spendable**
- Funds available for immediate use
- Can send or purchase tickets

**Locked by Tickets**
- Funds locked in active tickets
- Released when ticket votes/expires

**Immature**
- Mining and stake-generation rewards awaiting maturity
- Requires 256 confirmations

**Unconfirmed**
- Transactions pending confirmation
- Not yet spendable

**Voting Authority**
- Funds in tickets you have voting rights for
- May not own the ticket

#### Formatting
- All balances shown with **2 decimal places**
- Icons for quick visual identification
- Compact, space-efficient layout
- Only non-zero balances displayed (Spendable is always shown)

---

### 4. Ticket Pool & Difficulty Card

Global Decred network ticket pool information:

#### Current Network Statistics

**Pool Size**
- Total number of live tickets in the pool
- Target: ~40,960 tickets

**Mempool Tickets**
- Pending ticket purchases across the network
- Awaiting confirmation

**Current Price**
- **Current ticket price** in DCR
- Price you'll pay for the next ticket purchase

**Expected Next**
- Estimated ticket price for the next window, with the change vs. the current price
- The ticket price adjusts every 144 blocks

**Expected Price Range**
- **Min**: Minimum estimated price
- **Expected**: Most likely price
- **Max**: Maximum estimated price

---

### 5. My Tickets Card

Your personal ticket statistics. Each count below is shown only when it is non-zero, and you can expand the card ("Show Details") to browse a filterable per-ticket list (All / Live / Immature / Voted):

#### Your Ticket Counts

**Mempool**
- Your tickets waiting for confirmation
- Not yet active in pool

**Immature**
- Recently purchased tickets
- Require 256 confirmations (~21 hours)

**Live**
- Active tickets in the pool
- Eligible to vote

**Voted**
- Tickets that have voted
- Earned voting rewards

**Revoked**
- Expired/missed tickets that were revoked
- Funds returned (minus fee)

**Expired**
- Tickets that expired without voting
- Can be revoked to recover funds

**Total Staking Rewards**
- Total voting rewards earned (shown when greater than zero)
- Accumulated from all votes

#### Watch-Only Wallet Disclaimer

When no tickets are detected, the card shows:
```
No tickets found

Connect to an external wallet via RPC to see stats

Tickets cannot be detected on watch-only wallets with imported x-pub keys.
A full wallet is required to track staking activity.
```

**Why?** Watch-only wallets (xpub imports) cannot access ticket information because:
- Ticket data requires private key access
- Xpub only provides address monitoring
- Connect via RPC for full ticket tracking

> The Overview shows only a summary. For ticket purchasing, the autobuyer, VSP setup, and detailed status, see the [Staking Guide](staking-guide.md).

---

### 6. Block Subsidy Card

Shows the current Proof-of-Stake reward per block, derived from `dcrd`:

- **Total Block Subsidy**: The full per-block reward
- **Per Vote**: PoS reward per ticket vote
- **PoS Share / PoW / Treasury**: How the subsidy splits across stakers, miners, and the treasury

It also reflects how many blocks remain until the next subsidy reduction.

---

### 7. Address Bookmarks Card

A local list of saved Decred addresses with friendly labels:

- Add, edit, and remove bookmarks (stored in your browser)
- Copy an address to the clipboard
- Import and export bookmarks as JSON
- Shows the first 5 by default, with an option to reveal all

These bookmarks are also used by the built-in explorer when viewing addresses.

---

## Sync Progress Tracking

When performing wallet operations (rescan, xpub import), a **sync progress bar** appears:

### Features
- **Real-time Progress**: Shows percentage complete, scan height, and chain height
- **Event-Driven**: Driven by a WebSocket stream from the wallet (roughly once per second), not by polling
- **Preparing State**: An immediate "Preparing Rescan..." state shows while addresses are discovered, before the scan height is known
- **Auto-Hide**: The bar appears while the wallet is behind the chain and disappears as soon as it is synced

### During Sync
- Dashboard cards are hidden
- Only the sync progress bar (or the preparing state) is visible
- This avoids flooding the wallet RPC with balance/account queries while it scans

### After Sync Completion
- Progress bar automatically hides
- Dashboard cards reappear
- Balances refresh on the normal interval (every 10 seconds)

---

## Configuration

### Auto-Refresh
The Overview refreshes balances and account data every **10 seconds** automatically. Sync status is delivered separately over a WebSocket stream.

To adjust the refresh interval, modify:
```typescript
// dashboard/web/src/pages/WalletDashboard.tsx
useEffect(() => {
  const interval = setInterval(fetchData, 10000); // Change 10000 to desired ms
  return () => clearInterval(interval);
}, []);
```

### Transaction Display Limits
The full transaction history (On-Chain Transactions > History) uses these defaults:
- Initial display: **5 transactions** (the Overview "Recent Transactions" card shows the latest 5)
- Initial fetch: **200 transactions**
- "Load More": reveals **10** more from the loaded set, then fetches additional batches of **50** from the server

To adjust:
```typescript
// dashboard/web/src/components/TransactionHistory.tsx
const [visibleCount, setVisibleCount] = useState(5);  // Initial display
const loadMoreCount = 10;       // Reveal increment (already-loaded)
const lazyLoadBatchSize = 50;   // Server fetch batch size

// Initial server fetch
const data = await getWalletTransactions(200);  // Total to fetch
```

---

## Related Documentation

- **[Wallet Operations](../guides/wallet-operations.md)** - Initial setup, import xpub, rescan, sync
- **[Staking Guide](staking-guide.md)** - Complete staking information
- **[Governance](governance.md)** - Consensus, treasury, and proposal voting
- **[Lightning](lightning.md)** - Lightning Network channels and payments
- **[Privacy Mixer](privacy-mixer.md)** - CoinJoin mixing
- **[Multiple Wallets](multi-wallet.md)** - Managing several wallets
- **[Timestamp](timestamp.md)** - File timestamping with dcrtime
- **[Backup & Restore](../guides/backup-restore.md)** - Protect your funds
- **[API Reference](../api/api-reference.md)** - Wallet API endpoints

---

## Tips & Best Practices

### For Best Performance
1. Wait for full blockchain sync before rescanning
2. Use appropriate gap limit (400 recommended)
3. Don't navigate away during wallet rescan
4. Monitor sync progress through dashboard

### For Accurate Balance Display
1. Allow time for confirmations to mature
2. Check "immature" balances for pending rewards
3. Remember locked balance requires ticket voting/expiry
4. Refresh dashboard if balances seem outdated

### For Watch-Only Wallets
1. Import xpub for address monitoring
2. Use appropriate gap limit (200+)
3. Ticket info requires full RPC connection
4. Cannot send transactions (watch-only)

### For Transaction History
1. Click transactions to view blockchain details
2. Use "Load More" for older transactions
3. Check confirmations before considering final
4. Reference txid for support/tracking

---

## Troubleshooting

### Balances Not Updating
**Problem**: Dashboard shows outdated balances

**Solutions:**
1. Check wallet is synced: Verify sync status in header
2. Wait for confirmations: Check transaction confirmations
3. Wait for the next refresh: Balances reload automatically every 10 seconds (or reload the page)
4. Check RPC connection: Verify wallet connectivity

### No Transactions Showing
**Problem**: Transaction history is empty

**Solutions:**
1. **Watch-only wallet**: Import xpub with correct gap limit
2. **New wallet**: No transactions yet
3. **High address index**: Increase gap limit and rescan
4. **Sync incomplete**: Wait for wallet to finish syncing

### Ticket Counts Incorrect
**Problem**: Ticket statistics don't match expectations

**Solutions:**
1. **Watch-only wallet**: Connect via RPC for ticket data
2. **Sync in progress**: Wait for sync completion
3. **Expired tickets**: Check "expired" count
4. **Recently purchased**: Check "immature" count

### Sync Progress Stuck
**Problem**: Rescan progress bar frozen

**Solutions:**
1. Check dcrwallet logs: `docker compose logs -f dcrwallet`
2. Verify dcrwallet is running: `docker compose ps`
3. Check for stale logs: Backend auto-detects stale progress
4. Restart if necessary: `docker compose restart dcrwallet`

### Cards Not Appearing After Rescan
**Problem**: Dashboard cards don't show after rescan completes

**Solutions:**
1. Refresh page: Force browser refresh (Ctrl+Shift+R)
2. Check for errors: Open browser console (F12)
3. Verify completion: Check logs for 100% progress
4. Reload the dashboard to re-fetch wallet data

---

## Understanding Balance Types

### Cumulative Total
**What it is**: Sum of all balances across all accounts

**Includes:**
- Spendable
- Locked by tickets
- Immature rewards

**Excludes:**
- Unconfirmed transactions (until confirmed)

**Use case**: Total wallet value

---

### Total Spendable
**What it is**: Immediately available funds

**Can be used for:**
- Sending transactions
- Purchasing tickets
- Any wallet operation

**Excludes:**
- Locked funds
- Immature rewards
- Unconfirmed transactions

**Use case**: Available balance for spending

---

### Total Locked by Tickets
**What it is**: Funds committed to active tickets

**Locked in:**
- Immature tickets
- Live tickets in pool
- Recently voted tickets (awaiting maturity)

**Released when:**
- Ticket votes (rewards mature after 256 blocks)
- Ticket expires (must revoke first)
- Ticket is revoked

**Use case**: Staking commitment tracking

---

### Immature Coinbase Rewards
**What it is**: Block mining rewards awaiting maturity

**Requirements:**
- 256 confirmations (~21 hours)
- Mined by your wallet's addresses

**After maturity:**
- Moves to spendable balance
- Can be used immediately

**Use case**: Mining reward tracking

---

### Immature Stake Generation
**What it is**: Voting/staking rewards awaiting maturity

**Requirements:**
- 256 confirmations (~21 hours)
- Earned from ticket votes

**After maturity:**
- Moves to spendable balance
- Includes original ticket price + reward

**Use case**: Staking reward tracking

---

### Unconfirmed
**What it is**: Pending transactions

**Status:**
- Not yet in a block
- Or recently in mempool
- Awaiting confirmation

**Becomes spendable:**
- After first confirmation
- May require multiple confirmations for large amounts

**Use case**: Pending transaction tracking

---

### Voting Authority
**What it is**: Tickets you control voting rights for

**Scenarios:**
- Solo staking: Matches your tickets
- VSP staking: May include delegated tickets
- Split tickets: May own partial ticket

**Use case**: Governance participation tracking

---

## Security Considerations

### RPC Credentials
- Stored server side only
- Never sent to the browser
- Use strong passwords
- Rotate regularly

### Watch-Only Wallets
- Safe for monitoring
- Cannot spend funds
- Limited information access
- Xpub can be shared (carefully)

### Wallet Access
- Secure RPC endpoint
- Use TLS for remote access
- Firewall RPC ports
- Monitor access logs

---

## Next Steps

After setting up your wallet dashboard:

1. **[Import Xpub](../guides/wallet-operations.md)** - Add watch-only addresses
2. **[Start Staking](staking-guide.md)** - Purchase tickets and earn rewards
3. **[Backup Wallet](../guides/backup-restore.md)** - Protect your funds

---

**Questions?** Check the [Troubleshooting Guide](../guides/troubleshooting.md).

