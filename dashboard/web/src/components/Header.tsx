// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, useLocation } from 'react-router-dom';
import { Wallet, Compass, Vote } from 'lucide-react';
import { useBisonrelayLive } from './bisonrelay/BisonrelayLiveProvider';

interface HeaderProps {
  nodeVersion?: string;
}

export const Header = ({ nodeVersion }: HeaderProps) => {
  const location = useLocation();
  const isWalletPage = location.pathname.startsWith('/wallet');
  const isExplorerPage = location.pathname.startsWith('/explorer');
  const isTreasuryPage = location.pathname.startsWith('/treasury');
  const isBisonrelayPage = location.pathname.startsWith('/br');
  const isDexPage = location.pathname.startsWith('/dex');
  const isNodePage = location.pathname === '/';

  // Unread Bison Relay messages, surfaced on the nav button so they are visible
  // from any tab. PMs show a numeric count; group-chat activity shows a dot.
  const { totalUnread, totalGCUnread } = useBisonrelayLive();
  const unreadLabelParts: string[] = [];
  if (totalUnread > 0) {
    unreadLabelParts.push(`${totalUnread} unread direct message${totalUnread === 1 ? '' : 's'}`);
  }
  if (totalGCUnread > 0) {
    unreadLabelParts.push('unread group-chat activity');
  }
  const unreadLabel = unreadLabelParts.join(', ');

  return (
    <div className="flex items-center justify-between mb-8 animate-fade-in">
      <Link to="/" className="shrink-0">
        <img
          src="/images/decred-logo.svg"
          alt="Decred"
          className="h-[72px] w-auto"
        />
      </Link>
      <div className="flex items-center gap-4">
        {/* Navigation Buttons */}
        <Link
          to="/"
          className={`px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isNodePage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary p-2">
            <img src="/images/dcrpulse.svg" alt="Decred" className="w-full h-full" />
          </div>
          <span className="text-primary font-semibold">Node</span>
        </Link>

        <Link
          to="/wallet"
          className={`px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isWalletPage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
            <Wallet className="h-5 w-5 text-white" />
          </div>
          <span className="text-primary font-semibold">Wallet</span>
        </Link>

        <Link
          to="/explorer"
          className={`px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isExplorerPage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
            <Compass className="h-5 w-5 text-white" />
          </div>
          <span className="text-primary font-semibold">Explorer</span>
        </Link>

        <Link
          to="/treasury"
          className={`px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isTreasuryPage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
            <Vote className="h-5 w-5 text-white" />
          </div>
          <span className="text-primary font-semibold">Treasury</span>
        </Link>

        <Link
          to="/br"
          className={`relative px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isBisonrelayPage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
            <img src="/images/bisonrelay.svg" alt="Bison Relay" className="h-6 w-auto" />
          </div>
          <span className="text-primary font-semibold">Bison Relay</span>
          {(totalUnread > 0 || totalGCUnread > 0) && (
            <span
              className="absolute -top-1 -right-1 flex items-center gap-0.5"
              aria-label={unreadLabel}
            >
              {totalUnread > 0 && (
                <span className="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold ring-2 ring-background">
                  {totalUnread > 99 ? '99+' : totalUnread}
                </span>
              )}
              {totalGCUnread > 0 && (
                <span
                  className="h-2 w-2 rounded-full bg-primary ring-2 ring-background"
                  title="Unread group-chat messages"
                />
              )}
            </span>
          )}
        </Link>

        <Link
          to="/dex"
          className={`px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
            isDexPage
              ? 'bg-primary/20 border-primary/40'
              : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
          }`}
        >
          <div className="h-10 w-auto rounded-lg flex items-center justify-center bg-gradient-primary px-2">
            <img src="/images/bisonwallet.svg" alt="Bison Wallet" className="h-6 w-auto" />
          </div>
          <span className="text-primary font-semibold">DEX</span>
        </Link>

        {nodeVersion && (
          <div className="px-4 py-2 rounded-lg bg-primary/10 border border-primary/20">
            <p className="text-sm text-muted-foreground">Version</p>
            <p className="text-lg font-semibold text-primary">{nodeVersion}</p>
          </div>
        )}
      </div>
    </div>
  );
};

