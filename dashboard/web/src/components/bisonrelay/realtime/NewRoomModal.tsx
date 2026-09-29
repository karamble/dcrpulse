// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, Loader2, Users, X } from 'lucide-react';
import {
  createRTDTSession,
  inviteToRTDTSession,
  joinRTDTSession,
} from '../../../services/bisonrelayApi';
import { apiError } from '../../../utils/apiError';
import { ContactList, filterContacts, useContacts, useUidSelection } from '../ContactList';
import { Modal } from '../Modal';

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
  const { contacts, error: loadErr } = useContacts();
  const [size, setSize] = useState(4);
  const [description, setDescription] = useState('');
  const { selected: selectedUids, toggle } = useUidSelection();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

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
          setErr(`Created, but invite failed: ${apiError(e, '')}`);
        }
      }
      try {
        await joinRTDTSession(sess.rv);
      } catch {
        /* allowed to fail if BR auto-joined us */
      }
      onJoined(sess.rv);
    } catch (e: any) {
      setErr(apiError(e, 'Create failed'));
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
          {(err || loadErr) && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span className="break-words">{err || loadErr}</span>
            </div>
          )}
        </div>
        <div className="px-2 pb-2 flex-1 overflow-y-auto min-h-[120px]">
          <div className="px-3 text-[10px] uppercase tracking-wide text-muted-foreground mb-1">
            Invite contacts
          </div>
          {!loadErr && (
            <ContactList
              contacts={contacts && filterContacts(contacts)}
              emptyText="You can create an empty room and invite people later."
              busy={busy}
              selected={selectedUids}
              onPick={(c) => toggle(c.id?.identity ?? '', size - 1)}
              isDisabled={(uid) => !selectedUids.has(uid) && selectedUids.size >= size - 1}
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
            disabled={busy}
            className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {busy && <Loader2 className="h-3 w-3 animate-spin" />}
            Create + Join
          </button>
        </div>
      </form>
    </Modal>
  );
};
