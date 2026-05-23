// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useState } from 'react';
import {
  AlertCircle,
  Check,
  Coins,
  Copy,
  Edit2,
  FileText,
  Folder,
  Handshake,
  List,
  Loader2,
  Paperclip,
  RotateCw,
  Rss,
  UserPlus,
  Users,
  X,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayContentItem,
  BisonrelayLiveEvent,
  BisonrelayPostListItem,
  fetchBisonrelayUserPost,
  handshakeBisonrelayContact,
  kxResetBisonrelayContact,
  listBisonrelayUserContent,
  listBisonrelayUserPosts,
  renameBisonrelayContact,
  subscribeBisonrelayPosts,
  suggestKxBisonrelayContact,
  tipBisonrelayContact,
  transResetBisonrelayContact,
  unsubscribeBisonrelayPosts,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { avatarDataUrl, colorForUid } from './bisonrelayAvatar';

// Layout + action ordering mirrors bruig's chat_side_menu / user_context_menu
// (companyzero/bisonrelay, ISC). "User Profile" is collapsed into the header
// card so we render eleven rows; only the first row (Send File) is live in
// P0, the rest are disabled placeholders that later phases unlock.

interface Props {
  contact: BisonrelayContact;
  nick: string;
  contacts: BisonrelayContact[];
  displayNick: (c: BisonrelayContact) => string;
  onClose: () => void;
  onSendFile: () => void;
  onRenamed?: (newNick: string) => void;
  onTip?: (uid: string, nick: string, dcrAmount: number) => void;
  onSubscribePosts?: (uid: string, nick: string) => void;
  onUnsubscribePosts?: (uid: string, nick: string) => void;
}

interface Row {
  id: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  onClick?: () => void;
  comingSoon?: boolean;
}

type ActiveModal =
  | null
  | 'rename'
  | 'kx-reset'
  | 'handshake'
  | 'suggest-kx'
  | 'trans-reset'
  | 'tip'
  | 'subscribe-posts'
  | 'unsubscribe-posts'
  | 'list-posts'
  | 'show-content';

export const BisonrelayUserSubNav = ({
  contact,
  nick,
  contacts,
  displayNick,
  onClose,
  onSendFile,
  onRenamed,
  onTip,
  onSubscribePosts,
  onUnsubscribePosts,
}: Props) => {
  const [modal, setModal] = useState<ActiveModal>(null);
  const uid = contact.id?.identity ?? '';

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      // Esc closes the active modal first if any; otherwise the sub-nav.
      if (e.key === 'Escape') {
        if (modal) setModal(null);
        else onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose, modal]);

  const subscribed = !!contact.posts_subscribed;
  const rows: Row[] = [
    { id: 'tip', label: 'Pay Tip', icon: Coins, onClick: () => setModal('tip') },
    { id: 'kx-reset', label: 'Request Ratchet Reset', icon: RotateCw, onClick: () => setModal('kx-reset') },
    { id: 'content', label: 'Show Content', icon: Folder, onClick: () => setModal('show-content') },
    {
      id: 'posts-toggle',
      label: subscribed ? 'Unsubscribe from Posts' : 'Subscribe to Posts',
      icon: Rss,
      onClick: () => setModal(subscribed ? 'unsubscribe-posts' : 'subscribe-posts'),
    },
    { id: 'list-posts', label: 'List Posts', icon: List, onClick: () => setModal('list-posts') },
    { id: 'send-file', label: 'Send File', icon: Paperclip, onClick: onSendFile },
    { id: 'pages', label: 'View Pages', icon: FileText, comingSoon: true },
    { id: 'rename', label: 'Rename User', icon: Edit2, onClick: () => setModal('rename') },
    { id: 'suggest-kx', label: 'Suggest User to KX', icon: UserPlus, onClick: () => setModal('suggest-kx') },
    { id: 'trans-reset', label: 'Issue Transitive Reset', icon: Users, onClick: () => setModal('trans-reset') },
    { id: 'handshake', label: 'Perform Handshake', icon: Handshake, onClick: () => setModal('handshake') },
  ];

  return (
    <>
      <div
        className="absolute inset-0 bg-black/20 backdrop-blur-[1px] z-10 rounded-xl"
        onClick={onClose}
        aria-hidden
      />
      <aside
        className="absolute right-0 top-0 bottom-0 w-64 flex flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 z-20 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <Header contact={contact} nick={nick} onClose={onClose} />
        <div className="flex-1 overflow-y-auto py-1">
          {rows.map((r) => (
            <ActionRow key={r.id} row={r} />
          ))}
        </div>
      </aside>
      {modal === 'rename' && (
        <RenameModal
          contact={contact}
          currentNick={nick}
          onClose={() => setModal(null)}
          onSuccess={(newNick) => {
            setModal(null);
            onRenamed?.(newNick);
          }}
        />
      )}
      {modal === 'kx-reset' && (
        <ConfirmActionModal
          title={`Request ratchet reset with ${nick}?`}
          body="Both sides derive new ratchet keys. Use this when messages stop arriving in either direction; the user is not notified out-of-band."
          confirmLabel="Reset ratchet"
          onClose={() => setModal(null)}
          onConfirm={() => kxResetBisonrelayContact(uid)}
          onSuccess={onClose}
        />
      )}
      {modal === 'handshake' && (
        <ConfirmActionModal
          title={`Send handshake to ${nick}?`}
          body="Starts a 3-way handshake. When the SYNACK comes back you know the connection is fully operational."
          confirmLabel="Send handshake"
          onClose={() => setModal(null)}
          onConfirm={() => handshakeBisonrelayContact(uid)}
          onSuccess={onClose}
        />
      )}
      {modal === 'suggest-kx' && (
        <ContactPickerModal
          title={`Suggest a user for ${nick} to KX with`}
          body={`Picks a contact and asks ${nick} to KX with them. The remote contact gets a message; they decide whether to follow up.`}
          contacts={contacts}
          excludeUid={uid}
          displayNick={displayNick}
          onClose={() => setModal(null)}
          onPick={(picked) =>
            suggestKxBisonrelayContact(uid, picked.id?.identity ?? '')
          }
          onSuccess={onClose}
        />
      )}
      {modal === 'trans-reset' && (
        <ContactPickerModal
          title={`Reset ratchet with ${nick} via a mediator`}
          body={`Picks a common contact and asks them to forward a ratchet-reset request to ${nick}. Use this when a direct reset is not landing.`}
          contacts={contacts}
          excludeUid={uid}
          displayNick={displayNick}
          onClose={() => setModal(null)}
          onPick={(picked) =>
            transResetBisonrelayContact(picked.id?.identity ?? '', uid)
          }
          onSuccess={onClose}
        />
      )}
      {modal === 'subscribe-posts' && (
        <ConfirmActionModal
          title={`Subscribe to ${nick}'s posts?`}
          body={`Asks ${nick} to send their existing and future posts. The subscription becomes active once their client replies, which may take a while if they're offline.`}
          confirmLabel="Subscribe"
          onClose={() => setModal(null)}
          onConfirm={() => {
            if (onSubscribePosts) onSubscribePosts(uid, nick);
            else subscribeBisonrelayPosts(uid);
          }}
          onSuccess={onClose}
          optimistic
        />
      )}
      {modal === 'unsubscribe-posts' && (
        <ConfirmActionModal
          title={`Unsubscribe from ${nick}'s posts?`}
          body={`Tells ${nick}'s client to stop sending you their posts. Existing posts in your history stay.`}
          confirmLabel="Unsubscribe"
          onClose={() => setModal(null)}
          onConfirm={() => {
            if (onUnsubscribePosts) onUnsubscribePosts(uid, nick);
            else unsubscribeBisonrelayPosts(uid);
          }}
          onSuccess={onClose}
          optimistic
        />
      )}
      {modal === 'list-posts' && (
        <PostsListModal
          nick={nick}
          uid={uid}
          onClose={() => setModal(null)}
          onPicked={onClose}
        />
      )}
      {modal === 'show-content' && (
        <ContentListModal
          nick={nick}
          uid={uid}
          onClose={() => setModal(null)}
        />
      )}
      {modal === 'tip' && (
        <TipModal
          nick={nick}
          uid={uid}
          onClose={() => setModal(null)}
          onSubmit={(dcr) => {
            if (onTip) onTip(uid, nick, dcr);
            else tipBisonrelayContact(uid, dcr);
            onClose();
          }}
        />
      )}
    </>
  );
};

