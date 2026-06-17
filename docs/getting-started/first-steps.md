# First Steps

What to do after installing Decred Pulse. This guide will help you get started with monitoring your Decred node and managing your wallet.

## Quick Checklist

After installation, complete these steps:

- [ ] Wait for initial blockchain sync
- [ ] Access the dashboard
- [ ] Verify node connectivity
- [ ] (Optional) Connect wallet
- [ ] (Optional) Import xpub for watch-only
- [ ] Explore the dashboard
- [ ] Set up monitoring routine

---

## 1. Wait for Initial Blockchain Sync

### What's Happening

After starting Decred Pulse, `dcrd` begins downloading and validating the blockchain.

**Time Required**:
- **Mainnet**: 4-8 hours
- **Testnet**: 30-60 minutes

**Disk Space**:
- **Mainnet**: ~30 GB and growing
- **Testnet**: ~1-2 GB

---

### Monitor Sync Progress

**Via Dashboard**:
1. Open http://localhost:8080
2. Node Status card shows sync progress
3. Progress bar indicates percentage complete

**Via CLI**:
```bash
# Check sync status
make sync-status

# View logs
make logs-dcrd

# Watch logs for progress
docker compose logs -f dcrd | grep -i "sync\|block"
```

**What to Look For**:
```
Syncing to block height 1016401 (100.00%)
Processed 36,000 headers in the last 30 seconds
Verified 1200 blocks in the last 30 seconds
```

---

### Sync Phases

#### Phase 1: Headers Sync (~5-15 minutes)
- Downloads block headers first
- Very fast
- Shows "Downloading headers..."

#### Phase 2: Blocks Sync (4-8 hours)
- Downloads full blocks
- Validates transactions
- Shows "Downloading blocks..."

#### Phase 3: Final Validation (~10-30 minutes)
- Completes verification
- Builds indexes
- Shows "Finalizing..."

---

### Tips During Sync

 **Do**:
- Let it run uninterrupted
- Keep computer running
- Maintain internet connection
- Monitor logs occasionally

 **Don't**:
- Stop services during sync
- Shut down computer
- Disconnect internet
- Modify configuration

---

## 2. Access the Dashboard

### Open in Browser

**URL**: http://localhost:8080

**Expected View**:
```
┌────────────────────────────────────────┐
│  Decred Pulse                          │
│  [Node Dashboard] [Wallet Dashboard]   │
├────────────────────────────────────────┤
│   Syncing Blockchain                 │
│  ━━━━━━━━━━━━━━░░░░░░  68%            │
│                                        │
│  Block 692,847 / 1,016,401            │
│  ~2 hours remaining                    │
└────────────────────────────────────────┘
```

---

### Dashboard Components

**Node Status Card**:
- Sync progress
- Version information
- Connection status

**Metrics Grid**:
- Circulating supply
- Staked supply
- Treasury size
- Exchange rate

**Blockchain Info**:
- Current block height
- Block hash
- Difficulty
- Chain size

**Network Peers**:
- Connected peers
- Peer versions
- Latency
- Traffic

**Mempool Activity**:
- Pending transactions
- Transaction types
- Mempool size

---

## 3. Verify Node Connectivity

### Check Node Status

**Via Dashboard**:
- Node Status card should show "Connected"
- Peer count should be > 0
- Sync progress increasing

**Via CLI**:
```bash
# Check status
make status

# Check peers
make peers

# View node info
make dcrctl CMD="getinfo"
```

---

### Healthy Node Indicators

 **Connected**: Green status indicator

 **Peers**: 8+ connected peers

 **Syncing**: Progress increasing steadily

 **No Errors**: Clean logs without errors

---

### Troubleshooting Connectivity

**Problem**: No peers connecting

**Solution**:
```bash
# Check firewall allows port 9108
sudo ufw allow 9108/tcp

# Check logs
make logs-dcrd | grep -i "peer\|connect"

# Restart dcrd
docker compose restart dcrd
```

