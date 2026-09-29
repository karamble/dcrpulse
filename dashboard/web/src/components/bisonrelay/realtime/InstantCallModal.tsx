// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, Phone, X } from 'lucide-react';
import {
  BisonrelayContact,
  createInstantRTDTSession,
} from '../../../services/bisonrelayApi';
import { apiError } from '../../../utils/apiError';
import { ContactList, filterContacts, useContacts } from '../ContactList';
import { Modal } from '../Modal';

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
  const { contacts, error: loadErr } = useContacts();
  const [query, setQuery] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handlePick = async (c: BisonrelayContact) => {
    const uid = c.id?.identity;
    if (!uid || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const sess = await createInstantRTDTSession([uid]);
      // Deliberately no join here. Bison Relay joins the caller itself once
      // the callee accepts, after it has sent out the publisher keys; joining
      // sooner would put us on the wire before the callee can decrypt us.
      onJoined(sess.rv);
    } catch (e: any) {
      setErr(apiError(e, 'Call failed'));
      setBusy(false);
    }
  };

  const filtered = contacts && filterContacts(contacts, query);

  return (
    <Modal
      onClose={onClose}
      busy={busy}
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
        {!loadErr && (
          <ContactList
            contacts={filtered}
            emptyText={
              contacts && contacts.length === 0
                ? 'You have no contacts yet. KX with someone first.'
                : 'No contacts match your search.'
            }
            busy={busy}
            onPick={handlePick}
          />
        )}
      </div>
    </Modal>
  );
};