const RenameModal = ({
  contact,
  currentNick,
  onClose,
  onSuccess,
}: {
  contact: BisonrelayContact;
  currentNick: string;
  onClose: () => void;
  onSuccess: (newNick: string) => void;
}) => {
  const [value, setValue] = useState(currentNick);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const uid = contact.id?.identity ?? '';
  const trimmed = value.trim();
  const canSubmit = !submitting && trimmed.length > 0 && trimmed !== currentNick && uid !== '';

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    setSubmitting(true);
    setErr(null);
    try {
      await renameBisonrelayContact(uid, trimmed);
      onSuccess(trimmed);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Rename failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold">Rename {currentNick}</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <p className="text-xs text-muted-foreground">
          Sets a local alias for this contact. The remote user is not notified
          and their own nick is unchanged.
        </p>
        <div>
          <label className="block text-xs text-muted-foreground mb-1" htmlFor="br-rename-input">
            New nick
          </label>
          <input
            id="br-rename-input"
            type="text"
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={submitting}
            maxLength={64}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
          />
        </div>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? 'Renaming…' : 'Rename'}
          </button>
        </div>
      </form>
    </div>
  );
};

const Header = ({
  contact,
  nick,
  onClose,
}: {
  contact: BisonrelayContact;
  nick: string;
  onClose: () => void;
}) => {
  const [copied, setCopied] = useState(false);
  const identity = contact.id?.identity ?? '';

  const onCopy = async () => {
    if (!identity) return;
    try {
      await navigator.clipboard.writeText(identity);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore clipboard errors */
    }
  };

  return (
    <div className="relative p-4 pb-3 border-b border-border/50">
      <button
        onClick={onClose}
        className="absolute right-2 top-2 p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
        title="Close"
        aria-label="Close"
      >
        <X className="h-4 w-4" />
      </button>
      <div className="flex flex-col items-center text-center gap-2 mt-1">
        <BigAvatar contact={contact} nick={nick} />
        <p className="text-sm font-semibold truncate w-full px-4">{nick}</p>
      </div>
      {identity && (
        <button
          onClick={onCopy}
          className="mt-2 w-full text-[10px] font-mono text-muted-foreground hover:text-foreground bg-muted/20 hover:bg-muted/30 rounded px-2 py-1.5 text-left flex items-start gap-1.5 transition-colors"
          title="Copy identity"
        >
          {copied ? (
            <Check className="h-3 w-3 mt-0.5 shrink-0 text-success" />
          ) : (
            <Copy className="h-3 w-3 mt-0.5 shrink-0" />
          )}
          <span className="flex-1 break-all">{identity}</span>
        </button>
      )}
    </div>
  );
};

