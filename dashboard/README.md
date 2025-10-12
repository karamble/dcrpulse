# dcrpulse Dashboard

Modern dashboard for monitoring Decred nodes and wallets with embedded frontend.

## Overview

The dcrpulse dashboard is a single Go binary that serves both the API backend and frontend static files. The frontend is embedded into the binary at build time using Go's `embed` package.

## Architecture

- **Backend**: Go HTTP server with REST API endpoints
- **Frontend**: React + TypeScript SPA (embedded)
- **Build**: Multi-stage Docker build or local compilation

## Directory Structure

```
dashboard/
├── cmd/
│   └── dcrpulse/         # Main application entry point
│       └── main.go       # HTTP server with embedded files
├── internal/             # Private application code
│   ├── handlers/         # HTTP request handlers
│   ├── services/         # Business logic
│   ├── rpc/             # Decred RPC clients
│   ├── types/           # Type definitions
│   └── utils/           # Utility functions
├── web/                  # Frontend source code
│   ├── src/             # React/TypeScript source
│   ├── public/          # Static assets
│   ├── dist/            # Build output (gitignored)
│   └── *.config.js      # Build configuration
├── Dockerfile           # Multi-stage build
├── go.mod
└── config.example
```

## Development

### Prerequisites

- Go 1.21+
- Node.js 18+
- npm

### Running in Development Mode

Development mode allows hot reloading for the frontend:

**Terminal 1: Start Go backend**
```bash
cd dashboard
go run cmd/dcrpulse/main.go
```

**Terminal 2: Start Vite dev server**
```bash
cd dashboard/web
npm install  # First time only
npm run dev
```

The Vite dev server (http://localhost:3000) will proxy API requests to the Go backend (http://localhost:8080).

### Environment Variables

Copy `config.example` to `.env` and configure:

```bash
# dcrd RPC
DCRD_RPC_HOST=localhost
DCRD_RPC_PORT=9109
DCRD_RPC_USER=your_user
DCRD_RPC_PASS=your_password
DCRD_RPC_CERT=/path/to/rpc.cert

# dcrwallet RPC
DCRWALLET_RPC_HOST=localhost
DCRWALLET_RPC_PORT=9110
DCRWALLET_GRPC_PORT=9111
DCRWALLET_RPC_USER=your_user
DCRWALLET_RPC_PASS=your_password
DCRWALLET_RPC_CERT=/path/to/rpc.cert

# Server
PORT=8080
```

## Production Build

### Local Build

1. **Build frontend:**
```bash
cd dashboard/web
npm install
npm run build
```

This creates `dashboard/web/dist/` with the production frontend build.

2. **Copy dist to embed location:**
```bash
# From project root
cp -r dashboard/web/dist dashboard/cmd/dcrpulse/web/
```

Or symlink (for development):
```bash
ln -s ../../web/dist dashboard/cmd/dcrpulse/web/dist
```

3. **Build Go binary (outputs binary named 'dcrpulse'):**
```bash
cd dashboard
go build ./cmd/dcrpulse
```

4. **Run:**
```bash
./dcrpulse
```

The binary contains all frontend files and serves them at http://localhost:8080.

**Note**: The Go `embed` directive requires `cmd/dcrpulse/web/dist` to exist at compile time. A placeholder is included in the repo, but for production builds, you must copy or symlink the built frontend files to this location.

### Docker Build

```bash
docker build -t dcrpulse:latest ./dashboard
docker run -p 8080:8080 \
  -e DCRD_RPC_HOST=dcrd \
  -e DCRD_RPC_USER=user \
  -e DCRD_RPC_PASS=pass \
  dcrpulse:latest
```

Or use docker-compose from the project root:
```bash
docker-compose up dashboard
```

## API Endpoints

### Node Endpoints
- `GET /api/dashboard` - Complete dashboard data
- `GET /api/node/status` - Node status
- `GET /api/blockchain/info` - Blockchain information
- `GET /api/network/peers` - Network peers
- `POST /api/connect` - Configure RPC connection

### Wallet Endpoints
- `GET /api/wallet/status` - Wallet status
- `GET /api/wallet/dashboard` - Wallet dashboard data
- `GET /api/wallet/transactions` - Transaction history
- `POST /api/wallet/importxpub` - Import extended public key
- `GET /api/wallet/grpc/stream-rescan` - WebSocket rescan progress

### Explorer Endpoints
- `GET /api/explorer/search` - Search blocks/transactions/addresses
- `GET /api/explorer/blocks/recent` - Recent blocks
- `GET /api/explorer/blocks/{height}` - Block by height
- `GET /api/explorer/transactions/{txhash}` - Transaction details

### Treasury Endpoints
- `GET /api/treasury/info` - Treasury information
- `POST /api/treasury/scan-history` - Trigger TSpend scan
- `GET /api/treasury/scan-progress` - Scan progress

## Frontend Routes

- `/` - Node Dashboard
- `/wallet` - Wallet Dashboard
- `/explorer` - Block Explorer
- `/governance` - Treasury & Governance
- `/block/:height` - Block Details
- `/tx/:txhash` - Transaction Details
- `/address/:address` - Address View

## Features

- **Node Monitoring**: Real-time dcrd status, blockchain info, peer connections
- **Wallet Management**: Balance tracking, transaction history, xpub import
- **Block Explorer**: Search and browse blocks, transactions, and addresses
- **Treasury**: Monitor Decred treasury and TSpend proposals
- **Staking**: Ticket pool info, voting statistics
- **WebSocket Streaming**: Real-time rescan progress updates

## License

ISC License - see LICENSE file for details.

