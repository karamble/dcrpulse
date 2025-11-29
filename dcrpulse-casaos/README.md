# dcrpulse for CasaOS

CasaOS AppStore integration for dcrpulse - a comprehensive Decred blockchain explorer and wallet dashboard.

## Overview

This directory contains the CasaOS-specific configuration for dcrpulse, enabling it to be submitted to and installed from the [CasaOS AppStore](https://github.com/IceWhaleTech/CasaOS-AppStore).

## What is dcrpulse?

dcrpulse combines three essential Decred components into one integrated application:

- **dcrd**: Full Decred blockchain node with P2P synchronization
- **dcrwallet**: HD wallet backend with secure key management
- **dashboard**: Modern web interface for monitoring and managing both node and wallet

## Network Architecture

One of the key features of this CasaOS implementation is its **secure network isolation**:

### Network Topology

```
┌─────────────────────────────────────────┐
│         Internet / P2P Network           │
└──────────────┬──────────────────────────┘
               │
               │ (blockchain sync)
               │
         ┌─────▼─────┐
         │   dcrd    │ ◄── Public network (dcrpulse)
         │  :9109    │     Has internet access
         └─────┬─────┘
               │
               │ (RPC cert sharing)
               │
    ┌──────────┴──────────┐
    │                     │
┌───▼────┐          ┌────▼─────┐
│dcrwallet│          │dashboard │
│ :9110   │          │  :8080   │
│ :9111   │◄─────────┤ (Web UI) │
└─────────┘          └──────────┘
    │                     │
    └──────────┬──────────┘
               │
    Internal Network (dcrpulse_internal)
    NO Internet Access - Isolated
```

### Security Benefits

1. **dcrd** (`dcrpulse` network):
   - Has internet access for blockchain P2P synchronization
   - Required to download and verify blockchain data
   - Generates RPC certificates for secure communication

2. **dcrwallet** (`dcrpulse_internal` network only):
   - **Completely isolated from internet** - cannot make outbound connections
   - Only communicates with dcrd via internal network
   - Protects wallet private keys from network exposure

3. **dashboard** (`dcrpulse_internal` network only):
   - **Completely isolated from internet** - cannot make outbound connections
   - Only communicates with dcrd and dcrwallet via internal network
   - Only port 8080 exposed to local network for web UI access

This architecture follows CasaOS best practices (similar to the Dify app) and provides defense-in-depth security.

## Files

- **docker-compose.yml**: Main CasaOS app definition with service configurations and metadata
- **icon.png**: App icon (192x192 PNG, transparent background)
- **screenshot_{1,2,3}.jpg**: App screenshots (1280x720 JPG)
- **README.md**: This file

## Default Credentials

For RPC communication between services:
- **Username**: `casaos`
- **Password**: `casaos`

⚠️ **Security Note**: These are default credentials. For production use, consider changing them by modifying the environment variables in the docker-compose.yml file.

## System Requirements

- **Disk Space**: ~15GB for blockchain data
- **RAM**: 4GB minimum recommended
- **Network**: Stable internet connection for initial sync
- **Architecture**: amd64 or arm64

## Installation

### Via CasaOS App Store (Recommended)

Once submitted and approved:

1. Open CasaOS Web UI
2. Navigate to App Store
3. Search for "dcrpulse"
4. Click Install
5. Wait for initial blockchain sync (4-8 hours)

### Manual Installation for Testing

For local testing before submission:

1. Copy this directory to your CasaOS test environment
2. Place it in the appropriate app store directory structure
3. Follow [CasaOS App Store contribution guidelines](https://github.com/IceWhaleTech/CasaOS-AppStore/blob/main/CONTRIBUTING.md)

## Local Testing with CasaOS

### Prerequisites

- A running CasaOS installation
- Docker and Docker Compose installed
- Access to CasaOS Web UI

### Testing Steps

1. **Install the app** from your local/test app store
2. **Monitor dcrd sync**: Check logs to verify blockchain synchronization
3. **Verify network isolation**:
   ```bash
   # dcrd should be able to ping external hosts
   docker exec dcrpulse-dcrd ping -c 1 8.8.8.8
   
   # dcrwallet should NOT have internet access (should fail)
   docker exec dcrpulse-dcrwallet ping -c 1 8.8.8.8
   
   # dashboard should NOT have internet access (should fail)
   docker exec dcrpulse-dashboard ping -c 1 8.8.8.8
   ```
4. **Test internal communication**:
   ```bash
   # dcrwallet should reach dcrd
   docker exec dcrpulse-dcrwallet nc -zv dcrd 9109
   
   # dashboard should reach both dcrd and dcrwallet
   docker exec dcrpulse-dashboard nc -zv dcrd 9109
   docker exec dcrpulse-dashboard nc -zv dcrwallet 9110
   ```
5. **Access Web UI**: Navigate to `http://your-casaos-ip:8080`
6. **Test wallet functions**: Create/import wallet, generate addresses, etc.
7. **Test explorer functions**: Browse blocks, search transactions, view mempool

## First Run

On first installation:

1. **Blockchain Sync**: dcrd will begin downloading and verifying the Decred blockchain. This process takes 4-8 hours depending on your internet speed and system performance.

2. **Monitor Progress**: Access the dashboard at port 8080 to see sync progress in real-time.

3. **Wallet Setup**: Once dcrd is synced, you can create a new wallet or import an existing one using an extended public key (xpub).

## Data Persistence

All data is stored in `/DATA/AppData/dcrpulse/`:
- `/DATA/AppData/dcrpulse/dcrd/`: Blockchain data, RPC certificates
- `/DATA/AppData/dcrpulse/dcrwallet/`: Wallet data, keys (encrypted)

**Backup Important**: Always backup your wallet data before uninstalling!

## Differences from Umbrel Version

| Aspect | Umbrel | CasaOS |
|--------|--------|--------|
| Metadata | Separate `umbrel-app.yml` | Inline `x-casaos` |
| Networks | Custom isolated + app_proxy | Two bridge networks |
| Internet | dcrd via TOR proxy optional | dcrd direct, wallet/dashboard isolated |
| Volumes | `${APP_DATA_DIR}/data` | `/DATA/AppData/$AppID` |
| Auth | `${APP_SEED}` for RPC | Hardcoded `casaos:casaos` |
| Variables | `$TOR_PROXY_IP`, `$APP_SEED` | `$PUID`, `$PGID`, `$TZ` |
| Assets | Local files in repo | jsDelivr CDN URLs |
| Port Exposure | Via app_proxy | Direct port 8080 |

## Troubleshooting

### Blockchain Sync Stuck

- Check dcrd logs: `docker logs dcrpulse-dcrd`
- Verify internet connectivity for dcrd container
- Ensure sufficient disk space

### Dashboard Can't Connect to dcrd/dcrwallet

- Verify all containers are running: `docker ps`
- Check health status: `docker ps --format "table {{.Names}}\t{{.Status}}"`
- Verify internal network: `docker network inspect dcrpulse_dcrpulse_internal`

### High Resource Usage

- Initial sync is CPU/disk intensive - this is normal
- After sync, resource usage should stabilize
- Adjust memory limits in docker-compose.yml if needed

## Contributing

To contribute improvements to this CasaOS integration:

1. Test changes locally on CasaOS
2. Validate docker-compose.yml syntax
3. Test network isolation
4. Submit PR to main dcrpulse repository
5. Separately submit to CasaOS AppStore following their guidelines

## Resources

- **dcrpulse Repository**: https://github.com/karamble/dcrpulse
- **CasaOS AppStore**: https://github.com/IceWhaleTech/CasaOS-AppStore
- **CasaOS Documentation**: https://casaos.io/docs
- **Decred Documentation**: https://docs.decred.org/

## Support

For issues specific to:
- **dcrpulse application**: Open issue on [dcrpulse repository](https://github.com/karamble/dcrpulse/issues)
- **CasaOS integration**: Open issue on dcrpulse repository with `[CasaOS]` tag
- **CasaOS platform**: Refer to [CasaOS support channels](https://casaos.io/community)

## License

Same as main dcrpulse project - see LICENSE file in repository root.

