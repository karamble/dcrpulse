# Governance

The **Governance** area lets you take part in Decred's on-chain and off-chain decision making directly from your wallet: voting on consensus rule changes, setting treasury-spend policies, and reading and voting on Politeia proposals.

## Overview

Decred is governed by its stakeholders. Anyone who buys tickets gains the right to vote on three separate decision tracks, and dcrpulse exposes all three from one place:

- **Consensus voting** - vote on proposed changes to Decred's consensus rules (agendas) during an active voting deployment.
- **Treasury** - set how your wallet votes on Treasury Spend (TSpend) transactions that pay out of the on-chain treasury.
- **Politeia proposals** - browse, read, and vote on off-chain funding and policy proposals from proposals.decred.org.

dcrpulse presents these in two separate places:

**Wallet governance area** (`/wallet/governance`)
- Reached by opening the wallet and selecting **Governance** in the wallet navigation.
- Three sub-tabs: **Consensus**, **Treasury**, **Proposals**, plus a proposal detail view.
- This is where you cast or change votes. All vote and policy changes require your wallet passphrase.

**Treasury monitor** (`/treasury`)
- A standalone, node-level page for watching the treasury: current balance, balance-over-time chart, payment history, active treasury votes, and a historical TSpend scanner.
- Read-only with respect to voting - it shows what the network is doing but does not cast votes itself.

---

## Wallet Governance Area

**Access**: Open your wallet, then select **Governance**. The page opens on the **Consensus** tab by default.

The three tabs share a common pattern:

- Lists refresh automatically (consensus and treasury policy tabs poll every 30 seconds).
- Any action that changes a vote or policy opens a **passphrase modal** - the wallet is unlocked briefly to write the change, then re-locked.
- Errors from the daemons (dcrd / dcrwallet) are surfaced as-is so you can see exactly what went wrong.

### 1. Consensus Tab

The **Consensus** tab shows the consensus agendas that are part of the current voting deployment, joined with the choice your wallet is currently set to cast.

**Data source**: Agendas come from dcrd's `getvoteinfo`, so the list reflects the live state of the network. Your wallet's per-agenda choice is read back from dcrwallet.

**When there are no agendas**

Decred only proposes new consensus rule changes when a deployment is scheduled. Outside of those windows the tab shows:

```
No active consensus agendas right now. Decred only proposes new
rule changes when there's a deployment scheduled; check back later.
```

**What each agenda shows**

For every agenda in the deployment:

- **Agenda ID** - the short identifier (e.g. the DCP being voted on).
- **Description** - a plain-language summary of the rule change.
- **Status badge** - the agenda's stage in the deployment:
  - **defined** - declared but not yet open for voting
  - **started** / **active** - voting is open
  - **lockedin** - the threshold was met and the change is queued to activate
  - **failed** - the agenda did not pass

**Setting your vote choice**

Each agenda lists its available choices (for example **abstain**, **yes**, **no**) as selectable cards. The choice your wallet is currently set to is marked **current choice**.

1. Click a choice card.
2. A **Confirm Agenda Vote Choice** passphrase modal appears.
3. Enter your wallet passphrase and confirm.
4. The choice is written to dcrwallet and the list refreshes to show the new current choice.

Your tickets then vote with this choice automatically as they are called to vote. You can change the choice at any time while the agenda is still open; the most recent choice is what your tickets will cast.

**API**: `GET /api/wallet/governance/agendas`, `POST /api/wallet/governance/agendas/set`

---

### 2. Treasury Tab

The **Treasury** tab controls how your wallet votes on **Treasury Spend (TSpend)** transactions - payouts from Decred's on-chain treasury. Unlike Politeia proposals, TSpend voting is handled by your wallet automatically according to the policies you set here; there is no per-event "cast vote" button. When a TSpend appears, your tickets vote according to the matching policy.

There are two levels of policy, shown as two cards.

#### Politeia Keys

A **blanket policy** that applies to every future TSpend signed by a given Politeia treasury key.