**See**: [Troubleshooting Guide](../guides/troubleshooting.md#no-peers-connecting)

---

## 4. Connect Wallet (Optional)

### If You Have a Wallet

If you have an existing `dcrwallet` or Decrediton wallet:

#### Option A: Use Built-in dcrwallet

**Status**: dcrwallet is already running in Docker

**Check Status**:
```bash
# View wallet status
make wallet-info

# View balance
make wallet-balance
```

#### Option B: Import Xpub (Watch-Only)

**For monitoring only** (cannot send transactions):

1. Export xpub from your wallet:
   ```bash
   # From dcrwallet
   dcrctl --wallet getmasterpubkey default
   
   # Or from Decrediton: Accounts → Export
   ```

2. Open Wallet Dashboard: http://localhost:8080/wallet

3. Click "Import Xpub" button

4. Enter:
   - **Xpub key**: Your extended public key
   - **Gap limit**: 400 (recommended)

5. Click "Import"

6. Wait for rescan to complete (~10-30 minutes)

**See**: [Wallet Operations Guide](../guides/wallet-operations.md#import-extended-public-key-xpub)

---

### If You Don't Have a Wallet

**Create a new wallet**:

```bash
# Create new wallet via CLI
docker exec -it dcrpulse-dcrwallet dcrctl --wallet create

# Follow prompts to:
# 1. Set passphrase
# 2. Generate seed
# 3. Confirm seed
```

** CRITICAL**: Save your seed phrase securely offline!

**View seed** (one-time, right after creation):
```bash
make wallet-seed
```

---

## 5. Explore the Dashboard

### Node Dashboard

Navigate to: http://localhost:8080

**What to explore**:

1. **Node Status**
   - Current sync status
   - dcrd version
   - Connection health

2. **Supply Metrics**
   - Circulating DCR
   - Staked percentage
   - Treasury balance

3. **Blockchain Data**
   - Current height
   - Latest block hash
   - Network difficulty
   - Average block time

4. **Peer Connections**
   - Number of peers
   - Peer locations
   - Connection quality
   - Traffic stats

5. **Mempool Activity**
   - Pending transactions
   - Ticket purchases
   - Votes
   - Regular transactions

---

### Wallet Dashboard

Navigate to: http://localhost:8080/wallet

**If wallet connected**:

1. **Account Balance**
   - Total balance
   - Spendable amount
   - Locked in tickets

2. **Transaction History**
   - Recent transactions
   - Transaction types
   - Confirmations
   - Click to load more

3. **Accounts List**
   - All accounts
   - Individual balances
   - Balance types

4. **Ticket Pool Info**
   - Current ticket price
   - Pool size
   - Next difficulty
   - Mempool tickets

5. **My Tickets**
   - Your tickets
   - Ticket statuses
   - Voting rewards
   - Immature/live/voted

**If no wallet**:
- Dashboard shows connection options
- Can import xpub
- Or connect external wallet

---

## 6. Set Up Monitoring Routine

### Daily Checks

**Quick health check**:
```bash
# Check all services
make status

# View recent logs
make logs --tail=50
```

**Dashboard checks**:
- [ ] Node showing "Connected"
- [ ] Sync at 100%
- [ ] Peer count healthy (8+)
- [ ] No error messages

---

### Weekly Checks

**System maintenance**:
```bash
# Check disk space
df -h

# Check Docker resources
docker stats --no-stream

# Review logs for errors
make logs | grep -i "error\|warn"
```

**Dashboard checks**:
- [ ] Block height matches network
- [ ] Peer connections stable
- [ ] Wallet synced (if using)
- [ ] No unusual activity

---

### Monthly Checks

**Updates and backups**:
```bash
# Backup blockchain data
make backup

# Check for dcrd updates
# Visit: https://github.com/decred/dcrd/releases

# Update if new version available
make update-dcrd
```

---

## 7. Common Tasks

### View Logs

```bash
# All services
make logs

# Specific service
make logs-dcrd
make logs-backend
make logs-dcrwallet

# Last 50 lines
docker compose logs --tail=50 dcrd
```

---

### Restart Services

```bash
# All services
make restart

# Specific service
docker compose restart dcrd
docker compose restart dashboard
```

---

### Check Node Info

```bash
# Quick status
make status

# Detailed info
make dcrctl CMD="getinfo"

# Block count
make dcrctl CMD="getblockcount"

# Peer info
make peers
```

---

### Check Wallet Info

```bash
# Wallet status
make wallet-info

# Balance
make wallet-balance

# Transactions
make dcrctl-wallet CMD="listtransactions * 10"
```

---

## Learning Resources

### Documentation to Read

**Essential**:
- [Node Dashboard Guide](../features/node-dashboard.md)
- [Wallet Dashboard Guide](../features/wallet-dashboard.md)
- [Staking Guide](../features/staking-guide.md)

**Reference**:
- [CLI Commands](../reference/cli-commands.md)
- [Configuration](../setup/configuration.md)
- [Troubleshooting](../guides/troubleshooting.md)

---

### Understanding Decred

**Official Resources**:
- [Decred Documentation](https://docs.decred.org)
- [Decred.org](https://decred.org)
- [dcrdata Block Explorer](https://dcrdata.org)

**Community**:
- [Decred Discord](https://discord.gg/decred)
- [Decred Matrix](https://chat.decred.org)
- [r/decred](https://reddit.com/r/decred)

---

## Common Use Cases

### Use Case 1: Node Monitoring

**Goal**: Monitor node health and network status

**Daily routine**:
1. Open dashboard: http://localhost:8080
2. Check node status (should be "Connected")
3. Verify peer count (8+ peers)
4. Monitor mempool activity
5. Review blockchain info

**When to act**:
- Peers drop to 0: Check network/firewall
- Sync stops: Check logs and disk space
- Errors appear: Review troubleshooting guide

---

### Use Case 2: Watch-Only Wallet

**Goal**: Monitor wallet balance without private keys

**Setup**:
1. Export xpub from main wallet
2. Import to Decred Pulse
3. Wait for rescan
4. View balances and transactions

**Benefits**:
- Monitor without risk
- Track multiple accounts
- View transaction history
- Cannot send transactions

---

### Use Case 3: Staking Monitoring

**Goal**: Track ticket purchases and voting

**Setup**:
1. Connect wallet with tickets
2. Navigate to Wallet Dashboard
3. View "Ticket Pool" and "My Tickets" cards

**What to monitor**:
- Current ticket price
- Your live tickets
- Voting rewards
- Immature tickets

**See**: [Staking Guide](../features/staking-guide.md)

---

## Common First-Time Issues

### Issue: "RPC Not Connected"

**Cause**: dcrd not ready yet

**Solution**: Wait 30-60 seconds, refresh page

**If persists**:
```bash
# Check dcrd status
make status

# View logs
make logs-dcrd

# Restart if needed
docker compose restart dcrd
```

---

### Issue: "Slow Sync"

**Cause**: Normal for first sync

**Solutions**:
- Be patient (4-8 hours normal)
- Ensure good internet
- Check peers connected (8+)
- Use SSD if possible

---

### Issue: "Out of Disk Space"

**Cause**: Blockchain needs ~30 GB and grows over time

**Solutions**:
```bash
# Check space
df -h

# Clean Docker
docker system prune -a

# Free system space
# Remove unnecessary files
```

---

### Issue: "Wallet Not Synced"

**Cause**: Need to wait for dcrd sync first

**Solution**:
1. Wait for dcrd to reach 100%
2. Then dcrwallet will sync automatically
3. May take additional 10-30 minutes

---

## Success Checklist

After completing first steps:

- [ ] Dashboard accessible at http://localhost:8080
- [ ] Node showing "Connected" status
- [ ] Blockchain fully synced (100%)
- [ ] Peer count > 8
- [ ] No errors in logs
- [ ] (Optional) Wallet connected and synced
- [ ] (Optional) Xpub imported successfully
- [ ] Understand dashboard layout
- [ ] Know how to check logs
- [ ] Know how to restart services

---

## What's Next?

### For Node Operators

1. **Monitor Regularly**: Check dashboard daily
2. **Keep Updated**: Watch for dcrd updates
3. **Backup**: Create regular backups
4. **Optimize**: Tune configuration as needed

**See**: [Node Dashboard Guide](../features/node-dashboard.md)

---

### For Wallet Users

1. **Import Xpub**: Set up watch-only monitoring
2. **Monitor Balances**: Track account balances
3. **Review Transactions**: Check transaction history
4. **Understand Staking**: Learn about tickets

**See**: [Wallet Dashboard Guide](../features/wallet-dashboard.md)

---

### For Stakers

1. **Connect Wallet**: Full wallet connection
2. **Monitor Tickets**: Track your tickets
3. **Watch Pool**: Monitor ticket pool stats
4. **Optimize Returns**: Learn staking strategies

**See**: [Staking Guide](../features/staking-guide.md)

---

## Additional Help

### Getting Support

**Documentation**:
- [Troubleshooting Guide](../guides/troubleshooting.md)
- [FAQ](../guides/troubleshooting.md)
- [CLI Commands](../reference/cli-commands.md)

**Community**:
- [Decred Discord](https://discord.gg/decred)
- [Decred Matrix](https://chat.decred.org)
- GitHub Issues

**When Asking for Help**:
- Include relevant logs
- Describe what you tried
- Mention your OS and versions
- Check troubleshooting guide first

---

**Congratulations!** You've completed the first steps. Your Decred Pulse is now up and running!

**Enjoy monitoring your Decred node and wallet!**

