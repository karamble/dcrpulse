import { useEffect, useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, ArrowLeftRight, Settings, Users, ShieldCheck, Ticket, Vote, Zap } from 'lucide-react';
import { checkWalletExists } from '../../services/api';
import { WalletSetup } from '../WalletSetup';

const navItemClass = ({ isActive }: { isActive: boolean }) =>
  `flex items-center gap-3 px-3 py-2 rounded-lg transition-colors ${
    isActive
      ? 'bg-primary/20 text-primary font-semibold'
      : 'text-foreground hover:bg-muted/20'
  }`;

export const WalletLayout = () => {
  // Gate the sidebar on wallet existence so the WalletSetup wizard renders
  // alone when no wallet has been created yet.
  const [walletExists, setWalletExists] = useState<boolean | null>(null);

  useEffect(() => {
    let cancelled = false;
    checkWalletExists()
      .then((res) => {
        if (!cancelled) setWalletExists(res.exists);
      })
      .catch((err) => {
        console.debug('checkWalletExists failed in WalletLayout:', err);
        if (!cancelled) setWalletExists(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (walletExists === null) {
    return (
      <div className="flex items-center justify-center min-h-[60vh]">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-4 border-primary border-t-transparent" />
      </div>
    );
  }

  if (walletExists === false) {
    return <WalletSetup />;
  }

  return (
    <div className="flex gap-6">
      <aside className="w-56 shrink-0">
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
        </nav>
      </aside>
      <div className="flex-1 min-w-0">
        <Outlet />
      </div>
    </div>
  );
};
