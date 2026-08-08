// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

export interface ToggleProps {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}

// Toggle is the labelled on/off row the settings sections share.
export const Toggle = ({ label, description, checked, disabled, onChange }: ToggleProps) => (
  <div className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
    <div>
      <span className="font-medium block">{label}</span>
      <span className="text-sm text-muted-foreground block">{description}</span>
    </div>
    <button
      type="button"
      onClick={() => onChange(!checked)}
      disabled={disabled}
      className={`shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
        checked
          ? 'bg-success/20 text-success hover:bg-success/30'
          : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
      } disabled:opacity-50 disabled:cursor-wait`}
    >
      {checked ? 'On' : 'Off'}
    </button>
  </div>
);
