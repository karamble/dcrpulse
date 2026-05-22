import { NavLink, Outlet } from 'react-router-dom';
import { LayoutDashboard, Network, Send, Inbox, Wrench } from 'lucide-react';

const navItemClass = ({ isActive }: { isActive: boolean }) =>
  `flex items-center gap-3 px-3 py-2 rounded-lg transition-colors text-sm whitespace-nowrap shrink-0 ${
    isActive
      ? 'bg-primary/20 text-primary font-semibold'
      : 'text-foreground hover:bg-muted/20'
  }`;

const comingSoonClass =
  'flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-muted-foreground/60 cursor-not-allowed whitespace-nowrap shrink-0';

export const LightningLayout = () => (
  <div className="flex flex-col md:flex-row gap-4 md:gap-6">
    {/* Content area: rendered first in source so screen readers see the
        meaningful content before the navigation. On md+ it occupies the
        left/main column; on mobile it falls below the nav via order-2. */}
    <div className="flex-1 min-w-0 order-2 md:order-1">
      <Outlet />
    </div>
    <aside className="md:w-36 shrink-0 order-1 md:order-2">
      <nav
        className="flex md:flex-col gap-1 overflow-x-auto md:overflow-visible -mx-2 px-2 md:mx-0 md:px-0 pb-1 md:pb-0"
        aria-label="Lightning navigation"
      >
        <NavLink to="/wallet/lightning" end className={navItemClass}>
          <LayoutDashboard className="h-4 w-4" />
          <span>Overview</span>
        </NavLink>
        <NavLink to="/wallet/lightning/channels" className={navItemClass}>
          <Network className="h-4 w-4" />
          <span>Channels</span>
        </NavLink>
        <NavLink to="/wallet/lightning/send" className={navItemClass}>
          <Send className="h-4 w-4" />
          <span>Send</span>
        </NavLink>
        <NavLink to="/wallet/lightning/receive" className={navItemClass}>
          <Inbox className="h-4 w-4" />
          <span>Receive</span>
        </NavLink>
        <div className={comingSoonClass} title="Coming in a follow-up PR">
          <Wrench className="h-4 w-4" />
          <span>Advanced</span>
        </div>
      </nav>
    </aside>
  </div>
);
