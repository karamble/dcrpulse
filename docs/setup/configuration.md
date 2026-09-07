# Configuration Guide

Complete guide to configuring Decred Pulse, including environment variables, the multi-daemon Docker Compose stack, and per-service settings.

Decred Pulse is a multi-daemon stack, not just dcrd plus dcrwallet. A full deployment runs:

- **dcrd** - Decred full node
- **dcrwallet** - wallet daemon
- **dcrlnd** - Lightning Network node (Decred lnd)
- **brclientd** - headless Bison Relay client daemon
- **dcrdex** (bisonw) - DCRDEX trading client
- **tor** - Tor SOCKS proxy and onion service host
- **dashboard** - the web application (Go backend plus embedded frontend)

The dashboard is the only service that serves a web UI, and it listens on port **8080**. There is no separate frontend container.

## Configuration Files

```
dcrpulse/
├── .env                 # Main environment configuration (you create this)
├── env.example          # Canonical template; copy to .env
└── docker-compose.yml   # Docker services, ports, volumes, env wiring
```

Daemon flags are not held in `.conf` files. Each daemon is launched by an entrypoint or supervisor script under its service directory (`dcrd/docker-entrypoint.sh`, `dcrwallet/docker-entrypoint.sh`, `dcrlnd/docker-entrypoint.sh`, `brclientd/supervisor.sh`, `dcrdex/supervisor.sh`). Those scripts read the environment variables below and pass the corresponding command-line flags to the daemon.

---

## Environment Variables (.env)

The `.env` file holds credentials and service configuration. Only a small set of variables are required; everything else has a sensible default baked into `docker-compose.yml`.

### Initial Setup

```bash
# Create from example
cp env.example .env

# Edit with your values
nano .env
```

Most variables in `env.example` are commented out. Uncomment only what you need to change. The credential variables (`DCRD_RPC_*`, `DCRWALLET_RPC_*`) are uncommented in the template because you should set your own passwords.

---

## Required Variables

These are the variables you should set before first start. They are read by the daemons and by the dashboard backend.

### `DCRD_RPC_USER`
**Description**: Username for dcrd RPC. Shared by dcrd, dcrwallet, and the dashboard.

**Default**: `decred`

**Example**: `DCRD_RPC_USER=decred`

---

### `DCRD_RPC_PASS`
**Description**: Password for dcrd RPC. Shared by dcrd, dcrwallet, and the dashboard.

**Default**: `change_this_to_a_secure_password` (the compose fallback if unset is `decredpass`)

**Example**: `DCRD_RPC_PASS=MySecurePassword123`

**Recommendations**:
- Use a strong password (16+ characters)
- Avoid `$ \ " ' ` (may need shell escaping)
- Never commit `.env` to version control

**Generate a secure password**:
```bash
openssl rand -base64 32
```

---

### `DCRWALLET_RPC_USER`
**Description**: Username for dcrwallet JSON-RPC. Used by dcrwallet and the dashboard.

**Default**: `dcrwallet`

**Example**: `DCRWALLET_RPC_USER=dcrwallet`

---

### `DCRWALLET_RPC_PASS`
**Description**: Password for dcrwallet JSON-RPC. Used by dcrwallet and the dashboard.

**Default**: `change_this_to_a_secure_wallet_password` (the compose fallback if unset is `dcrwalletpass`)

**Example**: `DCRWALLET_RPC_PASS=AnotherSecurePass456`

**Recommendations**:
- Use a different password than dcrd
- Same strength guidance as `DCRD_RPC_PASS`

---

## dcrd Variables

### `DCRD_EXTRA_ARGS`
**Description**: Extra command-line arguments appended to dcrd at launch.

**Default**: `--txindex`

**Example**: `DCRD_EXTRA_ARGS=--txindex --debuglevel=debug`

**Notes**:
- `--txindex` enables full transaction lookup by hash (required for the block explorer).
- First startup with `--txindex` triggers a full blockchain reindex, which can take hours.
- The dcrd entrypoint always passes `--appdata=/app-data/dcrd` and the RPC flags from the compose `command:` block; do not duplicate those here.

