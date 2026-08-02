// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { toYMDTime } from './utils/date';
import { ComponentType, Suspense, lazy, useEffect, useState } from 'react';
import { Header } from './components/Header';
import { Footer } from './components/Footer';
import { ExternalLinkGuard } from './components/ExternalLinkGuard';
import { ThemeProvider } from './services/themes/ThemeProvider';
import { NodeDashboard } from './pages/NodeDashboard';
import { WalletDashboard } from './pages/WalletDashboard';
import { WalletLayout } from './components/wallet/WalletLayout';
import { WalletSelection } from './pages/WalletSelection';
import { AccountsPage } from './pages/AccountsPage';
import { PrivacyPage } from './pages/PrivacyPage';
import { StakingPage } from './pages/StakingPage';
import { GovernancePage } from './pages/GovernancePage';
import { ConsensusTab } from './components/governance/ConsensusTab';
import { TreasuryTab } from './components/governance/TreasuryTab';
import { ProposalsTab } from './components/governance/ProposalsTab';
import { ProposalDetailPage } from './components/governance/ProposalDetailPage';
import { PurchaseTab } from './components/staking/PurchaseTab';
import { AutobuyerTab } from './components/staking/AutobuyerTab';
import { TicketStatusTab } from './components/staking/TicketStatusTab';
import { TicketHistoryTab } from './components/staking/TicketHistoryTab';
import { WatchOnlyGuard, RequireWatchOnly } from './components/common/WatchOnlyGuard';
import { BisonrelayLiveProvider } from './components/bisonrelay/BisonrelayLiveProvider';
import { AuthGate } from './components/auth/AuthGate';
import { getDashboardData } from './services/api';
import { getLightningInfo } from './services/lightningApi';
import { getBisonrelayVersion } from './services/bisonrelayApi';
import { getDexStatus } from './services/dcrdexApi';
import { useWalletReady } from './hooks/useWalletReady';

// React.lazy needs a default export and all route modules here use named
// exports. Chunks are also content-hashed and replaced wholesale on a daemon
// upgrade, so a long-lived tab can request a chunk that no longer exists;
// reload once to pick up the new index.html instead of rendering nothing.
const lazyRoute = <M,>(load: () => Promise<M>, pick: (m: M) => ComponentType<any>) =>
  lazy(() =>
    load().then(
      (m) => {
        sessionStorage.removeItem('dcrpulse.chunkReload');
        return { default: pick(m) };
      },
      (err) => {
        if (!sessionStorage.getItem('dcrpulse.chunkReload')) {
          sessionStorage.setItem('dcrpulse.chunkReload', '1');
          window.location.reload();
        }
        throw err;
      },
    ),
  );

// Heavy route trees load on demand so the initial chunk stays small; the big
// movers are klinecharts (DEX), recharts (treasury + staking statistics) and
// the Bison Relay UI.
const DexPage = lazyRoute(() => import('./pages/DexPage'), (m) => m.DexPage);
const BisonrelayPage = lazyRoute(() => import('./components/bisonrelay/BisonrelayPage'), (m) => m.BisonrelayPage);
const GovernanceDashboard = lazyRoute(() => import('./pages/GovernanceDashboard'), (m) => m.GovernanceDashboard);
const StatisticsTab = lazyRoute(() => import('./components/staking/StatisticsTab'), (m) => m.StatisticsTab);
const LightningPage = lazyRoute(() => import('./pages/LightningPage'), (m) => m.LightningPage);
const LightningOverviewTab = lazyRoute(() => import('./components/lightning/OverviewTab'), (m) => m.OverviewTab);
const ChannelsTab = lazyRoute(() => import('./components/lightning/channels/ChannelsTab'), (m) => m.ChannelsTab);
const ChannelDetailPage = lazyRoute(() => import('./components/lightning/channels/ChannelDetailPage'), (m) => m.ChannelDetailPage);
const LightningSendTab = lazyRoute(() => import('./components/lightning/send/SendTab'), (m) => m.SendTab);
const LightningReceiveTab = lazyRoute(() => import('./components/lightning/receive/ReceiveTab'), (m) => m.ReceiveTab);
const LightningAdvancedTab = lazyRoute(() => import('./components/lightning/advanced/AdvancedTab'), (m) => m.AdvancedTab);
const ExplorerLanding = lazyRoute(() => import('./pages/ExplorerLanding'), (m) => m.ExplorerLanding);
const BlockDetail = lazyRoute(() => import('./pages/BlockDetail'), (m) => m.BlockDetail);
const TransactionDetail = lazyRoute(() => import('./pages/TransactionDetail'), (m) => m.TransactionDetail);
const AddressView = lazyRoute(() => import('./pages/AddressView'), (m) => m.AddressView);
const MempoolView = lazyRoute(() => import('./pages/MempoolView'), (m) => m.MempoolView);
const TimestampPage = lazyRoute(() => import('./pages/TimestampPage'), (m) => m.TimestampPage);
const VerifyTimestampPage = lazyRoute(() => import('./pages/VerifyTimestampPage'), (m) => m.VerifyTimestampPage);
const SettingsPage = lazyRoute(() => import('./pages/SettingsPage'), (m) => m.SettingsPage);
const WalletSection = lazyRoute(() => import('./components/settings/WalletSection'), (m) => m.WalletSection);
const PrivacySection = lazyRoute(() => import('./components/settings/PrivacySection'), (m) => m.PrivacySection);
const LogsSection = lazyRoute(() => import('./components/settings/LogsSection'), (m) => m.LogsSection);
const AboutSection = lazyRoute(() => import('./components/settings/AboutSection'), (m) => m.AboutSection);
const ThemesSection = lazyRoute(() => import('./components/settings/themes/ThemesSection'), (m) => m.ThemesSection);
const SecuritySection = lazyRoute(() => import('./components/settings/SecuritySection'), (m) => m.SecuritySection);
const TorSection = lazyRoute(() => import('./components/settings/TorSection'), (m) => m.TorSection);
const OnChainTransactions = lazyRoute(() => import('./pages/OnChainTransactions'), (m) => m.OnChainTransactions);
const OnChainTransactionsIndex = lazyRoute(() => import('./pages/OnChainTransactions'), (m) => m.OnChainTransactionsIndex);
const SendTab = lazyRoute(() => import('./components/onchain/SendTab'), (m) => m.SendTab);
const ReceiveTab = lazyRoute(() => import('./components/onchain/ReceiveTab'), (m) => m.ReceiveTab);
const HistoryTab = lazyRoute(() => import('./components/onchain/HistoryTab'), (m) => m.HistoryTab);
const ExportTab = lazyRoute(() => import('./components/onchain/ExportTab'), (m) => m.ExportTab);
const OfflineSigningTab = lazyRoute(() => import('./components/onchain/OfflineSigningTab'), (m) => m.OfflineSigningTab);
const SharedWalletsPage = lazyRoute(() => import('./components/wallet/shared/SharedWalletsPage'), (m) => m.SharedWalletsPage);
const SharedWalletDetailPage = lazyRoute(() => import('./components/wallet/shared/SharedWalletDetailPage'), (m) => m.SharedWalletDetailPage);