- Each sanctioned treasury key is listed by its public key.
- For each key you can set **Approve** (yes), **Reject** (no), or **Abstain**.
- The default, when no policy is set, is **abstain**.
- These keys are the standing signers, so a key-level policy is a standing instruction for all spends they authorize.

Per-TSpend overrides (below) take precedence over the key policy for individual spends.

#### TSpend Overrides

A **per-hash policy** that overrides the matching key policy for one specific TSpend.

- Lists TSpends the wallet currently knows about. The wallet only knows about a TSpend it has actually observed in the mempool or in a mined block, so this list can be empty:

  ```
  No TSpends currently tracked. The wallet only knows about TSpends
  it has observed in the mempool or in mined blocks.
  ```

- Each tracked TSpend shows its transaction hash (linked to the explorer), and where available its **requested amount** in DCR and **expiry block**.
- Set **Approve** / **Reject** / **Abstain** per TSpend to override the key-level default for just that spend.

**Setting a policy**

1. Click **Approve**, **Reject**, or **Abstain** on a key card or a TSpend card.
2. A **Confirm Treasury Policy Change** passphrase modal appears.
3. Enter your wallet passphrase and confirm.
4. The policy is saved to dcrwallet and the cards refresh.

> Note: the key-policy list and the TSpend list are fetched independently. A transient outage of the wallet's voting service can hide the TSpend overrides while still showing the sanctioned key cards above.

**API**:
- `GET /api/wallet/governance/treasury/keys`, `POST /api/wallet/governance/treasury/keys/set`
- `GET /api/wallet/governance/treasury/tspends`, `POST /api/wallet/governance/treasury/tspends/set`

---

### 3. Proposals Tab

The **Proposals** tab is a reader and voting client for **Politeia**, Decred's off-chain proposal system at proposals.decred.org.

#### Politeia must be enabled

Politeia data is fetched from an external server, so it is gated behind a privacy toggle. When the toggle is off, the tab shows:

```
Politeia is disabled in Settings

Enable the Politeia external-request toggle to fetch off-chain
proposals from proposals.decred.org.
```

with a link to **Privacy & Security settings**. Enable the Politeia external-request toggle there to use this tab.

#### Browsing proposals

When enabled, the tab lists proposals with:

- **Title** and **author**.
- **Token** - the proposal's Politeia identifier (shown truncated).
- **Vote status badge**:
  - **authorized** / **unauthorized** - pre-vote states
  - **started** - voting is open
  - **approved** - passed
  - **rejected** - did not pass
  - **abandoned** - withdrawn
- **Blocks left** - for proposals in active voting.
- A **vote results bar** (yes / no / abstain split, cast vs. eligible tickets, and whether quorum is reached) for proposals where voting has begun.
- **you voted: X** - shown when your wallet has already voted on a proposal (see the local cache below).

**Filtering**

A status filter narrows the list. The buckets are **all**, **voting**, **pre-vote**, **finished**, and **abandoned**. The tab opens on **voting** so active votes are front and center.

**Refreshing**

The proposals list is cached on the backend so the external server is not hit on every page load. A **Refresh** button forces a re-fetch, subject to a cooldown:

- The button shows **updated X ago** and, while cooling down, **Refresh in Xm** with a live countdown.
- If you refresh during the cooldown the server returns a 429 and the countdown re-syncs to match the server.

#### Proposal detail view

Clicking a proposal opens its detail page (`/wallet/governance/proposals/:token`), which is also cached per proposal with its own refresh + cooldown. It shows:

- **Title, author, and full token**, plus a **View on Politeia** link to the original record.
- **Status, end block, blocks left, and eligible ticket count.**
- The **vote results bar** when voting is active.
- **Description** - the proposal body, rendered from Politeia's markdown. Embedded images are not loaded; the page links out to Politeia for the original visuals and formatting.
- **Cast your vote** section (only while the proposal is in **voting**) - opens the vote modal.
- **Discussion** - the Politeia comment thread, rendered as a reply tree with up/down vote counts and a net score per comment. Deleted comments are shown as removed rather than dropped.

#### Casting a Politeia vote

