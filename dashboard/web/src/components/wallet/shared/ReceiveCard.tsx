// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { AlertCircle, Loader2, RefreshCw } from 'lucide-react';
import { AddressGroups } from '../../AddressGroups';
import { CopyButton } from '../../explorer/CopyButton';
import { MsigReceive, freshMsigAddress } from '../../../services/msigApi';

// ReceiveCard shows the shared wallet's current receive address and
// mints the next one on demand. The ladder refuses to run more than a
// full gap window ahead of the last paid address, so every cosigner's
// wallet is guaranteed to be watching whatever address is handed out.
export const ReceiveCard = ({
  walletId,
  receive,
  m,
  n,
}: {
  walletId: string;
  receive: MsigReceive;
  m: number;
  n: number;
}) => {
  const [current, setCurrent] = useState<MsigReceive>(receive);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(
    receive.gapExhausted
      ? 'A full window of earlier addresses is still unpaid; new ones unlock when one receives funds.'
      : null,
  );

  const fresh = async () => {
    if (busy) return;
    setBusy(true);
    setNotice(null);
    try {
      setCurrent(await freshMsigAddress(walletId));
    } catch (e: any) {
      const body = e?.response?.data;
      setNotice(typeof body === 'string' ? body : e?.message || 'Could not generate an address');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
      <div className="flex items-center justify-between mb-3">
        <p className="text-sm font-medium">Receive</p>
        <span className="text-xs text-muted-foreground">address #{current.index}</span>
      </div>
      <div className="flex items-start gap-4 flex-wrap">
        <div className="p-3 bg-white rounded-lg">
          <QRCodeSVG value={current.address} size={160} level="H" />
        </div>
        {/* min-w matches the nowrap AddressGroups line so a narrow card
            wraps the address below the QR instead of clipping it. */}
        <div className="flex-1 min-w-[21rem] space-y-2">
          <AddressGroups value={current.address} />
          <div className="flex items-center gap-2">
            <CopyButton text={current.address} />
            <button
              type="button"
              onClick={fresh}
              disabled={busy}
              className="px-3 py-1.5 rounded-lg border border-border hover:bg-muted/30 text-sm disabled:opacity-50 inline-flex items-center gap-2"
            >
              {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <RefreshCw className="h-4 w-4" />}
              New address
            </button>
          </div>
          <p className="text-xs text-muted-foreground">
            Every address belongs to this wallet; a fresh one per payer keeps deposits
            apart. Spending needs {m} of the {n} cosigner keys confirmed at setup.
          </p>
          {notice && (
            <p className="text-xs text-warning flex items-start gap-1">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span>{notice}</span>
            </p>
          )}
        </div>
      </div>
    </div>
  );
};

export default ReceiveCard;
