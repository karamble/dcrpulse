import { useEffect, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, ArrowLeftRight, Settings, Users, ShieldCheck, Ticket, Vote, Zap, Wallet } from 'lucide-react';
import { listWallets } from '../../services/api';
import { WalletSetup } from '../WalletSetup';
import { WalletSelection } from '../../pages/WalletSelection';

const navItemClass = ({ isActive }: { isActive: boolean }) =>
  `flex items-center gap-3 px-3 py-2 rounded-lg transition-colors ${
    isActive
      ? 'bg-primary/20 text-primary font-semibold'
      : 'text-foreground hover:bg-muted/20'
  }`;

export const WalletLayout = () => {
  // Gate the sidebar on the wallet list: no wallets -> first-run setup; wallets
  // exist but none selected -> wallet picker; a wallet is active -> dashboard.
  const [gate, setGate] = useState<'loading' | 'setup' | 'select' | 'ready'>('loading');
  const [activeName, setActiveName] = useState('');

  useEffect(() => {
    let cancelled = false;
    listWallets()
      .then((res) => {
        if (cancelled) return;
        setActiveName(res.active);
        if (res.wallets.length === 0) {
          setGate('setup');
        } else if (!res.active) {
          setGate('select');
        } else {
          setGate('ready');
        }
      })
      .catch((err) => {
        console.debug('listWallets failed in WalletLayout:', err);
        if (!cancelled) setGate('ready');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (gate === 'loading') {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent" />
      </div>
    );
  }

  if (gate === 'setup') {
    return <WalletSetup />;
  }

  if (gate === 'select') {
    return <WalletSelection />;
  }

  return (
    <div className="flex gap-6">
      <aside className="w-56 shrink-0">
        {activeName && (
          <div className="mb-3 px-3 py-2 rounded-lg bg-muted/20 border border-border/50">
            <div className="text-xs text-muted-foreground">Active wallet</div>
            <div className="font-semibold truncate">{activeName}</div>
          </div>
        )}
        <nav className="space-y-1">
          <NavLink to="/wallet" end className={navItemClass}>
            <LayoutDashboard className="h-4 w-4" />
            <span>Overview</span>
          </NavLink>
          <NavLink to="/wallet/transactions" className={navItemClass}>
            <ArrowLeftRight className="h-4 w-4" />
            <span>On-Chain Transactions</span>
          </NavLink>
          <NavLink to="/wallet/privacy" className={navItemClass}>
            <ShieldCheck className="h-4 w-4" />
            <span>Privacy</span>
          </NavLink>
          <NavLink to="/wallet/staking" className={navItemClass}>
            <Ticket className="h-4 w-4" />
            <span>Staking</span>
          </NavLink>
          <NavLink to="/wallet/governance" className={navItemClass}>
            <Vote className="h-4 w-4" />
            <span>Governance</span>
          </NavLink>
          <NavLink to="/wallet/lightning" className={navItemClass}>
            <Zap className="h-4 w-4" />
            <span>Lightning</span>
          </NavLink>
          <NavLink to="/wallet/accounts" className={navItemClass}>
            <Users className="h-4 w-4" />
            <span>Accounts</span>
          </NavLink>
          <NavLink to="/wallet/settings" className={navItemClass}>
            <Settings className="h-4 w-4" />
            <span>Settings</span>
          </NavLink>
          <NavLink to="/wallet/select" className={navItemClass}>
            <Wallet className="h-4 w-4" />
            <span>Switch Wallet</span>
          </NavLink>
        </nav>
      </aside>
      <div className="flex-1 min-w-0">
        <Outlet />
      </div>
    </div>
  );
};
