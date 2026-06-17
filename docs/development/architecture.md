# System Architecture

Technical overview of Decred Pulse architecture, component design, data flow, and integration patterns.

## High-Level Architecture

The React frontend is built to static files and embedded into the Go binary, so a
single "dashboard" service serves both the user interface and the JSON API on one
port (8080). It talks to a set of Decred daemons over their respective RPC
interfaces, all on a private Docker bridge network. An optional Tor proxy routes
outbound daemon traffic when enabled.

```
┌──────────────────────────────────────────────────────────────────┐
│                          User's Browser                          │
│                          (React SPA)                             │
└──────────────────────┬───────────────────────────────────────────┘
                       │ HTTP/JSON + WebSocket (same origin)
                       │
┌──────────────────────▼───────────────────────────────────────────┐
│                  dashboard (Go, Port 8080)                       │
│        Serves embedded React UI  +  /api JSON endpoints          │
│  ┌──────────────┬──────────────┬──────────────┬───────────────┐ │
│  │   Handlers   │   Services   │     RPC      │  Middleware   │ │
│  └──────────────┴──────────────┴──────────────┴───────────────┘ │
└───┬──────────┬──────────┬──────────┬──────────┬──────────────────┘
    │ JSON-RPC │ JSON-RPC │  gRPC    │  RPC     │  RPC
    │  + gRPC  │          │          │          │
┌───▼────┐ ┌───▼──────┐ ┌─▼──────┐ ┌─▼────────┐ ┌─▼──────────┐
│  dcrd  │ │dcrwallet │ │ dcrlnd │ │brclientd │ │  dcrdex    │
│ 9108/  │ │ 9110 RPC │ │ 10009  │ │7676 RPC  │ │ (bisonw)   │
│ 9109   │ │ 9111 grpc│ │ gRPC   │ │7677 stat │ │ 5757/5758  │
└───┬────┘ └──────────┘ └────────┘ └──────────┘ └────────────┘
    │
    │ Decred P2P Protocol (Port 9108)
    │
┌───▼─────────────────────┐        ┌─────────────────────────┐
│   Decred Network        │        │   tor (SOCKS 9050,      │
│   (Global P2P)          │        │   control 9051)         │
└─────────────────────────┘        └─────────────────────────┘
```

All daemons depend, directly or indirectly, on dcrd being healthy. dcrwallet
backs dcrlnd and dcrdex; brclientd builds on dcrlnd. The dashboard reaches every
daemon by its service name on the bridge network.

---

## Component Overview

### Frontend (React + TypeScript)

**Purpose**: User interface for monitoring and managing a Decred node, wallet,
Lightning, Bison Relay, and DCRDEX

**Technology Stack**:
- **Framework**: React 18
- **Language**: TypeScript
- **Build Tool**: Vite
- **Styling**: Tailwind CSS (CSS-variable theming)
- **HTTP Client**: Axios (plus native fetch for some streams)
- **Icons**: Lucide React
- **Routing**: React Router DOM

**Architecture Pattern**: Component-based with a service layer

**Location**: `dashboard/web/src/`

**Build output**: `dashboard/web/dist/`. This directory is copied into the Go
build (`dashboard/cmd/dcrpulse/web/dist`) and embedded into the binary via
`go:embed`. There is no separate frontend container and no separate frontend
port in production - the Go binary serves the built SPA.

---

### Backend (Go API)

**Purpose**: Bridge between the frontend and the Decred RPC services; also serves
the embedded frontend

**Technology Stack**:
- **Language**: Go 1.26
- **Router**: Gorilla Mux
- **RPC Client**: dcrd rpcclient v8 (plus dcrwallet gRPC and dcrlnd gRPC)
- **WebSockets**: Gorilla WebSocket
- **Concurrency**: Goroutines and channels

**Architecture Pattern**: Layered architecture (Handlers -> Services -> RPC)

**Location**: `dashboard/internal/` (handlers, services, rpc, config, middleware,
auth, types, utils, timestamp, dexassets) and `dashboard/cmd/dcrpulse/main.go`
for the entry point. Shared helper packages live under `dashboard/pkg/`.

---

### dcrd (Decred Node)

**Purpose**: Full Decred blockchain node

**Functionality**:
- Blockchain synchronization
- P2P networking
- Block validation
- Transaction relay
- RPC interface (consumed by the dashboard, dcrwallet, and tooling)

**Built From**: Official dcrd source (GitHub)

