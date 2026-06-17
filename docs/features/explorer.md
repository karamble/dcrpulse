# Block Explorer

The **Block Explorer** lets you browse the Decred blockchain directly from your dashboard. Every result is served by your own local dcrd node over its JSON-RPC interface, so you are not relying on a third-party explorer and your lookups stay private.

## Overview

The explorer turns the dashboard into a self-hosted block explorer. You can search by block height, block hash, transaction id, or address, drill into individual blocks and transactions, inspect inputs and outputs, watch the live mempool, and verify dcrtime timestamp proofs against the chain.

**Access**: Click the **"Explorer"** link in the header navigation.

**Data source**: All data comes from your local dcrd node. dcrd does not maintain an address index by default, so address pages are intentionally limited (see [Address View](#address-view) below). Everything else - blocks, transactions, mempool - is fully served from the node.

---

## Universal Search

A single search box on the explorer landing page accepts four kinds of input and routes you to the right page automatically.

### What You Can Search

- **Block height** - a plain number (e.g., `1000000`)
- **Block hash** - a 64-character hex string
- **Transaction id** - a 64-character hex string
- **Address** - a Decred address starting with `D`

### How It Works

As you type, the search box detects the likely input type and updates its placeholder hint:
- Digits only -> "Block height detected..."
- 64 hex characters -> "Transaction hash detected..."
- A `D...` string -> "Address detected..."

When you submit, the query is sent to the backend, which asks dcrd to resolve it. Because a 64-character hex value can be either a block hash or a transaction id, the backend tries to identify what it actually is and returns the resolved type. The frontend then navigates to:
- A **block detail** page for a height or block hash
- A **transaction detail** page for a transaction id
- An **address view** for an address

If nothing matches, the search box shows an inline "Not found" message.

**API route**: `GET /api/explorer/search?q=<query>`

---

## Explorer Landing Page

The landing page is the explorer home. It shows:

### Search Bar
The universal search box described above.

### Verify Timestamp Card
A shortcut to the [Verify Timestamp](#verify-timestamp) tool, which checks a file or digest against dcrtime and confirms its anchor on the Decred chain.

### Recent Blocks
A paginated table of the most recent blocks, newest first. Each row shows:
- **Height** - block number (click the row to open the block)
- **Hash** - truncated block hash
- **Time** - relative time since the block was mined (e.g., "2m ago")
- **Txs** - number of transactions in the block
- **Size** - block size (B / KB / MB)

The table auto-refreshes every **30 seconds** while you are on the first page. Pagination controls let you page back through older blocks; the total block count (chain height) is shown above the table. Clicking any row opens that block's detail page.

**API route**: `GET /api/explorer/blocks/recent?page=<n>&pageSize=<n>` (page size defaults to 10, capped at 100; the landing page requests 20 per page)

---

## Block Detail View

Opened by clicking a block, searching a height/hash, or following a block link. The URL is `/explorer/block/<height>`.

### Header
- **Block number** with the block's timestamp (full date shown)
- **Previous** / **Next** navigation buttons to step through adjacent blocks (Previous is disabled at the genesis block; Next is disabled on the chain tip, which has no next block yet)

### Block Information
A grid of block fields, each with a copy button where relevant:
- **Block Hash**
- **Previous Block** (clickable - jumps to the prior block)
- **Merkle Root** - regular-transaction tree root
- **Stake Root** - stake-transaction tree root
- **Confirmations**
- **Difficulty**
- **Size**
- **Transactions** - count
- **Version** and **Stake Version**
- **Vote Bits** - shown in hex
- **Nonce**

A **View JSON** toggle replaces the grid with the raw block object returned by dcrd, for advanced inspection.

### Transactions
Because Decred has two transaction trees (regular and stake) plus special transaction types, the block's transactions are grouped into labeled sections, each with its own icon and color. Sections only appear when they contain transactions:

- **Treasury** - treasury spends (TSpend) and treasury additions (TBase)
- **Coinbase** - the block reward transaction
- **Votes** - ticket votes (SSGen)
- **Tickets** - ticket purchases (SSTx)
- **Regular** - ordinary value transfers
- **CoinJoin** - privacy mixing transactions
- **Revocations** - ticket revocations (SSRtx)

Each entry shows the transaction id (with a copy button), its size in bytes, and its total output value in DCR. Click any entry to open its transaction detail page.

**API routes**:
- By height: `GET /api/explorer/blocks/{height}`
- By hash: `GET /api/explorer/blocks/hash/{hash}`

---

## Transaction Detail View

Opened by clicking any transaction or following a transaction link. The URL is `/explorer/tx/<txid>`.

### Header
- A type icon and a **type badge** naming the Decred transaction class:
  - **Regular Transaction**
  - **Ticket Purchase (SSTx)**
  - **Vote (SSGen)**
  - **Revocation (SSRtx)**
  - **Coinbase**
  - **Treasury Spend (TSpend)**
  - **Treasury Addition (TBase)**
- The transaction's timestamp (full date)
- A **View JSON** toggle for the raw transaction object

### Transaction ID
The full txid with a copy button.

### Transaction Information
A grid of fields:
- **Block** - clickable block number, or **Mempool** if not yet mined
- **Confirmations**
- **Block Hash** (when mined)
- **Size** in bytes
- **Total Output Value** in DCR
- **Fee** in DCR
- **Fee Rate** in DCR/KB (computed from fee and size)
- **Version**
- **Lock Time**
- **Expiry**

### Treasury Spend Details (TSpend only)
For treasury spends, an extra section shows:
- **Politeia Proposal Key** (when present)
- **Expiry Height**
- **Recipients** count
- **Total Payout**
- **Transaction Version** (flagged as Treasury for version 3)

### Treasury Spend Approval (TSpend only)
Treasury spends are approved by stakeholder voting, so the page also renders a voting panel:
- A status badge - **Ongoing Vote** or **Voting Complete**, plus an approval classification (Fast Approval / Approval / Rejected) once voting finishes
- An **approval rate** bar and percentage
- **Quorum** status (votes cast versus required)
- A breakdown of **Yes** / **No** votes, **Eligible Votes**, **Votes Cast** with turnout, voting start/end times, and the voting period block range (start and end blocks are clickable)

Counting votes requires scanning the blocks in the voting window. While that scan runs, the panel shows a live **"Counting Votes..."** progress bar (current block, percent complete, estimated time remaining, and running Yes/No tallies), polled every 2 seconds until counting finishes.

### Inputs and Outputs
A detailed inputs/outputs list:

**Inputs** show, per input:
- The previous transaction id (clickable) and the output index and tree it spends
- The source **address** (clickable, when known)
- The **amount in** DCR and the block it came from
- Special cases are labeled **Coinbase** or **Stakebase** instead of a prevout
- An expandable **Script Signature**
- A running **Total** of input value

**Outputs** show, per output:
- The output index and its **script type** (color-coded; stake, pubkey, scripthash, nulldata, etc.)
- A **Spent** badge and a link to the spending transaction, when the output has been spent
- The destination **address(es)**, each clickable; `nulldata` outputs are labeled as OP_RETURN data, and non-standard scripts are noted
- The **value** in DCR and the output version
- Expandable **Script Details** (ASM and hex)
- A running **Total** of output value

A **Transaction Fee** summary at the bottom shows inputs minus outputs.

### Raw Transaction
When available, an expandable **Show Hex** panel displays the full raw transaction hex with a copy button.

**API routes**:
- Transaction: `GET /api/explorer/transactions/{txhash}`
- Vote-counting progress (TSpend): `GET /api/treasury/votes/{txhash}/progress`

---

## Address View

Opened by clicking any address or searching one. The URL is `/explorer/address/<address>`.

Because dcrd runs without an address index by default, the dashboard does not keep a local database of every transaction per address. The address view is therefore deliberately scoped to what the node can answer cheaply.

### Address Header
- The full address with a copy button
- A **Bookmark** button (star icon) - see [Address Bookmarks](#address-bookmarks)

### Address Status
- **Validity** - whether the address is a well-formed Decred address
- **On-Chain Status** - whether the address has been seen on the blockchain ("Used on blockchain" vs "Never used")

### Tickets Owned
If the address is associated with staking tickets, they are listed here, each linking to its ticket transaction. If the address exists but owns no tickets, a "No tickets found" note is shown.

### Limited Address Information Notice
A disclaimer explains the current capabilities (address validation, existence check, and ticket ownership lookup) and links out to the official Decred block explorer (`dcrdata.decred.org/address/<address>`) for full transaction history and balances.

**API route**: `GET /api/explorer/address/{address}`

### Address Bookmarks
You can bookmark any address with a name and notes for quick personal reference (for example, "My Mining Wallet"). Bookmarks are stored locally in your browser (`localStorage`), are not shared with the server, and are per-browser. The bookmark editor supports adding, updating, and deleting a bookmark, with a delete confirmation step.

---

## Mempool View

Opened from a mempool transaction link or by navigating to `/explorer/mempool`. It shows transactions that have been broadcast but not yet mined into a block, as reported by your node.

### Header
The page title with a count of pending transactions.

### Mempool Summary
Three summary cards:
- **Pending Transactions** - total count
- **Total Size** - combined mempool size (B / KB / MB)
- **Transaction Breakdown** - counts per type (Treasury, Tickets, Votes, Revocations, CoinJoin, Regular), showing only the types currently present

### Transactions
The pending transactions, grouped by type using the same labeled sections and icons as the block detail view (Treasury, Tickets, Votes, Revocations, CoinJoin, Regular). Each entry shows the txid (with copy button), size in bytes, and total value in DCR, and links to its transaction detail page. When the mempool is empty, an "Mempool is empty" message is shown instead.

The mempool view auto-refreshes every **30 seconds**.

**API route**: `GET /api/explorer/mempool`

---

## Verify Timestamp

The explorer includes a standalone dcrtime proof checker, reached from the **Verify Timestamp** card on the landing page (URL `/explorer/verify-timestamp`). It lets you check a file or digest against dcrtime and confirm that its anchor is recorded on the Decred chain. The verification is validated through this dashboard's own dcrd node rather than a remote service. It reuses the same verify flow as the dashboard's Timestamp tab, which also lets you create new timestamps.

---

## API Reference

All explorer endpoints are read-only `GET` requests under `/api/explorer` (plus the treasury vote-progress endpoint), backed by your local dcrd node:

| Route | Purpose |
| --- | --- |
| `GET /api/explorer/search?q=<query>` | Universal search (height, hash, txid, address) |
| `GET /api/explorer/blocks/recent?page=&pageSize=` | Paginated recent blocks |
| `GET /api/explorer/blocks/{height}` | Block detail by height |
| `GET /api/explorer/blocks/hash/{hash}` | Block detail by hash |
| `GET /api/explorer/transactions/{txhash}` | Transaction detail |
| `GET /api/explorer/address/{address}` | Address validation, existence, and tickets |
| `GET /api/explorer/mempool` | Current mempool transactions |
| `GET /api/treasury/votes/{txhash}/progress` | TSpend vote-counting progress |

---

## Tips & Best Practices

### Searching
1. Paste a full block hash or txid - both are 64 hex characters and the backend resolves which it is
2. Use a plain number to jump straight to a block by height
3. Address searches give validation and ticket data; use the linked official explorer for full history

### Navigating
1. Use the Previous/Next buttons to walk the chain block by block
2. Click any txid, address, or previous-output link to follow the chain of transactions
3. Use the copy buttons to grab hashes and addresses cleanly

### Treasury Spends
1. Open a TSpend transaction to see its approval status and vote breakdown
2. Let the vote-counting progress bar finish for the final tally on recent or ongoing votes

---

## Troubleshooting

### Block or Transaction Not Found
**Problem**: The explorer shows a "Not Found" page

**Solutions:**
1. Verify the height, hash, or txid is correct and complete
2. Make sure dcrd is fully synced - very recent blocks or transactions may not be on your node yet
3. Confirm dcrd is running and reachable from the dashboard

### Address Shows Limited Information
**Problem**: An address page does not show full balance or transaction history

**Explanation**: This is expected. dcrd runs without an address index, so the dashboard can only validate the address, report whether it has been used, and list owned tickets. Use the linked official Decred explorer for complete address history.

### Recent Blocks Not Updating
**Problem**: The recent-blocks table looks stale

**Solutions:**
1. The table only auto-refreshes on the first page - return to page 1
2. Confirm dcrd is synced and producing/relaying new blocks
3. Refresh the page

### Vote Count Stuck
**Problem**: A treasury spend's vote count does not finish

**Solutions:**
1. Vote counting scans every block in the voting window and can take time on a busy node
2. Ensure dcrd is responsive and fully synced
3. Reopen the transaction to restart the progress scan

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - Monitor your wallet's balances and transactions
- **[Staking Guide](staking-guide.md)** - Tickets, votes, and revocations explained
