// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType } from 'react';

export interface BrSidebarItem {
  id: string;
  label: string;
  hash: string;
  icon: ComponentType<{ className?: string }>;
}

// navigateTo drives the hash-based sub-navigation the BR pages share.
export const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

// BrSidebar is the section rail of the BR sub-pages. isActive lets a page
// treat nested views as belonging to a listed section.
export const BrSidebar = ({
  items,
  active,
  isActive,
}: {
  items: BrSidebarItem[];
  active: string;
  isActive?: (itemId: string, active: string) => boolean;
}) => (
  <aside className="md:w-44 shrink-0 rounded-xl bg-gradient-card border border-border/50 p-2 md:self-start">
    <nav className="flex md:flex-col gap-1 overflow-x-auto overflow-y-hidden md:overflow-visible">
      {items.map((item) => {
        const on = isActive ? isActive(item.id, active) : item.id === active;
        const Icon = item.icon;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => navigateTo(item.hash)}
            className={`shrink-0 whitespace-nowrap md:w-full px-3 py-2 rounded-md text-sm flex items-center gap-2 text-left transition-colors ${
              on
                ? 'bg-primary/20 text-primary font-semibold'
                : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground'
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span>{item.label}</span>
          </button>
        );
      })}
    </nav>
  </aside>
);
