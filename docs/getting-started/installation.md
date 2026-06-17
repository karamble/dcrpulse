# Installation Guide

Complete installation guide for Decred Pulse. Choose the installation method that best suits your needs.

## Prerequisites

Before installing Decred Pulse, ensure you have:

### Hardware Requirements

**Minimum**:
- 2 CPU cores
- 4 GB RAM
- 50 GB free disk space
- Internet connection

**Recommended**:
- 4+ CPU cores
- 8 GB RAM
- 80+ GB free disk space (SSD preferred)
- Stable internet (10+ Mbps)

### Software Requirements

Choose your installation method:

#### Option A: Docker Installation (Recommended)
- **Docker**: Version 20.10+
- **Docker Compose**: Version 2.0+
- **Operating System**: Linux, macOS, Windows, or Qubes OS (see the Qubes section below)

#### Option B: Manual Installation
- **Go**: Version 1.21+
- **Node.js**: Version 18+
- **Git**: Latest version
- **dcrd**: Running instance
- **dcrwallet**: Running instance (optional)

---

## Option A: Docker Installation (Recommended)

The easiest and most reliable way to run Decred Pulse. The stack is a multi-daemon
Docker Compose deployment. A single `docker compose up -d` brings up:

- **dcrd** - the Decred full node (syncs the blockchain)
- **dcrwallet** - the wallet daemon
- **dcrlnd** - the Lightning Network daemon
- **brclientd** - the Bison Relay client daemon
- **dcrdex** - the DCRDEX (bisonw) trading daemon
- **tor** - the Tor daemon used for onion routing
- **dashboard** - the unified web UI and API (this is what you open in the browser)

By default these services pull pre-built images from the GitHub Container Registry
(`ghcr.io/karamble/dcrpulse-*`). The first run syncs dcrd; the wallet, Lightning,
Bison Relay, and DEX daemons start as needed once you set up those features in the
dashboard.

### Step 1: Install Docker

**Linux (Ubuntu/Debian)**:
```bash
# Update package index
sudo apt update

# Install Docker
sudo apt install -y docker.io docker-compose

# Add user to docker group
sudo usermod -aG docker $USER

# Log out and back in for group changes to take effect
```

**macOS**:
```bash
# Install Docker Desktop from:
# https://www.docker.com/products/docker-desktop

# Or via Homebrew:
brew install --cask docker
```

**Windows**:
```powershell
# Install Docker Desktop from:
# https://www.docker.com/products/docker-desktop

# Requires WSL2 (Windows Subsystem for Linux)
```

**Verify Installation**:
```bash
docker --version
docker compose version
```

Expected output:
```
Docker version 24.0.5, build...
Docker Compose version v2.20.0
```

---

### Step 2: Clone Repository

```bash
# Clone the repository
git clone https://github.com/karamble/dcrpulse.git

# Enter directory
cd dcrpulse
```

---

### Step 3: Configure Environment

```bash
# Create .env file from example
cp env.example .env

# Edit with your credentials
nano .env
```

**Required changes**:
```bash
# Set secure RPC passwords
DCRD_RPC_PASS=your_secure_password_here
DCRWALLET_RPC_PASS=your_secure_wallet_password_here
```

**Generate secure passwords**:
```bash
# Random 32-character password
openssl rand -base64 32
```

You can also run `make setup`, which copies `env.example` to `.env` for you if one
does not already exist.

---

### Step 4: Start Services

```bash
# Start all services
docker compose up -d

# Or using Makefile
make start
```

**What happens**:
1. Pulls the pre-built images from the GitHub Container Registry (first time only)
2. Starts all containers (dcrd, dcrwallet, dcrlnd, brclientd, dcrdex, tor, dashboard)
3. dcrd begins the initial blockchain sync
4. The other daemons stand by until you set up their features in the dashboard

To build the images from source instead of pulling them, set the `*_VERSION`
build arguments in `.env` and run `docker compose build` before starting. Building
dcrd and dcrwallet from source can take several minutes the first time.

**Monitor startup**:
```bash
# View logs
docker compose logs -f

# Check status
docker compose ps
```

