# Node Dashboard

The **Node Dashboard** provides comprehensive real-time monitoring of your Decred node (`dcrd`), including blockchain status, network health, peer connections, mempool activity, and supply statistics.

## 📊 Overview

The Node Dashboard is the default view when you open Decred Pulse. It displays critical node metrics at a glance and updates automatically every 30 seconds.

**Access**: Open `http://localhost:3000` or click the **"Node Dashboard"** button in the header.

---

## 🎯 Dashboard Components

### 1. Node Status Card

Displays the current sync status and version information of your `dcrd` node.

#### Status Indicators

**Connected** ✅
- Node is running and responding
- RPC connection established
- Ready for use

**Syncing** 🔄
- Initial blockchain download in progress
- Shows sync progress percentage
- Displays current sync phase

**Error** ❌
- RPC connection failed
- Node not responding
- Configuration issue

#### Sync Progress

During initial sync:
```
┌────────────────────────────────────────────┐
│  🔄 Syncing Blockchain                     │
│  ━━━━━━━━━━━━━━━━░░░░░░░░  68%            │
│                                            │
│  Downloading blocks: 1,016,234 / 1,016,401│
│  ~2 hours remaining                        │
└────────────────────────────────────────────┘
```

**Sync Phases**:
1. **Headers Sync**: Downloading block headers (fast)
2. **Blocks Sync**: Downloading and validating full blocks (slower)

**Typical Sync Time**:
- Mainnet: 4-8 hours
- Testnet: 30-60 minutes
- Depends on: Internet speed, CPU, disk I/O

#### Version Information

Displays:
- **dcrd version**: e.g., "2.0.6"
- **Protocol version**: Network protocol version
- **Build info**: Compilation details

---

### 2. Blockchain Information Card

Core blockchain metrics and current state.

#### Block Height
**Current Block**: Latest block number
```
Block Height: 1,016,401
```

**What it means**:
- Total blocks in the chain
- Increases every ~5 minutes
- Must match network consensus

