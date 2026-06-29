// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext, Fragment, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { toYMD, toYMDTime } from '../../utils/date';
import {
  AlertCircle,
  Check,
  CheckCheck,
  ChevronLeft,
  Copy,
  Download,
  EyeOff,
  FileText,
  FolderCog,
  Loader2,
  MessageSquare,
  Paperclip,
  Send,
  UserPlus,
  Users,
  X,
  Zap,
} from 'lucide-react';
import {
  ARCHIVED_GROUP_ID,
  BisonrelayContact,
  BisonrelayDownloadEntry,
  BisonrelayGC,
  BisonrelayMessage,
  BisonrelayPMAttachment,
  acceptBisonrelayGCInvite,
  acceptBisonrelayInvite,
  acceptBisonrelayKxSuggestion,
  getBisonrelayContacts,
  getBisonrelayDownloads,
  getBisonrelayGCHistory,
  getBisonrelayIdentity,
  getBisonrelayMessages,
  joinDecredPulse,
  listBisonrelayGCInvites,
  listBisonrelayGCs,
  sendBisonrelayFile,
  sendBisonrelayGCMessage,
  sendBisonrelayPM,
  subscribeBisonrelayPosts,
  tipBisonrelayContact,
  unsubscribeBisonrelayPosts,
  writeBisonrelayInvite,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { GroupManagementModal } from './BisonrelayContactGroupModals';
import { useBrNotifPrefs } from './brNotifPrefs';
import {
  DownloadSegment,
  EmbedSegment,
  buildDownloadTag,
  downloadFileUrl,
  embedFileUrl,
  formatBytes,
  isImageMime,
  parseEmbeds,
} from './embedParser';
import { linkifyChatText } from './chatLinkify';
import { ImageAttachModal, ImageAttachResult, isCompressibleImage } from './editor';
import { EmojiPicker } from './EmojiPicker';
import { ChatFormatMenu } from './ChatFormatMenu';
import { TipModal } from './TipModal';
import { ImageViewerModal, ViewerImage } from './ImageViewerModal';
import { avatarDataUrl, colorForUid } from './bisonrelayAvatar';
import { AuthorAvatar } from './AuthorAvatar';
import { BisonrelayUserSubNav } from './BisonrelayUserSubNav';
import { CreateGCModal } from './gc/CreateGCModal';
import { GCInviteModal } from './gc/GCInviteModal';
import { GroupSubNav } from './gc/GroupSubNav';
import { IncomingGCInvitesBanner } from './gc/IncomingGCInvitesBanner';

const MAX_INLINE_BYTES = 800 * 1024;
const MAX_TRANSFER_BYTES = 100 * 1024 * 1024;

// DECRED_PULSE_GC is the name of the community welcome group chat the invite
// bot adds new users to. The "Join Decred chat networks" action is hidden once
// the user is already a member.
const DECRED_PULSE_GC = 'Decred Pulse';

interface StagedAttachment {
  file: File;
  mode: 'inline' | 'transfer';
  dataB64?: string;
}

interface ImageViewerOpenFn {
  (src: string, name: string, mime: string): void;
}

const ImageViewerCtx = createContext<ImageViewerOpenFn | null>(null);

// ActiveTarget tags the chat-window subject as either a 1:1 PM contact or
// an N-peer GC. Most existing code paths only care about the contact case;
// the explicit kind lets us branch send / history / header at the few
// places it matters without sprinkling null-checks everywhere.
type ActiveTarget =
  | { kind: 'contact'; value: BisonrelayContact }
  | { kind: 'group'; value: BisonrelayGC };

// Max pixel height the auto-growing message composer reaches before it scrolls
// internally. Keep in sync with the textarea's max-h-[9rem] class (9rem = 144px).
const COMPOSER_MAX_PX = 144;

export const BisonrelayMessagingPage = ({ ownNick }: { ownNick: string }) => {
  const [contacts, setContacts] = useState<BisonrelayContact[]>([]);
  const [contactsErr, setContactsErr] = useState<string | null>(null);
  const [gcs, setGCs] = useState<BisonrelayGC[]>([]);
  const [gcsErr, setGCsErr] = useState<string | null>(null);
  const [selected, setSelected] = useState<ActiveTarget | null>(null);
  // Derived accessors keep the rest of the file's existing references to
  // selectedContact?.* working without churn.
  const selectedContact = selected?.kind === 'contact' ? selected.value : null;
  const selectedGroup = selected?.kind === 'group' ? selected.value : null;
  // Derived from the live GC list (not the captured selection) so it flips the
  // moment refreshGCs reflects a removal: the open GC is read-only once we are
  // no longer a member.
  const selectedGroupRemoved =
    !!selectedGroup && gcs.some((g) => g.id === selectedGroup.id && g.local_is_member === false);
  const [messages, setMessages] = useState<BisonrelayMessage[]>([]);
  // Set when we are removed from / the dissolution of the GC we are actively
  // viewing arrives: the thread blanks out with a prominent notice.
  const [kickNotice, setKickNotice] = useState<{
    gcid: string;
    kind: 'kicked' | 'dissolved';
    name: string;
    reason?: string;
    by?: string;
  } | null>(null);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [messagesErr, setMessagesErr] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [showTip, setShowTip] = useState(false);
  const [showInviteCreate, setShowInviteCreate] = useState(false);
  const [showInviteAccept, setShowInviteAccept] = useState(false);
  const [showJoinDecredPulse, setShowJoinDecredPulse] = useState(false);
  const inDecredPulse = useMemo(
    () => gcs.some((g) => g.name === DECRED_PULSE_GC || g.alias === DECRED_PULSE_GC),
    [gcs]
  );
  // Our own identity in hex, to resolve "ourself" in member lists (the local
  // user is never in our own contacts). Best-effort; consumers fall back to the
  // raw uid if it is unavailable.
  const [ownUid, setOwnUid] = useState('');
  useEffect(() => {
    let cancelled = false;
    getBisonrelayIdentity()
      .then((id) => {
        if (!cancelled && id?.identity) setOwnUid(brIdentityToHex(id.identity));
      })
      .catch(() => {
        /* ignore; sidebar falls back to the raw uid */
      });
    return () => {
      cancelled = true;
    };
  }, []);
  const [attachment, setAttachment] = useState<StagedAttachment | null>(null);
  const [attachErr, setAttachErr] = useState<string | null>(null);
  const [pendingImage, setPendingImage] = useState<File | null>(null);
  const [viewerImage, setViewerImage] = useState<ViewerImage | null>(null);
  const [subNavContact, setSubNavContact] = useState<BisonrelayContact | null>(null);
  const [contactsCollapsed, setContactsCollapsed] = useState(
    () => localStorage.getItem('dcrpulse.br.sidebar.contacts-collapsed') === '1',
  );
  const [groupsCollapsed, setGroupsCollapsed] = useState(
    () => localStorage.getItem('dcrpulse.br.sidebar.groups-collapsed') === '1',
  );
  const [showCreateGC, setShowCreateGC] = useState(false);
  const [showGCInvite, setShowGCInvite] = useState(false);
  const [showGroupSubNav, setShowGroupSubNav] = useState(false);
  const [showGroupMgmt, setShowGroupMgmt] = useState(false);
  const [sectionCollapsed, setSectionCollapsed] = useState<Record<string, boolean>>(() => {
    try {
      const raw = localStorage.getItem('dcrpulse.br.sidebar.sections-collapsed');
      if (raw) return JSON.parse(raw);
    } catch {
      /* ignore */
    }
    return { [ARCHIVED_GROUP_ID]: true };
  });
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const draftInputRef = useRef<HTMLTextAreaElement | null>(null);
  // Grow the composer to fit its content up to COMPOSER_MAX_PX, then let it
  // scroll. Re-runs when the draft changes (typing, paste, and the reset to ''
  // after a send) and when the active chat changes (the textarea may remount).
  useLayoutEffect(() => {
    const el = draftInputRef.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_MAX_PX)}px`;
  }, [draft, selected]);
  const {
    unread,
    clearUnread,
    setActiveUid,
    gcUnread,
    clearGCUnread,
    pruneGCUnread,
    setActiveGCID,
    addListener,
    contactGroups,
  } = useBisonrelayLive();
  // The BR notification switches gate the unread indicators only; counting
  // continues so re-enabling a switch shows the true unread state.
  const notifPrefs = useBrNotifPrefs();

  useEffect(() => {
    localStorage.setItem('dcrpulse.br.sidebar.contacts-collapsed', contactsCollapsed ? '1' : '0');
  }, [contactsCollapsed]);
  useEffect(() => {
    localStorage.setItem('dcrpulse.br.sidebar.groups-collapsed', groupsCollapsed ? '1' : '0');
  }, [groupsCollapsed]);
  useEffect(() => {
    localStorage.setItem(
      'dcrpulse.br.sidebar.sections-collapsed',
      JSON.stringify(sectionCollapsed),
    );
  }, [sectionCollapsed]);

  const openImageViewer = useCallback<ImageViewerOpenFn>((src, name, mime) => {
    setViewerImage({ src, name, mime });
  }, []);

  // One-shot #chat/<uid> deep link (e.g. the profile page's Message button):
  // preselect the contact once the list is loaded, then consume the suffix
  // so refresh/back does not re-trigger the selection.
  useEffect(() => {
    const m = window.location.hash.replace(/^#/, '').match(/^chat\/([0-9a-f]{64})$/i);
    if (!m || contacts.length === 0) return;
    const c = contacts.find((x) => x.id?.identity?.toLowerCase() === m[1].toLowerCase());
    if (c) {
      setSelected({ kind: 'contact', value: c });
      window.history.replaceState(null, '', '#chat');
    }
  }, [contacts]);

  const refreshContacts = useCallback(async () => {
    try {
      const entries = await getBisonrelayContacts();
      setContacts(entries);
      setContactsErr(null);
    } catch (err: any) {
      setContactsErr(err?.message || 'Could not load contacts');
    }
  }, []);

  const refreshGCs = useCallback(async () => {
    try {
      const entries = await listBisonrelayGCs();
      setGCs(entries);
      setGCsErr(null);
      // Drop unread for GCs no longer in the list (e.g. kicked/dissolved) so the
      // nav dot can't get stuck on a group with no sidebar row to open.
      pruneGCUnread(entries.map((g) => g.id));
    } catch (err: any) {
      setGCsErr(err?.message || 'Could not load groups');
    }
  }, [pruneGCUnread]);

  useEffect(() => {
    refreshContacts();
    refreshGCs();
  }, [refreshContacts, refreshGCs]);

  const loadMessages = useCallback(async (contact: BisonrelayContact) => {
    const uid = contact.id?.identity;
    if (!uid) return;
    setMessagesLoading(true);
    setMessagesErr(null);
    try {
      const peerNick = contact.id?.nick ?? '';
      const [resp, downloads] = await Promise.all([
        getBisonrelayMessages(uid, 0, 100),
        peerNick ? getBisonrelayDownloads(peerNick) : Promise.resolve([] as BisonrelayDownloadEntry[]),
      ]);
      const pmEntries = resp.entries ?? [];
      const downloadEntries: BisonrelayMessage[] = downloads.map((d) => ({
        message: buildDownloadTag(peerNick, d.name, d.size, ''),
        from: peerNick,
        timestamp: d.mtime,
        internal: false,
      }));
      const merged = [...pmEntries, ...downloadEntries].sort((a, b) => a.timestamp - b.timestamp);
      setMessages(merged);
    } catch (err: any) {
      setMessagesErr(err?.message || 'Could not load messages');
    } finally {
      setMessagesLoading(false);
    }
  }, []);

  const loadGCMessages = useCallback(async (gc: BisonrelayGC) => {
    setMessagesLoading(true);
    setMessagesErr(null);
    try {
      const resp = await getBisonrelayGCHistory(gc.id, 0, 100);
      const entries = resp.entries ?? [];
      const sorted = [...entries].sort((a, b) => a.timestamp - b.timestamp);
      setMessages(sorted);
    } catch (err: any) {
      setMessagesErr(err?.message || 'Could not load group history');
    } finally {
      setMessagesLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!selected) {
      setMessages([]);
      setActiveUid('');
      setActiveGCID('');
      return;
    }
    if (selected.kind === 'contact') {
      loadMessages(selected.value);
      const uid = selected.value.id?.identity ?? '';
      setActiveUid(uid);
      setActiveGCID('');
      clearUnread(uid);
    } else {
      loadGCMessages(selected.value);
      setActiveUid('');
      setActiveGCID(selected.value.id);
      clearGCUnread(selected.value.id);
    }
  }, [
    selected,
    loadMessages,
    loadGCMessages,
    setActiveUid,
    setActiveGCID,
    clearUnread,
    clearGCUnread,
  ]);

  // selectedRef tracks the currently-open *contact* (kept narrow because
  // the live-event handlers below match PMs by contact uid).
  // selectedGroupRef does the same for groups so the gc-message handler
  // can decide whether to inject into the open thread.
  const selectedRef = useRef<BisonrelayContact | null>(null);
  useEffect(() => {
    selectedRef.current = selectedContact;
  }, [selectedContact]);

  const selectedGroupRef = useRef<BisonrelayGC | null>(null);
  useEffect(() => {
    selectedGroupRef.current = selectedGroup;
  }, [selectedGroup]);

  // Current contacts, readable from the once-registered live-event listener so
  // observed GC member-change lines can resolve a uid to a nick.
  const contactsRef = useRef<BisonrelayContact[]>([]);
  useEffect(() => {
    contactsRef.current = contacts;
  }, [contacts]);

  // The removal notice is scoped to its group: clear it once the user navigates
  // to a different conversation.
  useEffect(() => {
    if (kickNotice && selectedGroup?.id !== kickNotice.gcid) setKickNotice(null);
  }, [selectedGroup?.id, kickNotice]);

  // Keep the message input focused so the user can type immediately: when a
  // conversation is opened or switched, and again after a send completes (the
  // input is disabled while sending, so refocusing waits for it to re-enable).
  useEffect(() => {
    if (selected && !sending) {
      draftInputRef.current?.focus();
    }
  }, [selectedContact?.id?.identity, selectedGroup?.id, sending]);

  const handleAcceptSuggestion = useCallback(
    async (target: string, _targetNick: string) => {
      const cur = selectedRef.current;
      const mediator = cur?.id?.identity ?? '';
      if (!mediator || !target) {
        throw new Error('Missing mediator or target identity');
      }
      await acceptBisonrelayKxSuggestion(mediator, target);
    },
    [],
  );

  // handleTip kicks off a tip in the background and surfaces progress as an
  // optimistic placeholder in the chat thread. The placeholder is keyed by
  // tipKey so the eventual tip-sent / tip-failed live event can swap its
  // text + drop the spinner in place. If the brclientd call itself fails
  // (e.g. unknown user, transport error), we update the placeholder
  // ourselves with the error message — the BR notification path wouldn't
  // fire in that case.
  // handleSubscribePosts inserts an optimistic spinner placeholder into the
  // open thread and kicks off the BR subscribe/unsubscribe request in the
  // background. The live posts-subscribed / posts-unsubscribed event swaps
  // the placeholder text in place when the remote replies (which may be
  // immediately or much later if they're offline). If the dashboard call
  // itself fails (e.g. unknown user, transport error) we update the
  // placeholder ourselves with the failure reason.
  const handleSubscribePosts = useCallback(
    (kind: 'subscribe' | 'unsubscribe', uid: string, nick: string) => {
      const subKey = `sub-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      const verb = kind === 'subscribe' ? 'Subscribing to' : 'Unsubscribing from';
      setMessages((prev) => {
        const cur = selectedRef.current;
        if (!cur || cur.id?.identity !== uid) return prev;
        return [
          ...prev,
          {
            message: `${verb} ${nick}'s posts...`,
            from: '',
            timestamp: Math.floor(Date.now() / 1000),
            internal: true,
            pending: true,
            subKey,
          },
        ];
      });
      const apiCall =
        kind === 'subscribe'
          ? subscribeBisonrelayPosts(uid)
          : unsubscribeBisonrelayPosts(uid);
      apiCall.catch((err: any) => {
        const body = err?.response?.data;
        const msg = typeof body === 'string' ? body : err?.message || 'Request failed';
        const failVerb = kind === 'subscribe' ? 'Subscribe' : 'Unsubscribe';
        setMessages((prev) => {
          const idx = prev.findIndex((m) => m.subKey === subKey);
          if (idx === -1) return prev;
          const updated = [...prev];
          updated[idx] = {
            ...updated[idx],
            message: `${failVerb} request failed: ${msg}`,
            pending: false,
            subKey: undefined,
          };
          return updated;
        });
      });
    },
    [],
  );

  const handleTip = useCallback((recipientUid: string, nick: string, dcrAmount: number) => {
    const tipKey = `tip-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    // Pending text mirrors bruig's InflightTipW (events.dart:735).
    const pendingLine = `Requesting invoice for ${dcrAmount} DCR to tip ${nick}...`;
    setMessages((prev) => {
      const cur = selectedRef.current;
      if (!cur || cur.id?.identity !== recipientUid) return prev;
      return [
        ...prev,
        {
          message: pendingLine,
          from: '',
          timestamp: Math.floor(Date.now() / 1000),
          internal: true,
          pending: true,
          tipKey,
        },
      ];
    });
    tipBisonrelayContact(recipientUid, dcrAmount).catch((err: any) => {
      const body = err?.response?.data;
      const msg = typeof body === 'string' ? body : err?.message || 'Tip failed';
      setMessages((prev) => {
        const idx = prev.findIndex((m) => m.tipKey === tipKey);
        if (idx === -1) return prev;
        const updated = [...prev];
        updated[idx] = {
          ...updated[idx],
          message: `Tip attempt of ${dcrAmount} DCR failed due to ${msg}. Given up on attempting to tip.`,
          pending: false,
          tipKey: undefined,
        };
        return updated;
      });
    });
  }, []);

  // Lookup map for "do we already have the suggested user in our address
  // book?" — drives the SuggestedKXCard's idle/already-kxed state. Also
  // surfaces the contact's displayed nick (alias if any).
  const knownContactsByUid = useMemo(() => {
    const m = new Map<string, BisonrelayContact>();
    for (const c of contacts) {
      const id = c.id?.identity;
      if (id) m.set(id, c);
    }
    return m;
  }, [contacts]);

  // Lets MessageList resolve a GC sender's avatar from the nick carried on
  // each message (GC messages have no uid). Keyed by every name a sender
  // might appear under so known contacts still match.
  const contactByNick = useMemo(() => {
    const m = new Map<string, BisonrelayContact>();
    for (const c of contacts) {
      for (const k of [c.nick_alias, c.id?.nick, displayNick(c)]) {
        if (k && !m.has(k)) m.set(k, c);
      }
    }
    return m;
  }, [contacts]);

  // Unread DMs first, then alphabetical by display nick. The addressbook
  // itself arrives unordered (the BR library iterates a Go map).
  const sortedContacts = useMemo(() => {
    const hasUnread = (c: BisonrelayContact) => {
      const uid = c.id?.identity ?? '';
      return !!uid && notifPrefs.dms && (unread[uid] ?? 0) > 0;
    };
    return [...contacts].sort((a, b) => {
      const unreadDiff = Number(hasUnread(b)) - Number(hasUnread(a));
      if (unreadDiff !== 0) return unreadDiff;
      return displayNick(a).toLowerCase().localeCompare(displayNick(b).toLowerCase());
    });
  }, [contacts, unread, notifPrefs.dms]);

  // Partition the contact list into sidebar sections by group assignment
  // (uid-keyed). Assignments pointing at deleted groups fall back to the
  // regular list.
  const sectionedContacts = useMemo(() => {
    const assignments = contactGroups?.contacts ?? {};
    const customGroups = contactGroups?.groups ?? [];
    const known = new Set(customGroups.map((g) => g.id));
    const regular: BisonrelayContact[] = [];
    const archived: BisonrelayContact[] = [];
    const byGroup: Record<string, BisonrelayContact[]> = {};
    for (const c of sortedContacts) {
      const uid = c.id?.identity ?? '';
      const a = uid ? assignments[uid] : undefined;
      if (!a || (a.group !== ARCHIVED_GROUP_ID && !known.has(a.group))) {
        regular.push(c);
      } else if (a.group === ARCHIVED_GROUP_ID) {
        archived.push(c);
      } else {
        (byGroup[a.group] ??= []).push(c);
      }
    }
    return { regular, archived, byGroup, customGroups };
  }, [sortedContacts, contactGroups]);

  const sumUnread = (list: BisonrelayContact[]) =>
    notifPrefs.dms
      ? list.reduce((acc, c) => acc + (unread[c.id?.identity ?? ''] ?? 0), 0)
      : 0;

  const renderContactRow = (c: BisonrelayContact, dimmed = false) => {
    const nick = displayNick(c);
    const isSel =
      selectedContact?.id?.identity && c.id?.identity === selectedContact.id.identity;
    const uid = c.id?.identity ?? '';
    const count = uid && notifPrefs.dms ? unread[uid] ?? 0 : 0;
    const ignored = !!c.ignored;
    const heard = heardAge(c.last_dec_time);
    return (
      <div
        key={uid || nick}
        onClick={() => setSelected({ kind: 'contact', value: c })}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            setSelected({ kind: 'contact', value: c });
          }
        }}
        className={`w-full text-left px-3 py-2 rounded-md transition-colors text-sm flex items-center gap-2 cursor-pointer ${
          isSel ? 'bg-primary/20 text-foreground' : 'hover:bg-muted/30 text-muted-foreground'
        } ${ignored || dimmed ? 'opacity-50' : ''}`}
      >
        <span
          onClick={(e) => {
            e.stopPropagation();
            setSubNavContact(c);
          }}
          role="button"
          tabIndex={0}
          aria-label={`User actions for ${nick}`}
          className="inline-flex shrink-0 rounded-full hover:ring-2 hover:ring-primary/50 transition-shadow cursor-pointer"
        >
          <ContactAvatar contact={c} nick={nick} />
        </span>
        <span className="truncate flex-1">{nick}</span>
        {heard && (
          <span
            className="shrink-0 text-[10px] text-muted-foreground tabular-nums"
            title="Last message received"
          >
            {heard}
          </span>
        )}
        {ignored && (
          <EyeOff
            className="shrink-0 h-3.5 w-3.5 text-muted-foreground"
            aria-label="Ignored"
          />
        )}
        {count > 0 && (
          <span className="shrink-0 inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold">
            {count > 99 ? '99+' : count}
          </span>
        )}
      </div>
    );
  };

  useEffect(() => {
    return addListener((evt) => {
      // A contact changed profile fields (avatar/nick) or blocked us;
      // refetch so the sidebar and avatars reflect it live.
      if (evt.type === 'profile-updated' || evt.type === 'blocked-by-user') {
        refreshContacts();
        return;
      }
      if (evt.type === 'kx') {
        refreshContacts();
        // BR's client_kx.go:214 calls LogPM(internal=true) with "Completed
        // KX" on completion. Optimistically append the same line to the
        // open thread so the user sees feedback without a full refetch.
        const cur = selectedRef.current;
        if (cur && cur.id?.identity) {
          const fromUid = identityFromPayload(evt.payload ?? {});
          const matches = !fromUid || fromUid === cur.id.identity;
          if (matches) {
            setMessages((prev) => [
              ...prev,
              {
                message: 'Completed KX',
                from: '',
                timestamp: Math.floor(Date.now() / 1000),
                internal: true,
              },
            ]);
          }
        }
        return;
      }
      if (evt.type === 'tip-sent' || evt.type === 'tip-failed') {
        // Sender-side terminal events. If we have a pending placeholder
        // (from handleTip) for the open thread, replace its text in place
        // and drop the spinner — otherwise append fresh (e.g. tip was
        // initiated elsewhere).
        const payload = (evt.payload ?? {}) as Record<string, string>;
        const recipient = payload.recipient;
        const line = payload.line ?? '';
        if (!recipient || !line) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === recipient) {
          setMessages((prev) => {
            const idx = prev.findIndex((m) => m.pending && m.tipKey);
            if (idx !== -1) {
              const updated = [...prev];
              updated[idx] = {
                ...updated[idx],
                message: line,
                pending: false,
                tipKey: undefined,
              };
              return updated;
            }
            return [
              ...prev,
              {
                message: line,
                from: '',
                timestamp: Math.floor(Date.now() / 1000),
                internal: true,
              },
            ];
          });
        }
        return;
      }
      if (evt.type === 'tip-invoice-generated') {
        // The recipient's invoice arrived; upgrade the pending placeholder
        // text (the gap to the terminal tip-sent/tip-failed can otherwise
        // look stalled). Keep pending + tipKey so the terminal event still
        // replaces the same line.
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const uid = String(payload.uid ?? '');
        const nick = String(payload.nick ?? '');
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === uid) {
          setMessages((prev) => {
            const idx = prev.findIndex((m) => m.pending && m.tipKey);
            if (idx === -1) return prev;
            const updated = [...prev];
            updated[idx] = {
              ...updated[idx],
              message: `Invoice received, paying tip to ${nick}...`,
            };
            return updated;
          });
        }
        return;
      }
      if (evt.type === 'tip-received') {
        const payload = (evt.payload ?? {}) as Record<string, string>;
        const sender = payload.sender;
        const line = payload.line ?? '';
        if (!sender || !line) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === sender) {
          setMessages((prev) => [
            ...prev,
            {
              message: line,
              from: '',
              timestamp: Math.floor(Date.now() / 1000),
              internal: true,
            },
          ]);
        }
        return;
      }
      if (evt.type === 'posts-subscriber-updated') {
        // The remote user changed THEIR subscription to OUR posts (the
        // inverse of posts-subscribed). No local placeholder to swap;
        // just append the LogPM'd line in the matching thread.
        const payload = (evt.payload ?? {}) as Record<string, string>;
        const uid = payload.uid;
        const line = payload.line ?? '';
        if (!uid || !line) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === uid) {
          setMessages((prev) => [
            ...prev,
            {
              message: line,
              from: '',
              timestamp: Math.floor(Date.now() / 1000),
              internal: true,
            },
          ]);
        }
        return;
      }
      if (evt.type === 'posts-subscribed' || evt.type === 'posts-unsubscribed') {
        // The remote user confirmed our subscribe/unsubscribe request.
        // refreshContacts picks up the new posts_subscribed flag. If we
        // have an optimistic placeholder for this contact, swap its text
        // in place + drop the spinner; otherwise append a fresh line
        // (covers remote-initiated changes from other sessions).
        const payload = (evt.payload ?? {}) as Record<string, string>;
        const uid = payload.uid;
        const line = payload.line ?? '';
        if (!uid || !line) return;
        refreshContacts();
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === uid) {
          setMessages((prev) => {
            const idx = prev.findIndex((m) => m.pending && m.subKey);
            if (idx !== -1) {
              const updated = [...prev];
              updated[idx] = {
                ...updated[idx],
                message: line,
                pending: false,
                subKey: undefined,
              };
              return updated;
            }
            return [
              ...prev,
              {
                message: line,
                from: '',
                timestamp: Math.floor(Date.now() / 1000),
                internal: true,
              },
            ];
          });
        }
        return;
      }
      if (evt.type === 'kx-suggested') {
        // brclientd publishes this from its OnKXSuggested handler, having
        // already LogPM'd the matching "Suggested KX to ..." line so it
        // shows up on history scroll. We optimistically append the same
        // text to the open thread when the suggestion targets us via the
        // current contact (the invitee), so it renders without waiting
        // for a reload.
        const payload = (evt.payload ?? {}) as Record<string, string>;
        const invitee = payload.invitee ?? '';
        const target = payload.target ?? '';
        const targetNick = payload.targetNick ?? '';
        if (!invitee || !target) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === invitee) {
          const line = `Suggested KX to ${target} "${targetNick}"`;
          setMessages((prev) => [
            ...prev,
            {
              message: line,
              from: '',
              timestamp: Math.floor(Date.now() / 1000),
              internal: true,
            },
          ]);
        }
        return;
      }
      if (
        evt.type === 'file-invoice-capacity-low' ||
        evt.type === 'invoice-gen-failed' ||
        evt.type === 'posts-subscribe-error' ||
        evt.type === 'idle-unsubscribing'
      ) {
        // brclientd LogPM'd the same text into the user's thread (like
        // kx-suggested); append it optimistically when that thread is open.
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const uid = String(payload.uid ?? '');
        const line = String(payload.text ?? '');
        if (!uid || !line) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === uid) {
          setMessages((prev) => [
            ...prev,
            {
              message: line,
              from: '',
              timestamp: Math.floor(Date.now() / 1000),
              internal: true,
            },
          ]);
        }
        return;
      }
      if (evt.type === 'pm-delivered') {
        // The relay server acked one of our outbound PMs; upgrade the tick
        // on the first matching queued message in the open thread (FIFO
        // for identical texts).
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const uid = String(payload.uid ?? '');
        const text = String(payload.message ?? '');
        const cur = selectedRef.current;
        if (cur && cur.id?.identity === uid && text) {
          setMessages((prev) => {
            const idx = prev.findIndex(
              (m) => m.delivered === false && !m.internal && m.message === text,
            );
            if (idx === -1) return prev;
            const updated = [...prev];
            updated[idx] = { ...updated[idx], delivered: true };
            return updated;
          });
        }
        return;
      }
      if (evt.type === 'pm') {
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const fromUid = String(payload.from ?? '');
        const senderNick = String(payload.fromNick ?? '');
        const text = String(payload.message ?? '');
        const cur = selectedRef.current;
        if (cur && cur.id?.identity && fromUid === cur.id.identity) {
          // brclientd timestamps the event on the bus; use it when present,
          // falling back to client time.
          let ts = Math.floor(Date.now() / 1000);
          const evtTs = (evt as { timestamp?: string }).timestamp;
          if (evtTs) {
            const parsed = Date.parse(evtTs);
            if (Number.isFinite(parsed)) ts = Math.floor(parsed / 1000);
          }
          setMessages((prev) => [
            ...prev,
            {
              message: text,
              from: senderNick,
              timestamp: ts,
              internal: false,
            },
          ]);
        }
        return;
      }
      if (evt.type === 'gc-message') {
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const gcid = String(payload.gcid ?? '');
        const senderNick = String(payload.fromNick ?? '');
        const text = String(payload.message ?? '');
        if (!gcid || !text) return;
        const cur = selectedGroupRef.current;
        if (cur && cur.id === gcid) {
          // brclientd timestamps the event on the bus; we use it when
          // present, falling back to client time.
          let ts = Math.floor(Date.now() / 1000);
          const evtTs = (evt as { timestamp?: string }).timestamp;
          if (evtTs) {
            const parsed = Date.parse(evtTs);
            if (Number.isFinite(parsed)) ts = Math.floor(parsed / 1000);
          }
          setMessages((prev) => [
            ...prev,
            {
              message: text,
              from: senderNick,
              timestamp: ts,
              internal: false,
            },
          ]);
        }
        return;
      }
      if (evt.type === 'download') {
        const payload = evt.payload ?? {};
        const senderNick = payload.nick ?? '';
        const fileMeta = payload.fileMetadata ?? payload.file_metadata ?? {};
        const filename = fileMeta.filename ?? '';
        const size = Number(fileMeta.size ?? 0);
        const fromUid = identityFromPayload(payload);
        if (!filename) return;
        const cur = selectedRef.current;
        if (cur && cur.id?.identity && fromUid === cur.id.identity) {
          setMessages((prev) => [
            ...prev,
            {
              message: buildDownloadTag(senderNick, filename, size, ''),
              from: senderNick,
              timestamp: Math.floor(Date.now() / 1000),
              internal: false,
            },
          ]);
        }
        return;
      }
      // GC structural state events: refresh the sidebar so names, member
      // counts and admin/membership flags stay current, and surface a system
      // message in the open thread (own removal / dissolve prominently, and
      // observed member changes as lighter lines).
      if (
        evt.type === 'gc-joined' ||
        evt.type === 'gc-killed' ||
        evt.type === 'gc-parted' ||
        evt.type === 'gc-members-added' ||
        evt.type === 'gc-members-removed' ||
        evt.type === 'gc-admins-changed' ||
        evt.type === 'gc-upgraded'
      ) {
        refreshGCs();
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        const gcid = String(payload.gcid ?? '');
        const cur = selectedGroupRef.current;
        const isOpen = !!cur && cur.id === gcid;
        const nickFor = (uid: string) => {
          const c = contactsRef.current.find(
            (x) => x.id?.identity?.toLowerCase() === uid.toLowerCase()
          );
          return c?.nick_alias || c?.id?.nick || `${uid.slice(0, 8)}…`;
        };
        const sys = (text: string) =>
          setMessages((prev) => [
            ...prev,
            { message: text, from: '', timestamp: Math.floor(Date.now() / 1000), internal: true },
          ]);
        const uidList = (v: unknown) => (Array.isArray(v) ? (v as unknown[]).map(String) : []);

        if (evt.type === 'gc-parted') {
          // Own removal while actively viewing the group blanks the thread out
          // with a prominent notice (sending would now error); observed parts
          // are just a system line.
          if (payload.self) {
            if (isOpen) {
              setKickNotice({
                gcid,
                kind: 'kicked',
                name: String(payload.gcName ?? ''),
                reason: String(payload.reason ?? ''),
              });
            }
          } else if (isOpen) {
            sys(
              `${nickFor(String(payload.uid ?? ''))} ${payload.kicked ? 'was removed from' : 'left'} the group`
            );
          }
        } else if (evt.type === 'gc-killed') {
          // The GC is torn down locally - blank the open thread with the notice.
          if (isOpen) {
            setKickNotice({
              gcid,
              kind: 'dissolved',
              name: String(payload.gcName ?? ''),
              by: String(payload.byNick ?? ''),
              reason: String(payload.reason ?? ''),
            });
          }
        } else if (evt.type === 'gc-members-added') {
          if (isOpen) uidList(payload.added).forEach((u) => sys(`${nickFor(u)} joined the group`));
        } else if (evt.type === 'gc-members-removed') {
          // self removal is announced via gc-parted; only show others here.
          if (isOpen && !payload.self) {
            uidList(payload.removed).forEach((u) => sys(`${nickFor(u)} was removed from the group`));
          }
        } else if (evt.type === 'gc-admins-changed') {
          if (isOpen) {
            uidList(payload.added).forEach((u) => sys(`${nickFor(u)} is now an admin`));
            uidList(payload.removed).forEach((u) => sys(`${nickFor(u)} is no longer an admin`));
          }
        }
      }
    });
  }, [addListener, refreshContacts]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selected || sending) return;
    if (!draft.trim() && !attachment) return;
    const text = draft.trim();
    setSending(true);
    try {
      if (selected.kind === 'group') {
        // GC send. File-transfer-mode attachments are not supported in v1
        // because BR's shared-file mechanism is per-peer, not broadcast;
        // bruig doesn't ship GC file-transfer either. Inline embeds work
        // because they ride in the body string. Phase 6 adds the inline
        // path; for now PMs-only file send.
        if (attachment && attachment.mode === 'transfer') {
          setMessagesErr('File transfer in groups is not yet supported.');
          return;
        }
        const embed: BisonrelayPMAttachment | undefined =
          attachment && attachment.mode === 'inline' && attachment.dataB64
            ? {
                name: attachment.file.name,
                mime: attachment.file.type || 'application/octet-stream',
                data_b64: attachment.dataB64,
              }
            : undefined;
        const result = await sendBisonrelayGCMessage(selected.value.id, text, embed);
        // Sender's own GC message is logged immediately by BR on the
        // backend (no notification loopback). Optimistic insert mirrors
        // that exactly, internal: false so it renders as own bubble.
        setMessages((prev) => [
          ...prev,
          {
            message: result.body || text,
            from: ownNick,
            timestamp: Math.floor(Date.now() / 1000),
            internal: false,
          },
        ]);
        setDraft('');
        setAttachment(null);
        setAttachErr(null);
        return;
      }

      const recipient = nickOrUid(selected.value);
      if (attachment && attachment.mode === 'transfer') {
        if (text) {
          await sendBisonrelayPM(recipient, text);
          setMessages((prev) => [
            ...prev,
            {
              message: text,
              from: ownNick,
              timestamp: Math.floor(Date.now() / 1000),
              internal: false,
              delivered: false,
            },
          ]);
        }
        await sendBisonrelayFile(recipient, attachment.file);
        setMessages((prev) => [
          ...prev,
          {
            message: `Sent file "${attachment.file.name}"`,
            from: ownNick,
            timestamp: Math.floor(Date.now() / 1000),
            internal: true,
          },
        ]);
      } else {
        const embed: BisonrelayPMAttachment | undefined =
          attachment && attachment.mode === 'inline' && attachment.dataB64
            ? {
                name: attachment.file.name,
                mime: attachment.file.type || 'application/octet-stream',
                data_b64: attachment.dataB64,
              }
            : undefined;
        const result = await sendBisonrelayPM(recipient, text, embed);
        setMessages((prev) => [
          ...prev,
          {
            message: result.body || text,
            from: ownNick,
            timestamp: Math.floor(Date.now() / 1000),
            internal: false,
            delivered: false,
          },
        ]);
      }
      setDraft('');
      setAttachment(null);
      setAttachErr(null);
    } catch (err: any) {
      const body = err?.response?.data;
      setMessagesErr(typeof body === 'string' ? body : err?.message || 'Send failed');
    } finally {
      setSending(false);
    }
  };

  const handleAttachPick = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    if (f.size > MAX_TRANSFER_BYTES) {
      setAttachErr(`File is ${formatBytes(f.size)}. Maximum is ${formatBytes(MAX_TRANSFER_BYTES)}.`);
      return;
    }
    // Images go through the preview/compress modal; compression can bring a
    // too-large photo under the inline cap that would otherwise force file
    // transfer. SVG and non-images keep the direct path.
    if (isCompressibleImage(f.type)) {
      setPendingImage(f);
      setAttachErr(null);
      return;
    }
    const inlineable = isImageMime(f.type) && f.size <= MAX_INLINE_BYTES;
    if (!inlineable) {
      setAttachment({ file: f, mode: 'transfer' });
      setAttachErr(null);
      return;
    }
    try {
      const buf = await f.arrayBuffer();
      const bytes = new Uint8Array(buf);
      let binStr = '';
      const chunk = 0x8000;
      for (let i = 0; i < bytes.length; i += chunk) {
        binStr += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk) as unknown as number[]);
      }
      const dataB64 = btoa(binStr);
      setAttachment({ file: f, mode: 'inline', dataB64 });
      setAttachErr(null);
    } catch (err: any) {
      setAttachErr(err?.message || 'Could not read file');
    }
  };

  // The send path reads name/mime from attachment.file, so the compressed
  // variant is wrapped into a File carrying its JPEG mime; an over-cap pick
  // keeps today's behavior and falls back to file-transfer mode.
  const handleImageAttach = (r: ImageAttachResult) => {
    const file =
      r.blob instanceof File ? r.blob : new File([r.blob], r.name, { type: r.mime });
    if (r.size <= MAX_INLINE_BYTES) {
      setAttachment({ file, mode: 'inline', dataB64: r.dataB64 });
    } else {
      setAttachment({ file, mode: 'transfer' });
    }
    setAttachErr(null);
    setPendingImage(null);
  };

  // Splice an emoji into the draft at the textarea cursor, then restore focus
  // with the caret just past the inserted character so several can be added.
  const insertEmojiAtCursor = (emoji: string) => {
    const el = draftInputRef.current;
    if (!el) {
      setDraft((d) => d + emoji);
      return;
    }
    const start = el.selectionStart ?? draft.length;
    const end = el.selectionEnd ?? draft.length;
    setDraft(draft.slice(0, start) + emoji + draft.slice(end));
    queueMicrotask(() => {
      const pos = start + emoji.length;
      el.focus();
      el.setSelectionRange(pos, pos);
    });
  };

  // wrapDraftSelection wraps the composer's current selection (or inserts the
  // placeholder when nothing is selected) with markdown delimiters - the
  // actions behind the chat format menu. Mirrors the editor's wrapSelection.
  const wrapDraftSelection = (left: string, right: string, placeholder: string) => {
    const el = draftInputRef.current;
    if (!el) {
      setDraft((d) => d + left + placeholder + right);
      return;
    }
    const start = el.selectionStart ?? draft.length;
    const end = el.selectionEnd ?? draft.length;
    const content = draft.slice(start, end) || placeholder;
    setDraft(draft.slice(0, start) + left + content + right + draft.slice(end));
    queueMicrotask(() => {
      el.focus();
      const innerStart = start + left.length;
      el.setSelectionRange(innerStart, innerStart + content.length);
    });
  };

  return (
    <ImageViewerCtx.Provider value={openImageViewer}>
    <div className="flex flex-col">
      <IncomingGCInvitesBanner onAccepted={refreshGCs} />
      {pendingImage && (
        <ImageAttachModal
          file={pendingImage}
          maxInlineBytes={MAX_INLINE_BYTES}
          allowOversized
          oversizedHint="Will be sent as a file transfer."
          showAlt={false}
          onCancel={() => setPendingImage(null)}
          onAttach={handleImageAttach}
        />
      )}
      {showTip && selectedContact && (
        <TipModal
          nick={displayNick(selectedContact)}
          uid={selectedContact.id?.identity ?? ''}
          onClose={() => setShowTip(false)}
          onSubmit={(dcr) =>
            handleTip(selectedContact.id?.identity ?? '', displayNick(selectedContact), dcr)
          }
        />
      )}
    {showGroupMgmt && <GroupManagementModal onClose={() => setShowGroupMgmt(false)} />}
    <div className="relative flex gap-4 h-[calc(100dvh-9.5rem)] min-h-[320px] md:h-[calc(100vh-12rem)] md:min-h-[480px]">
      <aside className={`${selected ? 'hidden md:flex' : 'flex'} w-full md:w-72 flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50`}>
        <div className="p-3 border-b border-border/50 flex items-center justify-between">
          <h3 className="text-sm font-semibold">Chats</h3>
          <div className="flex gap-1">
            <button
              onClick={() => setShowInviteCreate(true)}
              className="p-1.5 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
              title="Create invite"
            >
              <UserPlus className="h-4 w-4" />
            </button>
            <button
              onClick={() => setShowInviteAccept(true)}
              className="p-1.5 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
              title="Import invite"
            >
              <MessageSquare className="h-4 w-4" />
            </button>
            <button
              onClick={() => setShowGroupMgmt(true)}
              className="p-1.5 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
              title="Manage contact groups"
            >
              <FolderCog className="h-4 w-4" />
            </button>
            {!inDecredPulse && (
              <button
                onClick={() => setShowJoinDecredPulse(true)}
                className="p-1.5 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
                title="Join Decred chat networks"
              >
                <Users className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>
        <div className="flex-1 overflow-y-auto p-2">
          <SidebarSectionHeader
            label="Contacts"
            count={sectionedContacts.regular.length}
            collapsed={contactsCollapsed}
            onToggle={() => setContactsCollapsed((v) => !v)}
            unread={sumUnread(sectionedContacts.regular)}
          />
          {!contactsCollapsed && (
            <div className="space-y-1 mb-2">
              {contactsErr && (
                <p className="text-xs text-destructive p-2">{contactsErr}</p>
              )}
              {contacts.length === 0 && !contactsErr && (
                <p className="text-xs text-muted-foreground p-2">
                  No contacts yet. Create an invite and share it out-of-band, or
                  paste a peer's invite to start a key exchange.
                </p>
              )}
              {sectionedContacts.regular.map((c) => renderContactRow(c))}
            </div>
          )}

          {sectionedContacts.customGroups.map((g) => {
            const members = sectionedContacts.byGroup[g.id] ?? [];
            return (
              <Fragment key={g.id}>
                <SidebarSectionHeader
                  label={g.name}
                  count={members.length}
                  collapsed={!!sectionCollapsed[g.id]}
                  onToggle={() =>
                    setSectionCollapsed((prev) => ({ ...prev, [g.id]: !prev[g.id] }))
                  }
                  unread={sumUnread(members)}
                />
                {!sectionCollapsed[g.id] && (
                  <div className="space-y-1 mb-2">
                    {members.map((c) => renderContactRow(c))}
                  </div>
                )}
              </Fragment>
            );
          })}

          {sectionedContacts.archived.length > 0 && (
            <>
              <SidebarSectionHeader
                label="Archived"
                count={sectionedContacts.archived.length}
                collapsed={!!sectionCollapsed[ARCHIVED_GROUP_ID]}
                onToggle={() =>
                  setSectionCollapsed((prev) => ({
                    ...prev,
                    [ARCHIVED_GROUP_ID]: !prev[ARCHIVED_GROUP_ID],
                  }))
                }
                unread={sumUnread(sectionedContacts.archived)}
              />
              {!sectionCollapsed[ARCHIVED_GROUP_ID] && (
                <div className="space-y-1 mb-2">
                  {sectionedContacts.archived.map((c) => renderContactRow(c, true))}
                </div>
              )}
            </>
          )}

          <SidebarSectionHeader
            label="Groups"
            count={gcs.length}
            collapsed={groupsCollapsed}
            onToggle={() => setGroupsCollapsed((v) => !v)}
            actionLabel="Create group"
            onAction={() => setShowCreateGC(true)}
            unread={
              notifPrefs.gcMessages
                ? gcs.reduce((acc, g) => acc + (gcUnread[g.id] ?? 0), 0)
                : 0
            }
          />
          {!groupsCollapsed && (
            <div className="space-y-1">
              {gcsErr && <p className="text-xs text-destructive p-2">{gcsErr}</p>}
              {gcs.length === 0 && !gcsErr && (
                <p className="text-xs text-muted-foreground p-2">
                  No groups yet. Group create / invite UX lands in a follow-up
                  phase; for now any GC you join via another client will show
                  up here.
                </p>
              )}
              {gcs.map((g) => {
                const label = g.alias || g.name;
                const isSel = selectedGroup?.id === g.id;
                const count = notifPrefs.gcMessages ? gcUnread[g.id] ?? 0 : 0;
                return (
                  <div
                    key={g.id}
                    onClick={() => setSelected({ kind: 'group', value: g })}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        setSelected({ kind: 'group', value: g });
                      }
                    }}
                    className={`w-full text-left px-3 py-2 rounded-md transition-colors text-sm flex items-center gap-2 cursor-pointer ${
                      isSel ? 'bg-primary/20 text-foreground' : 'hover:bg-muted/30 text-muted-foreground'
                    }`}
                  >
                    <span className="inline-flex shrink-0 h-7 w-7 rounded-full bg-muted/40 items-center justify-center text-[10px] font-semibold uppercase">
                      {label.slice(0, 2)}
                    </span>
                    <span
                      className={`truncate flex-1 ${
                        g.local_is_member === false ? 'line-through opacity-60' : ''
                      }`}
                      title={g.local_is_member === false ? 'You were removed from this group' : undefined}
                    >
                      {label}
                    </span>
                    {count > 0 ? (
                      <span className="shrink-0 inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold">
                        {count > 99 ? '99+' : count}
                      </span>
                    ) : (
                      <span className="shrink-0 text-[10px] text-muted-foreground tabular-nums">
                        {g.members.length}
                      </span>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </aside>

      <section className={`${selected ? 'flex max-md:fixed max-md:inset-0 max-md:z-20 max-md:rounded-none max-md:border-0 max-md:bg-background' : 'hidden md:flex'} relative flex-1 min-w-0 flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50`}>
        {kickNotice && selectedGroup?.id === kickNotice.gcid && (
          <div className="absolute inset-0 z-30 flex flex-col items-center justify-center gap-4 rounded-xl bg-background/95 backdrop-blur-sm p-6 text-center">
            <div className="h-16 w-16 rounded-full bg-rose-500/15 border border-rose-500/40 flex items-center justify-center">
              <AlertCircle className="h-8 w-8 text-rose-400" />
            </div>
            <h3 className="text-xl font-bold">
              {kickNotice.kind === 'dissolved' ? 'Group dissolved' : 'You were removed'}
            </h3>
            <p className="max-w-sm text-sm text-muted-foreground">
              {kickNotice.kind === 'dissolved'
                ? `"${kickNotice.name || 'This group'}" was dissolved${kickNotice.by ? ` by ${kickNotice.by}` : ''}.`
                : `You were removed from "${kickNotice.name || 'this group'}".`}
              {kickNotice.reason ? ` Reason: ${kickNotice.reason}` : ''}
            </p>
            <button
              type="button"
              onClick={() => {
                setKickNotice(null);
                setSelected(null);
              }}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold"
            >
              Close
            </button>
          </div>
        )}
        {!selected ? (
          <EmptyThread
            onCreate={() => setShowInviteCreate(true)}
            onAccept={() => setShowInviteAccept(true)}
            onJoin={inDecredPulse ? undefined : () => setShowJoinDecredPulse(true)}
          />
        ) : (
          <>
            <header className="p-3 border-b border-border/50 flex items-center gap-2">
              <button
                type="button"
                onClick={() => setSelected(null)}
                className="md:hidden shrink-0 p-1 -ml-1 rounded text-muted-foreground hover:text-foreground"
                aria-label="Back to chats"
              >
                <ChevronLeft className="h-5 w-5" />
              </button>
              <div className="flex-1 min-w-0">
                {selected.kind === 'contact' ? (
                  <div className="min-w-0">
                    <h3 className="text-sm font-semibold truncate">{displayNick(selected.value)}</h3>
                    {selected.value.id?.identity && (
                      <p className="text-[10px] text-muted-foreground font-mono truncate mt-0.5">
                        {selected.value.id.identity}
                      </p>
                    )}
                  </div>
                ) : (
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <h3 className="text-sm font-semibold flex items-center gap-2 flex-wrap">
                        <span>{selected.value.alias || selected.value.name}</span>
                        <span className="text-[10px] text-muted-foreground tabular-nums font-normal">
                          {selected.value.members.length} members
                        </span>
                        {selected.value.local_is_owner && (
                          <span className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-primary/15 text-primary">
                            Owner
                          </span>
                        )}
                      </h3>
                      <p className="text-[10px] text-muted-foreground font-mono truncate mt-0.5">
                        {selected.value.id}
                      </p>
                    </div>
                    <div className="shrink-0 flex items-center gap-1">
                      {selected.value.local_is_admin && (
                        <button
                          type="button"
                          onClick={() => setShowGCInvite(true)}
                          className="px-2.5 py-1 rounded-md text-[11px] border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1"
                          title="Invite a contact"
                        >
                          <UserPlus className="h-3 w-3" /> Invite
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => setShowGroupSubNav(true)}
                        className="px-2.5 py-1 rounded-md text-[11px] border border-border/50 text-foreground hover:bg-muted/30 inline-flex items-center gap-1"
                        title="Manage group"
                      >
                        Manage
                      </button>
                    </div>
                  </div>
                )}
              </div>
            </header>
            <div className="flex-1 overflow-y-auto overscroll-contain p-3 space-y-2">
              {messagesLoading ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Loading history…</span>
                </div>
              ) : messagesErr ? (
                <div className="flex items-start gap-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                  <span>{messagesErr}</span>
                </div>
              ) : messages.length === 0 ? (
                <p className="text-xs text-muted-foreground">No messages yet.</p>
              ) : (
                <MessageList
                  messages={messages}
                  ownNick={ownNick}
                  isGroup={selected?.kind === 'group'}
                  mediatorUid={selectedContact?.id?.identity ?? ''}
                  knownContactsByUid={knownContactsByUid}
                  contactByNick={contactByNick}
                  onOpenContact={setSubNavContact}
                  onAcceptSuggestion={handleAcceptSuggestion}
                />
              )}
            </div>
            {selectedGroupRemoved ? (
              <div className="p-3 border-t border-border/50 text-center text-xs italic text-muted-foreground">
                You're no longer a member of this group.
              </div>
            ) : (
            <form onSubmit={handleSend} className="p-3 border-t border-border/50 flex flex-col gap-2">
              {attachment && (
                <AttachmentPreview
                  attachment={attachment}
                  onRemove={() => {
                    setAttachment(null);
                    setAttachErr(null);
                  }}
                />
              )}
              {attachErr && (
                <p className="text-xs text-destructive flex items-center gap-1">
                  <AlertCircle className="h-3 w-3" /> {attachErr}
                </p>
              )}
              <div className="flex items-end gap-1 rounded-2xl border border-border bg-background px-2 py-1 transition-colors focus-within:border-primary">
                <input
                  ref={fileInputRef}
                  type="file"
                  className="hidden"
                  onChange={handleAttachPick}
                />
                <EmojiPicker onPick={insertEmojiAtCursor} disabled={sending} />
                <textarea
                  ref={draftInputRef}
                  rows={1}
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                      e.preventDefault();
                      e.currentTarget.form?.requestSubmit();
                    }
                  }}
                  placeholder={
                    selectedContact ? `Message ${displayNick(selectedContact)}…` : 'Type a message…'
                  }
                  disabled={sending}
                  className="flex-1 min-w-0 px-1 py-1.5 bg-transparent text-foreground leading-normal resize-none overflow-y-auto max-h-[9rem] focus:outline-none disabled:opacity-50"
                />
                <ChatFormatMenu onWrap={wrapDraftSelection} disabled={sending} />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={sending}
                  title="Attach a file"
                  aria-label="Attach a file"
                  className="shrink-0 p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-50"
                >
                  <Paperclip className="h-4 w-4" />
                </button>
                {selectedContact && (
                  <button
                    type="button"
                    onClick={() => setShowTip(true)}
                    disabled={sending}
                    title={`Pay a tip to ${displayNick(selectedContact)}`}
                    aria-label="Pay tip"
                    className="shrink-0 p-2 rounded-lg text-primary hover:bg-primary/10 transition-colors disabled:opacity-50"
                  >
                    <Zap className="h-4 w-4" />
                  </button>
                )}
                <button
                  type="submit"
                  disabled={(!draft.trim() && !attachment) || sending}
                  title="Send"
                  aria-label="Send"
                  className="shrink-0 p-2 rounded-lg text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                </button>
              </div>
            </form>
            )}
          </>
        )}
      </section>

      {subNavContact && (
        <BisonrelayUserSubNav
          contact={subNavContact}
          nick={displayNick(subNavContact)}
          contacts={contacts}
          displayNick={displayNick}
          onClose={() => setSubNavContact(null)}
          onSendFile={() => {
            setSelected({ kind: 'contact', value: subNavContact });
            setSubNavContact(null);
            fileInputRef.current?.click();
          }}
          onRenamed={(newNick) => {
            const uid = subNavContact.id?.identity;
            setSubNavContact(null);
            refreshContacts();
            // Optimistically patch the selected contact's alias so the chat
            // header updates without waiting for refreshContacts to land.
            if (uid && selectedContact?.id?.identity === uid) {
              setSelected({
                kind: 'contact',
                value: { ...selectedContact, nick_alias: newNick },
              });
            }
          }}
          onTip={(uid, nick, dcr) => {
            // Make sure the thread is open before the placeholder appears.
            setSelected({ kind: 'contact', value: subNavContact });
            handleTip(uid, nick, dcr);
          }}
          onSubscribePosts={(uid, nick) => {
            setSelected({ kind: 'contact', value: subNavContact });
            handleSubscribePosts('subscribe', uid, nick);
          }}
          onUnsubscribePosts={(uid, nick) => {
            setSelected({ kind: 'contact', value: subNavContact });
            handleSubscribePosts('unsubscribe', uid, nick);
          }}
          onContactsChanged={() => {
            // Block removes the contact; ignore flips its flag. If the changed
            // contact is the open thread, drop the selection so we don't keep
            // a stale/removed thread on screen.
            const changed = subNavContact.id?.identity;
            if (changed && selectedContact?.id?.identity === changed) {
              setSelected(null);
            }
            refreshContacts();
          }}
          onHistoryCleared={(clearedUid) => {
            // History wiped on disk; if this is the open thread, reload it so
            // the now-empty conversation is reflected immediately.
            if (clearedUid && selectedContact?.id?.identity === clearedUid) {
              loadMessages(subNavContact);
            }
          }}
        />
      )}
      {showJoinDecredPulse && (
        <JoinDecredPulseModal
          onClose={() => setShowJoinDecredPulse(false)}
          onJoined={refreshGCs}
        />
      )}
      {showInviteCreate && <InviteCreateModal onClose={() => setShowInviteCreate(false)} />}
      {showInviteAccept && (
        <InviteAcceptModal
          onClose={() => setShowInviteAccept(false)}
          onAccepted={refreshContacts}
        />
      )}
      {viewerImage && (
        <ImageViewerModal image={viewerImage} onClose={() => setViewerImage(null)} />
      )}
      {showCreateGC && (
        <CreateGCModal
          onClose={() => setShowCreateGC(false)}
          onCreated={(gc) => {
            setShowCreateGC(false);
            refreshGCs();
            setSelected({ kind: 'group', value: gc });
          }}
        />
      )}
      {showGCInvite && selectedGroup && (
        <GCInviteModal
          gc={selectedGroup}
          onClose={() => setShowGCInvite(false)}
          onInvited={() => setShowGCInvite(false)}
        />
      )}
      {showGroupSubNav && selectedGroup && (
        <GroupSubNav
          gc={selectedGroup}
          contactsByUid={knownContactsByUid}
          ownUid={ownUid}
          ownNick={ownNick}
          onClose={() => setShowGroupSubNav(false)}
          onMutated={refreshGCs}
          onPartedOrKilled={() => {
            setShowGroupSubNav(false);
            setSelected(null);
            refreshGCs();
          }}
        />
      )}
    </div>
    </div>
    </ImageViewerCtx.Provider>
  );
};

// Matches BR's clientdb SuggestedKXLogMsg format ("Suggested KX to %s %q"
// in fscdb.go:96). brclientd writes the same string for v0.2.4 since v0.2.4
// doesn't auto-log; later BR versions write it natively. Capture group 1
// is the 64-hex target identity, group 2 is the target nick.
const SUGGESTED_KX_RE = /^Suggested KX to ([0-9a-f]{64}) "(.*)"$/;

interface MessageListProps {
  messages: BisonrelayMessage[];
  ownNick: string;
  isGroup: boolean;
  mediatorUid: string;
  knownContactsByUid: Map<string, BisonrelayContact>;
  contactByNick: Map<string, BisonrelayContact>;
  onOpenContact: (c: BisonrelayContact) => void;
  onAcceptSuggestion: (target: string, targetNick: string) => Promise<void>;
}

// Day labels for the chat date separators: Today / Yesterday / a full date.
function startOfDay(ts: number): number {
  const d = new Date(ts * 1000);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

function formatDayLabel(ts: number): string {
  const today = startOfDay(Math.floor(Date.now() / 1000));
  const day = startOfDay(ts);
  const dayMs = 86400000;
  if (day === today) return 'Today';
  if (day === today - dayMs) return 'Yesterday';
  return toYMD(ts * 1000);
}

// SidebarSectionHeader is the collapsible header for the Contacts / Groups
// sections in the chat sidebar. Single click toggles. Persists state to
// localStorage via the parent's effect on contactsCollapsed/groupsCollapsed.
// Optional action button (e.g. "+ Create group") sits between count and
// toggle, doesn't propagate to the toggle.
const SidebarSectionHeader = ({
  label,
  count,
  collapsed,
  onToggle,
  actionLabel,
  onAction,
  unread = 0,
}: {
  label: string;
  count: number;
  collapsed: boolean;
  onToggle: () => void;
  actionLabel?: string;
  onAction?: () => void;
  unread?: number;
}) => (
  <div className="w-full px-2 py-1.5 mb-1 flex items-center gap-2 text-[10px] uppercase tracking-wide text-muted-foreground">
    <button
      type="button"
      onClick={onToggle}
      className="flex-1 flex items-center gap-2 hover:text-foreground transition-colors text-left"
    >
      <span className="inline-flex items-center justify-center w-3">
        {collapsed ? '▸' : '▾'}
      </span>
      <span className={`flex-1 ${unread > 0 ? 'text-foreground font-semibold' : ''}`}>
        {label}
      </span>
      {unread > 0 && (
        <span className="inline-flex items-center justify-center min-w-[1.1rem] h-4 px-1 rounded-full bg-primary text-primary-foreground text-[9px] font-semibold normal-case">
          {unread > 99 ? '99+' : unread}
        </span>
      )}
      <span className="tabular-nums">{count}</span>
    </button>
    {onAction && (
      <button
        type="button"
        onClick={onAction}
        title={actionLabel}
        className="p-0.5 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors text-sm leading-none"
      >
        +
      </button>
    )}
  </div>
);

const MessageList = ({
  messages,
  ownNick,
  isGroup,
  mediatorUid,
  knownContactsByUid,
  contactByNick,
  onOpenContact,
  onAcceptSuggestion,
}: MessageListProps) => {
  const endRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages.length]);
  return (
    <>
      {messages.map((m, i) => {
        const prev = i > 0 ? messages[i - 1] : undefined;
        const showDaySeparator = !prev || startOfDay(prev.timestamp) !== startOfDay(m.timestamp);
        const daySeparator = showDaySeparator ? (
          <div className="flex justify-center my-2">
            <span className="text-[10px] uppercase tracking-wide text-muted-foreground bg-muted/30 rounded-full px-2 py-0.5">
              {formatDayLabel(m.timestamp)}
            </span>
          </div>
        ) : null;

        // Internal log entries (BR writes "Completed KX", "Subscribed to
        // user's posts!" etc. via LogPM(internal=true)) render as centered
        // protocol notices, not chat bubbles. Mirrors bruig's chat events.
        if (m.internal) {
          const sugg = m.message.match(SUGGESTED_KX_RE);
          if (sugg) {
            const targetUid = sugg[1];
            const known = knownContactsByUid.get(targetUid);
            return (
              <Fragment key={i}>
                {daySeparator}
                <SuggestedKXCard
                  mediatorUid={mediatorUid}
                  targetUid={targetUid}
                  targetNick={sugg[2]}
                  timestamp={m.timestamp}
                  known={known}
                  onAccept={onAcceptSuggestion}
                />
              </Fragment>
            );
          }
          return (
            <Fragment key={i}>
              {daySeparator}
              <div className="flex justify-center py-0.5">
                <p className="text-[11px] italic text-muted-foreground/80 px-3 text-center inline-flex items-center gap-1.5 max-w-full">
                  {m.pending && <Loader2 className="h-3 w-3 animate-spin shrink-0" />}
                  <span className="min-w-0 break-words">{m.message}</span>
                  <span className="mx-1 opacity-50">·</span>
                  <span className="opacity-70 shrink-0">{toYMDTime(new Date(m.timestamp * 1000))}</span>
                </p>
              </div>
            </Fragment>
          );
        }
        const own = m.from === ownNick;
        // A run is a streak of consecutive bubbles from the same sender; only
        // the first carries the avatar + nick so repeats read cleanly.
        const startOfRun =
          !prev ||
          prev.internal ||
          prev.from !== m.from ||
          showDaySeparator ||
          m.timestamp - prev.timestamp > 300;
        const sender = !own
          ? isGroup
            ? contactByNick.get(m.from)
            : knownContactsByUid.get(mediatorUid)
          : undefined;
        return (
          <Fragment key={i}>
            {daySeparator}
            <div
              className={`flex ${own ? 'justify-end' : 'justify-start gap-2 items-end'} ${
                startOfRun ? 'mt-2' : 'mt-0.5'
              }`}
            >
              {!own &&
                (startOfRun ? (
                  sender ? (
                    <span
                      onClick={() => onOpenContact(sender)}
                      role="button"
                      tabIndex={0}
                      aria-label={`User actions for ${m.from}`}
                      className="inline-flex shrink-0 rounded-full hover:ring-2 hover:ring-primary/50 transition-shadow cursor-pointer"
                    >
                      <AuthorAvatar
                        size="sm"
                        uid={sender.id?.identity ?? ''}
                        nick={m.from}
                        avatarB64={sender.id?.avatar}
                      />
                    </span>
                  ) : (
                    <AuthorAvatar size="sm" uid="" nick={m.from} />
                  )
                ) : (
                  <div className="w-7 shrink-0" />
                ))}
              <div
                className={`max-w-[75%] rounded-lg px-3 py-1.5 text-sm ${
                  own ? 'bg-primary/20 text-foreground' : 'bg-muted/30 text-foreground'
                }`}
              >
                {startOfRun && !own && (
                  <p className="text-[10px] font-medium text-muted-foreground mb-0.5">{m.from}</p>
                )}
                <MessageBody body={m.message} />
                <p className="text-[10px] text-muted-foreground mt-0.5">
                  <span title={toYMDTime(new Date(m.timestamp * 1000))}>
                    {new Date(m.timestamp * 1000).toLocaleTimeString([], {
                      hour: '2-digit',
                      minute: '2-digit',
                    })}
                  </span>
                  {own && m.delivered !== undefined && (
                    m.delivered ? (
                      <CheckCheck
                        className="inline h-3 w-3 ml-1 text-primary"
                        aria-label="Delivered to server"
                      />
                    ) : (
                      <Check
                        className="inline h-3 w-3 ml-1 opacity-60"
                        aria-label="Queued"
                      />
                    )
                  )}
                </p>
              </div>
            </div>
          </Fragment>
        );
      })}
      <div ref={endRef} />
    </>
  );
};

const SuggestedKXCard = ({
  mediatorUid,
  targetUid,
  targetNick,
  timestamp,
  known,
  onAccept,
}: {
  mediatorUid: string;
  targetUid: string;
  targetNick: string;
  timestamp: number;
  known?: BisonrelayContact;
  onAccept: (target: string, targetNick: string) => Promise<void>;
}) => {
  const [state, setState] = useState<'idle' | 'submitting' | 'accepted' | 'error'>('idle');
  const [err, setErr] = useState<string | null>(null);

  // Once the suggested user shows up in our contacts (either we just
  // completed the KX they suggested, or we already knew them via another
  // path), there is nothing to accept anymore — render a resolved state.
  // Prefer the live contact's display nick over the embedded one.
  const knownNick = known ? known.nick_alias || known.id?.nick || '' : '';
  const displayName = knownNick || targetNick || targetUid.slice(0, 12);

  const handleClick = async () => {
    if (state === 'submitting' || state === 'accepted') return;
    setState('submitting');
    setErr(null);
    try {
      await onAccept(targetUid, targetNick);
      setState('accepted');
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Accept failed');
      setState('error');
    }
  };

  return (
    <div className="flex justify-center py-1">
      <div className="rounded-lg border border-border/40 bg-muted/20 px-3 py-2 max-w-[80%] text-xs space-y-1.5">
        <p className="italic text-muted-foreground/90">
          Suggested KX with{' '}
          <span className="font-mono text-foreground/90">{displayName}</span>
          <span className="mx-1.5 opacity-50">·</span>
          <span className="opacity-70">{toYMDTime(new Date(timestamp * 1000))}</span>
        </p>
        {known ? (
          <p className="text-muted-foreground">
            Already in your contacts.
          </p>
        ) : state === 'accepted' ? (
          <p className="text-success">
            Requested introduction. The KX completes once the mediator forwards your request.
          </p>
        ) : (
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleClick}
              disabled={state === 'submitting' || !mediatorUid}
              className="px-2.5 py-1 rounded-md bg-gradient-primary text-white text-[11px] font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {state === 'submitting' ? 'Accepting…' : 'Accept'}
            </button>
            {err && <span className="text-destructive break-words">{err}</span>}
          </div>
        )}
      </div>
    </div>
  );
};

const EmptyThread = ({
  onCreate,
  onAccept,
  onJoin,
}: {
  onCreate: () => void;
  onAccept: () => void;
  onJoin?: () => void;
}) => (
  <div className="flex-1 flex flex-col items-center justify-center gap-4 p-8 text-center">
    <MessageSquare className="h-10 w-10 text-muted-foreground" />
    <div>
      <p className="text-sm font-medium">Select a contact</p>
      <p className="text-xs text-muted-foreground mt-1">
        Or create an out-of-band invite to start a new key exchange.
      </p>
    </div>
    <div className="flex gap-2">
      <button
        onClick={onCreate}
        className="px-3 py-1.5 rounded-md bg-gradient-primary text-white text-xs font-semibold inline-flex items-center gap-1.5"
      >
        <UserPlus className="h-3.5 w-3.5" /> Create invite
      </button>
      <button
        onClick={onAccept}
        className="px-3 py-1.5 rounded-md bg-muted/20 text-foreground text-xs font-semibold inline-flex items-center gap-1.5"
      >
        <MessageSquare className="h-3.5 w-3.5" /> Import invite
      </button>
    </div>
    {onJoin && (
      <button
        onClick={onJoin}
        className="px-3 py-1.5 rounded-md bg-primary/10 text-primary text-xs font-semibold inline-flex items-center gap-1.5 hover:bg-primary/20 transition-colors"
      >
        <Users className="h-3.5 w-3.5" /> Join Decred chat networks
      </button>
    )}
  </div>
);

// JoinDecredPulseModal walks the user through joining the community "Decred
// Pulse" group chat via the welcome bot: it requests an invite (no funds), then
// waits for the bot's group-chat invite to arrive and auto-accepts it.
const JoinDecredPulseModal = ({
  onClose,
  onJoined,
}: {
  onClose: () => void;
  onJoined: () => void;
}) => {
  type Phase = 'intro' | 'joining' | 'waiting' | 'done' | 'timeout' | 'error';
  const [phase, setPhase] = useState<Phase>('intro');
  const [err, setErr] = useState<string | null>(null);

  // Once the invite has been requested, poll for the incoming group-chat invite
  // and accept it as soon as it arrives.
  useEffect(() => {
    if (phase !== 'waiting') return;
    let cancelled = false;
    let timer = 0;
    let attempts = 0;
    const tick = async () => {
      attempts++;
      try {
        // Success is membership in the group chat, regardless of which path
        // accepted the invite (this modal or the incoming-invites banner).
        const groups = await listBisonrelayGCs();
        if (groups.some((g) => g.name === DECRED_PULSE_GC || g.alias === DECRED_PULSE_GC)) {
          if (cancelled) return;
          onJoined();
          setPhase('done');
          timer = window.setTimeout(() => {
            if (!cancelled) onClose();
          }, 1500);
          return;
        }
        // Accept the bot's group-chat invite as soon as it arrives.
        const { invites } = await listBisonrelayGCInvites();
        const inv = invites.find((i) => i.name === DECRED_PULSE_GC && !i.accepted);
        if (inv) {
          await acceptBisonrelayGCInvite(inv.id);
        }
      } catch {
        // Keep polling; transient errors are expected while KX completes.
      }
      if (cancelled) return;
      if (attempts >= 40) {
        setPhase('timeout');
        return;
      }
      timer = window.setTimeout(tick, 3000);
    };
    timer = window.setTimeout(tick, 3000);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [phase, onClose, onJoined]);

  const confirm = async () => {
    setErr(null);
    setPhase('joining');
    try {
      await joinDecredPulse();
      setPhase('waiting');
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Request failed');
      setPhase('error');
    }
  };

  return (
    <Modal title="Join Decred chat networks" onClose={onClose}>
      {phase === 'intro' && (
        <>
          <p className="text-sm font-medium">Go public and meet the Decred community.</p>
          <p className="text-sm text-muted-foreground">
            Join the "Decred Pulse" welcome room to get started:
          </p>
          <ul className="text-sm text-muted-foreground space-y-1.5">
            <li className="flex gap-2">
              <span className="text-primary shrink-0">•</span>
              <span>
                <span className="font-medium text-foreground">Instant contacts</span> - everyone
                already in the room becomes someone you can message.
              </span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary shrink-0">•</span>
              <span>
                <span className="font-medium text-foreground">More rooms</span> - branch out into
                other community rooms from there.
              </span>
            </li>
            <li className="flex gap-2">
              <span className="text-primary shrink-0">•</span>
              <span>
                <span className="font-medium text-foreground">Your first followers</span> - members
                start seeing your posts automatically, so you have an audience from day one.
              </span>
            </li>
          </ul>
          <p className="text-xs text-muted-foreground">Optional, private, and free.</p>
          <div className="flex justify-end gap-2">
            <button
              onClick={onClose}
              className="px-3 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30"
            >
              Cancel
            </button>
            <button
              onClick={confirm}
              className="px-3 py-1.5 rounded-md bg-gradient-primary text-white text-xs font-semibold inline-flex items-center gap-1.5"
            >
              <Users className="h-3.5 w-3.5" /> Join Decred Pulse
            </button>
          </div>
        </>
      )}
      {(phase === 'joining' || phase === 'waiting') && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>
            {phase === 'joining'
              ? 'Requesting an invite…'
              : 'Key exchange in progress, waiting to join the group…'}
          </span>
        </div>
      )}
      {phase === 'done' && (
        <div className="flex items-center gap-2 text-sm text-success">
          <Check className="h-4 w-4" />
          <span>You have joined Decred Pulse.</span>
        </div>
      )}
      {phase === 'timeout' && (
        <p className="text-sm text-muted-foreground">
          Your request was sent. The group invite can take a moment to arrive;
          when it does you can accept it from the invites banner at the top of
          this page.
        </p>
      )}
      {phase === 'error' && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      )}
    </Modal>
  );
};

const InviteCreateModal = ({ onClose }: { onClose: () => void }) => {
  const [invite, setInvite] = useState<{ invite_bytes: string; invite_key: string } | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [copiedKey, setCopiedKey] = useState(false);
  const [copiedBlob, setCopiedBlob] = useState(false);

  useEffect(() => {
    let cancelled = false;
    writeBisonrelayInvite()
      .then((b) => {
        if (!cancelled) setInvite(b);
      })
      .catch((e) => {
        if (!cancelled) setErr(e?.message || 'Could not create invite');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const copy = async (text: string, setter: (v: boolean) => void) => {
    try {
      await navigator.clipboard.writeText(text);
      setter(true);
      setTimeout(() => setter(false), 1500);
    } catch {
      /* ignore */
    }
  };

  const downloadBlob = () => {
    if (!invite) return;
    const bin = atob(invite.invite_bytes);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    const blob = new Blob([bytes], { type: 'application/octet-stream' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'invite.bin';
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Modal title="Out-of-band invite" onClose={onClose}>
      <p className="text-sm text-muted-foreground">
        Share one of the forms below with the peer you want to chat with,
        through any out-of-band channel (signal, email, paste). They use
        "Import invite" on their end. Each invite can be redeemed once.
      </p>
      {loading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Generating invite…</span>
        </div>
      ) : err ? (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{err}</span>
        </div>
      ) : invite ? (
        <>
          <div className="space-y-1">
            <p className="text-xs font-semibold text-muted-foreground">Short key (recommended)</p>
            <div className="rounded-md bg-background border border-border p-2 flex items-start gap-2">
              <code className="font-mono text-xs break-all flex-1">{invite.invite_key}</code>
              <button
                onClick={() => copy(invite.invite_key, setCopiedKey)}
                className="p-1 rounded hover:bg-muted/30 text-muted-foreground"
                title="Copy"
              >
                <Copy className="h-3.5 w-3.5" />
              </button>
            </div>
            {copiedKey && <p className="text-[10px] text-success">Copied</p>}
          </div>
          <div className="space-y-1">
            <p className="text-xs font-semibold text-muted-foreground">Binary blob (advanced)</p>
            <div className="flex gap-2">
              <button
                onClick={downloadBlob}
                className="px-3 py-1.5 rounded-md bg-muted/20 text-foreground text-xs font-semibold"
              >
                Download invite.bin
              </button>
              <button
                onClick={() => copy(invite.invite_bytes, setCopiedBlob)}
                className="px-3 py-1.5 rounded-md bg-muted/20 text-foreground text-xs font-semibold inline-flex items-center gap-1.5"
              >
                <Copy className="h-3.5 w-3.5" /> {copiedBlob ? 'Copied' : 'Copy base64'}
              </button>
            </div>
          </div>
        </>
      ) : null}
    </Modal>
  );
};

const InviteAcceptModal = ({
  onClose,
  onAccepted,
}: {
  onClose: () => void;
  onAccepted: () => void;
}) => {
  const [value, setValue] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onFile = async (file: File) => {
    setErr(null);
    try {
      const buf = await file.arrayBuffer();
      const bytes = new Uint8Array(buf);
      let bin = '';
      for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
      setValue(btoa(bin));
    } catch (e: any) {
      setErr(e?.message || 'Could not read file');
    }
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!value.trim() || submitting) return;
    setSubmitting(true);
    setErr(null);
    try {
      await acceptBisonrelayInvite(value.trim());
      onAccepted();
      onClose();
    } catch (e: any) {
      setErr(e?.message || 'Could not accept invite');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal title="Import invite" onClose={onClose}>
      <form onSubmit={submit} className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Paste a short key (starts with <code className="font-mono">brpik1</code>),
          paste a base64 invite blob, or pick a <code className="font-mono">.bin</code> file
          your peer shared. A key exchange runs in the background and the new
          contact appears in the list once it completes.
        </p>
        <input
          type="file"
          accept=".bin,application/octet-stream"
          disabled={submitting}
          onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) onFile(f);
          }}
          className="block w-full text-xs text-muted-foreground file:mr-3 file:py-1.5 file:px-3 file:rounded-md file:border-0 file:bg-muted/20 file:text-foreground file:text-xs file:font-semibold hover:file:bg-muted/30"
        />
        <textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          disabled={submitting}
          placeholder="…or paste a brpik1… key, base64 invite blob"
          className="w-full h-32 px-3 py-2 rounded-lg bg-background border border-border text-foreground font-mono text-xs focus:outline-none focus:border-primary disabled:opacity-50"
        />
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{err}</span>
          </div>
        )}
        <div className="flex justify-end">
          <button
            type="submit"
            disabled={!value.trim() || submitting}
            className="px-3 py-1.5 rounded-md bg-gradient-primary text-white text-xs font-semibold disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? 'Importing…' : 'Import'}
          </button>
        </div>
      </form>
    </Modal>
  );
};

const Modal = ({
  title,
  children,
  onClose,
}: {
  title: string;
  children: React.ReactNode;
  onClose: () => void;
}) => (
  <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
    <div className="w-full max-w-xl mx-4 rounded-xl bg-card border border-border/50 shadow-xl">
      <div className="flex items-center justify-between p-4 border-b border-border/50">
        <h3 className="text-sm font-semibold">{title}</h3>
        <button
          onClick={onClose}
          className="p-1 rounded hover:bg-muted/30 transition-colors text-muted-foreground"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <div className="p-4 space-y-3">{children}</div>
    </div>
  </div>
);

// brIdentityToHex normalizes a BR identity to 64-char hex. /br/identity returns
// the local identity base64-encoded, whereas GC member uids are hex; contacts
// are already hex and pass through unchanged.
function brIdentityToHex(s: string): string {
  if (/^[0-9a-f]{64}$/i.test(s)) return s;
  try {
    const bin = atob(s);
    if (bin.length !== 32) return s;
    let hex = '';
    for (let i = 0; i < bin.length; i++) {
      hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  } catch {
    return s;
  }
}

function displayNick(c: BisonrelayContact): string {
  return c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || 'unknown';
}

function nickOrUid(c: BisonrelayContact): string {
  return c.nick_alias || c.id?.nick || c.id?.identity || '';
}

// heardAge renders a compact age ("3h", "2d") for when the contact was last
// heard from (last decrypted message); null when never heard.
function heardAge(iso?: string): string | null {
  if (!iso) return null;
  const t = Date.parse(iso);
  if (!t || Number.isNaN(t) || t <= 0) return null;
  const delta = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (delta < 60) return 'now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h`;
  return `${Math.floor(delta / 86400)}d`;
}

function identityFromPayload(payload: any): string {
  if (!payload) return '';
  const uid = payload.uid;
  if (typeof uid !== 'string' || !uid) return '';
  try {
    const bin = atob(uid);
    let hex = '';
    for (let i = 0; i < bin.length; i++) {
      hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  } catch {
    return '';
  }
}

const ContactAvatar = ({ contact, nick }: { contact: BisonrelayContact; nick: string }) => {
  const dataUrl = avatarDataUrl(contact.id?.avatar);
  const initial = nick.trim().charAt(0).toUpperCase() || '?';
  const bgClass = colorForUid(contact.id?.identity ?? nick);
  if (dataUrl) {
    return (
      <img
        src={dataUrl}
        alt=""
        className="shrink-0 h-7 w-7 rounded-full object-cover bg-muted/30"
      />
    );
  }
  return (
    <span className={`shrink-0 h-7 w-7 rounded-full flex items-center justify-center text-[11px] font-semibold text-white ${bgClass}`}>
      {initial}
    </span>
  );
};

const SENT_FILE_RE = /^Sent file "(.+)"(?:\s*\(([^)]*)\))?\s*$/;

const MessageBody = ({ body }: { body: string }) => {
  const trimmed = body.trim();
  const sent = trimmed.match(SENT_FILE_RE);
  if (sent) {
    return <SentFileChip filename={sent[1]} fileid={sent[2] ?? ''} />;
  }
  const segments = parseEmbeds(body);
  return (
    <div className="space-y-1">
      {segments.map((seg, i) => {
        if (seg.kind === 'text') {
          if (!seg.text.trim()) return null;
          return (
            <p key={i} className="whitespace-pre-wrap break-words">
              {linkifyChatText(seg.text)}
            </p>
          );
        }
        if (seg.kind === 'embed') {
          return <EmbedRenderer key={i} embed={seg} />;
        }
        return <DownloadRenderer key={i} download={seg} />;
      })}
    </div>
  );
};

const SentFileChip = ({ filename }: { filename: string; fileid: string }) => {
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded border border-border/40 bg-background/40">
      <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{filename}</p>
        <p className="text-[10px] text-muted-foreground">sent</p>
      </div>
    </div>
  );
};

