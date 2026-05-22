// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  AlertCircle,
  ArrowRight,
  Check,
  Circle,
  Copy,
  HelpCircle,
  Loader2,
  XCircle,
} from 'lucide-react';
import {
  BisonrelayStage,
  BisonrelayStatus,
  getBisonrelayStatus,
  setupBisonrelay,
} from '../../services/bisonrelayApi';
import { BisonrelayDisclaimer } from './BisonrelayDisclaimer';

const DISCLAIMER_ACCEPTED_KEY = 'dcrpulse.br.disclaimer-accepted';

interface Props {
  onReady: () => void;
}

type StepStatus = 'pending' | 'in-progress' | 'done' | 'blocked';

interface Step {
  id: 'ln-unlock' | 'ln-channel' | 'ln-graph' | 'br-identity' | 'br-server';
  label: string;
  description: string;
  status: StepStatus;
  detail?: string;
}

export const BisonrelaySetupWizard = ({ onReady }: Props) => {
  const [disclaimerAccepted, setDisclaimerAccepted] = useState<boolean>(() => {
    try {
      return localStorage.getItem(DISCLAIMER_ACCEPTED_KEY) === 'true';
    } catch {
      return false;
    }
  });
  const [status, setStatus] = useState<BisonrelayStatus | null>(null);
  const [statusErr, setStatusErr] = useState<string | null>(null);
  const [nick, setNick] = useState('');
  const [fullName, setFullName] = useState('');
  const [submitErr, setSubmitErr] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!disclaimerAccepted) return;
    let cancelled = false;
    const poll = async () => {
      while (!cancelled) {
        try {
          const s = await getBisonrelayStatus();
          if (cancelled) return;
          setStatus(s);
          setStatusErr(null);
          if (s.stage === 'ready') {
            setTimeout(() => onReady(), 400);
            return;
          }
        } catch (err: any) {
          if (!cancelled) {
            setStatus(null);
            setStatusErr(err?.message || 'Could not reach brclientd');
          }
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
    };
    poll();
    return () => {
      cancelled = true;
    };
  }, [disclaimerAccepted, onReady]);

  const acceptDisclaimer = () => {
    try {
      localStorage.setItem(DISCLAIMER_ACCEPTED_KEY, 'true');
    } catch {
      /* ignore quota / private-mode errors */
    }
    setDisclaimerAccepted(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!nick.trim() || submitting) return;
    setSubmitting(true);
    setSubmitErr(null);
    try {
      await setupBisonrelay(nick.trim(), fullName.trim());
    } catch (err: any) {
      const body = err?.response?.data;
      setSubmitErr(typeof body === 'string' ? body : err?.message || 'Setup failed');
    } finally {
      setSubmitting(false);
    }
  };

  const steps = useMemo<Step[]>(() => deriveSteps(status), [status]);

  if (!disclaimerAccepted) {
    return <BisonrelayDisclaimer onAcknowledge={acceptDisclaimer} />;
  }

  const currentStep = steps.find((s) => s.status === 'in-progress');
  const allDone = steps.every((s) => s.status === 'done');

  return (
    <div className="space-y-4 max-w-2xl">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-5">
        <div>
          <h2 className="text-lg font-semibold">Bison Relay setup</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Bison Relay sends encrypted messages paid for over Lightning.
            A few Lightning steps need to be in place first; the wizard
            walks through each one and lands you on the Bison Relay
            overview when they're all set.
          </p>
        </div>

        {statusErr && (
          <div className="rounded-lg bg-warning/10 border border-warning/30 p-3 flex items-start gap-2">
            <AlertCircle className="h-4 w-4 text-warning shrink-0 mt-0.5" />
            <div className="text-xs text-foreground/80">
              <p className="font-semibold text-warning">brclientd is not reachable yet</p>
              <p className="mt-1">
                It may be starting up after a stack restart, or the container
                is not running. The wizard keeps polling and will update as
                soon as it is back.
              </p>
              <pre className="mt-2 text-[10px] opacity-75 whitespace-pre-wrap break-words">
                {statusErr}
              </pre>
            </div>
          </div>
        )}

        <ol className="space-y-3">
          {steps.map((s) => (
            <ChecklistRow key={s.id} step={s} />
          ))}
        </ol>

        {currentStep?.id === 'br-identity' && (
          <IdentityForm
            nick={nick}
            fullName={fullName}
            submitting={submitting}
            submitErr={submitErr}
            onNickChange={setNick}
            onNameChange={setFullName}
            onSubmit={handleSubmit}
          />
        )}

        {currentStep?.id === 'ln-unlock' && (
          <Link
            to="/wallet/lightning"
            className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm"
          >
            Unlock Lightning <ArrowRight className="h-4 w-4" />
          </Link>
        )}

        {currentStep?.id === 'ln-channel' && status?.recommendedPeer && (
          <ChannelGateActions peer={status.recommendedPeer} />
        )}

        {currentStep?.id === 'ln-graph' && (
          <p className="text-xs text-muted-foreground">
            Nothing to do here. dcrlnd is waiting on the channel_update
            gossip message from the BR hub. This typically clears within
            a few minutes of the channel becoming active.
          </p>
        )}

        {allDone && status?.stage === 'ready' && (
          <p className="text-sm text-success">All checks pass. Loading overview…</p>
        )}
      </div>
    </div>
  );
};

