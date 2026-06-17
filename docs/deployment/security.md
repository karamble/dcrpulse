# Security Best Practices

Comprehensive security guide for hardening your Decred Pulse deployment against threats and vulnerabilities.

## Security Overview

### Security Layers

```
┌─────────────────────────────────────┐
│  Application Security               │
│  (RPC, API, Authentication)         │
├─────────────────────────────────────┤
│  Network Security                   │
│  (Firewall, TLS, HTTPS)             │
├─────────────────────────────────────┤
│  System Security                    │
│  (OS, Docker, Access Control)       │
├─────────────────────────────────────┤
│  Infrastructure Security            │
│  (Server, Physical, Cloud)          │
└─────────────────────────────────────┘
```

---

## Application Security

### RPC Credentials

**Critical**: RPC credentials protect access to your node and wallet.

#### Strong Passwords

```bash
# Generate secure random passwords
openssl rand -base64 32

# Or use password generator
pwgen -s 32 1
```

**Requirements**:
- Minimum 32 characters
- Mix of letters, numbers, symbols
- Unique for each service
- Never reused
- Never committed to git

#### Example .env (Production)

```bash
# Generate unique credentials
DCRD_RPC_USER=dcrd_$(openssl rand -hex 16)
DCRD_RPC_PASS=$(openssl rand -base64 32)

DCRWALLET_RPC_USER=wallet_$(openssl rand -hex 16)
DCRWALLET_RPC_PASS=$(openssl rand -base64 32)
```

####File Permissions

```bash
# Secure .env file
chmod 600 .env
chown $USER:$USER .env

# Verify
ls -la .env
# Should show: -rw------- (600)
```

---

### API Security

#### HTTPS Only

**Never run production without HTTPS!**

```nginx
# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS configuration
server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    
    # Modern SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384';
    ssl_prefer_server_ciphers off;
    
    # HSTS
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    
    # Your proxy configuration...
}
```

#### Security Headers

```nginx
# Essential security headers
add_header X-Frame-Options "DENY" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "no-referrer-when-downgrade" always;
add_header Content-Security-Policy "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline';" always;

# CORS (restrictive)
add_header Access-Control-Allow-Origin "https://your-domain.com" always;
```

#### Rate Limiting

```nginx
# Define rate limit zone
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;

# Apply to API endpoints
location /api/ {
    limit_req zone=api_limit burst=20 nodelay;
    
    # Your proxy configuration...
}
```

---

### Backend Security

#### Environment Variables

**Never hardcode credentials!**

```go
//  BAD
rpcUser := "decred"
rpcPass := "password123"

//  GOOD
rpcUser := os.Getenv("DCRD_RPC_USER")
rpcPass := os.Getenv("DCRD_RPC_PASS")
```

#### Input Validation

```go
// Validate all user inputs
func ImportXpubHandler(w http.ResponseWriter, r *http.Request) {
    var req ImportXpubRequest
    if err := json.NewDecoder(r).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    // Validate xpub format
    if !isValidXpub(req.Xpub) {
        http.Error(w, "Invalid xpub format", http.StatusBadRequest)
        return
    }
    
    // Validate gap limit range
    if req.GapLimit < 20 || req.GapLimit > 1000 {
        http.Error(w, "Gap limit out of range", http.StatusBadRequest)
        return
    }
    
    // Process request...
}
```

#### Timeouts

```go
// Set reasonable timeouts
client := &http.Client{
    Timeout: 30 * time.Second,
}

// Context with timeout for RPC
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
```

---

## Network Security

### Firewall Configuration

#### UFW (Ubuntu/Debian)

```bash
# Reset to defaults
sudo ufw --force reset

# Default policies
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow SSH (change port if not 22)
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow dcrd P2P (for peer connections)
sudo ufw allow 9108/tcp

# Enable firewall
sudo ufw enable

# Verify
sudo ufw status verbose
```

#### iptables (Advanced)

