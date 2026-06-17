# Staking Guide

Complete guide to Decred Proof-of-Stake (PoS) staking using the Decred Pulse dashboard. Learn how to buy tickets, run the autobuyer, choose a Voting Service Provider (VSP), and track your tickets and rewards.

**Access**: Open `http://localhost:8080`, click the **"Wallet"** button in the header, then **Staking** in the wallet sidebar. The Staking area has five tabs: **Purchase**, **Auto Buyer**, **Ticket Status**, **History**, and **Statistics**.

For consensus, treasury, and Politeia voting, see the [Governance Guide](governance.md).

## What is Decred Staking?

**Decred staking** is the process of time-locking DCR to purchase **tickets** that participate in network governance and consensus. In return, you earn staking rewards when your tickets are called to vote.

### Key Concepts

**Ticket**: A special transaction that locks DCR for voting
**Ticket Price**: The cost to purchase a ticket (changes every 144 blocks)
**Ticket Pool**: All active tickets waiting to be called to vote
**Voting**: When your ticket is selected to validate a block
**VSP**: Voting Service Provider (optional, ensures ticket votes even if offline)

---

## Ticket Lifecycle

### 1. Purchase (Mempool)
- Buy ticket at current ticket price
- Transaction enters mempool
- Awaiting confirmation
- **Duration**: ~5 minutes (1 block)

### 2. Immature
- Ticket confirmed on blockchain
- Not yet eligible to vote
- Gaining maturity
- **Duration**: ~21 hours (256 blocks)

### 3. Live/Unspent
- Ticket enters the pool
- Eligible to be called for voting
- **Duration**: ~28 days average (142 days max)
- **Probability**: Random selection per block

### 4. Voting
- Ticket is selected to vote
- Validates previous block
- Earns voting reward
- **Reward**: ~0.8% of ticket price

### 5. Voted
- Vote confirmed
- Reward + ticket price returned
- Rewards are immature for 256 blocks
- **Total Return**: Ticket price + ~0.8% reward

### Alternative Outcomes

**Expired**
- Not selected within 142 days (~40,960 blocks)
- Automatically revoked by the network (DCP-0009)
- No reward earned

**Revoked**
- Missed or expired ticket, revoked automatically since DCP-0009
- Original ticket price returned (minus small fee)
- No reward earned

---

## Understanding the Dashboard