**Version**: Pinned via the `DCRD_VERSION` build arg (default: `release-v2.1.5`)

---

### dcrwallet (Decred Wallet)

**Purpose**: HD wallet for managing DCR and tickets

**Functionality**:
- HD wallet management
- Transaction creation/signing
- Ticket purchasing/voting
- Address generation
- Balance tracking

**Built From**: Official dcrwallet source (GitHub)

**Interfaces**: JSON-RPC (9110) and gRPC (9111, used for streaming sync/rescan)

---

### dcrlnd (Lightning)

**Purpose**: Decred Lightning Network daemon

**Functionality**:
- Lightning channels (open/close, balances)
- Payments and invoices
- Channel and graph queries

**Interface**: gRPC (10009), backed by dcrwallet's gRPC. Powers the Lightning
section of the dashboard.

---

### brclientd (Bison Relay)

**Purpose**: Headless Bison Relay client daemon

**Functionality**:
- Encrypted messaging, group chats, and posts over the relay
- File transfers, paid content, pages/storefront
- Realtime voice/text (RTDT) session control

**Interfaces**: clientrpc (7676) and a status server (7677). The dashboard
prefers the status-server REST endpoints; brclientd builds on dcrlnd for LN
payments.

---

### dcrdex / bisonw (DEX)

**Purpose**: Decred DEX client (bisonw) run backend-only (no built-in web UI)

**Functionality**:
- Order placement and trade lifecycle
- Multi-asset wallets
- Market-maker bot management

**Interfaces**: RPC (5757) and a WebSocket feed (5758). The dashboard renders a
native full-width DEX experience against these.

---

### tor (Proxy)

**Purpose**: Optional Tor proxy for outbound daemon traffic and onion services

**Interfaces**: SOCKS proxy (9050) and control port (9051). Toggled at runtime;
when enabled, daemons route through it via the `TOR_PROXY_IP`/`TOR_PROXY_PORT`
environment variables.

---

## Data Flow

### Node Dashboard Data Flow

```
┌─────────────┐
│   Browser   │
└──────┬──────┘
       │ 1. GET /api/dashboard
       │
┌──────▼──────────────────────────────────────┐
│            dashboard (backend)              │
│                                             │
│  2. GetDashboardDataHandler                 │
│         │                                   │
│         ├─► 3. FetchNodeStatus()           │
│         │       └─► dcrd.GetInfo()         │
│         │                                   │
│         ├─► 4. FetchBlockchainInfo()       │
│         │       └─► dcrd.GetBlockchainInfo()│
│         │                                   │
│         ├─► 5. FetchNetworkPeers()         │
│         │       └─► dcrd.GetPeerInfo()     │
│         │                                   │
│         ├─► 6. FetchMempoolInfo()          │
│         │       └─► dcrd.GetRawMempool()   │
│         │                                   │
│         └─► 7. FetchSupplyInfo()           │
│                 └─► dcrd.GetCoinSupply()   │
│                                             │
│  8. Aggregate all data                     │
│  9. Return JSON response                   │
└──────┬──────────────────────────────────────┘
       │
       │ 10. Response (JSON)
       │
┌──────▼──────┐
│   Browser   │
│             │
│ 11. Update  │
│     UI      │
└─────────────┘
```

Node sync progress is additionally pushed over a WebSocket
(`/api/node/sync/stream`) and refreshed on dcrd block-connected notifications
rather than a fixed poll interval.

---

### Wallet Dashboard Data Flow

```
┌─────────────┐
│   Browser   │
└──────┬──────┘
       │ 1. GET /api/wallet/dashboard
       │
┌──────▼──────────────────────────────────────┐
│            dashboard (backend)              │
│                                             │
│  2. GetWalletDashboardHandler              │
│         │                                   │
│         ├─► 3. FetchWalletStatus()         │
│         │       └─► wallet.WalletInfo()    │
│         │                                   │
│         ├─► 4. FetchAccountInfo()          │
│         │       └─► wallet.GetBalance()    │
│         │                                   │
│         ├─► 5. FetchAllAccounts()          │
│         │       └─► wallet.GetBalance()    │
│         │                                   │
│         └─► 6. FetchWalletStakingInfo()    │
│                 ├─► wallet.GetStakeInfo()  │
│                 ├─► dcrd.GetStakeDifficulty()│
│                 └─► dcrd.EstimateStakeDiff()│
│                                             │
│  7. Aggregate all data                     │
│  8. Return JSON response                   │
└──────┬──────────────────────────────────────┘
       │
       │ 9. Response (JSON)
       │
┌──────▼──────┐
│   Browser   │
│             │
│ 10. Update  │
│     UI      │
└─────────────┘
```