**Health Check**: Compare with [dcrdata.org](https://dcrdata.org)

---

#### Block Hash
**Best Block Hash**: Hash of the current tip
```
Block Hash: 000000000000000000abc123...
```

**What it means**:
- Unique identifier for latest block
- Changes with each new block
- Used for verification

**Usage**: Verify your node is on the correct chain

---

#### Difficulty
**Mining Difficulty**: Current PoW difficulty
```
Difficulty: 223,847,291.45
```

**What it means**:
- How hard it is to mine a block
- Adjusts every 12 hours (144 blocks)
- Higher = more network hashrate

**Impact**: Indicates network security level

---

#### Chain Size
**Blockchain Size**: Disk space used
```
Chain Size: 8.45 GB
```

**What it means**:
- Total blockchain data size
- Grows over time
- Includes blocks and indexes

**Planning**: Ensure adequate disk space
- Current: ~10 GB
- Growth: ~2-3 GB per year

---

#### Average Block Time
**Block Time**: Average time between blocks
```
Block Time: 4m 52s
```

**Target**: 5 minutes per block

**What it means**:
- Network timing health
- Should be close to 5 minutes
- Variance is normal

**Concern if**: Consistently > 10 minutes (network issue)

---

### 3. Network Metrics Grid

Four key network statistics displayed as metric cards.

#### Circulating Supply
**Total DCR in circulation**
```
┌─────────────────────────┐
│ 💰 Circulating Supply   │
│                         │
│    15,234,567.89 DCR    │
│    of 21 million        │
└─────────────────────────┘
```

**What it is**: Total DCR mined and distributed

**Maximum Supply**: 21,000,000 DCR

**Emission Schedule**:
- Block reward decreases every 6,144 blocks (~21 days)
- Current reward: ~7.5 DCR per block
- Final DCR: ~2140 (estimated)

---

#### Staked Supply
**DCR locked in tickets**
```
┌─────────────────────────┐
│ 🔒 Staked Supply        │
│                         │
│    6,123,456.78 DCR     │
│    40.2% of supply      │
└─────────────────────────┘
```

**What it is**: Total DCR in active tickets

**Calculation**:
```
Staked = Pool Size × Ticket Price
       = 40,960 × ~293 DCR
       ≈ 12,001,280 DCR
```

**Percentage**:
- Typical: 40-60%
- Higher = more participation
- Lower = more liquid supply

---

#### Treasury Size
**Decred Treasury Balance**
```
┌─────────────────────────┐
│ 🏦 Treasury Size        │
│                         │
│    890,123.45 DCR       │
│    Self-funded          │
└─────────────────────────┘
```

**What it is**: Decred DAO treasury

**Purpose**:
- Fund development
- Marketing initiatives
- Contractor payments
- Community projects

**Funding**: 10% of block reward

**Governance**: Stakeholders vote on spending

---

#### Exchange Rate
**Current DCR Price**
```
┌─────────────────────────┐
│ 💵 Exchange Rate        │
│                         │
│    $32.45 USD           │
│    ↑ 2.3% (24h)         │
└─────────────────────────┘
```

**What it is**: Current DCR/USD price

**Source**: External price API

**Note**: May be delayed or unavailable

---

### 4. Network Peers Card

Lists all connected peer nodes with detailed statistics.

#### Peer Information

Each peer displays:

**Address** 🌐
- IP address and port
- Geographic location (if known)
- Connection type (inbound/outbound)

**Version** 🔢
- Peer's dcrd version
- Protocol compatibility
- User agent string

**Latency** ⚡
- Round-trip time (RTT)
- Measured in milliseconds
- Lower = better connection

**Connected Time** ⏱️
- Duration of connection
- Format: "2h 34m" or "5d 12h"
- Longer = more stable

**Traffic** 📊
- Data sent/received
- Upload/download ratio
- Total bytes transferred

**Sync Node** 🎯
- Primary sync peer indicator
- Used for block download
- Changes based on performance

#### Example Peer List

```
┌──────────────────────────────────────────────────┐
│ 🌐 Connected Peers (12)                          │
├──────────────────────────────────────────────────┤
│ 192.0.2.1:9108                        🎯 Sync    │
│ dcrd:2.0.6 │ 45ms │ 2h 34m │ ↑ 1.2 MB ↓ 45.6 MB │
│                                                  │
│ 198.51.100.42:9108                               │
│ dcrd:2.0.5 │ 89ms │ 5d 12h │ ↑ 5.6 MB ↓ 120 MB  │
│                                                  │
│ 203.0.113.15:9108                                │
│ dcrd:2.0.6 │ 67ms │ 1h 08m │ ↑ 0.8 MB ↓ 12.3 MB │
└──────────────────────────────────────────────────┘
```

#### Peer Count

**Healthy Range**: 8-125 peers

**Low Peers** (< 5):
- May indicate network issues
- Check firewall settings
- Verify P2P port (9108) is open

**High Peers** (> 100):
- Normal for well-connected node
- May use more bandwidth
- Adjust `maxpeers` in config if needed

---

### 5. Staking Statistics Card

Network-wide staking information (from node perspective).

#### Ticket Price
**Current Ticket Cost**
```
Ticket Price: 293.08 DCR
```

- Price to purchase one ticket
- Updates every 144 blocks (~12 hours)
- Adjusts based on demand

#### Pool Size
**Active Tickets**
```
Pool Size: 41,095 tickets
```

- Total live tickets in pool
- Target: ~40,960
- Indicates network participation

#### Locked DCR
**Total Staked**
```
Locked DCR: 12,012,456.00 DCR
```

- Calculation: Pool Size × Ticket Price
- Represents total staking commitment
- Typically 40-60% of supply

#### Participation Rate
**Staking Percentage**
```
Participation Rate: 52.3%
```

- Percentage of supply staked
- Higher = more PoS security
- Typical: 45-60%

---

### 6. Mempool Activity Card

Real-time mempool transaction statistics.

#### Transaction Count
**Pending Transactions**
```
Size: 18 transactions
```

**What it is**: Unconfirmed transactions

**Normal Range**: 0-50

**High Count** (> 100):
- Network congestion
- May increase fees
- Longer confirmation times

#### Mempool Size
**Data Size**
```
Bytes: 16,160 bytes
```

**What it is**: Total mempool data

**Usage**: Indicates transaction volume

#### Transaction Breakdown

**Regular Transactions** 💸
- Standard DCR transfers
- P2PKH, P2SH transactions
- Most common type

**Ticket Purchases** 🎫
- Stake submission outputs
- Calculate from stake difficulty
- Entering ticket pool

**Votes** ✅
- SSGen transactions
- Tickets that have voted
- Block validation

**Revocations** ❌
- SSRtx transactions
- Expired ticket recovery
- Funds being returned

#### Example Display

```
┌────────────────────────────────────┐
│ 📊 Mempool Activity                │
├────────────────────────────────────┤
│ Total: 18 transactions (16.2 KB)  │
│                                    │
│ 💸 Regular:      15                │
│ 🎫 Tickets:      15                │
│ ✅ Votes:        5                 │
│ ❌ Revocations:  0                 │
└────────────────────────────────────┘
```

---

## 🔄 Auto-Refresh

Dashboard automatically refreshes every **30 seconds**.

### Refresh Behavior

**Automatic**:
- Fetches new data every 30s
- Updates all components
- Preserves scroll position
- No page reload

**Manual**:
- Refresh browser (F5)
- Force refresh (Ctrl+Shift+R)
- Navigate away and back

**Failed Refresh**:
- Shows last known data
- Displays error message
- Continues attempting

### Adjust Refresh Interval

To change the 30-second interval:

**Frontend Configuration**:
```typescript
// frontend/src/pages/NodeDashboard.tsx
useEffect(() => {
  const interval = setInterval(fetchData, 30000); // Change to desired ms
  return () => clearInterval(interval);
}, []);
```

**Recommended Intervals**:
- Real-time monitoring: 10-15 seconds
- Normal use: 30 seconds (default)
- Low bandwidth: 60 seconds

---

## 🎨 Dashboard Layout

### Desktop Layout

```
┌─────────────────────────────────────────────────┐
│  Header: Node Dashboard | Wallet Dashboard      │
├─────────────────────────────────────────────────┤
│  Node Status Card (full width)                  │
├───────────┬───────────┬───────────┬─────────────┤
│ Circ.     │ Staked    │ Treasury  │ Exchange    │
│ Supply    │ Supply    │ Size      │ Rate        │
├───────────┴───────────┴───────────┴─────────────┤
│  Blockchain Information Card                    │
├─────────────────────────┬───────────────────────┤
│  Network Peers          │  Staking Statistics   │
│  (Scrollable list)      │  (Network-wide)       │
├─────────────────────────┴───────────────────────┤
│  Mempool Activity Card                          │
├─────────────────────────────────────────────────┤
│  Last updated: 2025-10-06 12:34:56             │
└─────────────────────────────────────────────────┘
```

### Mobile Layout

Stacks vertically:
1. Node Status
2. Metric Cards (2 columns)
3. Blockchain Info
4. Network Peers
5. Staking Stats
6. Mempool Activity

---

## 🔍 Monitoring Best Practices

### Daily Checks

✅ **Node Status**: Ensure "Connected" and synced  
✅ **Block Height**: Compare with network  
✅ **Peer Count**: 8+ peers connected  
✅ **Sync Progress**: 100% if not initial sync

### Weekly Checks

✅ **Disk Space**: Sufficient for growth (~10 GB current)  
✅ **Peer Latency**: Reasonable ping times (< 500ms)  
✅ **Mempool Size**: Not consistently full  
✅ **Version**: Check for dcrd updates

### Monthly Checks

✅ **Performance**: Response times acceptable  
✅ **Bandwidth**: Within expected limits  
✅ **Logs**: Review for errors or warnings  
✅ **Backups**: Verify blockchain data backups

---

## 🐛 Troubleshooting

### Dashboard Shows "RPC Not Connected"

**Problem**: Cannot connect to dcrd node

**Solutions**:

1. **Check dcrd is running**:
   ```bash
   docker compose ps dcrd
   # or
   ps aux | grep dcrd
   ```

2. **Verify RPC credentials**:
   - Check `.env` file
   - Ensure `DCRD_RPC_USER` and `DCRD_RPC_PASS` are set
   - These are passed to dcrd via docker-compose.yml

3. **Check RPC port**:
   ```bash
   netstat -tulpn | grep 9109
   ```

4. **Review logs**:
   ```bash
   docker compose logs dcrd
   docker compose logs backend
   ```

5. **Restart services**:
   ```bash
   docker compose restart dcrd backend
   ```

---

### Node Stuck Syncing

**Problem**: Sync progress not increasing

**Solutions**:

1. **Check peer connections**:
   - Need at least 1 peer
   - More peers = faster sync
   - Check firewall

2. **Verify disk space**:
   ```bash
   df -h
   ```

3. **Check logs for errors**:
   ```bash
   docker compose logs -f dcrd | grep -i error
   ```

4. **Restart sync**:
   ```bash
   docker compose restart dcrd
   ```

5. **Check system resources**:
   - CPU not maxed out
   - RAM available
   - Disk I/O not saturated

---

### No Peers Connecting

**Problem**: Peer count is 0 or very low

**Solutions**:

1. **Open P2P port (9108)**:
   ```bash
   # Check if port is open
   sudo ufw allow 9108/tcp
   
   # Or iptables
   sudo iptables -A INPUT -p tcp --dport 9108 -j ACCEPT
   ```

2. **Add seed nodes manually** via .env:
   ```bash
   # Edit .env
   DCRD_EXTRA_ARGS=--txindex --addpeer=mainnet-seed.decred.org --addpeer=mainnet-seed.decredbrasil.com
   
   # Restart
   docker compose restart dcrd
   ```

4. **Verify internet connection**:
   ```bash
   ping mainnet-seed.decred.org
   ```

5. **Check Docker network**:
   ```bash
   docker network inspect decred-pulse_decred-network
   ```

---

### Incorrect Block Height

**Problem**: Block height doesn't match network

**Solutions**:

1. **Verify network consensus**:
   - Check [dcrdata.org](https://dcrdata.org)
   - Compare block hash

2. **Check if on wrong fork**:
   - Review peers list
   - Look for peer version mismatches

3. **Restart node**:
   ```bash
   docker compose restart dcrd
   ```

4. **Reindex blockchain** (last resort):
   ```bash
   docker compose down dcrd
   docker volume rm dcrd-data
   docker compose up -d dcrd
   ```
   ⚠️ **Warning**: Requires full re-sync

---

### High Mempool Size

**Problem**: Mempool consistently > 100 transactions

**Solutions**:

1. **Check network conditions**:
   - May be network-wide congestion
   - Compare with dcrdata.org mempool

2. **Verify node is mining/voting**:
   - Low vote count may indicate issues
   - Check ticket voting setup

3. **Review memory usage**:
   ```bash
   docker stats decred-pulse-dcrd
   ```

4. **Check for dcrd errors**:
   ```bash
   docker compose logs dcrd | tail -50
   ```

---

## 📊 Understanding Metrics

### Circulating Supply

**Formula**: 
```
Circulating Supply = Total Mined - Treasury - Unmined
```

**Growth Rate**: ~7.5 DCR per block (~2,160 DCR/day)

**Use Case**: Market cap calculation

---

### Staked Percentage

**Formula**:
```
Staked % = (Pool Size × Ticket Price) / Circulating Supply × 100
```

**Healthy Range**: 45-60%

**Significance**: Higher = more PoS security

---

### Network Hashrate

**Formula**:
```
Hashrate = (Difficulty × 2^32) / Block Time
```

**Units**: TH/s (terahashes per second)

**Significance**: PoW security level

---

## 🔐 Security Indicators

### Healthy Node Signs

✅ **Synced**: 100% sync progress  
✅ **Connected**: 8+ peers  
✅ **Updated**: Latest dcrd version  
✅ **Stable**: Uptime > 24 hours  
✅ **Responsive**: Low latency peers

### Warning Signs

⚠️ **Stuck Sync**: Progress not moving  
⚠️ **No Peers**: Isolated from network  
⚠️ **High Latency**: Slow peer connections  
⚠️ **Old Version**: Outdated dcrd  
⚠️ **Fork Risk**: Different block hash than network

---

## 🚀 Performance Optimization

### Faster Sync

1. **Open P2P port (9108)**: Allow inbound peers
2. **Increase peer limit**: More concurrent downloads
3. **Use SSD**: Much faster than HDD
4. **Good internet**: 10+ Mbps recommended

### Lower Resource Usage

1. **Reduce peers**: Lower `maxpeers` in config
2. **Disable bloom filters**: If not needed
3. **Limit RPC connections**: Reduce polling frequency
4. **Use lighter indexes**: Disable non-essential indexes

---

## 📚 Related Documentation

- **[Quick Start](../quickstart.md)** - Initial setup
- **[Docker Setup](../docker-setup.md)** - Docker configuration
- **[Staking Guide](staking-guide.md)** - Staking information
- **[API Reference](../api/api-reference.md)** - API endpoints
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues

---

## ✅ Node Health Checklist

### Pre-Flight Check
- [ ] Docker/dcrd installed
- [ ] RPC credentials configured
- [ ] Firewall allows port 9108
- [ ] Sufficient disk space (15+ GB)
- [ ] Backend started successfully

### Running Status
- [ ] Node status shows "Connected"
- [ ] Sync progress at 100%
- [ ] Peer count: 8+
- [ ] Block height matches network
- [ ] Dashboard refreshing normally

### Maintenance
- [ ] Check logs weekly
- [ ] Update dcrd when available
- [ ] Monitor disk space
- [ ] Review peer connections
- [ ] Backup important data

---

**Questions?** Check the [FAQ](../reference/faq.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

