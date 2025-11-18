# Configuration Guide

Complete guide to configuring Decred Pulse for different use cases, including environment variables, dcrd/dcrwallet configuration, and Docker Compose settings.

## 📋 Configuration Files

Decred Pulse uses multiple configuration files:

```
decred-pulse/
├── .env                    # Main environment configuration
├── dcrwallet/
│   └── dcrwallet.conf      # dcrwallet configuration (in container)
└── docker-compose.yml      # Docker services configuration
```

---

## 🔐 Environment Variables (.env)

The `.env` file contains sensitive credentials and service configuration.

### Initial Setup

```bash
# Create from example
cp env.example .env

# Edit with your values
nano .env
```

### Required Variables

#### `DCRD_RPC_USER`
**Description**: Username for dcrd RPC authentication

**Default**: `decred`

**Example**: `DCRD_RPC_USER=decred`

**Recommendations**:
- Use alphanumeric characters
- Avoid special characters
- Keep simple for local development
- Different username for production

---

#### `DCRD_RPC_PASS`
**Description**: Password for dcrd RPC authentication

**Default**: `change_this_to_a_secure_password`

**Example**: `DCRD_RPC_PASS=MySecurePassword123`

**Recommendations**:
- ✅ Use strong password (16+ characters)
- ✅ Mix letters, numbers, symbols
- ⚠️ Avoid: `$ \ " ' `` (may cause issues)
- 🔒 Never commit to version control
- 🔒 Different password for production

**Generate secure password**:
```bash
# Random 32-character password
openssl rand -base64 32
```

---

#### `DCRWALLET_RPC_USER`
**Description**: Username for dcrwallet RPC authentication

**Default**: `dcrwallet`

**Example**: `DCRWALLET_RPC_USER=dcrwallet`

---

#### `DCRWALLET_RPC_PASS`
**Description**: Password for dcrwallet RPC authentication

**Default**: `change_this_to_a_secure_wallet_password`

**Example**: `DCRWALLET_RPC_PASS=AnotherSecurePass456`

**Recommendations**:
- Use different password than dcrd
- Same security requirements as `DCRD_RPC_PASS`

---

### Optional Variables

#### `DCRD_VERSION`
**Description**: dcrd version/branch to build from source

**Default**: `master` (latest development)

**Values**:
- `master` - Latest development code
- `release-v2.0.6` - Stable release 2.0.6
- `release-v2.0.5` - Previous stable
- Any Git tag or commit hash

**Example**:
```bash
# Latest stable
DCRD_VERSION=release-v2.0.6

# Specific commit
DCRD_VERSION=abc123def456

# Latest development (default)
# DCRD_VERSION=master
```

**When to use**:
- Production: Use stable release tags
- Testing: Use master for latest features
- Debugging: Use specific commits

---

#### `DCRWALLET_VERSION`
**Description**: dcrwallet version/branch to build

**Default**: `master`

**Example**: `DCRWALLET_VERSION=release-v2.0.6`

---

#### `DCRWALLET_GAP_LIMIT`
**Description**: HD wallet gap limit for address discovery

**Default**: `400`

**Range**: `20` - `10000`

**Example**: `DCRWALLET_GAP_LIMIT=500`

**What it controls**:
- Number of consecutive unused addresses to monitor
- Higher = discovers funds at higher address indices
- Lower = faster scanning, may miss funds

**Recommendations**:
```
New wallet:     20-50    (few addresses used)
Normal use:     400      (recommended default)
Active wallet:  500-1000 (many transactions)
Legacy wallet:  1000+    (old or heavily used)
```

**Impact**:
- **Memory**: Linear increase with limit
- **Scan time**: Proportional to limit
- **Accuracy**: Higher = more thorough

**Example scenarios**:
```bash
# Standard usage
DCRWALLET_GAP_LIMIT=400

# Missing funds (increase gradually)
DCRWALLET_GAP_LIMIT=500

