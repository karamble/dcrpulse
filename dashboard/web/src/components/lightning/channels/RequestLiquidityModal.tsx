// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, AlertTriangle, ChevronDown, Loader2, X } from 'lucide-react';
import {
  LightningBalance,
  LiquidityEstimate,
  RequestLiquidityResult,
  estimateLiquidityChannel,
  getLightningBalance,
  getLightningChannels,
  getLiquidityDefaults,
  requestLiquidityChannel,
} from '../../../services/lightningApi';
import { fmtDcr } from '../StatCard';

interface Props {
  onClose: () => void;
  onSuccess?: (channelPoint: string) => void;
}

const dcrToAtoms = (dcr: string): number => {
  const n = Number(dcr);
  if (!isFinite(n) || n <= 0) return 0;
  return Math.round(n * 1e8);
};

const humanizeSeconds = (s: number): string => {
  if (s >= 86400) {
    const d = Math.round(s / 86400);
    return `${d} day${d === 1 ? '' : 's'}`;
  }
  if (s >= 3600) {
    const h = Math.round(s / 3600);
    return `${h} hour${h === 1 ? '' : 's'}`;
  }
  const m = Math.max(1, Math.round(s / 60));
  return `${m} minute${m === 1 ? '' : 's'}`;
};

type Step = 'form' | 'confirm' | 'progress';