const DownloadRenderer = ({ download }: { download: DownloadSegment }) => {
  const url = downloadFileUrl(download.nick, download.filename);
  if (!url) {
    return (
      <p className="text-[11px] text-muted-foreground italic">
        [file {download.filename || 'unnamed'} not available]
      </p>
    );
  }
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded border border-border/40 bg-background/40">
      <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{download.filename}</p>
        <p className="text-[10px] text-muted-foreground">
          {download.mime || 'file'}{download.size ? ' · ' + formatBytes(download.size) : ''}
        </p>
      </div>
      <a
        href={url}
        download={download.filename}
        className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
        title="Save"
      >
        <Download className="h-4 w-4" />
      </a>
    </div>
  );
};

const EmbedRenderer = ({ embed }: { embed: EmbedSegment }) => {
  const openViewer = useContext(ImageViewerCtx);
  const inlineUrl = embed.dataB64 ? `data:${embed.mime};base64,${embed.dataB64}` : '';
  const fileUrl = inlineUrl || embedFileUrl(embed.localFilename);
  if (!fileUrl) {
    return (
      <p className="text-[11px] text-muted-foreground italic">
        [attachment {embed.name || embed.filename || 'unnamed'} not available]
      </p>
    );
  }
  if (isImageMime(embed.mime)) {
    const displayName = embed.name || embed.filename || 'image';
    return (
      <button
        type="button"
        onClick={() => openViewer?.(fileUrl, displayName, embed.mime)}
        className="block p-0 border-0 bg-transparent cursor-zoom-in"
      >
        <img
          src={fileUrl}
          alt={embed.alt || displayName}
          loading="lazy"
          className="max-h-72 max-w-full rounded border border-border/40 object-contain bg-background/40"
        />
      </button>
    );
  }
  return <NonImageEmbed embed={embed} fileUrl={fileUrl} />;
};

