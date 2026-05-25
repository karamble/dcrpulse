import { useState } from 'react';
import { OpenChannelForm } from './OpenChannelForm';
import { ChannelList } from './ChannelList';
import { AutopilotSwitch } from './AutopilotSwitch';
import { ChannelFundingBalance } from './ChannelFundingBalance';

export const ChannelsTab = () => {
  const [reloadKey, setReloadKey] = useState(0);
  return (
    <div className="space-y-6">
      <AutopilotSwitch />
      <ChannelFundingBalance key={reloadKey} />
      <OpenChannelForm onChannelOpened={() => setReloadKey((k) => k + 1)} />
      <ChannelList key={reloadKey} />
    </div>
  );
};