Voting is intentionally gated so the heavy work only runs when you ask for it.

1. On a proposal that is in **voting**, click **Vote** (or **View vote** if you have already voted) in the **Cast your vote** section.
2. The **vote modal** opens and computes your eligibility on demand. This is where the wallet fetches the eligible-ticket snapshot from Politeia and works out:
   - how many of **your** tickets are eligible to vote on this proposal,
   - the available vote options, and
   - whether this wallet has **already voted**.
   This snapshot work runs only on opening the modal, never on a plain detail-page view.
3. The modal then shows one of:
   - **You voted "X"** - your wallet already voted; each ticket votes once, so it cannot vote again from this wallet.
   - **None of your tickets are eligible** - you hold no tickets eligible for this vote (the total eligible count is shown for context).
   - A **vote form** - when you have eligible tickets that have not voted.
4. In the vote form, pick a choice (e.g. **yes** / **no** / **abstain**), enter your **wallet passphrase**, and click **Sign & cast**.
5. The ballot is signed and broadcast. The modal reports how many ticket votes were **cast**, how many were **skipped**, and lists any per-ticket errors.

#### The local "you voted" cache

After a successful cast - and also whenever the eligibility check discovers your wallet has already voted - dcrpulse records your choice locally in the wallet's config (keyed per network and per wallet). This local "you voted X" cache is what powers the **you voted: X** badges in the list and the **You voted "X"** notices on the detail page and in the modal, without re-deriving it from the chain on every view. The cached choice is layered onto the proposal data when it is served, so it survives restarts and refreshes.

**API**:
- `GET /api/wallet/governance/proposals` (list), `POST /api/wallet/governance/proposals/refresh`
- `GET /api/wallet/governance/proposals/{token}` (detail), `POST /api/wallet/governance/proposals/{token}/refresh`
- `POST /api/wallet/governance/proposals/{token}/vote-eligibility` (compute eligibility on modal open)
- `POST /api/wallet/governance/proposals/cast-vote` (sign + cast ballot)

When Politeia is disabled, these endpoints return **503 Service Unavailable**.

---

## Treasury Monitor

**Access**: Open the **Treasury** page (`/treasury`). This is a node-level monitor, separate from the wallet governance area, and does not require an unlocked wallet.

It reads treasury data straight from dcrd and tracks the historical record locally in your browser.

### Treasury Statistics and Charts

The top of the page shows treasury statistics and a balance-over-time chart, backed by:

- **Current balance** - the live treasury balance from dcrd (`gettreasurybalance`).
- **Balance history** - a balance-over-time series sampled at a coarse (roughly monthly) cadence and cached on the backend, used to draw the chart.

### Active Treasury Votes

A live view of TSpends currently in the voting window (scanned from the mempool). For each active TSpend it shows:

- The **requested amount** in DCR and the **payee** (or transaction hash).
- **Blocks remaining** until the vote ends.
- The current **yes / no tally** and a yes/no split bar.
- Whether it is **passing** - a TSpend requires **60% yes** of the cast yes/no votes to pass (the 3/5 required-approval multiplier).

When nothing is in voting it reads **No treasury votes in progress.**

This panel shows the network-wide tally. To control how *your* wallet votes on these TSpends, use the **Treasury** tab in the wallet governance area.

### Historical TSpend Scanner

Because dcrd does not serve the full historical list of TSpends, dcrpulse can scan the blockchain for them and store the results in your browser's local storage.

- **Scan Historical TSpends** button starts a scan from where it last left off (or from the treasury activation height, block 552,448, on a first run).
- The scan strides by the treasury vote interval, since a block can only contain a TSpend on that cadence.
- A **progress bar** shows current height, total height, and TSpends found while the scan runs.
- Newly found TSpends are saved to local storage as they are discovered, and a final sync at the end catches anything missed if the browser was closed mid-scan.
- The **last scan** date, height, and total found are shown after completion.

> Because the historical record lives in browser local storage, it is per-browser. Other clients (or a different browser) will not see a scan you ran locally until they scan themselves.

