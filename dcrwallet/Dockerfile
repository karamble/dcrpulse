# Dockerfile for building dcrwallet from source
# Based on dcrwallet repository: https://github.com/decred/dcrwallet
# Pattern follows dcrd setup for consistency

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git openssl

# Set working directory
WORKDIR /go/src/github.com/decred

# Clone the dcrwallet repository
ARG DCRWALLET_VERSION=master
RUN git clone --depth 1 --branch ${DCRWALLET_VERSION} https://github.com/decred/dcrwallet.git

# Build dcrwallet
WORKDIR /go/src/github.com/decred/dcrwallet
RUN go install .

# Build dcrctl for wallet commands (from dcrctl repo)
WORKDIR /go/src/github.com/decred
RUN git clone --depth 1 https://github.com/decred/dcrctl.git || echo "dcrctl clone may have failed"

# Build dcrctl if repository was cloned successfully
WORKDIR /go/src/github.com/decred/dcrctl
RUN if [ -f "go.mod" ]; then go install .; else \
    echo '#!/bin/sh' > /go/bin/dcrctl && \
    echo 'echo "dcrctl not available"' >> /go/bin/dcrctl && \
    chmod +x /go/bin/dcrctl; fi

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl openssl expect

# Copy binaries from builder
COPY --from=builder /go/bin/dcrwallet /usr/local/bin/
COPY --from=builder /go/bin/dcrctl /usr/local/bin/

# Create user with UID 1000 for Umbrel compatibility
# Umbrel requires UID 1000, but username can be anything
RUN addgroup -g 1000 dcrpulse && adduser -u 1000 -G dcrpulse -s /bin/sh -D dcrpulse

# Create data directory structure
RUN mkdir -p /app-data/dcrwallet && \
    chown -R 1000:1000 /app-data

# Copy entrypoint script
COPY --chown=1000:1000 docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Switch to dcrpulse user (UID 1000)
USER 1000:1000
WORKDIR /home/dcrpulse

# Expose ports
# 9110 - Mainnet RPC
# 9111 - Mainnet gRPC
# 19110 - Testnet RPC
# 19111 - Testnet gRPC
EXPOSE 9110 9111 19110 19111

# Use entrypoint script
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

