import { useState } from 'react';
import { OpenChannelForm } from './OpenChannelForm';
import { ChannelList } from './ChannelList';
import { AutopilotSwitch } from './AutopilotSwitch';
import { ChannelFundingBalance } from './ChannelFundingBalance';
import { RequestLiquidityModal } from './RequestLiquidityModal';

export const ChannelsTab = () => {
  const [reloadKey, setReloadKey] = useState(0);
  const [liquidityOpen, setLiquidityOpen] = useState(false);
  return (
    <div className="space-y-6">
      <AutopilotSwitch />
      <ChannelFundingBalance key={reloadKey} />
      <div className="flex justify-end">
        <button
          type="button"
          onClick={() => setLiquidityOpen(true)}
          className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm font-medium"
        >
          Request Inbound Channel
        </button>
      </div>
      <OpenChannelForm onChannelOpened={() => setReloadKey((k) => k + 1)} />
      <ChannelList key={reloadKey} />
      {liquidityOpen && (
        <RequestLiquidityModal
          onClose={() => setLiquidityOpen(false)}
          onSuccess={() => setReloadKey((k) => k + 1)}
        />
      )}
    </div>
  );
};
