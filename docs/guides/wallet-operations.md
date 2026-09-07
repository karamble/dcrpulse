# Wallet Operations

Complete guide to managing your Decred wallet through the Decred Pulse dashboard, including importing xpub keys, rescanning the blockchain, and monitoring sync progress.

## Overview

Wallet operations allow you to:
- Import extended public keys (xpub) for watch-only monitoring
- Rescan the blockchain to discover transactions
- Track sync progress in real-time
- Manage wallet connectivity and configuration

---

## Import Extended Public Key (Xpub)

Import an **xpub key** to monitor wallet addresses without private key access. This creates a **watch-only wallet** that can view balances and transactions but cannot spend funds.

### What is an Xpub Key?

An **Extended Public Key (xpub)** is a master public key that can derive all public addresses for a wallet account. It's safe to share for monitoring purposes because:
- Can generate all receiving addresses
- Can view all transactions
- Cannot spend funds
- Cannot access private keys

**Use cases**:
- Monitor cold storage wallets
- Track balances without exposing private keys
- Audit wallet activity
- Portfolio monitoring

---

### How to Import Xpub

#### 1. Get Your Xpub Key

**From `dcrwallet` CLI:**
```bash
dcrctl --wallet getmasterpubkey default
```

**From Decrediton:**
1. Go to Accounts tab
2. Select account
3. Click "Export"
4. Copy the xpub key

**Example xpub**:
```
dpubZF6ScrXjYgjGdVL2FzAWMYpRbWbUk7VJT9JZjNGjqB9p5KMkJyKhGv8xv8riFP8...
```

**Security Notes**:
- Xpub reveals all addresses and transactions
- Can compromise privacy if shared carelessly
- Cannot spend funds (safe for viewing)
- Store securely but less critical than private keys

---

#### 2. Open Import Modal

In the Wallet Dashboard:
1. Click **"Import Xpub"** button (top right)
2. Modal dialog appears

---

#### 3. Enter Xpub Information

**From the hardware wallet's SD card** (optional)
- Loads the device's `accounts.dcr` export file so you pick accounts instead of pasting a key
- On the device: **Accounts**, then **"Write to SD card"** (one account) or **"Export all to SD card"**
- A file selection replaces the manual fields below, and the chosen accounts import one at a time, waiting out the 30-second rate limit between each

**Extended Public Key (xpub)** (required)
- Paste your extended public key
- Starts with `dpub` for Decred mainnet (or `tpub` for testnet)
- Long alphanumeric string

**Account Name** (required)
- A friendly name for the new watch-only account
- 50 characters or fewer
- Cannot be a reserved name (`mixed`, `unmixed`, `lightning`, `dex`, `imported`)
- Cannot match an account that already exists

**Account Index** (optional; shown only when the wallet you are importing into is itself watch-only)
- The BIP44 account number you exported from the device (0, 1, 2, ...)
- Leave it empty for a monitor-only xpub; set it to spend from this account through **Offline signing**
- Must be 0 to 2147483647 and must not repeat an index another imported account already uses

