# Bison Relay

**Bison Relay** is an end-to-end encrypted, Lightning-paid messaging network built on Decred. dcrpulse runs the Bison Relay client headlessly inside the dashboard (the `brclientd` daemon) and bridges it to your existing dcrlnd node for payments, so you get direct messages, group chats, a social feed, hosted pages, a storefront, file sharing, and tipping without running a separate desktop client.

## Overview

Bison Relay has no central server that can read your messages. Every message is end-to-end encrypted, and the network charges tiny Lightning micropayments to push data through a relay server. Because of this, Bison Relay needs a funded Lightning channel before it can operate. dcrpulse handles the wiring: it manages your identity key on disk in the `brclientd` data volume, pays the per-message fees from your dcrlnd wallet, and surfaces the whole network as tabs inside the dashboard.

**Access**: Click the **"Bison Relay"** button in the header navigation, or open `/br` directly.

**What `brclientd` is**: `brclientd` is a general-purpose headless Bison Relay daemon. It holds your identity, talks to the relay server, signs and decrypts messages, and exposes a local API that the dashboard backend proxies under the `/api/br/...` route group. The dashboard never speaks the raw Bison Relay protocol itself; it calls `brclientd`.

**Important cautions** (shown on the enable screen):

- Bison Relay is pre-1.0 and unaudited. Treat it as experimental.
- dcrpulse holds your Bison Relay identity key on disk in the `brclientd` data volume. Anyone with access to that volume can impersonate you.
- Sending and receiving messages costs DCR Lightning micropayments. You need a funded dcrlnd channel before Bison Relay will operate.

---

## The Bison Relay Workspace

Once your identity exists and the daemon is connected, the Bison Relay area is organized into tabs along the top:

- **Chat** - direct messages and group chats
- **Feed** - the social timeline (posts, hearts, comments)
- **Files** - shared files and downloads
- **Stats** - identity, payment, network, contact, and content analytics
- **Pages** - host and view markdown pages (and the storefront)
- **Settings** - identity, connection, backups, filters, and more

A **notification bell** sits at the end of the tab bar. The active tab is reflected in the URL hash (for example `#feed`, `#pages`, `#settings`), so deep links and browser back/forward work. Some tabs support subpaths such as `#feed/<author>/<post-id>` or `#pages/visit/<user-id>/index.md`.

