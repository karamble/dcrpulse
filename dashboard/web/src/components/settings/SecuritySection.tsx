// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, type FormEvent } from 'react';
import { ShieldCheck, ShieldOff, Loader2 } from 'lucide-react';
import {
  setupAppPassword,
  changeAppPassword,
  disableAppPassword,
} from '../../services/auth';
import { useAuth } from '../auth/AuthGate';
import { useDemo } from '../DemoProvider';

const inputClass =
  'w-full px-4 py-3 rounded-lg bg-background border border-border/60 focus:border-primary outline-none';

export const SecuritySection = () => {
  // Use the shared AuthGate context so enabling/disabling here immediately
  // updates the gate and the Header (logout button) without a page reload.
  const { status, refresh } = useAuth();
  const { demoMode, showDemoDisabledModal } = useDemo();
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');
  const [err, setErr] = useState('');
  const [pw, setPw] = useState('');
  const [pw2, setPw2] = useState('');
  const [cur, setCur] = useState('');
  const [next, setNext] = useState('');
  const [disPw, setDisPw] = useState('');

  const enable = async (e: FormEvent) => {
    e.preventDefault();
    if (demoMode) {
      showDemoDisabledModal();
      return;
    }
    setErr('');
    setMsg('');
    if (!pw) {
      setErr('Enter a password.');
      return;
    }
    if (pw !== pw2) {
      setErr('Passwords do not match.');
      return;
    }
    setBusy(true);
    try {
      await setupAppPassword(pw);
      setPw('');
      setPw2('');
      setMsg('App password enabled.');
      await refresh();
    } catch (e: any) {
      setErr(e?.response?.data || e?.message || 'Failed to enable.');
    } finally {
      setBusy(false);
    }
  };

  const change = async (e: FormEvent) => {
    e.preventDefault();
    if (demoMode) {
      showDemoDisabledModal();
      return;
    }
    setErr('');
    setMsg('');
    if (!cur || !next) {
      setErr('Fill in both fields.');
      return;
    }
    setBusy(true);
    try {
      await changeAppPassword(cur, next);
      setCur('');
      setNext('');
      setMsg('Password changed.');
      await refresh();
    } catch (e: any) {
      setErr(
        e?.response?.status === 400
          ? 'Current password is incorrect.'
          : e?.message || 'Failed to change password.',
      );
    } finally {
      setBusy(false);
    }
  };

  const disable = async (e: FormEvent) => {
    e.preventDefault();
    if (demoMode) {
      showDemoDisabledModal();
      return;
    }
    setErr('');
    setMsg('');
    if (!disPw) {
      setErr('Enter your current password.');
      return;
    }
    setBusy(true);
    try {
      await disableAppPassword(disPw);
      setDisPw('');
      setMsg('App password disabled.');
      await refresh();
    } catch (e: any) {
      setErr(
        e?.response?.status === 400
          ? 'Current password is incorrect.'
          : e?.message || 'Failed to disable.',
      );
    } finally {
      setBusy(false);
    }
  };

  const enabled = !!status?.enabled;

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50">
        <div className="flex items-center gap-3">
          {enabled ? (
            <ShieldCheck className="h-6 w-6 text-success" />
          ) : (
            <ShieldOff className="h-6 w-6 text-muted-foreground" />
          )}
          <div>
            <h3 className="text-lg font-semibold">Dashboard app password</h3>
            <p className="text-sm text-muted-foreground">
              {enabled
                ? 'A login is required to use this dashboard.'
                : 'Optional. When enabled, a password is required to use this dashboard - it covers the whole API and all live connections.'}
            </p>
          </div>
        </div>
      </div>

      {msg && <p className="text-sm text-success">{msg}</p>}
      {err && <p className="text-sm text-red-500">{String(err)}</p>}

      {!enabled ? (
        <form
          onSubmit={enable}
          className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-3 max-w-md"
        >
          <h4 className="font-semibold">Enable app password</h4>
          <input
            type="password"
            autoComplete="new-password"
            value={pw}
            onChange={(e) => setPw(e.target.value)}
            placeholder="New password"
            className={inputClass}
          />
          <input
            type="password"
            autoComplete="new-password"
            value={pw2}
            onChange={(e) => setPw2(e.target.value)}
            placeholder="Confirm password"
            className={inputClass}
          />
          {pw2.length > 0 && pw !== pw2 && (
            <p className="text-sm text-red-500">Passwords do not match.</p>
          )}
          <button
            type="submit"
            disabled={busy || !pw || pw !== pw2}
            className="px-4 py-3 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50 flex items-center gap-2"
          >
            {busy && <Loader2 className="h-4 w-4 animate-spin" />}
            Enable
          </button>
        </form>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <form
            onSubmit={change}
            className="p-6 rounded-xl bg-gradient-card border border-border/50 space-y-3"
          >
            <h4 className="font-semibold">Change password</h4>
            <input
              type="password"
              autoComplete="current-password"
              value={cur}
              onChange={(e) => setCur(e.target.value)}
              placeholder="Current password"
              className={inputClass}
            />
            <input
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
              placeholder="New password"
              className={inputClass}
            />
            <button
              type="submit"
              disabled={busy}
              className="px-4 py-3 rounded-lg bg-primary text-primary-foreground font-semibold disabled:opacity-50 flex items-center gap-2"
            >
              {busy && <Loader2 className="h-4 w-4 animate-spin" />}
              Change
            </button>
          </form>
          <form
            onSubmit={disable}
            className="p-6 rounded-xl bg-gradient-card border border-red-500/30 space-y-3"
          >
            <h4 className="font-semibold text-red-500">Disable app password</h4>
            <p className="text-sm text-muted-foreground">
              Turns off the login requirement. Anyone who can reach the dashboard
              will be able to use it.
            </p>
            <input
              type="password"
              autoComplete="current-password"
              value={disPw}
              onChange={(e) => setDisPw(e.target.value)}
              placeholder="Current password"
              className={inputClass}
            />
            <button
              type="submit"
              disabled={busy}
              className="px-4 py-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-500 font-semibold disabled:opacity-50 flex items-center gap-2"
            >
              {busy && <Loader2 className="h-4 w-4 animate-spin" />}
              Disable
            </button>
          </form>
        </div>
      )}
    </div>
  );
};
