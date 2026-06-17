# Monitoring Setup

Complete guide to setting up production monitoring, alerting, and observability for Decred Pulse.

## Monitoring Overview

### Why Monitor?

- **Detect issues early**: Before users notice
- **Track performance**: Identify bottlenecks
- **Plan capacity**: When to scale
- **Debug problems**: Historical data
- **Security**: Detect anomalies

---

### Monitoring Stack Options

**Simple** (Recommended for small deployments):
- Docker stats
- System tools (htop, iotop)
- Log files
- Email alerts

**Intermediate**:
- Prometheus + Grafana
- Loki for logs
- Alertmanager for notifications

**Advanced**:
- Full ELK stack (Elasticsearch, Logstash, Kibana)
- Jaeger for tracing
- PagerDuty for on-call

---

## Basic Monitoring (No Additional Tools)

### Docker Health Checks

Already configured in `docker-compose.yml`:

```yaml
services:
  dcrd:
    healthcheck:
      test: ["CMD", "dcrctl", "...", "getblockcount"]
      interval: 30s
      timeout: 10s
      retries: 5
```

**Check status**:
```bash
docker compose ps
# Look for "healthy" status
```

---

### System Monitoring Script

Create `/opt/dcrpulse/monitor.sh`:

```bash
#!/bin/bash

LOG_FILE="/var/log/dcrpulse-monitor.log"
ALERT_EMAIL="admin@your-domain.com"

# Timestamp
echo "=== Monitoring Check: $(date) ===" >> $LOG_FILE

# Check Docker containers
echo "Checking containers..." >> $LOG_FILE
if ! docker compose ps | grep -q "Up (healthy)"; then
    echo "ERROR: Containers unhealthy" >> $LOG_FILE
    echo "Decred Pulse: Unhealthy containers detected" | mail -s "ALERT: Container Health" $ALERT_EMAIL
fi

# Check disk space
DISK_USAGE=$(df -h / | awk 'NR==2 {print $5}' | sed 's/%//')
echo "Disk usage: ${DISK_USAGE}%" >> $LOG_FILE
if [ $DISK_USAGE -gt 90 ]; then
    echo "WARNING: Disk usage high: ${DISK_USAGE}%" >> $LOG_FILE
    echo "Disk usage is at ${DISK_USAGE}%" | mail -s "ALERT: Disk Space" $ALERT_EMAIL
fi

# Check memory
MEM_USAGE=$(free | awk '/Mem:/ {printf "%.0f", $3/$2 * 100}')
echo "Memory usage: ${MEM_USAGE}%" >> $LOG_FILE
if [ $MEM_USAGE -gt 90 ]; then
    echo "WARNING: Memory usage high: ${MEM_USAGE}%" >> $LOG_FILE
    echo "Memory usage is at ${MEM_USAGE}%" | mail -s "ALERT: Memory" $ALERT_EMAIL
fi

# Check API health
if ! curl -sf http://localhost:8080/api/health > /dev/null; then
    echo "ERROR: API health check failed" >> $LOG_FILE
    echo "API health check failed" | mail -s "ALERT: API Down" $ALERT_EMAIL
fi

# Check dcrd sync
BLOCK_COUNT=$(docker exec dcrpulse-dcrd dcrctl \
    --rpcuser=$DCRD_RPC_USER \
    --rpcpass=$DCRD_RPC_PASS \
    --rpcserver=127.0.0.1:9109 \
    --rpccert=/certs/rpc.cert \
    getblockcount 2>/dev/null)

if [ -n "$BLOCK_COUNT" ]; then
    echo "Current block: $BLOCK_COUNT" >> $LOG_FILE
else
    echo "ERROR: Cannot get block count" >> $LOG_FILE
    echo "dcrd appears to be down or not responding" | mail -s "ALERT: dcrd Down" $ALERT_EMAIL
fi

echo "" >> $LOG_FILE
```

**Setup**:
```bash
chmod +x /opt/dcrpulse/monitor.sh

# Add to crontab (every 5 minutes)
crontab -e
*/5 * * * * /opt/dcrpulse/monitor.sh
```

---

### Log Monitoring

**Centralize logs**:
```bash
# Create log directory
mkdir -p /var/log/dcrpulse

# Stream Docker logs to files
docker compose logs -f dcrd > /var/log/dcrpulse/dcrd.log 2>&1 &
docker compose logs -f dashboard > /var/log/dcrpulse/backend.log 2>&1 &
```

**Monitor for errors**:
```bash
# Watch for errors in real-time
tail -f /var/log/dcrpulse/*.log | grep -i "error\|fatal\|panic"

# Daily error report
#!/bin/bash
grep -i "error\|fatal" /var/log/dcrpulse/*.log | \
    mail -s "Daily Error Report" admin@your-domain.com
```

---

## Prometheus + Grafana Setup

### Architecture

```
┌─────────────────┐
│  Grafana        │  (Visualization)
│  Port 3001      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Prometheus     │  (Metrics Storage)
│  Port 9090      │
└────────┬────────┘
         │
         ├──► Node Exporter (System Metrics)
         ├──► cAdvisor (Docker Metrics)
         └──► Backend /metrics (App Metrics)
```

