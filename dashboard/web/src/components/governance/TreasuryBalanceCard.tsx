// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { Landmark } from 'lucide-react';
import { getTreasuryInfo, TreasuryInfo } from '../../services/treasuryApi';

export const TreasuryBalanceCard = () => {
  const [info, setInfo] = useState<TreasuryInfo | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchData = async () => {
    try {
      const data = await getTreasuryInfo();
      setInfo(data);
      setLoading(false);
    } catch (err) {
      console.error('Failed to fetch treasury info:', err);
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    // Auto-refresh every 60 seconds
    const interval = setInterval(fetchData, 60000);
    return () => clearInterval(interval);
  }, []);

  const formatBalance = (amount: number) => {
    const integerPart = Math.floor(amount);
    const formattedInteger = integerPart.toLocaleString('en-US');
    const fullAmount = amount.toFixed(8);
    const mainPart = amount.toFixed(2);
    const decimalPart = fullAmount.substring(mainPart.length);
    
    return { mainPart: formattedInteger + mainPart.substring(integerPart.toString().length), decimalPart };
  };

  if (loading) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <p className="text-sm text-muted-foreground mb-1">Treasury Balance</p>
            <div className="text-center py-4">
              <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-primary"></div>
            </div>
          </div>
          <div className="p-3 rounded-xl bg-primary/10 border border-primary/20">
            <Landmark className="h-5 w-5 text-primary" />
          </div>
        </div>
      </div>
    );
  }

  const balance = info?.balance || 0;
  const { mainPart, decimalPart } = formatBalance(balance);

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 hover:border-primary/20 transition-all duration-300 group animate-fade-in">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <p className="text-sm text-muted-foreground mb-1">Treasury Balance</p>
          <h3 className="text-3xl font-bold mb-1 group-hover:text-primary transition-colors">
            {mainPart}
            <span className="text-lg opacity-70">{decimalPart}</span>
            {' '}
            <span className="text-xl">DCR</span>
          </h3>
          <p className="text-xs text-muted-foreground">
            Current decentralized treasury funds
          </p>
        </div>
        <div className="p-3 rounded-xl bg-primary/10 border border-primary/20 transition-all duration-300">
          <Landmark className="h-5 w-5 text-primary" />
        </div>
      </div>
    </div>
  );
};

