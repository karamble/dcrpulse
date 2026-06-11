// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Block-explorer links for DEX swap/redeem/refund coins, mirroring bisonw's
// mainnet CoinExplorers map (client/webserver/site/src/js/coinexplorers.ts).
// dcrpulse's DEX is mainnet-only, so only the mainnet URLs are mapped. Token
// asset ids point at their parent chain's explorer, exactly as the reference
// table does.

const TAKER_FOUND = 'TakerFoundMakerRedemption:';

// ethBasedExplorerArg mirrors the reference: a 42-char coin id (or the
// TakerFoundMakerRedemption marker) is an address, otherwise it is a tx hash.
const ethBasedArg = (cid: string): [string, boolean] => {
  if (cid.startsWith(TAKER_FOUND)) return [cid.slice(TAKER_FOUND.length), true];
  if (cid.length === 42) return [cid, true];
  return [cid, false];
};

const evmExplorer = (host: string) => (cid: string): string => {
  const [arg, isAddr] = ethBasedArg(cid);
  return isAddr ? `https://${host}/address/${arg}` : `https://${host}/tx/${arg}`;
};

// UTXO coin ids are "txid:vout"; the explorer uses the txid.
const utxoTx = (host: string) => (cid: string): string => `https://${host}/tx/${cid.split(':')[0]}`;

const etherscan = evmExplorer('etherscan.io');
const polygonscan = evmExplorer('polygonscan.com');

// Decred (42) is intentionally absent: DCR coins link to dcrpulse's own internal
// explorer (/explorer/tx) rather than an external site, matching the wallet tx
// history. Only chains dcrpulse can't show itself fall through to external
// explorers below.
const EXPLORERS: Record<number, (cid: string) => string> = {
  0: utxoTx('mempool.space'), // btc
  2: (cid: string) => `https://ltc.bitaps.com/${cid.split(':')[0]}`, // ltc
  20: utxoTx('digiexplorer.info'), // dgb
  3: (cid: string) => `https://dogeblocks.com/tx/${cid.split(':')[0]}`, // doge
  5: (cid: string) => `https://blockexplorer.one/dash/mainnet/tx/${cid.split(':')[0]}`, // dash
  133: (cid: string) => `https://zcashblockexplorer.com/transactions/${cid.split(':')[0]}`, // zec
  136: utxoTx('explorer.firo.org'), // firo
  145: (cid: string) => `https://bch.loping.net/tx/${cid.split(':')[0]}`, // bch
  // eth + its tokens
  60: etherscan,
  60001: etherscan,
  60002: etherscan,
  // polygon + its tokens
  966: polygonscan,
  966001: polygonscan,
  966002: polygonscan,
  966003: polygonscan,
  966004: polygonscan,
};

// dexCoinExplorer returns a block-explorer URL for a coin id on the given asset,
// or null when the asset has no known explorer or the id is empty.
export const dexCoinExplorer = (assetID: number, cid?: string): string | null => {
  if (!cid) return null;
  const fn = EXPLORERS[assetID];
  return fn ? fn(cid) : null;
};
