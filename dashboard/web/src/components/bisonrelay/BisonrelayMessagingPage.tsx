// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  Copy,
  Loader2,
  MessageSquare,
  Send,
  UserPlus,
  X,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayLiveEvent,
  BisonrelayMessage,
  acceptBisonrelayInvite,
  getBisonrelayContacts,
  getBisonrelayMessages,
  sendBisonrelayPM,
  writeBisonrelayInvite,
} from '../../services/bisonrelayApi';

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
    if (selected) loadMessages(selected);
    else setMessages([]);
  }, [selected, loadMessages]);

  useEffect(() => {
    const url = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/br/events`;
    const ws = new WebSocket(url);
    ws.onmessage = (e) => {
      try {
        const evt: BisonrelayLiveEvent = JSON.parse(e.data);
        if (evt.type === 'kx') {
          refreshContacts();
          return;
        }
        if (evt.type === 'pm') {
          const payload = evt.payload ?? {};
          const senderNick = payload.nick ?? '';
          const text = payload.msg?.message ?? '';
          const fromUid = identityFromPayload(payload);
          setMessages((prev) => {
            if (selected && selected.id?.identity && fromUid === selected.id.identity) {
              return [
                ...prev,
                {
                  message: text,
                  from: senderNick,
                  timestamp: Math.floor(Date.now() / 1000),
                  internal: false,
                },
              ];
            }
            return prev;
          });
        }
      } catch {
        /* ignore */
      }
    };
    return () => ws.close();
  }, [refreshContacts, selected]);

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selected || !draft.trim() || sending) return;
    const recipient = nickOrUid(selected);
    const text = draft.trim();
    setSending(true);
    try {
      await sendBisonrelayPM(recipient, text);
      setMessages((prev) => [
        ...prev,
        { message: text, from: ownNick, timestamp: Math.floor(Date.now() / 1000), internal: true },
      ]);
      setDraft('');
    } catch (err: any) {
      setMessagesErr(err?.message || 'Send failed');
    } finally {
      setSending(false);
    }
  };

  return (
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
            return (
              <button
                key={c.id?.identity ?? nick}
                onClick={() => setSelected(c)}
                className={`w-full text-left px-3 py-2 rounded-md transition-colors text-sm ${
                  isSel ? 'bg-primary/20 text-foreground' : 'hover:bg-muted/30 text-muted-foreground'
                }`}
              >
                {nick}
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
            <form onSubmit={handleSend} className="p-3 border-t border-border/50 flex gap-2">
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
                disabled={!draft.trim() || sending}
                className="px-3 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
              >
                {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                <span>Send</span>
              </button>
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
    </div>
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
              <p className="whitespace-pre-wrap break-words">{m.message}</p>
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
  if (typeof uid === 'string') return uid;
  return '';
}
