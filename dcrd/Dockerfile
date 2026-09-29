# Dockerfile for building dcrd from source
# Based on official dcrd repository: https://github.com/decred/dcrd/tree/master/contrib/docker

# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git openssl

# Set working directory
WORKDIR /go/src/github.com/decred

# Clone the dcrd repository
ARG DCRD_VERSION=release-v2.1.6
RUN git clone --depth 1 --branch ${DCRD_VERSION} https://github.com/decred/dcrd.git

# Build dcrd
WORKDIR /go/src/github.com/decred/dcrd
RUN go install .

# Try to build dcrctl from separate repository (it's optional for healthchecks)
WORKDIR /go/src/github.com/decred
RUN git clone --depth 1 --branch ${DCRD_VERSION} https://github.com/decred/dcrctl.git 2>/dev/null || \
    git clone --depth 1 https://github.com/decred/dcrctl.git || \
    echo "dcrctl repository not available"

# Build dcrctl if repository was cloned successfully
WORKDIR /go/src/github.com/decred/dcrctl
RUN if [ -f "go.mod" ]; then go install .; else \
    echo '#!/bin/sh' > /go/bin/dcrctl && \
    echo 'echo "dcrctl not available"' >> /go/bin/dcrctl && \
    chmod +x /go/bin/dcrctl; fi

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl openssl

# Copy binaries from builder
COPY --from=builder /go/bin/dcrd /usr/local/bin/
COPY --from=builder /go/bin/dcrctl /usr/local/bin/

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

RUN mkdir -p /app-data && chown 1000:1000 /app-data

# Switch to UID 1000 for Umbrel compatibility
USER 1000
WORKDIR /app

# Expose ports
# 9108 - Mainnet P2P
# 9109 - Mainnet RPC
# 19108 - Testnet P2P
# 19109 - Testnet RPC
EXPOSE 9108 9109 19108 19109

# Use entrypoint script
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

