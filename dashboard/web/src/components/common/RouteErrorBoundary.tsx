// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Component, type ErrorInfo, type ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';

// RouteErrorBoundary keeps a page that throws while rendering from unmounting
// the whole dashboard: the page is replaced by an error card and the header
// and navigation keep working. Key it on the route so navigating resets it.
export class RouteErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Page render failed:', error, info.componentStack);
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <div className="p-6 rounded-xl bg-gradient-card border border-destructive/30 space-y-3 max-w-2xl">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <AlertTriangle className="h-5 w-5 text-destructive" />
          This page hit an error
        </h2>
        <p className="text-sm text-muted-foreground break-words font-mono">{error.message || String(error)}</p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold"
        >
          Reload
        </button>
      </div>
    );
  }
}