---

### Step 5: Access Dashboard

**Open browser**: http://localhost:8080

The single `dashboard` container serves both the web UI and the API on port 8080.
There is no separate frontend service.

**Initial view**:
- Node Dashboard shows sync status
- "Syncing" status with progress bar
- Peer connections establishing

**Note**: First sync takes 4-8 hours for mainnet

---

### Step 6: Verify Installation

```bash
# Check all services are running
make status

# Or
docker compose ps
```

**Expected output**:
```
NAME                   STATUS          PORTS
dcrpulse-dcrd          Up (healthy)    9108-9109
dcrpulse-dcrwallet     Up (healthy)    9110-9111
dcrpulse-dcrlnd        Up
dcrpulse-brclientd     Up
dcrpulse-dcrdex        Up
dcrpulse-tor           Up
dcrpulse-dashboard     Up              8080
```

---

## Option B: Manual Installation

For development or custom setups. This runs the dashboard outside Docker against
your own dcrd and dcrwallet instances. The dashboard combines the backend and the
frontend into a single Go binary with embedded static files.

### Step 1: Install Dependencies

**Go (Backend)**:
```bash
# Download from https://go.dev/dl/
# Or via package manager

# Ubuntu/Debian
sudo apt install golang-go

# macOS
brew install go

# Verify
go version  # Should be 1.21+
```

**Node.js (Frontend)**:
```bash
# Download from https://nodejs.org/
# Or via package manager

# Ubuntu/Debian
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt install -y nodejs

# macOS
brew install node

# Verify
node --version  # Should be 18+
npm --version
```

---

### Step 2: Install dcrd

```bash
# Clone dcrd
git clone https://github.com/decred/dcrd.git
cd dcrd

# Build
go install .

# Verify
dcrd --version
```

**Configure dcrd**:
```bash
# Create config directory
mkdir -p ~/.dcrd

# Create config file
nano ~/.dcrd/dcrd.conf
```

**Minimal dcrd.conf**:
```ini
rpcuser=your_rpc_username
rpcpass=your_rpc_password
rpclisten=127.0.0.1:9109
txindex=1
```

**Start dcrd**:
```bash
dcrd
```

---

### Step 3: Install dcrwallet (Optional)

```bash
# Clone dcrwallet
git clone https://github.com/decred/dcrwallet.git
cd dcrwallet

# Build
go install .

# Verify
dcrwallet --version
```

**Configure dcrwallet**:
```bash
# Create config directory
mkdir -p ~/.dcrwallet

# Create config file
nano ~/.dcrwallet/dcrwallet.conf
```

**Minimal dcrwallet.conf**:
```ini
username=your_wallet_username
password=your_wallet_password
rpclisten=127.0.0.1:9110
```

**Start dcrwallet**:
```bash
dcrwallet
```

---

### Step 4: Run the Dashboard (Backend)

```bash
# Clone repository
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse/dashboard

# Install dependencies
go mod download

# Set environment variables
export DCRD_RPC_HOST=localhost
export DCRD_RPC_PORT=9109
export DCRD_RPC_USER=your_rpc_username
export DCRD_RPC_PASS=your_rpc_password

# If using wallet
export DCRWALLET_RPC_HOST=localhost
export DCRWALLET_RPC_PORT=9110
export DCRWALLET_RPC_USER=your_wallet_username
export DCRWALLET_RPC_PASS=your_wallet_password

# Start the dashboard
go run cmd/dcrpulse/main.go
```

**The dashboard serves on**: http://localhost:8080

The same command is available as `make dev-backend` from the repository root.
In production this single binary serves the embedded frontend as well, so port
8080 is all you need.

---

### Step 5: Frontend Development Server (Optional)

You only need this for live frontend development with hot reload. In production the
backend already serves the built frontend, so this step is not required to use the
dashboard.

```bash
# In a new terminal, navigate to the frontend
cd dcrpulse/dashboard/web

# Install dependencies
npm install

# Start development server
npm run dev
```

**The dev server starts on**: http://localhost:3000

