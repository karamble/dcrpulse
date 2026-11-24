// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Fragment } from 'react';
import {
  ArrowDownCircle,
  ArrowLeftRight,
  ArrowUpCircle,
  BadgeDollarSign,
  Check,
  Clock,
  Coins,
  Repeat2,
  Shuffle,
  Split,
  Ticket,
  Undo2,
  X,
  Zap,
  type LucideIcon,
} from 'lucide-react';
import type { TxDexSwap, WalletTransaction } from '../services/api';
import { timeAgo } from '../utils/date';
import { fmtMatch, prettyStatus } from './dex/dexFormat';

// One set of rules for how a wallet transaction row looks, shared by the
// Overview's recent list and the Transactions page.

// DEX rows use bisonw's own names for its tx types.
const dexIcons: Record<TxDexSwap['role'], LucideIcon> = { swap: Repeat2, redeem: Repeat2, refund: Undo2, funding: Split };
const dexLabels: Record<TxDexSwap['role'], string> = {
  swap: 'DEX Swap',
  redeem: 'DEX Redeem',
  refund: 'DEX Refund',
  funding: 'DEX Order Funding',
};

const categoryIcon = (tx: WalletTransaction): [LucideIcon, string] => {
  const { category, txType, isMixed } = tx;
  if (txType === 'ticket') return [Ticket, 'text-warning'];
  if (txType === 'vote') return [Check, 'text-success'];
  if (txType === 'revocation') return [X, 'text-destructive'];
  if (tx.isChannelFunding || tx.isChannelClose) return [Zap, 'text-warning'];
  if (tx.dexSwap) return [dexIcons[tx.dexSwap.role] ?? Repeat2, 'text-cyan-600'];
  if (category === 'vspfee') return [BadgeDollarSign, 'text-orange-500'];
  if (category === 'coinjoin' || (isMixed && (category === 'send' || category === 'receive'))) {
    return [Shuffle, 'text-purple-500'];
  }
  if (category === 'send') return [ArrowUpCircle, 'text-red-500'];
  if (category === 'receive') return [ArrowDownCircle, 'text-success'];
  if (category === 'self') return [ArrowLeftRight, 'text-muted-foreground'];
  if (category === 'generate') return [Coins, 'text-primary'];
  if (category === 'immature') return [Clock, 'text-muted-foreground'];
  return [Coins, 'text-muted-foreground'];
};

export const TxCategoryIcon = ({ tx, className }: { tx: WalletTransaction; className: string }) => {
  const [Icon, color] = categoryIcon(tx);
  return <Icon className={`${className} ${color}`} />;
};

export const txCategoryLabel = (tx: WalletTransaction) => {
  const { category, txType, isMixed } = tx;
  if (txType === 'ticket') return 'Ticket Purchase';
  if (txType === 'vote') return 'Vote';
  if (txType === 'revocation') return 'Revocation';
  if (tx.isChannelFunding) return 'Channel Open';
  if (tx.isChannelClose) return 'Channel Close';
  if (tx.dexSwap) return dexLabels[tx.dexSwap.role] ?? 'DEX Swap';
  if (category === 'vspfee') return 'VSP Fee';
  if (category === 'coinjoin') return 'CoinJoin';
  if (category === 'send') return isMixed ? 'Sent (CoinJoin)' : 'Sent';
  if (category === 'receive') return isMixed ? 'Received (CoinJoin)' : 'Received';
  if (category === 'self') return 'Self Transfer';
  if (category === 'generate') return 'Mined';
  if (category === 'immature') return 'Immature';
  return 'Transaction';
};

export const txAmountColor = (tx: WalletTransaction) => {
  if (tx.txType === 'ticket') return 'text-warning';
  if (tx.txType === 'vote') return 'text-success';
  if (tx.txType === 'revocation') return 'text-destructive';
  if (tx.category === 'vspfee') return 'text-orange-500';
  if (tx.category === 'coinjoin') return 'text-purple-500';
  if (tx.category === 'send') return 'text-red-500';
  if (tx.category === 'receive') return 'text-success';
  if (tx.category === 'generate') return 'text-primary';
  return 'text-muted-foreground';
};