const ChecklistRow = ({ step }: { step: Step }) => {
  const Icon = step.status === 'done' ? Check
    : step.status === 'in-progress' ? Loader2
    : step.status === 'blocked' ? XCircle
    : Circle;
  const iconClass = step.status === 'done' ? 'text-success'
    : step.status === 'in-progress' ? 'text-primary animate-spin'
    : step.status === 'blocked' ? 'text-destructive'
    : 'text-muted-foreground';
  const titleClass = step.status === 'pending' ? 'text-muted-foreground'
    : step.status === 'done' ? 'text-foreground'
    : 'text-foreground';

  return (
    <li className="flex items-start gap-3">
      <Icon className={`h-5 w-5 mt-0.5 shrink-0 ${iconClass}`} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className={`text-sm font-medium ${titleClass}`}>{step.label}</span>
          <InfoTooltip text={step.description} />
        </div>
        {step.detail && (
          <pre className="mt-1 text-[11px] text-muted-foreground whitespace-pre-wrap break-words">
            {step.detail}
          </pre>
        )}
      </div>
    </li>
  );
};

const InfoTooltip = ({ text }: { text: string }) => (
  <span className="relative group inline-flex">
    <HelpCircle className="h-3.5 w-3.5 text-muted-foreground/60 hover:text-muted-foreground cursor-help" />
    <span className="pointer-events-none absolute left-5 top-0 w-72 p-2 rounded-md bg-popover border border-border/50 shadow-lg text-xs text-foreground/90 opacity-0 group-hover:opacity-100 transition-opacity z-10">
      {text}
    </span>
  </span>
);

const ChannelGateActions = ({ peer }: { peer: string }) => {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(peer);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* ignore */
    }
  };
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        Open a channel to this peer URI on the Lightning page. 0.5 DCR or
        more is recommended (1000 milliatoms is BR's minimum, anything
        larger gives routing headroom).
      </p>
      <div className="rounded-md bg-muted/20 border border-border/30 p-2 flex items-start gap-2">
        <code className="font-mono text-xs break-all flex-1">{peer}</code>
        <button
          onClick={onCopy}
          className="p-1 rounded hover:bg-muted/30 transition-colors"
          title="Copy peer URI"
        >
          <Copy className="h-4 w-4 text-muted-foreground" />
        </button>
      </div>
      {copied && <p className="text-xs text-success">Copied!</p>}
      <Link
        to="/wallet/lightning/channels"
        className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold text-sm"
      >
        Open channel on Lightning <ArrowRight className="h-4 w-4" />
      </Link>
    </div>
  );
};

interface IdentityFormProps {
  nick: string;
  fullName: string;
  submitting: boolean;
  submitErr: string | null;
  onNickChange: (v: string) => void;
  onNameChange: (v: string) => void;
  onSubmit: (e: React.FormEvent) => void;
}

