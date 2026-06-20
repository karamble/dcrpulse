// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, Outlet, useLocation } from 'react-router-dom';
import { useWalletReady } from '../hooks/useWalletReady';

const tabs = [
  { path: 'purchase', label: 'Purchase' },
  { path: 'autobuyer', label: 'Auto Buyer' },
  { path: 'status', label: 'Ticket Status' },
  { path: 'history', label: 'History' },
  { path: 'statistics', label: 'Statistics' },
];

export const StakingPage = () => {
  const location = useLocation();
  const active = location.pathname.split('/').pop() || 'purchase';
  // Watch-only wallets can view tickets but cannot buy them (no signing), so
  // drop the Purchase and Auto Buyer tabs.
  const { isWatchOnly } = useWalletReady();
  const visibleTabs = isWatchOnly
    ? tabs.filter((t) => t.path !== 'purchase' && t.path !== 'autobuyer')
    : tabs;

  return (
    <div className="space-y-6">

      <nav className="flex gap-2 border-b border-border overflow-x-auto overflow-y-hidden -mx-3 px-3 sm:mx-0 sm:px-0">
        {visibleTabs.map((tab) => {
          const isActive = active === tab.path;
          return (
            <Link
              key={tab.path}
              to={tab.path}
              className={`px-4 py-2 border-b-2 whitespace-nowrap shrink-0 transition-colors ${
                isActive
                  ? 'border-primary text-primary font-semibold'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
            >
              {tab.label}
            </Link>
          );
        })}
      </nav>

      <Outlet />
    </div>
  );
};
