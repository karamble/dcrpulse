// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Check, Loader2, Users, X } from 'lucide-react';
import {
  BisonrelayContact,
  createRTDTSession,
  getBisonrelayContacts,
  inviteToRTDTSession,
  joinRTDTSession,
} from '../../../services/bisonrelayApi';

const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || '(unnamed)';

// NewRoomModal creates a group RTDT room: capacity, description, and an
// initial multi-select set of invitees. The room owner is auto-joined to
// the live audio on success.
export const NewRoomModal = ({
  onClose,
  onJoined,
}: {
  onClose: () => void;
  onJoined: (rv: string) => void;
}) => {
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
  const [size, setSize] = useState(4);
  const [description, setDescription] = useState('');
  const [selectedUids, setSelectedUids] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    getBisonrelayContacts()
      .then(setContacts)
      .catch((e) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load contacts');
      });
  }, []);

  const toggleUid = (uid: string) => {
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) {
        next.delete(uid);
      } else if (next.size < size - 1) {
        next.add(uid);
      }
      return next;
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    if (size < 2) {
      setErr('Capacity must be at least 2');
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      const sess = await createRTDTSession(size, description.trim());
      if (selectedUids.size > 0) {
        try {
          await inviteToRTDTSession(sess.rv, Array.from(selectedUids), true);
        } catch (e: any) {
          const body = e?.response?.data;
          setErr(`Created, but invite failed: ${typeof body === 'string' ? body : e?.message}`);
        }
      }
      try {
        await joinRTDTSession(sess.rv);
      } catch {
        /* allowed to fail if BR auto-joined us */
      }
      onJoined(sess.rv);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Create failed');
      setBusy(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[85vh]"
      >
        <form onSubmit={handleSubmit} className="flex flex-col min-h-0 flex-1">
          <div className="p-5 pb-3 space-y-3">
            <div className="flex items-start justify-between">
              <h3 className="text-base font-semibold pr-4 flex items-center gap-2">
                <Users className="h-4 w-4 text-primary" /> New group room
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
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label
                  htmlFor="rtdt-new-room-size"
                  className="block text-[10px] uppercase tracking-wide text-muted-foreground mb-1"
                >
                  Capacity
                </label>
                <input
                  id="rtdt-new-room-size"
                  type="number"
                  min={2}
                  max={32}
                  value={size}
                  onChange={(e) => setSize(Number(e.target.value))}
                  disabled={busy}
                  className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
                />
              </div>
              <div>
                <label className="block text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
                  Invited
                </label>
                <div className="px-3 py-2 rounded-lg bg-background border border-border text-sm tabular-nums">
                  {selectedUids.size} / {Math.max(0, size - 1)}
                </div>
              </div>
            </div>
            <div>
              <label
                htmlFor="rtdt-new-room-desc"
                className="block text-[10px] uppercase tracking-wide text-muted-foreground mb-1"
              >
                Description (optional)
              </label>
              <input
                id="rtdt-new-room-desc"
                type="text"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={busy}
                maxLength={120}
                placeholder="What is this room about?"
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
              />
            </div>
            {err && (
              <div className="flex items-start gap-2 text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            )}
          </div>
          <div className="px-2 pb-2 flex-1 overflow-y-auto min-h-[120px]">
            <div className="px-3 text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
              Invite contacts
            </div>
            {contacts === null && !err ? (
              <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
                <Loader2 className="h-3 w-3 animate-spin" />
                <span>Loading contacts…</span>
              </div>
            ) : contacts && contacts.length === 0 ? (
              <p className="text-xs text-muted-foreground px-3 py-4">
                You can create an empty room and invite people later.
              </p>
            ) : (
              contacts?.map((c) => {
                const uid = c.id?.identity ?? '';
                if (!uid) return null;
                const selected = selectedUids.has(uid);
                const atCap = !selected && selectedUids.size >= size - 1;
                return (
                  <button
                    key={uid}
                    type="button"
                    onClick={() => toggleUid(uid)}
                    disabled={busy || atCap}
                    className={`w-full px-3 py-2 rounded-md text-left flex items-center gap-2 text-sm transition-colors ${
                      selected
                        ? 'bg-primary/15 text-primary'
                        : 'text-foreground hover:bg-muted/30'
                    } disabled:opacity-50 disabled:cursor-not-allowed`}
                  >
                    <span
                      className={`h-4 w-4 rounded border flex items-center justify-center shrink-0 ${
                        selected
                          ? 'border-primary bg-primary text-white'
                          : 'border-border/60'
                      }`}
                    >
                      {selected && <Check className="h-3 w-3" />}
                    </span>
                    <span className="truncate">{displayNick(c)}</span>
                    <span className="ml-auto text-[10px] text-muted-foreground font-mono shrink-0">
                      {uid.slice(0, 8)}…
                    </span>
                  </button>
                );
              })
            )}
          </div>
          <div className="border-t border-border/40 p-3 flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={busy}
              className="px-3 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {busy && <Loader2 className="h-3 w-3 animate-spin" />}
              Create + Join
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
