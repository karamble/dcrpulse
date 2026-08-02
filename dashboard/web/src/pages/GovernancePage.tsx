// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useState } from 'react';
import { Link, Outlet, useLocation } from 'react-router-dom';
import { VoteTrickleBadge, VoteTrickleOutletContext } from '../components/governance/VoteTrickleCard';
import { VoteTrickleStatus, getVoteTrickleWorkers } from '../services/api';
import { useVisiblePoll } from '../hooks/useVisiblePoll';

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

  // One worker poll for the whole governance tree: the header badge and the
  // proposals-tab cards read this list via the Outlet context instead of each
  // polling the endpoint themselves.
  const [workers, setWorkers] = useState<VoteTrickleStatus[]>([]);
  const refreshWorkers = useCallback(async () => {
    try {
      setWorkers(await getVoteTrickleWorkers());
    } catch {
      /* ignore transient poll errors */
    }
  }, []);
  useVisiblePoll(refreshWorkers, 5000);

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
          <VoteTrickleBadge workers={workers} />
        </div>
      </nav>

      <Outlet context={{ workers, refreshWorkers } satisfies VoteTrickleOutletContext} />
    </div>
  );
};
