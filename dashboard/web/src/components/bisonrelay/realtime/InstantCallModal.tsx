// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Loader2, Phone, X } from 'lucide-react';
import {
  BisonrelayContact,
  createInstantRTDTSession,
  getBisonrelayContacts,
  joinRTDTSession,
} from '../../../services/bisonrelayApi';

const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || '(unnamed)';

// InstantCallModal lets the user pick a single contact to start a 1:1
// instant call. On success, the wrapper component routes to the active
// call view via onJoined(rv).
export const InstantCallModal = ({
  onClose,
  onJoined,
}: {
  onClose: () => void;
  onJoined: (rv: string) => void;
}) => {
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getBisonrelayContacts()
      .then((c) => {
        setContacts(c);
        setLoadErr(null);
      })
      .catch((e) => {
        const body = e?.response?.data;
        setLoadErr(typeof body === 'string' ? body : e?.message || 'Could not load contacts');
      });
  }, []);

  const handlePick = async (c: BisonrelayContact) => {
    const uid = c.id?.identity;
    if (!uid || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const sess = await createInstantRTDTSession([uid]);
      // CreateInstantRTDTSession on the BR side auto-joins the live
      // session for the creator, but the join is asynchronous; explicitly
      // call /join to ensure the live RTDT manager is up before the WS
      // tries to attach an audio sink to it.
      try {
        await joinRTDTSession(sess.rv);
      } catch {
        /* OK if already joined */
      }
      onJoined(sess.rv);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Call failed');
      setBusy(false);
    }
  };

  const q = query.trim().toLowerCase();
  const filtered =
    contacts?.filter((c) => {
      if (!c.id?.identity) return false;
      if (!q) return true;
      return displayNick(c).toLowerCase().includes(q);
    }) ?? null;

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
            <h3 className="text-base font-semibold pr-4 flex items-center gap-2">
              <Phone className="h-4 w-4 text-primary" /> Instant call
            </h3>
            <button
              type="button"
              onClick={onClose}
              disabled={busy}
              className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-40"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <p className="text-xs text-muted-foreground">
            Pick a contact to call. They will receive an invite and the
            session will auto-join on both sides.
          </p>
          <input
            type="text"
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search contacts…"
            disabled={busy}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
          />
          {err && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}
          {loadErr && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span className="break-words">{loadErr}</span>
            </div>
          )}
        </div>
        <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[120px]">
          {filtered === null && !loadErr ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
              <Loader2 className="h-3 w-3 animate-spin" />
              <span>Loading contacts…</span>
            </div>
          ) : filtered && filtered.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              {contacts && contacts.length === 0
                ? 'You have no contacts yet. KX with someone first.'
                : 'No contacts match your search.'}
            </p>
          ) : (
            filtered?.map((c) => {
              const uid = c.id?.identity ?? '';
              return (
                <button
                  key={uid}
                  type="button"
                  onClick={() => handlePick(c)}
                  disabled={busy}
                  className="w-full px-3 py-2 rounded-md text-left flex items-center gap-2 hover:bg-muted/30 text-foreground text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {busy && <Loader2 className="h-3 w-3 animate-spin shrink-0" />}
                  <span className="truncate">{displayNick(c)}</span>
                  <span className="ml-auto text-[10px] text-muted-foreground font-mono shrink-0">
                    {uid.slice(0, 8)}…
                  </span>
                </button>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
};
