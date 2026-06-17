# Timestamping (dcrtime)

The **Timestamp** feature lets you prove that a file existed at a point in time by anchoring its cryptographic fingerprint to the Decred blockchain. It uses the public **dcrtime** service, hashes every file locally in your browser, and verifies anchors end to end through this dashboard's own dcrd node.

## Overview

A timestamp answers a simple question: "did this exact file exist no later than this date?" dcrtime collects file digests from many users, batches them into a Merkle tree, and writes the tree's root into a Decred transaction roughly once per hour. Once that transaction is mined, the file's digest is permanently anchored: anyone holding the file and the proof can show it existed before the block was created, without trusting dcrpulse or even dcrtime.

What a timestamp proves:

- **Existence**: the file (byte-for-byte) existed at or before the anchored block's time.
- **Integrity**: any later change to the file produces a different digest, so the proof no longer matches.

What a timestamp does NOT do:

- It does not prove **who** created the file or who owns it.
- It does not prove the file is **original**, **true**, or first.
- It does not reveal or store the file itself. Only the 32-byte SHA-256 digest is ever sent off your device.

**Access**: Open the **Wallet** section and click the **"Timestamp"** tab. A standalone verifier is also available from the **Explorer** landing page as the **"Verify Timestamp"** tool.

---

## Privacy: the file never leaves your device

dcrpulse hashes your file entirely in the browser using a Web Worker (SHA-256). The file is never uploaded, never written to disk by the dashboard, and never sent to dcrtime or the Decred network. Only the resulting 64-character hex digest is transmitted.

Because the digest is one-way, dcrtime (and anyone watching the chain) learns only that *some* file with that fingerprint was timestamped. It cannot reconstruct the file's contents, name, or size from the digest.

dcrtime requests are governed by a single toggle under **Settings -> Privacy** (the "dcrtime" external-request switch). When it is off, submitting and verifying are disabled and the Timestamp tab shows a notice. The toggle defaults to on.

---

## The Timestamp Tab

The tab header shows current feature status: the active network (mainnet/testnet), a reachability dot for dcrtime, a count of digests still awaiting an anchor, and an estimate of the time until the next hourly anchor. Below the header are three sub-tabs:

- **Stamp** - hash a file and submit it for timestamping.
- **Library** - browse, search, and manage your archived timestamps.
- **Verify** - check any file or digest against dcrtime and the chain.

---

### Stamp

Creating a timestamp is a guided, three-stage flow.

#### 1. Choose a file

Drop a file onto the upload area, or click to pick one. The dashboard immediately begins hashing it locally, showing a progress bar. When hashing finishes, the SHA-256 digest is displayed and can be copied.

#### 2. Add optional metadata

Before submitting you can attach descriptive metadata, all of which is stored **only in your local archive** (never sent to dcrtime):

- **Title** - a friendly name (for example, "Signed supplier contract").
- **Description** - free-form notes.
- **Tags** - comma-separated labels for filtering later.

The dashboard also records the file name, size, MIME type, and last-modified time alongside the digest, again purely for your own reference.

#### 3. Submit the digest

Click **"Timestamp this file"**. The dashboard:

1. Saves a record locally with status **Submitted**.
2. Posts the digest to dcrtime.
3. Immediately performs one verification so that a digest someone already anchored reflects its real anchor right away.

A staged progress trace on the right shows each step resolving:

- **Hash the file in your browser** - the file never leaves your device.
- **Submit the digest to dcrtime** - accepted into the next anchor batch.
- **Anchor to the Decred blockchain** - completed hourly; the Library updates automatically.

If the same digest is already in your archive, the dashboard tells you (HTTP 409) and shows the existing record instead of creating a duplicate. dcrtime returning "Exists" (the digest was already submitted, by anyone) is treated as a normal, successful outcome, not an error.

---

### Record States and Lifecycle

Every archived timestamp has one of five statuses. The four anchor-related states mirror dcrtime's own anchoring stages.

| Status | Badge label | Meaning |
| --- | --- | --- |
| `submitted` | Submitted | Accepted by dcrtime; the anchor stage is not yet known. |
| `awaiting` | Awaiting anchor | Queued for the next hourly anchor; not yet placed in a transaction. |
| `pending` | Confirming | Placed in an anchor transaction that is not yet confirmed on-chain. |
| `anchored` | Anchored | Committed to the Decred blockchain. This is the terminal, proven state. |
| `failed` | Failed | The submission errored. The digest is safe locally and can be retried. |

#### How records advance

dcrtime flushes pending digests into an anchor transaction **hourly, on the hour**. A record therefore progresses roughly like this:

1. **Submitted / Awaiting anchor** right after you stamp it.
2. **Confirming** once dcrtime includes the digest in an anchor transaction (the transaction id becomes known, but it is not yet mined).
3. **Anchored** once that transaction is mined and dcrtime reports a chain timestamp.

