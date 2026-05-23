// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle, Loader2, Users, X } from 'lucide-react';
import { BisonrelayGC, createBisonrelayGC } from '../../../services/bisonrelayApi';

// CreateGCModal creates a new GC with the caller as owner. The GC name is
// immutable per BR (the local alias can be changed via /alias later), so
// the input is enforced non-empty and trimmed.
export const CreateGCModal = ({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (gc: BisonrelayGC) => void;
}) => {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setErr(null);
    try {
      const gc = await createBisonrelayGC(trimmed);
      onCreated(gc);
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
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col"
      >
        <form onSubmit={handleSubmit}>
          <div className="p-5 pb-3 space-y-3">
            <div className="flex items-start justify-between">
              <h3 className="text-base font-semibold pr-4 flex items-center gap-2">
                <Users className="h-4 w-4 text-primary" /> New group
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
              You become the owner. Invite members from the group header
              after creation. The name is immutable; use the alias action
              later to rename it locally.
            </p>
            <input
              type="text"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Group name"
              disabled={busy}
              maxLength={64}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
            />
            {err && (
              <div className="flex items-start gap-2 text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
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
              disabled={!name.trim() || busy}
              className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {busy && <Loader2 className="h-3 w-3 animate-spin" />}
              Create
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
