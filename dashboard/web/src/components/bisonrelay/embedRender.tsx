// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Download, FileText } from 'lucide-react';
import { EmbedSegment, embedFileUrl, formatBytes, isImageMime } from './embedParser';
import { QuoteEmbedCard } from './QuoteEmbedCard';

// ImageViewerOpenFn opens the shared image lightbox. Callers supply their own
// opener: the chat reads it from context, the feed wraps its viewer state.
export interface ImageViewerOpenFn {
  (src: string, name: string, mime: string): void;
}

// EmbedRenderer renders one parsed --embed[...]-- segment: an inline-base64 or
// local-file image becomes a click-to-zoom <img>; anything else is a download
// chip. Shared by the chat message body and post comments.
// MAX_INLINE_RENDER_BYTES bounds how large an inline base64 image we decode
// into an <img>; a larger payload (e.g. a crafted comment) renders as a
// download chip instead of allocating a huge bitmap.
const MAX_INLINE_RENDER_BYTES = 4 * 1024 * 1024;

export const EmbedRenderer = ({
  embed,
  openViewer,
}: {
  embed: EmbedSegment;
  openViewer?: ImageViewerOpenFn | null;
}) => {
  if (embed.mime === 'quote' && embed.quoteFrom && embed.quotePost) {
    return <QuoteEmbedCard from={embed.quoteFrom} post={embed.quotePost} alt={embed.alt} />;
  }
  const inlineUrl = embed.dataB64 ? `data:${embed.mime};base64,${embed.dataB64}` : '';
  const fileUrl = inlineUrl || embedFileUrl(embed.localFilename);
  if (!fileUrl) {
    return (
      <p className="text-[11px] text-muted-foreground italic">
        [attachment {embed.name || embed.filename || 'unnamed'} not available]
      </p>
    );
  }
  const inlineBytes = embed.dataB64 ? Math.floor((embed.dataB64.length * 3) / 4) : 0;
  if (isImageMime(embed.mime) && inlineBytes <= MAX_INLINE_RENDER_BYTES) {
    const displayName = embed.name || embed.filename || 'image';
    return (
      <button
        type="button"
        onClick={() => openViewer?.(fileUrl, displayName, embed.mime)}
        className="block p-0 border-0 bg-transparent cursor-zoom-in"
      >
        <img
          src={fileUrl}
          alt={embed.alt || displayName}
          loading="lazy"
          className="max-h-72 max-w-full rounded border border-border/40 object-contain bg-background/40"
        />
      </button>
    );
  }
  return <NonImageEmbed embed={embed} fileUrl={fileUrl} />;
};

const NonImageEmbed = ({ embed, fileUrl }: { embed: EmbedSegment; fileUrl: string }) => {
  const filename = embed.name || embed.filename || 'attachment';
  const bytes = embed.dataB64 ? Math.floor((embed.dataB64.length * 3) / 4) : embed.size;
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded border border-border/40 bg-background/40">
      <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">{filename}</p>
        <p className="text-[10px] text-muted-foreground">
          {embed.mime || 'unknown'}{bytes ? ' · ' + formatBytes(bytes) : ''}
        </p>
      </div>
      <a
        href={fileUrl}
        download={filename}
        className="p-1 rounded hover:bg-muted/30 text-muted-foreground hover:text-foreground transition-colors"
        title="Save"
      >
        <Download className="h-4 w-4" />
      </a>
    </div>
  );
};
