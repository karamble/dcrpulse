# Backup & Restore Guide

This guide covers backing up and restoring your Decred Pulse data, including blockchain data, wallet data, and configuration files.

## 📋 Table of Contents

- [What to Backup](#what-to-backup)
- [Backup Strategies](#backup-strategies)
- [Blockchain Data Backup](#blockchain-data-backup)
- [Wallet Data Backup](#wallet-data-backup)
- [Configuration Backup](#configuration-backup)
- [Complete System Backup](#complete-system-backup)
- [Restore Procedures](#restore-procedures)
- [Automated Backups](#automated-backups)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## What to Backup

### Critical Data

**🔴 Must Backup (Can't be recovered):**
- Wallet seed phrase (if spending wallet)
- Private RPC credentials (`.env` file)
- Custom configuration files

**🟡 Should Backup (Saves time):**
- Blockchain data (dcrd - can be resynced)
- Wallet database (dcrwallet - can be recreated from seed)
- Certificates (auto-regenerated if missing)

**🟢 Optional Backup:**
- Docker images (can be rebuilt)
- Frontend/backend code (in git)

### Data Locations

```
decred-pulse/
├── .env                          # RPC credentials (CRITICAL)
├── docker-compose.yml            # Service configuration
├── backups/                      # Backup directory
└── Docker Volumes:
    ├── dcrd-data                 # Blockchain (~12GB)
    ├── dcrd-certs                # TLS certificates
    └── dcrwallet-data            # Wallet database
```

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
**Size:** ~12 GB  
**Frequency:** Monthly or before major updates

```bash
# Backup blockchain + wallet + config
make backup
```

---

## Blockchain Data Backup

### Using Make Command

The easiest way to backup blockchain data:

```bash
# Backup dcrd data
make backup
```

This creates: `backups/dcrd-backup-YYYYMMDD-HHMMSS.tar.gz`

**What's included:**
- Complete blockchain data
- Block index
- Transaction index (if enabled)
- Address index (if enabled)

### Manual Docker Volume Backup

If you need more control:

```bash
# Create backup directory
mkdir -p backups

# Backup dcrd data volume
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrd-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .

# Backup certificates
docker run --rm \
  -v decred-pulse_dcrd-certs:/certs \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrd-certs-$(date +%Y%m%d-%H%M%S).tar.gz -C /certs .
```

### Verify Backup

```bash
# Check backup file size
ls -lh backups/

# Test backup integrity
tar -tzf backups/dcrd-backup-*.tar.gz > /dev/null && echo "✅ Backup is valid" || echo "❌ Backup is corrupted"
```

### When to Backup Blockchain

**You should backup dcrd data when:**
- ✅ Before major system upgrades
- ✅ Before changing hardware
- ✅ After initial sync completes (saves hours)
- ✅ Before testing experimental features

**You can skip backup if:**
- ❌ You have fast internet (can resync in hours)
- ❌ You're on testnet (smaller blockchain)
- ❌ Storage space is limited

---

## Wallet Data Backup

### Critical: Wallet Seed

**Most Important:** Your wallet seed phrase is the ONLY way to recover funds.

```bash
# View wallet seed (first-time setup only)
make wallet-seed
```

**Save the seed phrase:**
1. Write it on paper (not digital)
2. Store in a secure location (fireproof safe)
3. Consider splitting across multiple locations
4. NEVER store online or in cloud
5. NEVER take photos of it

### Wallet Database Backup

For watch-only wallets with imported xpub keys:

```bash
# Create backup directory
mkdir -p backups

# Backup wallet database
docker run --rm \
  -v decred-pulse_dcrwallet-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrwallet-backup-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .
```

**What's included:**
- Wallet database (wallet.db)
- Imported xpub keys
- Address cache
- Transaction history
- Configuration

**Why backup wallet.db:**
- Saves rescan time after restore
- Preserves imported xpub keys
- Keeps transaction labels/notes
- Maintains address generation history

### Wallet Configuration

```bash
# Backup wallet configuration
docker exec decred-pulse-dcrwallet cat /root/.dcrwallet/dcrwallet.conf > backups/dcrwallet.conf.backup
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
- RPC usernames and passwords
- Custom port configurations
- Network selection (mainnet/testnet)
- Gap limit settings
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

echo "✅ Configuration backed up to backups/config-backup-$(date +%Y%m%d).tar.gz"
```

---

## Complete System Backup

### Full Backup Script

Create a backup of everything:

```bash
#!/bin/bash
# Full Decred Pulse backup script

BACKUP_DATE=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="backups/full-backup-${BACKUP_DATE}"

echo "🔄 Creating full backup..."
mkdir -p "${BACKUP_DIR}"

# 1. Configuration files
echo "📄 Backing up configuration..."
cp .env "${BACKUP_DIR}/.env" 2>/dev/null || echo "No .env file"
cp docker-compose.yml "${BACKUP_DIR}/docker-compose.yml"

# 2. Blockchain data
echo "⛓️  Backing up blockchain data (this may take a while)..."
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/dcrd-data.tar.gz -C /data .

# 3. Certificates
echo "🔐 Backing up certificates..."
docker run --rm \
  -v decred-pulse_dcrd-certs:/certs \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/dcrd-certs.tar.gz -C /certs .

# 4. Wallet data
echo "💰 Backing up wallet data..."
docker run --rm \
  -v decred-pulse_dcrwallet-data:/data \
  -v $(pwd)/${BACKUP_DIR}:/backup \
  alpine tar czf /backup/dcrwallet-data.tar.gz -C /data .

# 5. Create final archive
echo "📦 Creating final archive..."
cd backups
tar czf "full-backup-${BACKUP_DATE}.tar.gz" "full-backup-${BACKUP_DATE}/"
rm -rf "full-backup-${BACKUP_DATE}/"
cd ..

echo "✅ Full backup completed: backups/full-backup-${BACKUP_DATE}.tar.gz"
echo "📊 Backup size:"
ls -lh "backups/full-backup-${BACKUP_DATE}.tar.gz"
```

Save as `backup-full.sh`, make executable, and run:

```bash
chmod +x backup-full.sh
./backup-full.sh
```

---

## Restore Procedures

### Restore Blockchain Data

Using the make command:

```bash
# Restore from backup
make restore BACKUP=backups/dcrd-backup-YYYYMMDD-HHMMSS.tar.gz
```

Manual restore:

```bash
# Stop services
docker compose down

# Restore dcrd data
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrd-backup-YYYYMMDD-HHMMSS.tar.gz -C /data"

# Start services
docker compose up -d

# Verify
make sync-status
```

### Restore Wallet Data

```bash
# Stop wallet
docker compose stop dcrwallet

# Restore wallet database
docker run --rm \
  -v decred-pulse_dcrwallet-data:/data \
  -v $(pwd)/backups:/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrwallet-backup-YYYYMMDD-HHMMSS.tar.gz -C /data"

# Start wallet
docker compose up -d dcrwallet

# Verify
make wallet-info
```

### Restore Configuration

```bash
# Restore .env file
cp backups/.env.backup-YYYYMMDD .env

# Restart services to apply
docker compose restart
```

### Restore from Seed Phrase

If you lost everything but have your seed phrase:

```bash
# 1. Clean existing wallet
make clean-dcrwallet

# 2. Start fresh wallet
docker compose up -d dcrwallet

# 3. Access wallet container
docker exec -it decred-pulse-dcrwallet /bin/sh

# 4. Stop wallet daemon
pkill dcrwallet

# 5. Create new wallet from seed
dcrwallet --create

# Follow prompts:
# - Enter new private passphrase
# - Choose "existing seed"
# - Enter your 33-word seed phrase
# - Confirm seed

# 6. Exit and restart
exit
docker compose restart dcrwallet
```

### Complete System Restore

Restore from a full backup:

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

# Restore dcrd data
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrd-data.tar.gz -C /data"

# Restore certificates
docker run --rm \
  -v decred-pulse_dcrd-certs:/certs \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /certs/* && tar xzf /backup/dcrd-certs.tar.gz -C /certs"

# Restore wallet
docker run --rm \
  -v decred-pulse_dcrwallet-data:/data \
  -v $(pwd):/backup \
  alpine sh -c "rm -rf /data/* && tar xzf /backup/dcrwallet-data.tar.gz -C /data"

# Start services
cd ../..
docker compose up -d

echo "✅ Full restore completed"
```

---

## Automated Backups

### Cron Job Setup

Create automated daily backups:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * cd /path/to/decred-pulse && make backup >> /var/log/decred-pulse-backup.log 2>&1

# Add weekly wallet backup on Sundays at 3 AM
0 3 * * 0 cd /path/to/decred-pulse && docker run --rm -v decred-pulse_dcrwallet-data:/data -v $(pwd)/backups:/backup alpine tar czf /backup/dcrwallet-backup-$(date +\%Y\%m\%d).tar.gz -C /data . >> /var/log/decred-pulse-backup.log 2>&1

# Add monthly config backup on 1st of month at 1 AM
0 1 1 * * cd /path/to/decred-pulse && cp .env backups/.env.backup-$(date +\%Y\%m\%d) >> /var/log/decred-pulse-backup.log 2>&1
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

# Remove daily backups older than 7 days
find ${BACKUP_DIR} -name "dcrd-backup-*.tar.gz" -mtime +${KEEP_DAILY} -delete

# Remove weekly backups older than 4 weeks
find ${BACKUP_DIR} -name "dcrwallet-backup-*.tar.gz" -mtime +$((KEEP_WEEKLY * 7)) -delete

# Remove monthly backups older than 3 months
find ${BACKUP_DIR} -name "full-backup-*.tar.gz" -mtime +$((KEEP_MONTHLY * 30)) -delete

echo "✅ Old backups rotated"
```

Add to cron:

```bash
# Run daily at 4 AM
0 4 * * * /path/to/decred-pulse/backup-rotate.sh >> /var/log/decred-pulse-backup.log 2>&1
```

### Systemd Timer (Alternative to Cron)

Create `/etc/systemd/system/decred-pulse-backup.service`:

```ini
[Unit]
Description=Decred Pulse Backup
After=docker.service

[Service]
Type=oneshot
User=your-user
WorkingDirectory=/path/to/decred-pulse
ExecStart=/usr/bin/make backup
```

Create `/etc/systemd/system/decred-pulse-backup.timer`:

```ini
[Unit]
Description=Daily Decred Pulse Backup
Requires=decred-pulse-backup.service

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
sudo systemctl enable decred-pulse-backup.timer
sudo systemctl start decred-pulse-backup.timer

# Check status
sudo systemctl status decred-pulse-backup.timer
```

---

## Best Practices

### Backup Checklist

**Daily:**
- ✅ Configuration changes are backed up immediately
- ✅ Automated backups are running

**Weekly:**
- ✅ Verify backup integrity
- ✅ Check backup log for errors
- ✅ Rotate old backups

**Monthly:**
- ✅ Test restore procedure
- ✅ Full system backup
- ✅ Verify wallet seed is still accessible

**Before Major Changes:**
- ✅ Full backup
- ✅ Verify backup integrity
- ✅ Document current state

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
gpg --symmetric --cipher-algo AES256 backups/dcrd-backup-*.tar.gz

# Decrypt when needed
gpg --decrypt backups/dcrd-backup-*.tar.gz.gpg > dcrd-backup-restored.tar.gz
```

**Secure Backup Locations:**
- Don't backup to public cloud without encryption
- Limit access to backup directory
- Use separate credentials for backup storage

**What NOT to Backup Online:**
- Wallet seed phrase (paper only!)
- Unencrypted private keys
- Unencrypted RPC credentials

---

## Troubleshooting

### Backup Fails with "No Space Left"

```bash
# Check available space
df -h

# Remove old backups
rm backups/dcrd-backup-*.tar.gz

# Or clean Docker cache
docker system prune -a
```

### Restore Fails with "Permission Denied"

```bash
# Fix volume permissions
docker run --rm -v decred-pulse_dcrd-data:/data alpine chmod -R 777 /data

# Retry restore
make restore BACKUP=backups/dcrd-backup-*.tar.gz
```

### Backup is Corrupted

```bash
# Verify backup
tar -tzf backups/dcrd-backup-*.tar.gz

# If corrupted, try previous backup
ls -lt backups/
make restore BACKUP=backups/dcrd-backup-[previous-date].tar.gz
```

### Can't Find Wallet Seed

```bash
# Check container logs
docker logs decred-pulse-dcrwallet 2>&1 | grep -A 40 "seed"

# If not found, check older logs (if rotated)
# Seed is only shown once during first wallet creation

# If completely lost, you'll need to:
# 1. Create new wallet (new seed)
# 2. Transfer funds from old wallet using recovery phrase
```

### Restore Works But Data is Old

```bash
# After restore, rescan to get latest data
make wallet-info

# Trigger rescan from wallet dashboard
# Or via API:
curl -X POST http://localhost:8080/api/wallet/rescan \
  -H "Content-Type: application/json" \
  -d '{"beginHeight": 0}'
```

### Backup Takes Too Long

```bash
# Use faster compression
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar cf /backup/dcrd-backup-$(date +%Y%m%d).tar -C /data .
# (No gzip compression - faster but larger)

# Or exclude unnecessary files
docker run --rm \
  -v decred-pulse_dcrd-data:/data \
  -v $(pwd)/backups:/backup \
  alpine tar czf /backup/dcrd-backup-$(date +%Y%m%d).tar.gz \
  --exclude='*.tmp' \
  --exclude='*.log' \
  -C /data .
```

---

## Disaster Recovery Plan

### Complete Data Loss Scenario

If you lose everything:

**With Seed Phrase:**
1. ✅ Reinstall Decred Pulse
2. ✅ Restore wallet from seed
3. ✅ Wait for blockchain resync (6-12 hours)
4. ✅ Import xpub keys if needed
5. ✅ Rescan wallet

**With Backup:**
1. ✅ Reinstall Decred Pulse
2. ✅ Restore configuration files
3. ✅ Restore blockchain data
4. ✅ Restore wallet data
5. ✅ Verify balances

**Without Seed or Backup:**
- ❌ Watch-only wallets can be recreated (just import xpub again)
- ❌ Spending wallets: Funds are PERMANENTLY LOST

### Migration to New Server

```bash
# 1. On old server: Create full backup
./backup-full.sh

# 2. Copy backup to new server
scp backups/full-backup-*.tar.gz newserver:/path/to/decred-pulse/backups/

# 3. On new server: Install Decred Pulse
git clone https://github.com/karamble/decred-pulse.git
cd decred-pulse

# 4. Restore from backup
tar xzf backups/full-backup-*.tar.gz
cd full-backup-*
cp .env ../../.env
# ... (follow complete restore procedure above)

# 5. Start services
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
# Backup blockchain
make backup

# Restore blockchain
make restore BACKUP=backups/dcrd-backup-*.tar.gz

# View wallet seed
make wallet-seed

# Backup configuration
cp .env .env.backup

# Clean old backups
find backups/ -name "*.tar.gz" -mtime +30 -delete
```

### Recovery Priority

1. **Wallet Seed** - Write down immediately after wallet creation
2. **RPC Credentials** - Backup `.env` file after setup
3. **Wallet Database** - Weekly backups
4. **Blockchain Data** - Monthly backups (optional)

---

**Remember:** The wallet seed phrase is the most important thing to backup. Everything else can be recreated or resynced, but without the seed, funds in a spending wallet are permanently lost.

**Made with ❤️ for the Decred community**

