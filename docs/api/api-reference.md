# API Reference

API documentation for the Decred Pulse dashboard backend. Most endpoints use JSON for request and response bodies; a few stream over Server-Sent Events or WebSocket, and several serve or accept binary payloads (file embeds, downloads, backups, uploads).

This reference documents the routes that actually exist, grouped the same way they are registered in `dashboard/cmd/dcrpulse/main.go`. The Node and Wallet sections below are documented in detail; the remaining feature groups are summarized with a representative list of endpoints and a pointer to the matching feature doc.

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

Request bodies on state-changing methods are capped at 1 MiB (multipart uploads are exempt so file-attachment handlers can apply their own larger limit). A handful of expensive or daemon-cycling routes are additionally rate limited (see the per-group notes below) and return `429 Too Many Requests` when the allowance is exceeded.

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

Streaming routes (paths ending in `/events`, `/stream`, `*-events`, or `/ws`) upgrade to WebSocket or Server-Sent Events instead of returning a single JSON body; binary routes (file embeds/downloads, backup export, uploads) return or accept raw bytes.

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
  "timestamp": "2025-10-06T12:34:56Z"
}
```

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
  "node": {
    "version": 20006,
    "versionStr": "2.0.6",
    "protocolVersion": 8,
    "blocks": 1016401,
    "timeOffset": 0,
    "connections": 12,
    "proxy": "",
    "difficulty": 223847291.45,
    "testnet": false,
    "relayFee": 0.0001,
    "errors": ""
  },
  "blockchain": {
    "chain": "mainnet",
    "blocks": 1016401,
    "headers": 1016401,
    "bestBlockHash": "000000000000000000abc123...",
    "difficulty": 223847291.45,
    "verificationProgress": 1.0,
    "chainWork": "00000000000000000000000000abc...",
    "initialBlockDownload": false,
    "maxBlockSize": 393216,
    "deployments": {...}
  },
  "peers": [
    {
      "id": 1,
      "addr": "192.0.2.1:9108",
      "addrLocal": "10.0.0.1:54321",
      "services": "0000000000000005",
      "version": 20006,
      "subVer": "/dcrd:2.0.6/",
      "startingHeight": 1016350,
      "currentHeight": 1016401,
      "bytesReceived": 12345678,
      "bytesSent": 23456789,
      "connTime": 1696600000,
      "timeOffset": 0,
      "pingTime": 0.045,
      "inbound": false
    }
  ],
  "mempool": {
    "size": 18,
    "bytes": 16160
  },
  "supply": {
    "circulating": 15234567.89,
    "staked": 6123456.78,
    "mixed": 4567890.12
  },
  "lastUpdate": "2025-10-06T12:34:56.789Z"
}
```

**Status Codes**:
- `200`: Success
- `503`: RPC client not connected

---

### Node Status

Get current node information and sync status.

```http
GET /api/node/status
```

**Response**:
```json
{
  "version": 20006,
  "versionStr": "2.0.6",
  "protocolVersion": 8,
  "blocks": 1016401,
  "timeOffset": 0,
  "connections": 12,
  "proxy": "",
  "difficulty": 223847291.45,
  "testnet": false,
  "relayFee": 0.0001,
  "errors": ""
}
```

**Fields**:
- `version`: dcrd version number
- `versionStr`: Human-readable version
- `protocolVersion`: Network protocol version
- `blocks`: Current block height
- `timeOffset`: Time offset in seconds
- `connections`: Number of peer connections
- `difficulty`: Current mining difficulty
- `testnet`: `true` if testnet, `false` if mainnet
- `relayFee`: Minimum relay fee in DCR
- `errors`: Any error messages

**Status Codes**:
- `200`: Success
- `503`: Node RPC not connected

---

### Blockchain Information

Get detailed blockchain state and sync information.

```http
GET /api/blockchain/info
```

**Response**:
```json
{
  "chain": "mainnet",
  "blocks": 1016401,
  "headers": 1016401,
  "bestBlockHash": "000000000000000000abc123...",
  "difficulty": 223847291.45,
  "verificationProgress": 1.0,
  "chainWork": "00000000000000000000000000abc...",
  "initialBlockDownload": false,
  "maxBlockSize": 393216,
  "deployments": {
    "pos": {
      "status": "active",
      "since": 4096
    }
  }
}
```

