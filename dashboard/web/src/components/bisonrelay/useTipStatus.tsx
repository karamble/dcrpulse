// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { BisonrelayLiveEvent, tipBisonrelayContact } from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { apiError } from '../../utils/apiError';

export interface TipStatus {
  state: 'requesting' | 'paying' | 'sent' | 'failed';
  line: string;
}

// useTipStatus follows one contact's tip from submit to its outcome. Tips are
// fire-and-forget: the live tip-invoice-generated event upgrades the line and
// tip-sent / tip-failed carry the final line in bruig's wording. onSettled runs
// on either outcome.
export const useTipStatus = (uid: string, nick: string, onSettled?: () => void) => {
  const [status, setStatus] = useState<TipStatus | null>(null);
  const { addListener } = useBisonrelayLive();

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      const p = (evt.payload ?? {}) as Record<string, unknown>;
      if (evt.type === 'tip-invoice-generated') {
        if (p.uid !== uid) return;
        setStatus((prev) =>
          prev?.state === 'requesting'
            ? { state: 'paying', line: `Invoice received, paying tip to ${nick}...` }
            : prev,
        );
        return;
      }
      if (evt.type !== 'tip-sent' && evt.type !== 'tip-failed') return;
      if (p.recipient !== uid || typeof p.line !== 'string' || !p.line) return;
      setStatus({ state: evt.type === 'tip-sent' ? 'sent' : 'failed', line: p.line });
      onSettled?.();
    });
  }, [addListener, uid, nick, onSettled]);

  const submit = (dcrAmount: number) => {
    setStatus({ state: 'requesting', line: `Requesting invoice for ${dcrAmount} DCR to tip ${nick}...` });
    tipBisonrelayContact(uid, dcrAmount).catch((e: any) => {
      const msg = apiError(e, 'Tip failed');
      setStatus({
        state: 'failed',
        line: `Tip attempt of ${dcrAmount} DCR failed due to ${msg}. Given up on attempting to tip.`,
      });
    });
  };

  return { status, submit };
};

// TipStatusLine shows a tip's progress: a spinner while it is in flight, green
// once sent, red when it failed. Layout classes come from the caller.
export const TipStatusLine = ({
  status,
  className,
  lineClassName,
}: {
  status: TipStatus | null;
  className: string;
  lineClassName: string;
}) => {
  if (!status) return null;
  const tone =
    status.state === 'sent' ? 'text-success' : status.state === 'failed' ? 'text-destructive' : 'text-muted-foreground';
  return (
    <div className={`${className} ${tone}`}>
      {(status.state === 'requesting' || status.state === 'paying') && (
        <Loader2 className="h-3 w-3 shrink-0 animate-spin" />
      )}
      <span className={lineClassName} title={status.line}>
        {status.line}
      </span>
    </div>
  );
};
