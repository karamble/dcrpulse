# Troubleshooting Guide

Complete troubleshooting guide for Decred Pulse. This guide covers common issues with node operations, wallet management, Docker deployment, and general connectivity problems.

## 📋 Quick Diagnostic Steps

When something goes wrong, follow these steps:

1. **Check service status**:
   ```bash
   docker compose ps
   ```

2. **View logs**:
   ```bash
   docker compose logs -f
   ```

3. **Check connectivity**:
   - Frontend: `http://localhost:3000`
   - Backend: `http://localhost:8080/api/health`

4. **Verify credentials** in `.env` file

5. **Review this guide** for specific issues

---

## 🔧 Node Issues

### Node Status Shows "RPC Not Connected"

**Symptoms**:
- Dashboard displays "RPC client not connected" error
- No node data visible
- 503 Service Unavailable errors

**Solutions**:

#### 1. Verify dcrd is Running
```bash
docker compose ps dcrd
# Should show "Up" status

# If not running:
docker compose up -d dcrd
```

#### 2. Check RPC Credentials
```bash
# View current credentials
cat .env | grep DCRD_RPC

# Verify dcrd is using these credentials
docker exec decred-pulse-dcrd ps aux | grep dcrd
```

**Common mistake**: Mismatched username/password

**Fix**:
```bash
# Edit .env file
nano .env

# Update:
DCRD_RPC_USER=your_username
DCRD_RPC_PASS=your_password

# Restart backend
docker compose restart backend
```

#### 3. Verify RPC Port
```bash
# Check port 9109 is listening
docker exec decred-pulse-dcrd netstat -tulpn | grep 9109

# Should show:
# tcp    0    0 127.0.0.1:9109    0.0.0.0:*    LISTEN
```

#### 4. Check dcrd Logs
```bash
docker compose logs -f dcrd | tail -50

# Look for:
# - "DCRD: Version X.X.X"
# - "RPC server listening"
# - Any error messages
```

#### 5. Check Backend Logs
```bash
docker compose logs -f backend | tail -50

# Look for:
# - "Starting Decred Dashboard API server"
# - Connection errors
# - RPC initialization messages
```

#### 6. Restart Services
```bash
# Restart dcrd and backend
docker compose restart dcrd backend

# Wait 30 seconds, then check status
docker compose ps
```

#### 7. Check Certificate
```bash
# Verify RPC certificate exists
docker exec decred-pulse-dcrd ls -la /certs/rpc.cert

# If missing, regenerate:
docker compose down dcrd
docker volume rm decred-pulse_certs
docker compose up -d dcrd
```

---

### Node Stuck Syncing

**Symptoms**:
- Sync progress not increasing
- Same block height for extended period
- "Syncing" status for hours/days

**Solutions**:

#### 1. Check Peer Connections
```bash
# View connected peers
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getpeerinfo | grep -c '"addr"'

# Should show 5+ peers
```

**If no peers**:
```bash
# Check P2P port
docker compose logs dcrd | grep -i "peer\|connect"

# Ensure port 9108 is exposed
docker port decred-pulse-dcrd
```

#### 2. Verify Disk Space
```bash
# Check available space
df -h

# Should have 15+ GB free for mainnet
```

**If low on space**:
```bash
# Clean Docker
docker system prune -a

# Or free system space
sudo apt clean  # Ubuntu/Debian
```

#### 3. Check System Resources
```bash
# Check resource usage
docker stats decred-pulse-dcrd

# CPU should be < 100%
# Memory should be < 2GB
```

**If high usage**:
- Wait for sync to complete
- Close other applications
- Consider upgrading hardware

#### 4. Review Sync Logs
```bash
# Watch sync progress
docker compose logs -f dcrd | grep -i "sync\|blocks\|headers"

# Look for:
# - "Syncing to block height..."
# - "Processed X blocks/headers..."
# - Error messages
```

#### 5. Restart Sync
```bash
# Restart dcrd
docker compose restart dcrd

# Monitor logs
docker compose logs -f dcrd
```

