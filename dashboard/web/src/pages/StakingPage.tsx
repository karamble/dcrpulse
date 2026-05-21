import { useEffect, useState } from 'react';
import { Ticket } from 'lucide-react';
import { MyTicketsInfo } from '../components/MyTicketsInfo';
import { TicketPoolInfo } from '../components/TicketPoolInfo';
import { PurchaseTicketForm } from '../components/staking/PurchaseTicketForm';
import { WalletDashboardData, getWalletDashboard } from '../services/api';

export const StakingPage = () => {
  const [data, setData] = useState<WalletDashboardData | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = () => {
      getWalletDashboard()
        .then((d) => {
          if (!cancelled) setData(d);
        })
        .catch(() => {});
    };
    load();
    const id = window.setInterval(load, 10000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <Ticket className="h-5 w-5 text-primary" />
        </div>
        <h2 className="text-2xl font-bold">Staking</h2>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <PurchaseTicketForm />
        {data?.stakingInfo && (
          <TicketPoolInfo
            poolSize={data.stakingInfo.poolSize}
            currentDifficulty={data.stakingInfo.currentDifficulty}
            estimatedMin={data.stakingInfo.estimatedMin}
            estimatedMax={data.stakingInfo.estimatedMax}
            estimatedExpected={data.stakingInfo.estimatedExpected}
            allMempoolTix={data.stakingInfo.allMempoolTix}
          />
        )}
      </div>

      {data?.stakingInfo && (
        <MyTicketsInfo
          ownMempoolTix={data.stakingInfo.ownMempoolTix}
          immature={data.stakingInfo.immature}
          unspent={data.stakingInfo.unspent}
          voted={data.stakingInfo.voted}
          revoked={data.stakingInfo.revoked}
          unspentExpired={data.stakingInfo.unspentExpired}
          totalSubsidy={data.stakingInfo.totalSubsidy}
        />
      )}
    </div>
  );
};
