# Decred Pulse for umbrelOS

This directory contains the Umbrel-specific configuration for running Decred Pulse on umbrelOS.

## Overview

Decred Pulse is a comprehensive Decred node dashboard that includes:
- Full dcrd blockchain node with transaction indexing
- dcrwallet with watch-only wallet support
- Block explorer with mempool monitoring
- Transaction categorization (regular, tickets, votes, revocations, CoinJoin)
- Wallet management with xpub/vpub import support

## Architecture

The app consists of three Docker containers:
1. **dcrd** - Full Decred node (mainnet)
2. **dcrwallet** - Decred wallet daemon
3. **dashboard** - Web UI and API backend

All services use pre-built images from GitHub Container Registry (GHCR).

## Data Storage

All persistent data is stored in `${APP_DATA_DIR}`:
- `dcrd/` - Blockchain data
- `dcrwallet/` - Wallet data
- `certs/` - RPC certificates (shared)

## Network Access

- Dashboard is accessible via Umbrel's app proxy (requires authentication)
- dcrd and dcrwallet are isolated on an internal network
- Only the dashboard can access dcrd and dcrwallet (not accessible to other Umbrel apps)
- No ports are exposed to the host or external network

## Testing

### Local Testing (Development Environment)

1. **Set up umbrelOS development environment:**

   ```bash
   # Clone the Umbrel repository
   git clone https://github.com/getumbrel/umbrel.git
   cd umbrel
   
   # Install dependencies
   npm install
   
   # Start umbrelOS development environment
   npm run dev
   ```

   Wait for umbrelOS to start and visit http://umbrel-dev.local to create an account.

2. **Copy the app directory to the umbrel-dev app store:**

   ```bash
   # From your machine, copy dcrpulse-umbrel to umbrel's app-stores directory
   rsync -av --exclude=".gitkeep" /path/to/dcrpulse/dcrpulse-umbrel ~/path/to/umbrel/app-stores/dcrpulse
   ```

3. **Install the app:**

   From the umbrelOS homescreen at http://umbrel-dev.local, go to the App Store and find Decred Pulse, then click Install.
   
   Or install using umbrel-dev scripts:
   ```bash
   # From the umbrel repository directory
   npm run dev client -- apps.install.mutate -- --appId dcrpulse
   ```

4. **Access the app:**

   The app should now be accessible at http://umbrel-dev.local (via the homescreen icon)

5. **Check logs:**

   ```bash
   docker compose logs -f dcrpulse_dcrd_1
   docker compose logs -f dcrpulse_dcrwallet_1
   docker compose logs -f dcrpulse_dashboard_1
   ```

### Testing on Physical Device

If you have umbrelOS running on a Raspberry Pi, Umbrel Home, or other hardware:

1. **Copy the app to your umbrelOS device:**

   ```bash
   rsync -av --exclude=".gitkeep" dcrpulse-umbrel/ umbrel@umbrel.local:/home/umbrel/umbrel/app-stores/getumbrel-umbrel-apps-github-53f74447/dcrpulse/
   ```

2. **Install via the App Store UI** or via terminal:

   ```bash
   ssh umbrel@umbrel.local
   umbreld client apps.install.mutate --appId dcrpulse
   ```

### Uninstall

**Development environment:**
```bash
# From umbrel repository directory
npm run dev client -- apps.uninstall.mutate -- --appId dcrpulse
```

**Physical device:**
```bash
umbreld client apps.uninstall.mutate --appId dcrpulse
```

> **Note:** When testing, verify that application state persists across restarts. Restart the app (right-click icon → Restart) and ensure no data is lost. All persistent data should be in volumes.

## Submission to Umbrel App Store

1. Fork the [getumbrel/umbrel-apps](https://github.com/getumbrel/umbrel-apps) repository
2. Copy this entire directory to `umbrel-apps/dcrpulse/`
3. Add gallery images (3-5 high-quality screenshots, 1440x900px PNG)
4. Add icon (256x256 SVG, no rounded corners)
5. Open a pull request with the following template:

### Pull Request Template

```markdown
# App Submission

### App name
Decred Pulse

### 256x256 SVG icon
(Upload icon with no rounded corners)

### Gallery images
(Upload 3-5 screenshots at 1440x900px)

### I have tested my app on:
- [ ] umbrelOS on a Raspberry Pi
- [ ] umbrelOS on an Umbrel Home
- [ ] umbrelOS on Linux VM
```

## Resources

- **Main Repository**: https://github.com/karamble/dcrpulse
- **Umbrel App Framework**: https://github.com/getumbrel/umbrel-apps
- **Decred Documentation**: https://docs.decred.org/
- **dcrd**: https://github.com/decred/dcrd
- **dcrwallet**: https://github.com/decred/dcrwallet

## Requirements

- Minimum 50GB free storage (blockchain size + growth)
- 2GB RAM for dcrd
- 1GB RAM for dcrwallet
- 256MB RAM for dashboard

Initial blockchain sync will take several hours depending on network speed and hardware.

## Support

For issues and feature requests, please use:
https://github.com/karamble/dcrpulse/issues

