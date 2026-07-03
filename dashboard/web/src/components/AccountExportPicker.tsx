// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, FolderOpen, Loader2 } from 'lucide-react';
import { AccountExportEntry, parseAccountExport } from '../services/api';
import { KeyEnds } from './AddressGroups';

// AccountExportPicker loads a device account-export file (accounts.dcr from a
// Foundation Passport's SD card) and lets the user pick which accounts to
// import. Entries the server annotated as alreadyImported (same xpub AND same
// index) are shown checked-off and skipped silently; entries with a conflict
// (same key under a different index, or index taken by a different key) are
// blocked with the reason - that combination means the export and the wallet
// disagree and the user must look. Names are editable suggestions. Shared by
// the Add-xpub modal and the create-watch-only wizard.

export interface SelectedAccountEntry extends AccountExportEntry {
  editedName: string;
}

// Binary-safe file read (same pattern as the offline-signing uploads).
const fileToBase64 = (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const s = String(reader.result);
      const comma = s.indexOf(',');
      resolve(comma >= 0 ? s.slice(comma + 1) : s);
    };
    reader.onerror = () => reject(new Error('could not read file'));
    reader.readAsDataURL(file);
  });

interface AccountExportPickerProps {
  onSelectionChange: (selected: SelectedAccountEntry[]) => void;
  disabled?: boolean;
  // The create-watch-only wizard imports into a wallet that does not exist
  // yet, so entries must not be checked against the currently open wallet.
  newWallet?: boolean;
}

export const AccountExportPicker = ({ onSelectionChange, disabled, newWallet }: AccountExportPickerProps) => {
  const fileRef = useRef<HTMLInputElement>(null);
  const [entries, setEntries] = useState<AccountExportEntry[] | null>(null);
  const [checked, setChecked] = useState<Record<number, boolean>>({});
  const [names, setNames] = useState<Record<number, string>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const emit = (
    es: AccountExportEntry[] | null,
    chk: Record<number, boolean>,
    nms: Record<number, string>,
  ) => {
    if (!es) {
      onSelectionChange([]);
      return;
    }
    onSelectionChange(
      es
        .filter((e) => chk[e.account] && !e.conflict && !e.alreadyImported)
        .map((e) => ({
          ...e,
          editedName: (nms[e.account] ?? (e.name || `account-${e.account}`)).trim(),
        })),
    );
  };

  const onFile = async (file: File) => {
    setLoading(true);
    setError('');
    try {
      const parsed = await parseAccountExport(await fileToBase64(file), !!newWallet);
      if (parsed.length === 0) {
        throw new Error('The file contains no accounts');
      }
      const chk: Record<number, boolean> = {};
      const nms: Record<number, string> = {};
      for (const e of parsed) {
        chk[e.account] = !e.conflict && !e.alreadyImported;
        nms[e.account] = e.name || `account-${e.account}`;
      }
      setEntries(parsed);
      setChecked(chk);
      setNames(nms);
      emit(parsed, chk, nms);
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Could not read the file');
      setEntries(null);
      emit(null, {}, {});
    } finally {
      setLoading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const toggle = (account: number) => {
    const chk = { ...checked, [account]: !checked[account] };
    setChecked(chk);
    emit(entries, chk, names);
  };

  const rename = (account: number, value: string) => {
    const nms = { ...names, [account]: value };
    setNames(nms);
    emit(entries, checked, nms);
  };

  return (
    <div className="space-y-3">
      <input
        ref={fileRef}
        type="file"
        accept=".dcr,application/octet-stream"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) onFile(f);
        }}
      />
      <button
        type="button"
        disabled={disabled || loading}
        onClick={() => fileRef.current?.click()}
        className="px-4 py-2 rounded-lg border border-border/50 hover:border-primary/50 hover:bg-muted/10 transition-all inline-flex items-center gap-2 text-sm disabled:opacity-50"
      >
        {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <FolderOpen className="h-4 w-4" />}
        Import from file (accounts.dcr)
      </button>
      <p className="text-xs text-muted-foreground">
        On the device: Accounts, then "Write to SD card" (one account) or "Export all to SD card",
        then load the accounts.dcr file from the card here.
      </p>

      {error && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{error}</span>
        </div>
      )}

      {entries && (
        <div className="space-y-2">
          {entries.map((e) => (
            <div
              key={e.fp + e.account}
              className={`p-3 rounded-lg border ${
                e.conflict
                  ? 'border-destructive/40 bg-destructive/5'
                  : e.alreadyImported
                    ? 'border-border/30 bg-muted/5 opacity-70'
                    : 'border-border/50 bg-muted/5'
              }`}
            >
              <div className="flex items-center gap-3">
                {e.alreadyImported ? (
                  <CheckCircle2 className="h-4 w-4 text-success shrink-0" />
                ) : (
                  <input
                    type="checkbox"
                    checked={!!checked[e.account] && !e.conflict}
                    disabled={disabled || !!e.conflict}
                    onChange={() => toggle(e.account)}
                    className="h-4 w-4 accent-primary shrink-0"
                  />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    {checked[e.account] && !e.conflict && !e.alreadyImported ? (
                      <input
                        type="text"
                        maxLength={50}
                        value={names[e.account] ?? ''}
                        disabled={disabled}
                        onChange={(ev) => rename(e.account, ev.target.value)}
                        className="px-2 py-1 rounded bg-muted/10 border border-border/50 text-sm focus:border-primary/50 focus:outline-none"
                      />
                    ) : (
                      <span className="text-sm font-medium">{e.name || `account-${e.account}`}</span>
                    )}
                    <span className="text-xs text-muted-foreground font-mono">
                      m/44'/42'/{e.account}' · fp {e.fp}
                    </span>
                  </div>
                  <KeyEnds value={e.dpub} className="text-xs text-muted-foreground" />
                  {e.alreadyImported && (
                    <p className="text-xs text-muted-foreground mt-1">Already imported - will be skipped.</p>
                  )}
                  {e.conflict && (
                    <p className="text-xs text-destructive mt-1 break-words">{e.conflict}</p>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
