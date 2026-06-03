// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useState } from 'react';
import { BarChart3, FileText, FolderOpen, MessageSquare, Phone, Rss } from 'lucide-react';
import { BisonrelaySetupWizard } from './BisonrelaySetupWizard';
import { BisonrelayMessagingPage } from './BisonrelayMessagingPage';
import { BisonrelayFeed } from './BisonrelayFeed';
import { BisonrelayFiles } from './BisonrelayFiles';
import { BisonrelayStats } from './BisonrelayStats';
import { BisonrelayRealtime } from './BisonrelayRealtime';
import { BisonrelayPages } from './BisonrelayPages';
import { BisonrelayStatus, getBisonrelayStatus } from '../../services/bisonrelayApi';
import { useWalletReady } from '../../hooks/useWalletReady';
import { WalletSyncGate } from '../common/WalletSyncGate';

type TabId = 'chat' | 'feed' | 'files' | 'stats' | 'realtime' | 'pages';

interface TabDef {
  id: TabId;
  label: string;
  icon: ComponentType<{ className?: string }>;
}

const tabs: TabDef[] = [
  { id: 'chat', label: 'Chat', icon: MessageSquare },
  { id: 'feed', label: 'Feed', icon: Rss },
  { id: 'files', label: 'Files', icon: FolderOpen },
  { id: 'stats', label: 'Stats', icon: BarChart3 },
  { id: 'realtime', label: 'Realtime', icon: Phone },
  { id: 'pages', label: 'Pages', icon: FileText },
];

// readHashTab returns the active tab based on the URL hash. Feed, Files,
// Stats, and Realtime support subpaths (e.g. `feed/<author>/<pid>`,
// `files/shared`, `stats/payments`, `realtime/room/<rv>`) which the
// embedded surfaces parse on their own.
const readHashTab = (): TabId => {
  const h = window.location.hash.replace('#', '').toLowerCase();
  if (h.startsWith('feed')) return 'feed';
  if (h.startsWith('files')) return 'files';
  if (h.startsWith('stats')) return 'stats';
  if (h.startsWith('realtime')) return 'realtime';
  if (h.startsWith('pages')) return 'pages';
  return 'chat';
};

export const BisonrelayPage = () => {
  const [ready, setReady] = useState<BisonrelayStatus | null>(null);
  const [activeTab, setActiveTab] = useState<TabId>(readHashTab);
  const wallet = useWalletReady();

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
    if (!wallet.ready) {
      return <WalletSyncGate feature="Bison Relay" message={wallet.message} progress={wallet.progress} />;
    }
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
      <nav className="flex gap-1 border-b border-border overflow-x-auto -mx-3 px-3 sm:mx-0 sm:px-0">
        {tabs.map((tab) => {
          const isActive = activeTab === tab.id;
          const Icon = tab.icon;
          return (
            <button
              key={tab.id}
              type="button"
              onClick={() => setActiveTab(tab.id)}
              className={`px-4 py-2 -mb-px border-b-2 inline-flex items-center gap-2 whitespace-nowrap shrink-0 text-sm transition-colors ${
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
      {activeTab === 'files' && <BisonrelayFiles />}
      {activeTab === 'stats' && <BisonrelayStats />}
      {activeTab === 'realtime' && <BisonrelayRealtime />}
      {activeTab === 'pages' && <BisonrelayPages />}
    </div>
  );
};
