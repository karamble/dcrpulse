import { useEffect, useState } from 'react';
import { CheckCircle2, Copy } from 'lucide-react';
import { LightningInfo, getLightningInfo } from '../../../services/lightningApi';
import { TorStatus, getTorStatus } from '../../../services/tor/client';

const trunc = (s: string, head = 14, tail = 8) =>
  s.length <= head + tail + 1 ? s : `${s.slice(0, head)}…${s.slice(-tail)}`;

export const InfosSection = () => {
  const [info, setInfo] = useState<LightningInfo | null>(null);
  const [tor, setTor] = useState<TorStatus | null>(null);
  const [copied, setCopied] = useState<'pubkey' | 'uri' | null>(null);

  useEffect(() => {
    let cancelled = false;
    getLightningInfo()
      .then((r) => {
        if (!cancelled) setInfo(r);
      })
      .catch(() => {
        /* keep null */
      });
    getTorStatus()
      .then((r) => {
        if (!cancelled) setTor(r);
      })
      .catch(() => {
        /* row stays absent */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const copy = async (v: string, which: 'pubkey' | 'uri') => {
    try {
      await navigator.clipboard.writeText(v);
      setCopied(which);
      window.setTimeout(() => setCopied(null), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  const onionUri =
    info && tor?.settings.enabled && tor.settings.lnOnion && tor.lnOnionAddress
      ? `${info.identityPubkey}@${tor.lnOnionAddress}:9735`
      : null;

  return (
    <div className="p-5 rounded-xl bg-gradient-card border border-border/60 space-y-3">
      <h3 className="text-lg font-semibold">Node info</h3>
      {!info ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : (
        <div className="space-y-2 text-sm">
          <div className="flex items-start justify-between gap-3">
            <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">
              Alias
            </span>
            <span className="text-right">{info.alias || '-'}</span>
          </div>
          <div className="flex items-start justify-between gap-3">
            <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">
              Identity pubkey
            </span>
            <span className="font-mono text-right break-all">
              {trunc(info.identityPubkey)}
              <button
                onClick={() => copy(info.identityPubkey, 'pubkey')}
                className="ml-2 inline-flex items-center text-muted-foreground hover:text-foreground"
                title="Copy"
                type="button"
              >
                {copied === 'pubkey' ? (
                  <CheckCircle2 className="h-3.5 w-3.5 text-success" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </button>
            </span>
          </div>
          {onionUri && (
            <div className="flex items-start justify-between gap-3">
              <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">
                Onion URI
              </span>
              <span className="font-mono text-right break-all">
                {trunc(onionUri)}
                <button
                  onClick={() => copy(onionUri, 'uri')}
                  className="ml-2 inline-flex items-center text-muted-foreground hover:text-foreground"
                  title="Copy"
                  type="button"
                >
                  {copied === 'uri' ? (
                    <CheckCircle2 className="h-3.5 w-3.5 text-success" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                </button>
              </span>
            </div>
          )}
          <div className="flex items-start justify-between gap-3">
            <span className="text-xs uppercase tracking-wide text-muted-foreground shrink-0">
              Block height
            </span>
            <span className="text-right">{info.blockHeight.toLocaleString()}</span>
          </div>
        </div>
      )}
    </div>
  );
};
