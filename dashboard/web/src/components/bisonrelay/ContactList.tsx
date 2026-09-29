// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Check, Loader2 } from 'lucide-react';
import { BisonrelayContact, getBisonrelayContacts } from '../../services/bisonrelayApi';
import { apiError } from '../../utils/apiError';
import { displayNick } from './bisonrelayNick';

// useContacts loads the contact list once for a picker.
export const useContacts = () => {
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    getBisonrelayContacts()
      .then(setContacts)
      .catch((e) => setError(apiError(e, 'Could not load contacts')));
  }, []);
  return { contacts, error };
};

// filterContacts keeps the contacts with an identity whose nick contains the
// query, ignoring case.
export const filterContacts = (contacts: BisonrelayContact[], query = '') => {
  const q = query.trim().toLowerCase();
  return contacts.filter((c) => c.id?.identity && (!q || displayNick(c).toLowerCase().includes(q)));
};

// useUidSelection holds a multi-select's picked identities; toggle adds one
// only while fewer than limit are picked.
export const useUidSelection = () => {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const toggle = (uid: string, limit = Infinity) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(uid)) next.delete(uid);
      else if (next.size < limit) next.add(uid);
      return next;
    });
  return { selected, toggle };
};

// ContactList renders a picker's rows, with a checkbox per row when selected
// is given. null contacts means still loading.
export const ContactList = ({
  contacts,
  emptyText,
  busy,
  onPick,
  selected,
  isDisabled,
}: {
  contacts: BisonrelayContact[] | null;
  emptyText: string;
  busy?: boolean;
  onPick: (c: BisonrelayContact) => void;
  selected?: Set<string>;
  isDisabled?: (uid: string) => boolean;
}) => {
  if (contacts === null) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
        <Loader2 className="h-3 w-3 animate-spin" />
        <span>Loading contacts…</span>
      </div>
    );
  }
  if (contacts.length === 0) {
    return <p className="text-xs text-muted-foreground px-3 py-4 text-center">{emptyText}</p>;
  }
  return (
    <>
      {contacts.map((c) => {
        const uid = c.id?.identity ?? '';
        const on = selected?.has(uid) ?? false;
        return (
          <button
            key={uid}
            type="button"
            onClick={() => onPick(c)}
            disabled={busy || isDisabled?.(uid)}
            className={`w-full px-3 py-2 rounded-md text-left flex items-center gap-2 text-sm transition-colors ${
              on ? 'bg-primary/15 text-primary' : 'text-foreground hover:bg-muted/30'
            } disabled:opacity-50 disabled:cursor-not-allowed`}
          >
            {selected ? (
              <span
                className={`h-4 w-4 rounded border flex items-center justify-center shrink-0 ${
                  on ? 'border-primary bg-primary text-white' : 'border-border/60'
                }`}
              >
                {on && <Check className="h-3 w-3" />}
              </span>
            ) : (
              busy && <Loader2 className="h-3 w-3 animate-spin shrink-0" />
            )}
            <span className="truncate">{displayNick(c)}</span>
            <span className="ml-auto text-[10px] text-muted-foreground font-mono shrink-0">{uid.slice(0, 8)}…</span>
          </button>
        );
      })}
    </>
  );
};
