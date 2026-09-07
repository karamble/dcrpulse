# API Reference

API documentation for the Decred Pulse dashboard backend. Most endpoints use JSON for request and response bodies; fourteen upgrade to a WebSocket, and several serve or accept binary payloads (file embeds, downloads, backups, uploads). No `/api` route uses Server-Sent Events; the MCP server on its own port does, because the protocol calls for it (see [AI Agents (MCP)](../features/ai-agents-mcp.md)).

This reference documents the routes that actually exist, grouped the same way they are registered in `dashboard/cmd/dcrpulse/main.go`. The Node and Wallet sections below are documented in detail, and each feature group that follows carries the endpoints worth calling out plus a pointer to the matching feature doc. Every one of the 393 registered routes is listed in [Complete Route Index](#complete-route-index) at the end.

## Base URL

The frontend and the API are served by the same Go process, so the API is same-origin with the dashboard UI:

```
/api
```

In local development the dashboard listens on `http://localhost:8080` by default (override with the `PORT` environment variable), so the absolute base URL is `http://localhost:8080/api`. In a packaged deployment (Umbrel, CasaOS, or a reverse proxy) the host and scheme are whatever the proxy exposes; set `TRUSTED_PROXY=true` so the backend honors `X-Forwarded-Host` / `X-Forwarded-Proto` when validating same-origin requests.

The backend also requires RPC credentials to reach the underlying daemons (`dcrd`, `dcrwallet`, and, when enabled, `dcrlnd`, `brclientd`, and `dcrdex`); these are configured via environment variables, never exposed to the frontend.

---

## Authentication

Decred Pulse is a single-user dashboard. There are two layers in front of every `/api` route:

1. **Same-origin enforcement.** State-changing requests (`POST`, `PUT`, `PATCH`, `DELETE`) must carry an `Origin` header matching the dashboard's own host, otherwise they are rejected with `403 Forbidden`. Read-only requests (`GET`, `HEAD`, `OPTIONS`) bypass this check. WebSocket upgrades use the same host check. Behind a reverse proxy, set `TRUSTED_PROXY=true` so the forwarded host is honored.

2. **Optional app-password gate.** A single dashboard-wide password can be enabled under `/api/auth/*`. It is **off by default**, in which case the gate is a pass-through. When enabled, every `/api` route requires a valid signed session cookie (`dcrpulse_session`, HttpOnly, SameSite=Strict, 30-day TTL); only the login handshake (`/api/auth/login` and `/api/auth/status`) is exempt so the client can reach it. A failed gate returns `401 Unauthorized` with an `X-Dashboard-Auth: required` header so the frontend can distinguish it from a downstream daemon `401`.

Request bodies on state-changing methods are capped at 1 MiB. The cap skips `multipart/*` requests, but no route accepts one today: uploads (avatars, file sends, store files) arrive base64-encoded in JSON and are bounded by the same 1 MiB. A handful of expensive or daemon-cycling routes are additionally rate limited (see the per-group notes below) and return `429 Too Many Requests` when the allowance is exceeded.

There is no separate RPC/credential layer for clients: the backend speaks to the daemons on the client's behalf using its environment-configured credentials.

---

## Response Format

Most successful responses return JSON with appropriate HTTP status codes:

- **200 OK**: Successful request
- **400 Bad Request**: Invalid request body or parameters
- **401 Unauthorized**: App-password gate enabled and the session cookie is missing or invalid
- **403 Forbidden**: Cross-origin state-changing request rejected
- **429 Too Many Requests**: Rate-limited route called too frequently
- **500 Internal Server Error**: Server-side error
- **503 Service Unavailable**: Required RPC client (dcrd, dcrwallet, or a feature daemon) not initialized

Error responses:
```json
{
  "error": "Error description"
}
```

Fourteen routes upgrade to a WebSocket instead of returning a single JSON body. Most are named for it, but two are not - `GET /api/wallet/ln/send` and `GET /api/br/rtdt/sessions/{rv}/audio` - so read the Notes column in the [Complete Route Index](#complete-route-index) rather than the path:

`/api/node/sync/stream`, `/api/wallet/privacy/events`, `/api/wallet/staking/purchase/events`, `/api/wallet/staking/autobuyer/events`, `/api/wallet/governance/votetrickle/events`, `/api/wallet/grpc/stream-rescan`, `/api/wallet/ln/channel-events`, `/api/wallet/ln/invoice-events`, `/api/wallet/ln/liquidity/confirm/events`, `/api/wallet/ln/send`, `/api/br/events`, `/api/br/rtdt/sessions/{rv}/audio`, `/api/dcrdex/ws`, `/api/dcrdex/notify`.

Every browser socket is opened through one helper that applies the same-origin check and a read limit: 64 KiB on the control and event sockets, 4 MiB on the DCRDEX relay, and the Bison Relay protocol maximum on the BR sockets.

Binary routes (file embeds and downloads, backup and CSV export, the timestamp proof) return raw bytes rather than JSON. Bytes that came from a Bison Relay peer are served as `application/octet-stream` with `Content-Disposition: attachment` unless the declared type is an allowlisted image, and always under `default-src 'none'; sandbox`.

---

## Node Endpoints

Endpoints for monitoring Decred node (`dcrd`) status and blockchain information.

### Health Check

Check if the API server is running.

```http
GET /api/health
```

**Response**:
```json
{
  "status": "healthy",
  "rpcConnected": true,
  "walletRPCConnected": true,
  "dcrdTLS": true,
  "walletTLS": true,
  "time": "2025-10-06T12:34:56.789Z"
}
```

**Fields**:
- `status`: Always `"healthy"` when the server answers
- `rpcConnected`: A dcrd RPC client is initialized
- `walletRPCConnected`: A dcrwallet RPC client is initialized
- `dcrdTLS`: The dcrd connection uses TLS
- `walletTLS`: The dcrwallet connection uses TLS
- `time`: Server time when the response was built

**Status Codes**:
- `200`: Server is healthy

---

### Dashboard Data (All-in-One)

Get complete dashboard data in a single request. Combines node status, blockchain info, network peers, mempool, and supply data.

```http
GET /api/dashboard
```

**Response**:
```json
{
  "nodeStatus": {
    "status": "running",
    "syncProgress": 100,
    "version": "v2.0.6",
    "syncPhase": "synced",
    "syncMessage": "Fully synced"
  },
  "blockchainInfo": {
    "blockHeight": 1113281,
    "blockHash": "000000000000000000abc123...",
    "difficulty": 223847291.45,
    "chainSize": 0,
    "blockTime": "2m 14s",
    "recentBlocks": [
      {
        "height": 1113281,
        "hash": "000000000000000000abc123...",
        "timestamp": 1696600000
      }
    ]
  },
  "networkInfo": {
    "peerCount": 12,
    "hashrate": "612.40 PH/s",
    "networkHashPS": 612400000000000000
  },
  "peers": [
    {
      "id": 1,
      "address": "192.0.2.1:9108",
      "protocol": "TCP",
      "latency": "45ms",
      "connTime": "2h 15m",
      "traffic": "35.71 MB",
      "version": "2.0.6",
      "isSyncNode": false,
      "inbound": false,
      "tor": false
    }
  ],
  "supplyInfo": {
    "circulatingSupply": 15234567.89,
    "stakedSupply": 6123456.78,
    "stakedPercent": 40.19,
    "exchangeRate": "N/A",
    "treasurySize": 812345.67,
    "mixedPercent": "N/A"
  },
  "stakingInfo": {
    "ticketPrice": 293.0845535,
    "nextTicketPrice": 293.0845535,
    "poolSize": 41095,
    "lockedDCR": 6123456.78,
    "participationRate": 40.19,
    "allMempoolTix": 15,
    "immature": 1284,
    "live": 41095,
    "voted": 0,
    "missed": 0,
    "revoked": 0
  },
  "mempoolInfo": {
    "size": 18,
    "bytes": 16160,
    "txCount": 18,
    "totalFee": 0.00214,
    "averageFeeRate": 0.0001,
    "tickets": 3,
    "votes": 5,
    "revocations": 0,
    "regularTxs": 9,
    "coinJoinTxs": 1
  },
  "lastUpdate": "2025-10-06T12:34:56.789Z"
}
```

**Fields**:
- `nodeStatus`: dcrd process state (`running`, `syncing`, `connecting`), sync percentage, version string, and the current phase (`synced`, `starting`, `headers`, `blocks`) with a human-readable message
- `blockchainInfo`: Best block height and hash, difficulty ratio, time since the last block, and the three most recent blocks
- `networkInfo`: Peer count and network hashrate, both raw and formatted
- `peers`: One entry per connected peer, with formatted latency, connection age, and total traffic, plus `isSyncNode`, `inbound`, and `tor` flags
- `supplyInfo`: Circulating, staked, and treasury amounts in DCR. An amount dcrd could not supply is omitted rather than sent as zero; `exchangeRate` and `mixedPercent` are `"N/A"`
- `stakingInfo`: Ticket price, pool size, locked DCR, participation rate, and mempool ticket counts
- `mempoolInfo`: Mempool size and byte count, fee totals, and per-type transaction counts
- `degraded`: Present only when a section failed this poll; it names the sections that are zeroed so the UI can show them as unavailable rather than as real zeros

**Status Codes**:
- `200`: Success
- `503`: RPC client not connected

---

### Node Sync Stream

Subscribe to dcrd sync-progress snapshots. The connection upgrades to a WebSocket and receives the current snapshot immediately, then one message per refresh. Refreshes are driven by dcrd's block-connected notifications, throttled to at most one per second, with a safety timer every 20 seconds while the node is running and every 3 seconds while it is not.

```http
GET /api/node/sync/stream
```

**Message**:
```json
{
  "status": "syncing",
  "syncProgress": 68.5,
  "syncPhase": "blocks",
  "syncMessage": "Downloading blocks: 758,120 of 1,113,281",
  "blocks": 758120,
  "headers": 1113281,
  "syncHeight": 1113281
}
```

**Fields**:
- `status`: `running`, `syncing`, `connecting`, `starting`, `upgrading`, or `error`
- `syncProgress`: Percentage complete (0-100)
- `syncPhase`: `synced`, `starting`, `headers`, or `blocks`
- `syncMessage`: Human-readable status line
- `blocks`, `headers`: dcrd's current block and header counts
- `syncHeight`: The sync peer's advertised tip, used as the denominator so the bar cannot run backwards
- `startupNote`, `startupLog`: Present only while dcrd is unreachable - the startup explanation and the dcrd log line that classified it

This is the only route registered under `/api/node`. There are no `/api/node/status`, `/api/blockchain/*`, or `/api/network/*` routes; node, blockchain, and peer data comes from `GET /api/dashboard` above.

---

## Wallet Endpoints

Endpoints for managing and monitoring Decred wallet (`dcrwallet`).

### Wallet Status

Check wallet connectivity and basic status.

```http
GET /api/wallet/status
```

**Response**:
```json
{
  "status": "synced",
  "syncProgress": 100,
  "syncHeight": 1113281,
  "bestBlockHash": "000000000000000000abc123...",
  "version": "v2.1.6",
  "unlocked": true,
  "daemonConnected": true,
  "rescanInProgress": false,
  "syncMessage": "Fully synced",
  "isWatchOnly": false
}
```

**Fields**:
- `status`: Wallet state (see Status Values)
- `syncProgress`: Percentage complete (0-100)
- `syncHeight`: Wallet's best block height
- `bestBlockHash`: Wallet's best block hash
- `version`: dcrwallet version string
- `unlocked`: `true` if the wallet is unlocked
- `daemonConnected`: `true` if dcrwallet is connected to dcrd
- `rescanInProgress`: `true` while a rescan is running
- `syncMessage`: Human-readable status line
- `isWatchOnly`: `true` if the wallet has no spending keys

**Status Values**:
- `synced`: Wallet is loaded and fully synced
- `syncing`: Wallet is rescanning, fetching filters or headers, or discovering addresses
- `disconnected`: dcrwallet is not connected to dcrd
- `no_wallet`: No wallet is loaded

**Status Codes**:
- `200`: Success
- `503`: Wallet RPC not connected

---

### Wallet Dashboard Data

Get complete wallet dashboard data including balances, accounts, staking info, and wallet status.

```http
GET /api/wallet/dashboard
```

**Response**:
```json
{
  "walletStatus": {
    "status": "synced",
    "syncProgress": 100,
    "syncHeight": 1113281,
    "bestBlockHash": "000000000000000000abc123...",
    "version": "v2.1.6",
    "unlocked": true,
    "daemonConnected": true,
    "rescanInProgress": false,
    "syncMessage": "Fully synced",
    "isWatchOnly": false
  },
  "accountInfo": {
    "accountName": "default",
    "accountNumber": 0,
    "totalBalance": 1234.56789012,
    "spendableBalance": 1000.12345678,
    "immatureBalance": 0,
    "unconfirmedBalance": 0,
    "lockedByTickets": 234.44443334,
    "cumulativeTotal": 1234.56789012,
    "totalSpendable": 1000.12345678,
    "totalLockedByTickets": 234.44443334
  },
  "accounts": [
    {
      "accountName": "default",
      "accountNumber": 0,
      "totalBalance": 500.12345678,
      "spendableBalance": 450.12345678,
      "immatureBalance": 25.0,
      "unconfirmedBalance": 0,
      "immatureCoinbaseRewards": 0,
      "immatureStakeGeneration": 25.0,
      "lockedByTickets": 25.0,
      "votingAuthority": 0,
      "accountEncrypted": true,
      "accountUnlocked": false,
      "reserved": false
    },
    {
      "accountName": "mixed",
      "accountNumber": 1,
      "totalBalance": 734.44443334,
      "spendableBalance": 550.0,
      "immatureBalance": 0,
      "unconfirmedBalance": 0,
      "immatureCoinbaseRewards": 0,
      "immatureStakeGeneration": 0,
      "lockedByTickets": 184.44443334,
      "votingAuthority": 0,
      "accountEncrypted": true,
      "accountUnlocked": false,
      "reserved": true
    }
  ],
  "stakingInfo": {
    "poolSize": 41095,
    "allMempoolTix": 15,
    "ownMempoolTix": 0,
    "immature": 2,
    "unspent": 10,
    "voted": 45,
    "revoked": 1,
    "unspentExpired": 0,
    "totalSubsidy": 23.45678901,
    "currentDifficulty": 293.0845535,
    "nextDifficulty": 293.0845535,
    "estimatedMin": 291.54056324,
    "estimatedMax": 294.59121783,
    "estimatedExpected": 292.20480639
  },
  "lastUpdate": "2025-10-06T12:34:56.789Z"
}
```

**Field Descriptions**:

**walletStatus**:
- Status and connectivity information

**accountInfo** (summary):
- Wallet-wide balance totals
- Primary account information

**accounts** (detailed list):
- Individual account balances
- Granular balance types:
  - `spendableBalance`: Available for use
  - `immatureBalance`: Coinbase and stake rewards not yet mature
  - `immatureCoinbaseRewards`: Mining rewards awaiting maturity
  - `immatureStakeGeneration`: Voting rewards awaiting maturity
  - `lockedByTickets`: Funds in active tickets
  - `votingAuthority`: Delegated voting rights
  - `unconfirmedBalance`: Pending transactions
  - `reserved`: Account another daemon binds to by name, or the imported bucket
- `cumulativeTotal`, `totalSpendable` and `totalLockedByTickets` are wallet-wide
  totals and appear only on `accountInfo`, never on an entry in this list.

**stakingInfo**:
- Network pool statistics
- Personal ticket counts
- Difficulty information
- Estimated next difficulty

**Status Codes**:
- `200`: Success
- `503`: Wallet RPC not connected

---

### Transaction History

Get wallet transaction history with pagination support.

```http
GET /api/wallet/transactions?count=50&from=0
```

**Query Parameters**:
- `count` (optional): Number of transactions to return (default: 50)
- `from` (optional): Starting index for pagination (default: 0)

**Response**:
```json
{
  "transactions": [
    {
      "txid": "abc123def456...",
      "amount": 10.5,
      "fee": 0.0001,
      "confirmations": 6,
      "blockHash": "000000000000...",
      "blockTime": 1696600000,
      "time": "2025-10-06T12:34:56Z",
      "category": "receive",
      "txType": "regular",
      "address": "DsXyz...",
      "account": "default",
      "vout": 0,
      "generated": false
    },
    {
      "txid": "def456ghi789...",
      "amount": -5.25,
      "fee": 0.0001,
      "confirmations": 12,
      "blockHash": "000000000001...",
      "blockTime": 1696599000,
      "time": "2025-10-06T12:20:00Z",
      "category": "send",
      "txType": "regular",
      "address": "DsAbc...",
      "account": "default",
      "vout": 0,
      "generated": false
    },
    {
      "txid": "ghi789jkl012...",
      "amount": 293.08,
      "fee": 0,
      "confirmations": 256,
      "blockHash": "000000000002...",
      "blockTime": 1696590000,
      "time": "2025-10-06T09:00:00Z",
      "category": "immature",
      "txType": "ticket",
      "address": "",
      "account": "default",
      "vout": 0,
      "generated": false
    }
  ],
  "total": 127
}
```

**Transaction Fields**:
- `txid`: Transaction ID (hash)
- `amount`: Transaction amount in DCR (negative for sends)
- `fee`: Transaction fee in DCR
- `confirmations`: Number of confirmations
- `blockHash`: Block hash containing transaction
- `blockTime`: Block timestamp (Unix)
- `time`: Transaction time (ISO 8601)
- `category`: Transaction category
  - `send`: Outgoing transaction
  - `receive`: Incoming transaction
  - `immature`: Immature rewards
  - `generate`: Mined/staked generation
- `txType`: Transaction type
  - `regular`: Standard transaction
  - `ticket`: Ticket purchase
  - `vote`: Ticket vote
  - `revocation`: Ticket revocation
- `address`: Related address
- `account`: Wallet account name
- `vout`: Output index
- `generated`: `true` if coinbase/stakebase

**Status Codes**:
- `200`: Success
- `503`: Wallet RPC not connected

---

### Import Extended Public Key (Xpub)

Import an extended public key for watch-only wallet monitoring.

```http
POST /api/wallet/importxpub
```

**Request Body**:
```json
{
  "xpub": "dpubZF6ScrXjYgjGdVL2FzAWMYpRbWbUk7VJT9JZjNGjqB...",
  "accountName": "trezor",
  "accountIndex": 0,
  "rescan": true
}
```

**Request Fields**:
- `xpub` (required): Extended public key starting with `dpub` (or `tpub`)
- `accountName` (required): Name for the new watch-only account, 50 characters or fewer. It must not be a reserved account name (`imported`, `mixed`, `unmixed`, `lightning`, `dex`) or match an existing account.
- `accountIndex` (optional): The real BIP44 account index (0 to 2147483647) the xpub was derived from on the signing device, recorded so offline signing derives against the right account. Omit it for a monitor-only xpub. An index another imported account already uses is rejected.
- `rescan`: Accepted for compatibility. The import always runs address discovery and a rescan from block 0, so this field changes nothing.

**Response**:
```json
{
  "success": true,
  "message": "Xpub import started for account 'trezor'. Now discovering addresses and rescanning blockchain. This typically takes 5-30 minutes."
}
```

The import runs in the background, so the response returns immediately. `accountNum` is part of the response shape but is omitted here because the new account number is not known yet.

**Status Codes**:
- `200`: Import started, or the xpub prefix was rejected (with `success: false` in the body)
- `400`: Invalid request body, missing or over-long `accountName`, reserved name, or an out-of-range `accountIndex`
- `409`: The account name, the BIP44 index, or the xpub itself is already in use
- `503`: Wallet RPC not connected

**Note**: After import, wallet automatically begins rescanning. Monitor progress via `/api/wallet/sync-progress`.

---

### Rescan Wallet

Manually trigger wallet rescan to discover transactions.

```http
POST /api/wallet/rescan
```

**Request Body**:
```json
{
  "beginHeight": 0
}
```
`beginHeight` is the block to rescan from. An empty or unparsable body defaults
it to `0`, a rescan from genesis.

**Response**:
```json
{
  "success": true,
  "message": "Discovering addresses and rescanning blockchain from block 0. This may take 30+ minutes."
}
```

**Status Codes**:
- `200`: Rescan started
- `503`: Wallet RPC not connected

The rescan itself runs in the background - address discovery over JSON-RPC,
then a gRPC rescan - so the handler answers before any of it happens and a
later failure is reported in the log and the progress stream, not here.

**Note**: Monitor rescan progress via `/api/wallet/sync-progress`.

---

### Sync Progress

Get current wallet sync/rescan progress.

```http
GET /api/wallet/sync-progress
```

**Response (Active)**:
```json
{
  "isRescanning": true,
  "scanHeight": 758120,
  "chainHeight": 1113281,
  "progress": 68.5,
  "message": "Rescanning... 758120/1113281 blocks (68.1%)",
  "phase": "rescanning",
  "daemonConnected": true,
  "peerCount": 8,
  "cfiltersStart": 0,
  "cfiltersEnd": 0,
  "headersCount": 0,
  "firstHeaderTime": 0,
  "lastHeaderTime": 0
}
```

**Response (Complete)**:
```json
{
  "isRescanning": false,
  "scanHeight": 0,
  "chainHeight": 1113281,
  "progress": 0,
  "message": "Synced",
  "phase": "synced",
  "daemonConnected": true,
  "peerCount": 8,
  "cfiltersStart": 0,
  "cfiltersEnd": 0,
  "headersCount": 0,
  "firstHeaderTime": 0,
  "lastHeaderTime": 0
}
```

**Fields**:
- `isRescanning`: `true` while the phase is `fetching_cfilters`, `fetching_headers`, `discovering_addresses`, or `rescanning`
- `scanHeight`: Block the current phase has reached. Only `rescanning` and `fetching_cfilters` report one; every other phase sends `0`.
- `chainHeight`: Denominator for the current phase - the rescan's target during `rescanning`, dcrd's tip during `fetching_cfilters` and the phases with no bar of their own, and `0` throughout `fetching_headers`, which has no usable height to count against
- `progress`: Percentage complete for the phase in flight (0-100). It reports `0` in the `synced` phase, so read `phase` rather than `progress` to detect completion.
- `message`: Human-readable status message, derived from `phase`
- `phase`: One of `unknown`, `unsynced`, `fetching_cfilters`, `fetching_headers`, `discovering_addresses`, `rescanning`, `synced`
- `daemonConnected`: `true` if dcrwallet is connected to dcrd. When `false`, `message` reads `"Disconnected from dcrd"`.
- `peerCount`: Peers dcrwallet is synced against
- `cfiltersStart`, `cfiltersEnd`: Committed-filter range being fetched, during the `fetching_cfilters` phase
- `headersCount`: Headers fetched this session, during the `fetching_headers` phase
- `firstHeaderTime`, `lastHeaderTime`: Timestamps of the first and last header fetched this session, used to derive time-based progress while headers are downloading

**Status Codes**:
- `200`: Success
- `500`: Error reading progress

**Polling Recommendation**: Poll every 2 seconds during active rescan, stop when `isRescanning` is `false`.

---

## Wallet Operations

Beyond status and history, the wallet group exposes lifecycle, account, address, send, and settings endpoints. Selected routes:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallet/exists` | Whether a wallet database exists on disk |
| `GET` | `/api/wallet/loaded` | Whether the wallet is currently loaded by dcrwallet |
| `POST` | `/api/wallet/generate-seed` | Generate a new wallet seed |
| `POST` | `/api/wallet/decode-seed` | Decode/validate a seed mnemonic |
| `GET` | `/api/wallet/seed-words` | Word list used for seed entry/autocomplete |
| `POST` | `/api/wallet/create` | Create a wallet from a seed |
| `POST` | `/api/wallet/open` | Open (load + unlock) the wallet |
| `POST` | `/api/wallet/close` | Close the loaded wallet |
| `GET` | `/api/wallet/accounts` | List accounts with balances |
| `POST` | `/api/wallet/create-account` | Create a new account |
| `POST` | `/api/wallet/rename-account` | Rename an account (reserved accounts are protected) |
| `GET` | `/api/wallet/account-extended-pubkey` | Extended public key for an account |
| `GET` | `/api/wallet/next-address` | Fresh receive address |
| `GET` | `/api/wallet/validate-address` | Validate an address |
| `POST` | `/api/wallet/construct-transaction` | Build an unsigned send transaction |
| `POST` | `/api/wallet/sign-publish-transaction` | Sign and broadcast a transaction |
| `GET` | `/api/wallet/export` | Download wallet history as CSV. `?type=` selects `transactions` (the default), `tickets`, `votetime`, `balances`, or `dailybalances`. |
| `GET`/`POST` | `/api/wallet/settings` | Read / save wallet dashboard settings |
| `POST` | `/api/wallet/settings/change-passphrase` | Change the wallet passphrase |
| `POST` | `/api/wallet/settings/discover-addresses` | Re-run address discovery (rate limited: 1 / 30s) |
| `GET` | `/api/wallet/settings/logs` | Recent dcrwallet log lines |

See [Wallet Dashboard](../features/wallet-dashboard.md) and [Wallet Operations](../guides/wallet-operations.md).

---

## Multi-Wallet

Manage multiple independent wallet stacks (each with its own dcrwallet, dcrlnd, Bison Relay, and DEX state). Select/create/rename/delete relaunch the dcrwallet daemon and are rate limited (1 / 5s each).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallets` | List configured wallets and the active one |
| `POST` | `/api/wallets/select` | Switch the active wallet (relaunches daemons) |
| `POST` | `/api/wallets/create` | Create a new named wallet |
| `POST` | `/api/wallets/rename` | Rename a wallet |
| `POST` | `/api/wallets/delete` | Delete a wallet |

See [Multi-Wallet](../features/multi-wallet.md).

---

## Shared Wallets (Multisig)

Multisig wallets whose setup and signing rounds are coordinated over Bison Relay, under `/api/msig/*`. A manual wallet carries the same frames by hand instead, through the outbox and import routes.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/msig/wallets` | List the active wallet's shared-wallet records |
| `POST` | `/api/msig/wallets/invite` | Start a shared wallet round |
| `POST` | `/api/msig/wallets/accept` | Accept an incoming invite |
| `POST` | `/api/msig/wallets/decline` | Decline an incoming invite |
| `POST` | `/api/msig/wallets/activate` | Initiator checkpoint: every cosigner's key is in |
| `POST` | `/api/msig/wallets/confirm` | Cosigner checkpoint: the roster is verified |
| `POST` | `/api/msig/wallets/cancel` | Withdraw a round this wallet initiated |
| `GET` | `/api/msig/wallets/detail` | One record, with live balance data |
| `GET` | `/api/msig/wallets/backup` | Export the backup card for one shared wallet |
| `POST` | `/api/msig/wallets/restore` | Import a backup card into the active wallet |
| `POST` | `/api/msig/proposals/propose` | Build and dispatch a payment |
| `POST` | `/api/msig/proposals/sign` | Add this wallet's signature to an incoming request |
| `POST` | `/api/msig/proposals/reject` | Decline an incoming payment request |
| `POST` | `/api/msig/proposals/abort` | Cancel a payment this wallet proposed |
| `POST` | `/api/msig/proposals/rebroadcast` | Retry a fully signed payment |
| `GET` | `/api/msig/pending` | Open invites and deferred imports across all shared wallets |
| `POST` | `/api/msig/refresh` | Retry unsent frames and resume wallet-gated steps |
| `POST` | `/api/msig/receive` | Next receive address of an HD shared wallet |
| `GET` | `/api/msig/outbox` | A manual wallet's frames waiting to be handed over |
| `POST` | `/api/msig/outbox/done` | Record a frame as handed over |
| `POST` | `/api/msig/import` | Ingest one hand-carried coordination message |

See [Shared Wallets](../features/shared-wallets.md).

---

## Privacy / Mixer

Control the in-wallet CoinJoin mixer (account setup, start/stop, live event stream).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallet/privacy/status` | Mixer status and mixed/unmixed account info |
| `POST` | `/api/wallet/privacy/setup` | Set up the mixed/unmixed account pair |
| `POST` | `/api/wallet/privacy/start` | Start mixing |
| `POST` | `/api/wallet/privacy/stop` | Stop mixing |
| `GET` | `/api/wallet/privacy/events` | WebSocket stream of mixer log events |
| `GET`/`POST` | `/api/wallet/mixer/debug` | Mixer debug inspection/toggle |

See [Privacy / Mixer](../features/privacy-mixer.md).

---

## Staking

Ticket purchasing, VSP management, the autobuyer, and ticket listing.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallet/staking/vsps` | List known VSPs from the registry |
| `GET` | `/api/wallet/staking/vsp-info` | Probe a single VSP's `vspinfo` |
| `POST` | `/api/wallet/staking/purchase` | Purchase tickets |
| `GET` | `/api/wallet/staking/purchase/status` | Current purchase status |
| `GET` | `/api/wallet/staking/purchase/events` | WebSocket stream of purchase progress |
| `GET` | `/api/wallet/staking/tickets` | List wallet tickets |
| `POST` | `/api/wallet/staking/sync-failed-vsp-tickets` | Re-sync tickets that failed VSP registration |
| `POST` | `/api/wallet/staking/process-unmanaged-vsp-tickets` | Re-track unmanaged VSP tickets |
| `GET` | `/api/wallet/staking/autobuyer/status` | Autobuyer running state |
| `GET`/`POST` | `/api/wallet/staking/autobuyer/settings` | Read / save autobuyer settings |
| `POST` | `/api/wallet/staking/autobuyer/start` | Start the autobuyer |
| `POST` | `/api/wallet/staking/autobuyer/stop` | Stop the autobuyer |
| `GET` | `/api/wallet/staking/autobuyer/events` | WebSocket stream of autobuyer events |

See [Staking Guide](../features/staking-guide.md).

---

## Governance

Consensus agenda voting, treasury (TSpend) policies, and Politeia proposal browsing and voting. These live under `/api/wallet/governance/*`.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallet/governance/agendas` | Active consensus agendas and current choices |
| `POST` | `/api/wallet/governance/agendas/set` | Set an agenda vote choice |
| `GET` | `/api/wallet/governance/treasury/keys` | Treasury key vote policies |
| `POST` | `/api/wallet/governance/treasury/keys/set` | Set a treasury key policy |
| `GET` | `/api/wallet/governance/treasury/tspends` | Per-TSpend vote policies |
| `POST` | `/api/wallet/governance/treasury/tspends/set` | Set a TSpend vote policy |
| `GET` | `/api/wallet/governance/proposals` | List Politeia proposals |
| `GET` | `/api/wallet/governance/proposals/{token}` | Proposal detail |
| `POST` | `/api/wallet/governance/proposals/{token}/vote-eligibility` | Prepare a vote (eligible-ticket snapshot) |
| `POST` | `/api/wallet/governance/proposals/cast-vote` | Cast a Politeia vote |
| `POST` | `/api/wallet/governance/proposals/refresh` | Refresh the proposal list |
| `POST` | `/api/wallet/governance/proposals/{token}/refresh` | Refresh a single proposal |
| `POST` | `/api/wallet/governance/votetrickle/start` | Sign a proposal's eligible votes up front, then submit them spread over a duration |
| `POST` | `/api/wallet/governance/votetrickle/stop` | Stop a running trickle or dismiss a finished one (`?token=`) |
| `GET` | `/api/wallet/governance/votetrickle/status` | Live status of every trickle run |
| `GET` | `/api/wallet/governance/votetrickle/events` | WebSocket stream of trickle events (replays the last 200, then live) |

See [Governance](../features/governance.md).

---

## Treasury

Read the project treasury balance and scan its TSpend history. The full-history scan is expensive and rate limited (1 / 60s).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/treasury/info` | Treasury balance and summary |
| `GET` | `/api/treasury/balance-history` | Treasury balance over time |
| `POST` | `/api/treasury/scan-history` | Trigger a full TSpend history scan (rate limited) |
| `GET` | `/api/treasury/scan-progress` | TSpend scan progress |
| `GET` | `/api/treasury/scan-results` | TSpend scan results |

See [Governance](../features/governance.md).

---

## Explorer

A read-only block explorer over the connected dcrd node. All routes are `GET`.

| Path | Purpose |
| --- | --- |
| `/api/explorer/search` | Search by block height, hash, txid, or address |
| `/api/explorer/blocks/recent` | Most recent blocks |
| `/api/explorer/blocks/{height}` | Block by height |
| `/api/explorer/blocks/hash/{hash}` | Block by hash |
| `/api/explorer/transactions/{txhash}` | Transaction detail |
| `/api/explorer/address/{address}` | Address summary and history |
| `/api/explorer/mempool` | Current mempool transactions |

See [Explorer](../features/explorer.md).

---

## Lightning

Lightning Network operations backed by the optional `dcrlnd` daemon, under `/api/wallet/ln/*`. Endpoints cover daemon lifecycle, node/balance info, channels, liquidity, payments, invoices, backup, watchtowers, and graph queries. Representative routes:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallet/ln/status` | dcrlnd availability and lock/sync state |
| `POST` | `/api/wallet/ln/setup` | Initialize the Lightning wallet |
| `POST` | `/api/wallet/ln/unlock` | Unlock dcrlnd |
| `GET` | `/api/wallet/ln/info` | Node info (pubkey, peers, sync) |
| `GET` | `/api/wallet/ln/balance` | On-chain and channel balances |
| `GET` | `/api/wallet/ln/activity` | Recent Lightning activity |
| `GET` | `/api/wallet/ln/network` | Network/graph summary |
| `GET` | `/api/wallet/ln/channels` | List channels |
| `POST` | `/api/wallet/ln/channels/open` | Open a channel |
| `POST` | `/api/wallet/ln/channels/close` | Close a channel |
| `GET` | `/api/wallet/ln/peer-presets` | Suggested peer presets (Bison Relay seeder) |
| `GET` | `/api/wallet/ln/channel-events` | WebSocket stream of channel events |
| `GET`/`POST` | `/api/wallet/ln/liquidity/*` | Liquidity-ad defaults, estimate, request |
| `GET`/`POST` | `/api/wallet/ln/autopilot` | Autopilot status / configure |
| `POST` | `/api/wallet/ln/send/decode` | Decode a payment request |
| `GET` | `/api/wallet/ln/send` | Send a payment (streamed result) |
| `GET` | `/api/wallet/ln/payments` | Payment history |
| `GET` | `/api/wallet/ln/invoices` | List invoices |
| `POST` | `/api/wallet/ln/invoices/add` | Create an invoice |
| `POST` | `/api/wallet/ln/invoices/cancel` | Cancel an invoice |
| `GET` | `/api/wallet/ln/invoice-events` | WebSocket stream of invoice updates |
| `GET` | `/api/wallet/ln/backup` | Export the channel backup (SCB) |
| `POST` | `/api/wallet/ln/backup/verify` | Verify a channel backup |
| `GET`/`POST` | `/api/wallet/ln/watchtowers*` | List / add / remove watchtowers |
| `GET`/`POST` | `/api/wallet/ln/graph/*` | Graph node lookup, route query, search |

See [Lightning](../features/lightning.md).

---

## DEX (DCRDEX)

Decentralized-exchange trading backed by the optional `dcrdex` (bisonw) daemon, under `/api/dcrdex/*`. Endpoints cover client lifecycle, wallet management, the trade/order lifecycle, the market-maker bot, and live feeds. Representative routes:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/dcrdex/status` | Client status and registered exchanges |
| `POST` | `/api/dcrdex/init` | Initialize the DEX client |
| `POST` | `/api/dcrdex/unlock` | Unlock the DEX client |
| `POST` | `/api/dcrdex/lock` | Lock the DEX client |
| `GET` | `/api/dcrdex/exchanges` | Known exchanges / markets |
| `GET` | `/api/dcrdex/dexconfig` | Config for a DEX host |
| `POST` | `/api/dcrdex/postbond` | Post a bond to register |
| `GET`/`POST`/`DELETE` | `/api/dcrdex/wallets`, `/api/dcrdex/wallet*` | Manage per-asset DEX wallets (create, details, send, tx history, peers, deposit address) |
| `POST` | `/api/dcrdex/trade` | Place an order |
| `POST` | `/api/dcrdex/preorder` | Pre-order fee/option estimate |
| `POST` | `/api/dcrdex/maxbuy`, `/api/dcrdex/maxsell` | Max order size estimates |
| `GET` | `/api/dcrdex/myorders` | This account's orders |
| `POST` | `/api/dcrdex/orders`, `/api/dcrdex/order` | Order history / single order |
| `POST` | `/api/dcrdex/cancel` | Cancel an order |
| `GET`/`POST` | `/api/dcrdex/mm/*` | Market-maker status, market report, run logs, config, start/stop, and the running-bot config/inventory updates |
| `GET` | `/api/dcrdex/ws`, `/api/dcrdex/notify` | WebSocket live feeds (book/notifications) |

See [DEX](../features/dex.md).

---

## Bison Relay

Bison Relay messaging backed by the optional `brclientd` daemon, under `/api/br/*`. This is the largest group; it is organized by sub-area. Representative routes per area:

- **Status and identity**: `GET /api/br/version`, `GET /api/br/status`, `POST /api/br/setup`, `GET /api/br/identity`, `POST /api/br/avatar`, `GET`/`POST` `/api/br/connection`.
- **Backup and restore**: `GET /api/br/backup`, `POST /api/br/backup/prepare`, `GET /api/br/backup/status`, `POST /api/br/backup/restore`.
- **Messaging**: `GET /api/br/messages`, `POST /api/br/messages/clear`, `POST /api/br/pm`, `GET /api/br/events` (WebSocket event stream).
- **Contacts and KX**: `GET /api/br/contacts`, `POST /api/br/contacts/rename`, `.../block`, `.../unblock`, `.../ignore`, `.../kx-reset`, `.../handshake`, `.../tip`, contact groups under `/api/br/contacts/groups*`, and key-exchange listings under `/api/br/kx/*`.
- **Invites**: `POST /api/br/invites/write`, `POST /api/br/invites/accept`, `POST /api/br/join-decred-pulse`.
- **Posts (feed)**: `GET /api/br/posts`, `GET /api/br/posts/body`, `GET /api/br/posts/comments`, `POST /api/br/posts/comment`, `GET /api/br/posts/hearts`, `POST /api/br/posts/heart`, `POST /api/br/posts/new`, `POST /api/br/posts/relay`.
- **Group chat (GC)**: `GET /api/br/gc`, `POST /api/br/gc/create`, GC invites under `/api/br/gc/invites*`, and per-GC actions under `/api/br/gc/{gcid}/*` (message, history, invite, part, kick, admins, owner, alias, ...).
- **Files and shared content**: `GET /api/br/shared-files`, `POST /api/br/files/send`, downloads under `/api/br/downloads/{contact}*`, embeds under `/api/br/embeds/{contact}/{filename}`, and content fetch under `/api/br/content/*`.
- **Pages and storefront**: markdown pages under `/api/br/pages/*` and the runtime-switchable simplestore under `/api/br/store/*` (mode, products, orders, templates, uploads).
- **RTDT (realtime voice/text)**: session management under `/api/br/rtdt/sessions*` including per-session `invite`, `accept`, `join`, `leave`, `chat`, and `audio` (WebSocket).
- **Stats**: `/api/br/stats/*` (overview, payments, network, contacts, posts).
- **Payments and rates**: `GET /api/br/payments/tips`, `GET /api/br/rates`.

See [Bison Relay](../features/bison-relay.md).

---

## Timestamp (dcrtime)

File timestamping anchored to the Decred chain via the public dcrtime server, under `/api/timestamp/*`.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/timestamp/records` | List timestamp records |
| `POST` | `/api/timestamp/records` | Create a timestamp record from a digest |
| `GET` | `/api/timestamp/records/{digest}` | Get a record |
| `PATCH` | `/api/timestamp/records/{digest}` | Update record metadata |
| `DELETE` | `/api/timestamp/records/{digest}` | Delete a record |
| `POST` | `/api/timestamp/records/{digest}/retry` | Retry anchoring |
| `GET` | `/api/timestamp/records/{digest}/proof` | Inclusion proof |
| `POST` | `/api/timestamp/verify` | Verify a digest against the chain |
| `POST` | `/api/timestamp/validate` | Validate a proof file |
| `POST` | `/api/timestamp/refresh` | Refresh anchoring status |
| `GET` | `/api/timestamp/status` | Worker/anchoring status |
| `GET` | `/api/timestamp/export` | Export records |

See [Timestamp](../features/timestamp.md).

---

## Tor

Toggle and inspect the optional Tor runtime for daemon connectivity.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET`/`POST` | `/api/tor` | Read / set the Tor enabled state |
| `GET` | `/api/tor/status` | Tor bootstrap/connection status |
| `GET` | `/api/tor/control` | Tor control-port info |
| `POST` | `/api/tor/newidentity` | Request a new Tor circuit/identity |

---

## Themes

Server-persisted CSS-variable theming for the dashboard.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/themes` | Load saved themes and the active selection |
| `POST` | `/api/themes` | Save / switch / import themes |

See [Settings](../features/settings.md).

---

## Alerts

The dashboard-wide alert ring, with per-category opt-outs across `node`, `wallet`, `staking`, `lightning`, `dex`, `bisonrelay`, and `system`. The ring is capped at a few hundred entries, so filtering stays client-side.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/alerts` | Every entry, newest first |
| `GET` | `/api/alerts/summary` | The pill payload: unread and active counts, plus the highest severity among them |
| `POST` | `/api/alerts/{id}/read` | Mark one entry read |
| `POST` | `/api/alerts/read-all` | Mark every entry read |
| `GET` | `/api/alerts/settings` | Per-category opt-outs, defaults filled in |
| `POST` | `/api/alerts/settings` | Save the per-category opt-outs |

---

## Agents (MCP)

The agent surface: the dashboard's own MCP listener and its agent roster under `/api/settings/mcp/*`, and the Bison Relay MCP bridge under `/api/br/mcp/*`. **Every route in this group requires the app-password gate to be enabled**; with the gate off they return `401 Unauthorized` with an `X-Dashboard-Auth: password-required` header and the message `set a dashboard app password before managing AI agent access`, so minting a token or widening an agent's authority is never open to anyone who can reach the port.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/settings/mcp` | Live listener state, agent roster, and open sessions |
| `POST` | `/api/settings/mcp/enable` | Start or stop the MCP listener |
| `POST` | `/api/settings/mcp/tokens` | Mint a named agent token (the plaintext is returned exactly once) |
| `DELETE` | `/api/settings/mcp/tokens/{id}` | Delete an agent identity |
| `POST` | `/api/settings/mcp/agents/{id}/domains` | Replace an agent's capability domains |
| `POST` | `/api/settings/mcp/agents/{id}/ips` | Replace an agent's source-IP allowlist (IPs or CIDR prefixes) |
| `POST` | `/api/settings/mcp/agents/{id}/grant` | Grant an account-scoped spend capability |
| `DELETE` | `/api/settings/mcp/agents/{id}/grant` | Clear an agent's spend grant |
| `POST` | `/api/settings/mcp/agents/{id}/unblock` | Clear an agent's tripwire block |
| `POST` | `/api/settings/mcp/freeze-all` | Kill switch: revoke every grant and block every token |
| `GET` | `/api/settings/mcp/audit/export` | Download the persisted spend-audit trail as JSON |
| `POST` | `/api/settings/mcp/notify` | Save the Bison Relay oversight contact and on/off state |
| `POST` | `/api/settings/mcp/logging` | Turn agent-activity logging on or off |
| `GET`/`POST` | `/api/br/mcp/settings` | Read / save the Bison Relay MCP client settings |
| `GET` | `/api/br/mcp/pending` | Payments awaiting approval |
| `POST` | `/api/br/mcp/pending/resolve` | Approve or deny one pending payment |
| `GET` | `/api/br/mcp/spend` | The spend log and the rolling-day total |

See [AI Agents (MCP)](../features/ai-agents-mcp.md) and [Bison Relay MCP](../features/bison-relay-mcp.md).

---

## Auth (app-password gate)

Manage the optional dashboard-wide password. `/api/auth/status` and `/api/auth/login` are exempt from the gate so the client can reach the login handshake; `/api/auth/login` is rate limited (5 / s).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/auth/status` | Whether the gate is enabled/configured and the session state |
| `POST` | `/api/auth/login` | Exchange the password for a session cookie (rate limited) |
| `POST` | `/api/auth/setup` | First-time password configuration |
| `POST` | `/api/auth/skip-setup` | Dismiss the first-run setup prompt |
| `POST` | `/api/auth/logout` | Clear the session cookie |
| `POST` | `/api/auth/change` | Change the password |
| `POST` | `/api/auth/disable` | Disable the gate (clears the password) |

---

## Complete Route Index

Every route registered on the `/api` subrouter, in the groups above. The Notes
column carries only what is not true of every route: `App password` for the
routes behind `RequireAppPassword`, the allowance for a rate-limited route,
`WebSocket` for a route that upgrades, and the payload shape for one that does
not answer JSON. All 393 are subject to the same-origin check and, once the
gate is on, the session cookie.

### Node (3 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/dashboard` |  |
| `GET` | `/api/health` |  |
| `GET` | `/api/node/sync/stream` | WebSocket |

### Wallet (30 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/account-extended-pubkey` |  |
| `GET` | `/api/wallet/accounts` |  |
| `POST` | `/api/wallet/broadcast-signed-transaction` |  |
| `POST` | `/api/wallet/build-sign-request` |  |
| `GET` | `/api/wallet/claimable-account-names` |  |
| `POST` | `/api/wallet/close` |  |
| `POST` | `/api/wallet/construct-transaction` |  |
| `POST` | `/api/wallet/create` |  |
| `POST` | `/api/wallet/create-account` |  |
| `GET` | `/api/wallet/dashboard` |  |
| `POST` | `/api/wallet/decode-seed` |  |
| `POST` | `/api/wallet/decode-signed-transaction` |  |
| `GET` | `/api/wallet/device-balance` |  |
| `GET` | `/api/wallet/exists` |  |
| `GET` | `/api/wallet/export` | CSV download |
| `POST` | `/api/wallet/generate-seed` |  |
| `POST` | `/api/wallet/importxpub` | Rate limit 1 per 30s |
| `GET` | `/api/wallet/loaded` |  |
| `GET`, `POST` | `/api/wallet/mixer/debug` |  |
| `GET` | `/api/wallet/next-address` |  |
| `POST` | `/api/wallet/open` | Rate limit 5 per second |
| `POST` | `/api/wallet/parse-account-export` |  |
| `POST` | `/api/wallet/rename-account` |  |
| `POST` | `/api/wallet/rescan` | Rate limit 1 per 60s |
| `GET` | `/api/wallet/seed-words` |  |
| `POST` | `/api/wallet/sign-publish-transaction` |  |
| `GET` | `/api/wallet/status` |  |
| `GET` | `/api/wallet/sync-progress` |  |
| `GET` | `/api/wallet/transactions` |  |
| `GET` | `/api/wallet/validate-address` |  |

### Wallet gRPC (1 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/grpc/stream-rescan` | WebSocket |

### Wallet settings (5 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/settings` |  |
| `POST` | `/api/wallet/settings` |  |
| `POST` | `/api/wallet/settings/change-passphrase` |  |
| `POST` | `/api/wallet/settings/discover-addresses` | Rate limit 1 per 30s |
| `GET` | `/api/wallet/settings/logs` |  |

### Multi-wallet (5 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallets` |  |
| `POST` | `/api/wallets/create` | Rate limit 1 per 5s |
| `POST` | `/api/wallets/delete` | Rate limit 1 per 5s |
| `POST` | `/api/wallets/rename` | Rate limit 1 per 5s |
| `POST` | `/api/wallets/select` | Rate limit 1 per 5s |

### Shared (multisig) wallets (21 routes)

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/msig/import` |  |
| `GET` | `/api/msig/outbox` |  |
| `POST` | `/api/msig/outbox/done` |  |
| `GET` | `/api/msig/pending` |  |
| `POST` | `/api/msig/proposals/abort` |  |
| `POST` | `/api/msig/proposals/propose` |  |
| `POST` | `/api/msig/proposals/rebroadcast` |  |
| `POST` | `/api/msig/proposals/reject` |  |
| `POST` | `/api/msig/proposals/sign` |  |
| `POST` | `/api/msig/receive` |  |
| `POST` | `/api/msig/refresh` |  |
| `GET` | `/api/msig/wallets` |  |
| `POST` | `/api/msig/wallets/accept` |  |
| `POST` | `/api/msig/wallets/activate` |  |
| `GET` | `/api/msig/wallets/backup` |  |
| `POST` | `/api/msig/wallets/cancel` |  |
| `POST` | `/api/msig/wallets/confirm` |  |
| `POST` | `/api/msig/wallets/decline` |  |
| `GET` | `/api/msig/wallets/detail` |  |
| `POST` | `/api/msig/wallets/invite` |  |
| `POST` | `/api/msig/wallets/restore` |  |

### Privacy / mixer (5 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/privacy/events` | WebSocket |
| `POST` | `/api/wallet/privacy/setup` |  |
| `POST` | `/api/wallet/privacy/start` |  |
| `GET` | `/api/wallet/privacy/status` |  |
| `POST` | `/api/wallet/privacy/stop` |  |

### Staking (14 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/staking/autobuyer/events` | WebSocket |
| `GET` | `/api/wallet/staking/autobuyer/settings` |  |
| `POST` | `/api/wallet/staking/autobuyer/settings` |  |
| `POST` | `/api/wallet/staking/autobuyer/start` |  |
| `GET` | `/api/wallet/staking/autobuyer/status` |  |
| `POST` | `/api/wallet/staking/autobuyer/stop` |  |
| `POST` | `/api/wallet/staking/process-unmanaged-vsp-tickets` | Rate limit 1 per 30s |
| `POST` | `/api/wallet/staking/purchase` |  |
| `GET` | `/api/wallet/staking/purchase/events` | WebSocket |
| `GET` | `/api/wallet/staking/purchase/status` |  |
| `POST` | `/api/wallet/staking/sync-failed-vsp-tickets` | Rate limit 1 per 30s |
| `GET` | `/api/wallet/staking/tickets` |  |
| `GET` | `/api/wallet/staking/vsp-info` |  |
| `GET` | `/api/wallet/staking/vsps` |  |

### Governance (18 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/governance/agendas` |  |
| `POST` | `/api/wallet/governance/agendas/set` |  |
| `GET` | `/api/wallet/governance/agendas/{id}/votes` |  |
| `GET` | `/api/wallet/governance/proposals` |  |
| `POST` | `/api/wallet/governance/proposals/cast-vote` |  |
| `POST` | `/api/wallet/governance/proposals/load-more` |  |
| `POST` | `/api/wallet/governance/proposals/refresh` |  |
| `GET` | `/api/wallet/governance/proposals/{token}` |  |
| `POST` | `/api/wallet/governance/proposals/{token}/refresh` |  |
| `POST` | `/api/wallet/governance/proposals/{token}/vote-eligibility` |  |
| `GET` | `/api/wallet/governance/treasury/keys` |  |
| `POST` | `/api/wallet/governance/treasury/keys/set` |  |
| `GET` | `/api/wallet/governance/treasury/tspends` |  |
| `POST` | `/api/wallet/governance/treasury/tspends/set` |  |
| `GET` | `/api/wallet/governance/votetrickle/events` | WebSocket |
| `POST` | `/api/wallet/governance/votetrickle/start` |  |
| `GET` | `/api/wallet/governance/votetrickle/status` |  |
| `POST` | `/api/wallet/governance/votetrickle/stop` |  |

### Treasury (5 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/treasury/balance-history` |  |
| `GET` | `/api/treasury/info` |  |
| `POST` | `/api/treasury/scan-history` | Rate limit 1 per 60s |
| `GET` | `/api/treasury/scan-progress` |  |
| `GET` | `/api/treasury/scan-results` |  |

### Explorer (7 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/explorer/address/{address}` |  |
| `GET` | `/api/explorer/blocks/hash/{hash}` |  |
| `GET` | `/api/explorer/blocks/recent` |  |
| `GET` | `/api/explorer/blocks/{height:[0-9]+}` |  |
| `GET` | `/api/explorer/mempool` |  |
| `GET` | `/api/explorer/search` |  |
| `GET` | `/api/explorer/transactions/{txhash}` |  |

### Lightning (36 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/wallet/ln/activity` |  |
| `GET` | `/api/wallet/ln/autopilot` |  |
| `POST` | `/api/wallet/ln/autopilot` |  |
| `GET` | `/api/wallet/ln/autopilot/scores` |  |
| `GET` | `/api/wallet/ln/backup` |  |
| `POST` | `/api/wallet/ln/backup/verify` |  |
| `GET` | `/api/wallet/ln/balance` |  |
| `GET` | `/api/wallet/ln/channel-events` | WebSocket |
| `GET` | `/api/wallet/ln/channels` |  |
| `POST` | `/api/wallet/ln/channels/close` |  |
| `POST` | `/api/wallet/ln/channels/open` |  |
| `GET` | `/api/wallet/ln/graph/node` |  |
| `POST` | `/api/wallet/ln/graph/routes` |  |
| `GET` | `/api/wallet/ln/graph/search` |  |
| `GET` | `/api/wallet/ln/info` |  |
| `GET` | `/api/wallet/ln/invoice-events` | WebSocket |
| `GET` | `/api/wallet/ln/invoices` |  |
| `POST` | `/api/wallet/ln/invoices/add` |  |
| `POST` | `/api/wallet/ln/invoices/cancel` |  |
| `POST` | `/api/wallet/ln/liquidity/confirm` |  |
| `GET` | `/api/wallet/ln/liquidity/confirm/events` | WebSocket |
| `GET` | `/api/wallet/ln/liquidity/confirm/pending` |  |
| `GET` | `/api/wallet/ln/liquidity/defaults` |  |
| `POST` | `/api/wallet/ln/liquidity/estimate` |  |
| `POST` | `/api/wallet/ln/liquidity/request` |  |
| `GET` | `/api/wallet/ln/network` |  |
| `GET` | `/api/wallet/ln/payments` |  |
| `GET` | `/api/wallet/ln/peer-presets` |  |
| `GET` | `/api/wallet/ln/send` | WebSocket |
| `POST` | `/api/wallet/ln/send/decode` |  |
| `POST` | `/api/wallet/ln/setup` |  |
| `GET` | `/api/wallet/ln/status` |  |
| `POST` | `/api/wallet/ln/unlock` | Rate limit 5 per second |
| `GET` | `/api/wallet/ln/watchtowers` |  |
| `POST` | `/api/wallet/ln/watchtowers/add` |  |
| `POST` | `/api/wallet/ln/watchtowers/remove` |  |

### DEX (DCRDEX) (60 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/dcrdex/account` |  |
| `GET` | `/api/dcrdex/actions` |  |
| `POST` | `/api/dcrdex/actions/take` |  |
| `GET` | `/api/dcrdex/assets` |  |
| `POST` | `/api/dcrdex/bondopts` |  |
| `GET` | `/api/dcrdex/bondsfeebuffer` |  |
| `POST` | `/api/dcrdex/cancel` |  |
| `GET` | `/api/dcrdex/dexconfig` |  |
| `POST` | `/api/dcrdex/discover-account` | Rate limit 1 per 10s |
| `GET` | `/api/dcrdex/exchanges` |  |
| `POST` | `/api/dcrdex/init` | Rate limit 5 per second |
| `POST` | `/api/dcrdex/lock` |  |
| `POST` | `/api/dcrdex/maxbuy` |  |
| `POST` | `/api/dcrdex/maxsell` |  |
| `GET` | `/api/dcrdex/mm/archivedruns` |  |
| `GET` | `/api/dcrdex/mm/availablebalances` |  |
| `POST` | `/api/dcrdex/mm/cexconfig` |  |
| `POST` | `/api/dcrdex/mm/config` |  |
| `POST` | `/api/dcrdex/mm/config/remove` |  |
| `GET` | `/api/dcrdex/mm/marketreport` |  |
| `GET` | `/api/dcrdex/mm/runlogs` |  |
| `POST` | `/api/dcrdex/mm/running/config` |  |
| `POST` | `/api/dcrdex/mm/running/inventory` |  |
| `POST` | `/api/dcrdex/mm/start` |  |
| `GET` | `/api/dcrdex/mm/status` |  |
| `POST` | `/api/dcrdex/mm/stop` |  |
| `GET` | `/api/dcrdex/myorders` |  |
| `GET` | `/api/dcrdex/notifications` |  |
| `GET` | `/api/dcrdex/notify` | WebSocket |
| `POST` | `/api/dcrdex/order` |  |
| `POST` | `/api/dcrdex/order/accelerate` |  |
| `POST` | `/api/dcrdex/order/acceleration-estimate` |  |
| `POST` | `/api/dcrdex/order/preaccelerate` |  |
| `POST` | `/api/dcrdex/orders` |  |
| `POST` | `/api/dcrdex/postbond` |  |
| `GET` | `/api/dcrdex/postbond/status` |  |
| `POST` | `/api/dcrdex/preorder` |  |
| `GET` | `/api/dcrdex/rates` |  |
| `POST` | `/api/dcrdex/seed` |  |
| `POST` | `/api/dcrdex/seed/backed-up` |  |
| `GET` | `/api/dcrdex/status` |  |
| `POST` | `/api/dcrdex/trade` |  |
| `POST` | `/api/dcrdex/unlock` | Rate limit 5 per second |
| `GET` | `/api/dcrdex/wallet` |  |
| `POST` | `/api/dcrdex/wallet` |  |
| `GET` | `/api/dcrdex/wallet/address-used` |  |
| `POST` | `/api/dcrdex/wallet/close` |  |
| `POST` | `/api/dcrdex/wallet/create` |  |
| `POST` | `/api/dcrdex/wallet/new-address` |  |
| `POST` | `/api/dcrdex/wallet/open` |  |
| `DELETE` | `/api/dcrdex/wallet/peers` |  |
| `GET` | `/api/dcrdex/wallet/peers` |  |
| `POST` | `/api/dcrdex/wallet/peers` |  |
| `POST` | `/api/dcrdex/wallet/rescan` | Rate limit 1 per 60s |
| `POST` | `/api/dcrdex/wallet/send` |  |
| `POST` | `/api/dcrdex/wallet/toggle` |  |
| `POST` | `/api/dcrdex/wallet/txfee` |  |
| `GET` | `/api/dcrdex/wallet/txs` |  |
| `GET` | `/api/dcrdex/wallets` |  |
| `GET` | `/api/dcrdex/ws` | WebSocket |

### Bison Relay (138 routes)

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/br/avatar` |  |
| `GET` | `/api/br/backup` | file download |
| `POST` | `/api/br/backup/prepare` |  |
| `POST` | `/api/br/backup/restore` |  |
| `GET` | `/api/br/backup/status` |  |
| `GET`, `POST` | `/api/br/connection` |  |
| `GET` | `/api/br/contacts` |  |
| `POST` | `/api/br/contacts/accept-suggestion` |  |
| `POST` | `/api/br/contacts/block` |  |
| `GET` | `/api/br/contacts/blocked` |  |
| `POST` | `/api/br/contacts/fetch-post` |  |
| `GET`, `POST` | `/api/br/contacts/groups` |  |
| `POST` | `/api/br/contacts/groups/assign` |  |
| `POST` | `/api/br/contacts/groups/settings` |  |
| `POST` | `/api/br/contacts/handshake` |  |
| `POST` | `/api/br/contacts/ignore` |  |
| `POST` | `/api/br/contacts/kx-reset` |  |
| `POST` | `/api/br/contacts/list-content` |  |
| `POST` | `/api/br/contacts/list-posts` |  |
| `POST` | `/api/br/contacts/rename` |  |
| `POST` | `/api/br/contacts/reset-all` |  |
| `POST` | `/api/br/contacts/subscribe-posts` |  |
| `POST` | `/api/br/contacts/suggest-kx` |  |
| `POST` | `/api/br/contacts/tip` |  |
| `POST` | `/api/br/contacts/trans-reset` |  |
| `POST` | `/api/br/contacts/unblock` |  |
| `POST` | `/api/br/contacts/unsubscribe-posts` |  |
| `GET` | `/api/br/content/file` | peer bytes |
| `POST` | `/api/br/content/get` |  |
| `GET` | `/api/br/downloads/{contact}` |  |
| `GET` | `/api/br/downloads/{contact}/{filename}` | peer bytes |
| `GET` | `/api/br/embeds/{contact}/{filename}` | peer bytes |
| `GET` | `/api/br/events` | WebSocket |
| `POST` | `/api/br/files/add` |  |
| `GET` | `/api/br/files/downloads` |  |
| `POST` | `/api/br/files/downloads/cancel` |  |
| `POST` | `/api/br/files/downloads/delete` |  |
| `POST` | `/api/br/files/send` |  |
| `POST` | `/api/br/files/shared/remove` |  |
| `GET`, `POST` | `/api/br/filters` |  |
| `POST` | `/api/br/filters/delete` |  |
| `GET` | `/api/br/gc` |  |
| `POST` | `/api/br/gc/create` |  |
| `GET` | `/api/br/gc/invites` |  |
| `POST` | `/api/br/gc/invites/accept` |  |
| `GET` | `/api/br/gc/{gcid}` |  |
| `POST` | `/api/br/gc/{gcid}/admins` |  |
| `POST` | `/api/br/gc/{gcid}/alias` |  |
| `POST` | `/api/br/gc/{gcid}/block` |  |
| `GET` | `/api/br/gc/{gcid}/history` |  |
| `POST` | `/api/br/gc/{gcid}/history/clear` |  |
| `POST` | `/api/br/gc/{gcid}/invite` |  |
| `POST` | `/api/br/gc/{gcid}/kick` |  |
| `POST` | `/api/br/gc/{gcid}/kill` |  |
| `POST` | `/api/br/gc/{gcid}/message` |  |
| `POST` | `/api/br/gc/{gcid}/owner` |  |
| `POST` | `/api/br/gc/{gcid}/part` |  |
| `POST` | `/api/br/gc/{gcid}/resend-list` |  |
| `POST` | `/api/br/gc/{gcid}/unblock` |  |
| `POST` | `/api/br/gc/{gcid}/upgrade` |  |
| `GET` | `/api/br/identity` |  |
| `POST` | `/api/br/invites/accept` |  |
| `POST` | `/api/br/invites/write` |  |
| `POST` | `/api/br/join-decred-pulse` |  |
| `GET` | `/api/br/kx/list` |  |
| `GET`, `POST` | `/api/br/kx/mediateids` |  |
| `GET` | `/api/br/kx/searches` |  |
| `GET` | `/api/br/mcp/pending` | App password |
| `POST` | `/api/br/mcp/pending/resolve` | App password |
| `GET`, `POST` | `/api/br/mcp/settings` | App password |
| `GET` | `/api/br/mcp/spend` | App password |
| `GET` | `/api/br/messages` |  |
| `POST` | `/api/br/messages/clear` |  |
| `POST` | `/api/br/notifications/clear` |  |
| `POST` | `/api/br/notifications/delete` |  |
| `GET` | `/api/br/notifications/recent` |  |
| `POST` | `/api/br/pages/fetch` |  |
| `GET` | `/api/br/pages/local` |  |
| `POST` | `/api/br/pages/local/delete` |  |
| `GET` | `/api/br/pages/local/file` |  |
| `POST` | `/api/br/pages/local/save` |  |
| `POST` | `/api/br/pages/render` |  |
| `GET` | `/api/br/payments/tips` |  |
| `GET` | `/api/br/payments/tips/running` |  |
| `POST` | `/api/br/pm` |  |
| `GET` | `/api/br/posts` |  |
| `GET` | `/api/br/posts/body` |  |
| `POST` | `/api/br/posts/comment` |  |
| `GET` | `/api/br/posts/comment-receivereceipts` |  |
| `GET` | `/api/br/posts/comments` |  |
| `GET` | `/api/br/posts/embed-data` | peer bytes |
| `POST` | `/api/br/posts/heart` |  |
| `GET` | `/api/br/posts/hearts` |  |
| `POST` | `/api/br/posts/new` |  |
| `GET` | `/api/br/posts/receivereceipts` |  |
| `POST` | `/api/br/posts/relay` |  |
| `POST` | `/api/br/posts/render` |  |
| `POST` | `/api/br/posts/subscribe-all` |  |
| `GET` | `/api/br/rates` |  |
| `GET` | `/api/br/rtdt/sessions` |  |
| `POST` | `/api/br/rtdt/sessions/create` |  |
| `POST` | `/api/br/rtdt/sessions/create-instant` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/accept` |  |
| `GET` | `/api/br/rtdt/sessions/{rv}/audio` | WebSocket |
| `POST` | `/api/br/rtdt/sessions/{rv}/chat` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/dissolve` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/invite` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/join` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/kick` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/leave` |  |
| `GET` | `/api/br/rtdt/sessions/{rv}/messages` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/remove` |  |
| `POST` | `/api/br/rtdt/sessions/{rv}/rotate-cookies` |  |
| `GET`, `POST` | `/api/br/settings/behavior` |  |
| `POST` | `/api/br/setup` | Rate limit 5 per second |
| `GET` | `/api/br/shared-files` |  |
| `GET` | `/api/br/stats/contacts` |  |
| `GET` | `/api/br/stats/network` |  |
| `GET` | `/api/br/stats/overview` |  |
| `GET` | `/api/br/stats/payments` |  |
| `POST` | `/api/br/stats/payments/clear` |  |
| `GET` | `/api/br/stats/posts` |  |
| `GET` | `/api/br/status` |  |
| `POST` | `/api/br/store/files/delete` |  |
| `GET` | `/api/br/store/files/get` | peer bytes |
| `GET` | `/api/br/store/files/list` |  |
| `POST` | `/api/br/store/files/upload` |  |
| `GET`, `POST` | `/api/br/store/mode` |  |
| `GET` | `/api/br/store/orders` |  |
| `POST` | `/api/br/store/orders/comment` |  |
| `POST` | `/api/br/store/orders/status` |  |
| `GET`, `POST` | `/api/br/store/products` |  |
| `POST` | `/api/br/store/products/delete` |  |
| `GET` | `/api/br/store/templates` |  |
| `POST` | `/api/br/store/templates/delete` |  |
| `GET` | `/api/br/store/templates/file` |  |
| `POST` | `/api/br/store/templates/save` |  |
| `GET` | `/api/br/version` |  |

### Timestamp (dcrtime) (12 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/timestamp/export` | file download |
| `GET` | `/api/timestamp/records` |  |
| `POST` | `/api/timestamp/records` |  |
| `DELETE` | `/api/timestamp/records/{digest}` |  |
| `GET` | `/api/timestamp/records/{digest}` |  |
| `PATCH` | `/api/timestamp/records/{digest}` |  |
| `GET` | `/api/timestamp/records/{digest}/proof` | file download |
| `POST` | `/api/timestamp/records/{digest}/retry` |  |
| `POST` | `/api/timestamp/refresh` | Rate limit 1 per 30s |
| `GET` | `/api/timestamp/status` |  |
| `POST` | `/api/timestamp/validate` |  |
| `POST` | `/api/timestamp/verify` |  |

### Alerts (6 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/alerts` |  |
| `POST` | `/api/alerts/read-all` |  |
| `GET` | `/api/alerts/settings` |  |
| `POST` | `/api/alerts/settings` |  |
| `GET` | `/api/alerts/summary` |  |
| `POST` | `/api/alerts/{id}/read` |  |

### Settings (13 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/settings/mcp` | App password |
| `POST` | `/api/settings/mcp/agents/{id}/domains` | App password |
| `DELETE` | `/api/settings/mcp/agents/{id}/grant` | App password |
| `POST` | `/api/settings/mcp/agents/{id}/grant` | App password |
| `POST` | `/api/settings/mcp/agents/{id}/ips` | App password |
| `POST` | `/api/settings/mcp/agents/{id}/unblock` | App password |
| `GET` | `/api/settings/mcp/audit/export` | file download, App password |
| `POST` | `/api/settings/mcp/enable` | App password |
| `POST` | `/api/settings/mcp/freeze-all` | App password |
| `POST` | `/api/settings/mcp/logging` | App password |
| `POST` | `/api/settings/mcp/notify` | App password |
| `POST` | `/api/settings/mcp/tokens` | App password |
| `DELETE` | `/api/settings/mcp/tokens/{id}` | App password |

### Tor (5 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/tor` |  |
| `POST` | `/api/tor` |  |
| `GET` | `/api/tor/control` |  |
| `POST` | `/api/tor/newidentity` |  |
| `GET` | `/api/tor/status` |  |

### Themes (2 routes)

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/themes` |  |
| `POST` | `/api/themes` |  |

### Auth (app-password gate) (7 routes)

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/auth/change` | Rate limit 5 per second |
| `POST` | `/api/auth/disable` | Rate limit 5 per second |
| `POST` | `/api/auth/login` | Rate limit 5 per second |
| `POST` | `/api/auth/logout` |  |
| `POST` | `/api/auth/setup` |  |
| `POST` | `/api/auth/skip-setup` |  |
| `GET` | `/api/auth/status` |  |

---

## Error Handling

### Common Error Responses

**RPC Not Connected**:
```json
{
  "error": "Node RPC client not initialized"
}
```
Status: `503`

**Invalid Request**:
```json
{
  "error": "Invalid request body"
}
```
Status: `400`

**Server Error**:
```json
{
  "error": "Failed to fetch node status: connection refused"
}
```
Status: `500`

### Error Handling Best Practices

1. **Check status codes**: Always verify HTTP status
2. **Parse error messages**: Use `error` field for user feedback
3. **Implement retries**: For `503` errors, retry with backoff
4. **Handle timeouts**: Set appropriate request timeouts
5. **Log errors**: Log full error response for debugging

---

## Rate Limiting

Most endpoints are not rate limited (the dashboard is single-user). A token-bucket limiter is applied per-route to a handful of expensive or daemon-cycling operations; exceeding the allowance returns `429 Too Many Requests`:

- `POST /api/auth/login`: 5 / second
- `POST /api/auth/change` and `POST /api/auth/disable`: 5 / second, shared between the two
- `POST /api/wallet/open`, `/api/wallet/ln/unlock`, `/api/br/setup`, `/api/dcrdex/init`, `/api/dcrdex/unlock`: 5 / second, shared across all five, because each costs a daemon a key derivation
- `POST /api/wallets/select`, `/create`, `/rename`, `/delete`: 1 / 5 seconds each
- `POST /api/dcrdex/discover-account`: 1 / 10 seconds
- `POST /api/wallet/importxpub`: 1 / 30 seconds
- `POST /api/wallet/settings/discover-addresses`: 1 / 30 seconds
- `POST /api/wallet/staking/sync-failed-vsp-tickets`: 1 / 30 seconds
- `POST /api/wallet/staking/process-unmanaged-vsp-tickets`: 1 / 30 seconds
- `POST /api/timestamp/refresh`: 1 / 30 seconds
- `POST /api/wallet/rescan`: 1 / 60 seconds
- `POST /api/dcrdex/wallet/rescan`: 1 / 60 seconds
- `POST /api/treasury/scan-history`: 1 / 60 seconds

---

## Security Model

- **Same-origin protected.** State-changing requests must originate from the dashboard's own host; cross-origin POST/PUT/PATCH/DELETE are rejected with `403`. WebSocket upgrades enforce the same origin check.
- **Optional app-password gate.** When enabled, every `/api` route requires a signed `dcrpulse_session` cookie (see Authentication).
- **Request-body cap.** JSON bodies on state-changing methods are limited to 1 MiB. The middleware skips `multipart/*`, which no route currently uses.
- **Hardening headers.** Every response carries `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: no-referrer`, and a restrictive `Permissions-Policy`.
- **Credentials stay server-side.** RPC credentials for the daemons are read from environment variables and never exposed to the frontend.

### Deployment notes
- Behind a reverse proxy (Umbrel, CasaOS), set `TRUSTED_PROXY=true` so `X-Forwarded-Host` / `X-Forwarded-Proto` are honored for the same-origin check and for secure-cookie detection.
- Terminate TLS at the proxy so session cookies are issued with the `Secure` flag.
- Enable the app-password gate if the dashboard is reachable beyond localhost.

---

## Testing Endpoints

### Using `curl`

**Health Check**:
```bash
curl http://localhost:8080/api/health
```

**Dashboard Data**:
```bash
curl http://localhost:8080/api/dashboard
```

**Import Xpub**:
```bash
curl -X POST http://localhost:8080/api/wallet/importxpub \
  -H "Content-Type: application/json" \
  -H "Origin: http://localhost:8080" \
  -d '{
    "xpub": "dpubZF...",
    "accountName": "trezor",
    "accountIndex": 0,
    "rescan": true
  }'
```

**Wallet Transactions**:
```bash
curl "http://localhost:8080/api/wallet/transactions?count=10&from=0"
```

### Using JavaScript/TypeScript

See [`dashboard/web/src/services/api.ts`](../../dashboard/web/src/services/api.ts) for the dashboard's own integration code.

**Example** (using axios). Because the API is same-origin with the UI, use a relative base URL so the same code works in development and behind a proxy:
```typescript
import axios from 'axios';

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
  withCredentials: true, // send the session cookie when the app-password gate is on
});

// Get dashboard data
const dashboard = await api.get('/dashboard');

// Import xpub
const result = await api.post('/wallet/importxpub', {
  xpub: 'dpubZF...',
  accountName: 'trezor',
  accountIndex: 0,
  rescan: true,
});

// Get transactions
const txHistory = await api.get('/wallet/transactions', {
  params: { count: 50, from: 0 }
});
```

---

## Related Documentation

**Per-feature guides** (each backs one of the route groups above):

- **[Node Dashboard](../features/node-dashboard.md)** - Node, blockchain, and network status
- **[Wallet Dashboard](../features/wallet-dashboard.md)** - Balances, accounts, transactions
- **[Multi-Wallet](../features/multi-wallet.md)** - Multiple wallet stacks
- **[Shared Wallets](../features/shared-wallets.md)** - Multisig coordinated over Bison Relay
- **[Privacy / Mixer](../features/privacy-mixer.md)** - CoinJoin mixer
- **[Staking Guide](../features/staking-guide.md)** - Tickets, VSPs, autobuyer
- **[Governance](../features/governance.md)** - Agendas, treasury, Politeia
- **[Explorer](../features/explorer.md)** - Block explorer
- **[Lightning](../features/lightning.md)** - Lightning Network (dcrlnd)
- **[DEX](../features/dex.md)** - DCRDEX trading (bisonw)
- **[Bison Relay](../features/bison-relay.md)** - Bison Relay messaging (brclientd)
- **[Timestamp](../features/timestamp.md)** - dcrtime file timestamping
- **[AI Agents (MCP)](../features/ai-agents-mcp.md)** - Agent tokens, domains, and spend grants
- **[Bison Relay MCP](../features/bison-relay-mcp.md)** - The Bison Relay MCP bridge
- **[Settings](../features/settings.md)** - Themes and dashboard settings

**Other references**:

- **[Architecture](../development/architecture.md)** - How the dashboard, daemons, and frontend fit together
- **[Wallet Operations](../guides/wallet-operations.md)** - Walkthroughs for common wallet tasks

---

## API Versioning

The API is currently unversioned (no `/api/v1` prefix). Routes are added and refined alongside the dashboard; this reference is kept in sync with the registered routes in `dashboard/cmd/dcrpulse/main.go`.

---

## FAQ

**Q: Do I need authentication to access the API?**
A: By default no - the API is same-origin with the dashboard UI and has no password. You can enable an optional dashboard-wide app password under `/api/auth/*`, after which every request needs a valid session cookie.

**Q: Can I call the API from another origin (a separate site or script)?**
A: Not for state-changing requests. POST/PUT/PATCH/DELETE must come from the dashboard's own origin or they are rejected with `403`. Read-only GET requests are not origin-checked. The dashboard serves its own frontend, so there is no CORS allow-list to widen.

**Q: How often should I poll the dashboard endpoint?**
A: Recommended interval: 30 seconds. Faster polling increases load. For live data, prefer the WebSocket/SSE streaming routes (paths ending in `/events`, `/stream`, or `/ws`) over tight polling.

**Q: What happens if a daemon is disconnected during a request?**
A: You'll receive a `503 Service Unavailable` error with details. Feature routes (Lightning, DEX, Bison Relay) also return `503` when their optional daemon is unavailable.

**Q: I got a 429 - why?**
A: A rate-limited route (see Rate Limiting) was called too frequently. Retry after the indicated interval.

---

**Need Help?** Check the [Troubleshooting Guide](../guides/troubleshooting.md) or the [Architecture](../development/architecture.md) overview.

