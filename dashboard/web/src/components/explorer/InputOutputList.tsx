// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { TxInput, TxOutput } from '../../services/explorerApi';
import { CopyButton } from './CopyButton';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { Link } from 'react-router-dom';

interface InputOutputListProps {
  inputs: TxInput[];
  outputs: TxOutput[];
}

export const InputOutputList = ({ inputs, outputs }: InputOutputListProps) => {
  const formatDCR = (amount: number) => {
    return amount.toFixed(8);
  };

  const getScriptTypeColor = (type: string) => {
    if (type.includes('stake')) return 'text-warning';
    if (type.includes('pubkey')) return 'text-primary';
    if (type.includes('scripthash')) return 'text-blue-500';
    if (type === 'nulldata') return 'text-muted-foreground';
    return 'text-foreground';
  };

  const totalInputs = inputs.reduce((sum, input) => sum + input.amountIn, 0);
  const totalOutputs = outputs.reduce((sum, output) => sum + output.value, 0);

  return (
    <div className="space-y-6">
      {/* Inputs */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <ArrowDown className="h-5 w-5 text-red-500" />
            <h3 className="text-lg font-semibold">Inputs ({inputs.length})</h3>
          </div>
          <div className="text-sm">
            <span className="text-muted-foreground">Total: </span>
            <span className="font-semibold">{formatDCR(totalInputs)} DCR</span>
          </div>
        </div>

        <div className="space-y-3">
          {inputs.map((input, idx) => (
            <div
              key={idx}
              className="p-4 rounded-lg bg-background/50 border border-border/30"
            >
              <div className="flex items-start justify-between mb-3">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs font-medium text-muted-foreground">Input #{idx}</span>
                  </div>

                  {input.coinbase ? (
                    <div className="text-sm">
                      <span className="font-mono text-purple-500">Coinbase</span>
                      <p className="text-xs text-muted-foreground mt-1">{input.coinbase}</p>
                    </div>
                  ) : input.stakebase ? (
                    <div className="text-sm">
                      <span className="font-mono text-success">Stakebase</span>
                      <p className="text-xs text-muted-foreground mt-1">{input.stakebase}</p>
                    </div>
                  ) : (
                    <>
                      <div className="flex items-center gap-2 mb-2">
                        <Link 
                          to={`/explorer/tx/${input.prevTxid}`}
                          className="font-mono text-sm break-all text-primary hover:text-primary/80 hover:underline"
                        >
                          {input.prevTxid}
                        </Link>
                        <CopyButton text={input.prevTxid || ''} />
                      </div>
                      <div className="text-xs text-muted-foreground">
                        Output #{input.vout} • Tree {input.tree}
                      </div>
                      {input.address && (
                        <div className="mt-2 flex items-center gap-2">
                          <Link
                            to={`/explorer/address/${input.address}`}
                            className="text-sm font-mono text-primary hover:text-primary/80 hover:underline"
                          >
                            {input.address}
                          </Link>
                          <CopyButton text={input.address} />
                        </div>
                      )}
                    </>
                  )}
                </div>

                <div className="text-right ml-4">
                  <div className="text-lg font-semibold text-red-500">
                    {formatDCR(input.amountIn)} DCR
                  </div>
                  {input.blockHeight > 0 && (
                    <div className="text-xs text-muted-foreground">
                      Block {input.blockHeight.toLocaleString()}
                    </div>
                  )}
                </div>
              </div>

              {input.scriptSig && (
                <details className="mt-2">
                  <summary className="text-xs text-muted-foreground cursor-pointer hover:text-foreground">
                    Script Signature
                  </summary>
                  <pre className="mt-2 p-2 rounded bg-muted/20 text-xs font-mono overflow-auto">
                    {input.scriptSig}
                  </pre>
                </details>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Outputs */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <ArrowUp className="h-5 w-5 text-success" />
            <h3 className="text-lg font-semibold">Outputs ({outputs.length})</h3>
          </div>
          <div className="text-sm">
            <span className="text-muted-foreground">Total: </span>
            <span className="font-semibold">{formatDCR(totalOutputs)} DCR</span>
          </div>
        </div>

        <div className="space-y-3">
          {outputs.map((output, idx) => (
            <div
              key={idx}
              className="p-4 rounded-lg bg-background/50 border border-border/30"
            >
              <div className="flex items-start justify-between mb-3">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="text-xs font-medium text-muted-foreground">Output #{output.index}</span>
                    <span className={`text-xs font-medium ${getScriptTypeColor(output.scriptPubKey.type)}`}>
                      {output.scriptPubKey.type}
                    </span>
                    {output.spent && (
                      <span className="text-xs px-2 py-0.5 rounded bg-red-500/10 text-red-500">
                        Spent
                      </span>
                    )}
                  </div>

                  {output.scriptPubKey.addresses && output.scriptPubKey.addresses.length > 0 ? (
                    <div className="space-y-1">
                      {output.scriptPubKey.addresses.map((address, addrIdx) => (
                        <div key={addrIdx} className="flex items-center gap-2">
                          <Link
                            to={`/explorer/address/${address}`}
                            className="text-sm font-mono break-all text-primary hover:text-primary/80 hover:underline"
                          >
                            {address}
                          </Link>
                          <CopyButton text={address} />
                        </div>
                      ))}
                    </div>
                  ) : output.scriptPubKey.type === 'nulldata' ? (
                    <div className="text-xs text-muted-foreground">
                      OP_RETURN (data output)
                    </div>
                  ) : (
                    <div className="text-xs text-muted-foreground">
                      No address (non-standard script)
                    </div>
                  )}

                  {output.spentBy && (
                    <div className="mt-2 text-xs">
                      <span className="text-muted-foreground">Spent by: </span>
                      <Link
                        to={`/explorer/tx/${output.spentBy}`}
                        className="font-mono text-primary hover:text-primary/80 hover:underline"
                      >
                        {output.spentBy.substring(0, 16)}...
                      </Link>
                    </div>
                  )}
                </div>

                <div className="text-right ml-4">
                  <div className="text-lg font-semibold text-success">
                    {formatDCR(output.value)} DCR
                  </div>
                  <div className="text-xs text-muted-foreground">
                    v{output.version}
                  </div>
                </div>
              </div>

              <details className="mt-2">
                <summary className="text-xs text-muted-foreground cursor-pointer hover:text-foreground">
                  Script Details
                </summary>
                <div className="mt-2 space-y-2">
                  <div>
                    <p className="text-xs text-muted-foreground mb-1">ASM:</p>
                    <pre className="p-2 rounded bg-muted/20 text-xs font-mono overflow-auto">
                      {output.scriptPubKey.asm}
                    </pre>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground mb-1">Hex:</p>
                    <pre className="p-2 rounded bg-muted/20 text-xs font-mono overflow-auto">
                      {output.scriptPubKey.hex}
                    </pre>
                  </div>
                </div>
              </details>
            </div>
          ))}
        </div>
      </div>

      {/* Fee Calculation */}
      {totalInputs > 0 && (
        <div className="p-4 rounded-lg bg-gradient-to-r from-muted/10 to-muted/5 border border-border/30">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-muted-foreground">Transaction Fee</p>
              <p className="text-xs text-muted-foreground mt-1">
                Inputs ({formatDCR(totalInputs)}) - Outputs ({formatDCR(totalOutputs)})
              </p>
            </div>
            <div className="text-right">
              <p className="text-xl font-bold text-warning">
                {formatDCR(totalInputs - totalOutputs)} DCR
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

