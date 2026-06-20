import { useState } from 'react';
import { AlertCircle, CheckCircle2, Download, Loader2 } from 'lucide-react';
import { exportWalletCsv } from '../../services/api';

const exportTypes = [
  { value: 'transactions', label: 'Transactions' },
  { value: 'dailybalances', label: 'Daily Balances' },
  { value: 'balances', label: 'Balances' },
  { value: 'votetime', label: 'Vote Time' },
  { value: 'tickets', label: 'Tickets' },
];

// Reads the download filename from a Content-Disposition header, falling back
// to a sensible default.
const filenameFrom = (disposition: string | undefined, type: string): string => {
  const m = disposition?.match(/filename="?([^"]+)"?/);
  return m?.[1] || `dcrpulse-${type}-${Math.floor(Date.now() / 1000)}.csv`;
};

export const ExportTab = () => {
  const [type, setType] = useState('transactions');
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);

  const onExport = async () => {
    setExporting(true);
    setError(null);
    setDone(null);
    try {
      const resp = await exportWalletCsv(type);
      const name = filenameFrom(resp.headers['content-disposition'], type);
      const blob = new Blob([resp.data], { type: 'text/csv' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setDone(name);
    } catch (err: any) {
      // A blob response body needs to be decoded to read the server's message.
      let msg = err?.message || 'Export failed';
      const body = err?.response?.data;
      if (body instanceof Blob) {
        try {
          msg = (await body.text()) || msg;
        } catch {
          // keep the generic message
        }
      } else if (typeof body === 'string') {
        msg = body;
      }
      setError(msg);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="p-8 rounded-lg bg-card border border-border space-y-5 max-w-xl">
      <div>
        <h2 className="text-2xl font-semibold mb-1">Export</h2>
        <p className="text-sm text-muted-foreground">
          Download wallet history and statistics as CSV. The format matches Decrediton's
          export, with amounts in DCR and timestamps in UTC.
        </p>
      </div>

      <div className="space-y-2">
        <label htmlFor="export-type" className="text-sm font-medium">
          Export type
        </label>
        <select
          id="export-type"
          value={type}
          onChange={(e) => setType(e.target.value)}
          disabled={exporting}
          className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground disabled:opacity-50"
        >
          {exportTypes.map((t) => (
            <option key={t.value} value={t.value}>
              {t.label}
            </option>
          ))}
        </select>
      </div>

      <button
        type="button"
        onClick={onExport}
        disabled={exporting}
        className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {exporting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
        {exporting ? 'Exporting…' : 'Export CSV'}
      </button>

      {done && (
        <div className="text-xs text-success inline-flex items-center gap-1">
          <CheckCircle2 className="h-3.5 w-3.5" />
          Downloaded {done}.
        </div>
      )}
      {error && (
        <div className="text-xs text-destructive inline-flex items-center gap-1">
          <AlertCircle className="h-3.5 w-3.5" />
          {error}
        </div>
      )}
    </div>
  );
};
