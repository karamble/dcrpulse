// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { createPortal } from 'react-dom';
import { X, Copy, Check, Plug } from 'lucide-react';

interface McpHelpModalProps {
  title: string;
  // agentName is the local name the server is registered under in the agent.
  agentName: string;
  connectUrl: string;
  // token, when known, is filled into the command; otherwise tokenHint
  // explains how to obtain one and the command shows a placeholder.
  token?: string;
  tokenHint?: string;
  onClose: () => void;
}

const portOf = (url: string): string => {
  try {
    return new URL(url).port || '';
  } catch {
    return '';
  }
};

export const McpHelpModal = ({
  title,
  agentName,
  connectUrl,
  token,
  tokenHint,
  onClose,
}: McpHelpModalProps) => {
  const [copied, setCopied] = useState<string | null>(null);

  const bearer = token || 'YOUR_TOKEN';
  const command = `claude mcp add --transport http ${agentName} ${connectUrl} --header "Authorization: Bearer ${bearer}"`;
  const port = portOf(connectUrl) || '8891';

  const copy = (text: string, key: string) => {
    if (!navigator.clipboard) return;
    navigator.clipboard.writeText(text).then(
      () => {
        setCopied(key);
        setTimeout(() => setCopied((c) => (c === key ? null : c)), 1500);
      },
      () => {},
    );
  };

  // Portal to body: a position:fixed overlay is trapped inside any ancestor
  // with backdrop-filter/transform (the settings cards use backdrop-blur), so
  // it would fill the card instead of the viewport.
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm animate-fade-in">
      <div className="w-full max-w-lg mx-4 rounded-xl bg-card border border-border/50 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between p-6 border-b border-border/50">
          <h3 className="text-lg font-semibold flex items-center gap-2">
            <Plug className="h-5 w-5 text-primary" />
            {title}
          </h3>
          <button
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="p-6 space-y-4 text-sm">
          <p className="text-muted-foreground">
            Add this server to your AI agent (e.g. Claude Code) with one command:
          </p>

          <div className="rounded-lg border border-border/50 bg-muted/10 p-3">
            <div className="flex items-start gap-2">
              <code className="font-mono text-xs break-all flex-1 select-all">{command}</code>
              <button
                type="button"
                onClick={() => copy(command, 'cmd')}
                className="p-1 rounded text-muted-foreground hover:text-foreground shrink-0"
                aria-label="Copy command"
                title="Copy command"
              >
                {copied === 'cmd' ? (
                  <Check className="h-4 w-4 text-success" />
                ) : (
                  <Copy className="h-4 w-4" />
                )}
              </button>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground w-20 shrink-0">Endpoint</span>
              <code className="font-mono text-xs break-all flex-1 select-all">{connectUrl}</code>
              <button
                type="button"
                onClick={() => copy(connectUrl, 'url')}
                className="p-1 rounded text-muted-foreground hover:text-foreground shrink-0"
                aria-label="Copy endpoint"
              >
                {copied === 'url' ? (
                  <Check className="h-3.5 w-3.5 text-success" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
              </button>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground w-20 shrink-0">Token</span>
              {token ? (
                <>
                  <code className="font-mono text-xs break-all flex-1 select-all">{token}</code>
                  <button
                    type="button"
                    onClick={() => copy(token, 'tok')}
                    className="p-1 rounded text-muted-foreground hover:text-foreground shrink-0"
                    aria-label="Copy token"
                  >
                    {copied === 'tok' ? (
                      <Check className="h-3.5 w-3.5 text-success" />
                    ) : (
                      <Copy className="h-3.5 w-3.5" />
                    )}
                  </button>
                </>
              ) : (
                <span className="text-muted-foreground flex-1">{tokenHint}</span>
              )}
            </div>
          </div>

          <p className="text-xs text-muted-foreground">
            The port is published on 127.0.0.1. To connect from another machine, forward it over
            SSH (<code className="font-mono">ssh -L {port}:127.0.0.1:{port} user@host</code>) or
            expose the port, then use that host in the endpoint above.
          </p>
          <p className="text-xs text-muted-foreground">
            Then run <code className="font-mono">/mcp</code> in your agent to connect.
          </p>
        </div>

        <div className="flex justify-end p-6 pt-0">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 rounded-lg text-sm bg-muted/20 hover:bg-muted/30"
          >
            Done
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
};
