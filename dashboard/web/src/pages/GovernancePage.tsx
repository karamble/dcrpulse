// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, Outlet, useLocation } from 'react-router-dom';
import { Vote } from 'lucide-react';

const tabs = [
  { path: 'consensus', label: 'Consensus' },
  { path: 'treasury', label: 'Treasury' },
  { path: 'proposals', label: 'Proposals' },
];

export const GovernancePage = () => {
  const location = useLocation();
  // /wallet/governance/proposals/:token shouldn't change the active tab.
  const segments = location.pathname.split('/').filter(Boolean);
  const idx = segments.indexOf('governance');
  const active = (idx >= 0 && segments[idx + 1]) || 'consensus';

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <Vote className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h2 className="text-xl font-semibold">Governance</h2>
          <p className="text-sm text-muted-foreground">
            Cast on-chain and off-chain votes from this wallet.
          </p>
        </div>
      </div>

      <nav className="flex gap-2 border-b border-border">
        {tabs.map((tab) => {
          const isActive = active === tab.path;
          return (
            <Link
              key={tab.path}
              to={tab.path}
              className={`px-4 py-2 -mb-px border-b-2 transition-colors ${
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
