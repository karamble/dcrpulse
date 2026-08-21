// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { FileText, X } from 'lucide-react';
import { isImageMime } from './embedParser';
import { formatBytes } from '../../utils/bytes';

export interface StagedAttachment {
  file: File;
  mode: 'inline' | 'transfer';
  dataB64?: string;
}

export const ChatAttachmentPreview = ({
  attachment,
  onRemove,
  transfer,
}: {
  attachment: StagedAttachment;
  onRemove: () => void;
  transfer?: { pct: number; phase: 'upload' | 'relay' } | null;
}) => {
  const mime = attachment.file.type || 'application/octet-stream';
  const showImage = attachment.mode === 'inline' && isImageMime(mime) && attachment.dataB64;
  const modeLabel = attachment.mode === 'inline' ? 'inline embed' : 'file transfer';
  const uploading = transfer != null;
  const relaying = transfer?.phase === 'relay';
  const transferLabel = relaying
    ? `Sending ${transfer?.pct ?? 0}%`
    : `Uploading ${transfer?.pct ?? 0}%`;
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded-md border border-border/40 bg-muted/10">
      {showImage ? (
        <img
          src={`data:${mime};base64,${attachment.dataB64}`}
          alt={attachment.file.name}
          className="h-10 w-10 rounded object-cover"
        />
      ) : (
        <FileText className="h-5 w-5 text-muted-foreground shrink-0" />
      )}
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{attachment.file.name}</p>
        {uploading ? (
          <>
            <p className="text-[10px] text-muted-foreground">{transferLabel}</p>
            <div className="mt-1 h-1 w-full overflow-hidden rounded bg-muted/30">
              <div
                className="h-full bg-primary transition-[width] duration-150"
                style={{ width: `${transfer?.pct ?? 0}%` }}
              />
            </div>
          </>
        ) : (
          <p className="text-[10px] text-muted-foreground">
            {mime} · {formatBytes(attachment.file.size)} · {modeLabel}
          </p>
        )}
      </div>
      {!uploading && (
        <button
          type="button"
          onClick={onRemove}
          className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
          title="Remove attachment"
        >
          <X className="h-4 w-4" />
        </button>
      )}
    </div>
  );
};
