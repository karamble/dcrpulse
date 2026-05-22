// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  Copy,
  Download,
  FileText,
  Loader2,
  MessageSquare,
  Paperclip,
  Send,
  UserPlus,
  X,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayMessage,
  BisonrelayPMAttachment,
  acceptBisonrelayInvite,
  getBisonrelayContacts,
  getBisonrelayMessages,
  sendBisonrelayPM,
  writeBisonrelayInvite,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { EmbedSegment, embedFileUrl, formatBytes, isImageMime, parseEmbeds } from './embedParser';

const MAX_INLINE_BYTES = 800 * 1024;

interface ImageViewerOpenFn {
  (src: string, name: string, mime: string): void;
}

const ImageViewerCtx = createContext<ImageViewerOpenFn | null>(null);

interface ViewerImage {
  src: string;
  name: string;
  mime: string;
}

export const BisonrelayMessagingPage = ({ ownNick }: { ownNick: string }) => {
  const [contacts, setContacts] = useState<BisonrelayContact[]>([]);
  const [contactsErr, setContactsErr] = useState<string | null>(null);
  const [selected, setSelected] = useState<BisonrelayContact | null>(null);
  const [messages, setMessages] = useState<BisonrelayMessage[]>([]);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [messagesErr, setMessagesErr] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [showInviteCreate, setShowInviteCreate] = useState(false);
  const [showInviteAccept, setShowInviteAccept] = useState(false);
  const [attachment, setAttachment] = useState<BisonrelayPMAttachment | null>(null);
  const [attachErr, setAttachErr] = useState<string | null>(null);
  const [viewerImage, setViewerImage] = useState<ViewerImage | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const { unread, clearUnread, setActiveUid, addListener } = useBisonrelayLive();

  const openImageViewer = useCallback<ImageViewerOpenFn>((src, name, mime) => {
    setViewerImage({ src, name, mime });
  }, []);

  const refreshContacts = useCallback(async () => {
    try {
      const entries = await getBisonrelayContacts();
      setContacts(entries);
      setContactsErr(null);
    } catch (err: any) {
      setContactsErr(err?.message || 'Could not load contacts');
    }
  }, []);

  useEffect(() => {
    refreshContacts();
  }, [refreshContacts]);

  const loadMessages = useCallback(async (contact: BisonrelayContact) => {
    const uid = contact.id?.identity;
    if (!uid) return;
    setMessagesLoading(true);
    setMessagesErr(null);
    try {
      const resp = await getBisonrelayMessages(uid, 0, 100);
      setMessages(resp.entries ?? []);
    } catch (err: any) {
      setMessagesErr(err?.message || 'Could not load messages');
    } finally {
      setMessagesLoading(false);
    }
  }, []);

  useEffect(() => {
    if (selected) {
      loadMessages(selected);
      const uid = selected.id?.identity ?? '';
      setActiveUid(uid);
      clearUnread(uid);
    } else {
      setMessages([]);
      setActiveUid('');
    }
  }, [selected, loadMessages, setActiveUid, clearUnread]);

  const selectedRef = useRef<BisonrelayContact | null>(null);
  useEffect(() => {
    selectedRef.current = selected;
  }, [selected]);

  useEffect(() => {
    return addListener((evt) => {
      if (evt.type === 'kx') {
        refreshContacts();
        return;
      }
      if (evt.type === 'pm') {
        const payload = evt.payload ?? {};
        const senderNick = payload.nick ?? '';
        const text = payload.msg?.message ?? '';
        const fromUid = identityFromPayload(payload);
        const cur = selectedRef.current;
        if (cur && cur.id?.identity && fromUid === cur.id.identity) {
          setMessages((prev) => [
            ...prev,
            {
              message: text,
              from: senderNick,
              timestamp: Math.floor(Date.now() / 1000),
              internal: false,
            },
          ]);
        }
      }
    });
  }, [addListener, refreshContacts]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selected || sending) return;
    if (!draft.trim() && !attachment) return;
    const recipient = nickOrUid(selected);
    const text = draft.trim();
    setSending(true);
    try {
      const result = await sendBisonrelayPM(recipient, text, attachment ?? undefined);
      setMessages((prev) => [
        ...prev,
        {
          message: result.body || text,
          from: ownNick,
          timestamp: Math.floor(Date.now() / 1000),
          internal: true,
        },
      ]);
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
    if (f.size > MAX_INLINE_BYTES) {
      setAttachErr(`File is ${formatBytes(f.size)}. Inline attachments cap at ${formatBytes(MAX_INLINE_BYTES)}.`);
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
      setAttachment({ name: f.name, mime: f.type || 'application/octet-stream', data_b64: dataB64 });
      setAttachErr(null);
    } catch (err: any) {
      setAttachErr(err?.message || 'Could not read file');
    }
  };

  return (
    <ImageViewerCtx.Provider value={openImageViewer}>
    <div className="flex gap-4 h-[calc(100vh-12rem)] min-h-[480px]">
      <aside className="w-72 flex flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        <div className="p-3 border-b border-border/50 flex items-center justify-between">
          <h3 className="text-sm font-semibold">Contacts</h3>
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
          </div>
        </div>
        <div className="flex-1 overflow-y-auto p-2 space-y-1">
          {contactsErr && (
            <p className="text-xs text-destructive p-2">{contactsErr}</p>
          )}
          {contacts.length === 0 && !contactsErr && (
            <p className="text-xs text-muted-foreground p-2">
              No contacts yet. Create an invite and share it out-of-band, or
              paste a peer's invite to start a key exchange.
            </p>
          )}
          {contacts.map((c) => {
            const nick = displayNick(c);
            const isSel = selected?.id?.identity && c.id?.identity === selected.id.identity;
            const uid = c.id?.identity ?? '';
            const count = uid ? unread[uid] ?? 0 : 0;
            return (
              <button
                key={uid || nick}
                onClick={() => setSelected(c)}
                className={`w-full text-left px-3 py-2 rounded-md transition-colors text-sm flex items-center gap-2 ${
                  isSel ? 'bg-primary/20 text-foreground' : 'hover:bg-muted/30 text-muted-foreground'
                }`}
              >
                <ContactAvatar contact={c} nick={nick} />
                <span className="truncate flex-1">{nick}</span>
                {count > 0 && (
                  <span className="shrink-0 inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold">
                    {count > 99 ? '99+' : count}
                  </span>
                )}
              </button>
            );
          })}
        </div>
      </aside>

      <section className="flex-1 flex flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        {!selected ? (
          <EmptyThread onCreate={() => setShowInviteCreate(true)} onAccept={() => setShowInviteAccept(true)} />
        ) : (
          <>
            <header className="p-3 border-b border-border/50">
              <h3 className="text-sm font-semibold">{displayNick(selected)}</h3>
              {selected.id?.identity && (
                <p className="text-[10px] text-muted-foreground font-mono break-all mt-0.5">
                  {selected.id.identity}
                </p>
              )}
            </header>
            <div className="flex-1 overflow-y-auto p-3 space-y-2">
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
                <MessageList messages={messages} ownNick={ownNick} />
              )}
            </div>
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
              <div className="flex gap-2">
                <input
                  ref={fileInputRef}
                  type="file"
                  className="hidden"
                  onChange={handleAttachPick}
                />
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={sending}
                  title="Attach a file (max 800 KiB)"
                  className="p-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
                >
                  <Paperclip className="h-4 w-4" />
                </button>
                <input
                  type="text"
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  placeholder="Type a message…"
                  disabled={sending}
                  className="flex-1 px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
                />
                <button
                  type="submit"
                  disabled={(!draft.trim() && !attachment) || sending}
                  className="px-3 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
                >
                  {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                  <span>Send</span>
                </button>
              </div>
            </form>
          </>
        )}
      </section>

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
    </div>
    </ImageViewerCtx.Provider>
  );
};

