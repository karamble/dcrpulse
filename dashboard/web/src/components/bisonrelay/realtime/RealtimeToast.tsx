// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Phone, X } from 'lucide-react';
import { useRealtimeNotices } from '../../../hooks/useRealtimeNotices';

// RealtimeToast reports calls ending, removals and room admin changes wherever
// the user happens to be. It stacks above the incoming-call bubble and shows
// nothing when there is nothing to say.
export const RealtimeToast = () => {
  const { notices, dismiss } = useRealtimeNotices();
  if (notices.length === 0) return null;

  return (
    <div className="fixed bottom-36 right-4 z-40 w-72 space-y-2">
      {notices.map((n) => (
        <div
          key={n.id}
          className={`rounded-lg shadow-xl ring-1 px-3 py-2.5 flex items-start gap-2 text-xs ${
            n.tone === 'warn'
              ? 'bg-amber-500/15 ring-amber-500/40 text-amber-200'
              : 'bg-card ring-border/60 text-card-foreground'
          }`}
        >
          <Phone className="h-3.5 w-3.5 mt-0.5 shrink-0 opacity-80" />
          <span className="flex-1 break-words">{n.text}</span>
          <button
            type="button"
            onClick={() => dismiss(n.id)}
            aria-label="Dismiss"
            className="shrink-0 opacity-60 hover:opacity-100"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ))}
    </div>
  );
};