The dev server proxies API calls to the backend on http://localhost:8080. The same
command is available as `make dev-frontend` from the repository root.

---

### Step 6: Verify Installation

**Test the dashboard**:
```bash
curl http://localhost:8080
```

**Test node connection**: Open http://localhost:8080 in your browser. The dashboard
should show node data.

---

## Linux-Specific Installation

### Ubuntu/Debian

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Docker
sudo apt install -y docker.io docker-compose git

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Clone and start
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse
cp env.example .env
nano .env  # Edit passwords
make start
```

### Fedora/RHEL

```bash
# Install Docker
sudo dnf install -y docker docker-compose git

# Start Docker service
sudo systemctl start docker
sudo systemctl enable docker

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Clone and start
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse
cp env.example .env
nano .env  # Edit passwords
make start
```

### Arch Linux

```bash
# Install Docker
sudo pacman -S docker docker-compose git

# Start Docker service
sudo systemctl start docker.service
sudo systemctl enable docker.service

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Clone and start
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse
cp env.example .env
nano .env  # Edit passwords
make start
```

---

## macOS-Specific Installation

```bash
# Install Homebrew (if not installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Docker Desktop
brew install --cask docker

# Start Docker Desktop from Applications

# Install Git (if not installed)
brew install git

# Clone repository
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# Setup and start
cp env.example .env
nano .env  # Edit passwords
make start
```

---

## Windows-Specific Installation

### Windows with WSL2 (Recommended)

```powershell
# Install WSL2
wsl --install

# Restart computer

# In WSL2 terminal:
# Install Docker Desktop for Windows with WSL2 backend

# Clone repository
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# Setup and start
cp env.example .env
nano .env  # Edit passwords
make start
```

### Windows with Docker Desktop

```powershell
# Install Docker Desktop from:
# https://www.docker.com/products/docker-desktop

# Install Git from:
# https://git-scm.com/download/win

# Clone repository (Git Bash)
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# Create .env file
copy env.example .env
notepad .env  # Edit passwords

# Start (PowerShell or Git Bash)
docker compose up -d
```

---

## Qubes OS-Specific Installation

Qubes OS needs a few extra steps. An AppVM's root filesystem is volatile: it is reset from
its TemplateVM on every shutdown, and only `/home`, `/rw`, and `/usr/local` survive a
reboot. Docker stores all of its images, containers, and named volumes under
`/var/lib/docker`, which lives on that volatile root. Without intervention, every dcrpulse
daemon's data (including the multi-gigabyte dcrd blockchain) would be wiped each time the
AppVM restarts.

dcrpulse keeps all daemon data in Docker named volumes (the shared `/app-data` volume),
which live under `/var/lib/docker/volumes/`. Making `/var/lib/docker` persistent therefore
preserves the entire stack in one step. The Qubes `bind-dirs` mechanism does exactly this:
it bind mounts `/var/lib/docker` from the AppVM's persistent private volume (`/rw`) early in
boot, before the Docker daemon starts.

### Step 1: Prepare a TemplateVM with Docker

Because the AppVM root resets on every boot, Docker must be installed in the TemplateVM. Use
Docker's official package repository rather than the distribution's `docker.io`/`docker`
package, which lags behind. Follow the official guides:

- Debian: https://docs.docker.com/engine/install/debian/
- Fedora: https://docs.docker.com/engine/install/fedora/

Clone a template so you do not modify the base one:

```bash
# In dom0
qvm-clone debian-12 debian-12-docker
```

Open a terminal in the new template (via the Qube Manager, or `qvm-run debian-12-docker
xterm`). A TemplateVM has no direct network access; its package manager reaches the
repositories through the Qubes update proxy automatically, but a plain `curl` does not, so
the GPG key download below is routed through the proxy at `http://127.0.0.1:8082`.

Debian:

