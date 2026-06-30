import { Link, Navigate, Outlet, useLocation } from 'react-router-dom';
import { useWalletReady } from '../hooks/useWalletReady';

const allTabs = [
  { path: 'send', label: 'Send' },
  { path: 'receive', label: 'Receive' },
  { path: 'history', label: 'History' },
  { path: 'export', label: 'Export' },
  { path: 'offline', label: 'Offline signing' },
];

// OnChainTransactionsIndex picks the default sub-tab once the wallet status is
// known: watch-only wallets cannot sign in-app, so they land on Offline signing
// (export -> sign on device -> import -> broadcast) rather than Send.
export const OnChainTransactionsIndex = () => {
  const { isWatchOnly, loading } = useWalletReady();
  if (loading) return null;
  return <Navigate to={isWatchOnly ? 'offline' : 'send'} replace />;
};

export const OnChainTransactions = () => {
  const location = useLocation();
  const active = location.pathname.split('/').pop() || 'send';
  const { isWatchOnly } = useWalletReady();

  // A watch-only wallet cannot sign in-app, so it spends through the Offline
  // signing tab (Send hidden). A full wallet signs in-app and has no use for
  // Offline signing (that tab hidden).
  const tabs = allTabs.filter((t) => (isWatchOnly ? t.path !== 'send' : t.path !== 'offline'));

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
