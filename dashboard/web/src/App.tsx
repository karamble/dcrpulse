// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Header } from './components/Header';
import { NodeDashboard } from './pages/NodeDashboard';
import { WalletDashboard } from './pages/WalletDashboard';
import { ExplorerLanding } from './pages/ExplorerLanding';
import { BlockDetail } from './pages/BlockDetail';
import { TransactionDetail } from './pages/TransactionDetail';
import { AddressView } from './pages/AddressView';
import { GovernanceDashboard } from './pages/GovernanceDashboard';
import { getDashboardData } from './services/api';

function App() {
  const [nodeVersion, setNodeVersion] = useState<string>('');

  // Fetch node version for header
  useEffect(() => {
    const fetchVersion = async () => {
      try {
        const data = await getDashboardData();
        setNodeVersion(data.nodeStatus?.version || '');
      } catch (err) {
        console.error('Error fetching node version:', err);
      }
    };
    fetchVersion();
  }, []);

  return (
    <BrowserRouter>
      <div className="min-h-screen bg-background p-6">
        <div className="max-w-7xl mx-auto space-y-6">
          <Header nodeVersion={nodeVersion} />
          <Routes>
            <Route path="/" element={<NodeDashboard />} />
            <Route path="/wallet" element={<WalletDashboard />} />
            <Route path="/explorer" element={<ExplorerLanding />} />
            <Route path="/explorer/block/:heightOrHash" element={<BlockDetail />} />
            <Route path="/explorer/tx/:txhash" element={<TransactionDetail />} />
            <Route path="/explorer/address/:address" element={<AddressView />} />
            <Route path="/governance" element={<GovernanceDashboard />} />
          </Routes>
        </div>
      </div>
    </BrowserRouter>
  );
}

export default App;

