# Node Dashboard

The **Node Dashboard** provides comprehensive real-time monitoring of your Decred node (`dcrd`), including blockchain status, network health, peer connections, mempool activity, and supply statistics.

## Overview

The Node Dashboard is the default view when you open Decred Pulse. It displays critical node metrics at a glance and updates automatically every 30 seconds.

**Access**: Open `http://localhost:8080` or click the **"Node"** button in the header. The header nav links are Node, Wallet, Explorer, Treasury, Bison Relay, and DEX.

---

## Dashboard Components

### 1. Node Status Card

Displays the current sync status and version information of your `dcrd` node. The card header reads "Node Status" with a "Decred <version>" subtitle and a status badge.

#### Status Indicators

**Fully Synced**
- Node is running, responding, and caught up to the chain tip
- RPC connection established
- Ready for use

**Syncing**
- Initial blockchain download or catch-up in progress
- Shows sync progress percentage
- Displays the current sync message

**Stopped**
- Node not responding
- Configuration issue

**Connecting...**
- Default/unknown state shown before the first status is known

#### Sync Progress

During initial sync:
```
+--------------------------------------------+
|  Syncing Blockchain                  68%   |
|  [================........]                |
|                                            |
|  Processing blocks: 1,016,234 / 1,016,401  |
+--------------------------------------------+
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
- **dcrd version**: e.g., "2.0.6", shown as the card subtitle ("Decred 2.0.6")

The same version also appears in the "Version" badge in the header.

---

### 2. Recent Blocks Card

Lists the most recently mined blocks (newest first). It is titled "Recent Blocks" with a "Latest mined blocks" subtitle.

Each row shows:
- **Block height**: e.g., "Block #1,016,401"
- **Block hash**: First 16 characters, shown in monospace
- **Time ago**: How long since the block was mined (for example "2m ago")

Each row links to that block's page in the built-in [Explorer](explorer.md) (`/explorer/block/<height>`).

**Health Check**: Compare the latest height with a public explorer such as [dcrdata.org](https://dcrdata.org).

---

### 3. Network Metrics Grids

The page shows two rows of metric cards.

**First row** (four cards):
- **Circulating Supply** - total DCR mined and distributed, with a "DCR of 21 million" subtitle (max supply 21,000,000 DCR)
- **Network Peers** - count of connected peer nodes
- **Block Height** - latest block number
- **Network Hashrate** - estimated total network proof-of-work power

**Second row**:
- **Treasury Balance** - Decred DAO treasury balance ("Self-funded from block reward")
- **Supply Staked** - total DCR locked in tickets, with a "% of supply" indicator
- **Ticket Pool** - a wider card covering the live ticket pool (see below)

#### Circulating Supply
**Total DCR in circulation**
```
+-------------------------+
|  Circulating Supply     |
|                         |
|    15,234,567.89 DCR    |
|    DCR of 21 million    |
+-------------------------+
```

**What it is**: Total DCR mined and distributed

**Maximum Supply**: 21,000,000 DCR

**Emission Schedule**:
- Block reward decreases every 6,144 blocks (~21 days)
- Current reward: ~7.5 DCR per block
- Final DCR: ~2140 (estimated)

---

#### Supply Staked
**DCR locked in tickets**
```
+-------------------------+
|  Supply Staked          |
|                         |
|    6,123,456.78 DCR     |
|    40.2% of supply      |
+-------------------------+
```

**What it is**: Total DCR in active tickets

**Calculation**:
```
Staked = Pool Size x Ticket Price
       = 40,960 x ~293 DCR
       ~ 12,001,280 DCR
