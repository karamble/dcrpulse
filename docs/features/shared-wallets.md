# Shared Wallets

Shared wallets are m-of-n multisig wallets whose cosigners coordinate
over Bison Relay private messages. Invitations, key exchange and payment
approvals all travel over the encrypted connection you already have with
your contacts; no server, no email, no files to pass around. A wallet
can instead opt into manual coordination at creation, where the same
messages are carried by the participants themselves — see the
transports section.

A shared wallet is not one address but a ladder of them: every
participant contributes the extended public key of a dedicated account,
and address i is the m-of-n script over everyone's key at index i. The
wallet hands out a fresh address per payer, sends change to addresses of
its own, and no address ever needs to be reused.

## Using them

The MultiSig Wallet entry in the wallet sidebar lists your shared
wallets. Creating one takes three steps: name it, choose how many
signatures each payment needs and how the wallet coordinates, pick the
Bison Relay contacts who hold the other keys (or name them, on a
manually coordinated wallet), then review and send the invitations.
Creating or accepting asks for your wallet passphrase once: it creates
a dedicated account in your wallet, and only that account's extended
public key is shared with your cosigners — your other accounts stay
private. Declining an invitation costs nothing and shares nothing.

On a manually coordinated wallet the invitations do not travel by
themselves: the wallet's Coordination card lists every message waiting
to be handed over, and everything cosigners hand back is imported
there. An invitation received out of band is imported from the shared
wallets page.

Receive addresses appear once every cosigner has confirmed. The Receive
card shows the current address and mints the next one on demand; a fresh
address per payer keeps deposits apart. The wallet refuses to run more
than a full window of unpaid addresses ahead (see the gap rules below) —
new ones unlock as earlier ones receive funds.

Paying out works the same way as before: fill in the destination and
amount (or sweep everything with "Send all"), choose which cosigners to
ask and in what order, and sign. The payment travels to each cosigner in
turn, and once enough approvals are collected your dcrpulse broadcasts
it automatically. Cosigners see the decoded transaction, verified
against their own wallet, before approving; change back to the wallet is
recognized and labeled by each cosigner independently.

Download the backup card for each shared wallet and keep it with your
seed backup. The card holds the extended-key roster and the wallet's
coordinates; without it a restored wallet cannot rebuild the ladder,
because the roster lives outside the seed.

The rest of this document specifies the message format, the derivation,
the two protocols carried over it, and the guarantees each side
enforces. It is normative for interoperating implementations.

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

## Derivation

The derivation scheme is named `dcrpulse-msig-hd-1` and consists of, in
full:

- Each participant's contribution is a dcrwallet ACCOUNT extended public
  key (the dedicated account created for the round).
- The child key of participant p at branch b, index i is
  `xpub_p / b / i` using dcrd hdkeychain's `Child` derivation — the same
  calls dcrwallet's address manager makes, so the wallet can always sign
  for the derived keys once its branch index covers them.
- Branch 0 receives; branch 1 takes change.
- Address (b, i) is the P2SH of `OP_m <keys...> OP_n OP_CHECKMULTISIG`
  over the LEXICOGRAPHICALLY SORTED serialized compressed child keys at
  (b, i). Sorting is per index: the key order may differ between
  indices, and the roster's canonical xpub order (ascending strings,
  used for identity and duplicate rejection on the wire) is independent
  of any index's key order.
