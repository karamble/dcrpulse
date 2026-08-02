# Shared Wallets

Shared wallets are m-of-n P2SH multisig wallets whose cosigners coordinate
over Bison Relay private messages. Invitations, key exchange and payment
approvals all travel over the encrypted connection you already have with
your contacts; no server, no email, no files to pass around.

## Using them

The Shared Wallets entry in the wallet sidebar lists your multisig
wallets. Creating one takes three steps: name it and choose how many
signatures each payment needs, pick the Bison Relay contacts who hold the
other keys, then review and send the invitations. Each contact sees an
invitation in their own dcrpulse and contributes a key by accepting. The
shared address appears only once everyone has confirmed; do not send funds
before then, because the address does not exist until the full key set is
known.

Paying out works the same way in reverse: fill in the destination and
amount, choose which cosigners to ask and in what order, and sign. The
payment travels to each cosigner in turn, and once enough approvals are
collected your dcrpulse broadcasts it automatically. Cosigners see the
decoded transaction, verified against their own wallet, before approving.

Download the backup card for each shared wallet and keep it with your
seed backup. The card holds the roster and the script; without it a
restored wallet cannot rebuild the shared address, because the script
lives outside the seed.

The rest of this document specifies the message format, the two protocols
carried over it, and the guarantees each side enforces. It is normative
for interoperating implementations.

## Envelope

One coordination message occupies one whole private message body:

```
--msig[v=1,mid=<hex>,exp=<unixsecs>]--<base64(payload)>
```

Rules:

- The body must match the envelope exactly, from first character to last.
  A message that merely contains an envelope is chat, not protocol.
- `v` is the envelope version. A receiver that does not implement the
  version ignores the frame. Unknown `k=v` header keys are tolerated so
  later versions can add fields without breaking older nodes.
- `mid` is 1 to 32 lowercase hex characters, unique per frame. It is the
  idempotency key: a receiver that has already processed a mid ignores
  the repeat. Implementations emit 16 hex characters.
- `exp` is the unix second after which the frame is void. Receivers drop
  expired frames without journaling them, allowing 300 seconds of clock
  skew. Bison Relay queues undelivered messages for about seven days, so
  a frame can legitimately arrive long after it was sent.
- The payload is base64 of a JSON object. The alphabet is restricted to
  base64 characters plus whitespace so a payload can never smuggle a
  second envelope.
- Frames are single part. Payloads are capped at 256 KiB, far below the
  1 MiB private message floor.

brclientd recognizes envelopes, keeps them out of chat history and
notification badges, refuses content-filter rules that would match them,
and serves them back for replay at `GET /msig/history`. It does not parse
payloads: the protocol lives entirely in the dashboard.

## Message types

Every payload carries `type`. Handshake messages carry `tempId`, which is
the mid of the invite that opened the round. Messages after activation
carry `walletId`, the shared P2SH address. Receivers ignore unknown
fields, and ignore unknown types after journaling their mid.

| Type | From | Fields | TTL |
|---|---|---|---|
| `invite` | initiator | tempId, label, m, n, network, pubkey | 7 d |
| `accept` | cosigner | tempId, pubkey | 7 d |
| `decline` | cosigner | tempId, reason | 7 d |
| `roster` | initiator | tempId, label, m, n, network, pubkeys, scriptHex, address | 7 d |
| `ready` | cosigner | tempId, walletId | 7 d |
| `invite_cancel` | initiator | tempId | 7 d |
| `sign_req` | proposer | walletId, txid, rawTx, note, sigsHave | 24 h default |
| `sig` | cosigner | walletId, txid, rawTx | 7 d |
| `sig_decline` | cosigner | walletId, txid, reason | 7 d |
| `broadcast` | proposer | walletId, txid | 7 d |

Public keys are 33-byte compressed secp256k1 keys in lowercase hex.
Amounts, where present, are in atoms. Handshake lifetimes match the relay
server's queueing horizon so "expired" and "undeliverable" coincide. Only
`sign_req` is deliberately short, so a stale approval prompt dies once the
proposer has moved on.

## Handshake

The initiator is the hub. Cosigners need a Bison Relay connection to the
initiator only, not to each other.

1. The initiator derives one key, records the round and sends `invite` to
   each participant.
2. A participant answers `accept` with its own freshly derived key, or
   `decline`. Keys are derived only on acceptance, so declining costs
   nothing.
