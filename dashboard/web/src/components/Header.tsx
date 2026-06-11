// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Wallet, Compass, Vote, Menu, X, LogOut } from 'lucide-react';
import { useBisonrelayLive } from './bisonrelay/BisonrelayLiveProvider';
import { useBrNotifPrefs } from './bisonrelay/brNotifPrefs';
import { useAuth } from './auth/AuthGate';
import { logout } from '../services/auth';

interface HeaderProps {
  nodeVersion?: string;
}

export const Header = ({ nodeVersion }: HeaderProps) => {
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const { status: authStatus, refresh: refreshAuth } = useAuth();

  const isWalletPage = location.pathname.startsWith('/wallet');
  const isExplorerPage = location.pathname.startsWith('/explorer');
  const isTreasuryPage = location.pathname.startsWith('/treasury');
  const isBisonrelayPage = location.pathname.startsWith('/br');
  const isDexPage = location.pathname.startsWith('/dex');
  const isNodePage = location.pathname === '/';

  // Unread Bison Relay messages, surfaced on the nav button so they are visible
  // from any tab. PMs show a numeric count; group-chat activity shows a dot.
  // The BR notification switches gate the indicators only; the provider keeps
  // counting, so re-enabling a switch shows the true unread state.
  const { totalUnread, totalGCUnread } = useBisonrelayLive();
  const notifPrefs = useBrNotifPrefs();
  const shownUnread = notifPrefs.dms ? totalUnread : 0;
  const shownGCUnread = notifPrefs.gcMessages ? totalGCUnread : 0;
  const hasUnread = shownUnread > 0 || shownGCUnread > 0;
  const unreadLabelParts: string[] = [];
  if (shownUnread > 0) {
    unreadLabelParts.push(`${shownUnread} unread direct message${shownUnread === 1 ? '' : 's'}`);
  }
  if (shownGCUnread > 0) {
    unreadLabelParts.push('unread group-chat activity');
  }
  const unreadLabel = unreadLabelParts.join(', ');

  // Close the mobile drawer on navigation.
  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  // While the drawer is open, lock body scroll and close on Escape.
  useEffect(() => {
    if (!menuOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('keydown', onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prevOverflow;
    };
  }, [menuOpen]);

  const linkClass = (active: boolean) =>
    `px-4 py-3 rounded-lg border transition-all duration-300 flex items-center gap-2 ${
      active
        ? 'bg-primary/20 border-primary/40'
        : 'bg-primary/10 border-primary/20 hover:bg-primary/20'
    }`;

  const brBadge = hasUnread ? (
    <span className="absolute -top-1 -right-1 flex items-center gap-0.5" aria-label={unreadLabel}>
      {shownUnread > 0 && (
        <span className="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 rounded-full bg-primary text-primary-foreground text-[10px] font-semibold ring-2 ring-background">
          {shownUnread > 99 ? '99+' : shownUnread}
        </span>
      )}
      {shownGCUnread > 0 && (
        <span
          className="h-2 w-2 rounded-full bg-primary ring-2 ring-background"
          title="Unread group-chat messages"
        />
      )}
    </span>
  ) : null;

  // The nav links, reused by the desktop bar and the mobile drawer.
  const navLinks = (
    <>
      <Link to="/" className={linkClass(isNodePage)}>
        <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary p-2">
          <img src="/images/dcrpulse.svg" alt="Decred" className="w-full h-full" />
        </div>
        <span className="text-primary font-semibold">Node</span>
      </Link>
      <Link to="/wallet" className={linkClass(isWalletPage)}>
        <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
          <Wallet className="h-5 w-5 text-white" />
        </div>
        <span className="text-primary font-semibold">Wallet</span>
      </Link>
      <Link to="/explorer" className={linkClass(isExplorerPage)}>
        <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
          <Compass className="h-5 w-5 text-white" />
        </div>
        <span className="text-primary font-semibold">Explorer</span>
      </Link>
      <Link to="/treasury" className={linkClass(isTreasuryPage)}>
        <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
          <Vote className="h-5 w-5 text-white" />
        </div>
        <span className="text-primary font-semibold">Treasury</span>
      </Link>
      <Link to="/br" className={`relative ${linkClass(isBisonrelayPage)}`}>
        <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
          <img src="/images/bisonrelay.svg" alt="Bison Relay" className="h-6 w-auto" />
        </div>
        <span className="text-primary font-semibold">Bison Relay</span>
        {brBadge}
      </Link>
      <Link to="/dex" className={linkClass(isDexPage)}>
        <div className="h-10 w-auto rounded-lg flex items-center justify-center bg-gradient-primary px-2">
          <img src="/images/bisonwallet.svg" alt="Bison Wallet" className="h-6 w-auto" />
        </div>
        <span className="text-primary font-semibold">DEX</span>
      </Link>
    </>
  );

  const versionBadge = nodeVersion ? (
    <div className="px-4 py-2 rounded-lg bg-primary/10 border border-primary/20">
      <p className="text-sm text-muted-foreground">Version</p>
      <p className="text-lg font-semibold text-primary">{nodeVersion}</p>
    </div>
  ) : null;

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      await refreshAuth();
    }
  };
  const showLogout = !!authStatus?.enabled && !!authStatus?.authenticated;
  // Desktop main nav: icon-only with a title tooltip.
  const logoutIconButton = showLogout ? (
    <button
      type="button"
      onClick={handleLogout}
      title="Log out"
      aria-label="Log out"
      className="p-3 rounded-lg border border-primary/20 bg-primary/10 hover:bg-primary/20 transition-colors"
    >
      <LogOut className="h-6 w-6 text-primary" />
    </button>
  ) : null;
  // Mobile drawer: labeled, matching the other drawer items.
  const logoutDrawerButton = showLogout ? (
    <button type="button" onClick={handleLogout} className={linkClass(false)}>
      <div className="h-10 w-10 rounded-lg flex items-center justify-center bg-gradient-primary">
        <LogOut className="h-5 w-5 text-white" />
      </div>
      <span className="text-primary font-semibold">Log out</span>
    </button>
  ) : null;

  return (
    <div className="flex items-center justify-between mb-6 sm:mb-8 animate-fade-in">
      <Link to="/" className="shrink-0">
        <img src="/images/decred-logo.svg" alt="Decred" className="h-12 sm:h-[72px] w-auto" />
      </Link>

      {/* Desktop navigation */}
      <div className="hidden lg:flex items-center gap-4">
        {navLinks}
        {versionBadge}
        {logoutIconButton}
      </div>

      {/* Mobile hamburger */}
      <button
        type="button"
        onClick={() => setMenuOpen(true)}
        aria-label="Open menu"
        aria-expanded={menuOpen}
        className="relative lg:hidden p-3 rounded-lg border border-primary/20 bg-primary/10 hover:bg-primary/20 transition-colors"
      >
        <Menu className="h-6 w-6 text-primary" />
        {hasUnread && (
          <span
            className="absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full bg-primary ring-2 ring-background"
            aria-label={unreadLabel}
          />
        )}
      </button>

      {/* Mobile slide-in drawer */}
      {menuOpen && (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in"
            onClick={() => setMenuOpen(false)}
          />
          <div className="absolute inset-y-0 right-0 w-72 max-w-[80vw] flex flex-col overflow-y-auto border-l border-border/50 bg-background p-4 shadow-xl">
            <div className="mb-4 flex items-center justify-between">
              <span className="text-sm font-semibold text-muted-foreground">Menu</span>
              <button
                type="button"
                onClick={() => setMenuOpen(false)}
                aria-label="Close menu"
                className="rounded-lg p-2 transition-colors hover:bg-muted/20"
              >
                <X className="h-5 w-5 text-muted-foreground" />
              </button>
            </div>
            <nav className="flex flex-col gap-2" onClick={() => setMenuOpen(false)}>
              {navLinks}
            </nav>
            {logoutDrawerButton && <div className="mt-4">{logoutDrawerButton}</div>}
            {versionBadge && <div className="mt-4">{versionBadge}</div>}
          </div>
        </div>
      )}
    </div>
  );
};