The **Purchase** tab shows two read-only cards alongside the purchase form: **Ticket Pool & Difficulty** (network-wide) and **My Tickets** (your own). The other tabs are covered in [Staking Operations](#staking-operations) below.

### Ticket Pool & Difficulty Card

Monitor the global Decred ticket pool:

#### Pool Size
**Current**: Number of live tickets in the pool
**Target**: ~40,960 tickets
**Significance**: Higher pool = more decentralization

**What you see**:
```
Pool Size: 41,095 tickets
```

**What it means**:
- Network has 41,095 active tickets
- Your ticket competes with these for voting
- Healthy network participation

---

#### Current Price
**What it is**: The current ticket price (stake difficulty) in DCR
**Updates**: Every 144 blocks (~12 hours)
**Algorithm**: Adjusts to maintain ~40,960 pool size

**What you see**:
```
Current Price: 293.08 DCR
```

**What it means**:
- A ticket purchased now costs 293.08 DCR
- This is the price you'll pay now
- Changes at next difficulty adjustment

---

#### Expected Next
**What it is**: Estimated next ticket price
**Timing**: Applies at the next difficulty window
**Calculation**: Based on recent demand

**What you see**:
```
Expected Next: 293.08 DCR
```
The card also shows the change versus the current price (up, down, or unchanged).

**What it means**:
- Expected price for next window
- Same = stable demand
- Higher = increasing demand
- Lower = decreasing demand

---

#### Expected Price Range
**What it is**: Price prediction range
**Based on**: Recent ticket purchases
**Algorithm**: `estimatestakediff` RPC

**What you see**:
```
Min: 291.54 DCR
Expected: 292.20 DCR
Max: 294.59 DCR
```

**What it means**:
- **Min**: Lowest possible next price
- **Expected**: Most likely next price
- **Max**: Highest possible next price

**Usage**: Plan your ticket purchases

---

#### Mempool Tickets
**What it is**: Network-wide pending ticket purchases
**Status**: Awaiting confirmation
**Accuracy**: Calculated using stake difficulty

**What you see**:
```
All Mempool Tickets: 15
```

**What it means**:
- 15 tickets being purchased network-wide
- Will confirm in next block
- Will become immature tickets

**Technical Note**: Count calculated by:
```
Total Stake Submission Outputs / Current Stake Difficulty
```
This accurately counts tickets even in coinjoin transactions.

---

### My Tickets Card

Track your personal ticket statistics:

#### Mempool
**Your pending tickets**
- Just purchased
- Awaiting first confirmation
- Not yet immature

**Typical count**: 0-1 (unless batch purchasing)

**What to expect**:
- Moves to immature after ~5 minutes
- If stuck, check transaction fee

---

#### Immature
**Your maturing tickets**
- Confirmed but not yet live
- Requires 256 confirmations
- ~21 hours until live

**Typical count**: 0-5 (depending on purchase frequency)

**What to expect**:
- Becomes live after 256 blocks
- Cannot vote yet
- Funds are locked

---

#### Live/Unspent
**Your active tickets**
- In the ticket pool
- Eligible to vote on every block
- Average lifetime: ~28 days

**Typical count**: Varies by staking strategy

**What to expect**:
- Random selection for voting
- Average 28-day wait
- Max 142-day expiry

**Probability per block**:
```
Chance = 5 votes needed / ~40,960 pool size
      ~ 0.012% per block
      ~ 28 days average
```

---

#### Voted
**Your successful votes**
- Tickets that have voted
- Rewards earned
- Historical count

**Typical count**: Accumulates over time

**What to expect**:
- Rewards + ticket price returned
- Rewards immature for 256 blocks
- Then available as spendable

---

#### Revoked
**Your revoked tickets**
- Missed or expired tickets that were revoked
- Funds recovered (minus a small fee)

**Typical count**: Low (most tickets vote)

**What to expect**:
- A small fraction of tickets miss or expire
- Since DCP-0009 (auto-revocation, 2022), revocations are created automatically by the network; no manual action is needed

---

#### Expired
**Your expired unspent tickets**
- Not selected within 142 days

**Typical count**: Should be 0 on current mainnet

**What to expect**:
- Under the auto-revocation rules active since DCP-0009, expired tickets are revoked automatically and their funds returned; you do not revoke them by hand

---

#### Total Subsidy
**Cumulative voting rewards**
- All-time earnings from voting
- Excludes original ticket prices
- Only the ~0.8% rewards

**Typical value**: Grows with each vote

**Calculation**:
```
Per Vote ~ Ticket Price x 0.008
```

**Example**:
- Ticket Price: 293 DCR
- Reward: ~2.34 DCR per vote
- 10 votes: ~23.4 DCR total subsidy

---

## Staking Economics

### Costs

**Ticket Price**
- Current: Check the "Current Price" card on the Purchase tab
- Historical Range: 30-600 DCR
- Current Range: 200-350 DCR (typical)

**VSP Fee** (if using VSP)
- Typical: 0.5-5% of reward
- One-time per ticket
- Deducted from voting reward

**Transaction Fees**
- Purchase: ~0.01 DCR
- Revocation: ~0.01 DCR
- Voting: Paid by VSP or your wallet

### Returns

**Voting Reward**
- Approximate: ~0.8% of ticket price per vote
- Annual ROI: ~6-7% (varies)
- Compounds if restaked

**Example**:
```
Ticket Price: 300 DCR
Voting Reward: ~2.4 DCR
Time Locked: ~28 days average
Annual ROI: ~7.1%
```

**Calculation**:
```
ROI per vote = (Reward / Ticket Price) x 100
             = (2.4 / 300) x 100
             ~ 0.8%

Annual ROI = 0.8% x (365 / 28)
           ~ 10.4% theoretical

Actual ROI ~ 6-7% (accounting for expiries, timing)
```

### Risk Factors

**Expiration Risk**
- ~5% chance ticket expires without voting
- No reward if expired
- Can revoke to recover funds

**Price Volatility**
- DCR price may fluctuate
- Locked for ~28 days average
- Consider your risk tolerance

**Opportunity Cost**
- Funds locked during staking
- Cannot use for other purposes
- Compare returns to alternatives

---

## Staking Strategies

### Conservative Staking
**Goal**: Steady, predictable returns
**Approach**:
- Purchase tickets regularly (DCA)
- Use VSP for reliability
- Auto-revoke expired tickets
- Reinvest rewards

**Best for**: Long-term holders, passive stakers

---

### Active Staking
**Goal**: Maximize returns, active management
**Approach**:
- Time purchases at low difficulty
- Monitor pool size trends
- Solo stake (if always online)
- Optimize transaction fees

**Best for**: Experienced users, technical operators

---

### Accumulation Staking
**Goal**: Grow DCR holdings
**Approach**:
- Reinvest all rewards into new tickets
- Compound returns over time
- Long-term commitment
- Regular monitoring

**Best for**: HODLers, accumulation phase

---

## Monitoring Your Stakes

### Daily Checks
 Live ticket count (stable or growing?)
 Voted tickets (earning rewards?)
 Ticket fee status on the Ticket Status tab (all Confirmed?)
 Immature tickets (recently purchased?)

### Weekly Checks
 Total subsidy (rewards accumulating?)
 Ticket difficulty trends (buy now or wait?)
 Pool size changes (network health?)
 Revoked + missed count (within expected range?)

### Monthly Reviews
 Total ROI calculation
 Reward reinvestment strategy
 Difficulty trend analysis
 VSP performance (if using)

---

## Staking Operations

All staking operations are done from the dashboard tabs. You never need to edit
`dcrwallet.conf` or run `dcrctl`. Signing a purchase, starting the autobuyer, or
syncing fees prompts for your private passphrase in a modal.

### Purchasing Tickets (Purchase tab)

The **Purchase** tab holds the purchase form next to the **Ticket Pool &
Difficulty** and **My Tickets** cards.

1. **Source account**: Pick the account to fund the purchase from. When privacy
   is configured the source is locked to your **mixed** account and the ticket
   is bought as a mixed (private) ticket.
2. **Voting Service Provider (VSP)**: Search the VSP picker or type a host to
   use. The list is sorted by fee, lowest first. See [Choosing a VSP](#choosing-a-vsp-voting-service-provider) below.
3. **Number of tickets**: Enter how many to buy.
4. Review the cost breakdown (ticket price, stake total, estimated VSP fee, and
   the balance remaining after purchase), then click **Purchase** and enter your
   passphrase.

On success the tab lists each new ticket hash with a link to the block explorer.

**Mixed (private) purchases** run in the background: your funds are
CoinShuffle++ mixed before the ticket is bought, which can take up to ~10
minutes. A progress panel streams the log, and you can leave the page while it
runs.

---

### Automatic Ticket Buying (Auto Buyer tab)

The **Auto Buyer** tab runs an autobuyer that keeps buying tickets while the
source account's spendable balance stays above a threshold you set.

1. **Source account**: As on the Purchase tab, this is locked to your mixed
   account when privacy is configured.
2. **Voting Service Provider (VSP)**: Choose the VSP the autobuyer should use.
3. **Balance to maintain (DCR)**: The autobuyer keeps buying while spendable
   balance is above this value, and stops once it drops to the threshold.
4. **Save settings** to persist the configuration, then **Start** and enter your
   passphrase. Use **Stop** to halt it.

A status badge shows whether the autobuyer is running, and an **Autobuyer
events** log streams its activity (and the last error, if any). With privacy
configured, starting the autobuyer stops the standalone mixer; the autobuyer
mixes the tickets it buys while it runs.

**Caution**: Only enable this with a balance you intend to fully commit to
tickets.

---

### Choosing a VSP (Voting Service Provider)

**Why use a VSP?**
- Ensures votes even if your wallet is offline
- Professional infrastructure
- Small fee for reliability

A VSP is selected directly in the **Purchase**, **Auto Buyer**, and **Ticket
Status** tabs using the VSP picker; you do not register a VSP in a config file.

**How the picker works**:
- It lists VSPs from the public registry (`api.decred.org`), or, if the registry
  is disabled in Settings or unreachable, the VSPs you have already used here.
- Entries are sorted by fee percentage, lowest first.
- You can type any VSP host directly; the dashboard probes its
  `/api/v3/vspinfo` over HTTPS to fetch the fee and pubkey before selecting it.

The VSP handles the voting process for your tickets, and you earn rewards minus
the VSP fee.

---

### Ticket fee status and recovery (Ticket Status tab)

The **Ticket Status** tab groups your active tickets (Unmined, Immature, Live)
by their VSP fee status: **Fee Error**, **Unpaid Fee**, **Paid Fee**, **Confirmed
Fee**, and **Untracked**. A ticket only votes once its fee reaches **Confirmed**.

- **Sync Failed VSP Tickets**: Retries fee payment for tickets in Fee Error and
  re-checks paid fees against the VSP. You can also use it to migrate tracked
  tickets to a different VSP. Select a fee account and VSP, then run it and enter
  your passphrase.
- **Process Unmanaged Tickets**: Appears when you have live tickets that are not
  associated with a VSP (shown as Untracked). This typically happens after
  restoring or importing a wallet: the tickets are recovered but their VSP fee
  records are not. Select the VSP you bought them from to re-associate them so
  their fees are confirmed and they keep voting. Run it once per VSP if you used
  more than one.

### Revocation

Since DCP-0009 (auto-revocation, activated in 2022), missed and expired tickets
are revoked automatically by the network and their committed funds returned.
There is no manual revoke step and the dashboard does not expose one. Revoked
tickets simply appear in the My Tickets and History views.

---

## Troubleshooting

### No Tickets Showing in Dashboard

**Problem**: My Tickets card shows zeros

**Solutions**:
1. **Watch-only wallet**:
   - Cannot display tickets with xpub import
   - Need full RPC connection
   - See: [Wallet Setup](../guides/wallet-operations.md)

2. **Recently purchased**:
   - Check mempool count
   - Wait for confirmation
   - May take 5+ minutes

3. **RPC connection issue**:
   - Verify the wallet daemon is connected and synced
   - Check the dashboard's wallet RPC credentials in Settings
   - Restart the `dcrpulse-dashboard` and `dcrpulse-dcrwallet` containers if needed

---

### Ticket Stuck in Mempool

**Problem**: Mempool ticket not confirming

**Solutions**:
1. **Low transaction fee**:
   - May take multiple blocks
   - Check mempool competition
   - Consider fee bump (advanced)

2. **Full mempool**:
   - Wait for next block
   - Typically confirms within 1-2 blocks

3. **Transaction error**:
   - Check wallet logs
   - Verify sufficient balance
   - Ensure correct ticket price

---

### Tickets Stuck in Fee Error or Untracked

**Problem**: On the Ticket Status tab, tickets sit in **Fee Error** or
**Untracked** and are not voting.

**Solutions**:
1. **Fee Error**: Use **Sync Failed VSP Tickets** on the Ticket Status tab to
   retry the fee payment and re-check it against the VSP. The fee must reach
   **Confirmed** before the ticket can vote. The VSP may need more time, so
   re-sync shortly if it stays in error.

2. **Untracked** (after restore/import): Use **Process Unmanaged Tickets** to
   re-associate the tickets with the VSP you bought them from. Run it once per
   VSP if you used more than one.

3. **Check the wallet**: The wallet daemon must be running, synced, and able to
   reach the VSP over HTTPS.

Note: missed and expired tickets are revoked automatically since DCP-0009; a
growing expired count is not something you fix by hand.

---

### Lower Than Expected ROI

**Problem**: Returns below advertised ~7%

**Possible causes**:
1. **Recent start**: Not enough data yet
2. **Expired tickets**: ~5% don't vote
3. **VSP fees**: Reduces net return
4. **Price timing**: Bought at high difficulty
5. **Short duration**: ROI averages over time

**Check**:
- The **Vote Success** and **Avg Reward per Vote** metrics on the Statistics tab
- Average time to vote
- VSP fee structure
- Difficulty trend during purchases

---

## Advanced Metrics

### Effective ROI Calculation

```
Total Earned = Total Subsidy
Total Invested = (Avg Ticket Price) x (Total Tickets Bought)
Time Period = Days since first ticket

Annual ROI = (Total Earned / Total Invested) x (365 / Time Period) x 100
```

### Expected Vote Time

```
Average Time = (Pool Size / 5 votes per block) x 5 minutes per block

Current: (40,960 / 5) x 5 = 40,960 minutes
       = 28.4 days average
```

### Expiration Probability

```
Probability = 1 - (1 - 5/PoolSize)^MaxBlocks

Max Blocks = 40,960 (142 days)
Pool Size = 40,960
Probability ~ 5%
```

---

## Learning Resources

### Official Documentation
- [Decred Staking Guide](https://docs.decred.org/proof-of-stake/overview/)
- [Ticket Lifecycle](https://docs.decred.org/proof-of-stake/overview/#ticket-lifecycle)
- [VSP List](https://decred.org/vsp/)

### Dashboard Features
- [Wallet Dashboard](wallet-dashboard.md) - Balance and ticket overview
- [Wallet Operations](../guides/wallet-operations.md) - Manage your wallet
- [Governance](governance.md) - Consensus, treasury, and Politeia voting
- [Privacy Mixer](privacy-mixer.md) - Mixed (private) ticket purchases

### Community
- [Decred Discord](https://discord.gg/decred) - Ask questions
- [Decred Matrix](https://chat.decred.org) - Community chat
- [r/decred](https://reddit.com/r/decred) - Reddit community

---

## Staking Checklist

Before you start staking:

- [ ] Wallet fully synced
- [ ] Sufficient DCR balance (check current ticket price + fees)
- [ ] Understand ticket lifecycle
- [ ] Chosen a VSP in the picker
- [ ] Reviewed current difficulty trends
- [ ] Decided manual purchases or the autobuyer

During staking:

- [ ] Monitor dashboard regularly
- [ ] Track live ticket count
- [ ] Keep ticket fee status at Confirmed (Ticket Status tab)
- [ ] Review voting rewards (Statistics tab)
- [ ] Adjust strategy as needed

---

## Next Steps

Ready to start staking?

1. **[Setup Wallet](../guides/wallet-operations.md)** - Configure your wallet
2. **[Purchase Tickets](#purchasing-tickets-purchase-tab)** - Buy your first ticket
3. **[Monitor Dashboard](wallet-dashboard.md)** - Track your stakes
4. **[Join Community](#community)** - Get support

---

**Happy Staking!**

Questions? Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

