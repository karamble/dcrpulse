// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, Outlet, useLocation } from 'react-router-dom';
import { VoteTrickleBadge } from '../components/governance/VoteTrickleCard';

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

      <nav className="flex gap-2 border-b border-border overflow-x-auto overflow-y-hidden -mx-3 px-3 sm:mx-0 sm:px-0">
        {tabs.map((tab) => {
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
        <div className="ml-auto flex items-center pb-1.5 pl-2">
          <VoteTrickleBadge />
        </div>
      </nav>

      <Outlet />
    </div>
  );
};
