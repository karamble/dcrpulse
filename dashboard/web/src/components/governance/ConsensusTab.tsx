// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, CheckCircle2, Loader2 } from 'lucide-react';
import { Agenda, getAgendas, setAgendaChoice } from '../../services/api';
import { PassphraseModal } from '../wallet/PassphraseModal';

const statusColor = (status: string) => {
  switch (status) {
    case 'active':
    case 'started':
      return 'bg-info/15 text-info border border-info/30';
    case 'lockedin':
    case 'locked_in':
      return 'bg-success/15 text-success border border-success/30';
    case 'failed':
      return 'bg-destructive/15 text-destructive border border-destructive/30';
    case 'defined':
      return 'bg-muted/15 text-muted-foreground border border-border/50';
    default:
      return 'bg-muted/15 text-muted-foreground border border-border/50';
  }
};

export const ConsensusTab = () => {
  const [agendas, setAgendas] = useState<Agenda[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState<{ agendaID: string; choiceID: string } | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  const load = async () => {
    setError(null);
    try {
      setAgendas(await getAgendas());
    } catch (err: any) {
      const body = err?.response?.data;
      setError(typeof body === 'string' ? body : err?.message || 'Failed to load agendas');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    const id = window.setInterval(load, 30000);
    return () => window.clearInterval(id);
  }, []);

  const handleSubmit = async (passphrase: string) => {
    if (!pending) return;
    try {
      await setAgendaChoice(pending.agendaID, pending.choiceID, passphrase);
      setFeedback(`Saved choice "${pending.choiceID}" for ${pending.agendaID}.`);
      setPending(null);
      await load();
    } catch (err: any) {
      const body = err?.response?.data;
      throw new Error(typeof body === 'string' ? body : err?.message || 'Failed to set choice');
    }
  };

  if (loading && agendas.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground flex items-center gap-2">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading agendas...
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 rounded-xl bg-destructive/5 border border-destructive/30 text-sm text-destructive flex items-center gap-2">
        <AlertCircle className="h-4 w-4" />
        {error}
      </div>
    );
  }

  if (agendas.length === 0) {
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-border/50 text-sm text-muted-foreground">
        No active consensus agendas right now. Decred only proposes new rule changes when there's a
        deployment scheduled; check back later.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {feedback && (
        <div className="flex items-center gap-2 text-sm text-success">
          <CheckCircle2 className="h-4 w-4" />
          {feedback}
        </div>
      )}
      {agendas.map((a) => (
        <div
          key={a.id}
          className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-3"
        >
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="font-semibold">{a.id}</h3>
              <p className="text-sm text-muted-foreground mt-1">{a.description}</p>
            </div>
            <span className={`px-2 py-0.5 rounded text-xs font-medium ${statusColor(a.status)}`}>
              {a.status}
            </span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
            {a.choices.map((c) => {
              const isCurrent = a.currentChoice === c.id;
              return (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => {
                    setFeedback(null);
                    setPending({ agendaID: a.id, choiceID: c.id });
                  }}
                  className={`p-3 rounded-lg border text-left text-sm transition-colors ${
                    isCurrent
                      ? 'border-primary/40 bg-primary/10 text-foreground'
                      : 'border-border/50 bg-muted/10 hover:bg-muted/20'
                  }`}
                >
                  <div className="font-medium">{c.id}</div>
                  <div className="text-xs text-muted-foreground mt-0.5">{c.description}</div>
                  {isCurrent && <div className="text-xs text-primary mt-1">current choice</div>}
                </button>
              );
            })}
          </div>
        </div>
      ))}

      <PassphraseModal
        isOpen={pending !== null}
        title="Confirm Agenda Vote Choice"
        description={
          pending
            ? `Set "${pending.choiceID}" for agenda ${pending.agendaID}. The wallet is unlocked briefly to write the choice.`
            : ''
        }
        submitLabel="Save"
        busyLabel="Saving..."
        onSubmit={handleSubmit}
        onClose={() => setPending(null)}
      />
    </div>
  );
};