You do not need to keep the page open. A background worker in the dashboard polls dcrtime every 5 minutes and advances any not-yet-anchored records automatically, filling in the Merkle proof, anchor transaction id, anchor time, and confirmation count as they become available. A **Refresh** button in the Library (and the tab's manual refresh) triggers an immediate poll on demand.

A digest that dcrtime has not registered yet (brief propagation delay just after submission) is left untouched so the next poll retries it.

---

### Library

The Library lists every timestamp in your archive as a table: file (title or name, plus the short digest), status badge, submitted date, and anchored date.

#### Filtering and sorting

- **Search** - case-insensitive substring match across file name, title, and description.
- **Status filter** - All, Anchored, Confirming, Awaiting anchor, Submitted, or Failed.
- **Sort** - Newest (default), Oldest, or Title.

#### Refresh and export

- **Refresh** - asks dcrtime about every not-yet-anchored digest and updates the table, so you can pull newly committed proofs without waiting for the background poll.
- **Export** - downloads your entire archive as a single JSON file (`dcrpulse-timestamps.json`), including every record and its proof.

Click any row to open the record detail view.

---

### Record Detail

Opening a record shows everything stored about it and the actions available for it.

#### Metadata

- Digest (SHA-256), with copy.
- File name, description, tags.
- Size and MIME type.
- Submitted time, and a failure reason if the submission errored.

You can **Edit** the title, description, and tags at any time (this only changes your local metadata, never the anchored digest).

#### Anchor proof

Once a record is anchored (or at least placed in a transaction), this section shows the cryptographic proof:

- **Anchored** time.
- **Merkle root**, with copy.
- **Anchor tx** - links to the transaction in the built-in Explorer.
- **Confirmations** - current versus the minimum dcrtime requires.
- **Merkle path** - the verbatim dcrtime proof structure, expandable for inspection.

#### On-chain validation

Click **Validate** to re-check the proof against the Decred chain using this dashboard's own dcrd node (no external block explorer). The result is shown as a checklist (see [Verifying a File](#verifying-a-file) for what each step means).

#### Actions

- **Download proof** (anchored records only) - exports a self-contained proof JSON for this single record.
- **Re-verify** - asks dcrtime for the latest status and refreshes the record.
- **Retry submit** (failed records only) - resubmits the digest to dcrtime.
- **Edit** - change local metadata.
- **Delete** - removes the record and its stored proof from your archive (with a confirm step).

---

### Verify

The Verify sub-tab checks whether a given file or digest is timestamped, independently of your local archive. The same view powers the standalone **Verify Timestamp** tool on the Explorer landing page.

#### How to verify

- **Drop or pick a file** - it is hashed locally, then checked. The file is never uploaded.
- **Paste a digest** - enter a 64-character hex SHA-256 digest and click **Verify**.

#### What is checked

The result is a step-by-step trace. For a fully anchored file you will see:

1. **Found in your local archive** (or "Not stored locally" - verification does not depend on local state).
2. **dcrtime recognizes this digest** - dcrtime has a record of it.
3. **Merkle proof path is valid** - the proof's authentication path resolves to a root.
4. **Your file is included in the timestamp** - your digest is one of the leaves in that path.
5. **Merkle root matches the proof** - the root computed from the path equals the claimed root.
6. **Committed on the Decred blockchain** - the root is present in the anchor transaction's `OP_RETURN` output, confirmed via dcrd. The block height, anchor date, and confirmation count are shown, with a link to the anchor transaction.

If the digest is recognized but not yet anchored, the trace shows it is **awaiting** the next hourly anchor or **pending** confirmations instead. If dcrtime has never seen the digest, verification stops at step 2.

When all the cryptographic and on-chain checks pass, a green **Verified** banner confirms that this exact file was timestamped on the Decred chain, with the anchor date.

Crucially, the on-chain step does not trust dcrtime: the Merkle path must resolve to the claimed root, your digest must be a leaf in it, and that root must actually appear in the referenced Decred transaction on the chain your dcrd node is following.

---

## Exporting and Sharing Proofs

There are two export options, both producing self-contained JSON:

- **Single proof** - from the record detail view (**Download proof**) or via the API. The file is named `timestamp-proof-<short-digest>.json` and contains the digest, file name, title, Merkle root, the verbatim Merkle path, the anchor transaction id, anchor timestamp, the chain label (for example `decred-mainnet`), and the dcrtime server it was anchored against.
- **Full archive** - from the Library (**Export**), producing `dcrpulse-timestamps.json` with every record and proof.

A proof is designed to be verifiable by a third party who has the original file: they can hash the file, walk the Merkle path to the root, and confirm the root is committed in the referenced Decred transaction. Only **anchored** records can be exported as a single proof; a record that is not anchored yet returns HTTP 409.

---

## API Routes

All routes are under `/api`. Digests are lowercase 64-character hex SHA-256 strings.

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/timestamp/records` | List archive (query params: `q`, `status`, `tag`, `sort`). |
| POST | `/timestamp/records` | Create a record and submit its digest. |
| GET | `/timestamp/records/{digest}` | Fetch one record. |
| PATCH | `/timestamp/records/{digest}` | Edit title/description/tags. |
| DELETE | `/timestamp/records/{digest}` | Delete a record and its proof. |
| POST | `/timestamp/records/{digest}/retry` | Resubmit a failed record. |
| GET | `/timestamp/records/{digest}/proof` | Download the single-record proof JSON (anchored only). |
| POST | `/timestamp/verify` | Check a digest against the archive, dcrtime, and the chain. |
| POST | `/timestamp/validate` | Validate a proof on-chain via dcrd (body proof or archive lookup). |
| POST | `/timestamp/refresh` | Trigger an immediate anchor poll, then return the archive. |
| GET | `/timestamp/status` | Feature health (add `?ping=1` to probe dcrtime reachability). |
| GET | `/timestamp/export` | Download the whole archive as JSON. |

The archive is stored on the server, globally (not per wallet), because proofs are about files rather than wallet keys and must survive wallet switches.

---

## How It Works (Technical)

### Hashing

Files are hashed with SHA-256 in a browser Web Worker, so large files do not freeze the UI and the work stays confined to the worker. Only the hex digest is returned to the page and posted to the backend.

### dcrtime API

The dashboard talks to the public dcrtime v2 JSON API:

- **mainnet**: `https://time.decred.org:49152`
- **testnet**: `https://time-testnet.decred.org:59152`

(Note: `timestamp.decred.org` is the human-facing "Timestamply" web UI; the JSON API that dcrpulse and the dcrtime CLI use lives at `time.decred.org`.) Requests follow the dashboard's shared, Tor-aware outbound transport, the same routing as every other external request. Submit and verify are idempotent and retried with backoff on transient failures, so a retry never duplicates work.

A stable, random per-install client id is generated and persisted on first use, and sent with dcrtime calls.

### On-chain validation

Anchor validation is performed entirely through dcrpulse's own dcrd connection. The dashboard fetches the anchor transaction, confirms the Merkle root computed from the proof appears in the transaction's `OP_RETURN` (`nulldata`) output, and reports the block height, block time, and confirmations. No external block explorer is consulted.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - the Wallet section the Timestamp tab lives in
- **[API Reference](../api/api-reference.md)** - full API endpoint reference

---

## Tips & Best Practices

1. **Keep your original files unchanged.** A timestamp is tied to the exact bytes. Re-saving, re-encoding, or editing a file changes its digest and breaks the match. To prove a new version, timestamp it separately.
2. **Export and back up your proofs.** The anchor lives on-chain forever, but the Merkle path needed to verify it lives in your archive (or an exported proof). Keep a copy of the proof alongside the file.
3. **You do not have to wait.** Submit and walk away; the background poller advances records to Anchored on its own.
4. **Anchoring is hourly.** Expect up to an hour (plus a few confirmations) before a fresh timestamp reaches the Anchored state.
5. **The same digest is shared.** If two people timestamp the identical file, dcrtime returns "Exists" for the later one - both still have a valid proof to the same anchor.

---

## Troubleshooting

### "dcrtime requests are disabled"

**Problem**: A banner says timestamping is turned off.

**Solution**: Enable the dcrtime external-request toggle under **Settings -> Privacy**.

### dcrtime shows as unreachable

**Problem**: The status dot is red / "dcrtime unreachable".

**Solutions**:
1. Check your internet (or Tor) connectivity.
2. Confirm the active network matches the dcrtime host (mainnet vs testnet).
3. Retry - transient failures are retried automatically, and you can use the Library's Refresh.

### A record is stuck at "Submitted" or "Awaiting anchor"

**Problem**: A timestamp has not reached Anchored.

**Solutions**:
1. Wait for the next hourly anchor, then a few confirmations.
2. Click **Refresh** in the Library (or **Re-verify** in the record) to pull the latest status.
3. Remember the background poller checks every 5 minutes regardless.

### A submission failed

**Problem**: A record shows the **Failed** status.

**Solutions**:
1. Open the record and read the failure reason.
2. Click **Retry submit** - the digest is safe locally and resubmission is idempotent.

### Verification says "dcrtime has no record of this digest"

**Problem**: A file you expected to be timestamped is not recognized.

**Solutions**:
1. Confirm you are verifying the exact original file (any change alters the digest).
2. Confirm you are on the same network it was anchored on.
3. If you only just submitted it, allow a moment for dcrtime to register the digest, then re-verify.