**Fields**:
- `chain`: Network name ("mainnet", "testnet3")
- `blocks`: Current block height
- `headers`: Number of validated headers
- `bestBlockHash`: Hash of best block
- `difficulty`: Current PoW difficulty
- `verificationProgress`: Sync progress (0.0-1.0)
- `chainWork`: Accumulated chain work (hex)
- `initialBlockDownload`: `true` if still syncing
- `maxBlockSize`: Maximum block size in bytes
- `deployments`: Active consensus deployments

**Status Codes**:
- `200`: Success
- `503`: Node RPC not connected

---

### Network Peers

Get list of connected peers with statistics.

```http
GET /api/network/peers
```

**Response**:
```json
[
  {
    "id": 1,
    "addr": "192.0.2.1:9108",
    "addrLocal": "10.0.0.1:54321",
    "services": "0000000000000005",
    "version": 20006,
    "subVer": "/dcrd:2.0.6/",
    "startingHeight": 1016350,
    "currentHeight": 1016401,
    "bytesReceived": 12345678,
    "bytesSent": 23456789,
    "connTime": 1696600000,
    "timeOffset": 0,
    "pingTime": 0.045,
    "inbound": false
  }
]
```

**Peer Fields**:
- `id`: Peer ID number
- `addr`: Peer IP address and port
- `addrLocal`: Local address for this connection
- `services`: Supported services (hex)
- `version`: Peer's dcrd version
- `subVer`: Peer's user agent
- `startingHeight`: Peer's starting block height
- `currentHeight`: Peer's current block height
- `bytesReceived`: Total bytes received
- `bytesSent`: Total bytes sent
- `connTime`: Connection timestamp (Unix)
- `timeOffset`: Time offset in seconds
- `pingTime`: Ping latency in seconds
- `inbound`: `true` if inbound connection

**Status Codes**:
- `200`: Success
- `503`: Node RPC not connected

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
  "status": "connected",
  "synced": true,
  "unlocked": true,
  "message": "Wallet is connected and ready"
}
```

**Status Values**:
- `connected`: Wallet RPC connected and operational
- `syncing`: Wallet is syncing
- `locked`: Wallet is locked (encrypted)
- `no_wallet`: No wallet connected

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
    "status": "connected",
    "synced": true,
    "unlocked": true,
    "message": "Wallet operational"
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
      "spendable": 450.12345678,
      "immatureCoinbaseRewards": 0,
      "immatureStakeGeneration": 25.0,
      "lockedByTickets": 25.0,
      "votingAuthority": 0,
      "unconfirmed": 0
    },
    {
      "accountName": "mixed",
      "accountNumber": 0,
      "totalBalance": 734.44443334,
      "spendable": 550.0,
      "immatureCoinbaseRewards": 0,
      "immatureStakeGeneration": 0,
      "lockedByTickets": 184.44443334,
      "votingAuthority": 0,
      "unconfirmed": 0
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
  - `spendable`: Available for use
  - `immatureCoinbaseRewards`: Mining rewards awaiting maturity
  - `immatureStakeGeneration`: Voting rewards awaiting maturity
  - `lockedByTickets`: Funds in active tickets
  - `votingAuthority`: Delegated voting rights
  - `unconfirmed`: Pending transactions

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
  "gapLimit": 200
}
```

**Request Fields**:
- `xpub` (required): Extended public key starting with `dpub`
- `gapLimit` (required): Gap limit for address discovery (20-1000)

**Response**:
```json
{
  "status": "success",
  "message": "Xpub imported successfully. Wallet rescan started.",
  "account": "imported",
  "gapLimit": 200
}
```

**Status Codes**:
- `200`: Import successful, rescan started
- `400`: Invalid request body
- `500`: Import failed
- `503`: Wallet RPC not connected

**Note**: After import, wallet automatically begins rescanning. Monitor progress via `/api/wallet/sync-progress`.

---

### Rescan Wallet

Manually trigger wallet rescan to discover transactions.

```http
POST /api/wallet/rescan
```