**Common flags**:
- `--debuglevel=LEVEL` - trace, debug, info, warn, error, critical
- `--maxpeers=N` - maximum peer connections (dcrd default 125)

See [dcrd documentation](https://github.com/decred/dcrd/tree/master/docs).

---

### `DCRD_P2P_HOST_BIND`
**Description**: Host interface that dcrd's P2P port (9108) binds to. Standalone deployments only; the Umbrel build does not expose this port.

**Default**: `0.0.0.0` (accepts inbound connections on your public IP)

**Example**: `DCRD_P2P_HOST_BIND=127.0.0.1`

**Notes**:
- Set to `127.0.0.1` to refuse clearnet inbound and accept inbound only through the Tor onion service.
- The RPC port (9109) is always bound to `127.0.0.1` and is not affected by this variable.

---

### `DCRD_VERSION`
**Description**: dcrd version or branch to build from source. Used only when building locally (no image tag set).

**Default**: `release-v2.1.6` (the `docker-compose.yml` build-arg default)

**Values**:
- `release-v2.1.6` - pinned release used by the compose build
- A different release tag, branch name, or commit hash

**Example**: `DCRD_VERSION=release-v2.1.6`


---

### Network

There is no variable that puts dcrd on testnet. The entrypoint launches it with no network flag, so the node, the wallet, and the dashboard always run on mainnet. `LN_TESTNET` switches dcrlnd alone and is a development knob, not a stack-wide testnet mode.

---

## dcrwallet Variables

### `DCRWALLET_GAP_LIMIT`
**Description**: HD wallet address gap limit. Controls how many consecutive unused addresses the wallet monitors during discovery. Passed to dcrwallet as `--gaplimit`.

**Default**: `20` - dcrwallet's own BIP0044 default and what Decrediton ships.

**Example**: `DCRWALLET_GAP_LIMIT=20`

**Impact**:
- Raising the standing value permanently increases memory use and rescan time. Do not raise it to find funds after a restore: use Settings → Wallet → Address discovery instead, which scans once with a larger gap and rescans, leaving the standing limit at 20.

---

### `DCRWALLET_VERSION`
**Description**: dcrwallet version or branch to build from source. Used only when building locally.

**Default**: `release-v2.1.6` (the `docker-compose.yml` build-arg default)

**Example**: `DCRWALLET_VERSION=release-v2.1.6`


---

### dcrwallet RPC and chain wiring

These are set in `docker-compose.yml` and rarely need changing. They are listed here so the wiring is documented:

| Variable (in container) | Value | Purpose |
|---|---|---|
| `DCRWALLET_RPC_USER` | from `.env` | dcrwallet JSON-RPC user |
| `DCRWALLET_RPC_PASS` | from `.env` | dcrwallet JSON-RPC password |
| `DCRD_RPC_USER` | from `.env` | credentials dcrwallet uses to reach dcrd |
| `DCRD_RPC_PASS` | from `.env` | credentials dcrwallet uses to reach dcrd |
| `DCRD_RPC_HOST` | `dcrd` | dcrd service hostname on the Docker network |
| `DCRWALLET_GAP_LIMIT` | from `.env` | see above |

The entrypoint launches dcrwallet with `--rpclisten=0.0.0.0:9110` (JSON-RPC) and `--grpclisten=0.0.0.0:9111` (gRPC), connecting to `${DCRD_RPC_HOST}:9109`. The wallet shares dcrd's TLS certificate at `/app-data/dcrd/rpc.cert`.

---

## dcrlnd (Lightning) Variables

dcrlnd uses dcrwallet as its chain backend over gRPC and funds channels from a per-wallet "lightning" account.

### `LN_TESTNET`
**Description**: Run dcrlnd against testnet.

**Default**: `false`

**Example**: `LN_TESTNET=true`

---

### `DCRLND_VERSION`
**Description**: dcrlnd version or branch to build from source.

**Default**: `v0.8.1` (the `docker-compose.yml` build-arg default)

**Example**: `DCRLND_VERSION=v0.8.1`

---

### dcrlnd wiring (set in docker-compose.yml)

| Variable (in container) | Value | Purpose |
|---|---|---|
| `DCRWALLET_HOST` | `dcrwallet` | dcrwallet hostname for the gRPC chain backend |
| `DCRWALLET_GRPC_PORT` | `9111` | dcrwallet gRPC port |

dcrlnd serves its gRPC API on `10009` inside the Docker network only (no host port). The macaroon and TLS certificate are per-wallet and the dashboard resolves them in code, not from the environment: `/app-data/dcrlnd/admin.macaroon` and `/app-data/dcrlnd/tls.cert` for the default wallet, and `/app-data/dcrlnd/wallets/<name>/` for any other. The supervisor also supports `DCRLND_TLS_EXTRA_DOMAIN` (default `dcrlnd`) for an extra TLS SAN.

---

## brclientd (Bison Relay) Variables

brclientd is a headless Bison Relay client. It requires Lightning and idles until the active wallet's dcrlnd node is ready.

### `BRCLIENTD_DATA_DIR`
**Description**: Path where brclientd's appdata lives (clientdb, embeds, downloads, mTLS certs, logs). The dashboard reads embed/download files and the brclientd log from this path.

**Default**:
- Standalone (binary on host, no `--appdata`): `~/.brclientd`
- Docker/Umbrel: `docker-compose.yml` sets this to `/app-data/brclientd` to match the container's volume mount.

**Example**: `BRCLIENTD_DATA_DIR=~/.brclientd`

**Note**: override this only if you ran brclientd with a non-default `--appdata`.

---

### brclientd wiring (set in docker-compose.yml)

brclientd exposes its status server on `127.0.0.1:7677`, and the Bison Relay MCP bridge on `${MCP_BRIDGE_HOST:-127.0.0.1}:${MCP_BRIDGE_PORT:-8891}`. The dashboard connects with these values:

| Variable (in container) | Value | Purpose |
|---|---|---|
| `BRCLIENTD_HOST` | `brclientd` | brclientd hostname on the Docker network |
| `BRCLIENTD_PORT` | `7676` | clientrpc port (internal) |
| `BRCLIENTD_STATUS_PORT` | `7677` | status-server port the dashboard uses |
| `BRCLIENTD_DATA_DIR` | `/app-data/brclientd` | appdata path (see above) |

The `BRCLIENTD_IMAGE_TAG` build/pull tag defaults to `latest`.

---

### `MCP_BRIDGE_HOST` / `MCP_BRIDGE_PORT`
**Description**: Host interface and port the Bison Relay MCP bridge is published on, as `${MCP_BRIDGE_HOST:-127.0.0.1}:${MCP_BRIDGE_PORT:-8891}:8891`.

**Default**: `127.0.0.1` and `8891` - loopback only.

**Example**: `MCP_BRIDGE_HOST=0.0.0.0`

**Notes**:
- Set `MCP_BRIDGE_HOST=0.0.0.0` to reach it from another device. Like the agents MCP it is plain HTTP behind a bearer token, so prefer an SSH tunnel or a reverse proxy.
- This is only the host publish. The listener inside the container stays `0.0.0.0:8891`, set via `--mcp.mcplisten` in the supervisor, which overrides any `brclientd.conf` value.

---

## dcrdex (DCRDEX / bisonw) Variables

The dcrdex service runs bisonw in RPC mode for DCRDEX trading.

### `DCRDEX_RPC_USER`
**Description**: Username for the bisonw RPC server.

**Default**: `dcrdex`

**Example**: `DCRDEX_RPC_USER=dcrdex`

---

### `DCRDEX_RPC_PASS`
**Description**: Password for the bisonw RPC server.

**Default**: `dcrdexpass`

**Example**: `DCRDEX_RPC_PASS=MyDexPass789`

---

### dcrdex wiring (set in docker-compose.yml)

bisonw serves RPC on `5757` and a TLS web/WebSocket endpoint on `5758`, both inside the Docker network only (no host ports). The dashboard connects with:

| Variable (in dashboard) | Value | Purpose |
|---|---|---|
| `DCRDEX_RPC_HOST` | `dcrdex` | bisonw hostname on the Docker network |
| `DCRDEX_RPC_PORT` | `5757` | bisonw RPC port |
| `DCRDEX_RPC_USER` | from `.env` | bisonw RPC user |
| `DCRDEX_RPC_PASS` | from `.env` | bisonw RPC password |
| `DCRDEX_WS_PORT` | `5758` | bisonw web/WebSocket port |

The RPC and web TLS certificates are per-wallet and the dashboard resolves them in code, not from the environment: `/app-data/dcrdex/rpc.cert` and `/app-data/dcrdex/web.cert` for the default wallet, and `/app-data/dcrdex/wallets/<name>/` for any other.

The `DCRDEX_IMAGE_TAG` build/pull tag defaults to `latest`.

---

## Tor Variables

A Tor SOCKS proxy and onion-service host runs as the `tor` service. Tor is toggled at runtime from the dashboard (Settings), which writes a shared pointer file the daemon supervisors read. The proxy endpoint itself comes from environment variables that `docker-compose.yml` sets on every daemon:

| Variable | Value | Purpose |
|---|---|---|
| `TOR_PROXY_IP` | `tor` | Tor service hostname on the Docker network |
| `TOR_PROXY_PORT` | `9050` | Tor SOCKS port |
| `TOR_CONTROL_PORT` | `9051` | Tor control port (dashboard only) |

When Tor is enabled, each daemon routes its outbound traffic through `tor:9050`. dcrd can additionally publish an inbound onion service (see `DCRD_P2P_HOST_BIND`). The `TOR_IMAGE_TAG` build/pull tag defaults to `latest`.

---

## Dashboard Variables

The dashboard runs the Go backend and serves the embedded frontend on a single port.

### `PORT`
**Description**: Port the dashboard listens on inside the container.

**Default**: `8080` (set in `docker-compose.yml`, published to the host as `${DASHBOARD_HOST_BIND:-127.0.0.1}:${DASHBOARD_PORT:-8080}:8080`)

Access the UI at `http://127.0.0.1:8080` on the machine running the stack.

### `DASHBOARD_HOST_BIND` / `DASHBOARD_PORT`
**Description**: Host interface and port the UI is published on.

**Default**: `127.0.0.1` and `8080` - loopback only, so the UI is not reachable from the LAN.

Set `DASHBOARD_HOST_BIND=0.0.0.0` to reach the dashboard from another device, but enable the app password first: the API drives the wallet, speaks plain HTTP, and has no other gate. An SSH tunnel or a TLS reverse proxy is the better answer. The dashboard logs a warning at startup when it is published beyond loopback with no password set.

### `DASHBOARD_ALLOWED_HOSTS`
**Description**: Extra host names the dashboard answers to, comma separated. It already answers to IP addresses, `localhost`, single-label names (container and compose service names) and `.local`/`.onion` names, which covers direct, LAN, Umbrel and CasaOS access. A public domain has to be named here, otherwise the request is refused with `403 host not served by this dashboard`.

**Default**: unset

**Example**: `DASHBOARD_ALLOWED_HOSTS=dcrpulse.example.com`

**Notes**:
- Set this when you front the UI with a reverse proxy that passes the browser's `Host` through. This is the variable a reverse-proxy install fails without.
- A single `*` answers to anything; the dashboard logs a warning at startup when it is set that way.

---

### `DCRPULSE_LOG_LEVEL`
**Description**: Verbosity of the dashboard's own log, readable under Settings -> Logs and written to `/dashboard-data/logs/dcrpulse.log`. Either a level on its own, or a level followed by per-subsystem overrides.

**Default**: `info`

**Example**: `DCRPULSE_LOG_LEVEL=info,MSIG=debug`

**Values**:
- Levels: `trace`, `debug`, `info`, `warn`, `error`, `critical`, `off`
- Subsystems: `ALRT`, `BREL`, `DCRP`, `DEXC`, `GOVN`, `HTTP`, `LGHT`, `MCPS`, `MSIG`, `NODE`, `RPCC`, `SETT`, `STKE`, `TSTP`, `WLLT`

An unknown level or subsystem is reported at startup rather than ignored.

---

### `MCP_ENABLE`
**Description**: Seeds whether the dcrpulse agents MCP server comes up. It hands AI agents scoped, capped access to the wallet, so it is opted into rather than shipped on.

**Default**: `false`

**Example**: `MCP_ENABLE=true`

**Notes**:
- The toggle under Settings -> AI Agents is the normal way to turn it on. Once used it is remembered and overrides this variable, which then only seeds installs where the toggle has never been touched.
- The server does not start without an app password set, whatever this is set to, because the routes that mint tokens and widen an agent's authority are otherwise ungated.

---

### `MCP_AGENTS_HOST` / `MCP_AGENTS_PORT`
**Description**: Host interface and port the agents MCP server is published on, as `${MCP_AGENTS_HOST:-127.0.0.1}:${MCP_AGENTS_PORT:-8090}:8090`.

**Default**: `127.0.0.1` and `8090` - loopback only.

**Example**: `MCP_AGENTS_HOST=0.0.0.0`

**Notes**:
- Set `MCP_AGENTS_HOST=0.0.0.0` to let an AI agent on another LAN device reach it. The bearer token is the access gate and it crosses the LAN as plain HTTP, so prefer an SSH tunnel or a reverse proxy.
- This is only the host publish. Inside the container the listener binds on `MCP_BIND=0.0.0.0`, set in `docker-compose.yml`, and on port 8090, which is the `MCP_PORT` default compiled into the dashboard rather than a value the compose file sets.

---

### `AGENT_OUTBOX_DIR`
**Description**: Host directory the dashboard mounts read-only at `/agent-outbox` (set as `MCP_AGENT_OUTBOX_DIR` in `docker-compose.yml`). Files placed there can be sent to a Bison Relay contact by relative path with the `br_file_send_path` MCP tool, without base64-encoding through the agent.

**Default**: unset, which maps to the empty named volume `dcrpulse_agent-outbox` (the feature is present but unused).

**Example**: `AGENT_OUTBOX_DIR=/home/youruser/dcrpulse-agent-outbox`

**Note**: it is the only directory that tool may read, so an agent cannot reach other host files.

---

### Daemon RPC wiring

The dashboard receives the RPC host/port/user/pass/cert values for every daemon as environment variables (see the per-daemon wiring tables above). The full set in `docker-compose.yml` is:

- dcrd: `DCRD_RPC_HOST=dcrd`, `DCRD_RPC_PORT=9109`, `DCRD_RPC_USER`, `DCRD_RPC_PASS`, `DCRD_RPC_CERT=/app-data/dcrd/rpc.cert`
- dcrwallet: `DCRWALLET_RPC_HOST=dcrwallet`, `DCRWALLET_RPC_PORT=9110`, `DCRWALLET_GRPC_PORT=9111`, `DCRWALLET_RPC_USER`, `DCRWALLET_RPC_PASS`, `DCRWALLET_RPC_CERT=/app-data/dcrd/rpc.cert`
- dcrlnd: `DCRLND_HOST=dcrlnd`, `DCRLND_GRPC_PORT=10009` (the cert and macaroon are per-wallet paths resolved in code)
- brclientd: `BRCLIENTD_HOST=brclientd`, `BRCLIENTD_PORT=7676`, `BRCLIENTD_STATUS_PORT=7677`, `BRCLIENTD_DATA_DIR=/app-data/brclientd`
- dcrdex: the `DCRDEX_*` set listed above (the certs are per-wallet paths resolved in code)
- tor: `TOR_PROXY_IP=tor`, `TOR_PROXY_PORT=9050`, `TOR_CONTROL_PORT=9051`

The `DASHBOARD_IMAGE_TAG` build/pull tag defaults to `latest`.

---

## Image Tags and Storage Variables

### Image tags

Every service resolves to a published image, so an unedited checkout pulls all seven from the GitHub Container Registry (`ghcr.io/karamble/dcrpulse-*`) rather than building them. Each tag defaults to `latest`, the rolling tag published on each master push. Set one here to pin a service to a release:

```bash
#DCRD_IMAGE_TAG=latest
#DCRWALLET_IMAGE_TAG=latest
#DCRLND_IMAGE_TAG=latest
#BRCLIENTD_IMAGE_TAG=latest
#DCRDEX_IMAGE_TAG=latest
#TOR_IMAGE_TAG=latest
#DASHBOARD_IMAGE_TAG=latest
```

Run `docker compose pull` before `up` to move a `latest` deployment forward; compose keeps the image it already has otherwise. Building a service yourself overwrites the tag it resolves to here, so set the tag first (for example `BRCLIENTD_IMAGE_TAG=dev`) to keep a local build separate from the published image.

### `APP_DATA_DIR`
**Description**: Location of the shared application-data volume. For app-store integrations (Umbrel, Start9, CasaOS) this can point at a managed path.

**Default**: empty / `app-data`, which maps to the Docker named volume `dcrpulse_app-data`.

**Example**:
```bash
# Standalone (default)
#APP_DATA_DIR=app-data

# Umbrel
APP_DATA_DIR=/umbrel/app-data/<app-id>
```

### Per-service data directories

`docker-compose.yml` also defines override variables for each service's data volume. Leave them unset to use the default named volumes:

- `DASHBOARD_DATA_DIR` -> `dcrpulse_dashboard-data`
- `DCRLND_DATA_DIR` -> `dcrpulse_dcrlnd-data`
- `BRCLIENTD_DATA_DIR` -> `dcrpulse_brclientd-data`
- `DCRDEX_DATA_DIR` -> `dcrpulse_dcrdex-data`
- `TOR_DATA_DIR` -> `dcrpulse_tor-data`

---

## Example .env Files

**Minimal configuration** (set your own passwords, accept all other defaults):
```bash
DCRD_RPC_USER=decred
DCRD_RPC_PASS=MySecure123Pass

DCRWALLET_RPC_USER=dcrwallet
DCRWALLET_RPC_PASS=MyWalletPass456
```

**Production configuration**:
```bash
# Strong RPC credentials
DCRD_RPC_USER=dcrd_production
DCRD_RPC_PASS=$(openssl rand -base64 32)

DCRWALLET_RPC_USER=dcrwallet_production
DCRWALLET_RPC_PASS=$(openssl rand -base64 32)

# DCRDEX credentials
DCRDEX_RPC_USER=dex_production
DCRDEX_RPC_PASS=$(openssl rand -base64 32)

# Pin the published images to a release for reproducible deploys
DCRD_IMAGE_TAG=latest
DCRWALLET_IMAGE_TAG=latest
DASHBOARD_IMAGE_TAG=latest

# Standard gap limit and transaction indexing
DCRWALLET_GAP_LIMIT=20
DCRD_EXTRA_ARGS=--txindex
```

**dcrlnd on testnet** (a development knob; the node and wallet stay on mainnet):
```bash
LN_TESTNET=true
```

---

## Ports

`docker-compose.yml` publishes these ports to the host. Internal-only ports are reachable only between containers on the `decred-network` bridge.

| Service | Port | Bind | Protocol | Notes |
|---|---|---|---|---|
| dashboard | 8080 | `${DASHBOARD_HOST_BIND}` (default `127.0.0.1`) | HTTP | Web UI and API (the only externally served app) |
| dashboard | 8090 | `${MCP_AGENTS_HOST}` (default `127.0.0.1`) | HTTP | Agents MCP server; the listener runs only while MCP is enabled |
| dcrd | 9108 | `${DCRD_P2P_HOST_BIND}` (default `0.0.0.0`) | P2P | Peer connections |
| dcrd | 9109 | `127.0.0.1` | RPC | JSON-RPC (localhost only) |
| dcrwallet | 9110 | `127.0.0.1` | RPC | JSON-RPC (localhost only) |
| dcrwallet | 9111 | `127.0.0.1` | gRPC | gRPC (localhost only) |
| dcrlnd | 10009 | internal | gRPC | No host port |
| brclientd | 7677 | `127.0.0.1` | HTTP | Status server (localhost only) |
| brclientd | 7676 | internal | clientrpc | No host port |
| brclientd | 8891 | `${MCP_BRIDGE_HOST}` (default `127.0.0.1`) | HTTP | Bison Relay MCP bridge |
| dcrdex | 5757 | internal | RPC | bisonw RPC, no host port |
| dcrdex | 5758 | internal | HTTP/WS | bisonw web/WebSocket, no host port |
| tor | 9050 | internal | SOCKS | Tor proxy, no host port |
| tor | 9051 | internal | control | Tor control, no host port |

To change the exposed UI port, set `DASHBOARD_PORT` in `.env` rather than editing the mapping, so the loopback bind is preserved:
```
DASHBOARD_PORT=9000
```

---

## Volumes

`docker-compose.yml` defines named volumes (each overridable via the `*_DATA_DIR` variables above):

```yaml
volumes:
  app-data:        # Shared: dcrd chain (~tens of GB), dcrwallet, certs, control pointers
  dashboard-data:  # Dashboard state (themes, settings, audit trail)
  dcrlnd-data:     # Lightning node data and channel state
  brclientd-data:  # Bison Relay client data
  dcrdex-data:     # DCRDEX app seed, accounts, bonds
  tor-data:        # Tor data and onion-service keys
```

The `app-data` volume is mounted into most services. dcrd and dcrwallet write to it read-write; dcrlnd, brclientd, dcrdex, and the dashboard mount it (or parts of it) read-only and bind their own writable data on nested mount points.

```bash
# List volumes
docker volume ls | grep dcrpulse

# Inspect a volume
docker volume inspect dcrpulse_app-data
```

---

## Resource Limits and Health Checks

`docker-compose.yml` already sets per-service `deploy.resources` limits (for example dcrd is capped at 4G/2 CPUs, dcrwallet at 2G/1 CPU). Adjust these to your hardware.

dcrd and dcrwallet define health checks that other services depend on via `depends_on: condition: service_healthy`:

- **dcrd** is considered healthy when its RPC answers, or when its log shows a one-time database upgrade/reindex in progress (so the stack can come up and display "database upgrade in progress" instead of aborting).
- **dcrwallet** is healthy when its supervisor has refreshed `state.json` within the last 60 seconds.

---

## Configuration Validation

### Check what Compose resolved

```bash
# View your .env
cat .env

# See the fully resolved configuration
docker compose config

# Verify a container received the expected values
docker exec dcrpulse-dashboard env | grep -E 'DCRD_RPC|DCRWALLET_RPC|DCRDEX_RPC'
```

### Test RPC credentials

```bash
# dcrd RPC
docker exec dcrpulse-dcrd dcrctl \
  --rpcuser=$DCRD_RPC_USER \
  --rpcpass=$DCRD_RPC_PASS \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/app-data/dcrd/rpc.cert \
  getblockcount

# dcrwallet RPC
docker exec dcrpulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=$DCRWALLET_RPC_USER \
  --rpcpass=$DCRWALLET_RPC_PASS \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/app-data/dcrd/rpc.cert \
  walletinfo
```

---

## Troubleshooting Configuration

### Credentials not working

**Problem**: RPC authentication failed.

**Check**:
```bash
cat .env
docker compose config | grep -i rpc
docker exec dcrpulse-dashboard env | grep RPC
```

**Fix**: restart after `.env` changes so containers pick up new values:
```bash
docker compose down
docker compose up -d
```

### Changes not applied

**Problem**: changed config but no effect.

**Solutions**:
1. Restart services: `docker compose restart`
2. Rebuild if you changed Docker files: `docker compose build --no-cache && docker compose up -d`
3. Check logs: `docker compose logs dcrd | grep -i "config\|loaded"`

---

## Related Documentation

- **[Installation Guide](../getting-started/installation.md)** - Initial setup and deployment
- **[First Steps](../getting-started/first-steps.md)** - What to do after installation
- **[Multi-Wallet](../features/multi-wallet.md)** - Per-wallet daemon model
- **[Security Best Practices](../deployment/security.md)** - Hardening guidelines
- **[Production Deployment](../deployment/production.md)** - Production configuration
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues
- **[CLI Commands](../reference/cli-commands.md)** - Makefile and Docker Compose commands