A **Realtime** voice tab also exists but is hidden from the navigation; see [Realtime Voice (RTDT)](#realtime-voice-rtdt) below.

---

## Initial Setup

Before the workspace appears, dcrpulse runs a setup flow. Bison Relay first requires the wallet to be synced and responsive; until then a sync gate is shown instead of the wizard.

### Disclaimer

The first time you open Bison Relay you must read and accept the enable screen (**"Enable Bison Relay"**) by clicking **"I Understand, Continue"**. Acceptance is remembered in the browser.

### The setup checklist

The wizard (**"Bison Relay setup"**) shows a live checklist and advances on its own as each prerequisite is met. It polls status every couple of seconds and lands you on the workspace when everything passes.

1. **Lightning wallet unlocked** - Bison Relay pays message fees from your Lightning wallet, so it must be unlocked. If it is locked, an **"Unlock Lightning"** button links to the Lightning page.
2. **Lightning channel to the Bison Relay hub** - You need at least one open channel with the recommended hub. The wizard shows the recommended peer URI with a copy button and an **"Open channel on Lightning"** link that pre-fills the peer. The guidance recommends 0.5 DCR or more (1000 milliatoms is Bison Relay's minimum; more gives routing headroom). Once your channel is opening, the row shows on-chain confirmation progress (a channel needs three confirmations).
3. **Lightning network ready** - After a new channel opens, dcrlnd needs a few minutes to learn about it from network gossip before payments can route. This step usually clears on its own. Until the graph is synced, the wizard intentionally holds back nickname entry so you do not hit a misleading "insufficient local balance" routing error.
4. **Bison Relay nickname** - Once the Lightning steps pass, you choose your identity (see below).
5. **Connected to Bison Relay** - dcrpulse connects to the network automatically and loads the workspace.

If `brclientd` is not reachable (for example right after a stack restart), the wizard shows a warning and keeps polling until the daemon is back.

### Creating your identity

When the nickname step is active, the form asks for:

- **Nickname (required)** - identifies you to other Bison Relay users (up to 32 characters).
- **Display name (optional)** - a longer display name (up to 64 characters).

**Neither value can be changed once the identity is created.** A suggestion is shown to use a separate wallet for Bison Relay so your messaging identity and its channels stay isolated from your everyday funds; a **"Switch wallet"** link is provided. Click **"Create identity"** to finish.

### Restoring from a backup

Instead of creating a fresh identity, you can upload an existing Bison Relay backup (a `.tar.gz`) from the **"Restore from backup"** form on the same screen. Notes that apply:

- Only Bison Relay state is restored: identity, contacts, message history, posts, and shared files. A Lightning wallet inside the backup is **not** restored; Bison Relay payments use this node's Lightning wallet instead.
- Backups taken by other Bison Relay clients (bruig, brclient) do not include chat history (those clients keep logs outside the backup). Identity and contacts still restore fully.
- Stop using the client the backup came from before restoring. The same identity must never run in two clients at once or it corrupts the encrypted sessions with your contacts.

After upload, `brclientd` stages the tarball and restarts to extract it; the wizard continues automatically once it is back. After a restore the node automatically re-keys the encrypted session with every contact when it reconnects, so messaging resumes even from an older snapshot.

---

## Chat

The **Chat** tab is a two-pane messenger. On desktop a sidebar on the left lists your contacts and group chats; the conversation pane fills the rest. On mobile the two switch to full-width views with a back button. The sidebar header ("Chats") has toolbar buttons to create an invite, import an invite, manage contact groups, and join the Decred Pulse community.

### The contacts sidebar

Each direct-message contact row shows:

- **Display nick** - the local alias if you set one, otherwise the contact's own nickname
- **Unread badge** - a count pill (shows "99+" past 99); only shown when the Direct Messages notification preference is on
- **Last heard age** - a relative label (for example "2h") for when you last received from them
- **Avatar** - a colored initial badge or their uploaded avatar; click it to open per-contact actions
- **Ignored indicator** - a small eye-off icon and a dimmed row if the contact is ignored

Contacts are sorted **unread first, then alphabetically** by display nick. The list is sectioned into your regular contacts, any custom contact groups (collapsible), and an **Archived** section (collapsed by default). Group chats appear in their own **Groups** section.

### Direct messages

Selecting a contact loads the **last 100 messages** of history. The composer at the bottom grows as you type. Features:

- **Plain text** with automatic linkification of URLs.
- **Attachments** via the paperclip button. Compressible images open a preview/compression modal; a compressed copy small enough is embedded inline in the message, while larger images or other files are sent as a Lightning-paid file transfer.
- **Send** with the send button (disabled while a send is in flight).

Message delivery status is shown with check marks: a **single check** means sent to the relay, a **double check** means acknowledged by the relay server. Your own messages render on the right; the contact's messages render on the left with their nick and avatar. Files a contact sends you appear as download tags in the message stream.

### Per-contact actions

Clicking a contact's avatar opens a side panel of actions for that contact:

- **View Profile** - jump to the contact's full profile (stats and post feed).
- **Pay Tip** - open the tip modal (see [Tipping](#tipping)).
- **Send File** - attach and send a file.
- **Subscribe to Posts / Unsubscribe from Posts** - toggle whether their posts show up in your Feed.
- **List Posts** - browse the contact's posts with download buttons (already-fetched posts are marked).
- **Show Content** - list the contact's shared files with their prices and download buttons (you confirm before paying).
- **View Pages** - open the contact's hosted pages.
- **Rename User** - set a local alias (up to 64 characters); the peer is not notified and their own nick is unchanged.
- **Contact Group** - assign the contact to one of your groups.
- **Ignore User / Un-ignore User** - hide their messages and posts locally without removing them; reversible and nothing is sent to them.
- **Block User** - notifies them you blocked them and removes the contact and history locally; cannot be undone without a fresh invite.
- **Request Ratchet Reset** - re-key the encrypted session directly with the contact.
- **Perform Handshake** - send a 3-way handshake to confirm the connection is fully working.
- **Suggest User to KX** - pick another contact and suggest the two of them perform a key exchange.
- **Issue Transitive Reset** - pick a common contact to mediate a ratchet-reset request when a direct reset will not go through.
- **Clear Chat History** - permanently delete the local messages and inline media for this contact; you must type `DELETE` to confirm and it cannot be undone.

### Contacts, invites, and key exchange

Bison Relay contacts are added through **key exchange (KX)**, bootstrapped by an out-of-band invite that you share over some other channel.

- **Create invite** - generate an invite string (or file) to hand to someone. They import it to start the key exchange.
- **Import invite** - paste an invite string you received to begin the key exchange with its author. When the exchange completes, a "Completed KX" line appears in the thread and the contact shows up in the sidebar.
- **Suggest-KX** - rather than a raw invite, you can ask an existing contact to KX with another of your contacts; they decide whether to follow up.

These flows map to the `/api/br/invites/...` and `/api/br/contacts/...` route groups, plus `/api/br/kx/...` for listing in-flight key exchanges, searches, and mediated introductions.

### Group chats

Group chats appear in the **Groups** section of the sidebar. A group row shows the group alias (or name), an avatar, and an unread badge. Opening a group shows its conversation with a header that displays the group name, member count, an **Owner** badge if you own it, an **"Invite"** button (admins only), and a **"Manage"** button.

Creating a group uses the **"Create group"** action in the Groups header. Incoming group invites stack as a banner at the top of the chat area with **"Accept"** and **"Dismiss"** buttons; a stale local copy left over after a restore can show a "re-invite blocked" banner with a **"Leave local copy"** action so the inviter can send a fresh one.

The **"Manage"** panel lists members (with crown/shield/ban icons for owner, admin, and blocked members) and exposes:

- **Kick** a member (with an optional reason) - admins.
- **Block / Unblock** a member locally - admins (a client-side filter on their messages).
- **Promote to admin / Demote from admin** - the owner (on a version 1 group).
- **Transfer ownership** - the owner.
- **Rename locally (alias)** - set a sidebar-only name for the group.
- **Resend member list** - re-sync the roster (admins).
- **Upgrade to v1** - the owner can upgrade a legacy group to enable extra admins (one-way; all members must support v1).
- **Leave group** - non-owners leave and notify the others.
- **Dissolve group (owner)** - destroy the group for everyone (cannot be undone).

System lines (joins, leaves, kicks, admin changes) render as light gray internal messages. If you are removed from or the group is dissolved while you are viewing it, an overlay explains what happened and the thread becomes read-only. These actions map to the `/api/br/gc/...` route group.

---

## Feed

The **Feed** tab is a social timeline of posts from you and the authors you subscribe to, newest first, loaded in pages.

The sidebar within Feed has:

- **Feed** - all posts from subscribed authors (the default).
- **Your Posts** - only posts you authored.
- **Subscriptions** - manage who you subscribe to.
- **New Post** - compose and publish.

### Reading posts

Each post card shows the author, avatar, publication date, title, an optional description snippet, the first image (with a "+N more images" badge if there are more), a body preview, and engagement counts (hearts/"atoms" and comments). For your own posts a "Seen by" count is shown. A blue dot marks posts with new activity since you last opened them (subject to your notification preference). Click a card to open the full post.

In the post detail view:

- The **body renders as markdown** with inline images (click to zoom), inline file attachments, and any paid download embeds.
- **Atom** (the heart/like button) toggles your reaction; the count is shown, and as the author you can see who reacted.
- **Comment** adds a plain-text comment (markdown is not supported in comments). Comments are threaded with reply buttons; replies nest under a vertical rail. A comment shows a clock icon until it has been relayed by the post author, and a seen indicator where available.
- **Relay** (on posts you do not own) re-broadcasts the post to your own subscribers after a confirmation; once relayed the button shows a "Relayed" state.
- **Pay tip** opens the tip modal for the author (see [Tipping](#tipping)).
- As the author, **Seen by** and **Atomed by** sections list the subscribers who received and reacted to your post.

### Creating a post

**New Post** opens a markdown editor with a Write/Preview toggle and a formatting toolbar (bold, italic, strikethrough, headings, lists, blockquote, inline code, code blocks, links). You can:

- **Attach an image or file** - compressible images open an attach modal where you pick the original or a smaller compressed copy and add optional alt text; the image is embedded inline. Files that are too large to inline are rejected with a hint to use a shared-content link instead.
- **Link to shared content** - pick (or upload) one of your shared files and optionally set a price in DCR so readers pay to download it.
- **Description (optional)** - a short summary (up to 200 characters) shown under the title in feed cards.

A size footer shows the estimated wire size against the per-post cap (Bison Relay posts ride the same wire as chat, with a 1 MB hard cap and a soft warning earlier). **Publish** sends the post; it is disabled if the body is empty or the post is over the size limit.

### Subscriptions

The **Subscriptions** view has **Subscribed** and **Not subscribed** tabs, each with a count, listing your contacts with a **Subscribe** or **Unsubscribe** button. You can also subscribe to everyone at once from the Settings tab (see [Settings](#settings)). New contacts you key-exchange with subscribe to your posts by default. Subscribing asks the author to send their existing and future posts; if they are offline it takes effect when they return.

These actions map to the `/api/br/posts/...` and `/api/br/contacts/(un)subscribe-posts` routes.

### Paid download embeds

A post (or chat message) can carry a paid file embed, shown as a compact card with the filename, size, price in DCR, an approximate USD figure, and a lock icon. Clicking **Download** on a free embed starts the transfer immediately; for a paid embed a confirmation modal (**"Pay & download"** / **"Cancel"**) appears first. If the seller's actual price differs from what the post advertised, the modal asks you to confirm again at the real price. Progress is shown as chunks transfer, and once complete the embed becomes a direct download link (images render inline). USD figures are best-effort; the DCR price always shows even if the rate lookup fails.

---

## Files

The **Files** tab manages the files you share and the transfers in flight. It has three sub-views:

- **Add** - share a file. Pick a local file, set an optional cost in DCR (0 for free), choose a sharing preference (**Global** for all your subscribers, or a single contact), and add an optional description (up to 200 characters). Click **Share**.
- **Shared** - the files you currently share, each showing a Global or Per-user badge, size, cost (or "Free"), and file ID, with an **Unshare** button.
- **Downloads** - in-flight and recently completed transfers, each showing a Receiving or Sending badge, the contact, size, chunk progress with a progress bar, and the on-disk path when finished. In-progress receives can be canceled. Progress updates live.

Shared files are what you reference when you add a paid download to a post. These map to the `/api/br/files/...`, `/api/br/shared-files`, and `/api/br/downloads/...` routes.

---

## Pages

**Pages** lets you host and view markdown documents over the Bison Relay network. The same tab also hosts the optional storefront (see [Storefront](#storefront)). A single node hosts one mode at a time: deactivated, static pages, or a storefront.

### Visiting a page

The **Visit** view fetches another user's page. Enter their user ID (64-character hex or base64) and a path (defaults to `index.md`), and the page renders as markdown with its embedded images and downloadable files. Navigation supports **Back / Forward** and **Refresh** buttons, relative links within the same host's pages, and `br://userid/path.md` links to other users' pages. Pages can include interactive form sections that update in place.

### Hosting your own pages

The **My Pages** view manages the markdown files you host:

- **New page** - create a `.md` file. Names allow letters, digits, dash, underscore, dot, and subdirectories via `/`; `..` is blocked. `index.md` is the root page visitors get when no path is given.
- The **editor** is a markdown editor with embedded image and file support; file names are fixed once created.
- **Save page** writes the content, **Edit** and **Delete** act per page (delete asks for confirmation), and **Preview my page** renders a page the way visitors see it.

If hosting is not currently set to static pages, a banner notes that your pages are saved on disk but are not served until you switch hosting back to pages. These map to the `/api/br/pages/...` routes.

---

## Storefront

From the Pages area you can switch the node into **storefront** mode (also called simplestore), which serves a shoppable catalog over the relay instead of plain pages. The hosting-mode panel offers **Hosting deactivated**, **Hosting static pages**, and **Hosting a storefront**, with buttons to move between them. Switching away from the storefront warns that the invoice watcher stops and unpaid orders will not settle until you re-enable it. Setting up a storefront asks for a payment method (**Lightning** or **On-chain**), a wallet account for on-chain order addresses, and an optional shipping surcharge in USD. Enabling the storefront replaces page hosting on the node (your pages stay on disk for later).

When the storefront is active, the manager exposes three tabs:

- **Products** - your catalog (prices are in USD; the store charges the DCR equivalent at order time). Each product has a **Title**, a unique **SKU** (fixed after creation), a **Price (USD)**, a **Description**, comma-separated **Tags**, a **Requires shipping address** checkbox, a **Hidden** checkbox, and an optional **Digital download** file (a path under the store directory, with an **Upload file** button). Use **Add product**, the pencil to edit, and the trash to delete. Changes apply live.
- **Orders** - orders customers placed over the relay, newest first. Each shows an order ID, a color-coded status (**placed**, **paid**, **shipped**, **completed**, **canceled**), item count and total USD, the customer, payment type, and a timestamp; a dropdown advances the status. Expanding an order reveals the cart items, any shipping charge and shipping address, the invoice ID, and a messages thread where you can reply to the customer. When a paid order includes a digital download, the file is delivered to the buyer automatically. Orders refresh live.
- **Templates** - the Go templates the storefront renders from. Pick a template from the list, edit it, and **Save** (a syntax error keeps the previous version live). **New template** creates one, the trash deletes one, and **View storefront** previews the rendered output. Changes apply live.

These map to the `/api/br/store/...` routes.

---

## Stats

The **Stats** tab is a read-only analytics view with several sections:

- **Overview** - an identity strip (your nick, public ID, avatar, and connection status), big-number tiles (contacts and followers, posts and subscriptions, total DCR moved with fees, uptime), a top-contacts-by-activity list, and a server card.
- **Payments** - all-time sent/received/fees with a breakdown chart, tips currently in flight, server round-trip-time distribution, and a per-contact table you can expand for a per-event cost breakdown. A contact's recorded payment stats can be cleared (this does not touch funds, history, or the contact).
- **Network** - the relay server (LN node pubkey, recommended hub, connection time), the server policy (push fee per MB, subscription fee, retention, max message size, max push invoices), outbound queue state where available, and round-trip latency.
- **Contacts** - contact-health hero cards and a per-contact table with key-exchange age, status (active/idle/awaiting peer/offline), and expandable ratchet detail. A blocked-contacts card lists locally blocked users with an unblock action.
- **Content** - posts authored, atoms and comments received, your subscribers, and a top-posts-by-engagement list.

These map to the `/api/br/stats/...` routes.

---

## Settings

The **Settings** tab is organized into sections:

- **Account** - upload or clear your **avatar** (PNG/JPEG/GIF/WebP, up to 200 KiB; changes are broadcast to contacts), and **Subscribe to All Posts**, which asks every contact to send you their posts.
- **Appearance** - a **font-size scaling** control that scales only the Bison Relay section's text (stored in the browser).
- **Notifications** - toggles for **Direct Messages**, **Group Chat Messages**, and **Feed Posts** unread markers. Messages keep arriving either way; the toggles only hide the unread markers (stored in the browser).
- **Sessions** - **Reset All Sessions** re-keys the encrypted session with every contact (useful after a restore or connectivity trouble); **Reset Stale Sessions** only re-keys contacts unheard for 30+ days. This section also lists in-flight key exchanges, mediated introductions (with a cancel option), and KX searches.
- **Connection** - go **online/offline** (offline pauses the relay connection while messages keep queuing), view connection status and the server policy, toggle **Send Receive Receipts** (acknowledge posts and comments back to authors; changing this restarts the daemon), and **Request Inbound Channel** for inbound Lightning capacity.
- **Filters** - create content filters with a regular-expression **Pattern**, an optional user or group-chat scope, and checkboxes for which content types to hide (private messages, group messages, posts, comments). Matching content is hidden before it reaches the UI. Edit and delete existing filters from the list.
- **Backup** - download a full Bison Relay backup (see below).
- **About** - the daemon version, Bison Relay library version, and Go runtime version.

### Backing up your identity

The **Backup** section downloads a full snapshot of your Bison Relay state (identity, contacts, message history, posts, and shared files). Because building the tarball can take a while, it is a two-phase flow:

1. Click **"Download Backup"** to start preparation. The status shows **"Preparing..."** with an elapsed timer while `brclientd` assembles the archive.
2. When it is ready the status shows **"Backup ready"** with the size and the button changes to **"Save Backup File"**; the browser downloads the file automatically. You can navigate away and the file stays available until the next prepare or a dashboard restart.

Restore the resulting `.tar.gz` from the setup wizard on a fresh node (see [Restoring from a backup](#restoring-from-a-backup)). Sessions with contacts you message after taking the backup are re-keyed automatically on restore, so back up regularly. This maps to the `/api/br/backup`, `/api/br/backup/prepare`, `/api/br/backup/status`, and `/api/br/backup/restore` routes.

### Contact groups

The contact-groups manager (reached from the Chat sidebar toolbar or per-contact actions) lets you create, rename, and delete custom groups, assign a contact to a group, and configure the built-in **Archived** group. Assigning a contact to Archived stops it from producing unread badges (it still receives messages), and you can **pin** an archived contact so it stays archived even when new messages arrive. An **auto-archive** threshold archives contacts unheard for a set number of days; auto-archived contacts return to the contact list when they message again, while pinned ones stay archived. These map to the `/api/br/contacts/groups...` routes.

---

## Tipping

You can send a DCR tip over Lightning to a contact or a post author. The tip modal (**"Pay tip to <nick>"**) has an **Amount (DCR)** field and notes that the tip rides over your Lightning channel and both parties must be online for delivery; recent tip attempts to that user are listed with their amount, time, and status. Click **Send tip** and the modal closes; the attempt is tracked through live status updates (invoice generated, sent, or failed) that appear inline in the conversation or on the post. If the recipient is offline, delivery happens when their client is reachable again. Tipping maps to the `/api/br/contacts/tip` route, and tip history to `/api/br/payments/tips`.

---

## Notifications

The **notification bell** in the tab bar shows a badge with the unread count (capped at "99+"). Opening it shows a dropdown of recent events with a severity dot (error/warn/info), a subject, a timestamp, and detail text. Opening the bell marks everything currently shown as seen (tracked in the browser). Events that surface here include things like a tip received, a post heart, a new storefront order or status change, a failed invoice, low file-invoice capacity, a contact blocking you, being removed from a group, and connectivity warnings. The bell refreshes periodically and immediately on relevant live events. This is backed by the `/api/br/notifications/recent` route and the live event stream at `/api/br/events`.

---

## Realtime Voice (RTDT)

Bison Relay includes a real-time voice feature (RTDT, real-time data transport) for 1:1 calls and group rooms with in-call text chat. **This tab is hidden from the navigation** because the upstream Bison Relay build dcrpulse ships has no audio support; it remains reachable only via the `#realtime` URL hash as an easter egg. In the shipped build, treat it as a non-functional preview surface for voice: the room and call control surfaces and in-call text chat exist, but audio is not available without an upstream Bison Relay build that provides it (and a recent Chromium- or Firefox-based browser).

When reachable, the surface lets you start an **Instant call** with one contact or create a **New room** (capacity up to 32, optional description, and a list of contacts to invite), accept incoming call invites from a banner, and inside a session see participants, send in-call text messages, mute the mic, leave, and (as the owner/admin) invite more people, rotate access cookies, kick participants, or dissolve the room. These map to the `/api/br/rtdt/...` routes, with audio frames bridged over a WebSocket.

---

## Related Documentation

- **[Wallet Dashboard](wallet-dashboard.md)** - account balances and transaction history
- **[Staking Guide](staking-guide.md)** - tickets, voting, and rewards
- **[Node Dashboard](node-dashboard.md)** - node and network status

---

## Troubleshooting

### The setup wizard is stuck on "Lightning network ready"

This step waits for dcrlnd to learn about your new channel from network gossip; it usually clears within a few minutes of the channel becoming active. Make sure your Lightning wallet is unlocked. The wizard exposes the underlying daemon message behind a "Show technical details" toggle.

### "brclientd is not reachable yet"

The daemon may still be starting after a stack restart, or its container may not be running. The wizard keeps polling and recovers on its own once the daemon is back.

### Messages stopped flowing after restoring a backup

Use **Reset All Sessions** in Settings to re-key the encrypted session with all contacts. After a restore the node normally re-keys automatically on reconnect, but a manual reset forces it.

### A contact's messages disappeared

Check whether you **ignored** the contact (an eye-off icon and dimmed row in the sidebar) or set up a content **filter** in Settings that matches their messages. Both hide messages locally without affecting the contact.