const RouteFallback = () => (
  <div className="min-h-[40vh] flex items-center justify-center">
    <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-primary" />
  </div>
);

function AppContent() {
  const location = useLocation();
  const [nodeVersion, setNodeVersion] = useState<string>('');
  const [lndVersion, setLndVersion] = useState<string>('');
  const [brclientdVersion, setBrclientdVersion] = useState<string>('');
  const [bisonwVersion, setBisonwVersion] = useState<string>('');
  const [lastUpdate, setLastUpdate] = useState<string>('');
  // A watch-only wallet has no dcrlnd / brclientd / bisonw daemons, so skip
  // those version fetches (they would error against absent daemons). The
  // dcrwallet version rides along on the shared status snapshot.
  const { isWatchOnly, version: walletVersion } = useWalletReady();

  // Fetch versions for header and footer. Daemon versions never change within
  // a session, so this runs once (again only if the watch-only flag flips when
  // the first wallet status lands) instead of on every navigation. The LN /
  // BR / DEX daemons may be locked or absent; a rejected probe simply leaves
  // that footer entry blank.
  useEffect(() => {
    let cancelled = false;
    const fetchVersions = async () => {
      const [dash, ln, br, dex] = await Promise.allSettled([
        getDashboardData(),
        isWatchOnly ? null : getLightningInfo(),
        isWatchOnly ? null : getBisonrelayVersion(),
        isWatchOnly ? null : getDexStatus(),
      ]);
      if (cancelled) return;
      if (dash.status === 'fulfilled') {
        setNodeVersion(dash.value.nodeStatus?.version || '');
        if (dash.value.lastUpdate) {
          setLastUpdate(toYMDTime(new Date(dash.value.lastUpdate)));
        }
      }
      if (ln.status === 'fulfilled' && ln.value) setLndVersion(ln.value.version || '');
      if (br.status === 'fulfilled' && br.value) setBrclientdVersion(br.value.appVersion || '');
      if (dex.status === 'fulfilled' && dex.value) setBisonwVersion(dex.value.bisonwVersion || '');
    };
    fetchVersions();
    return () => {
      cancelled = true;
    };
  }, [isWatchOnly]);

  // The DEX trading view is full-bleed: skip the centered max-width container
  // and outer padding so it uses the full viewport width.
  const dexFullWidth = location.pathname.startsWith('/dex');
  // On mobile the BR page hides the version footer so the chat can use the
  // full viewport height.
  const brPage = location.pathname.startsWith('/br');
  return (
    <div className="min-h-screen bg-background">
      <div className="max-w-7xl mx-auto px-3 sm:px-6 pt-3 sm:pt-6">
        <Header nodeVersion={nodeVersion} />
      </div>
      <div
        className={
          dexFullWidth
            ? 'pt-3 space-y-3'
            : 'max-w-7xl mx-auto px-3 sm:px-6 pt-6 pb-3 sm:pb-6 space-y-6'
        }
      >
        <Suspense fallback={<RouteFallback />}>
          <Routes>
            <Route path="/" element={<NodeDashboard />} />
            <Route path="/wallet" element={<WalletLayout />}>
              <Route index element={<WalletDashboard />} />
              <Route path="select" element={<WalletSelection embedded />} />
              <Route path="privacy" element={<PrivacyPage />} />
              <Route path="staking" element={<StakingPage />}>
                <Route index element={<Navigate to="purchase" replace />} />
                <Route path="purchase" element={<WatchOnlyGuard feature="Ticket purchasing"><PurchaseTab /></WatchOnlyGuard>} />
                <Route path="autobuyer" element={<WatchOnlyGuard feature="The ticket auto buyer"><AutobuyerTab /></WatchOnlyGuard>} />
                <Route path="status" element={<TicketStatusTab />} />
                <Route path="history" element={<TicketHistoryTab />} />
                <Route path="statistics" element={<StatisticsTab />} />
              </Route>
              <Route path="governance" element={<GovernancePage />}>
                <Route index element={<Navigate to="consensus" replace />} />
                <Route path="consensus" element={<ConsensusTab />} />
                <Route path="treasury" element={<TreasuryTab />} />
                <Route path="proposals" element={<ProposalsTab />} />
                <Route path="proposals/:token" element={<ProposalDetailPage />} />
              </Route>
              <Route path="lightning" element={<LightningPage />}>
                <Route index element={<LightningOverviewTab />} />
                <Route path="channels" element={<ChannelsTab />} />
                <Route path="channels/:channelPoint" element={<ChannelDetailPage />} />
                <Route path="send" element={<LightningSendTab />} />
                <Route path="receive" element={<LightningReceiveTab />} />
                <Route path="advanced" element={<LightningAdvancedTab />} />
              </Route>
              <Route path="accounts" element={<AccountsPage />} />
              <Route
                path="shared"
                element={<WatchOnlyGuard feature="Shared wallets"><SharedWalletsPage /></WatchOnlyGuard>}
              />
              <Route
                path="shared/:id"
                element={<WatchOnlyGuard feature="Shared wallets"><SharedWalletDetailPage /></WatchOnlyGuard>}
              />
              <Route path="timestamp" element={<TimestampPage />} />
              <Route path="settings" element={<SettingsPage />}>
                <Route index element={<Navigate to="wallet" replace />} />
                <Route path="wallet" element={<WalletSection />} />
                <Route path="privacy" element={<PrivacySection />} />
                <Route path="logs" element={<LogsSection />} />
                <Route path="about" element={<AboutSection />} />
                <Route path="themes" element={<ThemesSection />} />
                <Route path="security" element={<SecuritySection />} />
                <Route path="tor" element={<TorSection />} />
              </Route>
              <Route path="transactions" element={<OnChainTransactions />}>
                <Route index element={<OnChainTransactionsIndex />} />
                <Route path="send" element={<WatchOnlyGuard feature="Sending"><SendTab /></WatchOnlyGuard>} />
                <Route path="receive" element={<ReceiveTab />} />
                <Route path="history" element={<HistoryTab />} />
                <Route path="export" element={<ExportTab />} />
                <Route path="offline" element={<RequireWatchOnly><OfflineSigningTab /></RequireWatchOnly>} />
              </Route>
            </Route>
            <Route path="/explorer" element={<ExplorerLanding />} />
            <Route path="/explorer/block/:heightOrHash" element={<BlockDetail />} />
            <Route path="/explorer/tx/:txhash" element={<TransactionDetail />} />
            <Route path="/explorer/address/:address" element={<AddressView />} />
            <Route path="/explorer/mempool" element={<MempoolView />} />
            <Route path="/explorer/verify-timestamp" element={<VerifyTimestampPage />} />
            <Route path="/treasury" element={<GovernanceDashboard />} />
            <Route path="/br" element={<BisonrelayPage />} />
            <Route path="/dex" element={<DexPage />} />
          </Routes>
        </Suspense>
        <Footer
          className={brPage ? 'hidden md:block' : undefined}
          dcrdVersion={nodeVersion}
          dcrwalletVersion={walletVersion}
          dcrlndVersion={lndVersion}
          brclientdVersion={brclientdVersion}
          bisonwVersion={bisonwVersion}
          lastUpdate={lastUpdate}
        />
      </div>
      <ExternalLinkGuard />
    </div>
  );
}

function App() {
  return (
    <ThemeProvider>
      <BrowserRouter>
        <AuthGate>
          <BisonrelayLiveProvider>
            <AppContent />
          </BisonrelayLiveProvider>
        </AuthGate>
      </BrowserRouter>
    </ThemeProvider>
  );
}

export default App;