const BigAvatar = ({ contact, nick }: { contact: BisonrelayContact; nick: string }) => {
  const dataUrl = avatarDataUrl(contact.id?.avatar);
  const initial = nick.trim().charAt(0).toUpperCase() || '?';
  const bgClass = colorForUid(contact.id?.identity ?? nick);
  if (dataUrl) {
    return (
      <img
        src={dataUrl}
        alt=""
        className="h-16 w-16 rounded-full object-cover bg-muted/30"
      />
    );
  }
  return (
    <span
      className={`h-16 w-16 rounded-full flex items-center justify-center text-2xl font-semibold text-white ${bgClass}`}
    >
      {initial}
    </span>
  );
};

const ContentListModal = ({
  nick,
  uid,
  onClose,
}: {
  nick: string;
  uid: string;
  onClose: () => void;
}) => {
  const [files, setFiles] = useState<BisonrelayContentItem[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  useEffect(() => {
    const unsubscribe = addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'content-list-received') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.uid !== uid) return;
      if (typeof payload.error === 'string') {
        setErr(payload.error);
        return;
      }
      const raw = (payload.files ?? []) as Array<Record<string, unknown>>;
      const list: BisonrelayContentItem[] = raw.map((f) => ({
        file_id: String(f.file_id ?? ''),
        filename: String(f.filename ?? ''),
        size: Number(f.size ?? 0),
        directory: String(f.directory ?? ''),
        description: String(f.description ?? ''),
        cost: Number(f.cost ?? 0),
        downloaded: !!f.downloaded,
      }));
      list.sort((a, b) => a.filename.localeCompare(b.filename));
      setFiles(list);
    });
    listBisonrelayUserContent(uid).catch((e: any) => {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not request content list');
    });
    return unsubscribe;
  }, [addListener, uid]);

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[80vh]"
      >
        <div className="p-5 pb-3 flex items-start justify-between">
          <div>
            <h3 className="text-base font-semibold">Content shared by {nick}</h3>
            <p className="text-xs text-muted-foreground mt-1">
              Asks {nick} for their list of shared files. If they're offline
              the reply arrives whenever they come back online.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[160px]">
          {err ? (
            <div className="flex items-start gap-2 text-sm text-destructive px-3 py-4">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          ) : files === null ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
              <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              <span>Waiting for {nick}'s reply…</span>
            </div>
          ) : files.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              {nick} has no shared content.
            </p>
          ) : (
            files.map((f) => (
              <div
                key={f.file_id}
                className="px-3 py-2 rounded-md text-sm flex flex-col gap-0.5"
              >
                <div className="flex items-center gap-2">
                  <span className="truncate font-medium text-foreground">
                    {f.filename || '(unnamed file)'}
                  </span>
                  {f.downloaded && (
                    <span className="shrink-0 text-[9px] uppercase tracking-wide text-success/80">
                      saved
                    </span>
                  )}
                </div>
                <div className="text-[10px] text-muted-foreground flex items-center gap-2">
                  <span>{formatBytesPretty(f.size)}</span>
                  {f.directory && (
                    <>
                      <span className="opacity-50">·</span>
                      <span className="font-mono">{f.directory}</span>
                    </>
                  )}
                </div>
                {f.description && (
                  <p className="text-[11px] text-muted-foreground/90 mt-0.5 break-words">
                    {f.description}
                  </p>
                )}
              </div>
            ))
          )}
        </div>
        <div className="flex justify-end gap-2 px-5 pb-5 pt-1 border-t border-border/30">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};