#### 6. Check Network Connectivity
```bash
# Test DNS
ping mainnet-seed.decred.org

# Test P2P port
telnet mainnet-seed.decred.org 9108
```

---

### No Peers Connecting

**Symptoms**:
- Peer count: 0
- "Listening on" but no connections
- Sync not progressing

**Solutions**:

#### 1. Open P2P Port (9108)

**Docker**:
```bash
# Verify docker-compose.yml exposes port
grep -A 3 "ports:" docker-compose.yml

# Should show:
#   - "9108:9108"  # P2P
```

**Firewall** (if not using Docker):
```bash
# UFW
sudo ufw allow 9108/tcp

# iptables
sudo iptables -A INPUT -p tcp --dport 9108 -j ACCEPT
sudo iptables-save > /etc/iptables/rules.v4

# Verify
sudo ufw status | grep 9108
```

#### 2. Add Seed Nodes
```bash
# Edit .env to add peer flags
nano .env

# Add to DCRD_EXTRA_ARGS:
DCRD_EXTRA_ARGS=--txindex --addpeer=mainnet-seed.decred.org --addpeer=mainnet-seed.decredbrasil.com

# Restart
docker compose restart dcrd
```

#### 3. Check Listen Address
```bash
# View dcrd process
docker exec decred-pulse-dcrd ps aux | grep dcrd

# dcrd listens on default port 9108 for P2P connections
```

#### 4. Verify Network Mode
```bash
# Check Docker network
docker network inspect decred-pulse_decred-network

# dcrd should be connected
```

#### 5. Test External Connectivity
```bash
# From another machine, test if port is reachable
telnet YOUR_IP 9108

# Or use online port checker:
# https://www.yougetsignal.com/tools/open-ports/
```

---

### Incorrect Block Height

**Symptoms**:
- Block height differs from network
- Different block hash than dcrdata.org
- Fork warning

**Solutions**:

#### 1. Verify Network Height
```bash
# Check dcrdata.org for correct height
curl -s https://dcrdata.org/api/block/best/height

# Compare with your node
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getblockcount
```

#### 2. Check for Forks
```bash
# Get your best block hash
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getbestblockhash

# Compare with dcrdata.org/block/best
```

**If different hash**:
- You're on a fork
- Need to resync

#### 3. Check Peer Versions
```bash
# View peer versions
docker compose logs dcrd | grep -i version

# Ensure peers are on compatible versions
# Update dcrd if old version
```

#### 4. Restart Node
```bash
docker compose restart dcrd

# Monitor sync
docker compose logs -f dcrd
```

#### 5. Reindex (Last Resort)
```bash
# WARNING: Requires full re-sync (4-8 hours)

# Stop and remove data
docker compose down dcrd
docker volume rm decred-pulse_dcrd-data

# Start fresh
docker compose up -d dcrd
docker compose logs -f dcrd
```

---

## 💼 Wallet Issues

### Wallet Not Connecting

**Symptoms**:
- "Wallet RPC client not initialized"
- No wallet data displayed
- 503 errors on wallet endpoints

**Solutions**:

#### 1. Verify dcrwallet is Running
```bash
docker compose ps dcrwallet

# If not running:
docker compose up -d dcrwallet
```

#### 2. Check Wallet RPC Credentials
```bash
# View credentials
cat .env | grep DCRWALLET

# Check dcrwallet config
docker exec decred-pulse-dcrwallet cat /home/dcrwallet/.dcrwallet/dcrwallet.conf | grep rpc
```

#### 3. Check Wallet Logs
```bash
docker compose logs -f dcrwallet | tail -50

# Look for:
# - "DCRWALLET: Version X.X.X"
# - "RPC server listening"
# - "Opened wallet"
# - Error messages
```

#### 4. Verify Wallet is Unlocked
```bash
# Check wallet status
docker exec decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  walletinfo | grep unlocked

# Should show: "unlocked": true
```

**If locked**:
```bash
# Unlock wallet
docker exec -it decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  walletpassphrase "your_wallet_passphrase" 0
```

