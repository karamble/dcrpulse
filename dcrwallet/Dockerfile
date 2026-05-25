# Dockerfile for building dcrwallet (+ csppsolver) from source
# Based on dcrwallet repository: https://github.com/decred/dcrwallet
# Pattern follows dcrd setup for consistency

# Build stage — debian-based so we can pull in libflint/libmpfr/libgmp,
# which csppsolver links against (CGo). Alpine doesn't carry flint in
# its main repos, so the build stage is debian.
FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git openssl ca-certificates \
    libflint-dev libmpfr-dev libgmp-dev \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /go/src/github.com/decred

# Clone the dcrwallet repository
ARG DCRWALLET_VERSION=release-v2.1.3
RUN git clone --depth 1 --branch ${DCRWALLET_VERSION} https://github.com/decred/dcrwallet.git

# Build dcrwallet
WORKDIR /go/src/github.com/decred/dcrwallet
RUN go install .

# Build csppsolver — required for the local node to actively participate
# in CoinShuffle++ rather than wait for a peer with a solver. CGo + flint.
RUN CGO_ENABLED=1 go install decred.org/cspp/v2/cmd/csppsolver@v2.4.0

# Build dcrctl for wallet commands (from dcrctl repo)
WORKDIR /go/src/github.com/decred
RUN git clone --depth 1 https://github.com/decred/dcrctl.git || echo "dcrctl clone may have failed"

# Build dcrctl if repository was cloned successfully
WORKDIR /go/src/github.com/decred/dcrctl
RUN if [ -f "go.mod" ]; then go install .; else \
    echo '#!/bin/sh' > /go/bin/dcrctl && \
    echo 'echo "dcrctl not available"' >> /go/bin/dcrctl && \
    chmod +x /go/bin/dcrctl; fi

# Runtime stage — debian-slim with just the runtime libs csppsolver needs.
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata curl openssl expect netcat-openbsd \
    libflint17 libmpfr6 libgmp10 \
    && rm -rf /var/lib/apt/lists/*

# Copy binaries from builder
COPY --from=builder /go/bin/dcrwallet /usr/local/bin/
COPY --from=builder /go/bin/dcrctl /usr/local/bin/
COPY --from=builder /go/bin/csppsolver /usr/local/bin/

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

RUN mkdir -p /app-data && chown 1000:1000 /app-data

# Switch to UID 1000 for Umbrel compatibility
USER 1000
WORKDIR /app

# Expose ports
# 9110 - Mainnet RPC
# 9111 - Mainnet gRPC
# 19110 - Testnet RPC
# 19111 - Testnet gRPC
EXPOSE 9110 9111 19110 19111

# Use entrypoint script
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
