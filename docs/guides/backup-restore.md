# Backup & Restore Guide

This guide covers backing up and restoring your Decred Pulse data, including blockchain data, wallet data, Lightning channel state, Bison Relay identity, DCRDEX state, and configuration files.

## Table of Contents

- [What to Backup](#what-to-backup)
- [Backup Strategies](#backup-strategies)
- [Blockchain Data Backup](#blockchain-data-backup)
- [Wallet Data Backup](#wallet-data-backup)
- [Lightning (dcrlnd) Backup](#lightning-dcrlnd-backup)
- [Bison Relay (brclientd) Backup](#bison-relay-brclientd-backup)
- [DCRDEX Backup](#dcrdex-backup)
- [Configuration Backup](#configuration-backup)
- [Complete System Backup](#complete-system-backup)
- [Restore Procedures](#restore-procedures)
- [Automated Backups](#automated-backups)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## What to Backup

### Critical Data

**Must Backup (Can't be recovered):**
- Wallet seed phrase (if spending wallet)
- Lightning channel backup (SCB) if you run dcrlnd with open channels
- Bison Relay identity (brclientd data) if you use Bison Relay
- Private RPC credentials (`.env` file)
- Custom configuration files

**Should Backup (Saves time):**
- Blockchain data (dcrd - can be resynced)
- Wallet database (dcrwallet - can be recreated from seed)
- DCRDEX state (trade and account history)
- Certificates (auto-regenerated if missing)

**Optional Backup:**
- Docker images (can be rebuilt)
- Dashboard code (in git)

### Data Model

Decred Pulse stores daemon state under an `/app-data` mount. dcrd, dcrwallet,
and the shared control files live in a single `dcrpulse_app-data` volume, each
daemon under its own subdirectory. dcrlnd, brclientd, DCRDEX, the dashboard,
and Tor each use a dedicated volume.

```
dcrpulse/
|-- .env                          # RPC credentials (CRITICAL)
|-- docker-compose.yml            # Service configuration
|-- backups/                      # Backup directory
`-- Docker Volumes:
    |-- dcrpulse_app-data         # Shared volume, mounted at /app-data
    |     |-- dcrd/               #   Blockchain + dcrd TLS certs (~30 GB)
    |     |-- dcrwallet/          #   Wallet database
    |     `-- control/            #   Active-wallet / Tor pointers
    |-- dcrpulse_dcrlnd-data      # Lightning channel state (-> /app-data/dcrlnd)
    |-- dcrpulse_brclientd-data   # Bison Relay identity (-> /app-data/brclientd)
    |-- dcrpulse_dcrdex-data      # DCRDEX state (-> /dex/.dexc)
    |-- dcrpulse_dashboard-data   # Dashboard settings/themes (-> /dashboard-data)
    `-- dcrpulse_tor-data         # Tor data and onion keys (-> /app-data/tor)
```

The services in the stack are: `dcrd`, `dcrwallet`, `dcrlnd`, `brclientd`,
`dcrdex`, `tor`, and `dashboard`. Containers are named `dcrpulse-<service>`
(for example `dcrpulse-dcrwallet`).

---

## Backup Strategies

### Quick Backup (Essential only)
**Time:** < 1 minute
**Size:** < 1 KB
**Frequency:** After every config change

```bash
# Backup configuration
cp .env .env.backup
```

### Standard Backup (Config + Wallet)
**Time:** 1-2 minutes
**Size:** ~50 MB
**Frequency:** Weekly

```bash
# Backup wallet data
make backup-wallet
```

### Full Backup (Everything)
**Time:** 10-30 minutes
**Size:** ~30 GB
**Frequency:** Monthly or before major updates

```bash
# Backup the entire app-data volume (blockchain + wallet + control)
make backup
```

Note: `make backup` archives the shared `dcrpulse_app-data` volume only. The
dcrlnd, brclientd, and DCRDEX volumes are separate; back them up with the
manual commands in their sections below, or use the full backup script under
[Complete System Backup](#complete-system-backup).

---

## Blockchain Data Backup

### Using Make Command

The easiest way to back up the shared app-data volume (which includes the
blockchain and dcrd certificates):

```bash
# Backup all app data
make backup
```

This creates: `backups/app-data-backup-YYYYMMDD-HHMMSS.tar.gz`

**What's included:**
- Complete blockchain data (under `dcrd/`)
- Wallet database (under `dcrwallet/`)
- dcrd TLS certificates (under `dcrd/`)
- Active-wallet and Tor control pointers (under `control/`)

### Manual Docker Volume Backup

If you only want the blockchain subdirectory:

```bash
# Create backup directory
mkdir -p backups

# Backup dcrd data (blockchain + certs live under /app-data/dcrd)
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrd-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /app-data/dcrd .
```

### Verify Backup

```bash
# Check backup file size
ls -lh backups/

# Test backup integrity
tar -tzf backups/dcrd-backup-*.tar.gz > /dev/null && echo "Backup is valid" || echo "Backup is corrupted"
```

### When to Backup Blockchain

**You should backup dcrd data when:**
- Before major system upgrades
- Before changing hardware
- After initial sync completes (saves hours)
- Before testing experimental features

**You can skip backup if:**
- You have fast internet (can resync in hours)
- You're on testnet (smaller blockchain)
- Storage space is limited

---

## Wallet Data Backup

### Critical: Wallet Seed

**Most Important:** Your wallet seed phrase is the ONLY way to recover funds.

The seed is shown once, in the dashboard, when you create the wallet during
first-time setup. Write it down then.

**Save the seed phrase:**
1. Write it on paper (not digital)
2. Store in a secure location (fireproof safe)
3. Consider splitting across multiple locations
4. NEVER store online or in cloud
5. NEVER take photos of it

### Wallet Database Backup

The wallet database lives at `/app-data/dcrwallet` inside the shared volume:

```bash
# Create backup directory
mkdir -p backups

# Backup wallet database
make backup-wallet

# Or manually:
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/wallet-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /app-data/dcrwallet .
```

**What's included:**
- Wallet database (wallet.db)
- Imported xpub keys
- Address cache
- Transaction history
- Configuration

**Why backup the wallet database:**
- Saves rescan time after restore
- Preserves imported xpub keys
- Keeps transaction labels/notes
- Maintains address generation history

### Wallet Configuration

```bash
# Backup wallet configuration
docker exec dcrpulse-dcrwallet cat /app-data/dcrwallet/dcrwallet.conf > backups/dcrwallet.conf.backup
```

---

## Lightning (dcrlnd) Backup

If you run the Lightning Network integration with open channels, dcrlnd holds
channel state in the `dcrpulse_dcrlnd-data` volume (mounted at
`/app-data/dcrlnd`). The most important file for disaster recovery is the
Static Channel Backup (SCB), `channel.backup`. The SCB lets you recover
on-chain channel funds by forcing your peers to close after a total data loss.
It does NOT preserve off-chain balances or channel history.

```bash
# Create backup directory
mkdir -p backups

# Backup the entire dcrlnd data directory
docker run --rm \
  -v dcrpulse_dcrlnd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrlnd-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .
```

**What's included:**
- Static Channel Backup (`channel.backup`)
- Channel database
- dcrlnd TLS cert and admin macaroon

**Important:** The SCB changes every time a channel opens or closes. Back it up
again after any channel change. Never restore a stale channel database over a
live node; use the SCB recovery flow instead. Off-chain Lightning balances are
not recoverable from a backup alone.

---

## Bison Relay (brclientd) Backup

If you use Bison Relay, brclientd stores your relay identity and message state
in the `dcrpulse_brclientd-data` volume (mounted at `/app-data/brclientd`).
This identity cannot be regenerated; losing it means losing your Bison Relay
account and contacts.

```bash
# Create backup directory
mkdir -p backups

# Backup the entire brclientd data directory
docker run --rm \
  -v dcrpulse_brclientd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/brclientd-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .
```

**What's included:**
- Bison Relay identity (private key)
- Contact and group lists
- Local message and post history

The dashboard also provides an in-app Bison Relay backup and restore flow,
which produces a portable archive of the same state. Either method works;
keep at least one current copy.

---

## DCRDEX Backup

If you trade on DCRDEX, bisonw stores account and trade state in the
`dcrpulse_dcrdex-data` volume (mounted at `/dex/.dexc`). This state records
your registered DEX servers and trade history. It does not hold spendable
funds (those live in the dcrwallet database), but losing it loses your trade
records and active-order tracking.

```bash
# Create backup directory
mkdir -p backups

# Backup the entire DCRDEX data directory
docker run --rm \
  -v dcrpulse_dcrdex-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrdex-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .
```

**Important:** Back up DCRDEX state only when there are no active orders or
in-flight swaps, so the archive is consistent. Stop the dcrdex service first if
you want a clean snapshot:

```bash
docker compose stop dcrdex
# ... run the backup command above ...
docker compose up -d dcrdex
```

---

## Configuration Backup

### Environment Variables

Your `.env` file contains critical RPC credentials:

```bash
# Backup .env file
cp .env backups/.env.backup-$(date +%Y%m%d)

# Verify backup
cat backups/.env.backup-*
```

**What to backup from `.env`:**
- RPC usernames and passwords (dcrd, dcrwallet, dcrdex)
- Custom port configurations
- Network selection (mainnet/testnet)
- Gap limit settings
- Volume path overrides (`APP_DATA_DIR`, `DCRLND_DATA_DIR`, etc.)
- Extra dcrd/dcrwallet arguments

### Docker Compose Configuration

```bash
# Backup docker-compose.yml
cp docker-compose.yml backups/docker-compose.yml.backup-$(date +%Y%m%d)
```

### Complete Configuration Backup

```bash
# Backup all configuration files
mkdir -p backups/config-$(date +%Y%m%d)
cp .env backups/config-$(date +%Y%m%d)/
cp docker-compose.yml backups/config-$(date +%Y%m%d)/
cp env.example backups/config-$(date +%Y%m%d)/

# Create archive
tar czf backups/config-backup-$(date +%Y%m%d).tar.gz -C backups config-$(date +%Y%m%d)/
rm -rf backups/config-$(date +%Y%m%d)/

echo "Configuration backed up to backups/config-backup-$(date +%Y%m%d).tar.gz"
```

---

## Complete System Backup

### Full Backup Script

Create a backup of everything, including the separate dcrlnd, brclientd, and
DCRDEX volumes:

```bash
#!/bin/bash
# Full Decred Pulse backup script

BACKUP_DATE=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="backups/full-backup-${BACKUP_DATE}"

echo "Creating full backup..."
mkdir -p "${BACKUP_DIR}"

# 1. Configuration files
echo "Backing up configuration..."
cp .env "${BACKUP_DIR}/.env" 2>/dev/null || echo "No .env file"
cp docker-compose.yml "${BACKUP_DIR}/docker-compose.yml"

# 2. Shared app-data volume (blockchain + wallet + control)
echo "Backing up app-data (this may take a while)..."
docker run --rm \
  -v dcrpulse_app-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/app-data.tar.gz -C /data .

# 3. Lightning channel state
echo "Backing up dcrlnd..."
docker run --rm \
  -v dcrpulse_dcrlnd-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/dcrlnd-data.tar.gz -C /data . 2>/dev/null || echo "No dcrlnd volume"

# 4. Bison Relay identity
echo "Backing up brclientd..."
docker run --rm \
  -v dcrpulse_brclientd-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/brclientd-data.tar.gz -C /data . 2>/dev/null || echo "No brclientd volume"

# 5. DCRDEX state
echo "Backing up dcrdex..."
docker run --rm \
  -v dcrpulse_dcrdex-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/dcrdex-data.tar.gz -C /data . 2>/dev/null || echo "No dcrdex volume"

# 6. Create final archive
echo "Creating final archive..."
cd backups
tar czf "full-backup-${BACKUP_DATE}.tar.gz" "full-backup-${BACKUP_DATE}/"
rm -rf "full-backup-${BACKUP_DATE}/"
cd ..

echo "Full backup completed: backups/full-backup-${BACKUP_DATE}.tar.gz"
echo "Backup size:"
ls -lh "backups/full-backup-${BACKUP_DATE}.tar.gz"
```

Save as `backup-full.sh`, make executable, and run:

```bash
chmod +x backup-full.sh
./backup-full.sh
```

For a fully consistent archive, stop the stack first with `docker compose down`
and start it again with `docker compose up -d` after the script finishes.

---

## Restore Procedures

### Restore App Data (Blockchain + Wallet)

Using the make command:

```bash
# Restore the whole app-data volume from backup
make restore BACKUP=backups/app-data-backup-YYYYMMDD-HHMMSS.tar.gz
```

Manual restore:

```bash
# Stop services
docker compose down

# Restore the app-data volume
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /app-data/* && tar xzf /backup/app-data-backup-YYYYMMDD-HHMMSS.tar.gz -C /app-data"

# Start services
docker compose up -d

# Verify
make sync-status
```

### Restore Wallet Data

```bash
# Restore just the wallet subdirectory from a wallet backup
make restore-wallet BACKUP=backups/wallet-backup-YYYYMMDD-HHMMSS.tar.gz
```

Manual restore:

```bash
# Stop wallet
docker compose stop dcrwallet

# Restore wallet database into /app-data/dcrwallet
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /app-data/dcrwallet/* && tar xzf /backup/wallet-backup-YYYYMMDD-HHMMSS.tar.gz -C /app-data/dcrwallet"

# Start wallet
docker compose up -d dcrwallet

# Verify
make wallet-info
```

### Restore Lightning (dcrlnd)

Do not restore a stale channel database over a running node. Restore the full
data directory only on a fresh setup; for recovery after data loss, use the
Static Channel Backup (SCB) flow so peers force-close and return your on-chain
funds.

```bash
# Stop dcrlnd
docker compose stop dcrlnd

# Restore the dcrlnd data directory
docker run --rm \
  -v dcrpulse_dcrlnd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrlnd-backup-YYYYMMDD-HHMMSS.tar.gz -C /data"

# Start dcrlnd
docker compose up -d dcrlnd
```

### Restore Bison Relay (brclientd)

```bash
# Stop brclientd
docker compose stop brclientd

# Restore the brclientd data directory
docker run --rm \
  -v dcrpulse_brclientd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/brclientd-backup-YYYYMMDD-HHMMSS.tar.gz -C /data"

# Start brclientd
docker compose up -d brclientd
```

You can also restore through the dashboard's in-app Bison Relay restore flow if
you backed up with that tool.

### Restore DCRDEX

```bash
# Stop dcrdex
docker compose stop dcrdex

# Restore the DCRDEX data directory
docker run --rm \
  -v dcrpulse_dcrdex-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrdex-backup-YYYYMMDD-HHMMSS.tar.gz -C /data"

# Start dcrdex
docker compose up -d dcrdex
```

### Restore Configuration

```bash
# Restore .env file
cp backups/.env.backup-YYYYMMDD .env

# Restart services to apply
docker compose restart
```

### Restore from Seed Phrase

If you lost everything but have your seed phrase, recreate the wallet through
the dashboard. After a clean install, open the dashboard and choose the
"restore from seed" option in the wallet setup wizard, then enter your seed.
The wallet rescans the blockchain to rediscover your funds.

If you need to wipe the existing wallet first:

```bash
# Remove only the wallet data (the blockchain stays intact)
make clean-dcrwallet

# Then open the dashboard and restore from seed in the setup wizard
```

### Complete System Restore

Restore from a full backup produced by `backup-full.sh`:

```bash
# Extract full backup
cd backups
tar xzf full-backup-YYYYMMDD-HHMMSS.tar.gz
cd full-backup-YYYYMMDD-HHMMSS

# Stop all services
docker compose down

# Restore configuration
cp .env ../../.env
cp docker-compose.yml ../../docker-compose.yml

# Restore the shared app-data volume
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /app-data/* && tar xzf /backup/app-data.tar.gz -C /app-data"

# Restore dcrlnd (if present)
docker run --rm \
  -v dcrpulse_dcrlnd-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrlnd-data.tar.gz -C /data" 2>/dev/null || true

# Restore brclientd (if present)
docker run --rm \
  -v dcrpulse_brclientd-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/brclientd-data.tar.gz -C /data" 2>/dev/null || true

# Restore dcrdex (if present)
docker run --rm \
  -v dcrpulse_dcrdex-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrdex-data.tar.gz -C /data" 2>/dev/null || true

# Start services
cd ../..
docker compose up -d

echo "Full restore completed"
```

---

## Automated Backups

### Cron Job Setup

Create automated daily backups:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * cd /path/to/dcrpulse && make backup >> /var/log/dcrpulse-backup.log 2>&1

# Add weekly wallet backup on Sundays at 3 AM
0 3 * * 0 cd /path/to/dcrpulse && make backup-wallet >> /var/log/dcrpulse-backup.log 2>&1

# Add monthly config backup on 1st of month at 1 AM
0 1 1 * * cd /path/to/dcrpulse && cp .env backups/.env.backup-$(date +\%Y\%m\%d) >> /var/log/dcrpulse-backup.log 2>&1
```

### Backup Rotation Script

Automatically clean old backups:

```bash
#!/bin/bash
# backup-rotate.sh - Keep only last N backups

BACKUP_DIR="backups"
KEEP_DAILY=7      # Keep 7 daily backups
KEEP_WEEKLY=4     # Keep 4 weekly backups
KEEP_MONTHLY=3    # Keep 3 monthly backups

# Remove daily app-data backups older than 7 days
find ${BACKUP_DIR} -name "app-data-backup-*.tar.gz" -mtime +${KEEP_DAILY} -delete

# Remove weekly wallet backups older than 4 weeks
find ${BACKUP_DIR} -name "wallet-backup-*.tar.gz" -mtime +$((KEEP_WEEKLY * 7)) -delete

# Remove monthly full backups older than 3 months
find ${BACKUP_DIR} -name "full-backup-*.tar.gz" -mtime +$((KEEP_MONTHLY * 30)) -delete

echo "Old backups rotated"
```

Add to cron:

```bash
# Run daily at 4 AM
0 4 * * * /path/to/dcrpulse/backup-rotate.sh >> /var/log/dcrpulse-backup.log 2>&1
```

### Systemd Timer (Alternative to Cron)

Create `/etc/systemd/system/dcrpulse-backup.service`:

```ini
[Unit]
Description=Decred Pulse Backup
After=docker.service

[Service]
Type=oneshot
User=your-user
WorkingDirectory=/path/to/dcrpulse
ExecStart=/usr/bin/make backup
```

Create `/etc/systemd/system/dcrpulse-backup.timer`:

```ini
[Unit]
Description=Daily Decred Pulse Backup
Requires=dcrpulse-backup.service

[Timer]
OnCalendar=daily
OnCalendar=02:00
Persistent=true

[Install]
WantedBy=timers.target
```

Enable:

```bash
sudo systemctl daemon-reload
sudo systemctl enable dcrpulse-backup.timer
sudo systemctl start dcrpulse-backup.timer

# Check status
sudo systemctl status dcrpulse-backup.timer
```

---

## Best Practices

### Backup Checklist

**Daily:**
- Configuration changes are backed up immediately
- Automated backups are running

**Weekly:**
- Verify backup integrity
- Check backup log for errors
- Rotate old backups

**Monthly:**
- Test restore procedure
- Full system backup
- Verify wallet seed is still accessible

**After any Lightning channel change:**
- Back up dcrlnd (the SCB changes on every open/close)

**Before Major Changes:**
- Full backup
- Verify backup integrity
- Document current state

### Storage Recommendations

**Local Storage:**
- Separate drive from system disk
- RAID configuration for redundancy
- Regular integrity checks

**Remote Backup:**
- Encrypt sensitive data before upload
- Use multiple providers for redundancy
- Test restore from remote location

**Offline Backup:**
- External USB drives
- Rotate between multiple drives
- Store in different physical locations

### Security Considerations

**Encrypt Sensitive Backups:**

```bash
# Encrypt backup
gpg --symmetric --cipher-algo AES256 backups/app-data-backup-*.tar.gz

# Decrypt when needed
gpg --decrypt backups/app-data-backup-*.tar.gz.gpg > app-data-backup-restored.tar.gz
```

**Secure Backup Locations:**
- Don't backup to public cloud without encryption
- Limit access to backup directory
- Use separate credentials for backup storage

**What NOT to Backup Online (without encryption):**
- Wallet seed phrase (paper only!)
- Bison Relay identity key
- Lightning channel database
- Unencrypted private keys
- Unencrypted RPC credentials

---

## Troubleshooting

### Backup Fails with "No Space Left"

```bash
# Check available space
df -h

# Remove old backups
rm backups/app-data-backup-*.tar.gz

# Or clean Docker cache
docker system prune -a
```

### Restore Fails with "Permission Denied"

```bash
# Fix volume permissions (daemons run as UID 1000)
docker run --rm -v dcrpulse_app-data:/data alpine chown -R 1000:1000 /data

# Retry restore
make restore BACKUP=backups/app-data-backup-*.tar.gz
```

### Backup is Corrupted

```bash
# Verify backup
tar -tzf backups/app-data-backup-*.tar.gz

# If corrupted, try previous backup
ls -lt backups/
make restore BACKUP=backups/app-data-backup-[previous-date].tar.gz
```

### Can't Find Wallet Seed

The seed is shown only once, in the dashboard, when the wallet is first
created. It is never written to disk or to container logs.

```bash
# If you did not record the seed and the wallet still exists, your funds are
# still accessible through the running wallet, but you have no recovery phrase.
# Move funds to a new wallet whose seed you do record:
# 1. Create a new wallet (new seed) and write the seed down
# 2. Send all funds from the old wallet to the new wallet's address
```

### Restore Works But Data is Old

```bash
# After restore, the wallet may need a rescan to catch up
make wallet-info

# Trigger a rescan from the wallet dashboard (Wallet > rescan),
# which rediscovers transactions from the blockchain.
```

### Backup Takes Too Long

```bash
# Use faster compression (no gzip - larger but quicker)
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine tar cf /backup/app-data-backup-$(date +%Y%m%d).tar -C /app-data .

# Or exclude log files
docker run --rm \
  -v dcrpulse_app-data:/app-data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/app-data-backup-$(date +%Y%m%d).tar.gz \
  --exclude='*.tmp' \
  --exclude='*/logs/*' \
  -C /app-data .
```

---

## Disaster Recovery Plan

### Complete Data Loss Scenario

If you lose everything:

**With Seed Phrase:**
1. Reinstall Decred Pulse
2. Restore wallet from seed in the dashboard setup wizard
3. Wait for blockchain resync (6-12 hours)
4. Import xpub keys if needed
5. Rescan wallet

**With Backup:**
1. Reinstall Decred Pulse
2. Restore configuration files
3. Restore the app-data volume (blockchain + wallet)
4. Restore dcrlnd, brclientd, and dcrdex volumes if you use them
5. Verify balances

**Without Seed or Backup:**
- Watch-only wallets can be recreated (just import the xpub again)
- Spending wallets: Funds are PERMANENTLY LOST
- Lightning: only on-chain channel funds may be recoverable, and only if you
  have a Static Channel Backup
- Bison Relay: the identity is unrecoverable without a backup

### Migration to New Server

```bash
# 1. On old server: Create full backup
./backup-full.sh

# 2. Copy backup to new server
scp backups/full-backup-*.tar.gz newserver:/path/to/dcrpulse/backups/

# 3. On new server: Install Decred Pulse
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# 4. Restore from backup (see Complete System Restore above)
cd backups
tar xzf full-backup-*.tar.gz
# ... follow the complete restore procedure ...

# 5. Start services
cd ../..
docker compose up -d

# 6. Verify
make status
```

---

## Related Documentation

- **[Installation Guide](../getting-started/installation.md)** - Initial setup
- **[Configuration Guide](../setup/configuration.md)** - Environment variables
- **[CLI Commands Reference](../reference/cli-commands.md)** - All make commands
- **[Troubleshooting](troubleshooting.md)** - Common issues
- **[Security Best Practices](../deployment/security.md)** - Production security

---

## Quick Reference

### Essential Commands

```bash
# Backup all app data (blockchain + wallet + control)
make backup

# Restore all app data
make restore BACKUP=backups/app-data-backup-*.tar.gz

# Backup wallet only
make backup-wallet

# Restore wallet only
make restore-wallet BACKUP=backups/wallet-backup-*.tar.gz

# Backup configuration
cp .env .env.backup

# Clean old backups
find backups/ -name "*.tar.gz" -mtime +30 -delete
```

### Recovery Priority

1. **Wallet Seed** - Write it down immediately after wallet creation
2. **Lightning SCB** - Re-back up after every channel open/close
3. **Bison Relay identity** - Back up the brclientd volume if you use Bison Relay
4. **RPC Credentials** - Back up the `.env` file after setup
5. **Wallet Database** - Weekly backups
6. **Blockchain Data** - Monthly backups (optional, resyncable)

---

**Remember:** The wallet seed phrase is the most important thing to back up.
Everything else can be recreated or resynced, but without the seed, funds in a
spending wallet are permanently lost. If you run Lightning or Bison Relay, also
back up the dcrlnd Static Channel Backup and the brclientd identity, because
those cannot be regenerated either.

**Made for the Decred community.**
