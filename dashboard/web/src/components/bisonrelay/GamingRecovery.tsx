import { useCallback, useRef, useState } from 'react';
import { ArchiveRestore, ArrowDownToLine, Download, Loader2, RefreshCw, ShieldCheck, Trash2, Upload } from 'lucide-react';
import { useVisiblePoll } from '../../hooks/useVisiblePoll';
import { apiError } from '../../utils/apiError';
import { formatAtomsTrimmed } from '../../utils/amounts';
import { archiveRecovery, closeRecoveryTable, confirmRecovery, getGamingLedgerBackup, getGamingRecovery, quoteRecovery, RecoveryDeposit, RecoveryQuote, restoreGamingLedger } from '../../services/gamingApi';

const labels: Record<string, string> = {seatbond: 'Admission bond', stake: 'Game stake', tablebond: 'Table bond'};
const states: Record<string, string> = {awaiting_payment: 'Awaiting payment', locked: 'Time locked', close_table: 'Ready after table closure', recoverable: 'Ready to recover', recovery_pending: 'Refund broadcast', spent: 'Refunded', needs_attention: 'Needs attention'};

// rank puts what the operator can act on first, then what is still locked,
// then what waits for payment or a broadcast, then what is finished.
const rank = (r: RecoveryDeposit): number => {
  switch (r.state) {
    case 'recoverable': case 'close_table': case 'needs_attention': return 0;
    case 'locked': return 1;
    case 'awaiting_payment': return 2;
    case 'recovery_pending': return 3;
    default: return 4;
  }
};
const byRank = (a: RecoveryDeposit, b: RecoveryDeposit): number =>
  rank(a) - rank(b) || (rank(a) === 1 ? a.remainingBlocks - b.remainingBlocks : 0) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);

export interface RecoveryGroup { table: string; game: string; items: RecoveryDeposit[] }

// recoveryGroups gathers one table's deposits under one heading, ordered by
// the table's most urgent deposit.
export function recoveryGroups(rows: RecoveryDeposit[]): RecoveryGroup[] {
  const groups = new Map<string, RecoveryGroup>();
  for (const r of rows) {
    const key = `${r.game}\u0000${r.table}`;
    const g = groups.get(key) ?? { table: r.table, game: r.game, items: [] };
    g.items.push(r);
    groups.set(key, g);
  }
  const out = [...groups.values()];
  for (const g of out) g.items.sort(byRank);
  return out.sort((a, b) => byRank(a.items[0], b.items[0]) || (a.table < b.table ? -1 : a.table > b.table ? 1 : 0));
}

