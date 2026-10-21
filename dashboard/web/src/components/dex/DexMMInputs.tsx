// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useId } from 'react';
import { Minus, Plus } from 'lucide-react';

export const inputCls =
  'px-2.5 py-1.5 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary';

const labelCls = 'text-[11px] uppercase tracking-wider text-muted-foreground/70';

export const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <label className="flex flex-col gap-1">
    <span className={labelCls}>{label}</span>
    {children}
  </label>
);

export const Slider = ({
  label,
  value,
  min,
  max,
  step,
  suffix,
  hint,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  min: number;
  max: number;
  step: number;
  suffix?: string;
  hint?: string;
  disabled?: boolean;
  onChange: (v: string) => void;
}) => (
  <div className={`space-y-1 ${disabled ? 'opacity-40' : ''}`}>
    <div className="flex items-center justify-between">
      <span className={labelCls}>{label}</span>
      <span className="font-mono text-sm">
        {value}
        {suffix}
      </span>
    </div>
    <input
      type="range"
      min={min}
      max={max}
      step={step}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className="w-full accent-primary"
    />
    {hint && <div className="text-[10px] text-muted-foreground">{hint}</div>}
  </div>
);

// atLeast returns v, or min when v is blank, not a number or below min, as
// bisonw's number inputs do when they lose focus.
const atLeast = (v: string, min: number): string => {
  const n = Number(v);
  return v.trim() === '' || !Number.isFinite(n) || n < min ? String(min) : v;
};

// NumberSlider is a typed value with a slider beside it. The slider covers the
// usual range; the typed value may go past it but not below min.
export const NumberSlider = ({
  label,
  value,
  min,
  max,
  step,
  suffix,
  hint,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  min: number;
  max: number;
  step: number;
  suffix?: string;
  hint?: string;
  disabled?: boolean;
  onChange: (v: string) => void;
}) => {
  const id = useId();
  return (
    <div className={`space-y-1 ${disabled ? 'opacity-40' : ''}`}>
      <div className="flex items-center justify-between gap-3">
        <label htmlFor={id} className={labelCls}>
          {label}
        </label>
        <span className="flex items-center gap-1">
          <input
            id={id}
            type="number"
            min={min}
            step={step}
            value={value}
            disabled={disabled}
            onChange={(e) => onChange(e.target.value)}
            onBlur={(e) => onChange(atLeast(e.target.value, min))}
            className={`${inputCls} w-24 text-right`}
          />
          {suffix && <span className="text-sm text-muted-foreground">{suffix}</span>}
        </span>
      </div>
      <input
        type="range"
        aria-label={`${label} slider`}
        min={min}
        max={max}
        step={step}
        value={Math.min(max, Math.max(min, Number(value) || min))}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className="w-full accent-primary"
      />
      {hint && <div className="text-[10px] text-muted-foreground">{hint}</div>}
    </div>
  );
};

// Stepper is a whole-number input with step buttons, bisonw's IncrementalInput.
export const Stepper = ({
  label,
  value,
  min,
  step,
  hint,
  action,
  onChange,
}: {
  label: string;
  value: string;
  min: number;
  step: number;
  hint?: string;
  action?: React.ReactNode;
  onChange: (v: string) => void;
}) => {
  const id = useId();
  const n = Number(value) || 0;
  const stepTo = (next: number) => onChange(String(Math.max(min, next)));
  const btnCls = 'p-1.5 rounded-lg border border-border hover:bg-muted/10 text-muted-foreground';
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between gap-3">
        <span className="flex items-center gap-1.5">
          <label htmlFor={id} className={labelCls}>
            {label}
          </label>
          {action}
        </span>
        <span className="flex items-center gap-1">
          <button type="button" aria-label={`Decrease ${label}`} onClick={() => stepTo(n - step)} className={btnCls}>
            <Minus className="h-3 w-3" />
          </button>
          <input
            id={id}
            type="number"
            min={min}
            step={step}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            onBlur={(e) => onChange(atLeast(e.target.value, min))}
            className={`${inputCls} w-24 text-right`}
          />
          <button type="button" aria-label={`Increase ${label}`} onClick={() => stepTo(n + step)} className={btnCls}>
            <Plus className="h-3 w-3" />
          </button>
        </span>
      </div>
      {hint && <div className="text-[10px] text-muted-foreground text-right">{hint}</div>}
    </div>
  );
};