```bash
# Flush existing rules
sudo iptables -F

# Default policies
sudo iptables -P INPUT DROP
sudo iptables -P FORWARD DROP
sudo iptables -P OUTPUT ACCEPT

# Allow loopback
sudo iptables -A INPUT -i lo -j ACCEPT

# Allow established connections
sudo iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Allow SSH
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT

# Allow HTTP/HTTPS
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Allow dcrd P2P
sudo iptables -A INPUT -p tcp --dport 9108 -j ACCEPT

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

---

### Port Security

**Exposed Ports**:
- **9108**: dcrd P2P (must be open for peers)
- **80**: HTTP (redirect to HTTPS)
- **443**: HTTPS (public access)

**Internal Ports** (Docker network only):
- **9109**: dcrd RPC (not exposed to internet)
- **9110**: dcrwallet RPC (not exposed to internet)
- **8080**: Backend API (behind Nginx proxy)

**Docker Compose (secure)**:
```yaml
services:
  dcrd:
    ports:
      - "9108:9108"  # P2P only
    # NOT exposed: 9109 (RPC - internal only)
  
  backend:
    # NOT exposed directly
    # Accessed via Nginx proxy
  
  frontend:
    # NOT exposed directly
    # Served by Nginx
```

---

### SSH Hardening

#### Disable Password Authentication

```bash
# Edit SSH config
sudo nano /etc/ssh/sshd_config

# Set these options:
PasswordAuthentication no
PubkeyAuthentication yes
PermitRootLogin no
Port 22  # Or change to non-standard port

# Restart SSH
sudo systemctl restart sshd
```

#### Use SSH Keys

```bash
# On your local machine, generate key
ssh-keygen -t ed25519 -C "your_email@example.com"

# Copy to server
ssh-copy-id user@your-server

# Test login
ssh user@your-server

# Now disable password auth (see above)
```

#### Fail2ban

```bash
# Install
sudo apt install fail2ban

# Configure
sudo cp /etc/fail2ban/jail.conf /etc/fail2ban/jail.local
sudo nano /etc/fail2ban/jail.local

# Enable SSH jail
[sshd]
enabled = true
port = 22
maxretry = 3
bantime = 3600

# Start service
sudo systemctl enable fail2ban
sudo systemctl start fail2ban

# Check status
sudo fail2ban-client status sshd
```

---

## System Security

### OS Hardening

#### Automatic Security Updates

```bash
# Install unattended-upgrades
sudo apt install unattended-upgrades

# Enable
sudo dpkg-reconfigure -plow unattended-upgrades

# Configure
sudo nano /etc/apt/apt.conf.d/50unattended-upgrades

# Enable security updates
Unattended-Upgrade::Allowed-Origins {
    "${distro_id}:${distro_codename}-security";
};

# Enable automatic reboot if needed (optional)
Unattended-Upgrade::Automatic-Reboot "false";
```

#### System Limits

```bash
# Edit limits
sudo nano /etc/security/limits.conf

# Add:
* soft nofile 65536
* hard nofile 65536
* soft nproc 4096
* hard nproc 4096

# Apply
sudo sysctl -p
```

---

### Docker Security

#### Run as Non-Root User

```dockerfile
# In Dockerfile
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser
```

#### Read-Only Filesystems

```yaml
services:
  backend:
    read_only: true
    tmpfs:
      - /tmp
```

#### No New Privileges

```yaml
services:
  backend:
    security_opt:
      - no-new-privileges:true
```

#### Drop Capabilities

```yaml
services:
  backend:
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE  # Only if needed
```

#### Resource Limits

```yaml
services:
  dcrd:
    mem_limit: 2g
    cpus: 2.0
    pids_limit: 100
```

---

### Log Security

#### Log Rotation

```bash
# Create logrotate config
sudo nano /etc/logrotate.d/dcrpulse

# Add:
/var/log/dcrpulse/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 root adm
}

# Test
sudo logrotate -d /etc/logrotate.d/dcrpulse
```

#### Secure Docker Logs

```yaml
services:
  dcrd:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

---

## Certificate Management

### Let's Encrypt SSL

#### Initial Setup

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d your-domain.com

# Test auto-renewal
sudo certbot renew --dry-run
```

#### Auto-Renewal

```bash
# Certbot installs cron job automatically
# Verify:
sudo systemctl list-timers | grep certbot

# Manual renewal test
sudo certbot renew --dry-run
```

#### Certificate Permissions

```bash
# Restrict access
sudo chmod 600 /etc/letsencrypt/live/your-domain.com/privkey.pem
sudo chmod 644 /etc/letsencrypt/live/your-domain.com/fullchain.pem
```

---

### Self-Signed Certificates (Development Only)

```bash
# Generate self-signed cert
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout /etc/nginx/ssl/selfsigned.key \
    -out /etc/nginx/ssl/selfsigned.crt