3. Once every participant has accepted, the initiator sorts the full key
   set lexicographically, builds the `OP_m ... OP_n OP_CHECKMULTISIG`
   redeem script, derives its P2SH address, imports the script locally
   and sends `roster` to everyone.
4. Each cosigner independently rebuilds the script from the sorted key
   set, requires byte equality with the received `scriptHex`, requires
   the address to match, and requires its own key to be present. Any
   mismatch fails the round permanently. On success it imports the script
   and answers `ready`.
5. The wallet is active for a cosigner once it has imported; for the
   initiator once every `ready` has arrived.

Lexicographic key sorting is the dcrpulse convention. Decred has no
BIP67 equivalent, so implementations that want to interoperate must sort
the same way or they will derive different addresses from the same keys.

A decline, a cancel or an expiry fails the round. Restarting means a new
round with a new tempId; keys are never reused across rounds.

## Spending

Any member may propose. The proposer is the hub for that payment.

1. The proposer selects unspent outputs of the shared address, builds the
   transaction with change returning to the shared address, signs with its
   own key and records the payment keyed by the transaction id.
2. `sign_req` goes to the first cosigner in a user-chosen queue that must
   hold at least m-1 entries.
3. The receiving cosigner verifies independently and never trusts the
   sender's summary: the transaction id must match the transaction, every
   input must be a current unspent output of the shared address, existing
   signatures must verify against roster keys, change must return to the
   shared address, and the fee must be positive and within ten times the
   relay floor. Failure produces an immediate `sig_decline` carrying the
   reason, so the proposer's relay advances at once.
4. An approving cosigner signs and returns `sig`.
5. The proposer verifies the returned transaction: identical transaction
   id, a strictly higher count of valid signatures, every signature
   attributable to a roster key. It then relays to the next cosigner
   automatically, with no human action and no unlock at the hub.
6. At m signatures the proposer broadcasts and sends `broadcast` to every
   member, including those never asked to sign.

Because Decred computes the transaction id over the prefix only, the id
is fixed at construction and unchanged by signing. A single id comparison
therefore proves inputs, outputs, lock time and expiry are untouched;
only signature scripts may differ between hops.

### Timeouts, conflicts and races

- Each hop carries a deadline. When it passes, the proposer marks the hop
  and moves to the next candidate. A queue that runs out fails the
  payment; the proposer can then re-route to other cosigners.
- A proposer will not build two payments spending the same output.
  Cosigners auto-decline requests that collide with a payment already
  live locally.
- Every node marks a payment superseded when its inputs leave the unspent
  set under a different transaction id, and confirmed when its own
  broadcast is the spender. Cross-member races resolve on chain: only one
  transaction reaching m signatures can spend a given output.

## Durability

- Frames are written to an outbox before their first send and resent
  byte-identically until the relay accepts them, so a crash between a
  state change and a send never loses a message. Receiver mid journals
  make the resends harmless.
- Live delivery rides a notification stream that drops events for slow
  consumers, so it is a hint only. The reliable path is replay from
  `GET /msig/history`, which the dashboard runs at startup, after every
  wallet switch, on a periodic sweep and on manual refresh.
- Each dcrpulse wallet has its own Bison Relay identity, so shared wallet
  membership is per wallet. Frames addressed to a record whose wallet is
  not active are persisted and surfaced as "switch to that wallet"; steps
  that need wallet keys resume automatically after the switch.

## Registry and backup

Records persist per wallet in `msig.json` beside the wallet's config: the
roster, the redeem script, the shared address, this wallet's own key with
its derivation coordinates, the creation height, peer states, payments and
the mid journal.

The wallet database holds the imported script but cannot enumerate
scripts, name cosigners or survive a seed restore, which is why the
registry exists. A backup card exports one record without device-local
data. Restoring it into a wallet created from the same seed advances the
address cursor, proves the recorded key belongs to that seed, and
re-imports the script with a rescan bounded by the creation height.

## Trust model

Bison Relay authenticates peers, so the identity carries no keys of its
own: a frame is accepted only from the round's initiator, from a member of
the named wallet, or from the cosigner currently holding the baton, as the
message type requires. Nothing in a frame is trusted beyond that. Scripts,
addresses, amounts, fees and signatures are all recomputed locally, and a
transaction is only ever signed after it has been verified against this
node's own view of the chain.
