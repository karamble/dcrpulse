import { useCallback, useRef, useState } from 'react';
import { ArrowDownToLine, Loader2, RefreshCw, ShieldCheck } from 'lucide-react';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';
import { formatAtomsTrimmed } from '../../utils/amounts';
import { closeRecoveryTable, confirmRecovery, getGamingRecovery, quoteRecovery, RecoveryDeposit, RecoveryQuote } from '../../services/gamingApi';

const labels: Record<string, string> = {seatbond: 'Admission bond', stake: 'Game stake', tablebond: 'Table bond'};
const states: Record<string, string> = {awaiting_payment: 'Awaiting payment', locked: 'Time locked', close_table: 'Ready after table closure', recoverable: 'Ready to recover', recovery_pending: 'Refund broadcast', spent: 'Refunded', needs_attention: 'Needs attention'};

export function GamingRecovery() {
  const [rows, setRows] = useState<RecoveryDeposit[] | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [quote, setQuote] = useState<RecoveryQuote | null>(null);
  const [notice, setNotice] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const loading = useRef(false);
  const action = useRef(false);
  const load = useCallback(async () => {
    if (loading.current) return;
    loading.current = true;
    try { setRows(await getGamingRecovery()); setError(''); }
    catch (e) { setError(apiError(e, 'Could not verify deposits')); }
    finally { loading.current = false; }
  }, []);
  useVisiblePoll(() => void load(), 15000);
  const perform = async (id: string, work: () => Promise<void>) => {
    if (action.current) return;
    action.current = true; setBusy(id); setNotice('');
    try { await work(); await load(); }
    catch (e) { setNotice(apiError(e, 'Recovery needs attention')); }
    finally { action.current = false; setBusy(''); }
  };
  return <section className="space-y-5">
    <div className="flex items-start justify-between gap-4">
      <div><h2 className="flex items-center gap-2 text-lg font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-400" /> Recover your deposits</h2>
        <p className="mt-1 text-sm text-gray-400">Bonds and stakes remain recorded here when a game closes. Recovery eligibility is checked against your node.</p></div>
      <button type="button" onClick={() => void load()} className="rounded-lg border border-gray-700 p-2" aria-label="Refresh recovery status"><RefreshCw className="h-4 w-4" /></button>
    </div>
    {error && <p role="alert" className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-300">{error} Recovery actions are disabled until fresh verification succeeds.</p>}
    {notice && <p role="status" className="break-all rounded-lg bg-blue-500/10 p-3 text-sm">{notice}</p>}
    {rows === null && !error && <p className="flex items-center gap-2 text-sm"><Loader2 className="h-4 w-4 animate-spin" /> Checking the deposit ledger…</p>}
    {rows?.length === 0 && <p className="rounded-xl border border-gray-700 p-5 text-sm text-gray-400">No deposits have been registered in the bridge ledger.</p>}
    <div className="grid gap-3">{rows?.map(row => <article key={row.id} className="rounded-xl border border-gray-700 bg-gray-900/40 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="font-medium">{labels[row.kind] ?? row.kind} <span className="text-sm text-gray-400">· {row.game}</span></h3><p className="mt-1 text-xl font-semibold">{formatAtomsTrimmed(row.atoms)} DCR</p></div><span className="rounded-full bg-gray-700/50 px-3 py-1 text-xs">{states[row.state] ?? row.state}</span></div>
      <p className="mt-3 break-all font-mono text-xs text-gray-500">Table {row.table || 'identity'}{row.outpoint && <><br />{row.outpoint}</>}</p>
      {row.state === 'locked' && <div className="mt-3"><progress className="h-1.5 w-full accent-emerald-400" max={row.lockBlocks} value={Math.max(0, row.confirmations)} /><p className="mt-1 text-xs text-gray-400">{row.remainingBlocks.toLocaleString()} blocks remaining · approximately {Math.ceil(row.remainingBlocks * 5 / 60)} hours</p></div>}
      {row.reason && <p className="mt-2 text-sm text-gray-400">{row.reason}</p>}
      <div className="mt-3 flex gap-2">{row.state === 'close_table' && <button type="button" disabled={!!busy || !!error} onClick={() => void perform(row.id, async () => { await closeRecoveryTable(row.id); setNotice('Table closed locally. Deposits remain tracked.'); })} className="rounded-lg border border-gray-600 px-3 py-2 text-sm disabled:opacity-40">Close table</button>}
      {row.canRecover && <button type="button" disabled={!!busy || !!error} onClick={() => void perform(row.id, async () => setQuote(await quoteRecovery(row.id)))} className="flex items-center gap-2 rounded-lg bg-emerald-600 px-3 py-2 text-sm font-medium disabled:opacity-40"><ArrowDownToLine className="h-4 w-4" />Take it back</button>}
      {busy === row.id && <Loader2 className="h-5 w-5 animate-spin" />}</div>
    </article>)}</div>
    {quote && <div role="dialog" aria-modal="true" aria-labelledby="recovery-title" className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"><div className="w-full max-w-lg space-y-4 rounded-2xl border border-gray-700 bg-gray-900 p-6">
      <h3 id="recovery-title" className="text-lg font-semibold">Confirm recovery</h3><dl className="space-y-2 text-sm"><div><dt className="text-gray-400">Returned to your wallet</dt><dd className="text-xl">{formatAtomsTrimmed(quote.returnAtoms)} DCR</dd></div><div><dt className="text-gray-400">Transaction fee</dt><dd>{formatAtomsTrimmed(quote.feeAtoms)} DCR</dd></div><div><dt className="text-gray-400">Destination</dt><dd className="break-all font-mono text-xs">{quote.destination}</dd></div></dl>
      <p className="text-xs text-gray-400">The bridge will recheck the deposit, then sign and broadcast this refund. The quote expires after two minutes.</p>
      <label className="block text-sm">Wallet account passphrase<input type="password" autoComplete="off" value={passphrase} onChange={e => setPassphrase(e.target.value)} className="mt-1 w-full rounded border border-gray-600 bg-gray-950 p-2" /></label>
      <div className="flex justify-end gap-3"><button type="button" disabled={!!busy} onClick={() => { setQuote(null); setPassphrase(''); }} className="px-3 py-2">Cancel</button><button type="button" disabled={!!busy || !!error} className="rounded-lg bg-emerald-600 px-4 py-2 disabled:opacity-40" onClick={() => void perform(quote.depositId, async () => { const result = await confirmRecovery(quote.depositId, quote.id, passphrase); if (result.error && !result.pending) throw new Error(result.error); setNotice(result.error ? 'Refund signed and saved. The broadcast will be retried automatically.' : `Refund broadcast: ${result.txid}. It is in the mempool and confirms with the next block, usually within a few minutes.`); setQuote(null); setPassphrase(''); })}>Approve recovery</button></div>
    </div></div>}
  </section>;
}
