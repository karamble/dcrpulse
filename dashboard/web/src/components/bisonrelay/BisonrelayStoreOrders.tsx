// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import { ChevronDown, ChevronRight } from 'lucide-react';
import {
  addBisonrelayStoreOrderComment,
  BisonrelayStoreOrder,
  getBisonrelayStoreOrders,
  setBisonrelayStoreOrderStatus,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

const STATUSES = ['placed', 'paid', 'shipped', 'completed', 'canceled'];

const STATUS_CLASS: Record<string, string> = {
  placed: 'bg-sky-500/15 text-sky-300',
  paid: 'bg-emerald-500/15 text-emerald-300',
  shipped: 'bg-violet-500/15 text-violet-300',
  completed: 'bg-muted/50 text-muted-foreground',
  canceled: 'bg-rose-500/15 text-rose-300',
};

const orderTotal = (o: BisonrelayStoreOrder): number => {
  const items = o.cart?.items ?? [];
  const sub = items.reduce((acc, it) => acc + (it.product?.price ?? 0) * it.quantity, 0);
  return sub + (o.ship_charge ?? 0);
};

// BisonrelayStoreOrders lists storefront orders and lets the merchant change an
// order's status. Status changes update the order on disk; the customer sees
// the new status when they next view it (no push DM in this phase).
export const BisonrelayStoreOrders = () => {
  const [orders, setOrders] = useState<BisonrelayStoreOrder[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const { addListener } = useBisonrelayLive();

  const load = useCallback(() => {
    getBisonrelayStoreOrders()
      .then((o) => {
        setOrders(o);
        setErr(null);
      })
      .catch((e: any) => setErr(e?.response?.data || e?.message || 'Could not load orders'));
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  // Refresh when the store reports a new or changed order (placed/paid events).
  useEffect(
    () =>
      addListener((evt) => {
        if (evt.type === 'store-order-placed' || evt.type === 'store-order-status') {
          load();
        }
      }),
    [addListener, load],
  );

  const sendComment = async (o: BisonrelayStoreOrder) => {
    const key = `${o.user}/${o.id}`;
    const text = (draft[key] ?? '').trim();
    if (!text) return;
    setBusy(key);
    setErr(null);
    try {
      await addBisonrelayStoreOrderComment(o.user, o.id, text);
      setDraft((d) => ({ ...d, [key]: '' }));
      load();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Could not send comment');
    } finally {
      setBusy(null);
    }
  };

  const changeStatus = async (o: BisonrelayStoreOrder, status: string) => {
    const key = `${o.user}/${o.id}`;
    setBusy(key);
    setErr(null);
    try {
      await setBisonrelayStoreOrderStatus(o.user, o.id, status);
      load();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Could not update status');
    } finally {
      setBusy(null);
    }
  };

  if (err && orders === null) {
    return <div className="text-sm text-rose-300">{err}</div>;
  }
  if (orders === null) {
    return <div className="text-sm text-muted-foreground">Loading…</div>;
  }

  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-lg font-semibold">Orders</h2>
        <p className="text-xs text-muted-foreground">
          Orders customers placed over the relay, newest first.
        </p>
      </div>

      {err && (
        <div className="rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">
          {err}
        </div>
      )}

      {orders.length === 0 ? (
        <div className="rounded-xl border border-border/50 bg-gradient-card p-6 text-sm text-muted-foreground">
          No orders yet.
        </div>
      ) : (
        <ul className="rounded-xl border border-border/50 bg-gradient-card divide-y divide-border/40">
          {orders.map((o) => {
            const key = `${o.user}/${o.id}`;
            const items = o.cart?.items ?? [];
            return (
              <li key={key} className="px-4 py-3 text-sm">
                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => setOpen((s) => ({ ...s, [key]: !s[key] }))}
                    className="text-muted-foreground hover:text-foreground"
                  >
                    {open[key] ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                  </button>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">#{o.id}</span>
                      <span
                        className={`text-[10px] px-1.5 py-0.5 rounded ${
                          STATUS_CLASS[o.status] || 'bg-muted/50 text-muted-foreground'
                        }`}
                      >
                        {o.status}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {items.length} item{items.length === 1 ? '' : 's'} · ${orderTotal(o).toFixed(2)}
                      </span>
                    </div>
                    <div className="text-[11px] text-muted-foreground font-mono truncate">
                      {o.user.slice(0, 12)}… · {o.pay_type} · {toYMDTime(new Date(o.placed_ts))}
                    </div>
                  </div>
                  <select
                    value={o.status}
                    disabled={busy === key}
                    onChange={(e) => changeStatus(o, e.target.value)}
                    className="rounded-md border border-border bg-background px-2 py-1 text-xs disabled:opacity-50"
                  >
                    {STATUSES.map((s) => (
                      <option key={s} value={s}>
                        {s}
                      </option>
                    ))}
                  </select>
                </div>

                {open[key] && (
                  <div className="mt-3 ml-7 space-y-2 text-xs text-muted-foreground">
                    <ul className="space-y-0.5">
                      {items.map((it, i) => (
                        <li key={i}>
                          {it.quantity} × {it.product?.title || it.product?.sku || 'item'}{' '}
                          <span className="opacity-70">(${it.product?.price ?? 0} ea)</span>
                        </li>
                      ))}
                    </ul>
                    {o.ship_charge > 0 && <div>Shipping: ${o.ship_charge.toFixed(2)}</div>}
                    {o.shipping && o.shipping.name && (
                      <div>
                        Ship to: {o.shipping.name}, {o.shipping.address1}
                        {o.shipping.address2 ? `, ${o.shipping.address2}` : ''}, {o.shipping.city}{' '}
                        {o.shipping.state} {o.shipping.postalCode} {o.shipping.countrycode}
                      </div>
                    )}
                    {o.invoice && <div className="font-mono break-all">Invoice: {o.invoice}</div>}

                    <div className="pt-1 space-y-1">
                      <div className="text-foreground/80 font-medium">Messages</div>
                      {(o.comments ?? []).length === 0 ? (
                        <div className="opacity-70">No messages yet.</div>
                      ) : (
                        (o.comments ?? []).map((c, i) => (
                          <div key={i}>
                            <span className={c.fromAdmin ? 'text-primary' : 'text-foreground/80'}>
                              {c.fromAdmin ? 'You' : 'Customer'}
                            </span>{' '}
                            <span className="opacity-60">{toYMDTime(new Date(c.ts))}</span>: {c.comment}
                          </div>
                        ))
                      )}
                      <div className="flex gap-2 pt-1">
                        <input
                          value={draft[key] ?? ''}
                          onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') sendComment(o);
                          }}
                          placeholder="Reply to the customer…"
                          className="flex-1 rounded-md border border-border bg-background px-2 py-1 text-xs"
                        />
                        <button
                          type="button"
                          disabled={busy === key || !(draft[key] ?? '').trim()}
                          onClick={() => sendComment(o)}
                          className="px-3 py-1 rounded-md bg-primary/20 text-primary text-xs font-semibold hover:bg-primary/30 disabled:opacity-50"
                        >
                          Send
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};
