import { InfosSection } from './InfosSection';
import { BackupSection } from './BackupSection';
import { WatchtowersSection } from './WatchtowersSection';
import { NetworkSection } from './NetworkSection';

export const AdvancedTab = () => (
  <div className="space-y-6">
    <div>
      <h2 className="text-xl font-semibold">Advanced</h2>
      <p className="text-sm text-muted-foreground">
        Node info, channel-state backup, watchtower clients, and channel-graph queries.
      </p>
    </div>
    <InfosSection />
    <BackupSection />
    <WatchtowersSection />
    <NetworkSection />
  </div>
);