---

### Xpub Import & Rescan Flow

```
┌─────────────┐
│   Browser   │
└──────┬──────┘
       │ 1. POST /api/wallet/importxpub
       │    Body: {xpub, gapLimit}
       │
┌──────▼──────────────────────────────────────┐
│            dashboard (backend)              │
│                                             │
│  2. ImportXpubHandler                       │
│         │                                   │
│         ├─► 3. Validate input              │
│         │                                   │
│         ├─► 4. wallet.ImportXpub()         │
│         │       (RPC to dcrwallet)          │
│         │                                   │
│         └─► 5. Trigger rescan              │
└──────┬──────────────────────────────────────┘
       │
       │ 6. Response: {status: "success"}
       │
┌──────▼──────┐
│   Browser   │
│             │
│ 7. Show     │
│    progress │
│             │
│ 8. Poll:    │
│    GET /api/│
│    wallet/  │
│    sync-    │
│    progress │
└──────┬──────┘
       │ (Every 2s)
       │
┌──────▼──────────────────────────────────────┐
│            dashboard (backend)              │
│                                             │
│  9. GetSyncProgressHandler                  │
│         │                                   │
│         ├─► 10. Read dcrwallet.log         │
│         │        (Last 500 lines)           │
│         │                                   │
│         ├─► 11. Parse rescan messages      │
│         │        Extract: progress %,       │
│         │        current block, total       │
│         │                                   │
│         ├─► 12. Check timestamp            │
│         │        (< 2 min = active)         │
│         │                                   │
│         └─► 13. Return progress            │
└──────┬──────────────────────────────────────┘
       │
       │ 14. Response: {isRescanning, progress, ...}
       │
┌──────▼──────┐
│   Browser   │
│             │
│ 15. Update  │
│     progress│
│     bar     │
│             │
│ 16. Repeat  │
│     until   │
│     complete│
└─────────────┘
```

A real-time gRPC-backed stream is also available
(`/api/wallet/grpc/stream-rescan`) for live rescan progress.

---

## Backend Layer Architecture

### Layer 1: Handlers (`dashboard/internal/handlers/`)

**Responsibility**: HTTP request handling and response formatting

**Files**: One file per feature area, for example `node.go`, `wallet.go`,
`lightning.go`, `bisonrelay*.go`, `dcrdex*.go`, `explorer.go`, `treasury.go`,
`auth.go`, `tor.go`, `timestamp.go`.

**Functions**:
- Parse HTTP requests
- Validate input
- Call service layer
- Format JSON responses (or upgrade to WebSocket/SSE for streams)
- Handle errors

**Example**:
```go
func GetDashboardDataHandler(w http.ResponseWriter, r *http.Request) {
    // Call service layer
    data, err := services.FetchDashboardData()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Return JSON
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}
```

---

### Layer 2: Services (`dashboard/internal/services/`)

**Responsibility**: Business logic and RPC orchestration

**Files**: Feature-oriented files (node, wallet, staking, governance, lightning,
bisonrelay, dcrdex, treasury, explorer, and the wallet sync supervisor among
others).

**Functions**:
- Make RPC/gRPC calls
- Process/transform data (atoms-to-DCR conversion happens here, not the frontend)
- Aggregate multiple RPC responses
- Handle concurrency and long-lived streams
- Error handling

**Example**:
```go
func FetchDashboardData() (*types.DashboardData, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Concurrent RPC calls
    results := make(chan result, 5)

    go fetchNodeStatus(ctx, results)
    go fetchBlockchainInfo(ctx, results)
    go fetchPeers(ctx, results)
    // ... more goroutines

    // Aggregate results
    return aggregateResults(results), nil
}
```

---

### Layer 3: Types (`dashboard/internal/types/`)

**Responsibility**: Data structure definitions

**Files**: `node.go`, `wallet.go`, `staking.go`, `governance.go`, `lightning.go`,
`explorer.go`, `treasury.go`, `settings.go`, `themes.go`, `tor.go`,
`wallet_loader.go`.

