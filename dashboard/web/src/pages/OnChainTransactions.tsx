import { Link, Outlet, useLocation } from 'react-router-dom';
import { useWalletReady } from '../hooks/useWalletReady';
import { WatchOnlyGate } from '../components/common/WatchOnlyGate';

const tabs = [
  { path: 'send', label: 'Send' },
  { path: 'receive', label: 'Receive' },
  { path: 'history', label: 'History' },
  { path: 'export', label: 'Export' },
];

export const OnChainTransactions = () => {
  const location = useLocation();
  const active = location.pathname.split('/').pop() || 'send';
  const { isWatchOnly } = useWalletReady();

  if (isWatchOnly) {
    return <WatchOnlyGate feature="On-chain transactions" />;
  }

  return (
    <div className="space-y-6">

      <nav className="flex gap-2 border-b border-border overflow-x-auto overflow-y-hidden -mx-3 px-3 sm:mx-0 sm:px-0">
        {tabs.map((tab) => {
          const isActive = active === tab.path;
          return (
            <Link
              key={tab.path}
              to={tab.path}
              className={`px-4 py-2 border-b-2 whitespace-nowrap shrink-0 transition-colors ${
                isActive
                  ? 'border-primary text-primary font-semibold'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              }`}
            >
              {tab.label}
            </Link>
          );
        })}
      </nav>
      <Outlet />
    </div>
  );
};
