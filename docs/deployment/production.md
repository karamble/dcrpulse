# Production Deployment

Guide to deploying Decred Pulse in a production environment with best practices for security, reliability, and performance.

## 🎯 Deployment Options

### Option 1: Docker Compose (Recommended)

**Pros**:
- ✅ Easiest to deploy and maintain
- ✅ Isolated environments
- ✅ Easy updates and rollbacks
- ✅ Consistent across environments

**Cons**:
- ⚠️ Requires Docker knowledge
- ⚠️ Overhead from containerization

**Best for**: Most production deployments

---

### Option 2: Systemd Services

**Pros**:
- ✅ Native OS integration
- ✅ Lower overhead
- ✅ Standard Linux management

**Cons**:
- ⚠️ More manual configuration
- ⚠️ Less portable

**Best for**: Bare metal servers, performance-critical deployments

---

### Option 3: Kubernetes

**Pros**:
- ✅ High availability
- ✅ Auto-scaling
- ✅ Advanced orchestration

**Cons**:
- ⚠️ Complex setup
- ⚠️ Requires K8s expertise
- ⚠️ Overkill for single-instance

**Best for**: Large-scale deployments, multiple instances

---

## 🚀 Production Deployment with Docker Compose

### Prerequisites

**Server Requirements**:
- Ubuntu 22.04 LTS (recommended) or similar
- 4+ CPU cores
- 8+ GB RAM
- 30+ GB SSD storage
- Static IP address
- Domain name (optional but recommended)

**Software**:
- Docker 24.0+
- Docker Compose 2.20+
- Nginx (for reverse proxy)
- UFW or iptables (firewall)

---

### Step 1: Server Setup

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Install Docker Compose
sudo apt install docker-compose-plugin

# Install utilities
sudo apt install -y git nginx certbot python3-certbot-nginx ufw

# Reboot to apply changes
sudo reboot
```

---

### Step 2: Firewall Configuration

```bash
# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS (for web access)
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow dcrd P2P (for peer connections)
sudo ufw allow 9108/tcp

# Enable firewall
sudo ufw enable

# Verify
sudo ufw status
```

---

### Step 3: Clone and Configure

```bash
# Create application directory
sudo mkdir -p /opt/decred-pulse
sudo chown $USER:$USER /opt/decred-pulse
cd /opt/decred-pulse

# Clone repository
git clone https://github.com/karamble/decred-pulse.git .

# Create .env file
cp env.example .env

# Generate secure passwords
DCRD_PASS=$(openssl rand -base64 32)
WALLET_PASS=$(openssl rand -base64 32)

# Update .env with secure credentials
cat > .env << EOF
# Production RPC Credentials
DCRD_RPC_USER=dcrd_prod_$(openssl rand -hex 8)
DCRD_RPC_PASS=$DCRD_PASS

DCRWALLET_RPC_USER=wallet_prod_$(openssl rand -hex 8)
DCRWALLET_RPC_PASS=$WALLET_PASS

# Use stable release
DCRD_VERSION=release-v2.0.6
DCRWALLET_VERSION=release-v2.0.6

# Gap limit
DCRWALLET_GAP_LIMIT=200

# Enable transaction indexing
DCRD_EXTRA_ARGS=--txindex
EOF