function formatBytesPretty(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

const PostsListModal = ({
  nick,
  uid,
  onClose,
  onPicked,
}: {
  nick: string;
  uid: string;
  onClose: () => void;
  // Called after a post is clicked + its fetch has been requested. The
  // page hash is already navigated by then; the sub-nav uses this to
  // close itself so the Feed view is unobstructed.
  onPicked?: () => void;
}) => {
  const [posts, setPosts] = useState<BisonrelayPostListItem[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [picking, setPicking] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

  const handlePick = async (post: BisonrelayPostListItem) => {
    if (picking) return;
    setPicking(post.id);
    setErr(null);
    try {
      await fetchBisonrelayUserPost(uid, post.id);
      window.location.hash = `feed/post/${uid}/${post.id}`;
      onClose();
      onPicked?.();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not request post');
      setPicking(null);
    }
  };

  // Subscribe to the live event BEFORE kicking off the request so we
  // don't miss a fast reply. Unsubscribe on unmount.
  useEffect(() => {
    const unsubscribe = addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'posts-list-received') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.uid !== uid) return;
      const raw = (payload.posts ?? []) as Array<Record<string, unknown>>;
      const list: BisonrelayPostListItem[] = raw.map((p) => ({
        id: String(p.id ?? ''),
        title: String(p.title ?? ''),
        timestamp: Number(p.timestamp ?? 0),
      }));
      // Newest first
      list.sort((a, b) => b.timestamp - a.timestamp);
      setPosts(list);
    });
    listBisonrelayUserPosts(uid).catch((e: any) => {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not request post list');
    });
    return unsubscribe;
  }, [addListener, uid]);

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[80vh]"
      >
        <div className="p-5 pb-3 flex items-start justify-between">
          <div>
            <h3 className="text-base font-semibold">Posts by {nick}</h3>
            <p className="text-xs text-muted-foreground mt-1">
              Asks {nick} for their post list. If they're offline the reply
              arrives whenever they come back online.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[160px]">
          {err ? (
            <div className="flex items-start gap-2 text-sm text-destructive px-3 py-4">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          ) : posts === null ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
              <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              <span>Waiting for {nick}'s reply…</span>
            </div>
          ) : posts.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              {nick} hasn't published any posts yet.
            </p>
          ) : (
            posts.map((p) => {
              const isPicking = picking === p.id;
              return (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => handlePick(p)}
                  disabled={picking !== null}
                  className="w-full text-left px-3 py-2 rounded-md text-sm flex flex-col gap-0.5 hover:bg-muted/30 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <span className="flex items-center gap-2">
                    <span className="truncate font-medium text-foreground">
                      {p.title || '(untitled)'}
                    </span>
                    {isPicking && <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />}
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    {new Date(p.timestamp * 1000).toLocaleString()}
                  </span>
                </button>
              );
            })
          )}
        </div>
        <div className="flex justify-end gap-2 px-5 pb-5 pt-1 border-t border-border/30">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};

const TipModal = ({
  nick,
  uid,
  onClose,
  onSubmit,
}: {
  nick: string;
  uid: string;
  onClose: () => void;
  // Fire-and-forget. The page inserts a pending placeholder into the
  // chat thread and tracks completion; this modal just collects the
  // amount and dismisses.
  onSubmit: (dcrAmount: number) => void;
}) => {
  const [value, setValue] = useState('');

  const parsed = parseFloat(value);
  const canSubmit = uid !== '' && Number.isFinite(parsed) && parsed > 0;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    onSubmit(parsed);
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold">Pay tip to {nick}</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <p className="text-xs text-muted-foreground">
          The tip rides over your Lightning channel. Both you and {nick} must
          be online; the payment is delivered when their client is reachable.
        </p>
        <div>
          <label className="block text-xs text-muted-foreground mb-1" htmlFor="br-tip-amount">
            Amount (DCR)
          </label>
          <input
            id="br-tip-amount"
            type="number"
            autoFocus
            min="0"
            step="any"
            inputMode="decimal"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="0.001"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
          />
        </div>
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Send tip
          </button>
        </div>
      </form>
    </div>
  );
};