**Structures**:
```go
type DashboardData struct {
    NodeStatus     NodeStatus
    BlockchainInfo BlockchainInfo
    NetworkInfo    NetworkInfo
    Peers          []Peer
    MempoolInfo    MempoolInfo
    SupplyInfo     SupplyInfo
    StakingInfo    StakingInfo
    LastUpdate     time.Time
}
```

---

### Layer 4: RPC Clients (`dashboard/internal/rpc/`)

**Responsibility**: RPC/gRPC connection management for every daemon

**Files**: `client.go` (dcrd + dcrwallet), `dcrd_notify.go` (dcrd notification
websocket), `dcrlnd_client.go`, `brclientd_client.go`, `brclientd_ws.go`,
`dcrdex_client.go`, and `tlspin.go` (certificate pinning).

**Functions**:
- Initialize RPC/gRPC connections (lazily for daemons that generate certs on
  first run)
- Maintain connection state and reconnect
- Provide client instances to the service layer

**Global Variables**:
```go
var (
    NodeClient   *rpcclient.Client  // dcrd RPC
    WalletClient *rpcclient.Client  // dcrwallet RPC
)
```

Clients are initialized in `main.go` from environment variables, and re-dialed on
demand (for example after a wallet switch or once a locked daemon is unlocked).

---

### Layer 5: Config, Middleware, Auth, Utilities

**`dashboard/internal/config/`** - per-wallet and global configuration, data
paths, and the active-wallet pointer.

**`dashboard/internal/middleware/`** (`security.go`) - `SecurityHeaders` (CSP and
hardening headers), `RequireSameOrigin`, `LimitJSONBody`, and `RateLimit`.

**`dashboard/internal/auth/`** (`auth.go`) - the optional app-password gate and
its `RequireAuth` middleware (off by default; pass-through when disabled).

**`dashboard/internal/utils/`** (`formatters.go`) - formatting helpers (DCR
amounts, byte sizes, durations).

**`dashboard/internal/timestamp/`** - dcrtime timestamp worker. **`dexassets/`** -
generated DEX asset catalog. **`dashboard/pkg/`** - shared helpers (`bisonw`,
`exchangerate`).

---

## Frontend Architecture

### Component Structure

The frontend is a single-page app routed by React Router. Top-level pages live in
`dashboard/web/src/pages/`, reusable and feature components in
`dashboard/web/src/components/` (grouped by area: `wallet/`, `lightning/`,
`staking/`, `governance/`, `bisonrelay/`, `onchain/`, `settings/`, `auth/`), and
API integration in `dashboard/web/src/services/`.

```
src/
├── App.tsx                   # Routes + layout shell
├── main.tsx                  # Entry point
│
├── pages/                    # Page-level components, e.g.
│   ├── NodeDashboard.tsx    # Node monitoring (route: /)
│   ├── WalletDashboard.tsx  # Wallet hub (route: /wallet)
│   ├── AccountsPage.tsx     # Accounts
│   ├── StakingPage.tsx      # Staking (sub-tabs)
│   ├── GovernancePage.tsx   # Consensus/Treasury/Proposals
│   ├── LightningPage.tsx    # Lightning (sub-tabs)
│   ├── PrivacyPage.tsx      # Mixer / privacy
│   ├── TimestampPage.tsx    # dcrtime timestamping
│   ├── ExplorerLanding.tsx  # Block explorer
│   ├── GovernanceDashboard.tsx # Treasury (route: /treasury)
│   ├── DexPage.tsx          # DCRDEX (route: /dex)
│   └── SettingsPage.tsx     # Settings (sub-tabs)
│
├── components/               # Reusable + feature UI
│   ├── Header.tsx           # App header/navigation
│   ├── Footer.tsx           # Daemon version footer
│   ├── wallet/ lightning/ staking/ governance/
│   ├── bisonrelay/          # Bison Relay (route: /br)
│   ├── onchain/ settings/ auth/
│   └── ...
│
├── services/                 # API integration layer
│   ├── api.ts               # Axios client, core API functions
│   ├── lightningApi.ts dcrdexApi.ts bisonrelayApi.ts ...
│   └── themes/              # CSS-variable theming
│
└── index.css                 # Global styles (Tailwind)
```

Top-level routes (see `App.tsx`): `/` (Node), `/wallet` and its nested sections
(dashboard, accounts, staking, governance, lightning, privacy, timestamp,
transactions, settings, select), `/explorer/*`, `/treasury`, `/br` (Bison Relay),
and `/dex` (DCRDEX).

