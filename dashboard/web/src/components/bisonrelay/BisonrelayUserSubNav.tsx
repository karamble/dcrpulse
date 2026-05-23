// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useState } from 'react';
import {
  Check,
  Coins,
  Copy,
  Edit2,
  FileText,
  Folder,
  Handshake,
  List,
  Paperclip,
  RotateCw,
  Rss,
  UserPlus,
  Users,
  X,
} from 'lucide-react';
import { BisonrelayContact } from '../../services/bisonrelayApi';
import { avatarDataUrl, colorForUid } from './bisonrelayAvatar';

// Layout + action ordering mirrors bruig's chat_side_menu / user_context_menu
// (companyzero/bisonrelay, ISC). "User Profile" is collapsed into the header
// card so we render eleven rows; only the first row (Send File) is live in
// P0, the rest are disabled placeholders that later phases unlock.

interface Props {
  contact: BisonrelayContact;
  nick: string;
  onClose: () => void;
  onSendFile: () => void;
}

interface Row {
  id: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  onClick?: () => void;
  comingSoon?: boolean;
}

export const BisonrelayUserSubNav = ({ contact, nick, onClose, onSendFile }: Props) => {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const rows: Row[] = [
    { id: 'tip', label: 'Pay Tip', icon: Coins, comingSoon: true },
    { id: 'kx-reset', label: 'Request Ratchet Reset', icon: RotateCw, comingSoon: true },
    { id: 'content', label: 'Show Content', icon: Folder, comingSoon: true },
    { id: 'subscribe', label: 'Subscribe to Posts', icon: Rss, comingSoon: true },
    { id: 'list-posts', label: 'List Posts', icon: List, comingSoon: true },
    { id: 'send-file', label: 'Send File', icon: Paperclip, onClick: onSendFile },
    { id: 'pages', label: 'View Pages', icon: FileText, comingSoon: true },
    { id: 'rename', label: 'Rename User', icon: Edit2, comingSoon: true },
    { id: 'suggest-kx', label: 'Suggest User to KX', icon: UserPlus, comingSoon: true },
    { id: 'trans-reset', label: 'Issue Transitive Reset', icon: Users, comingSoon: true },
    { id: 'handshake', label: 'Perform Handshake', icon: Handshake, comingSoon: true },
  ];

  return (
    <>
      <div
        className="absolute inset-0 bg-black/20 backdrop-blur-[1px] z-10 rounded-xl"
        onClick={onClose}
        aria-hidden
      />
      <aside
        className="absolute right-0 top-0 bottom-0 w-64 flex flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 z-20 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <Header contact={contact} nick={nick} onClose={onClose} />
        <div className="flex-1 overflow-y-auto py-1">
          {rows.map((r) => (
            <ActionRow key={r.id} row={r} />
          ))}
        </div>
      </aside>
    </>
  );
};

const Header = ({
  contact,
  nick,
  onClose,
}: {
  contact: BisonrelayContact;
  nick: string;
  onClose: () => void;
}) => {
  const [copied, setCopied] = useState(false);
  const identity = contact.id?.identity ?? '';

  const onCopy = async () => {
    if (!identity) return;
    try {
      await navigator.clipboard.writeText(identity);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore clipboard errors */
    }
  };

  return (
    <div className="relative p-4 pb-3 border-b border-border/50">
      <button
        onClick={onClose}
        className="absolute right-2 top-2 p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
        title="Close"
        aria-label="Close"
      >
        <X className="h-4 w-4" />
      </button>
      <div className="flex flex-col items-center text-center gap-2 mt-1">
        <BigAvatar contact={contact} nick={nick} />
        <p className="text-sm font-semibold truncate w-full px-4">{nick}</p>
      </div>
      {identity && (
        <button
          onClick={onCopy}
          className="mt-2 w-full text-[10px] font-mono text-muted-foreground hover:text-foreground bg-muted/20 hover:bg-muted/30 rounded px-2 py-1.5 text-left flex items-start gap-1.5 transition-colors"
          title="Copy identity"
        >
          {copied ? (
            <Check className="h-3 w-3 mt-0.5 shrink-0 text-success" />
          ) : (
            <Copy className="h-3 w-3 mt-0.5 shrink-0" />
          )}
          <span className="flex-1 break-all">{identity}</span>
        </button>
      )}
    </div>
  );
};

const BigAvatar = ({ contact, nick }: { contact: BisonrelayContact; nick: string }) => {
  const dataUrl = avatarDataUrl(contact.id?.avatar);
  const initial = nick.trim().charAt(0).toUpperCase() || '?';
  const bgClass = colorForUid(contact.id?.identity ?? nick);
  if (dataUrl) {
    return (
      <img
        src={dataUrl}
        alt=""
        className="h-16 w-16 rounded-full object-cover bg-muted/30"
      />
    );
  }
  return (
    <span
      className={`h-16 w-16 rounded-full flex items-center justify-center text-2xl font-semibold text-white ${bgClass}`}
    >
      {initial}
    </span>
  );
};

const ActionRow = ({ row }: { row: Row }) => {
  const Icon = row.icon;
  const disabled = !!row.comingSoon || !row.onClick;
  return (
    <button
      onClick={row.onClick}
      disabled={disabled}
      className={`w-full px-4 py-2 text-left text-sm flex items-center gap-3 transition-colors ${
        disabled
          ? 'text-muted-foreground/60 cursor-not-allowed'
          : 'text-foreground hover:bg-muted/30'
      }`}
      title={row.comingSoon ? 'Coming soon' : undefined}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span className="flex-1 truncate">{row.label}</span>
      {row.comingSoon && (
        <span className="shrink-0 text-[9px] uppercase tracking-wide text-muted-foreground/70">
          soon
        </span>
      )}
    </button>
  );
};
