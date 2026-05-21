// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BrowserRouter, Routes, Route, Navigate, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { Footer } from './components/Footer';
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
import { ExplorerLanding } from './pages/ExplorerLanding';
import { BlockDetail } from './pages/BlockDetail';
import { TransactionDetail } from './pages/TransactionDetail';
import { AddressView } from './pages/AddressView';
import { MempoolView } from './pages/MempoolView';
import { GovernanceDashboard } from './pages/GovernanceDashboard';
import { getDashboardData, getWalletStatus } from './services/api';

function AppContent() {
  const location = useLocation();
  const [nodeVersion, setNodeVersion] = useState<string>('');
  const [walletVersion, setWalletVersion] = useState<string>('');
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
            <Route path="staking" element={<StakingPage />} />
            <Route path="accounts" element={<AccountsPage />} />
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
        </Routes>
        <Footer dcrdVersion={nodeVersion} dcrwalletVersion={walletVersion} lastUpdate={lastUpdate} />
      </div>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  );
}

export default App;

