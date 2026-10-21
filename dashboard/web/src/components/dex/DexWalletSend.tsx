// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import {
  estimateDexSendFee,
  hasTrait,
  WalletTrait,
  type DexWalletState,
  type DexAssetInfo,
  type DexSendFee,
} from '../../services/dcrdexApi';
import { fmtAmt } from './dexFormat';
import { DexSendConfirmModal } from './DexSendConfirmModal';

interface Props {
  wallet: DexWalletState;
  asset?: DexAssetInfo | null;
  onSent: () => void;
}

// DexWalletSend is a send form for a wallet. The amount is conventional; the
// backend converts to atoms. Spending is confirmed in DexSendConfirmModal.
export const DexWalletSend = ({ wallet, asset, onSent }: Props) => {
  const [address, setAddress] = useState('');
  const [amount, setAmount] = useState('');
  const [confirming, setConfirming] = useState(false);
  const [sent, setSent] = useState<string | null>(null);
  const [fee, setFee] = useState<DexSendFee | null>(null);
  // Only a withdrawer wallet can take the fee out of the amount, as in bisonw.
  const canSubtract = hasTrait(wallet.traits, WalletTrait.Withdrawer);
  const [subtractBox, setSubtractBox] = useState(false);
  const subtract = canSubtract && subtractBox;

  // Display precision from the asset's conversion factor (e.g. DCR 1e8 -> 8,
  // POL 1e9 -> 9). The actual atom conversion happens in the backend per-asset.
  const decimals = asset ? Math.max(0, Math.round(Math.log10(asset.conversionFactor))) : 8;
  // Account-based chains pay the network/gas fee from the native (parent) balance
  // separately from the sent amount, so the full balance is not spendable as-is.
  const feeNote = asset?.isAccountBased
    ? `Network (gas) fees are paid from your ${asset.parentSymbol ?? wallet.symbol} balance, separately from this amount. Leave some headroom when sending the max.`
    : null;

  const value = parseFloat(amount);
  const valid = address.trim() !== '' && value > 0 && value <= wallet.available;

  // Debounced fee estimate from the bisonw webserver once an address + amount are
  // entered; also reports whether the address is valid for this asset.
  useEffect(() => {
    const addr = address.trim();
    if (!addr || !(value > 0)) {
      setFee(null);
      return;
    }
    let cancelled = false;
    const t = window.setTimeout(() => {
      estimateDexSendFee(wallet.assetID, value, addr, subtract)
        .then((f) => !cancelled && setFee(f))
        .catch(() => !cancelled && setFee(null));
    }, 500);
    return () => {
      cancelled = true;
      window.clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [address, amount, subtract, wallet.assetID]);

  const sentCoin = (coin: string) => {
    setSent(coin);
    setAddress('');
    setAmount('');
    setSubtractBox(false);
    setConfirming(false);
    onSent();
  };

  return (
    <div className="space-y-3">
      <div>
        <label className="block text-xs text-muted-foreground mb-1">Recipient address</label>
        <input
          value={address}
          onChange={(e) => {
            setAddress(e.target.value);
            setSent(null);
          }}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary"
        />
      </div>
      <div>
        <label className="flex items-center justify-between text-xs text-muted-foreground mb-1">
          <span>Amount ({wallet.symbol})</span>
          <button
            type="button"
            className="hover:text-foreground"
            onClick={() => {
              setAmount(String(wallet.available));
              setSubtractBox(canSubtract);
            }}
          >
            max {fmtAmt(wallet.available, decimals)}
          </button>
        </label>
        <input
          type="number"
          min={0}
          value={amount}
          onChange={(e) => {
            setAmount(e.target.value);
            setSent(null);
          }}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary"
        />
      </div>

      {canSubtract && (
        <label className="flex items-center gap-1.5 text-[11px] text-muted-foreground cursor-pointer select-none">
          <input type="checkbox" checked={subtract} onChange={(e) => setSubtractBox(e.target.checked)} className="accent-primary" />
          Subtract fees from amount sent.
        </label>
      )}
      {feeNote && <p className="text-[11px] text-muted-foreground">{feeNote}</p>}
      {fee && (
        <p className="text-[11px] text-muted-foreground">
          Estimated network fee: ~{fmtAmt(fee.fee, 8)} {fee.feeSymbol}
          {!fee.validAddress && <span className="text-warning"> &middot; address may be invalid</span>}
        </p>
      )}

      {sent && (
        <div className="p-2.5 rounded-lg bg-success/10 border border-success/30 text-xs text-success break-all">
          Sent. Coin: {sent}
        </div>
      )}

      <button
        type="button"
        disabled={!valid}
        onClick={() => setConfirming(true)}
        className="w-full bg-gradient-primary text-white font-semibold rounded-lg px-4 py-2 transition-colors hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Send
      </button>
      {confirming && (
        <DexSendConfirmModal
          assetID={wallet.assetID}
          symbol={wallet.symbol}
          address={address.trim()}
          value={value}
          decimals={decimals}
          subtract={subtract}
          onClose={() => setConfirming(false)}
          onSent={sentCoin}
        />
      )}
    </div>
  );
};
