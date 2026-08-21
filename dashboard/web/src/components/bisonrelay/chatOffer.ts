// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { BisonrelaySharedFile } from '../../services/bisonrelayApi';

// A file picked from disk: the bytes travel, either inline in the message body
// or pushed to the peer over BR's direct file transfer. Neither can be priced.
export interface StagedFile {
  mode: 'inline' | 'transfer';
  file: File;
  dataB64?: string;
  // The pick before ImageAttachModal re-encoded it. An offer sells this one.
  origFile?: File;
}

// A shared-file offer: the message carries only a reference, and the peer pays
// and pulls the bytes itself. Pricing lives on the share record, so this is the
// only staged shape that can carry one.
export interface StagedOffer {
  mode: 'offer';
  fid: string;
  // The name BR stored. brclientd sanitises an upload's filename and suffixes
  // collisions, so this is not always the name the browser sent.
  filename: string;
  size: number;
  mime: string;
  cost: number; // atoms (1 DCR = 1e8)
  scope: 'global' | 'targeted';
  targetUid: string; // '' when global
  // True only when this composer registered the share, which gates the unshare
  // on remove. False for anything that was already shared.
  createdHere: boolean;
}

export type StagedAttachment = StagedFile | StagedOffer;

// The dashboard's /br/files/add handler caps the body at the same figure.
export const MAX_OFFER_BYTES = 200 << 20;

// Readers split the tag on ',' and take the first '=', so either character
// inside a value truncates every field after it.
const clean = (s: string): string => s.replace(/[,=]/g, '');

// offerTagFor renders the BR wire tag for a staged offer. Field order follows
// mdembeds' EmbeddedArgs.String; BR itself does not depend on it.
export const offerTagFor = (o: StagedOffer): string => {
  const parts = [`download=${clean(o.fid)}`];
  if (o.filename) parts.push(`filename=${clean(o.filename)}`);
  if (o.mime) parts.push(`type=${clean(o.mime)}`);
  if (o.size > 0) parts.push(`size=${o.size}`);
  if (o.cost > 0) parts.push(`cost=${o.cost}`);
  return `--embed[${parts.join(',')}]--`;
};

// canOfferTo reports whether BR would serve this file to the conversation.
// A fetch is answered only for a globally shared file or one shared with that
// exact uid, so a group chat, which has no share of its own, can only use a
// global one. targetUid is null for a group.
export const canOfferTo = (f: BisonrelaySharedFile, targetUid: string | null): boolean => {
  if (f.global) return true;
  if (!targetUid) return false;
  const want = targetUid.toLowerCase();
  return (f.shares ?? []).some((uid) => uid.toLowerCase() === want);
};
