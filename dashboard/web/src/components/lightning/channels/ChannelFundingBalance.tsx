import { useEffect, useState } from 'react';
import { Network, Wallet } from 'lucide-react';
import { LightningBalance, getLightningBalance } from '../../../services/lightningApi';
import { StatCard, fmtDcr } from '../StatCard';

export const ChannelFundingBalance = () => {
  const [balance, setBalance] = useState<LightningBalance | null>(null);

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const b = await getLightningBalance();
        if (active) setBalance(b);
      } catch {
        // The Overview tab surfaces Lightning errors; keep this row quiet.
      }
    };
    load();
    const id = window.setInterval(load, 15000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, []);

  if (!balance) return null;

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
      <StatCard
        icon={<Wallet className="h-3.5 w-3.5" />}
        label="Spendable"
        value={fmtDcr(balance.onChainConfirmed)}
      />
      <StatCard
        icon={<Wallet className="h-3.5 w-3.5" />}
        label="Unconfirmed"
        value={fmtDcr(balance.onChainUnconfirmed)}
      />
      <StatCard
        icon={<Network className="h-3.5 w-3.5" />}
        label="In channels"
        value={fmtDcr(balance.channelLocal)}
      />
      <StatCard
        icon={<Network className="h-3.5 w-3.5" />}
        label="Pending"
        value={fmtDcr(balance.channelPending)}
      />
    </div>
  );
};