# Legacy wallet recovery
DCRWALLET_GAP_LIMIT=5000
```

---

#### `DCRD_TESTNET`
**Description**: Enable testnet mode

**Default**: Commented out (mainnet)

**Values**:
- Uncommented: Testnet
- Commented: Mainnet (default)

**Example**:
```bash
# Mainnet (production)
# DCRD_TESTNET=1

# Testnet (testing)
DCRD_TESTNET=1
```

**Note**: Changing networks requires clean restart:
```bash
make clean
# Edit .env
make start
```

---

#### `DCRD_EXTRA_ARGS`
**Description**: Additional command-line arguments for dcrd

**Default**: `--txindex`

**Example**: `DCRD_EXTRA_ARGS=--txindex --debuglevel=debug`

**Common arguments**:
```bash
# Enable transaction indexing (required for full explorer)
DCRD_EXTRA_ARGS=--txindex

# Increase logging
DCRD_EXTRA_ARGS=--txindex --debuglevel=debug

# Multiple arguments
DCRD_EXTRA_ARGS=--txindex --maxpeers=200 --debuglevel=info
```

**Available flags**: See [dcrd documentation](https://github.com/decred/dcrd/tree/master/docs)

---

### Example .env File

**Minimal configuration**:
```bash
# RPC Credentials
DCRD_RPC_USER=decred
DCRD_RPC_PASS=MySecure123Pass

DCRWALLET_RPC_USER=dcrwallet
DCRWALLET_RPC_PASS=MyWalletPass456

# Gap limit
DCRWALLET_GAP_LIMIT=400
```

**Production configuration**:
```bash
# Production RPC Credentials (strong passwords)
DCRD_RPC_USER=dcrd_production
DCRD_RPC_PASS=$(openssl rand -base64 32)

DCRWALLET_RPC_USER=dcrwallet_production
DCRWALLET_RPC_PASS=$(openssl rand -base64 32)

# Stable versions
DCRD_VERSION=release-v2.0.6
DCRWALLET_VERSION=release-v2.0.6

# Standard gap limit
DCRWALLET_GAP_LIMIT=400

# Transaction indexing
DCRD_EXTRA_ARGS=--txindex
```

**Testnet configuration**:
```bash
# Testnet RPC Credentials
DCRD_RPC_USER=testnet_user
DCRD_RPC_PASS=testnet_pass123

DCRWALLET_RPC_USER=testnet_wallet
DCRWALLET_RPC_PASS=testnet_wallet_pass456

# Enable testnet
DCRD_TESTNET=1

# Lower gap limit (faster testing)
DCRWALLET_GAP_LIMIT=100
```

---

## ⚙️ dcrd Configuration

dcrd configuration is set via command-line arguments in `docker-compose.yml`. There is no separate `dcrd.conf` file.

### Key Settings

#### Transaction Indexing

Set via `DCRD_EXTRA_ARGS` in `.env`:

```bash
# Enable full transaction index
DCRD_EXTRA_ARGS=--txindex
```

**Impact**:
- ✅ Enables transaction search by ID
- ✅ Required for full block explorer
- ⚠️ Increases disk usage (~15-20%)
- ⚠️ Slower initial sync

**Recommendation**: Enabled by default for full functionality

---

#### Advanced Configuration

To add additional dcrd flags, append them to `DCRD_EXTRA_ARGS`:

```bash
# Multiple flags (space-separated)
DCRD_EXTRA_ARGS=--txindex --debuglevel=debug --maxpeers=50
```

**Common Flags**:
- `--txindex` - Enable transaction indexing (enabled by default)
- `--debuglevel=LEVEL` - Set log level (trace, debug, info, warn, error, critical)
- `--maxpeers=N` - Maximum peer connections (default: 125)
- `--testnet` - Use testnet instead of mainnet

**Example Configurations**:

```bash
# Low bandwidth (fewer peers)
DCRD_EXTRA_ARGS=--txindex --maxpeers=25