#### 5. Restart Wallet
```bash
docker compose restart dcrwallet backend

# Wait 30 seconds
docker compose ps
```

---

### Xpub Import Failed

**Symptoms**:
- Import button does nothing
- Error message on import
- Import modal closes without confirmation

**Solutions**:

#### 1. Validate Xpub Format
```bash
# Xpub should start with "dpub" for mainnet
# Example: dpubZF6ScrXjYgjGdVL2FzAW...

# Check length: should be 111 characters
echo "your_xpub" | wc -c
```

#### 2. Check Gap Limit Range
- Minimum: 20
- Maximum: 1000
- Recommended: 200

#### 3. Check Wallet Connection
```bash
# Ensure wallet is connected
curl http://localhost:8080/api/wallet/status
```

#### 4. Review Backend Logs
```bash
docker compose logs -f backend | grep -i "xpub\|import"

# Look for detailed error messages
```

#### 5. Try Manual Import
```bash
# Import via dcrctl
docker exec -it decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  importxpub "account-name" "xpub-key"
```

#### 6. Check Browser Console
- Open browser DevTools (F12)
- Check Console tab for JavaScript errors
- Check Network tab for failed requests

---

### Rescan Stuck or Frozen

**Symptoms**:
- Progress bar not moving
- Same percentage for extended time
- No log activity

**Solutions**:

#### 1. Check Wallet Logs
```bash
docker compose logs -f dcrwallet | grep -i "rescan\|address"

# Should show progress messages
```

#### 2. Verify Wallet is Running
```bash
docker compose ps dcrwallet

# Should show "Up" status
```

#### 3. Check for Stale Detection
```bash
# Backend logs
docker compose logs backend | grep -i "stale\|rescan"

# If "stale" detected, rescan may have completed
```

#### 4. Restart Wallet
```bash
docker compose restart dcrwallet

# Rescan resumes from checkpoint
```

#### 5. Manual Rescan
```bash
# Trigger rescan via dcrctl
docker exec decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  rescan
```

#### 6. Check Blockchain Sync
```bash
# Ensure dcrd is fully synced first
curl http://localhost:8080/api/node/status | jq '.syncProgress'

# Should be 100
```

---

### Missing Transactions After Import

**Symptoms**:
- Expected transactions not showing
- Balance lower than expected
- Transaction history incomplete

**Solutions**:

#### 1. Increase Gap Limit
```bash
# Current gap limit may be too low
# Try doubling it: 200 → 400 → 800

# Re-import xpub with higher gap limit
# Dashboard → Import Xpub → Enter 500
```

#### 2. Verify Correct Account
```bash
# Check which account xpub is from
# In source wallet:
dcrctl --wallet getmasterpubkey default
dcrctl --wallet getmasterpubkey mixed
dcrctl --wallet getmasterpubkey unmixed

# Import correct account's xpub
```

#### 3. Check Address Derivation
```bash
# List addresses in imported account
docker exec decred-pulse-dcrwallet dcrctl \
  --wallet \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9110 \
  --rpccert=/certs/rpc.cert \
  listaddresses

# Compare with source wallet addresses
```

#### 4. Wait for Full Rescan
```bash
# Monitor rescan completion
curl http://localhost:8080/api/wallet/sync-progress

# Wait until progress: 100, isRescanning: false
```

#### 5. Verify Network Match
```bash
# Ensure mainnet xpub for mainnet node
# Check .env:
cat .env | grep NETWORK

# Should be NETWORK=mainnet or empty (defaults to mainnet)
```

#### 6. Check Blockchain Sync
```bash
# Transactions only appear after their block is synced
curl http://localhost:8080/api/node/status | jq '.blocks'

# Compare with transaction block height
```

---

### Balances Not Updating

**Symptoms**:
- Dashboard shows outdated balances
- Recent transactions not reflected
- Last update timestamp old

**Solutions**:

#### 1. Manual Refresh
- Click browser refresh (F5)
- Or force refresh (Ctrl+Shift+R)

