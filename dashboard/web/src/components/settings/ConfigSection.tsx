// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

// domainLabels maps a capability domain key to its friendly name. Shared by the
// agent capabilities toggles and the spend-grant scope groups so both read the
// same way.
export const domainLabels: Record<string, string> = {
  node: 'Node & blockchain',
  wallet: 'Wallet',
  staking: 'Staking',
  governance: 'Governance',
  privacy: 'Privacy',
  lightning: 'Lightning',
  dex: 'DCRDEX',
  bisonrelay: 'Bison Relay',
  treasury: 'Treasury',
  explorer: 'Explorer',
  timestamp: 'Timestamps',
  tor: 'Tor',
  audit: 'Spend audit',
  brmcp: 'BR-MCP bridge',
};

export const domainLabel = (key: string) => domainLabels[key] ?? key;

type Tone = 'primary' | 'success' | 'warning' | 'muted';

// Static tone -> chip classes (Tailwind cannot see dynamically built class names).
const toneChip: Record<Tone, string> = {
  primary: 'bg-primary/10 text-primary',
  success: 'bg-success/10 text-success',
  warning: 'bg-warning/10 text-warning',
  muted: 'bg-muted/20 text-muted-foreground',
};

interface ConfigSectionProps {
  icon: LucideIcon;
  title: string;
  description?: ReactNode;
  tone?: Tone;
  dimmed?: boolean;
  children?: ReactNode;
}

// ConfigSection is a titled sub-card: an icon chip, a heading and a short
// description, then its controls. Used to group each agent-configuration concern
// into a clearly labelled, self-explaining block.
export const ConfigSection = ({
  icon: Icon,
  title,
  description,
  tone = 'primary',
  dimmed = false,
  children,
}: ConfigSectionProps) => (
  <div
    className={`rounded-lg border border-border/50 bg-muted/10 p-4 space-y-3 ${
      dimmed ? 'opacity-60' : ''
    }`}
  >
    <div className="flex items-start gap-3">
      <div className={`shrink-0 rounded-lg p-2 ${toneChip[tone]}`}>
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <h4 className="text-sm font-semibold text-foreground">{title}</h4>
        {description && <p className="text-xs text-muted-foreground mt-0.5 leading-relaxed">{description}</p>}
      </div>
    </div>
    {children}
  </div>
);
