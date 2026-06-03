// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Link, Outlet, useLocation } from 'react-router-dom';

const tabs = [
  { path: 'wallet', label: 'Wallet' },
  { path: 'privacy', label: 'Privacy & Security' },
  { path: 'logs', label: 'Logs' },
  { path: 'themes', label: 'Themes' },
  { path: 'about', label: 'About' },
];

export const SettingsPage = () => {
  const location = useLocation();
  const active = location.pathname.split('/').pop() || 'wallet';

  return (
    <div className="space-y-6">

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
