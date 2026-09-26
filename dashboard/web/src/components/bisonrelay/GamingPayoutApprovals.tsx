import { useCallback, useRef, useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';
import { formatAtomsTrimmed } from '../../utils/amounts';
import { approveGamingPayout, getGamingPayouts, GamingPayout, rejectGamingPayout } from '../../services/gamingApi';

export function GamingPayoutApprovals() {
  const [rows, setRows] = useState<GamingPayout[]>([]);
  const [selected, setSelected] = useState<GamingPayout | null>(null);
  const [passphrase, setPassphrase] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const loading = useRef(false);
  const load = useCallback(async () => {
    if (loading.current) return;
    loading.current = true;
    try { setRows(await getGamingPayouts()); } catch (e) { setError(apiError(e, 'Payout ledger unavailable')); }
    finally { loading.current = false; }
  }, []);
  useVisiblePoll(() => void load(), 5000);
  const outputs = (p: GamingPayout) => p.payments.map(pay => <p key={pay.key} className="text-sm"><strong>{formatAtomsTrimmed(pay.atoms)} DCR</strong>{p.mine?.key === pay.key && <span className="ml-2 rounded bg-emerald-600/30 px-1.5 text-xs font-medium text-emerald-300">You</span>}<span className="block break-all font-mono text-xs text-gray-400">{p.destinations[pay.key]}</span></p>);
  const share = (p: GamingPayout) => <PayoutShare payout={p} />;
  return <section className="space-y-3">
    <h3 className="flex items-center gap-2 font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-400" /> Match payouts</h3>
    {error && <p role="alert" className="text-sm text-red-400">{error}</p>}
    {rows.length === 0 && <p className="text-sm text-gray-400">Payout proposals will appear here for your approval.</p>}
    {rows.map(p => <article key={p.id} className="space-y-2 rounded-xl border border-gray-700 bg-gray-900/40 p-4">
      <p className="font-medium">{p.scope.game} · {p.state.replace(/_/g, ' ')}</p>
      <p className="break-all font-mono text-xs text-gray-400">Table {p.table}</p>
      {share(p)}
      {outputs(p)}
      <p className="text-xs text-gray-400">Fee {formatAtomsTrimmed(p.feeAtoms)} DCR · {Object.keys(p.signatures).length}/{Object.keys(p.destinations).length} approvals</p>
      {p.state === 'awaiting_approval' && <button type="button" disabled={busy || Date.now() >= p.expiresAt * 1000} onClick={() => { setSelected(p); setPassphrase(''); setError(''); }} className="rounded-lg bg-emerald-600 px-3 py-2 text-sm disabled:opacity-40">Review payout</button>}
    </article>)}
    {selected && <div role="dialog" aria-modal="true" aria-labelledby="payout-title" className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"><div className="w-full max-w-lg space-y-4 rounded-xl border border-gray-700 bg-gray-900 p-6">
      <h3 id="payout-title" className="text-lg font-semibold">Approve this exact payout</h3>
      <p className="text-sm text-gray-300">Check the match result and recipients. Approval releases your signatures to the other players and cannot be revoked.</p>
      {share(selected)}
      {outputs(selected)}
      <p className="text-sm">Fee: {formatAtomsTrimmed(selected.feeAtoms)} DCR</p>
      <label className="block text-sm">Wallet account passphrase<input type="password" autoComplete="off" value={passphrase} onChange={e => setPassphrase(e.target.value)} className="mt-1 w-full rounded border border-gray-600 bg-gray-950 p-2" /></label>
      {error && <p role="alert" className="text-sm text-red-400">{error}</p>}
      <div className="flex justify-end gap-3"><button disabled={busy} onClick={() => { if (busy) return; setBusy(true); void rejectGamingPayout(selected.id).then(() => { setSelected(null); setPassphrase(''); return load(); }).catch(e => setError(apiError(e, 'Payout rejection failed'))).finally(() => { setPassphrase(''); setBusy(false); }); }} className="rounded-lg border border-red-500/60 px-3 py-2 text-red-300 disabled:opacity-40">Reject</button><button disabled={busy} onClick={() => { setSelected(null); setPassphrase(''); }} className="px-3 py-2">Cancel</button><button disabled={busy} onClick={() => { if (busy) return; setBusy(true); void approveGamingPayout(selected.id, passphrase).then(() => { setSelected(null); setPassphrase(''); return load(); }).catch(e => setError(apiError(e, 'Payout approval failed'))).finally(() => { setPassphrase(''); setBusy(false); }); }} className="rounded-lg bg-emerald-600 px-4 py-2 disabled:opacity-40">{busy ? 'Approving…' : 'Approve payout'}</button></div>
    </div></div>}
  </section>;
}

// PayoutShare states what the operator staked and what this payout pays them,
// and warns when it pays them nothing or their output cannot be found.
export function PayoutShare({ payout }: { payout: GamingPayout }) {
  const mine = payout.mine;
  if (!mine) {
    return <p role="alert" className="text-sm font-medium text-red-400">Your own output could not be identified. Do not approve unless you can match the addresses yourself.</p>;
  }
  const net = mine.receiveAtoms - mine.stakeAtoms;
  return <div className="space-y-1">
    <p className="text-sm">You staked <strong>{formatAtomsTrimmed(mine.stakeAtoms)} DCR</strong> · you receive <strong>{formatAtomsTrimmed(mine.receiveAtoms)} DCR</strong> · net <strong className={net >= 0 ? 'text-emerald-400' : 'text-red-400'}>{net >= 0 ? '+' : '−'}{formatAtomsTrimmed(Math.abs(net))} DCR</strong></p>
    {mine.receiveAtoms === 0 && <p role="alert" className="text-sm font-medium text-red-400">This payout pays you nothing. Approve it only if you lost the match.</p>}
  </div>;
}