- Index i on branch b is SKIPPED if and only if child derivation fails
  for any roster xpub (hdkeychain's invalid-child case, odds ~2^-127).
  Every participant computes over the identical xpub set, so skipping is
  deterministic; cursors count raw indices, holes included.
- The wallet's identity — `walletId` on the wire and the record's
  address — is the address at the smallest non-skipped external index.

Anything that changes any of the above is a NEW scheme with a new name,
never an amendment: deriving a card's ladder any other way rebuilds the
wrong wallet. Backup cards record the scheme name and restores refuse
schemes they do not implement.

### Gap windows

`GapExt = 20` (external) and `GapInt = 10` (internal) are protocol
constants. Every member keeps at least `[0, maxKnownUsedIndex + Gap)` of
each branch imported into its wallet (dcrwallet has no account notion
for multisig scripts, so each derived script is individually imported
and watched), and syncs its own account's branch indices at least as
far, so any imported index is also signable.

No member hands out an index at or beyond `localLastUsed + Gap` — the
receive UI surfaces the refusal. This bound is what keeps every member's
imported window covering every address any member can legally disclose.
Two members handing out addresses concurrently may mint the same index;
that is only address reuse between two payers, never a loss of funds.

Imports come in two classes. Pre-use imports (the activation batch and
the top-ups that precede handing out a receive or change address) happen
before the address is ever revealed, so they need no rescan. Imports
forced by OBSERVATION — chain usage at the window's edge caused by a
peer, a restore, or a payment request naming an index this node has not
imported yet — may postdate the funding they need to see, so they are
followed by one deferred wallet rescan bounded by the wallet's creation
height.

## Message types

Every payload carries `type`, and handshake messages carry `ver: 2`.
Handshake messages carry `tempId`, which is the mid of the invite that
opened the round. Messages after activation carry `walletId`, the
external index-0 address. Receivers ignore unknown fields, and ignore
unknown types after journaling their mid.

| Type | From | Fields | TTL |
|---|---|---|---|
| `invite` | initiator | ver, tempId, label, m, n, network, xpub | 7 d |
| `accept` | cosigner | ver, tempId, xpub | 7 d |
| `decline` | cosigner | tempId, reason | 7 d |
| `roster` | initiator | ver, tempId, label, m, n, network, xpubs, address | 7 d |
| `ready` | cosigner | tempId, walletId | 7 d |
| `invite_cancel` | initiator | tempId | 7 d |
| `sign_req` | proposer | walletId, txid, rawTx, note, sigsHave | 24 h default |
| `sig` | cosigner | walletId, txid, rawTx | 7 d |
| `sig_decline` | cosigner | walletId, txid, reason | 7 d |
| `broadcast` | proposer | walletId, txid | 7 d |

Extended public keys travel in their standard base58 encoding and must
be public-only. The roster's `xpubs` list is canonical: exactly n keys
in strictly ascending string order, which doubles as duplicate
rejection. Schemes are capped at 8 participants — the network itself
allows more, but the serial signing relay becomes impractical first.
A handshake frame carrying both the historical single-key
fields and the extended-key fields is invalid — receivers must reject
it rather than guess, so one frame can never mean different wallets to
different builds. Amounts, where present, are in atoms. Handshake
lifetimes match the relay server's queueing horizon; only `sign_req` is
deliberately short, so a stale approval prompt dies once the proposer
has moved on.

## Handshake

The initiator is the hub. Cosigners need a Bison Relay connection to the
initiator only, not to each other.

1. The initiator creates its dedicated account, records the round and
   sends `invite` with the account's xpub to each participant.
2. A participant answers `accept` with its own freshly created account's
   xpub, or `decline`. The account is created only on acceptance, so
   declining costs nothing. An xpub already present in the round fails
   it: two participants on one key would collapse the threshold.
3. Once every participant has accepted, the initiator sorts the xpub
   set, derives the wallet id, imports the initial gap windows of both
   branches, syncs its own branch indices and sends `roster` to
   everyone.
4. Each cosigner independently verifies the roster: the m, n, network
   and label must match the invite; its own xpub and the initiator's
   must be present; and the wallet id must re-derive byte-identically
   from the xpub set. Any mismatch fails the round permanently. On
   success it imports its own windows and answers `ready`.
5. The wallet is active for a cosigner once it has imported; for the
   initiator once every `ready` has arrived.

A decline, a cancel or an expiry fails the round. Restarting means a new
round with a new tempId and a new dedicated account; accounts are never
reused across rounds. dcrwallet accounts are permanent, so a failed
round leaves an empty account behind, and a wallet can hold at most on
the order of a hundred unfunded accounts.

## Spending

Any member may propose. The proposer is the hub for that payment.

1. The proposer selects unspent outputs across the imported ladder,
   builds the transaction with change paying a freshly allocated
   internal index (imported before the transaction exists), signs with
   its own account and records the payment keyed by the transaction id.
2. `sign_req` goes to the first cosigner in a user-chosen queue that must
   hold at least m-1 entries.
3. The receiving cosigner verifies independently and never trusts the
   sender's summary: the transaction id must match the transaction,
   every input must be a current unspent output of the ladder in this
   node's own view, existing signatures must verify per input against
   the keys at that input's index, and the fee must be positive and
   within ten times the relay floor. Outputs are classified, not
   vetoed: an output paying a derived internal address within the
   verifier's own window is labeled change; every other output is shown
   to the human as a recipient for approval. A request naming an index
   this node has not imported yet triggers one window top-up and
   deferred rescan before the request is declined. Failures produce an
   immediate `sig_decline` carrying the reason, so the proposer's relay
   advances at once.
4. An approving cosigner syncs its branch indices through the imported
   window (the wallet only signs with keys its address manager has
   seen), signs with its dedicated account and returns `sig`.
5. The proposer verifies the returned transaction: identical transaction
   id, and a strictly larger set of PARTICIPANTS whose signatures are
   present on every input — signatures attribute per input against that
   index's keys, and a participant only counts once it has signed all of
   them. It then relays to the next cosigner automatically, with no
   human action and no unlock at the hub.
6. At m participants the proposer broadcasts and sends `broadcast` to
   every member, including those never asked to sign. The notice is not
   trusted: a member's record only turns broadcast once its own wallet
   has seen the transaction, and the periodic sweep settles records to
   confirmed or superseded from the local chain view alone.

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
  set under a different transaction id, and confirmed once its own
  wallet sees the payment's transaction mined. Cross-member races
  resolve on chain: only one transaction reaching m signatures can spend
  a given output.

## Durability

- Frames are written to an outbox before their first send and resent
  byte-identically until the relay accepts them, so a crash between a
  state change and a send never loses a message. Receiver mid journals
  make the resends harmless.
- Live delivery rides a notification stream that is a hint only. The
  reliable path is replay from `GET /msig/history`, which the dashboard
  runs at startup, after every wallet switch, on a periodic sweep and on
  manual refresh.
- Each dcrpulse wallet has its own Bison Relay identity, so shared wallet
  membership is per wallet. Frames addressed to a record whose wallet is
  not active are persisted and surfaced as "switch to that wallet";
  steps that need wallet keys — activation, ladder imports, window
  top-ups — resume automatically after the switch.

## Registry and backup

Records persist per wallet in `msig.json` beside the wallet's config:
the xpub roster, this wallet's dedicated account number and xpub, the
per-branch cursors (next index to hand out, imported-through, last used
on chain), the creation height, peer states, payments and the mid
journal. A build refuses to open an `msig.json` written by a newer
schema: saving would silently strip the newer fields. Downgrading past
this version therefore destroys ladder state — keep backup cards.

The wallet database holds the imported scripts but cannot enumerate
them, name cosigners or survive a seed restore, which is why the
registry exists. A backup card exports one record without device-local
data, stamped with its `cardVersion` and `derivationScheme`. Restoring
it proves ownership by extended-key equality — the same seed always
derives the same account xpub — locating the account by scanning every
account of the wallet, which survives renames and renumbering. Only when
no account matches (a fresh wallet from the same seed) are accounts
recreated sequentially up to the card's number, which requires the
wallet passphrase; an xpub mismatch after recreation means the wrong
seed and fails the restore. The ladder windows are then re-imported and
one deferred rescan bounded by the recorded creation height recovers the
history. Cursors on a stale card only widen the initial window; later
usage is rediscovered from the chain.

### Recovering without dcrpulse

The card plus the seed are sufficient to recover funds with generic
tools; the seed alone recovers NOTHING, because the roster's xpubs exist
only in the card. The procedure is the derivation section verbatim:
recreate the account (its number is on the card), verify its xpub
matches the card's, derive every participant's child key per index,
build the sorted multisig script, and import or sign for the addresses
that carry funds. Any implementation that follows the named scheme
derives the identical wallet.

## Coordination transports

Each wallet chooses at creation how its frames travel, and both
transports speak the identical wire format.

- **Bison Relay** (default): frames are sent as direct messages between
  KX'd contacts, delivery and replay are automatic, and the relay's
  end-to-end encryption authenticates every sender.
- **Manual**: no frame is ever sent anywhere. Outbound frames wait on
  the wallet's Coordination card as per-cosigner hand-over items —
  copy, file download or QR for small frames — and inbound frames are
  pasted or opened there, attributed by the importer to the cosigner
  who handed them over. Cosigners are local labels with locally minted
  pseudo-identities; the wire carries no identities, so each
  participant's table is private and never needs to match anyone
  else's.

Manual frames are minted with a 30-day envelope lifetime, and exporting
a frame that has passed half of it re-wraps the stored payload with a
fresh expiry (same message id, so duplicates stay harmless). Signing
requests carry no per-cosigner deadlines — a courier cannot be timed
out — so a stalled request is resolved by the humans: decline it,
abort it, or just hand it over again. Rounds that never complete are
failed after 31 days. Hand-over items retire themselves once the
protocol state proves the counterpart acted on them.

## Trust model

On Bison Relay the transport authenticates peers, so the identity
carries no keys of its own: a frame is accepted only from the round's
initiator, from a member of the named wallet, or from the cosigner
currently holding the baton, as the message type requires. On the
manual transport that sender identity comes from the hand-over itself —
whoever gave you the blob is who you attribute it to — while everything
that guards funds stays cryptographic and is verified identically on
both transports. Nothing in a frame is trusted beyond that.
Rosters, wallet ids, scripts, addresses, amounts, fees and signatures
are all recomputed locally — at every index — and a transaction is only
ever signed after it has been verified against this node's own view of
the chain. Broadcast notices are courtesy signals; the local chain view
stays authoritative.

Sharing an account xpub lets every cosigner derive and watch ALL of the
shared wallet's addresses — that is what makes independent verification
possible — and nothing else: the dedicated account exists only for this
wallet, so no personal history is exposed. An invitation necessarily
reveals the initiator's xpub to every invitee, including ones who
decline; the dedicated account contains that disclosure.
