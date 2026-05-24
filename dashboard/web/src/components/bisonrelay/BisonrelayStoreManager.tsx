// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { BisonrelayStoreProducts } from './BisonrelayStoreProducts';
import { BisonrelayStoreOrders } from './BisonrelayStoreOrders';

// BisonrelayStoreManager is the storefront admin surface shown while the node
// hosts a store: product catalog management and order fulfillment.
export const BisonrelayStoreManager = () => {
  const [tab, setTab] = useState<'products' | 'orders'>('products');
  const tabClass = (active: boolean) =>
    `px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
      active ? 'bg-primary/20 text-primary' : 'text-muted-foreground hover:text-foreground hover:bg-muted/30'
    }`;
  return (
    <div className="space-y-4">
      <div className="flex gap-1">
        <button type="button" className={tabClass(tab === 'products')} onClick={() => setTab('products')}>
          Products
        </button>
        <button type="button" className={tabClass(tab === 'orders')} onClick={() => setTab('orders')}>
          Orders
        </button>
      </div>
      {tab === 'products' ? <BisonrelayStoreProducts /> : <BisonrelayStoreOrders />}
    </div>
  );
};
