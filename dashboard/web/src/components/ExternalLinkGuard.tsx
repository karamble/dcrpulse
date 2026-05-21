import { useEffect, useState } from 'react';
import { AlertTriangle, ExternalLink, X } from 'lucide-react';

export const ExternalLinkGuard = () => {
  const [pendingUrl, setPendingUrl] = useState<string | null>(null);

  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      if (event.defaultPrevented) return;
      if (event.button !== 0) return;
      if (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;

      const target = event.target as Element | null;
      const anchor = target?.closest?.('a');
      if (!anchor) return;

      const href = anchor.getAttribute('href');
      if (!href) return;

      let url: URL;
      try {
        url = new URL(href, window.location.href);
      } catch {
        return;
      }

      if (url.protocol !== 'http:' && url.protocol !== 'https:') return;
      if (url.hostname === window.location.hostname) return;

      event.preventDefault();
      event.stopPropagation();
      setPendingUrl(url.toString());
    };

    document.addEventListener('click', onClick, true);
    return () => document.removeEventListener('click', onClick, true);
  }, []);

  useEffect(() => {
    if (!pendingUrl) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPendingUrl(null);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [pendingUrl]);

  if (!pendingUrl) return null;

  const handleCancel = () => setPendingUrl(null);
  const handleVisit = () => {
    window.open(pendingUrl, '_blank', 'noopener,noreferrer');
    setPendingUrl(null);
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in"
      onClick={handleCancel}
    >
      <div
        className="w-full max-w-md mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-warning" />
            You're leaving dcrpulse
          </h3>
          <button
            onClick={handleCancel}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-4">
          <p className="text-sm text-muted-foreground">
            This link goes to an external site. Verify the destination before continuing.
          </p>

          <div className="rounded-lg bg-background border border-border px-3 py-2">
            <code className="block text-xs text-foreground break-all font-mono">
              {pendingUrl}
            </code>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={handleCancel}
              className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={handleVisit}
              className="px-4 py-2 rounded-lg bg-gradient-primary text-white font-semibold transition-all text-sm inline-flex items-center gap-1.5"
            >
              <ExternalLink className="h-4 w-4" />
              Visit Link
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
