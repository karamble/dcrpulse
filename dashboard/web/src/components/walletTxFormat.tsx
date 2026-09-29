// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import {
  ArrowDownCircle,
  ArrowLeftRight,
  ArrowUpCircle,
  BadgeDollarSign,
  Check,
  Clock,
  Coins,
  Shuffle,
  Ticket,
  X,
  Zap,
  type LucideIcon,
} from 'lucide-react';
import type { WalletTransaction } from '../services/api';
import { timeAgo } from '../utils/date';

// One set of rules for how a wallet transaction row looks, shared by the
// Overview's recent list and the Transactions page.

const categoryIcon = (tx: WalletTransaction): [LucideIcon, string] => {
  const { category, txType, isMixed } = tx;
  if (txType === 'ticket') return [Ticket, 'text-warning'];
  if (txType === 'vote') return [Check, 'text-success'];
  if (txType === 'revocation') return [X, 'text-destructive'];
  if (tx.isChannelFunding || tx.isChannelClose) return [Zap, 'text-warning'];
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
