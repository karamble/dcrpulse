// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { BisonrelayStoreProducts } from './BisonrelayStoreProducts';
import { BisonrelayStoreOrders } from './BisonrelayStoreOrders';
import { BisonrelayStoreTemplates } from './BisonrelayStoreTemplates';
import { BisonrelayStoreAssets } from './BisonrelayStoreAssets';

type StoreTab = 'products' | 'orders' | 'assets' | 'templates';

// BisonrelayStoreManager is the storefront admin surface shown while the node
// hosts a store: product catalog, order fulfillment, and template editing.
export const BisonrelayStoreManager = () => {
  const [tab, setTab] = useState<StoreTab>('products');
  const tabClass = (active: boolean) =>
    `shrink-0 whitespace-nowrap px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
      active ? 'bg-primary/20 text-primary' : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
    }`;
  const tabs: { id: StoreTab; label: string }[] = [
    { id: 'products', label: 'Products' },
    { id: 'orders', label: 'Orders' },
    { id: 'assets', label: 'Assets' },
    { id: 'templates', label: 'Templates' },
  ];
  return (
    <div className="space-y-4">
      <div className="flex gap-1 overflow-x-auto overflow-y-hidden">
        {tabs.map((t) => (
          <button key={t.id} type="button" className={tabClass(tab === t.id)} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      {tab === 'products' && <BisonrelayStoreProducts />}
      {tab === 'orders' && <BisonrelayStoreOrders />}
      {tab === 'assets' && <BisonrelayStoreAssets />}
      {tab === 'templates' && <BisonrelayStoreTemplates />}
    </div>
  );
};