#### 2. Check Auto-Refresh
```bash
# Dashboard should auto-refresh every 30s
# Open browser console (F12)
# Look for periodic API calls to /api/wallet/dashboard
```

#### 3. Verify Wallet Sync
```bash
curl http://localhost:8080/api/wallet/status | jq '.synced'

# Should return: true
```

#### 4. Check for Confirmations
```bash
# Recent transactions need confirmations
# View transaction in dashboard
# Check confirmation count
```

#### 5. Review Backend Logs
```bash
docker compose logs -f backend | grep -i "wallet\|balance"

# Look for errors fetching wallet data
```

#### 6. Restart Backend
```bash
docker compose restart backend

# Wait 30 seconds
# Refresh dashboard
```

---

## 🐳 Docker Issues

### Container Won't Start

**Symptoms**:
- `docker compose up` fails
- Container exits immediately
- "Unhealthy" or "Exited" status

**Solutions**:

#### 1. Check Container Status
```bash
docker compose ps

# Note which container is problematic
```

#### 2. View Logs
```bash
docker compose logs <service-name>

# Examples:
docker compose logs dcrd
docker compose logs dcrwallet
docker compose logs backend
docker compose logs frontend
```

#### 3. Check Port Conflicts
```bash
# See if ports are already in use
sudo netstat -tulpn | grep -E "3000|8080|9108|9109|9110"

# Kill conflicting processes or change ports in docker-compose.yml
```

#### 4. Verify Docker Resources
```bash
# Check Docker disk space
docker system df

# Clean if needed
docker system prune -a
```

#### 5. Check .env File
```bash
# Ensure .env exists
ls -la .env

# If missing:
cp env.example .env
nano .env  # Set passwords
```

#### 6. Rebuild Containers
```bash
# Clean rebuild
docker compose down
docker compose build --no-cache
docker compose up -d
```

---

### "Unhealthy" Container Status

**Symptoms**:
- Docker shows container as "unhealthy"
- Health check failing
- Dependent containers not starting

**Solutions**:

#### 1. Check Health Check Logs
```bash
# View health check results
docker inspect decred-pulse-dcrd | grep -A 10 Health

# Or for other containers:
docker inspect decred-pulse-dcrwallet | grep -A 10 Health
```

#### 2. Manually Test Health Check
```bash
# dcrd health check (from inside container)
docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getblockcount

# Should return a number
```

#### 3. Check RPC Readiness
```bash
# dcrd might still be starting
# Wait 1-2 minutes after startup
# Check logs for "RPC server listening"
docker compose logs dcrd | grep -i "rpc server"
```

#### 4. Increase Health Check Timeout
```bash
# Edit docker-compose.yml
nano docker-compose.yml

# Increase:
healthcheck:
  interval: 30s
  timeout: 10s  # Increase if needed
  retries: 5    # Increase if needed
  start_period: 60s  # Increase for slow startups
```

#### 5. Restart Container
```bash
docker compose restart dcrd
# Or the specific unhealthy container

# Wait 1-2 minutes
docker compose ps
```

---

### Port Already in Use

**Symptoms**:
- Error: "bind: address already in use"
- Container fails to start
- Port conflict message

**Solutions**:

#### 1. Identify Process Using Port
```bash
# Find process on port 3000 (frontend)
sudo lsof -i :3000

# Or port 8080 (backend)
sudo lsof -i :8080

# Or netstat
sudo netstat -tulpn | grep 3000
```

#### 2. Kill Conflicting Process
```bash
# Kill by PID
sudo kill <PID>

# Or killall
sudo killall -9 node  # For Node.js apps
```

#### 3. Change Port in docker-compose.yml
```yaml
services:
  frontend:
    ports:
      - "3001:80"  # Changed from 3000 to 3001

  backend:
    ports:
      - "8081:8080"  # Changed from 8080 to 8081
```

#### 4. Stop All Docker Containers
```bash
# Stop all containers using ports
docker compose down

# Or stop all Docker containers
docker stop $(docker ps -aq)
```

