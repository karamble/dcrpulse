import { useState } from 'react';
import { Network, Wallet } from 'lucide-react';
import { LightningBalance, getLightningBalance } from '../../../services/lightningApi';
import { StatCard } from '../StatCard';
import { formatAtomsDcr } from '../../../utils/amounts';
import { useVisiblePoll } from '../../../hooks/useVisiblePoll';

export const ChannelFundingBalance = () => {
  const [balance, setBalance] = useState<LightningBalance | null>(null);

  const load = async () => {
    try {
      const b = await getLightningBalance();
      setBalance(b);
    } catch {
      // The Overview tab surfaces Lightning errors; keep this row quiet.
    }
  };
  useVisiblePoll(load, 15000);

  if (!balance) return null;

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
      <StatCard
        icon={<Wallet className="h-3.5 w-3.5" />}
        label="Spendable"
        value={formatAtomsDcr(balance.onChainConfirmed)}
      />
      <StatCard
        icon={<Wallet className="h-3.5 w-3.5" />}
        label="Unconfirmed"
        value={formatAtomsDcr(balance.onChainUnconfirmed)}
      />
      <StatCard
        icon={<Network className="h-3.5 w-3.5" />}
        label="In channels"
        value={formatAtomsDcr(balance.channelLocal)}
      />
      <StatCard
        icon={<Network className="h-3.5 w-3.5" />}
        label="Pending"
        value={formatAtomsDcr(balance.channelPending)}
      />
    </div>
  );
};
