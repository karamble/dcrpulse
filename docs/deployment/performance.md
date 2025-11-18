# Performance Optimization

Guide to optimizing Decred Pulse for maximum performance, efficiency, and resource utilization.

## 📊 Performance Overview

### Optimization Areas

```
┌────────────────────────────────────────┐
│ Application Performance                │
│ (Backend, Frontend, Caching)           │
├────────────────────────────────────────┤
│ Database Performance                   │
│ (dcrd, dcrwallet, Indexing)            │
├────────────────────────────────────────┤
│ System Performance                     │
│ (CPU, RAM, Disk I/O)                   │
├────────────────────────────────────────┤
│ Network Performance                    │
│ (Bandwidth, Latency, CDN)              │
└────────────────────────────────────────┘
```

---

## ⚡ Quick Wins

### 1. Use SSD Storage

**Impact**: 10-50x faster than HDD

```bash
# Check disk type
lsblk -d -o name,rota
# rota=1: HDD, rota=0: SSD

# Move Docker data to SSD
sudo systemctl stop docker
sudo nano /etc/docker/daemon.json
```

```json
{
  "data-root": "/mnt/ssd/docker"
}
```

```bash
sudo systemctl start docker
```

---

### 2. Increase RAM

**Minimum**: 4 GB  
**Recommended**: 8 GB  
**Optimal**: 16+ GB

**Check current usage**:
```bash
free -h
docker stats
```

---

### 3. Optimize Docker Resources

```yaml
# docker-compose.prod.yml
services:
  dcrd:
    mem_limit: 2g      # Increase if available
    cpus: 2.0          # Increase for faster sync
    
  dcrwallet:
    mem_limit: 1g
    cpus: 1.0
    
  backend:
    mem_limit: 512m
    cpus: 1.0
```

---

### 4. Enable Transaction Indexing

```bash
# .env
DCRD_EXTRA_ARGS=--txindex

# Restart
docker compose restart dcrd
```

**Impact**:
- ✅ Faster transaction lookups
- ✅ Required for full explorer
- ⚠️ 15-20% more disk space
- ⚠️ Slower initial sync

---

## 🔧 Backend Optimization

### Concurrent RPC Calls

**Current implementation** uses goroutines for parallel RPC requests:

```go
// Example: services/node.go
func FetchDashboardData() (*types.DashboardData, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Concurrent fetching
    g, gctx := errgroup.WithContext(ctx)
    
    var (
        nodeStatus NodeStatus
        blockchain BlockchainInfo
        peers      []Peer
    )
    
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
    
    // Aggregate and return
}
```

**Benefits**:
- Parallel execution reduces total time
- Uses Go's efficient concurrency
- Respects timeouts

---

### Connection Pooling

**RPC connections** are reused:

```go
// rpc/client.go
var (
    NodeClient   *rpcclient.Client  // Singleton
    WalletClient *rpcclient.Client  // Singleton
)

// Initialize once, reuse for all requests
```

---

### Response Caching (Future Enhancement)

```go
// Example implementation
var cache = &sync.Map{}

func GetDashboardData(w http.ResponseWriter, r *http.Request) {
    // Check cache
    if cached, ok := cache.Load("dashboard"); ok {
        if time.Since(cached.Time) < 10*time.Second {
            json.NewEncoder(w).Encode(cached.Data)
            return
        }
    }
    
    // Fetch fresh data
    data, err := services.FetchDashboardData()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Cache result
    cache.Store("dashboard", CachedData{
        Data: data,
        Time: time.Now(),
    })
    
    json.NewEncoder(w).Encode(data)
}
```

---

## 🖥️ Frontend Optimization

### Build Optimization

```bash
# frontend/vite.config.ts
export default defineConfig({
  build: {
    target: 'es2015',
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: true,  // Remove console.logs
      },
    },
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'ui-vendor': ['lucide-react'],
        },
      },
    },
    chunkSizeWarningLimit: 1000,
  },
});
```

---

### Code Splitting

```typescript
// Use React.lazy for route-based splitting
import { lazy, Suspense } from 'react';

const NodeDashboard = lazy(() => import('./pages/NodeDashboard'));
const WalletDashboard = lazy(() => import('./pages/WalletDashboard'));

function App() {
  return (
    <Suspense fallback={<LoadingSpinner />}>
      <Routes>
        <Route path="/" element={<NodeDashboard />} />
        <Route path="/wallet" element={<WalletDashboard />} />
      </Routes>
    </Suspense>
  );
}
```

---

### Component Memoization

```typescript
import { memo, useMemo, useCallback } from 'react';

// Memoize expensive components
export const MetricCard = memo(({ title, value, icon }) => {
  return (
    <div className="metric-card">
      {/* ... */}
    </div>
  );
});

// Memoize expensive calculations
function Dashboard() {
  const sortedPeers = useMemo(() => {
    return peers.sort((a, b) => a.latency - b.latency);
  }, [peers]);
  
  const handleRefresh = useCallback(() => {
    fetchData();
  }, []);
}
```

