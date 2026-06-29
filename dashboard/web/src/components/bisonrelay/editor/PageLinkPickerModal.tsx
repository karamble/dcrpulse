// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Loader2, X } from 'lucide-react';
import {
  BisonrelayLocalPage,
  listBisonrelayLocalPages,
} from '../../../services/bisonrelayApi';

interface Props {
  onClose: () => void;
  onSubmit: (target: string) => void;
}

// PageLinkPickerModal lists the user's own hosted pages so a page link can be
// inserted by picking one (the relative page name is the link target) instead
// of typing the path from memory. The manual field keeps the
// br://<uid>/<path> (or any custom href) escape hatch for cross-user links.
export const PageLinkPickerModal = ({ onClose, onSubmit }: Props) => {
  const [pages, setPages] = useState<BisonrelayLocalPage[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [filter, setFilter] = useState('');
  const [manual, setManual] = useState('');

  useEffect(() => {
    listBisonrelayLocalPages()
      .then((list) => {
        list.sort((a, b) => a.name.localeCompare(b.name));
        setPages(list);
      })
      .catch((e: any) => {
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load pages');
      });
  }, []);

  // Plain action, not a form submit: the editor can sit inside another <form>
  // (the post composer), so a nested form would submit unpredictably.
  const pick = (target: string) => {
    const t = target.trim();
    if (!t) return;
    onSubmit(t);
    onClose();
  };

  const q = filter.trim().toLowerCase();
  const shown = (pages ?? []).filter((p) => !q || p.name.toLowerCase().includes(q));

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl flex flex-col max-h-[80vh]"
      >
        <div className="p-5 pb-3 flex items-start justify-between">
          <div>
            <h3 className="text-base font-semibold">Insert page link</h3>
            <p className="text-xs text-muted-foreground mt-1">
              Pick one of your pages to link to it, or enter a path / br:// link below.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {pages && pages.length > 0 && (
          <div className="px-5 pb-2">
            <input
              type="text"
              autoFocus
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter pages…"
              className="w-full px-3 py-1.5 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
            />
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-2 pb-2 min-h-[140px]">
          {err ? (
            <div className="flex items-start gap-2 text-sm text-destructive px-3 py-4">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          ) : pages === null ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground px-3 py-4">
              <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              <span>Loading pages…</span>
            </div>
          ) : pages.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              You have no saved pages yet. Use the field below to enter a path or br:// link.
            </p>
          ) : shown.length === 0 ? (
            <p className="text-xs text-muted-foreground px-3 py-4 text-center">
              No pages match that filter.
            </p>
          ) : (
            shown.map((p) => (
              <button
                key={p.name}
                type="button"
                onClick={() => pick(p.name)}
                className="w-full text-left px-3 py-2 rounded-md text-sm flex flex-col gap-0.5 hover:bg-muted/30 transition-colors"
              >
                <span className="truncate font-medium text-foreground">{p.name}</span>
                <span className="text-[10px] text-muted-foreground">
                  {formatBytes(p.size)}
                  {p.modified ? ` · ${new Date(p.modified * 1000).toLocaleDateString()}` : ''}
                </span>
              </button>
            ))
          )}
        </div>

        <div className="px-5 py-3 border-t border-border/40 flex items-center gap-2">
          <input
            type="text"
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                pick(manual);
              }
            }}
            placeholder="or a path / br://<uid>/<path>"
            className="flex-1 min-w-0 px-3 py-1.5 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary"
          />
          <button
            type="button"
            disabled={!manual.trim()}
            onClick={() => pick(manual)}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed shrink-0"
          >
            Insert
          </button>
        </div>
      </div>
    </div>
  );
};

function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}