export function GamingRecovery() {
  const [rows, setRows] = useState<RecoveryDeposit[] | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [quote, setQuote] = useState<RecoveryQuote | null>(null);
  const [notice, setNotice] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [saving, setSaving] = useState(false);
  const [showArchived, setShowArchived] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const pickFile = useRef<HTMLInputElement>(null);
  const loading = useRef(false);
  const action = useRef(false);
  const downloadBackup = async () => {
    setSaving(true); setNotice('');
    try {
      const url = URL.createObjectURL(await getGamingLedgerBackup());
      const a = document.createElement('a');
      a.href = url;
      a.download = `gaming-ledger-${Math.floor(Date.now() / 1000)}.json`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
    } catch (e) { setNotice(apiError(e, 'Could not export the deposit ledger')); }
    finally { setSaving(false); }
  };
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
  // A backup can only go back while the ledger is empty or missing.
  const canRestore = rows?.length === 0 || /ledger missing/.test(error);
  const restore = (file: File) => void perform('restore', async () => {
    const result = await restoreGamingLedger(file);
    setRestoreFile(null);
    setNotice(result.unownedKeys > 0
      ? `Ledger restored. ${result.unownedKeys} deposit key${result.unownedKeys === 1 ? ' is' : 's are'} not known to this wallet yet, so those deposits cannot be recovered until it has derived their addresses.`
      : 'Ledger restored.');
  });
  return <section className="space-y-5">
    <div className="flex items-start justify-between gap-4">
      <div><h2 className="flex items-center gap-2 text-lg font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-400" /> Recover your deposits</h2>
        <p className="mt-1 text-sm text-gray-400">Bonds and stakes remain recorded here when a game closes. Recovery eligibility is checked against your node.</p>
        <p className="mt-1 text-xs text-gray-500">The backup holds every deposit, open and settled, with its terms and transactions. It contains no keys, but it does show your games and amounts.</p></div>
      <div className="flex shrink-0 gap-2">
        <button type="button" disabled={saving} onClick={() => void downloadBackup()} className="flex items-center gap-2 rounded-lg border border-gray-700 px-3 py-2 text-sm disabled:opacity-40">{saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}Download backup</button>
        {canRestore && <>
          <input ref={pickFile} type="file" accept=".json,application/json" className="hidden" aria-label="Backup file" onChange={e => { setRestoreFile(e.target.files?.[0] ?? null); e.target.value = ''; }} />
          <button type="button" disabled={!!busy} onClick={() => pickFile.current?.click()} className="flex items-center gap-2 rounded-lg border border-gray-700 px-3 py-2 text-sm disabled:opacity-40"><Upload className="h-4 w-4" />Restore backup</button>
        </>}
        <button type="button" onClick={() => void load()} className="rounded-lg border border-gray-700 p-2" aria-label="Refresh recovery status"><RefreshCw className="h-4 w-4" /></button>
      </div>
    </div>
    {error && <p role="alert" className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-300">{error} Recovery actions are disabled until fresh verification succeeds.</p>}
    {restoreFile && <div role="alertdialog" aria-label="Restore the deposit ledger" className="space-y-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm">
      <p>Restore every deposit from <span className="break-all font-mono">{restoreFile.name}</span>? This only works while the gaming ledger is empty, and only for the wallet that made the backup.</p>
      <div className="flex gap-2"><button type="button" disabled={!!busy} onClick={() => setRestoreFile(null)} className="rounded-lg px-3 py-2">Cancel</button><button type="button" disabled={!!busy} onClick={() => restore(restoreFile)} className="flex items-center gap-2 rounded-lg bg-emerald-600 px-3 py-2 font-medium disabled:opacity-40">{busy === 'restore' && <Loader2 className="h-4 w-4 animate-spin" />}Restore ledger</button></div>
    </div>}
    {notice && <p role="status" className="break-all rounded-lg bg-blue-500/10 p-3 text-sm">{notice}</p>}
    {rows === null && !error && <p className="flex items-center gap-2 text-sm"><Loader2 className="h-4 w-4 animate-spin" /> Checking the deposit ledger…</p>}
    {rows?.length === 0 && <p className="rounded-xl border border-gray-700 p-5 text-sm text-gray-400">No deposits have been registered in the bridge ledger.</p>}
    {(['active', 'archived'] as const).map(part => {
      const list = (rows ?? []).filter(r => r.archived === (part === 'archived'));
      if (part === 'archived' && !showArchived) return null;
      return <div key={part} className="space-y-5">{recoveryGroups(list).map(g => <section key={`${part}-${g.game}-${g.table}`} className="space-y-3" aria-label={`Table ${g.table || 'identity'}`}>
        <h3 className="break-all font-mono text-xs text-gray-400">{g.game} · table {g.table || 'identity'}</h3>
        <div className="grid gap-3">{g.items.map(row => <article key={row.id} className="rounded-xl border border-gray-700 bg-gray-900/40 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="font-medium">{labels[row.kind] ?? row.kind} <span className="text-sm text-gray-400">· {row.game}</span></h3><p className="mt-1 text-xl font-semibold">{formatAtomsTrimmed(row.atoms)} DCR</p></div><span className="rounded-full bg-gray-700/50 px-3 py-1 text-xs">{states[row.state] ?? row.state}</span></div>
      {row.outpoint && <p className="mt-3 break-all font-mono text-xs text-gray-500">{row.outpoint}</p>}
      {row.state === 'locked' && <div className="mt-3"><progress className="h-1.5 w-full accent-emerald-400" max={row.lockBlocks} value={Math.max(0, row.confirmations)} /><p className="mt-1 text-xs text-gray-400">{row.remainingBlocks.toLocaleString()} blocks remaining · approximately {Math.ceil(row.remainingBlocks * 5 / 60)} hours</p></div>}
      {row.reason && <p className="mt-2 text-sm text-gray-400">{row.reason}</p>}
      <div className="mt-3 flex gap-2">{row.state === 'close_table' && <button type="button" disabled={!!busy || !!error} onClick={() => void perform(row.id, async () => { await closeRecoveryTable(row.id); setNotice('Table closed locally. Deposits remain tracked.'); })} className="rounded-lg border border-gray-600 px-3 py-2 text-sm disabled:opacity-40">Close table</button>}
      {row.canRecover && <button type="button" disabled={!!busy || !!error} onClick={() => void perform(row.id, async () => setQuote(await quoteRecovery(row.id)))} className="flex items-center gap-2 rounded-lg bg-emerald-600 px-3 py-2 text-sm font-medium disabled:opacity-40"><ArrowDownToLine className="h-4 w-4" />Take it back</button>}
      {row.state === 'spent' && !row.archived && <button type="button" disabled={!!busy} onClick={() => void perform(row.id, () => archiveRecovery(row.id, true))} className="ml-auto rounded-lg border border-gray-700 p-2 text-gray-400 hover:text-gray-200 disabled:opacity-40" aria-label="Archive" title="Archive: hide from this list, keep the record"><Trash2 className="h-4 w-4" /></button>}
      {row.archived && <button type="button" disabled={!!busy} onClick={() => void perform(row.id, () => archiveRecovery(row.id, false))} className="ml-auto flex items-center gap-2 rounded-lg border border-gray-700 px-3 py-2 text-sm disabled:opacity-40"><ArchiveRestore className="h-4 w-4" />Restore</button>}
      {busy === row.id && <Loader2 className="h-5 w-5 animate-spin" />}</div>
    </article>)}</div>
      </section>)}</div>;
    })}
    {(rows ?? []).some(r => r.archived) && <div className="space-y-1">
      <button type="button" onClick={() => setShowArchived(v => !v)} className="text-sm text-gray-400 underline hover:no-underline">{showArchived ? 'Hide archived' : `Show archived (${(rows ?? []).filter(r => r.archived).length})`}</button>
      <p className="text-xs text-gray-500">Archiving hides a finished deposit from this list. Its record stays in the ledger and in the backup.</p>
    </div>}
    {quote && <div role="dialog" aria-modal="true" aria-labelledby="recovery-title" className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"><div className="w-full max-w-lg space-y-4 rounded-2xl border border-gray-700 bg-gray-900 p-6">
      <h3 id="recovery-title" className="text-lg font-semibold">Confirm recovery</h3><dl className="space-y-2 text-sm"><div><dt className="text-gray-400">Returned to your wallet</dt><dd className="text-xl">{formatAtomsTrimmed(quote.returnAtoms)} DCR</dd></div><div><dt className="text-gray-400">Transaction fee</dt><dd>{formatAtomsTrimmed(quote.feeAtoms)} DCR</dd></div><div><dt className="text-gray-400">Destination</dt><dd className="break-all font-mono text-xs">{quote.destination}</dd></div></dl>
      <p className="text-xs text-gray-400">The bridge will recheck the deposit, then sign and broadcast this refund. The quote expires after two minutes.</p>
      <label className="block text-sm">Wallet account passphrase<input type="password" autoComplete="off" value={passphrase} onChange={e => setPassphrase(e.target.value)} className="mt-1 w-full rounded border border-gray-600 bg-gray-950 p-2" /></label>
      <div className="flex justify-end gap-3"><button type="button" disabled={!!busy} onClick={() => { setQuote(null); setPassphrase(''); }} className="px-3 py-2">Cancel</button><button type="button" disabled={!!busy || !!error} className="rounded-lg bg-emerald-600 px-4 py-2 disabled:opacity-40" onClick={() => void perform(quote.depositId, async () => { const result = await confirmRecovery(quote.depositId, quote.id, passphrase); if (result.error && !result.pending) throw new Error(result.error); setNotice(result.error ? 'Refund signed and saved. The broadcast will be retried automatically.' : `Refund broadcast: ${result.txid}. It is in the mempool and confirms with the next block, usually within a few minutes.`); setQuote(null); setPassphrase(''); })}>Approve recovery</button></div>
    </div></div>}
  </section>;
}
