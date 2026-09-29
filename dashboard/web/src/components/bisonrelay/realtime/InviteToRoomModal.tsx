// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useMemo, useState } from 'react';
import { AlertCircle, Loader2, UserPlus, X } from 'lucide-react';
import {
  RTDTSession,
  inviteToRTDTSession,
} from '../../../services/bisonrelayApi';
import { apiError } from '../../../utils/apiError';
import { ContactList, filterContacts, useContacts, useUidSelection } from '../ContactList';
import { Modal } from '../Modal';

// InviteToRoomModal lets an admin invite additional contacts to an
// existing RTDT session, filtering out any contact already in the
// member list. Capacity (size) caps how many can be selected.
export const InviteToRoomModal = ({
  session,
  onClose,
  onInvited,
}: {
  session: RTDTSession;
  onClose: () => void;
  onInvited: () => void;
}) => {
  const { contacts, error: loadErr } = useContacts();
  const { selected: selectedUids, toggle } = useUidSelection();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const memberUids = useMemo(() => new Set(session.members.map((m) => m.uid)), [session]);
  const remainingSeats = Math.max(0, session.size - session.members.length);

  const candidates = contacts && filterContacts(contacts).filter((c) => !memberUids.has(c.id?.identity ?? ''));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy || selectedUids.size === 0) return;
    setBusy(true);
    setErr(null);
    try {
      await inviteToRTDTSession(session.rv, Array.from(selectedUids), true);
      onInvited();
    } catch (e: any) {
      setErr(apiError(e, 'Invite failed'));
      setBusy(false);
    }
  };

  return (
    <Modal
      onClose={onClose}
      busy={busy}
      className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[85vh]"
    >
      <form onSubmit={handleSubmit} className="flex flex-col min-h-0 flex-1">
        <div className="p-5 pb-3 space-y-3">
          <div className="flex items-start justify-between">
            <h3 className="text-base font-semibold pr-4 flex items-center gap-2">
              <UserPlus className="h-4 w-4 text-primary" /> Invite to room
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
            {remainingSeats} of {session.size} seats free. Already in the
            room: {session.members.length} members.
          </p>
          {(err || loadErr) && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span className="break-words">{err || loadErr}</span>
            </div>
          )}
        </div>
        <div className="px-2 pb-2 flex-1 overflow-y-auto min-h-[120px]">
          {!loadErr && (
            <ContactList
              contacts={candidates}
              emptyText="Everyone you have KX'd with is already in this room."
              busy={busy}
              selected={selectedUids}
              onPick={(c) => toggle(c.id?.identity ?? '', remainingSeats)}
              isDisabled={(uid) => !selectedUids.has(uid) && selectedUids.size >= remainingSeats}
            />
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
    </Modal>
  );
};
