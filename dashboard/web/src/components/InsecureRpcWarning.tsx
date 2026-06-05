// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { ShieldAlert } from 'lucide-react';
import { getHealth } from '../services/api';

interface InsecureRpcWarningProps {
  kind: 'dcrd' | 'wallet';
}

// InsecureRpcWarning shows a red indicator when the dashboard's RPC connection
// to dcrd or dcrwallet is not using TLS (credentials and traffic sent in
// cleartext). It stays hidden when the connection is encrypted or not yet
// established, so a healthy TLS setup shows nothing.
export const InsecureRpcWarning = ({ kind }: InsecureRpcWarningProps) => {
  const [insecure, setInsecure] = useState(false);

  useEffect(() => {
    let active = true;
    getHealth()
      .then((h) => {
        if (!active) return;
        const connected = kind === 'dcrd' ? h.rpcConnected : h.walletRPCConnected;
        const tls = kind === 'dcrd' ? h.dcrdTLS : h.walletTLS;
        setInsecure(Boolean(connected) && !tls);
      })
      .catch(() => {
        // Health unavailable: do not show a misleading warning.
      });
    return () => {
      active = false;
    };
  }, [kind]);

  if (!insecure) return null;

  const daemon = kind === 'dcrd' ? 'dcrd' : 'dcrwallet';
  const envVar = kind === 'dcrd' ? 'DCRD_RPC_CERT' : 'DCRWALLET_RPC_CERT';

  return (
    <div
      title={`The ${daemon} RPC connection is not using TLS, so the RPC username, password, and all traffic are sent in cleartext. Point ${envVar} at the ${daemon} RPC certificate to encrypt the connection.`}
      className="flex items-center gap-2 px-4 py-3 rounded-xl bg-red-500/10 border-2 border-red-500/30"
    >
      <ShieldAlert className="h-5 w-5 text-red-500" />
      <span className="text-red-500 font-semibold text-sm">RPC not encrypted</span>
    </div>
  );
};
