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
- `dcrd/` - Blockchain data and RPC certificates
- `dcrwallet/` - Wallet data

## Network Access

- Dashboard is accessible via Umbrel's app proxy (requires authentication)
- dcrd and dcrwallet are connected to both:
  - **default network** - For internet access (blockchain sync, peer discovery)
  - **dcrpulse_internal** - For secure inter-service communication
- Custom DNS servers (1.1.1.1, 8.8.8.8) configured for reliable peer discovery
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
   
   # Start umbrelOS development environment (runs in background)
   npm run dev
   ```

   Wait for umbrelOS to start (about 30-60 seconds).

2. **Find your umbrel-dev container IP (if http://umbrel-dev.local domain doesn't work):**

   ```bash
   # Get the container IP address
   docker inspect umbrel-dev --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'
   # Example output: 172.17.0.2
   
   # Access via IP: http://172.17.0.2
   ```

   Create an account at http://umbrel-dev.local or http://<container-ip> and set a password

3. **Copy the app directory to the umbrel-dev container:**

   ```bash
   # Get the app store hash (changes on each fresh start)
   docker exec umbrel-dev ls /home/umbrel/umbrel/app-stores/
   # Example output: getumbrel-umbrel-apps-github-53f74447
   
   # Copy files directly to the container
   rsync -av --exclude=".gitkeep" /path/to/dcrpulse/dcrpulse-umbrel/ umbrel@<container-ip>:/home/umbrel/umbrel/app-stores/getumbrel-umbrel-apps-github-<hash>/dcrpulse/
   ```

4. **Fix permissions (required in dev environment):**

   ```bash
   # Pre-create app data directories with correct ownership
   docker exec umbrel-dev sh -c "mkdir -p /home/umbrel/umbrel/app-data/dcrpulse/dcrd /home/umbrel/umbrel/app-data/dcrpulse/dcrwallet && chown -R 1000:1000 /home/umbrel/umbrel/app-data/dcrpulse"
   ```

5. **Install the app:**

   From the umbrelOS homescreen, go to the App Store and find dcrpulse, then click Install.
   
   Or install via command line:
   ```bash
   docker exec umbrel-dev sh -c "cd /umbrel-dev && \
     npm run start --prefix packages/umbreld client apps.install.mutate '{\"appId\":\"dcrpulse\"}'"
   ```

6. **Access the app:**

   - Via homescreen: http://umbrel-dev.local or http://<container-ip>
   - Direct API access: http://<container-ip>:8080/api/dashboard

7. **Monitor sync progress:**

   ```bash
   # Check dcrd logs
   docker exec umbrel-dev docker logs dcrpulse_dcrd_1 --tail 50
   
   # Check all containers status
   docker exec umbrel-dev docker ps --filter name=dcrpulse
   
   # API endpoint for dashboard data
   docker exec umbrel-dev docker exec dcrpulse_dashboard_1 \
     wget -q -O- http://localhost:8080/api/dashboard | python3 -m json.tool
   ```

8. **Uninstall (for testing):**

   ```bash
   docker exec umbrel-dev sh -c "cd /umbrel-dev && \
     npm run start --prefix packages/umbreld client apps.uninstall.mutate '{\"appId\":\"dcrpulse\"}'"
   ```

> **Note:** The permission fix (step 4) is only needed in the dev environment. Production Umbrel installations have proper ownership configured automatically.

### Testing on Physical Device

If you have umbrelOS running on a Raspberry Pi, Umbrel Home, or other hardware:

1. **Copy the app to your umbrelOS device:**

   ```bash
   # Find the app store directory hash
   ssh umbrel@umbrel.local "ls /home/umbrel/umbrel/app-stores/"
   
   # Copy files to the device
   rsync -av --exclude=".gitkeep" \
     dcrpulse-umbrel/ \
     umbrel@umbrel.local:/home/umbrel/umbrel/app-stores/getumbrel-umbrel-apps-github-<hash>/dcrpulse/
   ```

2. **Install via the App Store UI** or SSH:

   ```bash
   ssh umbrel@umbrel.local "umbreld client apps.install.mutate --appId dcrpulse"
   ```

3. **Uninstall (for testing):**

   ```bash
   ssh umbrel@umbrel.local "umbreld client apps.uninstall.mutate --appId dcrpulse"
   ```

> **Note:** When testing, verify that application state persists across restarts. Restart the app (right-click icon → Restart) and ensure no data is lost. All persistent data should be in volumes. On production Umbrel, permissions are handled automatically.

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