---

### Reduce Bundle Size

```bash
# Analyze bundle
cd frontend
npm run build
npx vite-bundle-visualizer

# Remove unused dependencies
npm uninstall <unused-package>

# Use tree-shaking
# Vite does this automatically for ES modules
```

---

## 💾 Database Optimization

### dcrd Configuration

Configure via `DCRD_EXTRA_ARGS` in `.env`:

```bash
# High-performance: more peer connections for faster sync
DCRD_EXTRA_ARGS=--txindex --maxpeers=200

# Default (recommended for most users):
DCRD_EXTRA_ARGS=--txindex

# Low bandwidth/resource usage:
DCRD_EXTRA_ARGS=--txindex --maxpeers=25
```

**Valid command-line flags:**
- `--txindex` - Enable transaction indexing (required for block explorer)
- `--debuglevel=LEVEL` - Set log level (trace, debug, info, warn, error, critical)
- `--maxpeers=N` - Maximum peer connections (default: 125)

**Restart dcrd to apply**:
```bash
docker compose restart dcrd
```

---

### Sync Optimization

**Fast sync tips**:
1. **Good internet**: 50+ Mbps
2. **More peers**: `maxpeers=200`
3. **SSD storage**: 10-50x faster
4. **Higher dbcache**: `dbcache=500`
5. **Adequate RAM**: 4+ GB available

**Expected sync times**:
- **SSD + 8GB RAM + 100 Mbps**: 2-4 hours
- **HDD + 4GB RAM + 10 Mbps**: 8-12 hours

---

### dcrwallet Optimization

```bash
# dcrwallet.conf

# Gap limit (lower = faster, but may miss transactions)
gaplimit=200  # Default: 1000

# Account discovery (if using multiple accounts)
accountgaplimit=10

# Connection to dcrd
rpcconnect=dcrd:9109
```

---

## 🌐 Network Optimization

### Nginx Configuration

```nginx
# /etc/nginx/nginx.conf

# Worker processes (match CPU cores)
worker_processes auto;

# Worker connections
events {
    worker_connections 4096;
    use epoll;  # Linux only
}

http {
    # Compression
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_types text/plain text/css text/xml text/javascript
               application/javascript application/json application/xml+rss
               image/svg+xml;
    
    # Keep-alive connections
    keepalive_timeout 65;
    keepalive_requests 100;
    
    # Buffer sizes
    client_body_buffer_size 16K;
    client_header_buffer_size 1k;
    client_max_body_size 8m;
    large_client_header_buffers 2 1k;
    
    # Timeouts
    client_body_timeout 12;
    client_header_timeout 12;
    send_timeout 10;
    
    # Caching
    open_file_cache max=1000 inactive=20s;
    open_file_cache_valid 30s;
    open_file_cache_min_uses 2;
    open_file_cache_errors on;
    
    # Your server blocks...
}
```

---

### Frontend Caching

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;
    
    # Cache static assets
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        access_log off;
    }
    
    # Don't cache API responses
    location /api/ {
        add_header Cache-Control "no-cache, no-store, must-revalidate";
        proxy_pass http://localhost:8080/api/;
    }
    
    # Don't cache HTML (for updates)
    location / {
        add_header Cache-Control "no-cache";
        proxy_pass http://localhost:3000;
    }
}
```

---

### CDN Integration (Optional)

For global deployments:

1. **Cloudflare** (Free tier available)
   - DNS + CDN + DDoS protection
   - Caches static assets globally
   - Automatic HTTPS

2. **Configuration**:
   - Point domain to Cloudflare
   - Enable proxy (orange cloud)
   - Configure caching rules
   - Enable Brotli compression

---

## 💻 System Optimization

### Linux Kernel Tuning

```bash
# Edit sysctl
sudo nano /etc/sysctl.conf

# Add these optimizations:
```

```ini
# Network performance
net.core.rmem_max = 16777216
net.core.wmem_max = 16777216
net.ipv4.tcp_rmem = 4096 87380 16777216
net.ipv4.tcp_wmem = 4096 65536 16777216
net.ipv4.tcp_congestion_control = bbr

# File handles
fs.file-max = 65536

# Connection tracking
net.netfilter.nf_conntrack_max = 262144
net.ipv4.ip_local_port_range = 1024 65535

# Swappiness (lower = prefer RAM)
vm.swappiness = 10
```

```bash
# Apply
sudo sysctl -p
```

---

### Swap Configuration

```bash
# Check current swap
swapon --show

# Create swap file (if not exists)
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# Make permanent
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab

# Set swappiness
sudo sysctl vm.swappiness=10
echo 'vm.swappiness=10' | sudo tee -a /etc/sysctl.conf
```

---

### Disk I/O Optimization

```bash
# Check current scheduler
cat /sys/block/sda/queue/scheduler

# For SSD, use none or mq-deadline
echo none | sudo tee /sys/block/sda/queue/scheduler

# For HDD, use cfq or bfq
echo bfq | sudo tee /sys/block/sda/queue/scheduler

