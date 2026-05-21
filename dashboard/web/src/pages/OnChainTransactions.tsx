import { Link, Outlet, useLocation } from 'react-router-dom';
import { ArrowLeftRight } from 'lucide-react';

const tabs = [
  { path: 'send', label: 'Send' },
  { path: 'receive', label: 'Receive' },
  { path: 'history', label: 'History' },
  { path: 'export', label: 'Export' },
];

export const OnChainTransactions = () => {
  const location = useLocation();
  const active = location.pathname.split('/').pop() || 'send';

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
          <ArrowLeftRight className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h2 className="text-xl font-semibold">On-Chain Transactions</h2>
          <p className="text-sm text-muted-foreground">
            Send, receive, and review your wallet's regular transactions.
          </p>
        </div>
      </div>

      <nav className="flex gap-2 border-b border-border">
        {tabs.map((tab) => {
          const isActive = active === tab.path;
          return (
            <Link
              key={tab.path}
              to={tab.path}
              className={`px-4 py-2 -mb-px border-b-2 transition-colors ${
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