#### 5. Restart Docker
```bash
sudo systemctl restart docker

# Or on macOS:
# Restart Docker Desktop
```

---

### Out of Disk Space

**Symptoms**:
- "No space left on device"
- Container crashes
- Sync fails
- Database corruption

**Solutions**:

#### 1. Check Available Space
```bash
# System disk space
df -h

# Docker disk usage
docker system df
```

#### 2. Clean Docker
```bash
# Remove unused images
docker image prune -a

# Remove unused volumes
docker volume prune

# Remove everything unused
docker system prune -a --volumes

# WARNING: Only prune volumes if you have backups!
```

#### 3. Check Container Logs Size
```bash
# Find large log files
docker ps -qa | xargs -I {} docker inspect --format='{{.Name}} {{.LogPath}}' {} | xargs -I {} ls -lh {}

# Clear logs
truncate -s 0 $(docker inspect --format='{{.LogPath}}' decred-pulse-dcrd)
```

#### 4. Move Docker Data Directory
```bash
# Stop Docker
sudo systemctl stop docker

# Edit Docker config
sudo nano /etc/docker/daemon.json

# Add:
{
  "data-root": "/new/path/to/docker"
}

# Restart
sudo systemctl start docker
```

#### 5. Backup and Clean Blockchain Data
```bash
# Backup (if needed)
make backup

# Remove and re-sync (last resort)
docker compose down
docker volume rm decred-pulse_dcrd-data
docker compose up -d
```

---

## 🌐 Frontend Issues

### Dashboard Not Loading

**Symptoms**:
- Blank page
- Loading spinner forever
- Connection errors

**Solutions**:

#### 1. Check Frontend Container
```bash
docker compose ps frontend

# Should show "Up" status
```

#### 2. Test Frontend URL
```bash
curl http://localhost:3000

# Should return HTML
```

#### 3. Check Browser Console
- Open DevTools (F12)
- Check Console tab for errors
- Check Network tab for failed requests

#### 4. Verify Backend Connection
```bash
# Test backend
curl http://localhost:8080/api/health

# Should return: {"status":"healthy"}
```

#### 5. Check CORS Settings
```bash
# Backend logs
docker compose logs backend | grep -i "cors\|origin"

# CORS should allow frontend origin
```

#### 6. Clear Browser Cache
- Hard refresh: Ctrl+Shift+R
- Clear cache and cookies
- Try incognito mode

#### 7. Rebuild Frontend
```bash
docker compose down frontend
docker compose build --no-cache frontend
docker compose up -d frontend
```

---

### API Requests Failing

**Symptoms**:
- 404 errors
- CORS errors
- Network timeout

**Solutions**:

#### 1. Verify Backend URL
```bash
# Check frontend API configuration
# Should point to correct backend URL

# For Docker: http://localhost:8080/api
# For production: Update VITE_API_URL
```

#### 2. Test API Directly
```bash
# Health check
curl http://localhost:8080/api/health

# Node status
curl http://localhost:8080/api/node/status

# Dashboard data
curl http://localhost:8080/api/dashboard
```

#### 3. Check Network Tab
- Open browser DevTools (F12)
- Go to Network tab
- Refresh page
- Check failed requests
- View request/response details

#### 4. Check Backend Logs
```bash
docker compose logs -f backend | grep -E "GET|POST|ERROR"
```

#### 5. Verify CORS
```bash
# Backend should allow frontend origin
docker compose logs backend | grep -i cors
```

#### 6. Restart Backend
```bash
docker compose restart backend frontend
```

---

## 🔐 Authentication Issues

### RPC Authentication Failed

**Symptoms**:
- "401 Unauthorized"
- "Authentication failed"
- Cannot connect to node/wallet

**Solutions**:

#### 1. Verify Credentials Match
```bash
# Check .env
cat .env | grep RPC

# Check dcrd command-line args
docker exec decred-pulse-dcrd ps aux | grep dcrd

# Check dcrwallet.conf
docker exec decred-pulse-dcrwallet cat /home/dcrwallet/.dcrwallet/dcrwallet.conf | grep rpc

# All should match!
```

