// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Coins, Loader2, UserRound } from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayLiveEvent,
  tipBisonrelayContact,
} from '../../services/bisonrelayApi';
import { AuthorAvatar } from './AuthorAvatar';
import { TipModal } from './TipModal';
import { BisonrelayUserSubNav } from './BisonrelayUserSubNav';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

// displayNick is the shared contact-nick fallback used across the BR UI.
const displayNick = (c: BisonrelayContact): string =>
  c.nick_alias || c.id?.nick || (c.id?.identity ?? '').slice(0, 12) || 'unknown';

// BisonrelayUserBar is a compact owner bar (avatar + nickname + Pay tip + View
// profile) for one Bison Relay user. Clicking the avatar opens the shared user
// menu (BisonrelayUserSubNav) - the single source of per-user actions (View
// Profile, View Pages, Pay Tip, Rename, ...), so future menu changes live in
// one place. The menu drawer anchors to the nearest positioned ancestor, so
// place this bar inside a `relative` container to scope the drawer to it.
export const BisonrelayUserBar = ({
  uid,
  contact,
  contacts,
  onContactsChanged,
  className,
}: {
  uid: string;
  contact?: BisonrelayContact;
  contacts: BisonrelayContact[];
  onContactsChanged?: () => void;
  className?: string;
}) => {
  const [showTip, setShowTip] = useState(false);
  const [showMenu, setShowMenu] = useState(false);
  const [tipStatus, setTipStatus] = useState<{
    state: 'requesting' | 'paying' | 'sent' | 'failed';
    line: string;
  } | null>(null);
  const { addListener } = useBisonrelayLive();

  const nick = contact ? displayNick(contact) : `${uid.slice(0, 8)}…`;
  const avatarB64 = contact?.id?.avatar;

  // Track the tip outcome from the live event stream, the same way the user
  // profile header does.
  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'tip-invoice-generated') {
        const p = (evt.payload ?? {}) as Record<string, unknown>;
        if (String(p.uid ?? '') !== uid) return;
        setTipStatus((prev) =>
          prev && prev.state === 'requesting'
            ? { state: 'paying', line: `Invoice received, paying tip to ${nick}...` }
            : prev,
        );
        return;
      }
      if (evt.type === 'tip-sent' || evt.type === 'tip-failed') {
        const p = (evt.payload ?? {}) as Record<string, string>;
        if (p.recipient !== uid || !p.line) return;
        setTipStatus({ state: evt.type === 'tip-sent' ? 'sent' : 'failed', line: p.line });
      }
    });
  }, [addListener, uid, nick]);

  const submitTip = (dcrAmount: number) => {
    setTipStatus({
      state: 'requesting',
      line: `Requesting invoice for ${dcrAmount} DCR to tip ${nick}...`,
    });
    tipBisonrelayContact(uid, dcrAmount).catch((e: any) => {
      const body = e?.response?.data;
      const msg = typeof body === 'string' ? body : e?.message || 'Tip failed';
      setTipStatus({ state: 'failed', line: `Tip of ${dcrAmount} DCR failed: ${msg}` });
    });
  };

  const btnCls =
    'inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border/50 bg-muted/20 text-xs font-medium text-foreground hover:bg-muted/30 transition-colors';

  return (
    <div className={`flex flex-col items-end gap-1 ${className ?? ''}`}>
      <div className="flex items-center gap-2">
        {contact ? (
          <span
            role="button"
            tabIndex={0}
            aria-label={`User actions for ${nick}`}
            title={`${nick} - open menu`}
            onClick={(e) => {
              e.stopPropagation();
              setShowMenu(true);
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                setShowMenu(true);
              }
            }}
            className="inline-flex shrink-0 rounded-full cursor-pointer transition-shadow hover:ring-2 hover:ring-primary/50"
          >
            <AuthorAvatar uid={uid} nick={nick} avatarB64={avatarB64} size="sm" />
          </span>
        ) : (
          <AuthorAvatar uid={uid} nick={nick} avatarB64={avatarB64} size="sm" />
        )}
        <span className="max-w-[8rem] truncate text-sm font-medium text-foreground" title={nick}>
          {nick}
        </span>
        <button
          type="button"
          onClick={() => setShowTip(true)}
          className={btnCls}
          title={`Pay a tip to ${nick}`}
        >
          <Coins className="h-3.5 w-3.5" />
          <span className="hidden sm:inline">Pay tip</span>
        </button>
        <button
          type="button"
          onClick={() => navigateTo(`feed/user/${uid}`)}
          className={btnCls}
          title={`View ${nick}'s profile`}
        >
          <UserRound className="h-3.5 w-3.5" />
          <span className="hidden sm:inline">View profile</span>
        </button>
      </div>

      {tipStatus && (
        <div
          className={`flex items-center gap-1.5 text-[11px] ${
            tipStatus.state === 'sent'
              ? 'text-success'
              : tipStatus.state === 'failed'
                ? 'text-destructive'
                : 'text-muted-foreground'
          }`}
        >
          {(tipStatus.state === 'requesting' || tipStatus.state === 'paying') && (
            <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
          )}
          <span className="max-w-[16rem] truncate" title={tipStatus.line}>
            {tipStatus.line}
          </span>
        </div>
      )}

      {showTip && (
        <TipModal nick={nick} uid={uid} onClose={() => setShowTip(false)} onSubmit={submitTip} />
      )}

      {showMenu && contact && (
        <BisonrelayUserSubNav
          contact={contact}
          nick={nick}
          contacts={contacts}
          displayNick={displayNick}
          onClose={() => setShowMenu(false)}
          onSendFile={() => {
            setShowMenu(false);
            navigateTo(`chat/${uid}`);
          }}
          onRenamed={() => onContactsChanged?.()}
          onContactsChanged={() => onContactsChanged?.()}
        />
      )}
    </div>
  );
};
