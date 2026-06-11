// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import type { DexConfigOption } from '../../services/dcrdexApi';

interface Props {
  opts: DexConfigOption[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
}

const isTrue = (v: string) => v === '1' || v === 'true';

// DexWalletConfigForm renders a wallet type's config options as a form, mirroring
// upstream's WalletConfigForm: checkbox for booleans, password for no-echo
// fields, date for date fields, text otherwise.
export const DexWalletConfigForm = ({ opts, values, onChange }: Props) => {
  if (opts.length === 0) {
    return <p className="text-xs text-muted-foreground">No configuration needed for this wallet type.</p>;
  }
  return (
    <div className="space-y-3">
      {opts.map((o) => {
        const val = values[o.key] ?? o.default ?? '';
        if (o.isBoolean) {
          return (
            <label key={o.key} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={isTrue(val)}
                onChange={(e) => onChange(o.key, e.target.checked ? '1' : '0')}
              />
              <span>
                {o.displayName}
                {o.description && <span className="text-muted-foreground"> - {o.description}</span>}
              </span>
            </label>
          );
        }
        const type = o.noEcho ? 'password' : o.isDate ? 'date' : 'text';
        return (
          <div key={o.key}>
            <label className="block text-xs text-muted-foreground mb-1">
              {o.displayName}
              {o.required && <span className="text-destructive"> *</span>}
            </label>
            <input
              type={type}
              value={val}
              placeholder={o.default}
              onChange={(e) => onChange(o.key, e.target.value)}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary"
            />
            {o.description && <p className="text-[11px] text-muted-foreground mt-1">{o.description}</p>}
          </div>
        );
      })}
    </div>
  );
};