#### 2. Recreate .env File
```bash
# Backup old one
cp .env .env.backup

# Create from example
cp env.example .env

# Edit with correct values
nano .env
```

#### 3. Update and Restart
```bash
# After editing .env
docker compose down
docker compose up -d

# Wait 1-2 minutes for initialization
```

#### 4. Check for Special Characters
```bash
# Passwords with special characters may need escaping
# Avoid: $ \ " ' ` (in passwords)
# Use: Letters, numbers, basic symbols
```

#### 5. Reset RPC Password
```bash
# Generate new password
openssl rand -base64 32

# Update .env
nano .env

# Restart services
docker compose restart dcrd dcrwallet backend
```

---

## 🚀 Performance Issues

### Slow Dashboard Loading

**Symptoms**:
- Long wait for data
- Timeout errors
- Sluggish interface

**Solutions**:

#### 1. Check System Resources
```bash
# View resource usage
docker stats

# Check:
# - CPU < 80%
# - Memory < 80%
# - No swap usage
```

#### 2. Check Network Latency
```bash
# Test backend response time
time curl http://localhost:8080/api/health

# Should be < 100ms
```

#### 3. Check RPC Response Time
```bash
# Test dcrd RPC
time docker exec decred-pulse-dcrd dcrctl \
  --rpcuser=decred \
  --rpcpass=your_password \
  --rpcserver=127.0.0.1:9109 \
  --rpccert=/certs/rpc.cert \
  getblockcount

# Should be < 500ms
```

#### 4. Reduce Polling Frequency
```bash
# Edit frontend polling interval
# frontend/src/pages/NodeDashboard.tsx
# Change: setInterval(fetchData, 30000) to 60000 (60s)
```

#### 5. Optimize Docker
```bash
# Allocate more resources to Docker
# Docker Desktop → Settings → Resources
# Increase:
# - CPUs: 2+
# - Memory: 4GB+
# - Disk: 20GB+
```

#### 6. Use Faster Disk
- Move Docker data to SSD
- Blockchain sync and operations much faster on SSD

---

### High Memory Usage

**Symptoms**:
- System slowing down
- Swap usage high
- Out of memory errors

**Solutions**:

#### 1. Check Memory Usage
```bash
# View memory by container
docker stats --no-stream

# System memory
free -h
```

#### 2. Restart Heavy Containers
```bash
# Restart dcrd (usually highest memory)
docker compose restart dcrd

# Monitor memory
watch docker stats
```

#### 3. Limit Container Memory
```bash
# Edit docker-compose.yml
nano docker-compose.yml

# Add memory limits:
services:
  dcrd:
    mem_limit: 2g
  dcrwallet:
    mem_limit: 1g
  backend:
    mem_limit: 512m
  frontend:
    mem_limit: 256m
```

#### 4. Optimize Node Configuration
```bash
# Edit .env to reduce resource usage
nano .env

# Reduce peers to lower bandwidth/memory usage:
DCRD_EXTRA_ARGS=--txindex --maxpeers=25

# Restart dcrd
docker compose restart dcrd
```

#### 5. Upgrade System Memory
- 4GB+ recommended for full stack
- 8GB+ for comfortable operation

---

## 📝 Logging Issues

### Logs Not Showing

**Symptoms**:
- `docker compose logs` shows nothing
- Empty log output
- Old timestamps

**Solutions**:

#### 1. Check Container is Running
```bash
docker compose ps

# If not running, logs won't exist
```

#### 2. View All Logs
```bash
# Without -f flag (follow)
docker compose logs dcrd

# Last 50 lines
docker compose logs --tail=50 dcrd
```

#### 3. Check Log Files Directly
```bash
# Enter container
docker exec -it decred-pulse-dcrd sh

# View log file
cat /home/dcrd/.dcrd/logs/mainnet/dcrd.log