The import modal does not ask for a gap limit. The address gap limit is a
wallet-wide setting applied by the `dcrwallet` daemon (see
[Gap Limit Explained](#gap-limit-explained) below); it is not chosen per import.

---

#### 4. Import Process

After clicking **"Import Xpub"**:

1. **Validation** (instant)
   - Xpub format checked (must start with `dpub` or `tpub`)
   - Account name checked (length, reserved names, collisions)

2. **Import** (~1-2 seconds)
   - Xpub registered with the wallet (`importxpub`)
   - New watch-only account created
   - Address usage discovered (`discoverusage`)

3. **Blockchain Rescan** (automatic)
   - Starts automatically from block 0
   - Progress bar appears
   - Duration: 5-30 minutes depending on:
     - Blockchain height
     - Transaction count
     - System performance

4. **Completion**
   - Progress bar completes and hides once the wallet is synced
   - Dashboard cards appear
   - Balances and transactions visible

---

### Import Modal Reference

```
+---------------------------------------------+
|  Import Extended Public Key                 |
+---------------------------------------------+
|                                             |
|  From the hardware wallet's SD card         |
|  [ Import from file (accounts.dcr) ]        |
|                                             |
|  Extended Public Key (xpub) *               |
|  +-------------------------------------+    |
|  | dpubZF6ScrX...                      |    |
|  +-------------------------------------+    |
|                                             |
|  Account Name *                             |
|  +-------------------------------------+    |
|  | savings-xpub                        |    |
|  +-------------------------------------+    |
|                                             |
|  Account Index (optional, watch-only)       |
|  +-------------------------------------+    |
|  | 0                                   |    |
|  +-------------------------------------+    |
|                                             |
|  Note: after import, the wallet rescans     |
|  the blockchain from block 0 to find all    |
|  historical transactions. This typically    |
|  takes 5-30 minutes.                        |
|                                             |
|         [ Cancel ]    [ Import Xpub ]       |
+---------------------------------------------+
```

---

## Wallet Rescan

**Rescan** re-examines the blockchain to discover transactions and update balances. This is necessary when:
- Addresses were used while wallet was offline
- Gap limit was increased
- Transactions are missing
- Balance appears incorrect

### When to Rescan

 **After importing xpub** (automatic)
 **Increased gap limit** (manual)
 **Missing transactions** (manual)
 **Incorrect balance** (manual)
 **Wallet restored from seed** (automatic in dcrwallet)

 **Not needed for**:
- Regular operation
- New transactions (auto-detected)
- Wallet already synced

---

### How to Rescan

#### Automatic Rescan
- Triggered automatically when importing xpub
- No manual action needed
- Progress bar appears automatically

#### Manual Rescan

**Method 1: Dashboard Button**
1. Navigate to Wallet Dashboard
2. Click **"Rescan"** button (if available)
3. Confirm action
4. Progress bar appears

**Method 2: CLI** (Advanced)
```bash
dcrctl --wallet rescanwallet
```

**Method 3: RPC Endpoint**
```bash
curl -X POST http://localhost:8080/api/wallet/rescan \
  -H "Content-Type: application/json"
```

---

### Rescan Process

#### Phase 1: Initialization
- Wallet prepares for rescan
- Dashboard cards hidden
- Progress bar appears
- Normal 10s balance polling continues

#### Phase 2: Scanning
- Blockchain examined block by block
- Transactions discovered and indexed
- Balances calculated
- Progress updates arrive roughly once per second

**What you see**:
```
+--------------------------------------------+
|  Scanning Blockchain                       |
|  [==================--------]  68%         |
|                                            |
|  Block 758,120 / 1,113,281                 |
|  Finding your transactions...              |
+--------------------------------------------+
```

**Duration**:
- Empty wallet: 5-10 minutes
- Active wallet: 10-30 minutes
- Large gap limit (1000): 30-60 minutes

#### Phase 3: Completion
- Progress reaches 100%
- Progress bar auto-hides once the wallet is synced
- Dashboard cards reappear
- Data refreshed immediately
- Balance polling was never interrupted

---

## Sync Progress Monitoring

Real-time progress tracking during wallet rescan operations.

### Progress Bar Features

#### Visual Display
- **Percentage**: 0-100%
- **Progress Bar**: Animated fill
- **Block Count**: Current / Total
- **Status Message**: Operation description

#### Live Updates
- **Source**: gRPC rescan stream pushed over a WebSocket
- **Granularity**: Updates as each block range is scanned
- **Auto-Hide**: Disappears once the wallet is synced, not at a percentage threshold

#### Dashboard Behavior
- **During Sync**: Cards hidden, progress visible
- **After Sync**: Cards appear, progress hidden
- **On Navigation**: Persists if rescan active
- **Background**: Balance polling carries on; sync state arrives over the WebSocket

---

### Sync Progress States

#### Active Sync
```
Status: Rescanning...
Progress: 42%
Action: Wait for completion
```

**What happens**:
- Progress bar visible
- Dashboard cards hidden
- Dashboard streams gRPC rescan progress over a WebSocket
- Frontend displays real-time updates

---

#### Sync Completion
```
Status: Complete
Progress: 100%
Action: Dashboard refreshes
```

**What happens**:
- Progress bar auto-hides
- Dashboard cards appear
- Data fetched immediately
- Normal 10s balance polling continues

---

### Progress Tracking Technical Details

The dashboard runs a user-initiated rescan over gRPC and streams progress to the
browser over a WebSocket. The same stream carries the sync state the `dcrwallet`
daemon reports on its own (for example, after a restore), so there is one source
of progress for both.

#### Dashboard: rescan stream

**Location**: `dashboard/internal/handlers/wallet_grpc.go` (the WebSocket fan-out)

**Process**:
1. Starts the gRPC `Rescan` stream from a begin height (block 0 on xpub import).
2. Receives `RescanResponse` updates carrying the block height scanned through.
3. Fans out each update to subscribed WebSocket clients.
4. Marks the rescan finished when the stream completes.

WebSocket endpoint:
- `/api/wallet/grpc/stream-rescan` - the wallet sync snapshot (phase, percentage,
  block height). It pushes the current snapshot on connect, then one frame per
  update, with a 15-second keepalive ping.

#### Dashboard frontend: progress display

The wallet view subscribes to the rescan WebSocket, hides the dashboard cards
while a rescan is active, shows the progress bar, and re-fetches data from
`/api/wallet/dashboard` once the rescan finishes.

---

## Best Practices

### Import Xpub
 **Do**:
- Leave the standing gap limit at 20 (the dcrwallet default)
- Increase to 500-1000 if funds missing
- Wait for full rescan before using
- Keep xpub secure (privacy concern)

 **Don't**:
- Share xpub publicly (reveals all addresses)
- Set gap limit too low (may miss transactions)
- Navigate away during import
- Interrupt rescan process

---

### Wallet Rescan
 **Do**:
- Wait for blockchain sync completion first
- Use appropriate gap limit
- Monitor progress through dashboard
- Let rescan complete fully

 **Don't**:
- Rescan unnecessarily (wastes time)
- Stop rescan midway
- Rescan while blockchain syncing
- Use extremely high gap limits (>1000) unless needed

---

### Gap Limit Selection

The standing gap limit stays at `20` - dcrwallet's BIP0044 default, the value
Decrediton ships. When funds sit at higher address indices (a restored or
legacy wallet), do not raise the standing limit: run Settings → Wallet →
Address discovery with a larger one-shot gap for that scan.

| Scenario | One-shot discovery gap | Reasoning |
|----------|------------------------|-----------|
| New or normal wallet | not needed | The standing 20 covers it |
| Missing funds after restore | 500-1000 | High address indices |
| Legacy wallet | 1000+ | Very old or heavily used |

The standing limit is set on the `dcrwallet` daemon through the
`DCRWALLET_GAP_LIMIT` environment variable (default `20` everywhere). If you
change it in `.env`, recreate the container with `docker compose up -d
dcrwallet` - a plain restart keeps the old environment.

---

## Troubleshooting

### Xpub Import Failed

**Problem**: Import button does nothing or shows error

**Solutions**:
1. **Invalid xpub**:
   - Verify xpub format (starts with `dpub`)
   - Check for copy/paste errors
   - Ensure complete string copied

2. **Wallet not connected**:
   - Check RPC connection
   - Verify wallet is running
   - Check dashboard logs

3. **Account name rejected**:
   - Must be 50 characters or fewer
   - Cannot be a reserved name (`mixed`, `unmixed`, `lightning`, `dex`, `imported`)
   - Cannot match an account that already exists

---

### Rescan Stuck or Frozen

**Problem**: Progress bar not moving

**Solutions**:
1. **Check logs**:
   ```bash
   docker compose logs -f dcrwallet
   ```

2. **Verify wallet running**:
   ```bash
   docker compose ps dcrwallet
   ```

3. **Rescan already finished**:
   - Progress comes from dcrwallet's gRPC notifications, not from the log
   - A finished rescan simply stops sending them
   - Refresh page to check

4. **Wallet crashed**:
   ```bash
   docker compose restart dcrwallet
   ```
   - Rescan will resume from checkpoint

---

### Progress Bar Won't Disappear

**Problem**: Stuck at high percentage (90%+)

**Solutions**:
1. **Wait**: May be finalizing (can take 1-2 minutes)

2. **Check completion**:
   - Look for transactions in history
   - Check if balance updated
   - May have completed despite display

3. **Refresh page**:
   - Force refresh (Ctrl+Shift+R)
   - Progress should clear

4. **Manual clear**:
   - Navigate away and back
   - Dashboard will check real status

---

### Cards Not Appearing After Rescan

**Problem**: Progress bar gone but no dashboard cards

**Solutions**:
1. **Force refresh**:
   ```
   Ctrl + Shift + R (Windows/Linux)
   Cmd + Shift + R (Mac)
   ```

2. **Check browser console** (F12):
   - Look for JavaScript errors
   - Check network requests
   - Verify API responses

3. **Check wallet status**:
   ```bash
   docker compose logs dashboard | grep -i wallet
   ```

4. **Restart the dashboard**:
   ```bash
   docker compose restart dashboard
   ```

---

### Missing Transactions After Import

**Problem**: Expected transactions not showing

**Solutions**:
1. **Run a one-shot address discovery**:
   - Used addresses may be at high indices
   - Settings -> Wallet -> Address discovery, with a gap of 500 or 1000
   - Re-importing the xpub changes nothing: the import does not take a gap limit

2. **Wrong account**:
   - Ensure correct account xpub
   - Check account number in source wallet
   - Import additional account xpubs if needed

3. **Blockchain not fully synced**:
   - Check node sync status
   - Wait for full blockchain sync
   - Then rescan wallet

4. **Wrong network**:
   - Verify mainnet vs testnet
   - Check xpub corresponds to correct network

---

### Low Balance Found

**Problem**: Balance lower than expected

**Solutions**:
1. **Run a one-shot address discovery**:
   - The standing limit of 20 may be too low
   - High address indices not scanned
   - Settings -> Wallet -> Address discovery, doubling the one-shot gap

2. **Multiple accounts**:
   - Import xpubs for all accounts
   - Check source wallet account list
   - Each account needs separate xpub

3. **Rescan incomplete**:
   - Wait for 100% completion
   - Check the progress bar reached 100%
   - Allow background finalization

4. **Verify in source wallet**:
   - Compare with actual wallet balance
   - Check transaction history matches
   - Confirm correct xpub exported

---

## Gap Limit Explained

### What is a Gap Limit?

**Definition**: The number of consecutive unused addresses the wallet will monitor before assuming no more transactions exist.

**BIP0044 Standard**: Defines gap limit for HD wallets.

**Example**:
```
Address 0: Used
Address 1: Used
Address 2: Unused
Address 3: Unused
Address 4: Used
Address 5: Unused
...
Address 401: Unused

Gap Limit = 400
```

**With gap limit 400**:
- Monitors up to 400 consecutive unused addresses
- Finds address 4 (used)
- Keeps scanning as long as gaps stay under 400

**With gap limit 3**:
- Stops after 3 consecutive unused addresses
- Could miss a used address beyond that gap
- Incomplete balance

---

### Choosing Gap Limit

**Factors to consider**:
- **Wallet age**: Older = higher limit
- **Usage pattern**: Random = higher limit
- **Address reuse**: Sequential = lower limit
- **Scan time**: Higher = slower

**Scan time impact**:
```
Gap Limit 20:   ~2 minutes
Gap Limit 400:  ~10 minutes
Gap Limit 500:  ~25 minutes
Gap Limit 1000: ~50 minutes
```

---

## Security Considerations

### Xpub Safety

**What xpub reveals**:
- All public addresses
- All transactions
- Complete balance history

**What xpub cannot**:
- Spend funds
- Access private keys
- Sign transactions

**Privacy impact**:
- Links all addresses together
- Reveals transaction patterns
- Shows complete financial history

**Best practices**:
- Share xpub only with trusted parties
- Use different xpub for different purposes
- Consider privacy implications
- Store securely (less critical than private keys)

---

### Watch-Only Wallet Limitations

**Can do**:
- View balances
- Monitor transactions
- Generate receiving addresses
- Track transaction history

**Cannot do**:
- Sign transactions in the app (spend through **Offline signing** instead, below)
- Purchase tickets
- Sign messages
- Access private keys
- Vote with tickets
- Revoke tickets

**Use case**: Safe monitoring, with spending kept on an external signer.

---

### Offline Signing

A watch-only wallet holds no private keys, so it cannot sign in the app. It
spends through the **Offline signing** tab under **On-Chain Transactions**,
which replaces the **Send** tab for watch-only wallets and is the tab they open
by default. The flow is built for an air-gapped hardware wallet such as the
Foundation Passport, so the keys never leave the device.

1. **Export an unsigned transaction** - build the transaction in the dashboard,
   then download `unsigned.dcrtx` or scan the QR shown beside it (transactions
   too large for one QR are file-only).
2. **Sign it on the device** - carry the file across on microSD, or scan the QR.
   Verify the amount and recipient on the device before approving.
3. **Import the signed transaction** - drop in the device's `.dcrtx` file or
   paste the signed hex; the dashboard decodes it into a preview to check.
4. **Broadcast** - publish the signed transaction to the network.

The same tab exports `balance.dcr`, a per-account balance file the device reads
to show balances on its own screen. Put it in the same microSD folder as the
`.dcrtx` files.

**API**: `POST /api/wallet/build-sign-request`,
`POST /api/wallet/decode-signed-transaction`,
`POST /api/wallet/broadcast-signed-transaction`, and
`GET /api/wallet/device-balance`. None of them use private keys.

---

## Advanced Usage

### Multiple Account Monitoring

Import xpubs for multiple accounts:

1. **Export each account xpub**:
   ```bash
   dcrctl --wallet getmasterpubkey default
   dcrctl --wallet getmasterpubkey mixed
   dcrctl --wallet getmasterpubkey unmixed
   ```

2. **Import each individually**:
   - Use Import Xpub modal for each
   - Give each account a distinct name (there is no per-import gap limit)
   - Wait for each rescan to complete

3. **View in dashboard**:
   - All accounts appear in Accounts card
   - Each shows individual balances
   - Cumulative total includes all

---

### Automated Monitoring

For automated balance checking (advanced):

**API endpoint**:
```bash
curl http://localhost:8080/api/wallet/dashboard
```

**Response includes**:
```json
{
  "accountInfo": {...},
  "accounts": [...],
  "stakingInfo": {...},
  "walletStatus": {...}
}
```

See [API Reference](../api/api-reference.md) for details.

---

## Related Documentation

- **[Wallet Dashboard](../features/wallet-dashboard.md)** - Dashboard overview
- **[Configuration](../setup/configuration.md)** - Initial configuration
- **[Staking Guide](../features/staking-guide.md)** - Staking information
- **[API Reference](../api/api-reference.md)** - API documentation
- **[Troubleshooting](troubleshooting.md)** - Common issues

---

## Operations Checklist

### Before Importing Xpub
- [ ] Wallet RPC connected
- [ ] Blockchain fully synced
- [ ] Xpub key copied correctly
- [ ] Account name chosen (not reserved, not already in use)
- [ ] Address discovery run with a larger one-shot gap if funds sit past the default window (standing default 20)
- [ ] Time allocated (10-30 minutes)

### During Import
- [ ] Modal shows progress
- [ ] Don't navigate away
- [ ] Monitor progress bar
- [ ] Watch for completion

### After Import
- [ ] Verify balance matches expected
- [ ] Check transaction history
- [ ] Review all accounts
- [ ] Test dashboard features

### If Issues
- [ ] Check troubleshooting section
- [ ] Review logs
- [ ] Try higher gap limit
- [ ] Contact support if needed

---

**Need Help?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](troubleshooting.md)

