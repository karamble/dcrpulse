// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { HslChannels } from '../../../services/themes/types';
import { hexToHslChannels, hslChannelsToHex } from '../../../services/themes/themeMath';

interface ColorFieldProps {
  label: string;
  value: HslChannels;
  onChange: (channels: HslChannels) => void;
  // Optional warning (e.g. low WCAG contrast) shown under the label.
  warn?: string | null;
}

// One editable color row: native picker + hex text input, both bound to the
// underlying HSL-channel token. No third-party color picker.
export const ColorField = ({ label, value, onChange, warn }: ColorFieldProps) => {
  const hex = hslChannelsToHex(value);
  const [text, setText] = useState(hex);

  // Resync the hex text when the value changes from outside (reset, preset).
  useEffect(() => setText(hslChannelsToHex(value)), [value]);

  const commitText = (v: string) => {
    setText(v);
    if (/^#?[0-9a-fA-F]{6}$/.test(v.trim())) onChange(hexToHslChannels(v));
  };

  return (
    <div className="flex items-center gap-3 py-1">
      <input
        type="color"
        value={hex}
        onChange={(e) => onChange(hexToHslChannels(e.target.value))}
        className="h-8 w-9 shrink-0 cursor-pointer rounded border border-border/50 bg-transparent"
        aria-label={label}
      />
      <div className="min-w-0 flex-1">
        <div className="text-sm">{label}</div>
        {warn && <div className="text-[11px] text-warning">{warn}</div>}
      </div>
      <input
        type="text"
        value={text}
        onChange={(e) => commitText(e.target.value)}
        spellCheck={false}
        className="w-24 rounded border border-border bg-background px-2 py-1 font-mono text-xs focus:border-primary focus:outline-none"
      />
    </div>
  );
};
