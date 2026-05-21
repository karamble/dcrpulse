// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { MyTicketsInfo } from '../MyTicketsInfo';
import { TicketPoolInfo } from '../TicketPoolInfo';
import { PurchaseTicketForm } from './PurchaseTicketForm';
import { WalletDashboardData, getWalletDashboard } from '../../services/api';

export const PurchaseTab = () => {
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
