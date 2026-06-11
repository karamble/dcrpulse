// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Check, Loader2, UserPlus, X } from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayGC,
  getBisonrelayContacts,
  inviteToBisonrelayGC,
} from '../../../services/bisonrelayApi';

const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || c.id?.identity?.slice(0, 12) || '(unnamed)';

// GCInviteModal lets an admin invite contacts to an existing GC. Members
// already in the group + already-pending invites are filtered out. BR's
// InviteToGroupChat sends one invite per call so we loop client-side.
export const GCInviteModal = ({
  gc,
  onClose,
  onInvited,
}: {
  gc: BisonrelayGC;
  onClose: () => void;
  onInvited: () => void;
}) => {
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
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

  const memberUids = useMemo(() => new Set(gc.members), [gc.members]);

  const candidates = (contacts ?? []).filter((c) => {
    const uid = c.id?.identity;
    return uid && !memberUids.has(uid);
  });

  const toggleUid = (uid: string) => {
    setSelectedUids((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) {
        next.delete(uid);
      } else {
        next.add(uid);
      }
      return next;
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy || selectedUids.size === 0) return;
    setBusy(true);
    setErr(null);
    const uids = Array.from(selectedUids);
    const failed: string[] = [];
    for (const uid of uids) {
      try {
        await inviteToBisonrelayGC(gc.id, uid);
      } catch (e: any) {
        const body = e?.response?.data;
        const msg = typeof body === 'string' ? body : e?.message || 'invite failed';
        failed.push(`${uid.slice(0, 8)}…: ${msg}`);
      }
    }
    if (failed.length > 0) {
      setErr(`Sent ${uids.length - failed.length}/${uids.length}. Failures:\n${failed.join('\n')}`);
      setBusy(false);
      return;
    }
    onInvited();
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
                <UserPlus className="h-4 w-4 text-primary" /> Invite to {gc.alias || gc.name}
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
              Invites are sent one-by-one over BR. Contacts already in this
              group are hidden. Recipients see the invite in their Chat tab.
            </p>
            {err && (
              <div className="flex items-start gap-2 text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span className="break-words whitespace-pre-wrap">{err}</span>
              </div>
            )}
          </div>
          <div className="px-2 pb-2 flex-1 overflow-y-auto min-h-[120px]">
            {contacts === null && !err ? (
              <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
                <Loader2 className="h-3 w-3 animate-spin" />
                <span>Loading contacts…</span>
              </div>
            ) : candidates.length === 0 ? (
              <p className="text-xs text-muted-foreground px-3 py-4 text-center">
                Everyone you've KX'd with is already in this group.
              </p>
            ) : (
              candidates.map((c) => {
                const uid = c.id?.identity ?? '';
                const selected = selectedUids.has(uid);
                return (
                  <button
                    key={uid}
                    type="button"
                    onClick={() => toggleUid(uid)}
                    disabled={busy}
                    className={`w-full px-3 py-2 rounded-md text-left flex items-center gap-2 text-sm transition-colors ${
                      selected ? 'bg-primary/15 text-primary' : 'text-foreground hover:bg-muted/30'
                    } disabled:opacity-50 disabled:cursor-not-allowed`}
                  >
                    <span
                      className={`h-4 w-4 rounded border flex items-center justify-center shrink-0 ${
                        selected ? 'border-primary bg-primary text-white' : 'border-border/60'
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
              disabled={busy || selectedUids.size === 0}
              className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {busy && <Loader2 className="h-3 w-3 animate-spin" />}
              Send {selectedUids.size} invite{selectedUids.size === 1 ? '' : 's'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
