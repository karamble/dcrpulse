// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useRef, useState } from 'react';
import { Baseline, Bold, Code, Italic, Link2, Strikethrough } from 'lucide-react';

interface Props {
  // onWrap wraps the composer's current selection with the given markdown
  // delimiters (or inserts the placeholder when nothing is selected).
  onWrap: (left: string, right: string, placeholder: string) => void;
  disabled?: boolean;
}

const items: {
  label: string;
  icon: ComponentType<{ className?: string }>;
  left: string;
  right: string;
  placeholder: string;
}[] = [
  { label: 'Bold', icon: Bold, left: '**', right: '**', placeholder: 'bold' },
  { label: 'Italic', icon: Italic, left: '*', right: '*', placeholder: 'italic' },
  { label: 'Code', icon: Code, left: '`', right: '`', placeholder: 'code' },
  { label: 'Strikethrough', icon: Strikethrough, left: '~~', right: '~~', placeholder: 'text' },
  { label: 'Link', icon: Link2, left: '[', right: '](url)', placeholder: 'link text' },
];

// ChatFormatMenu is a Baseline toggle that opens a small popover of inline
// markdown formatters; each wraps the composer's current selection via onWrap.
// Mirrors EmojiPicker's open / outside-click / Escape behaviour.
export const ChatFormatMenu = ({ onWrap, disabled }: Props) => {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement | null>(null);

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

  return (
    <div ref={wrapRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        title="Text formatting"
        aria-label="Text formatting"
        className={`shrink-0 p-2 rounded-lg transition-colors disabled:opacity-50 ${
          open
            ? 'bg-muted/40 text-foreground'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
        }`}
      >
        <Baseline className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 right-0 z-40 flex items-center gap-0.5 rounded-xl bg-card border border-border/50 shadow-xl p-1">
          {items.map((it) => (
            <button
              key={it.label}
              type="button"
              title={it.label}
              aria-label={it.label}
              onClick={() => onWrap(it.left, it.right, it.placeholder)}
              className="h-8 w-8 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/40 transition-colors"
            >
              <it.icon className="h-4 w-4" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
