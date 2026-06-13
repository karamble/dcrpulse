// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Bug, CheckCircle2, ShieldCheck } from 'lucide-react';
import {
  ExternalRequestSettings,
  getMixerDebug,
  getSettings,
  saveSettings,
  setMixerDebug,
} from '../../services/api';

const defaultExternal: ExternalRequestSettings = {
  vspListing: true,
  politeia: true,
  brseeder: true,
};

const defaultBotUrl = 'https://brulse.decredcommunity.org';

interface ToggleProps {
  label: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (next: boolean) => void;
}

const Toggle = ({ label, description, checked, disabled, onChange }: ToggleProps) => (
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

export const PrivacySection = () => {
  const [external, setExternal] = useState<ExternalRequestSettings>(defaultExternal);
  const [botUrl, setBotUrl] = useState(defaultBotUrl);
  const [mixerDebug, setMixerDebugState] = useState(false);
  const [debugBusy, setDebugBusy] = useState(false);
  const [externalBusy, setExternalBusy] = useState(false);
  const [feedback, setFeedback] = useState<{ kind: 'info' | 'error'; text: string } | null>(null);

  useEffect(() => {
    getSettings()
      .then((s) => {
        if (s.global?.externalRequests) setExternal(s.global.externalRequests);
        if (s.global?.decredPulseBotUrl) setBotUrl(s.global.decredPulseBotUrl);
      })
      .catch(() => {});
    getMixerDebug()
      .then((r) => setMixerDebugState(r.enabled))
      .catch(() => {});
  }, []);

  const toggleDebug = async () => {
    if (debugBusy) return;
    setDebugBusy(true);
    setFeedback(null);
    try {
      const r = await setMixerDebug(!mixerDebug);
      setMixerDebugState(r.enabled);
    } catch (err: any) {
      setFeedback({ kind: 'error', text: err?.message || 'Failed to toggle debug logging' });
    } finally {
      setDebugBusy(false);
    }
  };

  const updateExternal = async (next: ExternalRequestSettings) => {
    if (externalBusy) return;
    setExternal(next);
    setExternalBusy(true);
    setFeedback(null);
    try {
      await saveSettings({ global: { externalRequests: next, decredPulseBotUrl: botUrl.trim() } });
      setFeedback({ kind: 'info', text: 'Preferences saved.' });
    } catch (err: any) {
      const body = err?.response?.data;
      setFeedback({
        kind: 'error',
        text: typeof body === 'string' ? body : err?.message || 'Failed to save preferences',
      });
    } finally {
      setExternalBusy(false);
    }
  };

  const saveBotUrl = async () => {
    if (externalBusy) return;
    const v = botUrl.trim();
    if (v !== '' && !/^https?:\/\//i.test(v)) {
      setFeedback({ kind: 'error', text: 'Bot URL must start with http:// or https://' });
      return;
    }
    setExternalBusy(true);
    setFeedback(null);
    try {
      await saveSettings({ global: { externalRequests: external, decredPulseBotUrl: v } });
      setBotUrl(v === '' ? defaultBotUrl : v);
      setFeedback({ kind: 'info', text: 'Preferences saved.' });
    } catch (err: any) {
      const body = err?.response?.data;
      setFeedback({
        kind: 'error',
        text: typeof body === 'string' ? body : err?.message || 'Failed to save preferences',
      });
    } finally {
      setExternalBusy(false);
    }
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
      <div className="flex items-center gap-2">
        <ShieldCheck className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">Privacy &amp; Security</h3>
      </div>

      {feedback && (
        <div
          className={`flex items-center gap-2 text-sm ${feedback.kind === 'error' ? 'text-destructive' : 'text-success'}`}
        >
          {feedback.kind === 'error' ? (
            <AlertCircle className="h-4 w-4" />
          ) : (
            <CheckCircle2 className="h-4 w-4" />
          )}
          {feedback.text}
        </div>
      )}

      <div className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <Bug className="h-4 w-4 text-warning" />
            <span className="font-medium">Mixer debug logging</span>
          </div>
          <p className="text-sm text-muted-foreground">
            Enables MIXC + TKBY debug logs in the dcrwallet container. Useful for diagnosing mixer
            issues; quite chatty.
          </p>
        </div>
        <button
          onClick={toggleDebug}
          disabled={debugBusy}
          className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${
            mixerDebug
              ? 'bg-warning/20 text-warning hover:bg-warning/30'
              : 'bg-muted/20 text-muted-foreground hover:bg-muted/30'
          } disabled:opacity-50 disabled:cursor-wait`}
        >
          {mixerDebug ? 'On' : 'Off'}
        </button>
      </div>

      <div className="space-y-2">
        <h4 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
          External requests
        </h4>
        <Toggle
          label="VSP registry"
          description="Fetch the VSP list from api.decred.org for the Staking page picker."
          checked={external.vspListing}
          disabled={externalBusy}
          onChange={(v) => updateExternal({ ...external, vspListing: v })}
        />
        <Toggle
          label="Politeia"
          description="Fetch off-chain proposals from proposals.decred.org for the Governance > Proposals tab."
          checked={external.politeia}
          disabled={externalBusy}
          onChange={(v) => updateExternal({ ...external, politeia: v })}
        />
        <Toggle
          label="Bison Relay LN seeder"
          description="Fetch Lightning peer suggestions from bisonrelay.org for the Channels tab's open-channel form. When disabled, the form shows no presets and you must type a peer URI manually."
          checked={external.brseeder}
          disabled={externalBusy}
          onChange={(v) => updateExternal({ ...external, brseeder: v })}
        />
        <p className="text-xs text-muted-foreground">
          These preferences are persisted now; enforcement at each call site will be wired in a
          follow-up.
        </p>
        <div className="flex items-start justify-between gap-4 p-3 rounded-lg bg-muted/10 border border-border/50">
          <div className="min-w-0">
            <span className="font-medium block">Decred Pulse bot URL</span>
            <span className="text-sm text-muted-foreground block">
              Endpoint for the &quot;Join Decred chat networks&quot; invite bot. Leave as the
              default unless you run your own brulse instance.
            </span>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <input
              type="url"
              value={botUrl}
              onChange={(e) => setBotUrl(e.target.value)}
              placeholder={defaultBotUrl}
              disabled={externalBusy}
              className="w-56 px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm disabled:opacity-50"
            />
            <button
              type="button"
              onClick={saveBotUrl}
              disabled={externalBusy}
              className="shrink-0 px-3 py-1.5 rounded-lg text-xs font-medium bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 disabled:cursor-wait"
            >
              Save
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