// Request an inbound channel from a dcrlnlpd liquidity provider, mirroring
// bruig's NeedsInChannelScreen + LNConfirmRecvChanPaymentScreen flow.
export const RequestLiquidityModal = ({ onClose, onSuccess }: Props) => {
  const [step, setStep] = useState<Step>('form');
  const [amountDcr, setAmountDcr] = useState('');
  const [server, setServer] = useState('');
  const [certPem, setCertPem] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [balance, setBalance] = useState<LightningBalance | null>(null);
  const [openChannels, setOpenChannels] = useState<number | null>(null);
  const [estimate, setEstimate] = useState<LiquidityEstimate | null>(null);
  const [result, setResult] = useState<RequestLiquidityResult | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getLiquidityDefaults()
      .then((d) => {
        setServer((prev) => prev || d.server);
        setCertPem((prev) => prev || d.certPem);
      })
      .catch(() => {
        /* no defaults (daemon down or unknown network); fields stay manual */
      });
    getLightningBalance()
      .then(setBalance)
      .catch(() => {});
    getLightningChannels()
      .then((r) => setOpenChannels(r.channels.filter((c) => c.status === 'open').length))
      .catch(() => {});
  }, []);

  const chanSizeAtoms = dcrToAtoms(amountDcr);

  const runEstimate = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const est = await estimateLiquidityChannel(
        chanSizeAtoms,
        server.trim() || undefined,
        certPem.trim() || undefined,
      );
      setEstimate(est);
      setStep('confirm');
    } catch (e: any) {
      const body = e?.response?.data;
      setError(typeof body === 'string' ? body : e?.message || 'Estimate failed');
    } finally {
      setSubmitting(false);
    }
  };

  const runRequest = async () => {
    if (!estimate) return;
    setStep('progress');
    setSubmitting(true);
    setError(null);
    try {
      const res = await requestLiquidityChannel(
        estimate.chanSizeAtoms,
        estimate.estimatedFeeAtoms,
        server.trim() || undefined,
        certPem.trim() || undefined,
      );
      setResult(res);
      onSuccess?.(res.channelPoint);
    } catch (e: any) {
      const body = e?.response?.data;
      setError(typeof body === 'string' ? body : e?.message || 'Request failed');
    } finally {
      setSubmitting(false);
    }
  };

  const stat = (label: string, value: string) => (
    <div className="rounded-lg bg-muted/10 border border-border/50 p-2">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="text-xs font-medium tabular-nums">{value}</div>
    </div>
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-lg mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold">
            {step === 'confirm'
              ? 'Confirm LN Payment to Open Receive Channel'
              : 'Request Inbound Channel'}
          </h3>
          <button
            onClick={onClose}
            disabled={submitting}
            className="text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {step === 'form' && (
          <div className="p-6 space-y-4">
            <p className="text-sm text-muted-foreground">
              The wallet needs channels with inbound capacity to receive
              payments from other users. A liquidity provider opens a channel
              back to this node for a small fee.
            </p>
            {balance && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                {stat('Outbound', fmtDcr(balance.channelLocal))}
                {stat('Inbound', fmtDcr(balance.channelRemote))}
                {stat('Pending', fmtDcr(balance.channelPending))}
                {stat('Channels', openChannels === null ? '...' : String(openChannels))}
              </div>
            )}
            {balance && balance.channelLocal === 0 && (
              <div className="rounded-lg bg-warning/10 border border-warning/30 p-3 text-xs text-foreground/80 flex items-start gap-2">
                <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
                <span>You need outbound capacity to pay the provider's fee.</span>
              </div>
            )}
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Add Inbound Capacity (DCR)</label>
              <input
                type="number"
                step="0.00000001"
                min="0.00001"
                value={amountDcr}
                onChange={(e) => setAmountDcr(e.target.value)}
                placeholder="0.01"
                disabled={submitting}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
              />
            </div>
            <button
              type="button"
              onClick={() => setAdvancedOpen((v) => !v)}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              <ChevronDown
                className={`h-3.5 w-3.5 transition-transform ${advancedOpen ? 'rotate-180' : ''}`}
              />
              Advanced
            </button>
            {advancedOpen && (
              <div className="space-y-3">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">LP Server Address</label>
                  <input
                    type="text"
                    value={server}
                    onChange={(e) => setServer(e.target.value)}
                    placeholder="https://lpd-server:port"
                    disabled={submitting}
                    className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm font-mono focus:outline-none focus:border-primary disabled:opacity-50"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">LP Server Cert (PEM)</label>
                  <textarea
                    rows={6}
                    value={certPem}
                    onChange={(e) => setCertPem(e.target.value)}
                    disabled={submitting}
                    className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-xs font-mono focus:outline-none focus:border-primary disabled:opacity-50 resize-y"
                  />
                </div>
              </div>
            )}
            {error && (
              <div className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{error}</span>
              </div>
            )}
            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={onClose}
                disabled={submitting}
                className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                onClick={runEstimate}
                disabled={submitting || chanSizeAtoms < 1000}
                className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
              >
                {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
                {submitting ? 'Fetching policy...' : 'Continue'}
              </button>
            </div>
          </div>
        )}

        {step === 'confirm' && estimate && (
          <div className="p-6 space-y-4">
            <div className="space-y-2 text-sm">
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Requested channel size</span>
                <span className="font-medium tabular-nums">{fmtDcr(estimate.chanSizeAtoms)}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Estimated fee</span>
                <span className="font-medium tabular-nums">{fmtDcr(estimate.estimatedFeeAtoms)}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Minimum channel lifetime</span>
                <span className="font-medium">{humanizeSeconds(estimate.minChanLifetimeSeconds)}</span>
              </div>
              <div className="flex justify-between gap-4">
                <span className="text-muted-foreground">Max channels per node</span>
                <span className="font-medium tabular-nums">{estimate.maxNbChannels}</span>
              </div>
            </div>
            <div className="space-y-1 text-xs text-muted-foreground">
              <div>Server node</div>
              <div className="font-mono break-all text-foreground/80">{estimate.node}</div>
              {estimate.addresses.length > 0 && (
                <div className="font-mono break-all">{estimate.addresses.join(', ')}</div>
              )}
            </div>
            <div className="rounded-lg bg-warning/10 border border-warning/30 p-3 text-xs text-foreground/80 flex items-start gap-2">
              <AlertTriangle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
              <span>
                The provider may close the channel after the minimum lifetime
                if not enough payments flow through it. The channel becomes
                active after up to 6 confirmations.
              </span>
            </div>
            {error && (
              <div className="flex items-start gap-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span className="break-words">{error}</span>
              </div>
            )}
            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => {
                  setError(null);
                  setStep('form');
                }}
                disabled={submitting}
                className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm disabled:opacity-50"
              >
                Back
              </button>
              <button
                onClick={runRequest}
                disabled={submitting}
                className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm transition-all disabled:opacity-50"
              >
                Pay
              </button>
            </div>
          </div>
        )}

        {step === 'progress' && (
          <div className="p-6 space-y-4">
            {submitting ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>Paying invoice and waiting for the provider's channel...</span>
              </div>
            ) : result ? (
              <div className="space-y-3">
                <p className="text-sm font-medium text-success">Inbound channel requested</p>
                <div className="text-xs text-muted-foreground font-mono break-all">
                  {result.channelPoint}
                </div>
                <p className="text-xs text-muted-foreground">
                  Capacity {fmtDcr(result.capacityAtoms)}. The channel becomes
                  active after confirmations.
                </p>
              </div>
            ) : (
              error && (
                <div className="flex items-start gap-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                  <span className="break-words">{error}</span>
                </div>
              )
            )}
            <div className="flex justify-end gap-2 pt-2">
              {!submitting && !result && (
                <button
                  onClick={() => {
                    setError(null);
                    setStep('confirm');
                  }}
                  className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm"
                >
                  Back
                </button>
              )}
              {!submitting && (
                <button
                  onClick={onClose}
                  className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm"
                >
                  Done
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