# High performance (more peers)
DCRD_EXTRA_ARGS=--txindex --maxpeers=200

# Debug mode
DCRD_EXTRA_ARGS=--txindex --debuglevel=debug
```

**Note**: RPC settings (`rpcuser`, `rpcpass`, `rpclisten`) are configured in `docker-compose.yml` and should not be changed.

---

## 💼 dcrwallet Configuration

dcrwallet configuration is set via environment variables and command-line arguments in Docker Compose.

### Key Settings

#### Gap Limit

Set via `.env`:
```bash
DCRWALLET_GAP_LIMIT=400
```

Passed to dcrwallet as `--gaplimit=200`

---

#### RPC

RPC credentials are set via environment variables:
- `DCRWALLET_RPC_USER`
- `DCRWALLET_RPC_PASS`

RPC listen address: `--rpclisten=:9110`

---

#### Network

Testnet mode: `--testnet` (set via `DCRD_TESTNET` in `.env`)

---

#### Wallet Location

Data directory: `/home/dcrwallet/.dcrwallet`

Wallet database: `/home/dcrwallet/.dcrwallet/mainnet/wallet.db`

---

## 🐳 Docker Compose Configuration

Advanced Docker Compose customization.

**File**: `docker-compose.yml`

### Port Mappings

```yaml
services:
  dcrd:
    ports:
      - "9108:9108"  # P2P port
      - "9109:9109"  # RPC port
  
  dcrwallet:
    ports:
      - "9110:9110"  # Wallet RPC port
  
  backend:
    ports:
      - "8080:8080"  # API port
  
  frontend:
    ports:
      - "3000:80"    # Web UI port
```

**To change ports**:
```yaml
# Use different external port
ports:
  - "8000:80"      # Access frontend on port 8000
  - "8081:8080"    # Access backend on port 8081
```

---

### Volume Configuration

```yaml
volumes:
  dcrd-data:       # Blockchain data (~10 GB)
  dcrwallet-data:  # Wallet database
  certs:           # RPC certificates
```

**Volume location**:
```bash
# List volumes
docker volume ls | grep decred-pulse

# Inspect volume
docker volume inspect decred-pulse_dcrd-data
```

---

### Resource Limits

Add memory and CPU limits:

```yaml
services:
  dcrd:
    mem_limit: 2g
    cpus: 2.0
  
  dcrwallet:
    mem_limit: 1g
    cpus: 1.0
  
  backend:
    mem_limit: 512m
    cpus: 0.5
  
  frontend:
    mem_limit: 256m
    cpus: 0.5
```

---

### Health Checks

Customize health check timing:

```yaml
healthcheck:
  interval: 30s      # Check every 30 seconds
  timeout: 10s       # Wait up to 10 seconds
  retries: 5         # Retry 5 times before marking unhealthy
  start_period: 60s  # Wait 60s before first check
```

---

### Build Arguments

Pass build-time variables:

```yaml
services:
  dcrd:
    build:
      context: ./dcrd
      args:
        DCRD_VERSION: ${DCRD_VERSION:-master}
```

---

## 🔒 Security Best Practices

### For Development

```bash
# Simple credentials
DCRD_RPC_USER=decred
DCRD_RPC_PASS=devpass123

# Local only
# No external exposure needed
```

### For Production

```bash
# Strong credentials
DCRD_RPC_USER=$(openssl rand -hex 16)
DCRD_RPC_PASS=$(openssl rand -base64 32)

# Firewall rules
# Only allow necessary ports
sudo ufw allow 9108/tcp  # P2P only

# TLS
# Use proper certificates (not self-signed)

