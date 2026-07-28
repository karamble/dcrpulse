# The gaming sandbox.
#
# Games are the one untrusted component in this stack, so this image carries as
# little as possible: a portal that supervises game binaries, and nothing else.
# There is no shell, no package manager and no CA bundle, because a game that
# got out of its process should find nothing useful around it.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY portal/ ./portal/

# The public key game binaries must be signed with. A build that does not set
# it produces a portal that installs nothing at all - which is the safe
# failure, because the alternative is a portal that installs whatever it is
# handed. Release builds pass the real key:
#
#   docker build --build-arg GAMING_SIGNING_KEY=<64 hex chars> ./gaming
ARG GAMING_SIGNING_KEY=""
RUN cd portal && CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.signingKeyHex=${GAMING_SIGNING_KEY}" -o /portal .

RUN mkdir -p /mnt/control /mnt/data

FROM scratch
COPY --from=build /portal /portal
COPY --from=build --chown=1000:1000 /mnt/control /control
COPY --from=build --chown=1000:1000 /mnt/data /data
# Games run as this uid. It owns /data and can read /control; it has no access
# to anything else, because nothing else is mounted.
USER 1000:1000
ENTRYPOINT ["/portal"]