---

### State Management

**Pattern**: Component-level state with React hooks

**No global state library**: Keep it simple, use props and local/context state

**Data fetching**: `useEffect` + `useState` pattern, with WebSocket/SSE
subscriptions for live data (sync progress, chat, mixer, DEX feeds)

**Example**:
```typescript
const [data, setData] = useState<DashboardData | null>(null);
const [loading, setLoading] = useState(true);
const [error, setError] = useState<string | null>(null);

useEffect(() => {
  const fetchData = async () => {
    try {
      const response = await getDashboardData();
      setData(response);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  fetchData();
  const interval = setInterval(fetchData, 30000); // Auto-refresh
  return () => clearInterval(interval);
}, []);
```

---

### API Service Layer

**File**: `dashboard/web/src/services/api.ts`

**Purpose**: Centralized API communication

**Pattern**: Axios instance with typed responses. The base URL is the relative
path `/api` (same origin as the served SPA), and requests send credentials so the
optional app-password session cookie is included.

**Example**:
```typescript
const api = axios.create({
  baseURL: '/api',
  timeout: 25000,
  withCredentials: true, // send the app-password session cookie (same-origin)
});

export const getDashboardData = async (): Promise<DashboardData> => {
  const response = await api.get<DashboardData>('/dashboard');
  return response.data;
};

export const importXpub = async (xpub: string, gapLimit: number) => {
  const response = await api.post('/wallet/importxpub', {
    xpub,
    gapLimit,
  });
  return response.data;
};
```

During local development the Vite dev server (port 3000) proxies `/api` to the Go
backend on `localhost:8080`, so the frontend still uses the same relative path.

---

## Communication Protocols

### Frontend <-> Backend

**Protocol**: HTTP/REST and WebSocket (live streams)

**Format**: JSON

**Method**: Axios / native fetch; Gorilla WebSocket for streams

**Endpoints**: `/api/*`, served from the same origin as the SPA

**Authentication**: Optional app-password gate (`RequireAuth`). Disabled by
default; when enabled, a signed HttpOnly session cookie protects every `/api`
route except the login handshake.

**Origin protection**: `RequireSameOrigin` rejects cross-origin state-changing
requests; WebSocket upgrades use the same host check.

---

### Backend <-> dcrd

**Protocol**: JSON-RPC over HTTPS (plus a notification WebSocket)

**Port**: 9109 (RPC); 9108 is dcrd's P2P port

**Authentication**: Username + Password (RPC credentials)

**TLS**: Self-signed certificate (shared via the app-data volume)

**Client**: `github.com/decred/dcrd/rpcclient/v8`

---

### Backend <-> dcrwallet

**Protocol**: JSON-RPC over HTTPS (9110) and gRPC (9111, for streaming)

**Authentication**: Username + Password (separate credentials)

**TLS**: Self-signed certificate

**Client**: `github.com/decred/dcrd/rpcclient/v8` (wallet mode) and a gRPC client

---

### Backend <-> dcrlnd / brclientd / dcrdex

**dcrlnd**: gRPC on 10009, authenticated with TLS cert + macaroon.

**brclientd**: clientrpc (7676) and status server (7677) over mutually
authenticated TLS; the dashboard prefers the status-server REST endpoints.

**dcrdex (bisonw)**: RPC on 5757 and a WebSocket feed on 5758, both over TLS with
credentials. Certificates are generated by each daemon on first run and pinned by
the dashboard (`rpc/tlspin.go`).

---

### dcrd <-> Decred Network

**Protocol**: Decred P2P wire protocol

**Port**: 9108 (P2P)

**Format**: Binary protocol messages

**Purpose**: Blockchain sync, transaction relay, block propagation

---

## Docker Architecture

### Container Orchestration

**Tool**: Docker Compose

**Network**: Bridge network (`decred-network`)

**Services**: `dcrd`, `dcrwallet`, `dcrlnd`, `brclientd`, `dcrdex`, `tor`, and
`dashboard`. The dashboard is the only service that publishes a user-facing port
(8080) and serves both the UI and the API.

**Volumes** (named, host-pathable via env):
- `app-data` - shared daemon data (dcrd chain, dcrwallet, dcrd rpc cert), mounted
  read-write by the daemons that own it and read-only where appropriate
- `dcrlnd-data` - dcrlnd state
- `brclientd-data` - Bison Relay state
- `dcrdex-data` - DEX (bisonw) state
- `dashboard-data` - dashboard config (themes, auth, settings)
- `tor-data` - Tor state and onion keys

