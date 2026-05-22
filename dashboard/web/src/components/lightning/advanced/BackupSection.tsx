import { useRef, useState } from 'react';
import { AlertCircle, CheckCircle2, Download, Loader2, Upload } from 'lucide-react';
import { getLnChannelBackup, verifyLnChannelBackup } from '../../../services/lightningApi';

// Decodes a base64 string back into a binary Blob suitable for download.
const base64ToBlob = (b64: string): Blob => {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return new Blob([out], { type: 'application/octet-stream' });
};

// ArrayBuffer -> base64. Avoids the call-stack overflow risk of
// String.fromCharCode(...bigArray) for files of any meaningful size.
const arrayBufferToBase64 = (buf: ArrayBuffer): string => {
  const bytes = new Uint8Array(buf);
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
};

export const BackupSection = () => {
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);
  const [exportResult, setExportResult] = useState<{ numChannels: number } | null>(null);

  const [verifying, setVerifying] = useState(false);
  const [verifyResult, setVerifyResult] = useState<{ ok: boolean; error?: string } | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const onExport = async () => {
    setExporting(true);
    setExportError(null);
    setExportResult(null);
    try {
      const r = await getLnChannelBackup();
      const blob = base64ToBlob(r.backupBase64);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `channel-backup-${Math.floor(Date.now() / 1000)}.scb`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setExportResult({ numChannels: r.numChannels });
    } catch (err: any) {
      const body = err?.response?.data;
      setExportError(typeof body === 'string' ? body : err?.message || 'Export failed');
    } finally {
      setExporting(false);
    }
  };

  const onPickFile = () => fileInputRef.current?.click();

  const onFileChosen = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    setVerifying(true);
    setVerifyResult(null);
    try {
      const buf = await f.arrayBuffer();
      const b64 = arrayBufferToBase64(buf);
      const r = await verifyLnChannelBackup(b64);
      setVerifyResult(r);
    } catch (err: any) {
      const body = err?.response?.data;
      setVerifyResult({ ok: false, error: typeof body === 'string' ? body : err?.message || 'Verify failed' });
    } finally {
      setVerifying(false);
    }
  };

  return (
    <div className="p-5 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/60 space-y-4">
      <div>
        <h3 className="text-lg font-semibold">Channel backup</h3>
        <p className="text-sm text-muted-foreground">
          Export the Static Channel Backup (SCB) of all your channels and store it somewhere
          safe. Without the SCB, channel-state recovery is impossible if dcrlnd's local state
          is lost.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div className="p-4 rounded-lg bg-background/40 border border-border/60 space-y-3">
          <div className="text-sm font-medium">Export channel backup</div>
          <button
            type="button"
            onClick={onExport}
            disabled={exporting}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {exporting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Download className="h-4 w-4" />}
            {exporting ? 'Exporting…' : 'Download backup'}
          </button>
          {exportResult && (
            <div className="text-xs text-success inline-flex items-center gap-1">
              <CheckCircle2 className="h-3.5 w-3.5" />
              Downloaded backup of {exportResult.numChannels} channel
              {exportResult.numChannels === 1 ? '' : 's'}.
            </div>
          )}
          {exportError && (
            <div className="text-xs text-destructive inline-flex items-center gap-1">
              <AlertCircle className="h-3.5 w-3.5" />
              {exportError}
            </div>
          )}
        </div>

        <div className="p-4 rounded-lg bg-background/40 border border-border/60 space-y-3">
          <div className="text-sm font-medium">Verify backup</div>
          <input
            ref={fileInputRef}
            type="file"
            className="hidden"
            onChange={onFileChosen}
          />
          <button
            type="button"
            onClick={onPickFile}
            disabled={verifying}
            className="px-4 py-2 rounded-lg bg-muted/30 text-foreground text-sm hover:bg-muted/50 inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {verifying ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
            {verifying ? 'Verifying…' : 'Choose backup file…'}
          </button>
          {verifyResult?.ok && (
            <div className="text-xs text-success inline-flex items-center gap-1">
              <CheckCircle2 className="h-3.5 w-3.5" /> Backup is valid.
            </div>
          )}
          {verifyResult && !verifyResult.ok && (
            <div className="text-xs text-destructive inline-flex items-center gap-1">
              <AlertCircle className="h-3.5 w-3.5" />
              {verifyResult.error || 'Backup is invalid.'}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