const MessageList = ({ messages, ownNick }: { messages: BisonrelayMessage[]; ownNick: string }) => {
  const endRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages.length]);
  return (
    <>
      {messages.map((m, i) => {
        const own = m.from === ownNick || m.internal;
        return (
          <div key={i} className={`flex ${own ? 'justify-end' : 'justify-start'}`}>
            <div
              className={`max-w-[75%] rounded-lg px-3 py-1.5 text-sm ${
                own ? 'bg-primary/20 text-foreground' : 'bg-muted/30 text-foreground'
              }`}
            >
              <MessageBody body={m.message} />
              <p className="text-[10px] text-muted-foreground mt-0.5">
                {m.from} · {new Date(m.timestamp * 1000).toLocaleString()}
              </p>
            </div>
          </div>
        );
      })}
      <div ref={endRef} />
    </>
  );
};

const EmptyThread = ({
  onCreate,
  onAccept,
}: {
  onCreate: () => void;
  onAccept: () => void;
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
  </div>
);

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

function displayNick(c: BisonrelayContact): string {
  return c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || 'unknown';
}

function nickOrUid(c: BisonrelayContact): string {
  return c.nick_alias || c.id?.nick || c.id?.identity || '';
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

function avatarDataUrl(b64?: string): string {
  if (!b64) return '';
  try {
    const bin = atob(b64);
    if (bin.length < 4) return '';
    const b0 = bin.charCodeAt(0);
    const b1 = bin.charCodeAt(1);
    const b2 = bin.charCodeAt(2);
    const b3 = bin.charCodeAt(3);
    let mime = '';
    if (b0 === 0x89 && b1 === 0x50 && b2 === 0x4e && b3 === 0x47) mime = 'image/png';
    else if (b0 === 0xff && b1 === 0xd8 && b2 === 0xff) mime = 'image/jpeg';
    else if (b0 === 0x47 && b1 === 0x49 && b2 === 0x46) mime = 'image/gif';
    else if (b0 === 0x52 && b1 === 0x49 && b2 === 0x46 && b3 === 0x46 && bin.length > 11 &&
      bin.substring(8, 12) === 'WEBP') mime = 'image/webp';
    if (!mime) return '';
    return `data:${mime};base64,${b64}`;
  } catch {
    return '';
  }
}

const MessageBody = ({ body }: { body: string }) => {
  const segments = parseEmbeds(body);
  return (
    <div className="space-y-1">
      {segments.map((seg, i) => {
        if (seg.kind === 'text') {
          if (!seg.text.trim()) return null;
          return (
            <p key={i} className="whitespace-pre-wrap break-words">
              {seg.text}
            </p>
          );
        }
        return <EmbedRenderer key={i} embed={seg} />;
      })}
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

const ImageViewerModal = ({
  image,
  onClose,
}: {
  image: ViewerImage;
  onClose: () => void;
}) => {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-label={image.name}
    >
      <div className="absolute top-3 right-3 flex items-center gap-2">
        <a
          href={image.src}
          download={image.name}
          onClick={(e) => e.stopPropagation()}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-background/80 hover:bg-background text-foreground text-xs font-medium"
          title="Download"
        >
          <Download className="h-4 w-4" />
          <span>Download</span>
        </a>
        <button
          type="button"
          onClick={onClose}
          className="p-1.5 rounded-md bg-background/80 hover:bg-background text-foreground"
          title="Close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <img
        src={image.src}
        alt={image.name}
        onClick={(e) => e.stopPropagation()}
        className="max-h-[92vh] max-w-[92vw] object-contain rounded shadow-2xl"
      />
    </div>
  );
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
  attachment: BisonrelayPMAttachment;
  onRemove: () => void;
}) => {
  const isImage = isImageMime(attachment.mime);
  const bytes = Math.floor((attachment.data_b64.length * 3) / 4);
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded-md border border-border/40 bg-muted/10">
      {isImage ? (
        <img
          src={`data:${attachment.mime};base64,${attachment.data_b64}`}
          alt={attachment.name}
          className="h-10 w-10 rounded object-cover"
        />
      ) : (
        <FileText className="h-5 w-5 text-muted-foreground shrink-0" />
      )}
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{attachment.name}</p>
        <p className="text-[10px] text-muted-foreground">
          {attachment.mime || 'unknown'} · {formatBytes(bytes)}
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

const avatarPalette = [
  'bg-rose-600', 'bg-amber-600', 'bg-emerald-600', 'bg-teal-600',
  'bg-sky-600', 'bg-indigo-600', 'bg-fuchsia-600', 'bg-pink-600',
];

function colorForUid(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return avatarPalette[h % avatarPalette.length];
}
