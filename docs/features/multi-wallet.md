# Multiple Wallets

dcrpulse can hold **several wallets** and switch between them from the dashboard. Only **one wallet is active at a time**, because dcrwallet loads a single wallet per process. Selecting a different wallet relaunches the per-wallet daemon stack against that wallet's own data directories.

## Overview

Each wallet has its own on-disk data (seed-derived keys, accounts, transaction history) and its own dashboard-side configuration (network, watch-only flag, privacy flag). The dashboard keeps a single **active wallet** and points the daemon supervisors at it through a shared control pointer. When you switch wallets, dcrwallet (and, where set up, dcrlnd, Bison Relay, and DCRDEX) restart against the selected wallet's directories.

**Access**: Open the **Wallet** section, then choose **Switch Wallet** from the wallet sidebar. The same picker appears automatically when wallets exist but none is currently open.

---

## The Wallet Gate

The Wallet section decides what to show based on the wallet list:

- **No wallets on disk** -> first-run **Wallet Setup** (create or restore your first wallet).
- **Wallets exist but none is active** -> the **wallet picker** ("Select a wallet").
- **A wallet is active** -> the normal wallet dashboard, with the active wallet's name shown at the top of the sidebar.

This means closing the active wallet returns you to the picker rather than to an empty screen.

---

## The Default Wallet

The **default wallet** (internally `default-wallet`) is special:

- It maps to dcrwallet's original single-wallet appdata path. An installation that existed before multi-wallet support keeps its wallet exactly where it was, with no migration, and that wallet appears in the list as the default.
- Its appdata root also holds the **shared control and backup directories** used by every wallet.
- It **cannot be renamed** and **cannot be deleted**. It is the fallback wallet the dashboard resolves to when no explicit selection exists.

Every other wallet you create lives in its own directory under the wallets root and can be renamed or deleted.

---

## Listing Wallets

The picker lists every wallet dcrwallet can load: the default wallet (when its database is present) plus each per-wallet directory. Entries are sorted by most-recently-accessed first, then by name.

Each wallet row shows:

- **Name** - the wallet's name.
- **Active** badge - on the wallet currently open.
- **Network** - mainnet, testnet, or simnet.
- **Watch-only** - shown when the wallet was set up from an imported xpub (cannot spend).
- **Privacy** - shown when the wallet has privacy (mixing) enabled.
- **No database** - shown when the directory exists but has no wallet database yet; such a wallet cannot be opened.

**API**: `GET /api/wallets` returns `{ wallets: [...], active: "<name>" }`. The `active` field is empty when no wallet is open.

---

## Selecting and Switching the Active Wallet

