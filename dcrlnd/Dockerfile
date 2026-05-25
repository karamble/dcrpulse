# Dockerfile for building dcrlnd from source.
# Pattern matches dcrd / dcrwallet containers in this repo.
#
# dcrlnd needs cgo for its DNS resolver inside linked-container networks;
# upstream's own Dockerfile (decred/dcrlnd:Dockerfile) sets GODEBUG=netdns=cgo
# and uses alpine-sdk to get gcc available.

FROM golang:1.25-alpine AS builder

ENV GODEBUG=netdns=cgo

RUN apk add --no-cache git make gcc musl-dev

WORKDIR /go/src/github.com/decred

ARG DCRLND_VERSION=master
RUN git clone --depth 1 --branch ${DCRLND_VERSION} https://github.com/decred/dcrlnd.git

WORKDIR /go/src/github.com/decred/dcrlnd
RUN make && make install

# Runtime stage. Mirrors the dcrwallet runtime image layout (alpine + a
# handful of debug-friendly utilities). curl is for healthchecks; nc lets
# the entrypoint wait for dcrwallet's gRPC port to come up.
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl jq netcat-openbsd

COPY --from=builder /go/bin/dcrlnd /usr/local/bin/
COPY --from=builder /go/bin/dcrlncli /usr/local/bin/

COPY docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Match the UID used by dcrwallet so the shared /app-data/dcrlnd directory
# has consistent ownership across containers.
RUN adduser -D -u 1000 dcrpulse
RUN mkdir -p /app-data/dcrlnd && chown -R 1000:1000 /app-data
USER 1000
WORKDIR /app

# 9735 = LN p2p (disabled by --nolisten in our config, but documented),
# 10009 = gRPC the dashboard talks to.
EXPOSE 9735 10009

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