#  Never use in production!
```

---

## Security Monitoring

### Failed Login Attempts

```bash
# Monitor SSH attempts
sudo tail -f /var/log/auth.log | grep "Failed password"

# Check Fail2ban
sudo fail2ban-client status sshd
```

### Docker Security Scanning

```bash
# Scan images for vulnerabilities
docker scan dcrpulse-dashboard:latest

# Or use Trivy
trivy image dcrpulse-dashboard:latest
```

### API Access Logs

```nginx
# Enhanced Nginx logging
log_format detailed '$remote_addr - $remote_user [$time_local] '
                    '"$request" $status $body_bytes_sent '
                    '"$http_referer" "$http_user_agent" '
                    '$request_time $upstream_response_time';

access_log /var/log/nginx/dcrpulse-access.log detailed;

# Monitor for suspicious activity
sudo tail -f /var/log/nginx/dcrpulse-access.log | grep -E "POST|DELETE"
```

---

## Incident Response

### Security Breach Checklist

1. **Isolate System**
   ```bash
   # Disable network access
   sudo ufw deny incoming
   ```

2. **Stop Services**
   ```bash
   docker compose down
   ```

3. **Preserve Evidence**
   ```bash
   # Copy logs
   sudo cp -r /var/log /backup/incident-$(date +%Y%m%d)
   docker compose logs > /backup/docker-logs-$(date +%Y%m%d).txt
   ```

4. **Assess Damage**
   - Check for unauthorized access
   - Review logs for suspicious activity
   - Check file integrity
   - Verify wallet balances

5. **Remediate**
   - Rotate all credentials
   - Apply security patches
   - Review and update firewall rules
   - Restore from clean backup if needed

6. **Prevent Recurrence**
   - Identify attack vector
   - Implement additional controls
   - Update procedures
   - Document incident

---

## Security Checklist

### Pre-Deployment

- [ ] Strong, unique RPC passwords generated
- [ ] `.env` file permissions set to 600
- [ ] SSH key authentication configured
- [ ] Password authentication disabled
- [ ] Firewall configured correctly
- [ ] Only necessary ports exposed
- [ ] HTTPS certificate obtained
- [ ] Security headers configured
- [ ] Rate limiting enabled

### Post-Deployment

- [ ] All services running as non-root
- [ ] Docker security options enabled
- [ ] Resource limits configured
- [ ] Log rotation enabled
- [ ] Monitoring alerts configured
- [ ] Backup strategy implemented
- [ ] Incident response plan documented

### Ongoing

- [ ] Regular security updates applied
- [ ] Credentials rotated quarterly
- [ ] Logs reviewed weekly
- [ ] Vulnerability scans monthly
- [ ] Backup tested quarterly
- [ ] Incident response tested annually

---

## Defense in Depth

### Layer 1: Network

- Firewall (UFW/iptables)
- Rate limiting (Nginx)
- DDoS protection (Cloudflare/similar)

### Layer 2: Application

- HTTPS/TLS
- Strong authentication
- Input validation
- Security headers

### Layer 3: System

- OS hardening
- Minimal attack surface
- Regular updates
- Access controls

### Layer 4: Monitoring

- Log aggregation
- Intrusion detection
- Alert on anomalies
- Regular audits

---

## Security Resources

### Tools

- **SSL Testing**: https://www.ssllabs.com/ssltest/
- **Security Headers**: https://securityheaders.com/
- **Port Scanning**: https://www.shodan.io/
- **Vulnerability Scanning**: Trivy, Clair, Snyk

### Documentation

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [Mozilla SSL Configuration](https://ssl-config.mozilla.org/)

### Community

- Decred Security: https://decred.org/security/
- Decred Bug Bounty: https://bounty.decred.org/

---

## Compliance

### GDPR Considerations

If collecting user data:
- Implement data protection policies
- Document data retention
- Enable data export/deletion
- Maintain audit logs

### PCI DSS (If Handling Payments)

Not applicable for standard Decred Pulse deployment, but if extended:
- Secure network
- Encrypt data in transit/rest
- Maintain secure systems
- Regular monitoring and testing

---

**Security is an ongoing process!** Regularly review and update your security posture.

**Questions?** Check the [Troubleshooting Guide](../guides/troubleshooting.md) or report security issues responsibly to the Decred team.