const IdentityForm = ({
  nick,
  fullName,
  submitting,
  submitErr,
  onNickChange,
  onNameChange,
  onSubmit,
}: IdentityFormProps) => (
  <form onSubmit={onSubmit} className="space-y-3 pt-2 border-t border-border/50">
    <p className="text-sm text-muted-foreground">
      Choose a nickname; it identifies you to other BR users. Neither value
      can be changed once the identity is created.
    </p>
    <div>
      <label className="block text-xs text-muted-foreground mb-1" htmlFor="br-nick">
        Nickname (required)
      </label>
      <input
        id="br-nick"
        type="text"
        autoFocus
        value={nick}
        onChange={(e) => onNickChange(e.target.value)}
        disabled={submitting}
        maxLength={32}
        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
      />
    </div>
    <div>
      <label className="block text-xs text-muted-foreground mb-1" htmlFor="br-name">
        Display name (optional)
      </label>
      <input
        id="br-name"
        type="text"
        value={fullName}
        onChange={(e) => onNameChange(e.target.value)}
        disabled={submitting}
        maxLength={64}
        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary disabled:opacity-50"
      />
    </div>
    {submitErr && (
      <div className="flex items-start gap-2 text-sm text-destructive">
        <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
        <span>{submitErr}</span>
      </div>
    )}
    <div className="flex justify-end pt-1">
      <button
        type="submit"
        disabled={!nick.trim() || submitting}
        className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {submitting ? 'Creating…' : 'Create identity'}
      </button>
    </div>
  </form>
);

function deriveSteps(status: BisonrelayStatus | null): Step[] {
  const stage = status?.stage;
  const walletErr = status?.walletCheckErr ?? '';
  const peer = status?.recommendedPeer;
  const nick = status?.nick;

  // We treat "no status yet" as everything pending. The first row is
  // pinned to in-progress so the user sees the spinner.
  const noStatus = !status;

  const isAfter = (cur: BisonrelayStage | undefined, target: BisonrelayStage) => {
    if (!cur) return false;
    const order: BisonrelayStage[] = [
      'waiting-for-dcrlnd',
      'waiting-for-channel',
      'needs-identity',
      'starting',
      'wallet-checking',
      'connecting',
      'disconnected',
      'ready',
    ];
    return order.indexOf(cur) > order.indexOf(target);
  };

  const isGraphSyncing =
    stage === 'wallet-checking' ||
    stage === 'disconnected' ||
    stage === 'connecting' ||
    (stage === 'starting' && walletErr !== '');

  return [
    {
      id: 'ln-unlock',
      label: 'Lightning wallet unlocked',
      description:
        'Bison Relay uses your Lightning wallet to pay tiny fees for sending messages. The wallet has to be unlocked first.',
      status: noStatus
        ? 'in-progress'
        : stage === 'waiting-for-dcrlnd'
        ? 'in-progress'
        : 'done',
      detail: stage === 'waiting-for-dcrlnd' ? walletErr : undefined,
    },
    {
      id: 'ln-channel',
      label: 'Lightning channel to the Bison Relay hub',
      description:
        'Messages are paid for through a Lightning channel. You need at least one open channel with the recommended hub before BR can start.',
      status: noStatus || stage === 'waiting-for-dcrlnd'
        ? 'pending'
        : stage === 'waiting-for-channel'
        ? 'in-progress'
        : 'done',
      detail:
        stage === 'waiting-for-channel'
          ? peer
            ? `Recommended peer: ${peer}`
            : walletErr
          : undefined,
    },
    {
      id: 'ln-graph',
      label: 'Lightning network ready',
      description:
        'After a new channel opens, Lightning needs a few minutes to learn about it before payments can be routed. This step waits for that to happen.',
      status: noStatus || !isAfter(stage, 'waiting-for-channel')
        ? 'pending'
        : isGraphSyncing
        ? 'in-progress'
        : 'done',
      detail: isGraphSyncing && walletErr ? walletErr : undefined,
    },
    {
      id: 'br-identity',
      label: 'Bison Relay nickname',
      description:
        'Pick a nickname so other Bison Relay users can find you. Your identity is stored locally; you cannot rename it later.',
      status: noStatus
        ? 'pending'
        : nick
        ? 'done'
        : stage === 'needs-identity'
        ? 'in-progress'
        : 'pending',
    },
    {
      id: 'br-server',
      label: 'Connected to Bison Relay',
      description:
        'Once the Lightning steps and your nickname are set, dcrpulse connects to the Bison Relay network automatically.',
      status: stage === 'ready' ? 'done' : 'pending',
    },
  ];
}