```bash
# Install prerequisites
sudo apt update
sudo apt install -y ca-certificates curl git

# Add Docker's official GPG key (through the Qubes update proxy)
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL --proxy http://127.0.0.1:8082 \
  https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

# Add the Docker repository
sudo tee /etc/apt/sources.list.d/docker.sources >/dev/null <<EOF
Types: deb
URIs: https://download.docker.com/linux/debian
Suites: $(. /etc/os-release && echo "$VERSION_CODENAME")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF

# Install Docker Engine
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

Fedora:

```bash
# Add the Docker repository
sudo dnf -y install dnf-plugins-core git
sudo dnf config-manager addrepo --from-repofile https://download.docker.com/linux/fedora/docker-ce.repo

# Install Docker Engine
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

Enable the service and add your user to the `docker` group. Both must be done in the
template so they persist into the AppVM:

```bash
sudo systemctl enable docker
sudo usermod -aG docker user
```

Shut the template down:

```bash
# In dom0
qvm-shutdown --wait debian-12-docker
```

### Step 2: Create the AppVM

Create an application qube based on the Docker template:

```bash
# In dom0
qvm-create -t debian-12-docker -l blue dcrpulse
```

Give it network access and enough resources:

- **NetVM**: set to `sys-firewall` for a normal connection, or `sys-whonix` to route all of
  the qube's traffic over Tor. Set it in Qube Settings, or
  `qvm-prefs dcrpulse netvm sys-firewall`.
- **Private storage**: the dcrd blockchain alone is around 30 GB and growing, so grow the
  private volume to at least 80 GB. Use Qube Settings -> Disk storage -> Private storage
  max size, or from dom0:

  ```bash
  qvm-volume resize dcrpulse:private 80G
  ```

- **Memory**: give the qube at least 4 GB of RAM (6-8 GB recommended), since dcrd and the
  rest of the daemon stack are memory-hungry. Set it in Qube Settings, or
  `qvm-prefs dcrpulse maxmem 8192`.

### Step 3: Make /var/lib/docker Persistent with bind-dirs

This is the step that keeps your data across reboots. Run it inside the AppVM, before
starting Docker for the first time:

```bash
sudo mkdir -p /rw/config/qubes-bind-dirs.d
echo "binds+=( '/var/lib/docker' )" | sudo tee /rw/config/qubes-bind-dirs.d/50_user.conf
```

Reboot the AppVM to apply it:

```bash
# In dom0
qvm-shutdown --wait dcrpulse
qvm-start dcrpulse
```

On each boot, Qubes copies `/var/lib/docker` into `/rw/bind-dirs/var/lib/docker` (only the
first time, if it is not already there) and bind mounts it back over `/var/lib/docker`
before the Docker daemon starts. Because `/rw` is the AppVM's persistent private volume,
every Docker image, container, and named volume now survives reboots and TemplateVM
updates.

Verify the bind mount is active in the AppVM:

```bash
findmnt /var/lib/docker
ls /rw/bind-dirs/var/lib/docker
```

`findmnt` should show `/var/lib/docker` as a bind mount, and the second command should list
the Docker data directory (`volumes`, `image`, `containers`, and so on).

### Step 4: Install and Run dcrpulse

Inside the AppVM, install dcrpulse exactly as on any other Linux host:

```bash
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse
cp env.example .env
nano .env   # set DCRD_RPC_PASS and DCRWALLET_RPC_PASS
docker compose up -d
```

Docker starts automatically on boot (enabled in Step 1), so the stack comes back up after a
reboot, reading its data from the now-persistent `/var/lib/docker`.

### Step 5: Access the Dashboard

The simplest option is to run a browser inside the `dcrpulse` AppVM and open
http://localhost:8080.

To reach the dashboard from a different qube (for example a dedicated browser qube), forward
the port with Qubes' `qubes.ConnectTCP` service. In the qube that should reach the
dashboard:

```bash
qvm-connect-tcp 8080:dcrpulse:8080
```

Then open http://localhost:8080 in that qube. This requires a matching `qubes.ConnectTCP`
policy entry in dom0 allowing the connection.

### Qubes Notes

- **Template updates are safe**: updating or reinstalling the TemplateVM resets the AppVM
  root, but `/var/lib/docker` lives in the private volume through bind-dirs, so your synced
  chain and wallet data are preserved.