// formatTxAmount signs an amount in DCR. adaptive shows 4 decimals, and 8 for
// fee-sized amounts below 0.001 DCR that 4 would hide; otherwise always 8.
export const formatTxAmount = (amount: number, adaptive = false) => {
  const abs = Math.abs(amount);
  const sign = amount < 0 ? '-' : '+';
  const decimals = adaptive && !(abs > 0 && abs < 0.001) ? 4 : 8;
  return `${sign}${abs.toFixed(decimals)} DCR`;
};

// txWhen is how long ago the transaction was mined, or seen while pending.
export const txWhen = (tx: WalletTransaction) => timeAgo(tx.blockTime ? tx.blockTime * 1000 : tx.time);

// isFoldedTx marks a DEX split whose fee its swap row shows; lists skip it.
export const isFoldedTx = (tx: WalletTransaction) => !!tx.dexSwap?.foldedInto;

// dexPair names what the trade gave and got, e.g. "DCR → BTC".
export const dexPair = (s: TxDexSwap) => {
  const base = s.baseSymbol.toUpperCase();
  const quote = s.quoteSymbol.toUpperCase();
  return s.sell ? `${base} → ${quote}` : `${quote} → ${base}`;
};

// dexStatus is the row's one status word, the worst across its matches; a
// finished trade has none.
export const dexStatus = (s: TxDexSwap): { word: string; tone: 'warning' | 'destructive' } | null => {
  const ms = s.matches ?? [];
  if (s.role !== 'refund' && ms.some((m) => m.refunded)) return { word: 'Refunded', tone: 'destructive' };
  if (ms.some((m) => m.revoked)) return { word: 'Revoked', tone: 'destructive' };
  if (ms.some((m) => m.active)) return { word: 'Settling', tone: 'warning' };
  return null;
};

const statusText = { warning: 'text-warning', destructive: 'text-destructive' };
const statusChip = {
  warning: 'text-xs px-2 py-0.5 rounded bg-warning/10 text-warning',
  destructive: 'text-xs px-2 py-0.5 rounded bg-destructive/10 text-destructive',
};

// DexStatusChip is the History row's status chip; its title lists bisonw's
// own match states.
export const DexStatusChip = ({ swap }: { swap: TxDexSwap }) => {
  const st = dexStatus(swap);
  if (!st) return null;
  const states = (swap.matches ?? []).map((m) => prettyStatus(m.status)).join(', ');
  return (
    <span className={statusChip[st.tone]} title={states}>
      {st.word}
    </span>
  );
};

// DexRecentWhen is the Recent row's time line: when, the status word if any,
// then the pair, so a narrow row cuts the pair first.
export const DexRecentWhen = ({ tx }: { tx: WalletTransaction }) => {
  const s = tx.dexSwap;
  const st = s ? dexStatus(s) : null;
  return (
    <div className="text-xs text-muted-foreground truncate">
      {txWhen(tx)}
      {st && (
        <>
          {' • '}
          <span className={statusText[st.tone]}>{st.word}</span>
        </>
      )}
      {s && ` • ${dexPair(s)}`}
    </div>
  );
};

// DexSwapLine is the History row's detail: the pair and each match as the
// DEX order detail shows it; past three, two and a count with all in its title.
export const DexSwapLine = ({ swap }: { swap: TxDexSwap }) => {
  const base = swap.baseSymbol.toUpperCase();
  const quote = swap.quoteSymbol.toUpperCase();
  const all = (swap.matches ?? []).map((m) => fmtMatch(m.qty, m.rate, swap.baseConv, swap.quoteConv, base, quote));
  const shown = all.length > 3 ? all.slice(0, 2) : all;
  const more = all.length - shown.length;
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 mt-0.5 text-xs text-muted-foreground" title={swap.host}>
      <span className="sm:whitespace-nowrap">{dexPair(swap)}</span>
      {shown.map((text, i) => (
        <Fragment key={i}>
          <span>•</span>
          <span className="font-mono tabular-nums sm:whitespace-nowrap">{text}</span>
        </Fragment>
      ))}
      {more > 0 && (
        <>
          <span>•</span>
          <span className="whitespace-nowrap" title={all.join('\n')}>
            +{more} more
          </span>
        </>
      )}
    </div>
  );
};