```

**Percentage**:
- Typical: 40-60%
- Higher = more participation
- Lower = more liquid supply

---

#### Treasury Balance
**Decred Treasury Balance**
```
+-------------------------+
|  Treasury Balance       |
|                         |
|    890,123.45 DCR       |
|    Self-funded          |
+-------------------------+
```

**What it is**: Decred DAO treasury

**Purpose**:
- Fund development
- Marketing initiatives
- Contractor payments
- Community projects

**Funding**: A share of each block reward

**Governance**: Stakeholders vote on spending

---

#### Network Hashrate
**Estimated network proof-of-work power**
```
+-------------------------+
|  Network Hashrate       |
|                         |
|    Total network power  |
+-------------------------+
```

**What it is**: Estimated total hashing power securing the chain

**Significance**: Higher = more proof-of-work security

---

#### Ticket Pool
**Live ticket pool summary** (wider card)

Shows the current pool size with an "At Target / Above Target / Below Target" badge (target 40,960), the current ticket price, the expected next price (with an up/down arrow), and the time until the next price adjustment.

---

### 4. Network Peers Card

Lists the connected peer nodes. The card header reads "Connected Peers" with a "<count> Active" badge and an "Active network connections" subtitle. It scrolls when there are many peers.

#### Peer Information

Each peer displays:

**Address**
- IP address and port, or ".onion" address for a Tor peer
- Tor peers show a purple onion icon; inbound Tor peers read "Inbound via Tor (<address>)"

**Version**
- Peer's dcrd version, shown as "dcrd <version>"

**Ping**
- Round-trip latency

**Up**
- How long the connection has been open (for example "2h 34m" or "5d 12h")

**Traffic**
- Data sent/received over the connection

**Sync Node**
- The current sync peer is marked with a star icon and a "SYNC" badge

#### Example Peer List

```
+--------------------------------------------------+
|  Connected Peers                  12 Active      |
+--------------------------------------------------+
| * 192.0.2.1:9108                          SYNC   |
|   dcrd 2.0.6                                      |
|   Ping: 45ms   Up: 2h 34m   Traffic: 45.6 MB     |
|                                                  |
|   198.51.100.42:9108                             |
|   dcrd 2.0.5                                      |
|   Ping: 89ms   Up: 5d 12h   Traffic: 120 MB      |
+--------------------------------------------------+
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

- Calculation: Pool Size x Ticket Price
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

The card header reads "Mempool Activity" with a "Current pending transactions" subtitle and a "Details" link to the mempool view (`/explorer/mempool`).

#### Pending Transactions
**Total unconfirmed transactions**
```
Pending Transactions: 18
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
Mempool Size: 16.16 KB
```

**What it is**: Total mempool data (auto-formatted as B, KB, or MB)

**Usage**: Indicates transaction volume

#### Transaction Breakdown

When there is activity, transactions are grouped into two sections. Only non-zero categories are shown.

**Staking Activity**
- **Tickets**: Stake submissions entering the ticket pool
- **Votes**: SSGen transactions (tickets that have voted)
- **Revocations**: SSRtx transactions (expired/missed ticket recovery)

**Regular Transactions**
- **Regular**: Standard DCR transfers
- **CoinJoin**: Mixed (CoinJoin) transactions

#### Example Display

```
+------------------------------------+
|  Mempool Activity        Details   |
+------------------------------------+
| Pending Transactions: 18           |
| Mempool Size: 16.16 KB             |
|                                    |
| Staking Activity                   |
|   Tickets: 15   Votes: 5           |
|                                    |
| Regular Transactions               |
|   Regular: 15   CoinJoin: 2        |
+------------------------------------+
```

---

## Auto-Refresh

Dashboard automatically refreshes every **30 seconds**.

### Refresh Behavior

**Automatic**:
- Fetches new data every 30s from `/api/dashboard`
- Updates all components
- Preserves scroll position
- No page reload