# Exit
exit
```

#### 4. Check Docker Logging Driver
```bash
# Check Docker config
docker inspect decred-pulse-dcrd | grep LogConfig -A 5

# Should show json-file driver
```

#### 5. Increase Log Level
```bash
# Edit .env
nano .env

# Add to DCRD_EXTRA_ARGS:
DCRD_EXTRA_ARGS=--txindex --debuglevel=debug

# Restart
docker compose restart dcrd
```

---

### Logs Too Large

**Symptoms**:
- Disk space warning
- Slow log viewing
- Performance issues

**Solutions**:

#### 1. Check Log Sizes
```bash
# Find large log files
docker ps -qa | xargs -I {} sh -c 'echo -n "{}: "; docker inspect --format="{{.LogPath}}" {} | xargs ls -lh | awk "{print \$5}"'
```

#### 2. Truncate Logs
```bash
# Clear specific container logs
truncate -s 0 $(docker inspect --format='{{.LogPath}}' decred-pulse-dcrd)
```

#### 3. Configure Log Rotation
```bash
# Edit /etc/docker/daemon.json
sudo nano /etc/docker/daemon.json

# Add:
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}

# Restart Docker
sudo systemctl restart docker
```

#### 4. Use Logrotate
```bash
# Create logrotate config
sudo nano /etc/logrotate.d/docker-containers

# Add:
/var/lib/docker/containers/*/*.log {
  rotate 7
  daily
  compress
  size=10M
  missingok
  delaycompress
  copytruncate
}
```

---

## 🆘 Emergency Recovery

### Complete System Reset

**When all else fails**, nuclear option:

```bash
# 1. Backup important data
make backup  # If Makefile available

# 2. Stop everything
docker compose down

# 3. Remove all volumes (WARNING: DELETES DATA!)
docker volume rm decred-pulse_dcrd-data
docker volume rm decred-pulse_dcrwallet-data
docker volume rm decred-pulse_certs

# 4. Remove all images
docker rmi $(docker images -q 'decred-pulse*')

# 5. Clean Docker
docker system prune -a --volumes

# 6. Recreate .env
cp env.example .env
nano .env  # Set passwords

# 7. Start fresh
docker compose up -d --build

# 8. Monitor startup
docker compose logs -f
```

**Note**: This requires full blockchain re-sync (4-8 hours for mainnet).

---

## 📚 Getting More Help

### Still Stuck?

1. **Check documentation**:
   - [Quick Start](../quickstart.md)
   - [Docker Setup](../docker-setup.md)
   - [Wallet Operations](wallet-operations.md)
   - [Node Dashboard](../features/node-dashboard.md)
   - [Wallet Dashboard](../features/wallet-dashboard.md)

2. **Review logs carefully**:
   ```bash
   # Save logs for review
   docker compose logs > decred-pulse-logs.txt
   ```

3. **Search existing issues**:
   - GitHub Issues
   - Decred Matrix channels
   - Decred Discord

4. **Ask for help**:
   - [Decred Discord](https://discord.gg/decred)
   - [Decred Matrix](https://chat.decred.org)
   - [r/decred](https://reddit.com/r/decred)
   - GitHub Issues

### When Reporting Issues

**Include**:
- Decred Pulse version
- Operating system
- Docker version
- Relevant logs (last 50-100 lines)
- Steps to reproduce
- What you've already tried

**Example Issue Report**:
```
**Environment**:
- Decred Pulse: latest
- OS: Ubuntu 22.04
- Docker: 24.0.5
- Docker Compose: 2.20.0

**Problem**:
dcrd container unhealthy after startup

**Steps to Reproduce**:
1. docker compose up -d
2. Wait 2 minutes
3. docker compose ps shows "unhealthy"

**Logs**:
[paste last 50 lines of docker compose logs dcrd]

**What I've Tried**:
- Restarted container
- Checked RPC credentials
- Verified port 9109 is open
```

---

**Questions?** Check the [FAQ](../reference/faq.md) or ask in the [Decred Community](https://decred.org/community/)

