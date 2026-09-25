// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertTriangle } from 'lucide-react';
import type { MCPGrant, MCPWriteScope } from '../../services/api';

// peerCanSteerSpend reports whether Bison Relay peers can message an agent that
// can also move funds without the operator's approval.
export const peerCanSteerSpend = (
  domains: string[],
  grant: MCPGrant | undefined,
  scopes: MCPWriteScope[],
  oversightOn: boolean,
): boolean => {
  if (oversightOn || !grant || !domains.includes('bisonrelay')) return false;
  const fundScopes = new Set(scopes.filter((s) => s.fund).map((s) => s.key));
  return grant.accounts.length > 0 || grant.writeScopes.some((k) => fundScopes.has(k));
};

export const PeerSpendNotice = (props: {
  domains: string[];
  grant: MCPGrant | undefined;
  scopes: MCPWriteScope[];
  oversightOn: boolean;
}) => {
  if (!peerCanSteerSpend(props.domains, props.grant, props.scopes, props.oversightOn)) return null;
  return (
    <div role="note" className="flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm">
      <AlertTriangle className="h-4 w-4 text-warning mt-0.5 shrink-0" />
      <span>
        Bison Relay contacts can message this agent, and it can spend. With Bison Relay oversight off,
        its spends are limited only by the caps below. Turn on oversight above to approve each one.
      </span>
    </div>
  );
};