dcrd's blockchain is the largest consumer of disk (the mainnet chain is about 30 GB and growing).

---

### Container Dependencies

```
                ┌──────────────┐
                │     tor      │ (depends_on: dcrd started)
                └──────────────┘

┌──────────────┐
│     dcrd     │ (root of the dependency tree; healthcheck-gated)
└──────┬───────┘
       │ service_healthy
   ┌───▼────────┐
   │ dcrwallet  │ (depends_on: dcrd healthy)
   └───┬────────┘
       │ service_healthy
   ┌───▼────────┐
   │  dcrlnd    │ (depends_on: dcrwallet healthy)
   └───┬────────┘
       │ service_started
   ┌───▼────────┐
   │ brclientd  │ (depends_on: dcrlnd started)
   └────────────┘

┌──────────────┐
│   dcrdex     │ (no compose dependency; waits on wallet at runtime)
└──────────────┘

┌──────────────┐
│  dashboard   │ (depends_on: dcrd healthy AND dcrwallet healthy)
└──────────────┘
```

**Health Checks**:
- dcrd: RPC `getinfo` answers, OR the log shows a one-time database
  upgrade/reindex in progress (so the stack can come up during an upgrade)
- dcrwallet: a freshly written control state file (supervisor heartbeat)
- dashboard, dcrlnd, brclientd, dcrdex, tor: no compose-level healthcheck; the
  dashboard exposes `/api/health` for external probes

---

### Build Process

**Dashboard build (multi-stage)**:
1. **Frontend stage** (`node:18-alpine`): `npm install` then `npm run build` to
   produce `web/dist`
2. **Go stage** (`golang:1.26-alpine`): copy `web/dist` into
   `cmd/dcrpulse/web/dist`, then `go build` so the SPA is embedded in the binary
3. **Runtime stage** (`alpine:latest`): copy the single static binary; it serves
   both UI and API

**Daemon builds** (dcrd shown; dcrwallet/dcrlnd analogous):
```dockerfile
# Stage 1: Build from source
FROM golang:1.26-alpine AS builder
RUN git clone --depth 1 --branch ${DCRD_VERSION} https://github.com/decred/dcrd.git
WORKDIR /go/src/github.com/decred/dcrd
RUN go install .

# Stage 2: Runtime
FROM alpine:latest
COPY --from=builder /go/bin/dcrd /usr/local/bin/
# ... setup user, directories, entrypoint
```

---

## Security Architecture

### Credential Management

**RPC Credentials**:
- Supplied via the `.env` file (not committed) and environment variables
- Passed to containers; never exposed to the frontend
- Daemons use self-signed TLS certificates shared through the app-data volume

**Certificate Handling**:
- Generated on first run by each daemon
- Stored in Docker volumes and mounted (read-only where the dashboard only reads)
- The dashboard pins daemon certificates (`rpc/tlspin.go`) rather than skipping
  verification

---

### Application Gate

**Optional app password**: A single-password gate (`internal/auth`) can be
enabled to protect the whole API and UI behind a signed, HttpOnly session cookie.
It is off by default and fails open if its config cannot load, so a broken config
never locks the user out.

**Request hardening** (`internal/middleware`):
- `SecurityHeaders` sets a strict Content-Security-Policy plus
  `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and
  `Permissions-Policy`
- `RequireSameOrigin` blocks cross-origin state-changing requests
- `LimitJSONBody` caps request body sizes
- `RateLimit` throttles sensitive endpoints (login, wallet switch, rescans,
  treasury scans)

---

### Network Isolation

**Docker Network**:
- Private bridge network; containers communicate by service name
- Only the dashboard's port is published to the host by default

**Port Exposure**:
- 8080: dashboard UI + API (published to host)
- 9108: dcrd P2P (published; bind address configurable, for peers)
- 9109: dcrd RPC (bound to 127.0.0.1 on the host)
- 9110 / 9111: dcrwallet RPC / gRPC (bound to 127.0.0.1 on the host)
- 7677: brclientd status (bound to 127.0.0.1 on the host)
- Other daemon ports (dcrlnd 10009, brclientd 7676, dcrdex 5757/5758) stay
  internal to the bridge network

---

### Frontend Security

**No sensitive data in the frontend**:
- RPC credentials never reach the browser
- All daemon RPC calls are proxied through the backend
- A strict CSP and same-origin checks constrain what the SPA can do

---

## Performance Considerations

### Backend Optimization

**Concurrent RPC Calls**:
```go
// Fetch multiple data sources concurrently
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

