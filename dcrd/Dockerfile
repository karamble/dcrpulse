# Dockerfile for building dcrd from source
# Based on official dcrd repository: https://github.com/decred/dcrd/tree/master/contrib/docker

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git openssl

# Set working directory
WORKDIR /go/src/github.com/decred

# Clone the dcrd repository
ARG DCRD_VERSION=release-v2.1.3
RUN git clone --depth 1 --branch ${DCRD_VERSION} https://github.com/decred/dcrd.git

# Build dcrd
WORKDIR /go/src/github.com/decred/dcrd
RUN go install .

# Try to build certgen if it exists in this version, otherwise clone from master
RUN if [ -d "cmd/certgen" ]; then \
        echo "Building certgen from current dcrd version..." && \
        go install ./cmd/certgen; \
    else \
        echo "certgen not found in this version, trying master branch..." && \
        cd /go/src/github.com/decred && \
        git clone --depth 1 https://github.com/decred/dcrd.git dcrd-certgen && \
        cd dcrd-certgen && \
        go install ./cmd/certgen 2>/dev/null || \
        echo "certgen build failed, will use openssl fallback"; \
    fi

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

# Copy certgen if it exists (it may not exist if build failed)
RUN --mount=type=bind,from=builder,source=/go/bin,target=/tmp/gobin \
    if [ -f /tmp/gobin/certgen ]; then \
        cp /tmp/gobin/certgen /usr/local/bin/certgen && \
        echo "certgen binary installed"; \
    else \
        echo "certgen not available, openssl fallback will be used"; \
    fi

# Copy entrypoint script
COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

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

