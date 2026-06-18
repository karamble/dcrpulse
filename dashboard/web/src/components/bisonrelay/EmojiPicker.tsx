// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Smile } from 'lucide-react';
import { ChatEmoji, EMOJI_CATEGORIES } from './chatEmojis';

interface EmojiPickerProps {
  onPick: (emoji: string) => void;
  disabled?: boolean;
}

// EmojiPicker renders a Smile button that opens a small popover with a search
// box and a categorized grid of unicode emojis. Picking one calls onPick; the
// popover stays open so several can be added in a row, and closes on
// outside-click or Escape.
export const EmojiPicker = ({ onPick, disabled }: EmojiPickerProps) => {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const searchRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) return undefined;
    const onClick = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onClick);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  useEffect(() => {
    if (open) searchRef.current?.focus();
    else setQuery('');
  }, [open]);

  const matches = useMemo<ChatEmoji[]>(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return EMOJI_CATEGORIES.flatMap((c) => c.emojis).filter((e) =>
      e.keywords.includes(q),
    );
  }, [query]);

  return (
    <div ref={wrapRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        title="Insert emoji"
        aria-label="Insert emoji"
        className="shrink-0 p-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
      >
        <Smile className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 left-0 z-40 w-72 rounded-xl bg-card border border-border/50 shadow-xl">
          <div className="p-2 border-b border-border/30">
            <input
              ref={searchRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search emoji…"
              className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm text-foreground focus:outline-none focus:border-primary"
            />
          </div>
          <div className="max-h-64 overflow-y-auto p-2">
            {query.trim() ? (
              matches.length === 0 ? (
                <p className="text-xs text-muted-foreground text-center py-4">No emoji found.</p>
              ) : (
                <div className="grid grid-cols-8 gap-0.5">
                  {matches.map((e, i) => (
                    <EmojiButton key={`${e.char}-${i}`} emoji={e.char} onPick={onPick} />
                  ))}
                </div>
              )
            ) : (
              EMOJI_CATEGORIES.map((cat) => (
                <div key={cat.name} className="mb-2 last:mb-0">
                  <p className="text-[10px] uppercase tracking-wide text-muted-foreground px-1 mb-1">
                    {cat.name}
                  </p>
                  <div className="grid grid-cols-8 gap-0.5">
                    {cat.emojis.map((e, i) => (
                      <EmojiButton key={`${e.char}-${i}`} emoji={e.char} onPick={onPick} />
                    ))}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
};

const EmojiButton = ({ emoji, onPick }: { emoji: string; onPick: (e: string) => void }) => (
  <button
    type="button"
    onClick={() => onPick(emoji)}
    className="h-8 w-8 flex items-center justify-center rounded-md text-lg hover:bg-muted/40 transition-colors"
  >
    {emoji}
  </button>
);
