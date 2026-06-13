// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Check, Circle, Loader2, X } from 'lucide-react';

export type StageState = 'pending' | 'active' | 'done' | 'fail' | 'skip';

export interface Stage {
  key: string;
  label: string;
  state: StageState;
  detail?: string;
}

// StageList renders a vertical, step-by-step progress trace so the user can see
// exactly what is happening at every stage (hashing, submitting, anchoring,
// verifying). Each stage shows a spinner while active and a check/cross when it
// resolves.
export const StageList = ({ stages }: { stages: Stage[] }) => (
  <ol className="space-y-2.5">
    {stages.map((s) => (
      <li key={s.key} className="flex items-start gap-2.5 text-sm">
        <span className="mt-0.5 shrink-0">
          {s.state === 'done' && <Check className="h-4 w-4 text-success" />}
          {s.state === 'fail' && <X className="h-4 w-4 text-destructive" />}
          {s.state === 'active' && <Loader2 className="h-4 w-4 text-primary animate-spin" />}
          {(s.state === 'pending' || s.state === 'skip') && (
            <Circle className="h-4 w-4 text-muted-foreground/40" />
          )}
        </span>
        <span className="flex-1 min-w-0">
          <span
            className={
              s.state === 'fail'
                ? 'text-destructive'
                : s.state === 'pending' || s.state === 'skip'
                  ? 'text-muted-foreground'
                  : 'text-foreground'
            }
          >
            {s.label}
          </span>
          {s.detail && <span className="block text-xs text-muted-foreground break-words">{s.detail}</span>}
        </span>
      </li>
    ))}
  </ol>
);
