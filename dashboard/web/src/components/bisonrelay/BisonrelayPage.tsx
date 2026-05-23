// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useState } from 'react';
import { MessageSquare, Rss } from 'lucide-react';
import { BisonrelaySetupWizard } from './BisonrelaySetupWizard';
import { BisonrelayMessagingPage } from './BisonrelayMessagingPage';
import { BisonrelayFeed } from './BisonrelayFeed';
import { BisonrelayStatus, getBisonrelayStatus } from '../../services/bisonrelayApi';

type TabId = 'chat' | 'feed';

interface TabDef {
  id: TabId;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

const tabs: TabDef[] = [
  { id: 'chat', label: 'Chat', icon: MessageSquare },
  { id: 'feed', label: 'Feed', icon: Rss },
];

// readHashTab returns the active tab based on the URL hash. The feed
// tab supports an optional subpath (`feed/<author>/<pid>`) used by the
// "fetch + open" deep-link from PostsListModal — the BisonrelayFeed
// component reads the subpath separately to auto-expand a card.
const readHashTab = (): TabId => {
  const h = window.location.hash.replace('#', '').toLowerCase();
  return h.startsWith('feed') ? 'feed' : 'chat';
};

export const BisonrelayPage = () => {
  const [ready, setReady] = useState<BisonrelayStatus | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>(readHashTab);

  useEffect(() => {
    if (!ready) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await getBisonrelayStatus();
        if (!cancelled && s.stage === 'ready') setReady(s);
      } catch {
        /* keep last known */
      }
    };
    const id = setInterval(tick, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [ready]);

  // Keep the URL hash in sync with the active tab so deep links and
  // browser back/forward work. Don't overwrite when the hash already
  // describes the same tab (e.g. `feed/<uid>/<pid>` while activeTab is
  // 'feed') so the post-target subpath is preserved.
  useEffect(() => {
    if (readHashTab() !== activeTab) {
      window.history.replaceState(null, '', `#${activeTab}`);
    }
  }, [activeTab]);

  useEffect(() => {
    const onHashChange = () => setActiveTab(readHashTab());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  if (!ready) {
    return (
      <BisonrelaySetupWizard
        onReady={async () => {
          try {
            const s = await getBisonrelayStatus();
            setReady(s);
          } catch {
            /* ignored - wizard keeps rendering */
          }
        }}
      />
    );
  }

  return (
    <div className="space-y-4">
      <nav className="flex gap-1 border-b border-border">
        {tabs.map((tab) => {
          const isActive = activeTab === tab.id;
          const Icon = tab.icon;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 -mb-px border-b-2 inline-flex items-center gap-2 text-sm transition-colors ${
                isActive
                  ? 'border-primary text-primary font-semibold'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </button>
          );
        })}
      </nav>

      {activeTab === 'chat' && <BisonrelayMessagingPage ownNick={ready.nick ?? 'unknown'} />}
      {activeTab === 'feed' && <BisonrelayFeed />}
    </div>
  );
};