const NonImageEmbed = ({ embed, fileUrl }: { embed: EmbedSegment; fileUrl: string }) => {
  const filename = embed.name || embed.filename || 'attachment';
  const bytes = embed.dataB64 ? Math.floor((embed.dataB64.length * 3) / 4) : embed.size;
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded border border-border/40 bg-background/40">
      <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{filename}</p>
        <p className="text-[10px] text-muted-foreground">
          {embed.mime || 'unknown'}{bytes ? ' · ' + formatBytes(bytes) : ''}
        </p>
      </div>
      <a
        href={fileUrl}
        download={filename}
        className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
        title="Save"
      >
        <Download className="h-4 w-4" />
      </a>
    </div>
  );
};

const AttachmentPreview = ({
  attachment,
  onRemove,
}: {
  attachment: StagedAttachment;
  onRemove: () => void;
}) => {
  const mime = attachment.file.type || 'application/octet-stream';
  const showImage = attachment.mode === 'inline' && isImageMime(mime) && attachment.dataB64;
  const modeLabel = attachment.mode === 'inline' ? 'inline embed' : 'file transfer';
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded-md border border-border/40 bg-muted/10">
      {showImage ? (
        <img
          src={`data:${mime};base64,${attachment.dataB64}`}
          alt={attachment.file.name}
          className="h-10 w-10 rounded object-cover"
        />
      ) : (
        <FileText className="h-5 w-5 text-muted-foreground shrink-0" />
      )}
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{attachment.file.name}</p>
        <p className="text-[10px] text-muted-foreground">
          {mime} · {formatBytes(attachment.file.size)} · {modeLabel}
        </p>
      </div>
      <button
        type="button"
        onClick={onRemove}
        className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
        title="Remove attachment"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
};

