import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, ArrowLeftRight, Users, ShieldCheck, Ticket } from 'lucide-react';

const navItemClass = ({ isActive }: { isActive: boolean }) =>
  `flex items-center gap-3 px-3 py-2 rounded-lg transition-colors ${
    isActive
      ? 'bg-primary/20 text-primary font-semibold'
      : 'text-foreground hover:bg-muted/20'
  }`;

export const WalletLayout = () => (
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
        <NavLink to="/wallet/accounts" className={navItemClass}>
          <Users className="h-4 w-4" />
          <span>Accounts</span>
        </NavLink>
      </nav>
    </aside>
    <div className="flex-1 min-w-0">
      <Outlet />
    </div>
  </div>
);