---

### Step 1: Add to Docker Compose

Create `docker-compose.monitoring.yml`:

```yaml
version: '3.8'

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: always
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/usr/share/prometheus/console_libraries'
      - '--web.console.templates=/usr/share/prometheus/consoles'
    networks:
      - decred-network

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: always
    ports:
      - "3001:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
    volumes:
      - grafana-data:/var/lib/grafana
    depends_on:
      - prometheus
    networks:
      - decred-network

  node-exporter:
    image: prom/node-exporter:latest
    container_name: node-exporter
    restart: always
    ports:
      - "9100:9100"
    command:
      - '--path.procfs=/host/proc'
      - '--path.sysfs=/host/sys'
      - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)'
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    networks:
      - decred-network

  cadvisor:
    image: gcr.io/cadvisor/cadvisor:latest
    container_name: cadvisor
    restart: always
    ports:
      - "8081:8080"
    volumes:
      - /:/rootfs:ro
      - /var/run:/var/run:ro
      - /sys:/sys:ro
      - /var/lib/docker/:/var/lib/docker:ro
    privileged: true
    networks:
      - decred-network

networks:
  decred-network:
    external: true

volumes:
  prometheus-data:
  grafana-data:
```

---

### Step 2: Configure Prometheus

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  # Prometheus itself
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  # System metrics
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']

  # Docker container metrics
  - job_name: 'cadvisor'
    static_configs:
      - targets: ['cadvisor:8080']

  # Backend application metrics (if implemented)
  - job_name: 'dcrpulse-dashboard'
    static_configs:
      - targets: ['backend:8080']
    metrics_path: '/metrics'
```

---

### Step 3: Start Monitoring Stack

```bash
# Start monitoring services
docker compose -f docker-compose.monitoring.yml up -d

# Verify
docker compose -f docker-compose.monitoring.yml ps

# Access Grafana
# http://localhost:3001
# Username: admin
# Password: admin (change on first login)
```

---

### Step 4: Configure Grafana

**Add Prometheus Data Source**:
1. Open Grafana: http://localhost:3001
2. Login (admin/admin)
3. Go to Configuration -> Data Sources
4. Add Data Source -> Prometheus
5. URL: `http://prometheus:9090`
6. Save & Test

**Import Dashboards**:
1. Go to Dashboards -> Import
2. Enter dashboard ID or JSON

**Recommended dashboards**:
- **Node Exporter Full**: ID 1860
- **Docker Container Metrics**: ID 193
- **Prometheus 2.0 Stats**: ID 3662

---

## Alerting Setup

### Prometheus Alertmanager

Create `alertmanager.yml`:

```yaml
global:
  resolve_timeout: 5m
  smtp_smarthost: 'smtp.gmail.com:587'
  smtp_from: 'alerts@your-domain.com'
  smtp_auth_username: 'alerts@your-domain.com'
  smtp_auth_password: 'your-app-password'

route:
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  receiver: 'email'

receivers:
  - name: 'email'
    email_configs:
      - to: 'admin@your-domain.com'
        headers:
          Subject: 'Decred Pulse Alert: {{ .GroupLabels.alertname }}'
```

Add to `docker-compose.monitoring.yml`:

```yaml
  alertmanager:
    image: prom/alertmanager:latest
    container_name: alertmanager
    restart: always
    ports:
      - "9093:9093"
    volumes:
      - ./alertmanager.yml:/etc/alertmanager/alertmanager.yml:ro
    command:
      - '--config.file=/etc/alertmanager/alertmanager.yml'
    networks:
      - decred-network
```

---

### Alert Rules

Create `alert-rules.yml`:

```yaml
groups:
  - name: decred_pulse_alerts
    interval: 30s
    rules:
      # Container down
      - alert: ContainerDown
        expr: up{job="cadvisor"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Container {{ $labels.instance }} is down"
          description: "Container has been down for more than 1 minute"

      # High CPU usage
      - alert: HighCPUUsage
        expr: rate(process_cpu_seconds_total[1m]) > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage on {{ $labels.instance }}"
          description: "CPU usage is above 80% for more than 5 minutes"

      # High memory usage
      - alert: HighMemoryUsage
        expr: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes > 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage on {{ $labels.instance }}"
          description: "Memory usage is above 90% for more than 5 minutes"

      # Disk space low
      - alert: DiskSpaceLow
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) < 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Low disk space on {{ $labels.instance }}"
          description: "Less than 10% disk space remaining"

      # API health check failed
      - alert: APIHealthCheckFailed
        expr: up{job="dcrpulse-dashboard"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "API health check failed"
          description: "Backend API is not responding"
```

Update `prometheus.yml`:

```yaml
rule_files:
  - 'alert-rules.yml'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']
```

---

## Custom Application Metrics

### Add Metrics to Backend

