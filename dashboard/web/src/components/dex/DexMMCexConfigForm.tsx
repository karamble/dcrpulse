// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { AlertCircle } from 'lucide-react';
import { updateMMCexConfig } from '../../services/dcrdexApi';
import { CEX_DISPLAY, CexIcon, SUPPORTED_CEXES } from './CexIcon';

// DexMMCexConfigForm stores a centralized-exchange API key/secret for the arb
// bots. v1.0.6 supports Binance and BinanceUS. Credentials persist in bisonw's
// encrypted database (updatecexconfig); they are never stored by the dashboard.
export const DexMMCexConfigForm = ({ onSaved }: { onSaved: () => void }) => {
  const [name, setName] = useState<string>(SUPPORTED_CEXES[0]);
  const [apiKey, setApiKey] = useState('');
  const [apiSecret, setApiSecret] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const save = async () => {
    if (!apiKey.trim() || !apiSecret.trim()) {
      setErr('API key and secret are required.');
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      await updateMMCexConfig({ name, apiKey: apiKey.trim(), apiSecret: apiSecret.trim() });
      setApiKey('');
      setApiSecret('');
      onSaved();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to save CEX credentials');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {SUPPORTED_CEXES.map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => setName(c)}
            className={`flex items-center gap-2 px-3 py-1.5 rounded-lg border text-sm transition-colors ${
              name === c ? 'border-primary bg-muted/20' : 'border-border hover:bg-muted/10'
            }`}
          >
            <CexIcon name={c} className="h-4 w-4" />
            {CEX_DISPLAY[c] || c}
          </button>
        ))}
      </div>

      <input
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
        placeholder="API key"
        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary"
      />
      <input
        value={apiSecret}
        onChange={(e) => setApiSecret(e.target.value)}
        type="password"
        placeholder="API secret"
        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-sm font-mono focus:outline-none focus:border-primary"
      />

      {err && (
        <div className="p-2.5 rounded-lg bg-destructive/5 border border-destructive/30 text-xs text-destructive flex items-start gap-2">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}

      <button
        type="button"
        disabled={busy}
        onClick={save}
        className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? 'Saving...' : `Save ${CEX_DISPLAY[name] || name} keys`}
      </button>
    </div>
  );
};