# Monitoring
# Set up alerting for service health
```

---

## 📊 Configuration Profiles

### Profile: Development

**Goals**: Fast iteration, verbose logging

**.env**:
```bash
DCRD_RPC_USER=decred
DCRD_RPC_PASS=devpass
DCRWALLET_RPC_USER=dcrwallet
DCRWALLET_RPC_PASS=walletpass
DCRWALLET_GAP_LIMIT=100
# DCRD_TESTNET=1  # Consider testnet
DCRD_EXTRA_ARGS=--txindex --debuglevel=debug
```

---

### Profile: Production

**Goals**: Security, stability, performance

**.env**:
```bash
DCRD_RPC_USER=prod_dcrd_$(openssl rand -hex 8)
DCRD_RPC_PASS=$(openssl rand -base64 32)
DCRWALLET_RPC_USER=prod_wallet_$(openssl rand -hex 8)
DCRWALLET_RPC_PASS=$(openssl rand -base64 32)
DCRD_VERSION=release-v2.0.6
DCRWALLET_VERSION=release-v2.0.6
DCRWALLET_GAP_LIMIT=400
DCRD_EXTRA_ARGS=--txindex --debuglevel=info
```

---

### Profile: Testnet

**Goals**: Testing, experimentation

**.env**:
```bash
DCRD_RPC_USER=testnet
DCRD_RPC_PASS=testpass123
DCRWALLET_RPC_USER=testnet_wallet
DCRWALLET_RPC_PASS=testwalletpass456
DCRD_TESTNET=1
DCRWALLET_GAP_LIMIT=100
DCRD_EXTRA_ARGS=--testnet --txindex
```

---

### Profile: Low Resource

**Goals**: Minimal RAM/CPU, slow sync acceptable

**.env**:
```bash
DCRD_RPC_USER=decred
DCRD_RPC_PASS=password123
DCRWALLET_RPC_USER=dcrwallet
DCRWALLET_RPC_PASS=walletpass456
DCRWALLET_GAP_LIMIT=100
DCRD_EXTRA_ARGS=--txindex --maxpeers=25
```

**docker-compose.yml additions**:
```yaml
services:
  dcrd:
    mem_limit: 1g
  dcrwallet:
    mem_limit: 512m
```

---

## 🔧 Configuration Validation

### Check Configuration

```bash
# View current .env
cat .env

# Check dcrd command-line args
docker exec decred-pulse-dcrd ps aux | grep dcrd

# Check what dcrd is using
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=your_user \
  --rpcpass=your_pass \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getinfo
```

### Test Credentials

```bash
# Test dcrd RPC
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=$DCRD_RPC_USER \
  --rpcpass=$DCRD_RPC_PASS \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getblockcount

# Test dcrwallet RPC
docker exec decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=$DCRWALLET_RPC_USER \
  --rpcpass=$DCRWALLET_RPC_PASS \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  walletinfo
```

---

## 🐛 Troubleshooting Configuration

### Credentials Not Working

**Problem**: RPC authentication failed

**Check**:
```bash
# Ensure .env is loaded
cat .env

# Check what Docker Compose sees
docker compose config | grep RPC

# Verify containers have correct values
docker exec decred-pulse-backend env | grep RPC
```

**Fix**:
```bash
# Restart after .env changes
docker compose down
docker compose up -d
```

---

### Configuration Changes Not Applied

**Problem**: Changed config but no effect

**Solutions**:
1. **Restart services**:
   ```bash
   docker compose restart
   ```

2. **Rebuild if changed Docker files**:
   ```bash
   docker compose build --no-cache
   docker compose up -d
   ```

3. **Check logs**:
   ```bash
   docker compose logs dcrd | grep -i "config\|loaded"
   ```

---

## 📚 Related Documentation

- **[Quick Start](../quickstart.md)** - Initial setup
- **[Docker Setup](../docker-setup.md)** - Docker configuration
- **[Environment Variables](../reference/environment-variables.md)** - Full variable reference
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues

---

**Questions?** Check the [FAQ](../reference/faq.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

