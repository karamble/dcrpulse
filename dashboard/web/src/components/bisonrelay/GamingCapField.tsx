// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { validateDcrAmount } from '../../utils/amounts';

// A cap on what a game may stake, where "no limit" is a choice and never a
// value somebody arrived at by accident.
//
// The field this replaces was a number input whose handler read
// `Number(e.target.value) || 0`, and the server reads zero as no cap at all.
// So clearing the box, or typing something the browser would not parse, turned
// a limit into its absence - a control labelled "Max buy-in per table" that
// read as its most restrictive setting and behaved as its least.
//
// Two things fix it. The amount is held as text, because a controlled number
// input cannot tell an empty field from a zero nor represent the moment
// halfway through typing "0.5"; and no-limit is a separate button, so it takes
// a deliberate press and a save rather than a backspace.

export interface CapValue {
  // limited false is the no-limit choice. amount is only meaningful when true.
  limited: boolean;
  amount: string;
}

// capFromDcr reads the stored number, where zero is the server's no-limit
// sentinel, into something the form can hold.
export const capFromDcr = (dcr: number): CapValue =>
  dcr > 0 ? { limited: true, amount: String(dcr) } : { limited: false, amount: '' };

// capToDcr writes it back. This is the only place the sentinel is produced, so
// nothing else in the interface has to reason about what zero means.
export const capToDcr = (v: CapValue): number => (v.limited ? Number(v.amount.trim()) : 0);

// capError is the reason this cap cannot be saved, or null.
export const capError = (v: CapValue): string | null =>
  v.limited ? validateDcrAmount(v.amount, { allowZero: false }) : null;

export const GamingCapField = ({
  label,
  hint,
  value,
  onChange,
}: {
  label: string;
  hint?: string;
  value: CapValue;
  onChange: (next: CapValue) => void;
}) => {
  const err = value.amount.trim() ? capError(value) : null;

  return (
    <label className="text-xs space-y-1">
      <span className="text-muted-foreground block">{label}</span>

      <div className="flex gap-1 p-0.5 rounded-lg bg-muted/20 border border-border/50">
        {[
          { on: true, text: 'Limit' },
          { on: false, text: 'No limit' },
        ].map((opt) => (
          <button
            key={opt.text}
            type="button"
            onClick={() => onChange({ ...value, limited: opt.on })}
            className={`flex-1 px-2 py-1 rounded-md text-xs font-medium transition-colors ${
              value.limited === opt.on
                ? 'bg-primary/20 text-primary'
                : 'text-muted-foreground hover:bg-muted/30'
            }`}
          >
            {opt.text}
          </button>
        ))}
      </div>

      {value.limited ? (
        <>
          {/* Text rather than a number input: the exponent and hex forms
           * Number() accepts never reach validateDcrAmount otherwise, and a
           * scroll wheel over a number input should not change what a game
           * may stake. */}
          <input
            type="text"
            inputMode="decimal"
            value={value.amount}
            onChange={(e) => onChange({ ...value, amount: e.target.value })}
            placeholder="0.05"
            className="w-full px-2 py-1.5 rounded-lg bg-background border border-border text-sm"
          />
          {err && <span className="text-destructive block">{err}</span>}
        </>
      ) : (
        <span className="text-warning block">
          No limit. Nothing here bounds what this game may stake.
        </span>
      )}

      {hint && <span className="text-muted-foreground block">{hint}</span>}
    </label>
  );
};
