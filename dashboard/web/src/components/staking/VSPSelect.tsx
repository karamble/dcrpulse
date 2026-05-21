// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ChevronDown, Plus, Server } from 'lucide-react';
import { ListVSPsResponse, VSPInfo, getVSPInfo, listVSPs } from '../../services/api';

interface Props {
  network: 'mainnet' | 'testnet';
  value: VSPInfo | null;
  onChange: (vsp: VSPInfo | null) => void;
}

const sanitizeHost = (s: string): string =>
  s
    .trim()
    .replace(/^https?:\/\//i, '')
    .replace(/\/+$/, '');

// Loose hostname test - enough to gate the "+ Use this host" row.
const HOSTNAME_RE = /^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?)+$/i;

export const VSPSelect = ({ network, value, onChange }: Props) => {
  const [reg, setReg] = useState<ListVSPsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState('');
  const [probing, setProbing] = useState(false);
  const [probeError, setProbeError] = useState<string | null>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    listVSPs()
      .then((r) => {
        if (!cancelled) setReg(r);
      })
      .catch((err) => {
        if (!cancelled) setLoadError(err?.message || 'Failed to load VSPs');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [open]);

  const sourceVSPs: VSPInfo[] = useMemo(() => {
    if (!reg) return [];
    if (reg.registryEnabled && reg.vsps.length > 0) {
      return reg.vsps.filter((v) => !v.network || v.network === network);
    }
    return reg.usedVSPs;
  }, [reg, network]);

  const sanitized = sanitizeHost(input);
  const filtered = useMemo(() => {
    const q = sanitized.toLowerCase();
    return sourceVSPs
      .filter((v) => !q || v.host.toLowerCase().includes(q))
      .sort((a, b) => (a.feePercentage || 0) - (b.feePercentage || 0));
  }, [sourceVSPs, sanitized]);

  const showAddRow =
    !!sanitized &&
    !sourceVSPs.some((v) => v.host.toLowerCase() === sanitized.toLowerCase()) &&
    HOSTNAME_RE.test(sanitized);

  const selectExisting = (vsp: VSPInfo) => {
    setInput('');
    setOpen(false);
    setProbeError(null);
    // used_vsps entries only carry host + pubkey, so fee% is 0/undefined.
    // Probe to fill in the live fee so the purchase form's fee preview is
    // accurate. Registry entries already have fee%, no extra round trip.
    if (vsp.feePercentage > 0) {
      onChange(vsp);
      return;
    }
    probeAndSelect(vsp.host);
  };

  const probeAndSelect = async (host: string) => {
    setProbing(true);
    setProbeError(null);
    try {
      const info = await getVSPInfo(host);
      onChange(info);
      setInput('');
      setOpen(false);
    } catch (err: any) {
      const body = err?.response?.data;
      setProbeError(typeof body === 'string' ? body : err?.message || 'VSP probe failed');
    } finally {
      setProbing(false);
    }
  };

  const registryDisabled = reg && !reg.registryEnabled;
  const registryFetchFailed = reg && reg.registryEnabled && !!reg.registryError;

  return (
    <div className="space-y-1" ref={wrapperRef}>
      <label className="text-sm font-medium text-muted-foreground flex items-center gap-2">
        <Server className="h-4 w-4" />
        Voting Service Provider (VSP)
      </label>

      <div className="relative">
        <div className="flex items-center gap-2 w-full px-3 py-2 rounded-lg bg-background border border-border/50 focus-within:border-primary/40 transition-colors">
          <input
            type="text"
            value={open || input ? input : value ? value.host : ''}
            placeholder={
              loading
                ? 'Loading VSPs...'
                : value
                  ? value.host
                  : 'Search or type a VSP host'
            }
            disabled={loading || probing}
            onFocus={() => setOpen(true)}
            onChange={(e) => {
              setInput(e.target.value);
              setOpen(true);
              setProbeError(null);
            }}
            className="flex-1 bg-transparent text-sm outline-none disabled:opacity-50"
          />
          {value && !open && (
            <span className="text-xs text-primary shrink-0">
              {value.feePercentage > 0 ? `${value.feePercentage.toFixed(2)}% fee` : 'used'}
            </span>
          )}
          <ChevronDown
            className="h-4 w-4 text-muted-foreground shrink-0 cursor-pointer"
            onClick={() => setOpen((v) => !v)}
          />
        </div>

        {open && reg && (
          <div className="absolute z-10 mt-1 w-full max-h-72 overflow-auto rounded-lg bg-card border border-border/50 shadow-xl">
            {(registryDisabled || registryFetchFailed) && (
              <div className="px-3 py-2 text-xs text-muted-foreground border-b border-border/30 bg-warning/5">
                {registryDisabled
                  ? 'VSP registry disabled in Settings. Showing VSPs you have used here, or type a host below.'
                  : `Registry unreachable: ${reg!.registryError}. Showing VSPs you have used here, or type a host below.`}
              </div>
            )}
            {filtered.length === 0 && !showAddRow && (
              <div className="px-3 py-2 text-xs text-muted-foreground">
                {sanitized
                  ? 'No VSP matches. Type a complete host to add it.'
                  : 'No VSPs available. Type a host to add one.'}
              </div>
            )}
            {filtered.map((vsp) => (
              <button
                key={vsp.host}
                type="button"
                onClick={() => selectExisting(vsp)}
                className="w-full px-3 py-2 text-left text-sm hover:bg-muted/20 transition-colors flex justify-between items-center gap-3"
              >
                <span className="font-mono text-xs truncate flex-1">{vsp.host}</span>
                {vsp.feePercentage > 0 && (
                  <span className="text-xs text-primary shrink-0">
                    {vsp.feePercentage.toFixed(2)}%
                  </span>
                )}
              </button>
            ))}
            {showAddRow && (
              <button
                type="button"
                onClick={() => probeAndSelect(sanitized)}
                disabled={probing}
                className="w-full px-3 py-2 text-left text-sm hover:bg-success/10 transition-colors flex items-center gap-2 border-t border-border/30 disabled:opacity-50 disabled:cursor-wait"
              >
                <Plus className="h-4 w-4 text-success shrink-0" />
                <span className="font-mono text-xs truncate text-success">
                  {probing ? `Probing ${sanitized}...` : `Use ${sanitized}`}
                </span>
              </button>
            )}
          </div>
        )}
      </div>

      {loadError && (
        <p className="text-xs text-destructive flex items-center gap-1">
          <AlertCircle className="h-3 w-3" />
          {loadError}
        </p>
      )}
      {probeError && (
        <p className="text-xs text-destructive flex items-center gap-1">
          <AlertCircle className="h-3 w-3" />
          {probeError}
        </p>
      )}
      {value && (
        <p className="text-xs text-muted-foreground">
          PubKey: <span className="font-mono">{value.pubkey ? `${value.pubkey.slice(0, 16)}...` : '(unknown)'}</span>
          {value.vspdVersion && <> &middot; vspd {value.vspdVersion}</>}
        </p>
      )}
    </div>
  );
};
