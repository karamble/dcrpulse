# API Reference

Complete API documentation for Decred Pulse backend endpoints. All endpoints use JSON for request/response bodies.

## 📡 Base URL

```
http://localhost:8080/api
```

For production, replace with your backend server address.

---

## 🔐 Authentication

Currently, the API does not require authentication for client connections. However, the backend requires RPC credentials to connect to `dcrd` and `dcrwallet`, configured via environment variables.

**Security Note**: In production, consider implementing API authentication and HTTPS.

---

## 📊 Response Format

All successful responses return JSON with appropriate HTTP status codes:

- **200 OK**: Successful request
- **500 Internal Server Error**: Server-side error
- **503 Service Unavailable**: RPC client not initialized

Error responses:
```json
{
  "error": "Error description"
}
```

---

## 🎯 Node Endpoints

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

## 💼 Wallet Endpoints

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

## 🔧 Error Handling

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

## 📊 Rate Limiting

**Current Implementation**: No rate limiting

**Recommendations**:
- Implement rate limiting in production
- Suggested limits:
  - Dashboard endpoints: 1 request / 5 seconds
  - Status endpoints: 1 request / 2 seconds
  - Write operations: 1 request / minute

---

## 🔐 Security Best Practices

### Development
- ✅ Use environment variables for credentials
- ✅ Don't expose RPC credentials to frontend
- ✅ CORS configured for localhost only

### Production
- ⚠️ Implement HTTPS
- ⚠️ Add API authentication (JWT, API keys)
- ⚠️ Restrict CORS to specific origins
- ⚠️ Implement rate limiting
- ⚠️ Use firewall rules for backend access
- ⚠️ Monitor and log API usage

---

## 🧪 Testing Endpoints

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

See [`frontend/src/services/api.ts`](../../frontend/src/services/api.ts) for complete integration examples.

**Example** (using axios):
```typescript
import axios from 'axios';

const api = axios.create({
  baseURL: 'http://localhost:8080/api',
  timeout: 10000,
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

## 📚 Related Documentation

- **[Wallet Endpoints](wallet-endpoints.md)** - Detailed wallet endpoint documentation
- **[Node Endpoints](node-endpoints.md)** - Detailed node endpoint documentation
- **[Wallet Dashboard](../features/wallet-dashboard.md)** - Using the wallet dashboard
- **[Development Guide](../development/development-setup.md)** - Local development setup

---

## 🔄 API Versioning

**Current Version**: v1 (implicit)

**Future Plans**: Version prefixing (`/api/v1/`, `/api/v2/`) for breaking changes.

---

## ❓ FAQ

**Q: Do I need authentication to access the API?**  
A: No, currently the API has no authentication. In production, implement authentication.

**Q: Can I use the API from a browser?**  
A: Yes, CORS is enabled. In production, restrict CORS to specific origins.

**Q: How often should I poll the dashboard endpoint?**  
A: Recommended interval: 30 seconds. Faster polling increases server load.

**Q: What happens if RPC is disconnected during a request?**  
A: You'll receive a `503 Service Unavailable` error with details.

**Q: Can I host the API and frontend on different domains?**  
A: Yes, configure CORS appropriately in `backend/main.go`.

---

**Need Help?** Check the [Troubleshooting Guide](../guides/troubleshooting.md) or [Development Setup](../development/development-setup.md)