**API**:
- `GET /api/treasury/info` - current balance + active (mempool) TSpends
- `GET /api/treasury/balance-history` - balance-over-time series
- `POST /api/treasury/scan-history` - start a historical scan (rate-limited)
- `GET /api/treasury/scan-progress` - scan progress
- `GET /api/treasury/scan-results` - results of the last completed scan
- `GET /api/treasury/mempool` - active TSpends in the mempool
- `GET /api/treasury/votes/{txhash}/progress` - vote-counting progress for one TSpend

---

## How Voting Is Gated and Cast

Each track casts votes differently, and dcrpulse follows the same model the daemons use.

**Consensus agendas**
- You set a **choice** per agenda; your tickets cast that choice automatically as they are selected to vote.
- Setting a choice requires the wallet passphrase. There is no separate "submit ballot" step - the choice itself is the instruction.

**Treasury spends (TSpends)**
- You set a **policy** (approve / reject / abstain) per treasury key and optionally per individual TSpend.
- Your wallet then votes on TSpends automatically according to those policies as it encounters them; there is no manual per-TSpend cast.
- Setting a policy requires the wallet passphrase.

**Politeia proposals**
- Voting is **on demand**: you open the vote modal, which computes eligibility (and fetches the ticket snapshot) only at that point.
- You then pick a choice and **sign and cast a ballot** with your eligible tickets in a single action, authorized by the wallet passphrase.
- Each ticket votes once per proposal; a second attempt is blocked and shown as already voted.

In all three cases the wallet is unlocked only for the moment of the change and the passphrase is never exposed to the frontend.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - balances, accounts, and voting authority
- **[Staking Guide](staking-guide.md)** - buying tickets, which is what grants voting rights
- **[Wallet Setup](../guides/wallet-operations.md)** - initial wallet configuration

---

## Tips & Best Practices

### Consensus Voting
1. Check the **Consensus** tab when a deployment is active; agendas only appear during voting windows.
2. Set your choice early so your tickets vote it as they are called.
3. You can change a choice any time while the agenda is still open.

### Treasury Policies
1. Set a key-level policy as your standing default, then add per-TSpend overrides only where you want to deviate.
2. Remember the TSpend list only includes spends the wallet has actually seen; an empty list is normal.
3. Use the **Treasury** monitor page to watch live tallies, and the wallet **Treasury** tab to control your own vote.

### Politeia Proposals
1. Enable the Politeia toggle in **Privacy & Security** settings first.
2. Use the status filter (defaulting to **voting**) to find proposals you can still vote on.
3. Opening the vote modal does real work (it fetches a ticket snapshot), so open it when you intend to vote.
4. The **you voted: X** badge reflects your local record of past votes.

---

## Troubleshooting

### Proposals Tab Shows "Politeia is disabled"
**Problem**: The proposals tab will not load any proposals.

**Solution**: Enable the Politeia external-request toggle in **Privacy & Security** settings. The proposal endpoints return 503 while it is off.

### No Consensus Agendas Listed
**Problem**: The Consensus tab is empty.

**Solution**: This is expected outside of an active voting deployment. Decred only lists agendas while a rule-change vote is scheduled.

### Refresh Button Is Disabled / Counting Down
**Problem**: You cannot refresh proposals immediately.

**Solution**: The list and detail views are cached with a refresh cooldown. Wait for the countdown (**Refresh in Xm**) to reach zero, or rely on the cached data, which is anchored to its last successful fetch.

### "None of your tickets are eligible"
**Problem**: The vote modal will not let you vote.

**Solutions**:
1. You must hold tickets that were live at the proposal's eligibility snapshot.
2. If you have already voted, the modal shows **You voted "X"** instead - each ticket votes once.
3. Watch-only wallets cannot vote, as voting requires signing with the wallet's keys.

### TSpend Overrides List Is Empty
**Problem**: The TSpend Overrides card shows no spends.

**Solution**: The wallet only tracks TSpends it has observed in the mempool or in mined blocks. With nothing currently active or recently seen, the list is empty. Use key-level policies for standing instructions.