var (
    nodeStatus NodeStatus
    blockchain BlockchainInfo
    peers      []Peer
)

g, gctx := errgroup.WithContext(ctx)

g.Go(func() error {
    nodeStatus, err = fetchNodeStatus(gctx)
    return err
})

g.Go(func() error {
    blockchain, err = fetchBlockchainInfo(gctx)
    return err
})

g.Go(func() error {
    peers, err = fetchPeers(gctx)
    return err
})

if err := g.Wait(); err != nil {
    return nil, err
}
```

**Streaming over polling**: Sync progress, rescans, chat, mixer events, and DEX
feeds are pushed over WebSocket/SSE so the UI updates without tight polling.

**Server timeouts**: `ReadHeaderTimeout` (15s) bounds header reads to defeat
Slowloris; read/write timeouts are intentionally left unset so long-lived streams
and large uploads are not cut off. `IdleTimeout` is 120s.

---

### Frontend Optimization

**Code Splitting**: Vite handles route/asset splitting

**Lazy Loading**: Heavy components loaded on demand

**Memoization**: `React.memo` for expensive renders

**Debouncing**: For search/input fields

---

### Database Optimization

**dcrd**:
- Configurable cache (`dbcache`)
- Transaction indexing optional (default build enables `--txindex`)

**dcrwallet**:
- Address gap-limit configuration
- Transaction indexing

---

## Monitoring & Observability

### Logging

**dashboard**: Standard library logging to stdout (captured by Docker)

**Daemons**: dcrd/dcrwallet/dcrlnd/brclientd/dcrdex write their own logs into
their data directories; some are surfaced in the UI (wallet logs, mixer events)

**Frontend**: Browser console + network inspector

---

### Health Checks

**dashboard**: `/api/health` endpoint

**dcrd**: RPC `getinfo` (also used by the compose healthcheck)

**dcrwallet**: supervisor state-file heartbeat (compose healthcheck)

**Docker**: Built-in healthcheck commands where defined

---

### Metrics

**Node Metrics**:
- Block height
- Peer count
- Mempool size
- Sync progress

**Wallet Metrics**:
- Balance
- Transaction count
- Ticket status
- Rescan progress

---

## Deployment Architecture

### Development

```
Local Machine
├── Frontend: http://localhost:3000 (Vite dev server, proxies /api -> :8080)
├── dashboard backend: http://localhost:8080 (Go binary)
├── dcrd, dcrwallet, dcrlnd, brclientd, dcrdex, tor: Docker containers
```

In this mode the frontend runs from the Vite dev server for fast iteration; the
Go binary still serves the embedded build when run on its own.

---

### Docker Compose (Recommended)

```
Host Machine
├── dcrd       (Docker container)
├── dcrwallet  (Docker container)
├── dcrlnd     (Docker container)
├── brclientd  (Docker container)
├── dcrdex     (Docker container)
├── tor        (Docker container)
└── dashboard  (Docker container, serves UI + API)
```

**Access**: `http://localhost:8080`

---

### Production (Future)

```
Server
├── Reverse proxy (TLS termination)
│   └── Proxy to the dashboard service
├── dashboard (UI + API)
└── dcrd / dcrwallet / dcrlnd / brclientd / dcrdex / tor
```

**Access**: `https://your-domain.com`

When behind a reverse proxy, set `TRUSTED_PROXY=true` so the same-origin checks
honor `X-Forwarded-Host`.

---

## Technology Choices

### Why Go for Backend?

- Native dcrd RPC client library
- Excellent concurrency (goroutines)
- Fast compilation and execution
- Strong typing
- Low memory footprint
- A single static binary that can embed the frontend

### Why React for Frontend?

- Component-based architecture
- Large ecosystem
- TypeScript support
- Fast development
- Virtual DOM performance

### Why Docker Compose?

- Simple orchestration
- Reproducible environments
- Easy dependency management
- Cross-platform compatibility
- Development/production parity

### Why Tailwind CSS?

- Utility-first approach
- Rapid prototyping
- Consistent design system (CSS-variable theming for the Themes feature)
- Small bundle size (purged)

---

## Related Documentation

- **[API Reference](../api/api-reference.md)** - API documentation
