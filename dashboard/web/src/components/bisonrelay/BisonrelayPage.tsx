// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { BisonrelaySetupWizard } from './BisonrelaySetupWizard';
import { BisonrelayMessagingPage } from './BisonrelayMessagingPage';
import { BisonrelayStatus, getBisonrelayStatus } from '../../services/bisonrelayApi';

export const BisonrelayPage = () => {
  const [ready, setReady] = useState<BisonrelayStatus | null>(null);

  useEffect(() => {
    if (!ready) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const s = await getBisonrelayStatus();
        if (!cancelled && s.stage === 'ready') setReady(s);
      } catch {
        /* keep last known */
      }
    };
    const id = setInterval(tick, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [ready]);

  if (ready) {
    return <BisonrelayMessagingPage ownNick={ready.nick ?? 'unknown'} />;
  }

  return (
    <BisonrelaySetupWizard
      onReady={async () => {
        try {
          const s = await getBisonrelayStatus();
          setReady(s);
        } catch {
          /* ignored - wizard keeps rendering */
        }
      }}
    />
  );
};
