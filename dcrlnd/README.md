# dcrpulse-dcrlnd

Container image for dcrlnd, the Decred fork of lnd, built from
upstream source at `github.com/decred/dcrlnd`. Mirrors the
Decrediton wiring documented in `app/main_dev/launch.js:846-957`:
dcrlnd connects to dcrwallet via mutual-TLS gRPC, not to dcrd
directly.

## Deferred-start model

dcrlnd needs a dcrwallet account number passed via
`--dcrwallet.accountnumber`. In Decrediton the user picks one through
a setup wizard before the daemon launches. Our equivalent: the
dashboard's Lightning setup wizard creates a `lightning` account in
dcrwallet, then writes the account number to
`/app-data/dcrlnd/.account`. This entrypoint polls for that file and
blocks until it exists. Until then the container sits idle.

## Inputs

Reads from the shared `/app-data` volume:

- `/app-data/dcrd/rpc.cert` — dcrwallet's TLS cert. Mutual-TLS auth
  on the dcrlnd ↔ dcrwallet hop uses this same cert as both server
  identity and client credential.
- `/app-data/dcrlnd/.account` — single-line file with the dcrwallet
  account number to bind. Written by the dashboard's
  `/api/wallet/ln/setup` handler.

## Outputs

Writes to `/app-data/dcrlnd/`:

- `tls.cert`, `tls.key` — auto-generated on first start, used by the
  dashboard to authenticate dcrlnd's gRPC.
- `admin.macaroon` — auto-generated, supplied as
  `grpc-metadata-macaroon` header on every dashboard call.
- `data/`, `logs/`, `dcrlnd.conf` — dcrlnd's own state directories.

## Environment variables

- `LN_TESTNET=true` — appends `--testnet` to the dcrlnd invocation.
- `DCRWALLET_HOST` — dcrwallet container hostname, defaults to
  `dcrwallet`.
- `DCRWALLET_GRPC_PORT` — defaults to `9111`.

## Build args

- `DCRLND_VERSION` — git ref to clone. Defaults to upstream master in
  the Dockerfile; the top-level compose pins this to a released tag.
