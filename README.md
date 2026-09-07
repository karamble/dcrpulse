# dcrpulse

The full Decred stack, on hardware you own.

![License](https://img.shields.io/badge/license-ISC-blue.svg)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![React](https://img.shields.io/badge/React-18+-61DAFB?logo=react)
![TypeScript](https://img.shields.io/badge/TypeScript-5+-3178C6?logo=typescript)

## What is dcrpulse?

Six daemons and one dashboard. [dcrd](https://github.com/decred/dcrd) for the
chain, [dcrwallet](https://github.com/decred/dcrwallet) for the coins,
[dcrlnd](https://github.com/decred/dcrlnd) for Lightning,
[bisonw](https://github.com/decred/dcrdex) for DCRDEX,
[brclientd](https://github.com/karamble/brclientd) for Bison Relay, and
[Tor](https://www.torproject.org/) in front of all of them once you switch it
on, with an inbound onion service for dcrd and another for Lightning. dcrpulse
starts them in the right order, wires them to each other, and puts a single web
interface over the lot.

Run the chain, hold the coins, stake them, trade them, and talk to people. No
custodians. No accounts. Your keys, your node.

The node syncs the full chain with transaction indexing. The wallet is a real
wallet: several of them, with accounts, xpub import and watch-only, and a mixer
that shuffles your coins in with everyone else's. Buy tickets and stake through a
VSP in a few clicks, or let the autobuyer do it. Open Lightning channels and pay
invoices. Trade peer to peer on DCRDEX, and run a market maker on it if you want
one. Message, post and tip over Bison Relay. Timestamp a file against the chain.
And read blocks, mempool and the treasury in an explorer that is genuinely nice
to look at.

**Everything past the node is optional.** Start as a node and an explorer, and
switch the rest on when you want it.

- Full dcrd node with transaction indexing
- Multi-wallet dcrwallet with accounts, xpub import and watch-only
- Shared wallets: m-of-n multisig, set up and signed over Bison Relay
- Staking with VSP support and an automatic ticket buyer
- CoinShuffle++ privacy mixer
- Lightning Network through dcrlnd
- DCRDEX for non-custodial trading and market making
- Bison Relay for encrypted messaging, posts and tipping
- Block explorer with mempool and treasury monitoring
- Governance: consensus agendas, Politeia proposals and treasury policy
- dcrtime timestamping with verifiable proofs
- Alerts for node, wallet, staking, Lightning, DEX, Bison Relay and disk conditions
- Optional Tor routing for the whole stack, with inbound onion services for dcrd and Lightning
- Optional MCP interfaces for AI agents, off by default ([guide](docs/features/ai-agents-mcp.md))

Node, wallet and explorer data comes from your own dcrd and dcrwallet over RPC,
never from a third party. The optional features reach the services they have to:
Politeia for proposals, dcrtime for timestamps, your chosen VSP, DEX servers,
Bison Relay relays, and a rate source for the DCR price.

## Quick Start

### Prerequisites
- Docker and Docker Compose
- 50 GB+ free disk space - the chain alone is around 30 GB and growing

### Launch

```bash
# 1. Clone the repository
git clone https://github.com/karamble/dcrpulse.git
cd dcrpulse

# 2. Set up environment
cp env.example .env
# Edit .env with your preferred RPC password
nano .env

# 3. Start all services
docker compose up -d

# 4. Access the dashboard
# Open http://localhost:8080 in your browser
```

The first run will sync the blockchain (takes 4-8 hours for mainnet). Monitor progress with:
```bash
docker compose logs -f dcrd
```

### Using Makefile

```bash
make start       # Start all services
make stop        # Stop all services
make logs        # View logs
make status      # Check status
```

For more commands: `make help`

## Documentation

Complete documentation is available in the [`docs/`](docs/) folder:

**[Documentation Index](docs/readme.md)** - Start here

### Quick Links

**Getting Started**
- [First Steps](docs/getting-started/first-steps.md) - What to do after installation
- [Installation Guide](docs/getting-started/installation.md) - Detailed setup instructions
- [Configuration Guide](docs/setup/configuration.md) - Configuration options

**Guides**
- [Wallet Operations](docs/guides/wallet-operations.md) - Import xpub, rescan, sync monitoring
- [Backup & Restore](docs/guides/backup-restore.md) - Protect your blockchain data
- [Troubleshooting](docs/guides/troubleshooting.md) - Common issues and solutions

**Features**
- [Node Dashboard](docs/features/node-dashboard.md) - Monitor your dcrd node
- [Wallet Dashboard](docs/features/wallet-dashboard.md) - Track balances and staking
- [Block Explorer](docs/features/explorer.md) - Browse blocks and transactions

**Deployment**
- [Production Deployment](docs/deployment/production.md) - Production setup
- [Monitoring Setup](docs/deployment/monitoring-setup.md) - Health checks and alerts

**Reference**
- [CLI Commands](docs/reference/cli-commands.md) - Makefile and Docker commands
- [Configuration](docs/setup/configuration.md) - All configuration options

## Project Structure

```
dcrpulse/
├── dashboard/          # Unified dashboard application
│   ├── cmd/           # Main application entry point
│   ├── internal/      # Go backend code
│   └── web/           # Frontend React app
├── dcrd/              # dcrd node Docker setup
├── dcrwallet/         # dcrwallet Docker setup
├── dcrlnd/            # Lightning daemon Docker setup
├── brclientd/         # Bison Relay daemon Docker setup
├── dcrdex/            # DCRDEX (bisonw) Docker setup
├── tor/               # Tor proxy Docker setup
├── dcrpulse-umbrel/   # Umbrel app package
├── dcrpulse-casaos/   # CasaOS app package
├── umbrel-widget/     # Umbrel home-screen widget
├── docs/              # Documentation
└── docker-compose.yml # Orchestration
```

See [dashboard/README.md](dashboard/README.md) for development and build instructions.

## Development

The dashboard combines backend and frontend into a single Go binary with embedded static files.

**Development mode** (hot reload):
```bash
# Terminal 1: Backend
cd dashboard
go run ./cmd/dcrpulse

# Terminal 2: Frontend
cd dashboard/web
npm install
npm run dev
```

Frontend dev server runs on http://localhost:3000 and proxies API calls to backend on http://localhost:8080.

**Production build**:
```bash
cd dashboard/web && npm run build
cd .. && go build ./cmd/dcrpulse
./dcrpulse  # Serves on http://localhost:8080
```

## Support

For issues and questions:
- [GitHub Issues](https://github.com/karamble/dcrpulse/issues)
- [Decred Matrix](https://chat.decred.org)
- [Decred Discord](https://discord.gg/decred)

## Related Projects

- [demarchy](https://github.com/karamble/demarchy) - a Decred mark for the
  [Omarchy](https://omarchy.org/) bar. It shows your staking record, the network
  you vote in, and your Bison Relay messages, all read from dcrpulse over its MCP
  interface with a token that can only read.

## License

ISC License - Part of the Decred community projects.

---

**Made with ❤️ for the Decred community**
