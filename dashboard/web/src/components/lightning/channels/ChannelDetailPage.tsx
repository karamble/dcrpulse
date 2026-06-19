import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ArrowLeft, ExternalLink, Loader2 } from 'lucide-react';
import {
  LightningChannel,
  getLightningChannels,
} from '../../../services/lightningApi';
import { CloseChannelModal } from './CloseChannelModal';
import { useDemo } from '../../DemoProvider';

const fmtDcr = (atoms?: number) => ((atoms || 0) / 1e8).toFixed(8) + ' DCR';

const fundingTxid = (channelPoint: string): string => {
  const colon = channelPoint.indexOf(':');
  return colon === -1 ? channelPoint : channelPoint.slice(0, colon);
};

const TxLink = ({ txid }: { txid?: string }) => {
  if (!txid) return <span className="text-muted-foreground">-</span>;
  return (
    <Link
      to={`/explorer/tx/${txid}`}
      className="font-mono break-all text-primary hover:underline inline-flex items-center gap-1"
    >
      {txid}
      <ExternalLink className="h-3 w-3 shrink-0" />
    </Link>
  );
};

// fieldsForStatus mirrors Decrediton's getOpenChannelDetails /
// getPendingChannelDetails / getClosedChannelDetails (details.js:6-220)
// — the displayed field set depends on the channel's status.
const fieldsForStatus = (c: LightningChannel): Array<[string, React.ReactNode]> => {
  const common: Array<[string, React.ReactNode]> = [
    ['Funding tx', <TxLink txid={fundingTxid(c.channelPoint)} />],
    ['Channel point', <span className="font-mono break-all">{c.channelPoint}</span>],
    ['Remote pubkey', <span className="font-mono break-all">{c.remotePubkey}</span>],
    ['Capacity', fmtDcr(c.capacity)],
  ];
  if (c.status === 'open') {
    return [
      ...common,
      ['Channel ID', String(c.channelId ?? '')],
      ['Local balance', fmtDcr(c.localBalance)],
      ['Remote balance', fmtDcr(c.remoteBalance)],
      ['Commit fee', fmtDcr(c.commitFee)],
      ['CSV delay', String(c.csvDelay ?? 0)],
      ['Unsettled balance', fmtDcr(c.unsettledBalance)],
      ['Total sent', fmtDcr(c.totalSent)],
      ['Total received', fmtDcr(c.totalReceived)],
      ['Updates', String(c.numUpdates ?? 0)],
      ['Initiator', c.initiator ? 'yes' : 'no'],
      ['Private', c.private ? 'yes' : 'no'],
      ['Active', c.active ? 'yes' : 'no'],
    ];
  }
  if (c.status === 'closed') {
    return [
      ...common,
      ['Close type', c.closeType || ''],
      ['Closing tx', <TxLink txid={c.closingTxHash} />],
      ['Settled balance', fmtDcr(c.settledBalance)],
      ['Time-locked balance', fmtDcr(c.timeLockedBalance)],
    ];
  }
  // Pending statuses
  const rows: Array<[string, React.ReactNode]> = [
    ...common,
    ['Local balance', fmtDcr(c.localBalance)],
    ['Remote balance', fmtDcr(c.remoteBalance)],
    ['Limbo balance', fmtDcr(c.limboBalance)],
    ['Closing tx', <TxLink txid={c.closingTxHash} />],
  ];
  if (c.status === 'pending-open' && c.requiredConfs && c.requiredConfs > 0) {
    rows.splice(4, 0, [
      'Confirmations',
      <span>
        {c.currentConfs ?? 0} / {c.requiredConfs}
      </span>,
    ]);
  }
  return rows;
};

export const ChannelDetailPage = () => {
  const { channelPoint } = useParams<{ channelPoint: string }>();
  const navigate = useNavigate();
  const [channel, setChannel] = useState<LightningChannel | null>(null);
  const [loading, setLoading] = useState(true);
  const [closeOpen, setCloseOpen] = useState(false);
  const { demoMode, showDemoDisabledModal } = useDemo();

  const load = async () => {
    setLoading(true);
    try {
      const r = await getLightningChannels();
      const cp = decodeURIComponent(channelPoint || '');
      setChannel(r.channels.find((c) => c.channelPoint === cp) || null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelPoint]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading channel…
      </div>
    );
  }

  if (!channel) {
    return (
      <div className="space-y-4">
        <button
          onClick={() => navigate('/wallet/lightning/channels')}
          className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to channels
        </button>
        <p className="text-sm text-muted-foreground">Channel not found.</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <button
        onClick={() => navigate('/wallet/lightning/channels')}
        className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to channels
      </button>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 space-y-4">
        <h3 className="text-lg font-semibold">Channel details</h3>
        <dl className="grid grid-cols-1 md:grid-cols-[200px_1fr] gap-x-4 gap-y-2 text-sm">
          {fieldsForStatus(channel).map(([label, value]) => (
            <div key={label} className="contents">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="text-foreground break-all">{value}</dd>
            </div>
          ))}
        </dl>

        {(channel.status === 'open' || channel.status === 'pending-open') && (
          <div className="flex justify-end pt-2">
            <button
              onClick={() => {
                if (demoMode) {
                  showDemoDisabledModal();
                  return;
                }
                setCloseOpen(true);
              }}
              className="px-4 py-2 rounded-lg bg-destructive/20 hover:bg-destructive/30 text-destructive text-sm font-semibold"
            >
              Close channel
            </button>
          </div>
        )}
      </div>

      {closeOpen && (
        <CloseChannelModal
          channel={channel}
          onClose={() => setCloseOpen(false)}
          onClosed={() => {
            setCloseOpen(false);
            navigate('/wallet/lightning/channels');
          }}
        />
      )}
    </div>
  );
};
