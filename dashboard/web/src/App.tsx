// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { Footer } from './components/Footer';
import { ExternalLinkGuard } from './components/ExternalLinkGuard';
import { NodeDashboard } from './pages/NodeDashboard';
import { WalletDashboard } from './pages/WalletDashboard';
import { WalletLayout } from './components/wallet/WalletLayout';
import { OnChainTransactions } from './pages/OnChainTransactions';
import { SendTab } from './components/onchain/SendTab';
import { ReceiveTab } from './components/onchain/ReceiveTab';
import { HistoryTab } from './components/onchain/HistoryTab';
import { ExportTab } from './components/onchain/ExportTab';
import { AccountsPage } from './pages/AccountsPage';
import { PrivacyPage } from './pages/PrivacyPage';
import { StakingPage } from './pages/StakingPage';
import { GovernancePage } from './pages/GovernancePage';
import { LightningPage } from './pages/LightningPage';
import { OverviewTab as LightningOverviewTab } from './components/lightning/OverviewTab';
import { ChannelsTab } from './components/lightning/channels/ChannelsTab';
import { ChannelDetailPage } from './components/lightning/channels/ChannelDetailPage';
import { SendTab as LightningSendTab } from './components/lightning/send/SendTab';
import { ReceiveTab as LightningReceiveTab } from './components/lightning/receive/ReceiveTab';
import { AdvancedTab as LightningAdvancedTab } from './components/lightning/advanced/AdvancedTab';
import { ConsensusTab } from './components/governance/ConsensusTab';
import { TreasuryTab } from './components/governance/TreasuryTab';
import { ProposalsTab } from './components/governance/ProposalsTab';
import { ProposalDetailPage } from './components/governance/ProposalDetailPage';
import { SettingsPage } from './pages/SettingsPage';
import { WalletSection } from './components/settings/WalletSection';
import { PrivacySection } from './components/settings/PrivacySection';
import { LogsSection } from './components/settings/LogsSection';
import { AboutSection } from './components/settings/AboutSection';
import { PurchaseTab } from './components/staking/PurchaseTab';
import { AutobuyerTab } from './components/staking/AutobuyerTab';
import { TicketStatusTab } from './components/staking/TicketStatusTab';
import { TicketHistoryTab } from './components/staking/TicketHistoryTab';
import { StatisticsTab } from './components/staking/StatisticsTab';
import { ExplorerLanding } from './pages/ExplorerLanding';
import { BlockDetail } from './pages/BlockDetail';
import { TransactionDetail } from './pages/TransactionDetail';
import { AddressView } from './pages/AddressView';
import { MempoolView } from './pages/MempoolView';
import { GovernanceDashboard } from './pages/GovernanceDashboard';
import { BisonrelayPage } from './components/bisonrelay/BisonrelayPage';
import { BisonrelayLiveProvider } from './components/bisonrelay/BisonrelayLiveProvider';
import { getDashboardData, getWalletStatus } from './services/api';
import { getLightningInfo } from './services/lightningApi';
import { getBisonrelayVersion } from './services/bisonrelayApi';

function AppContent() {
  const location = useLocation();
  const [nodeVersion, setNodeVersion] = useState<string>('');
  const [walletVersion, setWalletVersion] = useState<string>('');
  const [lndVersion, setLndVersion] = useState<string>('');
  const [brclientdVersion, setBrclientdVersion] = useState<string>('');
  const [lastUpdate, setLastUpdate] = useState<string>('');

  // Fetch versions for header and footer
  useEffect(() => {
    const fetchVersions = async () => {
      try {
        // Fetch node version and last update
        const dashboardData = await getDashboardData();
        setNodeVersion(dashboardData.nodeStatus?.version || '');
        if (dashboardData.lastUpdate) {
          setLastUpdate(new Date(dashboardData.lastUpdate).toLocaleString());
        }
        
        // Fetch wallet version
        try {
          const walletStatus = await getWalletStatus();
          setWalletVersion(walletStatus.version || '');
        } catch (walletErr) {
          // Wallet might not be available, that's ok
          console.debug('Wallet version not available:', walletErr);
        }

        // Fetch dcrlnd version. The LN wallet may be locked / un-set-up;
        // in those cases GetInfo returns 503 and we simply don't show the
        // footer entry. Backend already normalises this to a clean
        // "v0.8.1" via Versioner.GetVersion.
        try {
          const lnInfo = await getLightningInfo();
          setLndVersion(lnInfo.version || '');
        } catch (lnErr) {
          console.debug('Lightning version not available:', lnErr);
        }

        try {
          const br = await getBisonrelayVersion();
          setBrclientdVersion(br.appVersion || '');
        } catch (brErr) {
          console.debug('brclientd version not available:', brErr);
        }
      } catch (err) {
        console.error('Error fetching versions:', err);
      }
    };
    fetchVersions();
  }, [location.pathname]);

  return (
    <div className="min-h-screen bg-background p-6">
      <div className="max-w-7xl mx-auto space-y-6">
        <Header nodeVersion={nodeVersion} />
        <Routes>
          <Route path="/" element={<NodeDashboard />} />
          <Route path="/wallet" element={<WalletLayout />}>
            <Route index element={<WalletDashboard />} />
            <Route path="privacy" element={<PrivacyPage />} />
            <Route path="staking" element={<StakingPage />}>
              <Route index element={<Navigate to="purchase" replace />} />
              <Route path="purchase" element={<PurchaseTab />} />
              <Route path="autobuyer" element={<AutobuyerTab />} />
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
            <Route path="settings" element={<SettingsPage />}>
              <Route index element={<Navigate to="wallet" replace />} />
              <Route path="wallet" element={<WalletSection />} />
              <Route path="privacy" element={<PrivacySection />} />
              <Route path="logs" element={<LogsSection />} />
              <Route path="about" element={<AboutSection />} />
            </Route>
            <Route path="transactions" element={<OnChainTransactions />}>
              <Route index element={<Navigate to="send" replace />} />
              <Route path="send" element={<SendTab />} />
              <Route path="receive" element={<ReceiveTab />} />
              <Route path="history" element={<HistoryTab />} />
              <Route path="export" element={<ExportTab />} />
            </Route>
          </Route>
          <Route path="/explorer" element={<ExplorerLanding />} />
          <Route path="/explorer/block/:heightOrHash" element={<BlockDetail />} />
          <Route path="/explorer/tx/:txhash" element={<TransactionDetail />} />
          <Route path="/explorer/address/:address" element={<AddressView />} />
          <Route path="/explorer/mempool" element={<MempoolView />} />
          <Route path="/governance" element={<GovernanceDashboard />} />
          <Route path="/br" element={<BisonrelayPage />} />
        </Routes>
        <Footer
          dcrdVersion={nodeVersion}
          dcrwalletVersion={walletVersion}
          dcrlndVersion={lndVersion}
          brclientdVersion={brclientdVersion}
          lastUpdate={lastUpdate}
        />
      </div>
      <ExternalLinkGuard />
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <BisonrelayLiveProvider>
        <AppContent />
      </BisonrelayLiveProvider>
    </BrowserRouter>
  );
}

export default App;

