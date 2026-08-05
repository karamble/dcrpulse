// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Check } from 'lucide-react';
import { AlertEntry } from '../../services/api';
import { timeAgo, toYMDTime } from '../../utils/date';

const sevDot: Record<string, string> = {
  critical: 'bg-destructive',
  warning: 'bg-warning',
  info: 'bg-muted-foreground/50',
};

// alertCategoryRoute deep-links each category to the page that resolves it.
export const alertCategoryRoute: Record<string, string> = {
  node: '/',
  wallet: '/wallet',
  staking: '/wallet/staking',
  lightning: '/wallet/lightning',
  dex: '/dex',
  bisonrelay: '/br',
  system: '/',
};

// alertDuration renders a resolved condition's outage length compactly.
const alertDuration = (e: AlertEntry): string => {
  if (!e.resolvedAt) return '';
  const mins = Math.floor((new Date(e.resolvedAt).getTime() - new Date(e.createdAt).getTime()) / 60000);
  if (mins < 1) return '<1m';
  if (mins < 60) return `${mins}m`;
  const h = Math.floor(mins / 60);
  if (h < 24) return `${h}h ${mins % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
};

interface AlertRowProps {
  entry: AlertEntry;
  onRead?: (id: string) => void;
  onOpen?: (entry: AlertEntry) => void;
}

// AlertRow is the shared row for the pill panel, the /alerts page, and the
// Node page card: severity dot, title, relative time, explicit read control.
export const AlertRow = ({ entry, onRead, onOpen }: AlertRowProps) => {
  const dur = entry.kind === 'condition' && !entry.active ? alertDuration(entry) : '';
  return (
    <div
      onClick={onOpen ? () => onOpen(entry) : undefined}
      className={`px-3 py-2.5 border-b border-border/30 last:border-b-0${
        onOpen ? ' cursor-pointer hover:bg-muted/30' : ''
      }`}
    >
      <div className="flex items-center gap-2">
        <span
          className={`inline-block h-2 w-2 rounded-full shrink-0 ${sevDot[entry.severity] || sevDot.info}${
            entry.active ? ' animate-pulse' : ''
          }`}
        />
        <span
          className={`text-xs truncate ${
            entry.read && !entry.active ? 'text-muted-foreground' : 'font-medium text-foreground'
          }`}
        >
          {entry.title}
        </span>
        {entry.active && (
          <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded-full bg-muted/30 text-muted-foreground">
            active
          </span>
        )}
        <span
          className="ml-auto shrink-0 text-[10px] text-muted-foreground"
          title={toYMDTime(new Date(entry.createdAt))}
        >
          {timeAgo(entry.createdAt)}
        </span>
        {onRead && !entry.read && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              e.preventDefault();
              onRead(entry.id);
            }}
            aria-label="Mark read"
            title="Mark read"
            className="shrink-0 p-0.5 rounded text-muted-foreground/50 hover:text-primary hover:bg-primary/10 transition-colors"
          >
            <Check className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      {(entry.detail || dur) && (
        <p className="text-[11px] text-muted-foreground mt-1 break-words">
          {entry.detail}
          {dur ? `${entry.detail ? ' · ' : ''}lasted ${dur}` : ''}
        </p>
      )}
    </div>
  );
};