**Live sync progress**:
- While the node is syncing, the status bar also updates in real time over a WebSocket (`/api/node/sync/stream`), which is smoother than the 30s poll
- The 30s poll remains authoritative: once it reports the node is no longer syncing, the live frame is dropped

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
// dashboard/web/src/pages/NodeDashboard.tsx
useEffect(() => {
  fetchData();
  const interval = setInterval(fetchData, 30000); // Change to desired ms
  return () => clearInterval(interval);
}, []);
```

**Recommended Intervals**:
- Real-time monitoring: 10-15 seconds
- Normal use: 30 seconds (default)
- Low bandwidth: 60 seconds

---

## Dashboard Layout

### Desktop Layout

```
+-------------------------------------------------+
|  Header: Node | Wallet | Explorer | Treasury    |
|          Bison Relay | DEX | Version            |
+-------------------------------------------------+
|  Node Status Card (full width)                  |
+-----------+-----------+-----------+-------------+
| Circ.     | Network   | Block     | Network     |
| Supply    | Peers     | Height    | Hashrate    |
+-----------+-----------+-----------+-------------+
| Treasury  | Supply    | Ticket Pool             |
| Balance   | Staked    | (wide card)             |
+-----------+-----------+-------------------------+
|  Recent Blocks          |  Staking Statistics   |
+-------------------------+-----------------------+
|  Mempool Activity       |  Network Peers        |
|                         |  (scrollable list)    |
+-------------------------+-----------------------+
```

### Mobile Layout

Stacks vertically:
1. Node Status
2. Metric cards (1-2 columns)
3. Treasury Balance / Supply Staked / Ticket Pool
4. Recent Blocks
5. Staking Statistics
6. Mempool Activity
7. Network Peers

---

## Monitoring Best Practices

### Daily Checks

 **Node Status**: Ensure "Fully Synced"
 **Block Height**: Compare with network
 **Peer Count**: 8+ peers connected
 **Sync Progress**: 100% if not initial sync

### Weekly Checks

 **Disk Space**: Sufficient for growth (mainnet chain is ~30 GB and rising)
 **Peer Latency**: Reasonable ping times (< 500ms)
 **Mempool Size**: Not consistently full
 **Version**: Check for dcrd updates

### Monthly Checks

 **Performance**: Response times acceptable
 **Bandwidth**: Within expected limits
 **Logs**: Review for errors or warnings
 **Backups**: Verify blockchain data backups

---

## Troubleshooting

### Dashboard Shows "RPC client not connected"

**Problem**: Cannot connect to dcrd node (the dashboard shows a red error banner, for example "RPC client not connected. Please configure the connection below.")

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
   docker compose logs dashboard
   ```

5. **Restart services**:
   ```bash
   docker compose restart dcrd dashboard
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
   docker network inspect dcrpulse_decred-network
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

4. **Re-sync the chain** (last resort):
   ```bash
   docker compose stop dcrd
   # dcrd chain data lives under the shared app-data volume at /app-data/dcrd.
   # Removing it forces a full re-sync; it does not touch wallet data.
   docker run --rm -v dcrpulse_app-data:/data alpine sh -c 'rm -rf /data/dcrd'
   docker compose up -d dcrd
   ```
 **Warning**: Requires a full re-sync (several hours)

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
   docker stats dcrpulse-dcrd
   ```

4. **Check for dcrd errors**:
   ```bash
   docker compose logs dcrd | tail -50
   ```

---

## Understanding Metrics

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
Staked % = (Pool Size x Ticket Price) / Circulating Supply x 100
```

**Healthy Range**: 45-60%

**Significance**: Higher = more PoS security

---

### Network Hashrate

**Formula**:
```
Hashrate = (Difficulty x 2^32) / Block Time
```

**Units**: TH/s (terahashes per second)

**Significance**: PoW security level

---

## Security Indicators

### Healthy Node Signs

 **Synced**: 100% sync progress
 **Connected**: 8+ peers
 **Updated**: Latest dcrd version
 **Stable**: Uptime > 24 hours
 **Responsive**: Low latency peers

### Warning Signs

 **Stuck Sync**: Progress not moving
 **No Peers**: Isolated from network
 **High Latency**: Slow peer connections
 **Old Version**: Outdated dcrd
 **Fork Risk**: Different block hash than network

---

## Performance Optimization

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

## Related Documentation

- **[Quick Start](../getting-started/installation.md)** - Initial setup
- **[Docker Setup](../getting-started/installation.md)** - Docker configuration
- **[Staking Guide](staking-guide.md)** - Staking information
- **[API Reference](../api/api-reference.md)** - API endpoints
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues

---

## Node Health Checklist

### Pre-Flight Check
- [ ] Docker/dcrd installed
- [ ] RPC credentials configured
- [ ] Firewall allows port 9108
- [ ] Sufficient disk space (80+ GB recommended)
- [ ] Dashboard container started successfully

### Running Status
- [ ] Node status shows "Fully Synced"
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

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

