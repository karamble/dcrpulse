# Decred Pulse Documentation

Welcome to the Decred Pulse documentation. These guides cover installing, configuring, and using your self-hosted Decred node, wallet, and service stack dashboard.

## Documentation Index

### Getting Started

Start here if you are new to Decred Pulse:

- **[Installation Guide](getting-started/installation.md)** - Install the stack with Docker Compose
- **[First Steps](getting-started/first-steps.md)** - What to do after installation

### Setup and Configuration

- **[Configuration Guide](setup/configuration.md)** - Environment variables and per-daemon settings

### Features

The dashboard is organized into these areas:

- **[Node Dashboard](features/node-dashboard.md)** - Monitor your dcrd node: sync, peers, mempool
- **[Wallet Dashboard](features/wallet-dashboard.md)** - Accounts, balances, and transactions
- **[Multiple Wallets](features/multi-wallet.md)** - Create, switch, and manage several wallets
- **[Staking Guide](features/staking-guide.md)** - Tickets, the autobuyer, and pool statistics
- **[Governance](features/governance.md)** - Consensus voting, Politeia proposals, and the treasury
- **[Lightning Network](features/lightning.md)** - dcrlnd channels and off-chain payments
- **[DEX (DCRDEX)](features/dex.md)** - Decentralized trading via bisonw
- **[Bison Relay](features/bison-relay.md)** - Encrypted messaging, posts, pages, and storefront
- **[Privacy and the Mixer](features/privacy-mixer.md)** - Account-based CoinShuffle++ mixing
- **[Timestamping (dcrtime)](features/timestamp.md)** - Anchor file hashes to the chain
- **[Block Explorer](features/explorer.md)** - Browse blocks, transactions, and addresses
- **[Settings](features/settings.md)** - Tor, themes, the app-password gate, logs, and about

### User Guides

- **[Wallet Operations](guides/wallet-operations.md)** - Import xpub, rescan, sync monitoring
- **[Backup and Restore](guides/backup-restore.md)** - Protect wallet, Lightning, and node data
- **[Troubleshooting](guides/troubleshooting.md)** - Common issues and fixes

### API

- **[API Reference](api/api-reference.md)** - REST endpoints grouped by feature

### Development

- **[Architecture](development/architecture.md)** - System design, daemons, and data flow

### Deployment

- **[Production Deployment](deployment/production.md)** - Reverse proxy, TLS, backups
- **[Security Best Practices](deployment/security.md)** - Hardening guidance
- **[Performance Tuning](deployment/performance.md)** - Optimize the stack
- **[Monitoring Setup](deployment/monitoring-setup.md)** - Metrics and alerting

### Reference

- **[CLI Commands](reference/cli-commands.md)** - Makefile and Docker Compose commands

---

## Quick Navigation by Role

### I'm a node operator
1. [Installation Guide](getting-started/installation.md)
2. [Node Dashboard](features/node-dashboard.md)
3. [Backup and Restore](guides/backup-restore.md)

### I'm managing a wallet
1. [Wallet Dashboard](features/wallet-dashboard.md)
2. [Wallet Operations](guides/wallet-operations.md)
3. [Multiple Wallets](features/multi-wallet.md)

### I'm staking
1. [Staking Guide](features/staking-guide.md)
2. [Governance](features/governance.md)

### I'm using Lightning
1. [Lightning Network](features/lightning.md)
2. [Wallet Dashboard](features/wallet-dashboard.md)

### I'm trading on the DEX
1. [DEX (DCRDEX)](features/dex.md)

### I'm using Bison Relay
1. [Bison Relay](features/bison-relay.md)

### I'm a developer
1. [Architecture](development/architecture.md)
2. [API Reference](api/api-reference.md)
3. [Configuration Guide](setup/configuration.md)

### I'm deploying to production
1. [Production Deployment](deployment/production.md)
2. [Security Best Practices](deployment/security.md)
3. [Monitoring Setup](deployment/monitoring-setup.md)

---

## What is Decred Pulse?

Decred Pulse is a modern, self-hosted dashboard for running and monitoring a Decred node, wallet, and the wider Decred service stack in real time. All data comes from your own daemons over RPC; no third-party services are required.

### Key Features

- Node monitoring: sync progress, blockchain info, peers, and mempool.
- Wallet management: multiple wallets, account balances, transaction history, watch-only (xpub) import, and rescans.
- Staking: ticket purchasing, the autobuyer, VSP support, and pool statistics.
- Governance: consensus agenda voting, Politeia proposals, and treasury TSpend voting.
- Lightning: dcrlnd channels, payments, and invoices.
- DEX: decentralized trading through DCRDEX (bisonw).
- Bison Relay: encrypted messaging, group chats, posts, pages, and a storefront.
- Privacy: the account-based CoinShuffle++ mixer.
- Timestamping: anchoring file hashes to the chain with dcrtime.
- Block explorer: browse blocks, transactions, and addresses from your own node.
- Tor support, custom themes, and an optional app-password gate.

### Architecture

A single dashboard container serves the embedded React UI and the REST API on port 8080, and talks to each daemon over RPC or gRPC:

```
Browser
  | HTTP / REST (port 8080)
  v
dashboard  (Go backend + embedded React UI)
  |
  | RPC / gRPC
  +-- dcrd        full node
  +-- dcrwallet   wallet
  +-- dcrlnd      Lightning
  +-- brclientd   Bison Relay
  +-- dcrdex      DEX (bisonw)
  +-- tor         proxy
```

### Technology Stack

Frontend:
- React 18 and TypeScript
- Vite and Tailwind CSS
- Built and embedded into the Go binary and served by the dashboard (there is no separate web server)

Backend:
- Go with the Gorilla Mux router
- RPC and gRPC clients for dcrd and dcrwallet, gRPC to dcrlnd, and HTTP/RPC to brclientd and bisonw
- Same-origin protection on the API and an optional app-password gate

Infrastructure:
- Docker and Docker Compose
- Daemons: dcrd, dcrwallet, dcrlnd, brclientd, dcrdex (bisonw), and tor

---

## Getting Help

- Browse the [documentation index](#documentation-index) above.
- Review [Troubleshooting](guides/troubleshooting.md).
- Decred community: GitHub issues, Discord, and Matrix.

### Logs

```bash
# all services
docker compose logs -f

# a specific service
docker compose logs -f dashboard
docker compose logs -f dcrd
```

---

## Quick Links

- Main README: [README.md](../README.md)
- License: [ISC License](../LICENSE)