Click **Select** on any wallet to make it active (the active wallet's button reads **Open**). The dashboard then performs a full switch and reloads into that wallet.

### What a switch does

Switching is more than a UI change - it relaunches the daemon stack:

1. **Pause sync** so background polling does not race the changeover.
2. **Close the current wallet** cleanly before its daemon is stopped.
3. **Repoint the control pointer** at the new wallet's appdata and persist the selection.
4. **Wait for the dcrwallet supervisor** to report it is running the selected wallet.
5. **Reconnect gRPC** to the relaunched dcrwallet and wait for it to be ready.
6. **Open the wallet** (prompting for a public passphrase if the wallet is encrypted).
7. **Reconnect the rest of the stack** - the dcrlnd, DCRDEX, and Bison Relay clients are repointed at the new wallet's per-wallet certificates, and any session secret from the previous wallet is cleared. Those daemons relaunch independently from the same control pointer; their own status machines resolve to needs-unlock, needs-setup, or syncing as appropriate.

Because the daemons restart, a switch is not instant. The button shows **Switching...** while it runs, and the operation is bounded by a timeout.

#### When a switch is refused

A switch returns **409 Conflict** while the **privacy mixer**, the **ticket autobuyer**, or a **ticket purchase** is running, because those workers carry the current wallet's account numbers and would act on the next wallet. Stop the mixer or the autobuyer, or wait for the purchase to finish, then retry. Re-selecting the wallet that is already active is exempt, since the workers already belong to it.

#### Public passphrase prompt

If a wallet was created with a public passphrase, opening it fails until the passphrase is supplied. The picker detects this and prompts for the **public passphrase**, then retries the open. Wallets without a public passphrase open directly.

**API**: `POST /api/wallets/select` with `{ "name": "...", "publicPassphrase": "..." }`. A passphrase mismatch returns **401**, which is what triggers the prompt. This route is rate limited (one call per 5 seconds) because it cycles the daemon.

### Closing the active wallet

Closing the active wallet stops the wallet process (the supervisor idles), clears the previous wallet's DCRDEX session secret, and returns the UI to the picker. It is refused with **409 Conflict** under the same running-worker rule as a switch.

**API**: `POST /api/wallet/close`.

---

## Creating a New Wallet

From the picker, choose **Create new wallet** to run the standard setup flow under a new name. The wizard offers three modes: **Create new wallet** (generate a fresh seed), **Restore from seed** (enter an existing 33-word seed or raw hex), and **Watch-only wallet** (import an extended public key to monitor balances without spending keys). Creating a wallet switches the daemon to the new wallet's appdata first, then runs create/restore, and finally repoints the dcrlnd / DCRDEX / Bison Relay clients exactly as a switch does.

### Wallet names

- Allowed: 1 to 64 characters, letters, numbers, dash, or underscore.
- The name `imported` is **reserved** (it is dcrwallet's private-key account bucket).
- A name that already has a wallet database is rejected.
- An empty name falls back to the default wallet.

### Passphrase rules

- A **private passphrase** is required for a seeded wallet and must be at least 8 characters; its confirmation must match.
- A **public passphrase** is optional; when set it must also be at least 8 characters and match its confirmation.
- A **seed** is required for a seeded wallet (generated for a new wallet, or entered to restore one).
- A **watch-only wallet** takes neither a seed nor a private passphrase. It needs an **extended public key** starting with `dpub` (mainnet) or `tpub` (testnet), and it must be created from the device's account 0; import further device accounts after creation. The public passphrase stays optional under the same rules.

**API**: `POST /api/wallets/create` with the create-wallet body (name, passphrases, seed, discover-accounts flag), or with `"watchOnly": true` plus `extendedPubKey` for a watch-only wallet. Rate limited to one call per 5 seconds.

---

## Renaming a Wallet

In the picker, click **Edit**, then the rename (pencil) action on a wallet to give it a new name.

Constraints:

- The **default wallet cannot be renamed**.
- The **active wallet cannot be renamed** - close it first.
- The new name must pass the same name rules as creation, and must not already exist.

Renaming moves the wallet's data directory to the new name and renames its dashboard-side config directory to match.

**API**: `POST /api/wallets/rename` with `{ "from": "...", "to": "..." }`. Rate limited.

---

## Deleting a Wallet

In the picker, click **Edit**, then the delete (trash) action. A confirmation dialog warns that the deletion cannot be undone and that the wallet is not backed up, and it requires you to type **DELETE** to confirm before the button becomes usable.

Constraints:

- The **default wallet cannot be deleted**.
- The **active wallet cannot be deleted** - close it first.

### What delete does to your data

Deleting is **irreversible and takes no backup**. The wallet's data directory (`wallet.db` and everything beside it) is removed from disk outright, and the dashboard-side config directory (metadata only) is removed with it. A wallet whose seed you control can be restored from that seed; without the seed the coins are gone.

**API**: `POST /api/wallets/delete` with `{ "name": "..." }`. Rate limited.

---

## Cautions

### Funds and seed phrases
- **Always keep the seed phrase** for every wallet you create or restore. Deleting a wallet leaves no on-disk copy behind; the seed is the only recovery path.
- **Deleting a wallet erases its data.** The directory is removed from disk, so make sure you hold the seed before deleting.

### Switching takes time
- A switch **restarts dcrwallet and the dependent daemons**. Expect a short delay, and a fresh sync state on the newly opened wallet (the dashboard pauses polling during the changeover and resumes after).
- The active wallet's **Lightning, DCRDEX, and Bison Relay** clients re-resolve their state after a switch; if one of those needs unlocking or setup for the wallet you switched to, its panel will say so.

### One wallet at a time
- Only the active wallet is served by dcrwallet. Balances, transactions, staking, Lightning channels, DEX, and Bison Relay in the dashboard always reflect the **currently active** wallet, not all wallets combined.

---

## Troubleshooting

### Wallet asks for a passphrase on open
**Problem**: Selecting a wallet shows a "Public passphrase required" prompt.

**Cause**: The wallet was created with a public passphrase.

**Solution**: Enter the wallet's public passphrase. If you do not remember it, the wallet can be restored from its seed.

### "Switching..." takes a long time
**Problem**: The switch seems slow.

**Cause**: A switch relaunches the daemon stack and waits for the supervisor and gRPC to come back.

**Solutions:**
1. Wait for the relaunch to complete (the operation is bounded by a timeout).
2. Check the daemon logs if it does not finish: `docker compose logs -f dcrwallet`.

### A wallet shows "No database"
**Problem**: A wallet appears in the list but cannot be selected.

**Cause**: The wallet's directory exists without a wallet database (for example, an interrupted create).

**Solution**: Create or restore the wallet again, or delete the empty entry.

### Cannot rename or delete a wallet
**Problem**: The rename/delete actions are unavailable for a wallet.

**Cause**: The wallet is the **default wallet** or is **currently active**.

**Solution**: Close the active wallet first (returning to the picker), and remember that the default wallet is intentionally protected.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - Monitoring the active wallet
- **[Wallet Setup](../guides/wallet-operations.md)** - Creating or restoring a wallet
- **[Backup & Restore](../guides/backup-restore.md)** - Protecting your funds

---

**Questions?** Check the [FAQ](../guides/troubleshooting.md) or [Troubleshooting Guide](../guides/troubleshooting.md)
