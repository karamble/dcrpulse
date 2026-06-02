ARG BISONW_VERSION=v1.0.6

FROM node:20-alpine AS nodebuilder
ARG BISONW_VERSION
RUN apk add --no-cache git
WORKDIR /root
RUN git clone --depth 1 --branch ${BISONW_VERSION} https://github.com/decred/dcrdex.git dex
WORKDIR /root/dex/client/webserver/site
RUN npm clean-install && npm run build

FROM golang:1.25-alpine AS gobuilder
COPY --from=nodebuilder /root/dex /root/dex
WORKDIR /root/dex/client/cmd/bisonw
RUN CGO_ENABLED=0 GOOS=linux go build
WORKDIR /root/dex/client/cmd/bwctl
RUN CGO_ENABLED=0 GOOS=linux go build

FROM debian:bookworm-slim
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /dex
ENV HOME=/dex
RUN mkdir -p /dex/.dexc && chown 1000 /dex/.dexc
COPY --from=gobuilder /root/dex/client/cmd/bisonw/bisonw ./
COPY --from=gobuilder /root/dex/client/cmd/bwctl/bwctl ./
COPY supervisor.sh /supervisor.sh
USER 1000
ENTRYPOINT ["/bin/sh", "/supervisor.sh"]