- **Empty Docker data after a reboot** means the bind did not apply. Confirm
  `/rw/config/qubes-bind-dirs.d/50_user.conf` exists, that `findmnt /var/lib/docker` shows a
  bind mount, and that Docker started after bind-dirs (a reboot is the reliable way to order
  this).
- **"No space left on device"**: the blockchain grows over time. Increase the private
  volume, for example `qvm-volume resize dcrpulse:private 100G` from dom0.
- **Full-Tor operation**: set the AppVM's NetVM to `sys-whonix` to route all traffic over
  Tor. dcrpulse's own in-app Tor toggle still applies on top of this.

---

## Testnet Installation

For testing without real DCR:

```bash
# Clone repository
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# Create .env file
cp env.example .env

# Edit .env and enable testnet
nano .env
```

**Add/uncomment**:
```bash
DCRD_TESTNET=1
```

**Start services**:
```bash
make start
```

**Testnet benefits**:
- Faster sync (~30-60 minutes)
- Smaller size (~1-2 GB)
- Free testnet coins
- Safe for experimentation

---

## Post-Installation Configuration

### Customize Ports

By default the dashboard is published on host port 8080. To change it, edit
`docker-compose.yml`:

```yaml
services:
  dashboard:
    ports:
      - "8000:8080"  # Serve the dashboard on host port 8000 instead
```

### Transaction Indexing

dcrd runs with `--txindex` enabled by default (set via `DCRD_EXTRA_ARGS` in
`.env`), which allows full transaction lookup by hash for the block explorer.

```bash
# .env
DCRD_EXTRA_ARGS=--txindex
```

After changing it, restart dcrd:
```bash
docker compose restart dcrd
```

Note: enabling `--txindex` on an already-synced node triggers a one-time reindex
that can take a while.

### Adjust Gap Limit

Edit `.env`:
```bash
DCRWALLET_GAP_LIMIT=500  # Increase for older wallets
```

Restart:
```bash
docker compose restart dcrwallet
```

---

## Troubleshooting Installation

### Docker Installation Issues

**Problem**: "Cannot connect to Docker daemon"

**Solution**:
```bash
# Start Docker service
sudo systemctl start docker

# Or on macOS, start Docker Desktop app
```

---

**Problem**: "Permission denied"

**Solution**:
```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Log out and back in, or:
newgrp docker
```

---

**Problem**: "Port already in use"

**Solution**:
```bash
# Find process using port
sudo lsof -i :8080

# Kill process or change port in docker-compose.yml
```

---

### Build Issues

**Problem**: "dcrd build failed"

**Solution**:
```bash
# Clean rebuild
docker compose down
docker compose build --no-cache dcrd
docker compose up -d
```

---

**Problem**: "Out of disk space"

**Solution**:
```bash
# Clean Docker
docker system prune -a

# Check disk space
df -h
```

---

### Network Issues

**Problem**: "Cannot pull images"

**Solution**:
```bash
# Check internet connection
ping google.com

# Check DNS
cat /etc/resolv.conf

# Retry
docker compose pull
```

---

## Verify Installation

### Quick Health Check

```bash
# All services running?
docker compose ps

# Dashboard accessible?
curl http://localhost:8080

# dcrd syncing?
make sync-status
```

### Full Verification

```bash
# Node status
make status

# View logs
make logs

# Check peers
make peers

# Dashboard accessible
# Open http://localhost:8080 in browser
```

---

## Next Steps

After successful installation:

1. **[First Steps](first-steps.md)** - What to do next
2. **[Configuration](../setup/configuration.md)** - Customize settings
3. **[Node Dashboard](../features/node-dashboard.md)** - Monitor your node
4. **[Wallet Dashboard](../features/wallet-dashboard.md)** - Configure and track your wallet

---

## Additional Resources

- **[Documentation Index](../readme.md)** - All documentation
- **[Troubleshooting](../guides/troubleshooting.md)** - Common issues
- **[CLI Commands](../reference/cli-commands.md)** - Command reference

---

**Questions?** Check the [Troubleshooting Guide](../guides/troubleshooting.md).
