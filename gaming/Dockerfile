# The gaming sandbox.
#
# Games are the one untrusted component in this stack, so this image carries as
# little as possible: a portal that supervises game binaries, and nothing else.
# There is no shell, no package manager and no CA bundle, because a game that
# got out of its process should find nothing useful around it.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY portal/ ./portal/
RUN cd portal && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /portal .

FROM scratch
COPY --from=build /portal /portal
# Games run as this uid. It owns /data and can read /control; it has no access
# to anything else, because nothing else is mounted.
USER 1000:1000
ENTRYPOINT ["/portal"]