**Request Body**: Empty (`{}`) or omit

**Response**:
```json
{
  "status": "success",
  "message": "Wallet rescan initiated"
}
```

**Status Codes**:
- `200`: Rescan started
- `500`: Rescan failed
- `503`: Wallet RPC not connected

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
  "progress": 68.5,
  "currentBlock": 1016234,
  "totalBlocks": 1016401,
  "message": "Rescanning blockchain for addresses..."
}
```

**Response (Complete)**:
```json
{
  "isRescanning": false,
  "progress": 100,
  "message": "Wallet fully synced"
}
```

**Fields**:
- `isRescanning`: `true` if actively rescanning
- `progress`: Percentage complete (0-100)
- `currentBlock`: Current block being scanned
- `totalBlocks`: Total blocks to scan
- `message`: Human-readable status message

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
| `GET` | `/api/treasury/mempool` | TSpends currently in the mempool |
| `GET` | `/api/treasury/votes/{txhash}/progress` | Vote-parsing progress for a TSpend |

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
| `GET`/`POST` | `/api/dcrdex/mm/*` | Market-maker status, market report, run logs, config, start/stop |
| `GET` | `/api/dcrdex/ws`, `/api/dcrdex/notify` | WebSocket live feeds (book/notifications) |

See [DEX](../features/dex.md).

---

## Bison Relay

Bison Relay messaging backed by the optional `brclientd` daemon, under `/api/br/*`. This is the largest group; it is organized by sub-area. Representative routes per area:

- **Status and identity**: `GET /api/br/version`, `GET /api/br/status`, `POST /api/br/setup`, `GET /api/br/identity`, `POST /api/br/avatar`, `GET`/`POST` `/api/br/connection`.
- **Backup and restore**: `GET /api/br/backup`, `POST /api/br/backup/prepare`, `GET /api/br/backup/status`, `POST /api/br/backup/restore`.
- **Messaging**: `GET /api/br/messages`, `POST /api/br/messages/clear`, `POST /api/br/pm`, `GET /api/br/events` (WebSocket event stream).
- **Contacts and KX**: `GET /api/br/contacts`, `POST /api/br/contacts/rename`, `.../block`, `.../unblock`, `.../ignore`, `.../kx-reset`, `.../handshake`, `.../tip`, contact groups under `/api/br/contacts/groups*`, and key-exchange listings under `/api/br/kx/*`.
- **Invites**: `POST /api/br/invites/write`, `POST /api/br/invites/accept`, `POST /api/br/join-dcrpulse`.
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
- `POST /api/wallets/select`, `/create`, `/rename`, `/delete`: 1 / 5 seconds each
- `POST /api/wallet/importxpub`: 1 / 30 seconds
- `POST /api/wallet/settings/discover-addresses`: 1 / 30 seconds
- `POST /api/wallet/rescan`: 1 / 60 seconds
- `POST /api/treasury/scan-history`: 1 / 60 seconds

---

## Security Model

- **Same-origin protected.** State-changing requests must originate from the dashboard's own host; cross-origin POST/PUT/PATCH/DELETE are rejected with `403`. WebSocket upgrades enforce the same origin check.
- **Optional app-password gate.** When enabled, every `/api` route requires a signed `dcrpulse_session` cookie (see Authentication).
- **Request-body cap.** JSON bodies on state-changing methods are limited to 1 MiB (multipart uploads exempt).
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
  -d '{
    "xpub": "dpubZF...",
    "gapLimit": 200
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
  gapLimit: 200,
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
- **[Privacy / Mixer](../features/privacy-mixer.md)** - CoinJoin mixer
- **[Staking Guide](../features/staking-guide.md)** - Tickets, VSPs, autobuyer
- **[Governance](../features/governance.md)** - Agendas, treasury, Politeia
- **[Explorer](../features/explorer.md)** - Block explorer
- **[Lightning](../features/lightning.md)** - Lightning Network (dcrlnd)
- **[DEX](../features/dex.md)** - DCRDEX trading (bisonw)
- **[Bison Relay](../features/bison-relay.md)** - Bison Relay messaging (brclientd)
- **[Timestamp](../features/timestamp.md)** - dcrtime file timestamping
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

