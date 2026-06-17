# CLI Commands Reference

Complete reference for Makefile commands and Docker Compose operations for managing Decred Pulse.

## Table of Contents

- [Makefile Commands](#makefile-commands)
- [Docker Compose Commands](#docker-compose-commands)
- [Direct dcrctl Commands](#direct-dcrctl-commands)
- [Quick Reference](#quick-reference)

---

## Makefile Commands

The `Makefile` provides convenient shortcuts for common operations. All commands are run from the project root directory.

### Basic Operations

#### `make help`
Display all available make targets with descriptions.

```bash
make help
```

**Output**: List of all commands with brief descriptions

---

#### `make setup`
Initial setup - creates `.env` file from `env.example`.

```bash
make setup
```

**What it does**:
- Copies `env.example` to `.env`
- Prompts you to edit credentials
- One-time setup command

**Next steps**: Edit `.env` and run `make start`

---

#### `make start`
Start all services (dcrd, dcrwallet, dcrlnd, brclientd, dcrdex, tor, dashboard).

```bash
make start
```

**What it does**:
- Starts all Docker containers
- Builds images if needed
- Creates networks and volumes
- Dashboard available at http://localhost:8080

**Note**: First run takes 5-10 minutes to build dcrd from source

---

#### `make stop`
Stop all running services.

```bash
make stop
```

**What it does**:
- Stops all containers
- Preserves data volumes
- Networks remain configured

**Data preserved**: Blockchain, wallet, certificates

---

#### `make restart`
Restart all services.

```bash
make restart
```

**What it does**:
- Restarts all containers without rebuilding
- Faster than stop + start
- Data remains intact

**Use case**: Apply configuration changes, recover from errors

---

### Logging Commands

#### `make logs`
View logs from all services (follow mode).

```bash
make logs
```

**What it shows**:
- Combined logs from all containers
- Real-time updates (Ctrl+C to exit)
- Timestamps and service names

---

#### `make logs-dcrd`
View dcrd logs only.

```bash
make logs-dcrd
```

**Use for**:
- Monitoring blockchain sync
- Checking peer connections
- Debugging node issues

---

#### `make logs-backend`
View backend API logs.

```bash
make logs-backend
```

**Use for**:
- API request/response tracking
- RPC connection status
- Backend errors

---

#### `make logs-frontend`
View frontend Nginx logs.

```bash
make logs-frontend
```

**Use for**:
- Web server access logs
- Frontend build output
- HTTP errors

---

#### `make logs-dcrwallet`
View dcrwallet logs.

```bash
make logs-dcrwallet
```

**Use for**:
- Wallet sync status
- Transaction processing
- Rescan progress
- Wallet operations

---

### Build Commands

#### `make build`
Build/rebuild all Docker images (no cache).

```bash
make build
```

**What it does**:
- Rebuilds all images from scratch
- Downloads latest dependencies
- Takes 5-15 minutes

**When to use**:
- After code changes
- Update dependencies
- Clean rebuild

---

#### `make build-dcrd`
Build specific dcrd version.

```bash
# Build from master (latest)
make build-dcrd

# Build specific release
make build-dcrd VERSION=release-v2.0.6
```

**Available versions**:
- `master` - Latest development
- `release-v2.0.6` - Stable release
- `release-v2.0.5` - Previous release
- Any Git tag or commit hash

**Example**:
```bash
# Latest stable
make build-dcrd VERSION=release-v2.0.6

# Latest development
make build-dcrd
```

---

### Status & Monitoring Commands

#### `make status`
Show status of all services and dcrd info.

```bash
make status
```

**Output**:
```
=== Service Status ===
NAME                      STATUS      PORTS
dcrpulse-dcrd         Up          9108-9109
dcrpulse-dcrwallet    Up          9110
dcrpulse-dashboard      Up          8080

=== dcrd Info ===
{
  "version": 200006,
  "protocolversion": 8,
  "blocks": 1016401,
  "connections": 12,
  ...
}
```

---

#### `make sync-status`
Check blockchain sync status and progress.

```bash
make sync-status
```

**Output**:
```json
{
  "chain": "mainnet",
  "blocks": 1016401,
  "headers": 1016401,
  "verificationprogress": 1.0,
  "initialblockdownload": false
}
```

**Key fields**:
- `blocks`: Current synced height
- `verificationprogress`: 0.0-1.0 (0-100%)
- `initialblockdownload`: `true` if still syncing

---

#### `make peers`
Show connected peers.

```bash
make peers
```

**Output**:
```json
"id": 1
"addr": "192.0.2.1:9108"
"id": 2
"addr": "198.51.100.42:9108"
...
```

---

### Shell Access Commands

#### `make shell-dcrd`
Open interactive shell in dcrd container.

```bash
make shell-dcrd
```

**Use for**:
- Browse filesystem
- Check configurations
- Manual debugging

**Exit**: Type `exit` or press Ctrl+D

---

#### `make shell-backend`
Open shell in the dashboard container.

```bash
make shell-backend
```

---

#### `make shell-dcrwallet`
Open shell in dcrwallet container.

```bash
make shell-dcrwallet
```

---

### dcrctl Commands

#### `make dcrctl`
Run arbitrary dcrctl command for dcrd.

```bash
make dcrctl CMD="command_name [args]"
```

**Examples**:
```bash
# Get block count
make dcrctl CMD="getblockcount"

# Get block hash
make dcrctl CMD="getblockhash 1016401"

# Get block info
make dcrctl CMD="getblock 000000..."

# Get network info
make dcrctl CMD="getnetworkinfo"

# Get peer info
make dcrctl CMD="getpeerinfo"

# Get mining info
make dcrctl CMD="getmininginfo"

# Get mempool info
make dcrctl CMD="getmempoolinfo"

# Get raw mempool
make dcrctl CMD="getrawmempool"
```

---

#### `make dcrctl-wallet`
Run dcrctl wallet command.

```bash
make dcrctl-wallet CMD="command_name [args]"
```

**Examples**:
```bash
# Get balance
make dcrctl-wallet CMD="getbalance"

# Get wallet info
make dcrctl-wallet CMD="walletinfo"

# Get new address
make dcrctl-wallet CMD="getnewaddress"

# List accounts
make dcrctl-wallet CMD="listaccounts"

# Get stake info
make dcrctl-wallet CMD="getstakeinfo"

# List transactions
make dcrctl-wallet CMD="listtransactions * 10"

# Get transaction
make dcrctl-wallet CMD="gettransaction txid"
```

---

### Wallet Commands

#### `make wallet-info`
Get wallet information.

```bash
make wallet-info
```

**Output**:
```json
{
  "daemonconnected": true,
  "unlocked": true,
  "txfee": 0.0001,
  "votebits": 1,
  "voting": true,
  ...
}
```

---

#### `make wallet-balance`
Get wallet balance.

```bash
make wallet-balance
```

**Output**:
```json
{
  "balances": [...],
  "cumulativetotal": 1234.56789012,
  "totalspendable": 1000.12345678,
  "totallockedbytickets": 234.44443334
}
```

---

#### `make wallet-seed`
View wallet seed from logs (first-time setup only).

```bash
make wallet-seed
```

** Security Warning**:
- Only works immediately after wallet creation
- Seed shown in creation logs
- **Store seed securely offline!**
- Never share seed with anyone

**Output**:
```
============================================
WALLET SEED (if found in logs)
============================================
Your wallet generation seed is:

word1 word2 word3 ... word33

Hex: abc123...

  IMPORTANT: Store this seed in a safe place!
============================================
```

---

### Data Management Commands

#### `make backup`
Backup blockchain data to local directory.

```bash
make backup
```

**What it does**:
- Creates compressed backup of dcrd data
- Saves to `backups/` directory
- Filename: `dcrd-backup-YYYYMMDD-HHMMSS.tar.gz`

**Output**:
```bash
Creating backup of dcrd data...
Backup created in backups/dcrd-backup-20251006-123456.tar.gz
```

**Backup size**: ~30 GB (blockchain data compresses very little)

---

#### `make backup-wallet`
Backup wallet data to local directory.

```bash
make backup-wallet
```

**What it does**:
- Creates compressed backup of dcrwallet data
- Saves to `backups/` directory
- Filename: `dcrwallet-backup-YYYYMMDD-HHMMSS.tar.gz`

**What's included**:
- Wallet database (wallet.db)
- Imported xpub keys
- Transaction history
- Address cache

**Backup size**: ~50-100 MB

---

#### `make backup-certs`
Backup TLS certificates to local directory.

```bash
make backup-certs
```

**What it does**:
- Creates compressed backup of dcrd certificates
- Saves to `backups/` directory
- Filename: `dcrd-certs-YYYYMMDD-HHMMSS.tar.gz`

**Use case**: Preserve certificates before clean rebuild

**Backup size**: < 1 MB

---

#### `make restore`
Restore blockchain data from backup.

```bash
make restore BACKUP=backups/dcrd-backup-20251006-123456.tar.gz
```

**What it does**:
- Stops all services
- Restores blockchain data from specified backup
- Restarts services

** Warning**: Overwrites existing blockchain data

---

#### `make restore-wallet`
Restore wallet data from backup.

```bash
make restore-wallet BACKUP=backups/dcrwallet-backup-20251006-123456.tar.gz
```

**What it does**:
- Stops dcrwallet service
- Restores wallet data from specified backup
- Restarts dcrwallet

** Warning**: Overwrites existing wallet data

---

#### `make clean`
Remove all containers, volumes, and data (**DANGEROUS**).

```bash
make clean
```

**What it deletes**:
- All containers
- All volumes (blockchain, wallet, certificates)
- All networks
- **ALL DATA PERMANENTLY!**

** Confirmation required**: Prompts for `[y/N]`

**Use case**: Complete reset, start from scratch

---

#### `make clean-dcrd`
Remove only dcrd blockchain data.

```bash
make clean-dcrd
```

**What it deletes**:
- Blockchain data volume
- RPC certificates

**Preserved**:
- Wallet data
- Backend configuration

** Confirmation required**: Prompts for `[y/N]`

**Result**: Requires full blockchain re-sync

---

#### `make clean-dcrwallet`
Remove only wallet data.

```bash
make clean-dcrwallet
```

**What it deletes**:
- Wallet database
- **Funds are LOST if seed not backed up!**

** CRITICAL**: Backup seed before using!

** Confirmation required**: Prompts for `[y/N]`

---

### Update Commands

#### `make update`
Update all services to latest.

```bash
make update
```

**What it does**:
- Rebuilds all images from latest code
- Downloads updated dependencies
- Takes 10-20 minutes

**Next step**: Run `make restart`

---

#### `make update-dcrd`
Update dcrd to latest from source.

```bash
make update-dcrd
```

**What it does**:
- Rebuilds dcrd from latest source
- Automatically restarts dcrd
- Takes 5-10 minutes

**Data preserved**: Blockchain data remains intact

---

### Development Commands

#### `make dev-backend`
Run backend in development mode (outside Docker).

```bash
make dev-backend
```

**Requirements**:
- Go 1.21+
- RPC credentials in environment
- dcrd/dcrwallet running

**Use for**:
- Backend development
- Hot reloading
- Debugging

---

#### `make dev-frontend`
Run frontend in development mode (outside Docker).

```bash
make dev-frontend
```

**Requirements**:
- Node.js 18+
- Backend running (Docker or local)

**Access**: http://localhost:5173 (Vite dev server)

**Use for**:
- Frontend development
- Hot module replacement
- Fast iteration

---

#### `make install-backend`
Install backend Go dependencies.

```bash
make install-backend
```

**What it does**:
- Downloads Go modules
- Updates `go.sum`

---

#### `make install-frontend`
Install frontend npm dependencies.

```bash
make install-frontend
```

**What it does**:
- Runs `npm install`
- Creates `node_modules/`
- Updates `package-lock.json`

---

## Docker Compose Commands

Direct Docker Compose commands for more control.

### Basic Operations

#### Start Services
```bash
# All services
docker compose up -d

# Specific service
docker compose up -d dcrd
docker compose up -d backend
```

#### Stop Services
```bash
# All services
docker compose down

# Specific service
docker compose stop dcrd
```

#### Restart Services
```bash
# All services
docker compose restart

# Specific service
docker compose restart dashboard
```

#### View Status
```bash
docker compose ps
```

#### View Logs
```bash
# All services (follow)
docker compose logs -f

# Specific service
docker compose logs -f dcrd

# Last 50 lines
docker compose logs --tail=50 dcrd

# Without follow
docker compose logs dcrd
```

---

### Build Commands

#### Build All Images
```bash
# With cache
docker compose build

# Without cache (clean build)
docker compose build --no-cache
```

#### Build Specific Service
```bash
docker compose build dcrd
docker compose build backend
docker compose build frontend
```

#### Build with Variables
```bash
# Set dcrd version
DCRD_VERSION=release-v2.0.6 docker compose build dcrd

# Or edit .env file
echo "DCRD_VERSION=release-v2.0.6" >> .env
docker compose build dcrd
```

---

### Volume Management

#### List Volumes
```bash
docker volume ls | grep dcrpulse
```

**Output**:
```
dcrpulse_dcrd-data
dcrpulse_dcrwallet-data
dcrpulse_certs
```

#### Inspect Volume
```bash
docker volume inspect dcrpulse_dcrd-data
```

#### Remove Volumes
```bash
# Specific volume
docker volume rm dcrpulse_dcrd-data

# All unused volumes
docker volume prune

# All volumes (with down command)
docker compose down -v
```

---

### Network Management

#### List Networks
```bash
docker network ls | grep decred
```

#### Inspect Network
```bash
docker network inspect dcrpulse_decred-network
```

---

### Container Management

#### Execute Command in Container
```bash
# dcrd
docker exec dcrpulse-dcrd dcrctl --help

# dcrwallet
docker exec dcrpulse-dcrwallet dcrctl --wallet --help

# backend
docker exec dcrpulse-dashboard ls -la
```

#### Interactive Shell
```bash
docker exec -it dcrpulse-dcrd /bin/sh
docker exec -it dcrpulse-dashboard /bin/sh
```

#### View Container Details
```bash
docker inspect dcrpulse-dcrd
```

#### View Container Resources
```bash
# Real-time stats
docker stats

# Specific container
docker stats dcrpulse-dcrd
```

---

## Direct dcrctl Commands

Run `dcrctl` commands directly in containers.

### Node (dcrd) Commands

**Format**:
```bash
docker exec dcrpulse-dcrd dcrctl \
  --rpcuser=your_user \
  --rpcpass=your_pass \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  COMMAND
```

**Common Commands**:
```bash
# Blockchain info
getblockcount
getbestblockhash
getblock <hash>
getblockhash <height>
getblockchaininfo

# Network info
getnetworkinfo
getpeerinfo
getconnectioncount

# Mempool
getmempoolinfo
getrawmempool

# Mining
getmininginfo
getdifficulty

# Transactions
getrawtransaction <txid> 1
decoderawtransaction <hex>

# Staking
getstakedifficulty
getstakeinfo
getstakeversioninfo
estimatestakediff

# Utility
help [command]
getinfo
```

---

### Wallet (dcrwallet) Commands

**Format**:
```bash
docker exec dcrpulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=your_user \
  --rpcpass=your_pass \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  COMMAND
```

**Common Commands**:
```bash
# Wallet info
walletinfo
getbalance
listaccounts

# Addresses
getnewaddress
getaddressesbyaccount default
validateaddress <address>

# Transactions
listtransactions "*" 10
gettransaction <txid>
sendfrom <account> <address> <amount>

# Staking
getstakeinfo
purchaseticket <account> <price> <tickets>
gettickets true

# Accounts
listaccounts
getaccount <address>
getaccountaddress <account>

# Import/Export
importprivkey <privkey>
dumpprivkey <address>
importxpub <account> <xpub>

# Utility
help [command]
walletlock
walletpassphrase <passphrase> <timeout>
```

---

## Quick Reference

### Daily Operations

```bash
# Start dashboard
make start

# Check status
make status
docker compose ps

# View logs
make logs
make logs-dcrd
make logs-backend

# Stop
make stop
```

### Monitoring

```bash
# Sync progress
make sync-status

# Peer connections
make peers

# Wallet balance
make wallet-balance

# Service status
docker compose ps
docker stats
```

### Troubleshooting

```bash
# View logs with errors
make logs | grep -i error
make logs-dcrd | grep -i error

# Restart services
make restart
docker compose restart

# Rebuild
make build
make start

# Check connectivity
curl http://localhost:8080/api/health
curl http://localhost:8080
```

### Updates

```bash
# Update all
make update
make restart

# Update dcrd only
make update-dcrd

# Build specific version
make build-dcrd VERSION=release-v2.0.6
```

### Data Management

```bash
# Backup
make backup

# Restore
make restore BACKUP=backups/dcrd-backup-xxx.tar.gz

# Clean (careful!)
make clean         # Everything
make clean-dcrd    # Node data only
```

---

## Tips & Best Practices

### Use Aliases

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
# Decred Pulse shortcuts
alias dp-start='cd ~/dcrpulse && make start'
alias dp-stop='cd ~/dcrpulse && make stop'
alias dp-logs='cd ~/dcrpulse && make logs'
alias dp-status='cd ~/dcrpulse && make status'
alias dp-dcrd='cd ~/dcrpulse && make logs-dcrd'
```

### Tab Completion

Enable Docker Compose tab completion:

```bash
# Bash
sudo curl -L https://raw.githubusercontent.com/docker/compose/main/contrib/completion/bash/docker-compose \
  -o /etc/bash_completion.d/docker-compose

# Zsh
mkdir -p ~/.zsh/completion
curl -L https://raw.githubusercontent.com/docker/compose/main/contrib/completion/zsh/_docker-compose \
  > ~/.zsh/completion/_docker-compose
```

### Logging Best Practices

```bash
# Save logs to file
make logs > logs-$(date +%Y%m%d).txt

# Watch for specific errors
make logs | grep -i "error\|fail\|fatal"

# Monitor sync progress
make logs-dcrd | grep -i "sync\|blocks"

# Tail specific lines
docker compose logs --tail=100 dcrd
```

### Resource Monitoring

```bash
# Watch resource usage
watch docker stats

# Check disk space
df -h
docker system df

# Clean up unused resources
docker system prune -a
```

---

## Related Documentation

- **[Quick Start](../getting-started/installation.md)** - Getting started guide
- **[Docker Setup](../getting-started/installation.md)** - Complete Docker documentation
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues and solutions
- **[Configuration](../setup/configuration.md)** - Configuration options

---

**Need Help?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

