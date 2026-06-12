// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { MMBotProblems, MMCEXProblems, MMOrderReport, MMStampedError } from '../../services/dcrdexApi';
import { toYMDTime } from '../../utils/date';

const fmtTime = (stamp: number): string => toYMDTime(new Date(stamp * 1000));

// botProblemMessages turns a bot's pre-order problems into human-readable lines
// (mirrors bisonw's botProblemMessages), explaining why a bot could not place
// orders this epoch. symbolOf resolves an asset id to a display ticker.
export const botProblemMessages = (
  problems: MMBotProblems | null | undefined,
  opts: { cexName?: string; dexHost: string; symbolOf: (assetID: number) => string },
): string[] => {
  if (!problems) return [];
  const msgs: string[] = [];
  const eachAsset = (m: Record<number, boolean> | undefined, fmt: (sym: string) => string) => {
    if (!m) return;
    for (const [id, on] of Object.entries(m)) if (on) msgs.push(fmt(opts.symbolOf(Number(id))));
  };
  eachAsset(problems.walletNotSynced, (s) => `${s} wallet not synced`);
  eachAsset(problems.noWalletPeers, (s) => `${s} wallet has no peers`);
  if (problems.accountSuspended) msgs.push(`Account suspended on ${opts.dexHost}`);
  if (problems.userLimitTooLow) msgs.push(`Account trading limit too low on ${opts.dexHost}`);
  if (problems.noPriceSource) msgs.push('No price source (oracle or fiat rate) available');
  if (problems.oracleFiatMismatch) msgs.push('Market price is outside the oracle safe range');
  if (problems.cexOrderbookUnsynced) msgs.push(`${opts.cexName ?? 'CEX'} order book not synced`);
  if (problems.causesSelfMatch) msgs.push('Order would cause a self-match');
  if (problems.unknownError) msgs.push(problems.unknownError);
  return msgs;
};

// cexProblemMessages turns CEX errors into human-readable lines (mirrors
// bisonw's cexProblemMessages).
export const cexProblemMessages = (
  problems: MMCEXProblems | null | undefined,
  symbolOf: (assetID: number) => string,
): string[] => {
  if (!problems) return [];
  const msgs: string[] = [];
  const eachErr = (m: Record<number, MMStampedError> | undefined, label: string) => {
    if (!m) return;
    for (const [id, e] of Object.entries(m)) {
      if (e) msgs.push(`${label} error on ${symbolOf(Number(id))} at ${fmtTime(e.stamp)}: ${e.error}`);
    }
  };
  eachErr(problems.depositErr, 'Deposit');
  eachErr(problems.withdrawErr, 'Withdrawal');
  if (problems.tradeErr) msgs.push(`CEX trade error at ${fmtTime(problems.tradeErr.stamp)}: ${problems.tradeErr.error}`);
  return msgs;
};

// placedCount summarizes how many of a side's placements have their target lots
// working on the book (no error and standing+ordered lots cover the target).
export const placedCount = (report: MMOrderReport | null | undefined): { placed: number; total: number } | null => {
  if (!report || !report.placements) return null;
  const total = report.placements.length;
  const placed = report.placements.filter(
    (p) => !p.error && p.standingLots + p.orderedLots >= p.lots,
  ).length;
  return { placed, total };
};
