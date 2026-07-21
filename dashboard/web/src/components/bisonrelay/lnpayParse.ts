// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Matches an lnpay://<bolt11> link (the BR pay-link convention) or a bare
// bolt11 invoice pasted into chat. The HRPs cover dcrlnd's mainnet, testnet,
// simnet and regnet (lndcr/lntdcr/lnsdcr/lnrdcr); the long bech32 tail keeps
// ordinary prose from matching (real invoices run 200+ characters).
const LN_INVOICE_RE = /(?:lnpay:\/\/)?\b(ln[tsr]?dcr[0-9][a-z0-9]{50,})/gi;

export type LnPayPart =
  | { kind: 'text'; text: string }
  | { kind: 'invoice'; invoice: string };

// splitLnInvoices splits a chat text segment around Lightning invoices so the
// caller can interleave prose with pay chips. The invoice part carries the
// bare bolt11 string (lnpay:// scheme stripped), which is what the decode and
// pay endpoints expect. Returns a single text part when there is no invoice.
export const splitLnInvoices = (text: string): LnPayPart[] => {
  const re = new RegExp(LN_INVOICE_RE);
  const parts: LnPayPart[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push({ kind: 'text', text: text.slice(last, m.index) });
    parts.push({ kind: 'invoice', invoice: m[1] });
    last = m.index + m[0].length;
  }
  if (parts.length === 0) return [{ kind: 'text', text }];
  if (last < text.length) parts.push({ kind: 'text', text: text.slice(last) });
  return parts;
};