```go
// main.go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )
    
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "http_request_duration_seconds",
            Help: "HTTP request duration in seconds",
        },
        []string{"method", "endpoint"},
    )
    
    rpcCallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "rpc_calls_total",
            Help: "Total number of RPC calls",
        },
        []string{"method", "status"},
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal)
    prometheus.MustRegister(httpRequestDuration)
    prometheus.MustRegister(rpcCallsTotal)
}

func main() {
    // ... existing code ...
    
    // Metrics endpoint
    http.Handle("/metrics", promhttp.Handler())
    
    // ... rest of setup ...
}
```

---

## Notification Channels

### Email Alerts

Already configured in Alertmanager (see above).

---

### Slack Notifications

Update `alertmanager.yml`:

```yaml
receivers:
  - name: 'slack'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/WEBHOOK/URL'
        channel: '#alerts'
        title: 'Decred Pulse Alert'
        text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

---

### Discord Notifications

```yaml
receivers:
  - name: 'discord'
    webhook_configs:
      - url: 'https://discord.com/api/webhooks/YOUR/WEBHOOK/URL'
        send_resolved: true
```

---

### Telegram Notifications

```yaml
receivers:
  - name: 'telegram'
    telegram_configs:
      - bot_token: 'YOUR_BOT_TOKEN'
        chat_id: YOUR_CHAT_ID
        parse_mode: 'HTML'
        message: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

---

## Log Aggregation with Loki

### Add Loki to Stack

Add to `docker-compose.monitoring.yml`:

```yaml
  loki:
    image: grafana/loki:latest
    container_name: loki
    restart: always
    ports:
      - "3100:3100"
    volumes:
      - ./loki-config.yml:/etc/loki/local-config.yaml:ro
      - loki-data:/loki
    command: -config.file=/etc/loki/local-config.yaml
    networks:
      - decred-network

  promtail:
    image: grafana/promtail:latest
    container_name: promtail
    restart: always
    volumes:
      - ./promtail-config.yml:/etc/promtail/config.yml:ro
      - /var/log:/var/log:ro
      - /var/lib/docker/containers:/var/lib/docker/containers:ro
    command: -config.file=/etc/promtail/config.yml
    networks:
      - decred-network

volumes:
  loki-data:
```

Create `loki-config.yml`:

```yaml
auth_enabled: false

server:
  http_listen_port: 3100

ingester:
  lifecycler:
    ring:
      kvstore:
        store: inmemory
      replication_factor: 1
  chunk_idle_period: 5m
  chunk_retain_period: 30s

schema_config:
  configs:
    - from: 2020-05-15
      store: boltdb
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 168h

storage_config:
  boltdb:
    directory: /loki/index
  filesystem:
    directory: /loki/chunks
```

Create `promtail-config.yml`:

```yaml
server:
  http_listen_port: 9080

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki:3100/loki/api/v1/push

scrape_configs:
  # Docker containers
  - job_name: docker
    static_configs:
      - targets:
          - localhost
        labels:
          job: docker
          __path__: /var/lib/docker/containers/*/*.log
  
  # System logs
  - job_name: syslog
    static_configs:
      - targets:
          - localhost
        labels:
          job: syslog
          __path__: /var/log/syslog
```

---

## Troubleshooting Monitoring

### Prometheus Not Scraping

```bash
# Check Prometheus targets
# Visit: http://localhost:9090/targets

# Check configuration
docker exec prometheus promtool check config /etc/prometheus/prometheus.yml

# View Prometheus logs
docker compose logs prometheus
```

---

### Grafana Not Showing Data

```bash
# Check Grafana logs
docker compose logs grafana

# Verify Prometheus connection
# Grafana → Configuration → Data Sources → Test

# Check query in Explore tab
```

---

### Alerts Not Firing

```bash
# Check Alertmanager status
# Visit: http://localhost:9093

# Verify alert rules
docker exec prometheus promtool check rules /etc/prometheus/alert-rules.yml

# Check Alertmanager logs
docker compose logs alertmanager
```

---

## Dashboard Examples

### System Overview Dashboard

**Panels to include**:
- CPU Usage (%)
- Memory Usage (%)
- Disk Usage (%)
- Network Traffic (MB/s)
- Container Status
- Uptime

### Decred Pulse Dashboard

**Panels to include**:
- dcrd Block Height
- dcrd Sync Progress
- dcrd Peer Count
- Wallet Balance
- API Request Rate
- API Response Time
- Error Rate

### Alert Dashboard

**Panels to include**:
- Active Alerts
- Alert History
- Time to Resolve
- Alert Frequency

---

## Additional Resources

### Documentation
- [Prometheus](https://prometheus.io/docs/)
- [Grafana](https://grafana.com/docs/)
- [Loki](https://grafana.com/docs/loki/)
- [Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/)

### Community Dashboards
- [Grafana Dashboards](https://grafana.com/grafana/dashboards/)
- [Awesome Prometheus](https://github.com/roaldnefs/awesome-prometheus)

---

**Monitoring is essential for production!** Set it up early and iterate based on what you learn.

**Questions?** Check the [Performance Guide](performance.md) or [Troubleshooting Guide](../guides/troubleshooting.md)

