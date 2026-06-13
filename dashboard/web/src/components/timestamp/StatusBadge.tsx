// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { AlertCircle, CheckCircle2, Clock, Hourglass, Loader2, type LucideIcon } from 'lucide-react';
import type { TimestampStatus } from '../../services/timestampApi';

const META: Record<TimestampStatus, { label: string; cls: string; Icon: LucideIcon; spin?: boolean }> = {
  anchored: { label: 'Anchored', cls: 'bg-success/15 text-success border-success/30', Icon: CheckCircle2 },
  pending: { label: 'Confirming', cls: 'bg-primary/15 text-primary border-primary/30', Icon: Loader2, spin: true },
  awaiting: { label: 'Awaiting anchor', cls: 'bg-warning/15 text-warning border-warning/30', Icon: Hourglass },
  submitted: { label: 'Submitted', cls: 'bg-warning/15 text-warning border-warning/30', Icon: Clock },
  failed: { label: 'Failed', cls: 'bg-destructive/15 text-destructive border-destructive/30', Icon: AlertCircle },
};

export const StatusBadge = ({ status }: { status: TimestampStatus }) => {
  const m = META[status] ?? META.submitted;
  const Icon = m.Icon;
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs border whitespace-nowrap ${m.cls}`}>
      <Icon className={`h-3.5 w-3.5 ${m.spin ? 'animate-spin' : ''}`} />
      {m.label}
    </span>
  );
};
