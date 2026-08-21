// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useRef, useState } from 'react';
import { Archive, Download, Paperclip, Upload } from 'lucide-react';

interface Props {
  // A group cannot receive a direct file transfer, so the first row offers
  // only what a group message can actually carry.
  group?: boolean;
  onUpload: () => void;
  onOffer: () => void;
  onPickShared: () => void;
  disabled?: boolean;
}

// ChatAttachMenu is the paperclip's popover: one row per way of attaching a
// file, each naming what the other side ends up with. Mirrors EmojiPicker's
// open / outside-click / Escape behaviour.
export const ChatAttachMenu = ({ group, onUpload, onOffer, onPickShared, disabled }: Props) => {
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

  const rows: { label: string; caption: string; icon: ComponentType<{ className?: string }>; run: () => void }[] = [
    {
      label: group ? 'Upload an image' : 'Upload a file',
      caption: group
        ? 'Images up to 800 KB ride inside the message.'
        : 'Sent to them right away, free.',
      icon: Upload,
      run: onUpload,
    },
    {
      label: 'Offer for download',
      caption: group
        ? 'Members pull it when they want. You can set a price.'
        : 'They pull it when they want. You can set a price.',
      icon: Download,
      run: onOffer,
    },
    {
      label: 'From my shared files',
      caption: 'Attach a file you already share.',
      icon: Archive,
      run: onPickShared,
    },
  ];

  return (
    <div ref={wrapRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        disabled={disabled}
        title="Attach a file"
        aria-label="Attach a file"
        aria-expanded={open}
        className={`shrink-0 p-2 rounded-lg transition-colors disabled:opacity-50 ${
          open
            ? 'bg-muted/40 text-foreground'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
        }`}
      >
        <Paperclip className="h-4 w-4" />
      </button>
      {open && (
        <div className="absolute bottom-full mb-1 right-0 z-40 w-64 rounded-xl bg-card border border-border/50 shadow-xl p-1">
          {rows.map((r) => (
            <button
              key={r.label}
              type="button"
              onClick={() => {
                setOpen(false);
                r.run();
              }}
              className="w-full text-left px-3 py-2 rounded-lg hover:bg-muted/30 transition-colors flex items-start gap-2.5"
            >
              <r.icon className="h-4 w-4 mt-0.5 shrink-0 text-muted-foreground" />
              <span className="min-w-0">
                <span className="block text-sm text-foreground">{r.label}</span>
                <span className="block text-[10px] text-muted-foreground">{r.caption}</span>
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
};