const ContactPickerModal = ({
  title,
  body,
  contacts,
  excludeUid,
  displayNick,
  onClose,
  onPick,
  onSuccess,
}: {
  title: string;
  body: string;
  contacts: BisonrelayContact[];
  excludeUid: string;
  displayNick: (c: BisonrelayContact) => string;
  onClose: () => void;
  onPick: (c: BisonrelayContact) => Promise<void>;
  onSuccess?: () => void;
}) => {
  const [query, setQuery] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const q = query.trim().toLowerCase();
  const filtered = contacts.filter((c) => {
    if (c.id?.identity === excludeUid) return false;
    if (!q) return true;
    return displayNick(c).toLowerCase().includes(q);
  });

  const handlePick = async (c: BisonrelayContact) => {
    if (submitting) return;
    setSubmitting(true);
    setErr(null);
    try {
      await onPick(c);
      onClose();
      onSuccess?.();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Action failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[80vh]"
      >
        <div className="p-5 pb-3 space-y-3">
          <div className="flex items-start justify-between">
            <h3 className="text-base font-semibold pr-4">{title}</h3>
            <button
              type="button"
              onClick={onClose}
              className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <p className="text-xs text-muted-foreground">{body}</p>
          <input
            type="text"
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search contacts…"
            disabled={submitting}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
          />
        </div>
        <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[120px]">
          {filtered.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              {contacts.length <= 1
                ? 'You need at least one other contact to use this action.'
                : 'No contacts match your search.'}
            </p>
          ) : (
            filtered.map((c) => {
              const n = displayNick(c);
              return (
                <button
                  key={c.id?.identity ?? n}
                  onClick={() => handlePick(c)}
                  disabled={submitting}
                  className="w-full text-left px-3 py-2 rounded-md text-sm flex items-center gap-2 hover:bg-muted/30 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <span className="truncate">{n}</span>
                </button>
              );
            })
          )}
        </div>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive px-5 pb-3">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
        <div className="flex justify-end gap-2 px-5 pb-5 pt-1 border-t border-border/30">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
};

const ConfirmActionModal = ({
  title,
  body,
  confirmLabel,
  onClose,
  onConfirm,
  onSuccess,
  optimistic,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  onClose: () => void;
  onConfirm: () => Promise<void> | void;
  onSuccess?: () => void;
  // optimistic=true: don't await onConfirm and skip the modal-local error
  // state. The caller is responsible for showing in-flight + outcome
  // feedback elsewhere (e.g. as a placeholder message in the chat). Used
  // by posts subscribe/unsubscribe where the BR round-trip can take an
  // unbounded amount of time depending on the remote's reachability.
  optimistic?: boolean;
}) => {
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handleConfirm = async () => {
    if (optimistic) {
      try {
        onConfirm();
      } catch {
        // Optimistic callers handle errors via the placeholder; swallow
        // anything thrown synchronously here so the modal still closes.
      }
      onClose();
      onSuccess?.();
      return;
    }
    setSubmitting(true);
    setErr(null);
    try {
      await onConfirm();
      onClose();
      onSuccess?.();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Action failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold pr-4">{title}</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <p className="text-xs text-muted-foreground">{body}</p>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={submitting}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {submitting ? 'Working…' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

const ActionRow = ({ row }: { row: Row }) => {
  const Icon = row.icon;
  const disabled = !!row.comingSoon || !row.onClick;
  return (
    <button
      onClick={row.onClick}
      disabled={disabled}
      className={`w-full px-4 py-2 text-left text-sm flex items-center gap-3 transition-colors ${
        disabled
          ? 'text-muted-foreground/60 cursor-not-allowed'
          : 'text-foreground hover:bg-muted/30'
      }`}
      title={row.comingSoon ? 'Coming soon' : undefined}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="flex-1 truncate">{row.label}</span>
      {row.comingSoon && (
        <span className="shrink-0 text-[9px] uppercase tracking-wide text-muted-foreground/70">
          soon
        </span>
      )}
    </button>
  );
};