# Make permanent
sudo nano /etc/udev/rules.d/60-ioschedulers.rules
```

Add:
```
# SSD
ACTION=="add|change", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="0", ATTR{queue/scheduler}="none"

# HDD
ACTION=="add|change", KERNEL=="sd[a-z]", ATTR{queue/rotational}=="1", ATTR{queue/scheduler}="bfq"
```

---

## 📊 Monitoring Performance

### Baseline Metrics

**Before optimization**:
```bash
# CPU usage
top -bn1 | grep "Cpu(s)"

# Memory usage
free -h

# Disk I/O
iostat -x 1 10

# Network
iftop

# Docker stats
docker stats --no-stream
```

**After optimization**:
- Repeat measurements
- Compare improvements
- Document changes

---

### Performance Benchmarks

**dcrd sync time**:
```bash
# Start fresh sync
time docker compose up -d dcrd

# Monitor until complete
watch docker compose logs dcrd | grep "Syncing"

# Record total time
```

**API response time**:
```bash
# Measure API latency
time curl http://localhost:8080/api/health
time curl http://localhost:8080/api/dashboard

# Use Apache Bench for load testing
ab -n 1000 -c 10 http://localhost:8080/api/health
```

**Frontend load time**:
```bash
# Use browser DevTools
# Network tab → Reload page
# Check:
# - Total load time
# - Number of requests
# - Total size
# - Largest assets
```

---

## 🎯 Performance Goals

### Target Metrics

**Backend**:
- API response time: < 100ms (cached)
- API response time: < 500ms (fresh data)
- RPC timeout: 10 seconds max
- CPU usage: < 50% average
- Memory usage: < 2GB

**Frontend**:
- Initial load: < 2 seconds
- Time to Interactive: < 3 seconds
- Bundle size: < 500 KB (gzipped)
- Dashboard refresh: < 1 second

**dcrd**:
- Sync time: < 4 hours (SSD, good internet)
- Memory usage: < 2 GB
- Peer count: 8-50 (stable)
- Block validation: < 500ms per block

---

## 🚀 Advanced Optimization

### Database Sharding (Future)

For very large deployments:
- Separate read/write operations
- Use replica for queries
- Master for writes

### Load Balancing (Multiple Instances)

```nginx
upstream backend_pool {
    least_conn;
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}

server {
    location /api/ {
        proxy_pass http://backend_pool/api/;
    }
}
```

### Horizontal Scaling

- Multiple frontend instances (stateless)
- Load balancer (Nginx, HAProxy)
- Shared backend (or multiple with LB)
- Single dcrd/dcrwallet (shared resource)

---

## 🔧 Troubleshooting Performance

### Slow Dashboard

**Symptoms**: Long load times, timeouts

**Diagnose**:
```bash
# Check backend logs
docker compose logs backend | grep -i "timeout\|slow"

# Check RPC response time
docker exec decred-pulse-dcrd dcrctl --rpcuser=... getinfo

# Check network latency
ping localhost
```

**Solutions**:
- Increase backend timeout
- Add response caching
- Optimize RPC calls
- Check network connectivity

---

### High CPU Usage

**Symptoms**: System sluggish, high load average

**Diagnose**:
```bash
# Check CPU usage by container
docker stats

# Check system load
uptime
top
```

**Solutions**:
- Increase CPU allocation
- Reduce concurrent operations
- Lower log level
- Optimize code

---

### High Memory Usage

**Symptoms**: Swapping, OOM kills

**Diagnose**:
```bash
# Check memory by container
docker stats

# Check system memory
free -h
vmstat 1

# Check for memory leaks
docker exec <container> top
```

**Solutions**:
- Increase RAM
- Set memory limits
- Restart containers periodically
- Fix memory leaks

---

### Disk I/O Bottleneck

**Symptoms**: High wait time, slow sync

**Diagnose**:
```bash
# Check I/O wait
iostat -x 1 10

# Check disk usage
iotop

# Check if SSD or HDD
lsblk -d -o name,rota
```

**Solutions**:
- Use SSD
- Increase disk cache
- Optimize scheduler
- RAID 0 for performance (with backups!)

---

## 📚 Performance Resources

### Monitoring Tools

- **htop**: Interactive process viewer
- **iotop**: Disk I/O monitor
- **nethogs**: Network bandwidth per process
- **docker stats**: Container resource usage
- **prometheus + grafana**: Advanced metrics

### Benchmarking Tools

- **Apache Bench** (ab): HTTP load testing
- **wrk**: Modern HTTP benchmark
- **siege**: HTTP stress testing
- **sysbench**: System performance benchmark

### Documentation

- [Docker Performance](https://docs.docker.com/config/containers/resource_constraints/)
- [Nginx Tuning](https://nginx.org/en/docs/http/ngx_http_core_module.html)
- [Linux Performance](http://www.brendangregg.com/linuxperf.html)

---

**Performance is iterative!** Measure, optimize, measure again.

**Questions?** Check the [Monitoring Setup](monitoring-setup.md) for detailed performance tracking.