# Secure .env file
chmod 600 .env
```

---

### Step 4: Production Docker Compose

Create optimized `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  dcrd:
    build:
      context: ./dcrd
      args:
        DCRD_VERSION: ${DCRD_VERSION:-release-v2.0.6}
    container_name: decred-pulse-dcrd-prod
    restart: always
    volumes:
      - dcrd-data:/home/dcrd/.dcrd
      - certs:/certs
    ports:
      - "9108:9108"  # P2P
    networks:
      - decred-network
    environment:
      - DCRD_RPC_USER=${DCRD_RPC_USER}
      - DCRD_RPC_PASS=${DCRD_RPC_PASS}
    mem_limit: 2g
    cpus: 2.0
    healthcheck:
      test: ["CMD", "dcrctl", "--rpcuser=${DCRD_RPC_USER}", "--rpcpass=${DCRD_RPC_PASS}", "--rpcserver=127.0.0.1:9109", "--rpccert=/certs/rpc.cert", "getblockcount"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 120s
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  dcrwallet:
    build:
      context: ./dcrwallet
      args:
        DCRWALLET_VERSION: ${DCRWALLET_VERSION:-release-v2.0.6}
    container_name: decred-pulse-dcrwallet-prod
    restart: always
    depends_on:
      dcrd:
        condition: service_healthy
    volumes:
      - dcrwallet-data:/home/dcrwallet/.dcrwallet
      - certs:/certs:ro
    networks:
      - decred-network
    environment:
      - DCRWALLET_RPC_USER=${DCRWALLET_RPC_USER}
      - DCRWALLET_RPC_PASS=${DCRWALLET_RPC_PASS}
      - DCRD_RPC_USER=${DCRD_RPC_USER}
      - DCRD_RPC_PASS=${DCRD_RPC_PASS}
      - DCRWALLET_GAP_LIMIT=${DCRWALLET_GAP_LIMIT:-200}
    mem_limit: 1g
    cpus: 1.0
    healthcheck:
      test: ["CMD", "dcrctl", "--wallet", "--rpcuser=${DCRWALLET_RPC_USER}", "--rpcpass=${DCRWALLET_RPC_PASS}", "--rpcserver=127.0.0.1:9110", "--rpccert=/certs/rpc.cert", "walletinfo"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  backend:
    build:
      context: ./backend
    container_name: decred-pulse-backend-prod
    restart: always
    depends_on:
      dcrd:
        condition: service_healthy
      dcrwallet:
        condition: service_healthy
    volumes:
      - certs:/certs:ro
    networks:
      - decred-network
    environment:
      - PORT=8080
      - DCRD_RPC_HOST=dcrd
      - DCRD_RPC_PORT=9109
      - DCRD_RPC_USER=${DCRD_RPC_USER}
      - DCRD_RPC_PASS=${DCRD_RPC_PASS}
      - DCRWALLET_RPC_HOST=dcrwallet
      - DCRWALLET_RPC_PORT=9110
      - DCRWALLET_RPC_USER=${DCRWALLET_RPC_USER}
      - DCRWALLET_RPC_PASS=${DCRWALLET_RPC_PASS}
    mem_limit: 512m
    cpus: 1.0
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8080/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  frontend:
    build:
      context: ./frontend
    container_name: decred-pulse-frontend-prod
    restart: always
    depends_on:
      backend:
        condition: service_healthy
    networks:
      - decred-network
    mem_limit: 256m
    cpus: 0.5
    logging:
      driver: "json-file"
      options:
        max-size: "5m"
        max-file: "3"

networks:
  decred-network:
    driver: bridge

volumes:
  dcrd-data:
  dcrwallet-data:
  certs:
```

---

### Step 5: Start Services

```bash
# Start with production config
docker compose -f docker-compose.prod.yml up -d

# Monitor startup
docker compose -f docker-compose.prod.yml logs -f

# Verify all healthy
docker compose -f docker-compose.prod.yml ps
```

---

### Step 6: Nginx Reverse Proxy

```bash
# Create Nginx config
sudo nano /etc/nginx/sites-available/decred-pulse
```

**Basic HTTP configuration**:
```nginx
server {
    listen 80;
    server_name your-domain.com;

    # Frontend
    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }

    # Backend API
    location /api/ {
        proxy_pass http://localhost:8080/api/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for long-running requests
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
}
```

**Enable site**:
```bash
# Enable site
sudo ln -s /etc/nginx/sites-available/decred-pulse /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload Nginx
sudo systemctl reload nginx
```

---

### Step 7: SSL/TLS with Let's Encrypt

```bash
# Obtain SSL certificate
sudo certbot --nginx -d your-domain.com

# Follow prompts:
# - Enter email
# - Agree to terms
# - Choose redirect (recommended)

# Verify auto-renewal
sudo certbot renew --dry-run
```

**Nginx will be automatically updated** with HTTPS configuration.

---

### Step 8: Verify Deployment

```bash
# Test HTTPS access
curl https://your-domain.com

# Test API
curl https://your-domain.com/api/health

# Check all services
docker compose -f docker-compose.prod.yml ps

# Verify logs
docker compose -f docker-compose.prod.yml logs --tail=50
```

---

## 🔒 Production Security Checklist

### Application Security

- [ ] Strong RPC passwords (32+ characters)
- [ ] Unique credentials for dcrd and dcrwallet
- [ ] `.env` file permissions set to 600
- [ ] HTTPS enabled with valid certificate
- [ ] Security headers configured in Nginx

### Network Security

- [ ] Firewall enabled and configured
- [ ] Only necessary ports exposed
- [ ] SSH key authentication enabled
- [ ] Password authentication disabled
- [ ] Fail2ban configured (optional)

### System Security

- [ ] OS and packages up to date
- [ ] Automatic security updates enabled
- [ ] Non-root user for deployment
- [ ] Docker socket secured
- [ ] Log rotation configured

### Monitoring

- [ ] Health checks configured
- [ ] Log aggregation set up
- [ ] Alerts for service failures
- [ ] Disk space monitoring
- [ ] Resource usage tracking

---

## 📦 Updates and Maintenance

### Update Procedure

```bash
# Navigate to installation
cd /opt/decred-pulse

# Backup current state
docker compose -f docker-compose.prod.yml down
sudo tar czf /backup/decred-pulse-$(date +%Y%m%d).tar.gz .

# Pull latest code
git fetch origin
git pull origin main

# Rebuild images
docker compose -f docker-compose.prod.yml build --no-cache

# Restart services
docker compose -f docker-compose.prod.yml up -d

# Verify
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f
```

### Update dcrd Version

```bash
# Edit .env
nano .env

# Change version
DCRD_VERSION=release-v2.0.7

# Rebuild and restart
docker compose -f docker-compose.prod.yml build --no-cache dcrd
docker compose -f docker-compose.prod.yml up -d dcrd

# Verify
docker compose -f docker-compose.prod.yml logs -f dcrd
```

---

## 💾 Backup Strategy

### What to Backup

**Critical** (must backup):
- `.env` file (credentials)
- Blockchain data (optional, can re-sync)
- Wallet data (if using)

**Nice to have**:
- Configuration files
- Nginx configs
- SSL certificates (can regenerate)

---

### Backup Script

Create `/opt/decred-pulse/backup.sh`:

```bash
#!/bin/bash

BACKUP_DIR="/backup/decred-pulse"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p $BACKUP_DIR

# Backup .env
cp .env "$BACKUP_DIR/env-$DATE"

# Backup configs
tar czf "$BACKUP_DIR/configs-$DATE.tar.gz" \
    .env \
    docker-compose.prod.yml

# Backup volumes (blockchain data - optional, large)
# Uncomment if needed
# docker run --rm \
#     -v decred-pulse_dcrd-data:/data \
#     -v $BACKUP_DIR:/backup \
#     alpine tar czf /backup/dcrd-data-$DATE.tar.gz -C /data .

# Keep only last 7 backups
find $BACKUP_DIR -type f -mtime +7 -delete

echo "Backup completed: $DATE"
```

**Setup cron**:
```bash
# Make executable
chmod +x /opt/decred-pulse/backup.sh

# Add to crontab
crontab -e

# Daily backup at 2 AM
0 2 * * * /opt/decred-pulse/backup.sh >> /var/log/decred-pulse-backup.log 2>&1
```

---

## 🔍 Monitoring and Alerts

### Basic Monitoring

```bash
# Create monitoring script
cat > /opt/decred-pulse/monitor.sh << 'EOF'
#!/bin/bash

# Check if services are running
if ! docker compose -f /opt/decred-pulse/docker-compose.prod.yml ps | grep -q "Up"; then
    echo "ERROR: Some services are down!"
    docker compose -f /opt/decred-pulse/docker-compose.prod.yml ps
    exit 1
fi

# Check API health
if ! curl -sf http://localhost:8080/api/health > /dev/null; then
    echo "ERROR: API health check failed!"
    exit 1
fi

# Check disk space
DISK_USAGE=$(df -h / | awk 'NR==2 {print $5}' | sed 's/%//')
if [ $DISK_USAGE -gt 90 ]; then
    echo "WARNING: Disk usage above 90%: ${DISK_USAGE}%"
fi

echo "All checks passed"
EOF

chmod +x /opt/decred-pulse/monitor.sh

# Add to crontab (every 5 minutes)
*/5 * * * * /opt/decred-pulse/monitor.sh >> /var/log/decred-pulse-monitor.log 2>&1
```

---

## 🚨 Disaster Recovery

### Service Failure

```bash
# Check status
docker compose -f docker-compose.prod.yml ps

# View logs
docker compose -f docker-compose.prod.yml logs <service-name>

# Restart service
docker compose -f docker-compose.prod.yml restart <service-name>

# Full restart
docker compose -f docker-compose.prod.yml restart
```

### Data Corruption

```bash
# Stop services
docker compose -f docker-compose.prod.yml down

# Restore from backup
tar xzf /backup/dcrd-data-YYYYMMDD.tar.gz -C /var/lib/docker/volumes/dcrd-data/_data

# Start services
docker compose -f docker-compose.prod.yml up -d
```

### Complete System Recovery

```bash
# Reinstall from backup
cd /opt
sudo rm -rf decred-pulse
sudo tar xzf /backup/decred-pulse-YYYYMMDD.tar.gz
cd decred-pulse

# Restore volumes if needed
# Restore .env and configs

# Start services
docker compose -f docker-compose.prod.yml up -d
```

---

## 📊 Performance Tuning

See [Performance Guide](performance.md) for detailed optimization.

**Quick wins**:
- Use SSD for blockchain storage
- Allocate adequate RAM (8+ GB)
- Tune Docker resource limits
- Enable swap if RAM limited
- Use CDN for frontend (optional)

---

## 🔗 Additional Resources

- **[Security Guide](security.md)** - Hardening and best practices
- **[Performance Guide](performance.md)** - Optimization techniques
- **[Monitoring Setup](monitoring-setup.md)** - Advanced monitoring
- **[Configuration](../setup/configuration.md)** - Configuration options

---

**Need Help?** Check the [Troubleshooting Guide](../guides/troubleshooting.md) or ask in the [Decred Community](https://decred.org/community/)

