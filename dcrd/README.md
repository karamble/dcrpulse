# dcrd Docker Build

This directory contains the Dockerfile to build `dcrd` from the official Decred source repository.

## Why Build from Source?

The official `decred/dcrd` Docker Hub image is outdated (last updated 6+ years ago). Building from source ensures you're running the latest version with all security updates and features.

## Source Repository

This Dockerfile is based on the official dcrd repository:
- **Repository**: https://github.com/decred/dcrd
- **Docker Contrib**: https://github.com/decred/dcrd/tree/master/contrib/docker

## What Gets Built

The multi-stage Dockerfile:

1. **Build Stage** (golang:1.26-alpine):
   - Clones the official dcrd repository
   - Builds `dcrd` (the node daemon)
   - Builds `dcrctl` (the control utility)

2. **Runtime Stage** (alpine:latest):
   - Minimal Alpine Linux base
   - Only contains the compiled binaries
   - Runs as non-root user (`dcrd`)
   - Small image size (~30-40MB)

## Customization

### Build Specific Version

By default, builds from `master` branch. To build a specific version:

**Option 1: Via environment variable**

Edit `.env`:
```env
DCRD_VERSION=release-v2.0.6
```

**Option 2: Via docker-compose build arg**

```bash
docker-compose build --build-arg DCRD_VERSION=release-v2.0.6 dcrd
```

**Option 3: Direct docker build**

```bash
cd dcrd
docker build --build-arg DCRD_VERSION=release-v2.0.6 -t dcrd:v2.0.6 .
```

### Available Versions

Find available tags and branches at:
https://github.com/decred/dcrd/tags
https://github.com/decred/dcrd/branches

Examples:
- `master` - Latest development (default)
- `release-v2.0.6` - Stable release v2.0.6
- `release-v2.0.5` - Stable release v2.0.5
- `release-v2.0.4` - Stable release v2.0.4

## Building

### Via Docker Compose (Recommended)

```bash
# Build all services including dcrd
docker-compose build

# Build only dcrd
docker-compose build dcrd

# Build with no cache (clean build)
docker-compose build --no-cache dcrd
```

### Standalone Build

```bash
cd dcrd
docker build -t dcrd:latest .

# Build specific version
docker build --build-arg DCRD_VERSION=release-v2.0.6 -t dcrd:v2.0.6 .
```

## Dockerfile Details

### Build Arguments

- `DCRD_VERSION`: Git branch or tag to build (default: `master`)

### Exposed Ports

- `9108` - Mainnet P2P port
- `9109` - Mainnet RPC port
- `19108` - Testnet P2P port
- `19109` - Testnet RPC port

### Volumes

- `/home/dcrd/.dcrd` - Blockchain data directory

### User

Runs as non-root user `dcrd` for security.

## Included Binaries

- **`dcrd`** - The Decred daemon (full node)
- **`dcrctl`** - Command-line utility to control dcrd

## Usage Examples

### Run Standalone

```bash
docker build -t dcrd:latest .

docker run -d \
  --name dcrd \
  -p 9108:9108 -p 9109:9109 \
  -v dcrd-data:/home/dcrd/.dcrd \
  dcrd:latest \
  --rpcuser=decred \
  --rpcpass=yourpassword \
  --rpclisten=0.0.0.0:9109 \
  --notls
```

### Use dcrctl

```bash
# Inside running container
docker exec dcrd dcrctl --rpcuser=decred --rpcpass=yourpassword --rpcserver=127.0.0.1:9109 --notls getinfo

# Get block count
docker exec dcrd dcrctl --rpcuser=decred --rpcpass=yourpassword --rpcserver=127.0.0.1:9109 --notls getblockcount

# Get peer info
docker exec dcrd dcrctl --rpcuser=decred --rpcpass=yourpassword --rpcserver=127.0.0.1:9109 --notls getpeerinfo
```

## Build Time

- **First build**: 5-10 minutes (downloads Go modules, compiles)
- **Subsequent builds**: Uses Docker cache (faster)
- **Clean build**: 5-10 minutes

## Image Size

- **Build stage**: ~500MB (includes Go compiler and build tools)
- **Final image**: ~30-40MB (only runtime dependencies and binaries)

## Security Features

- ✅ Runs as non-root user
- ✅ Minimal Alpine base image
- ✅ No unnecessary tools or packages
- ✅ Built from official source
- ✅ Multi-stage build (no build artifacts in final image)

## Updating dcrd

To update to the latest version:

```bash
# Pull latest source and rebuild
docker-compose build --no-cache dcrd
docker-compose up -d dcrd

# Or build specific version
DCRD_VERSION=release-v2.0.7 docker-compose build --no-cache dcrd
docker-compose up -d dcrd
```

**Note**: Blockchain data persists in the volume, so you won't need to re-sync.

## Troubleshooting

### Build fails with "git clone failed"

Check your internet connection and try again. The build needs to clone the dcrd repository from GitHub.

### Build is very slow

First build downloads all Go dependencies. Subsequent builds use Docker's build cache and are much faster.

### Out of disk space during build

The build stage requires ~500MB. Ensure you have sufficient disk space:

```bash
docker system df
docker system prune  # Clean up unused data
```

### Want to use a specific commit

```bash
docker build --build-arg DCRD_VERSION=abc123def456 -t dcrd:custom .
```

Replace `abc123def456` with the actual commit hash.

## Comparison: Old Image vs Build from Source

| Aspect | Old Image (decred/dcrd:latest) | Build from Source |
|--------|-------------------------------|-------------------|
| **Last Updated** | 6+ years ago | Current (you control) |
| **Version** | Very old | Latest or specific version |
| **Security** | Outdated | Up-to-date |
| **Customization** | None | Full control |
| **Build Time** | Instant (pull) | 5-10 minutes (build) |
| **Source** | Unknown | Official GitHub repo |

## References

- **dcrd Repository**: https://github.com/decred/dcrd
- **Docker Contrib**: https://github.com/decred/dcrd/tree/master/contrib/docker
- **Decred Documentation**: https://docs.decred.org/
- **Release Notes**: https://github.com/decred/dcrd/releases

## License

This Dockerfile builds dcrd, which is licensed under the ISC License.
See: https://github.com/decred/dcrd/blob/master/LICENSE

