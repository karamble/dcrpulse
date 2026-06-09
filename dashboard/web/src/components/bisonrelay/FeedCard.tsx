// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { isImageMime } from './embedParser';
import {
  Atom,
  Eye,
  Image,
  Images,
  Lock,
  MessageCircle,
  Paperclip,
  Repeat2,
} from 'lucide-react';
import {
  BisonrelayPostEmbedMeta,
  BisonrelayPostSummary,
  bisonrelayPostEmbedUrl,
} from '../../services/bisonrelayApi';
import { AuthorAvatar } from './AuthorAvatar';
import { formatDcrFromAtoms, formatDownloadBytes } from './DownloadEmbed';

export const relativeTime = (ts: number): string => {
  if (!ts) return '';
  const now = Math.floor(Date.now() / 1000);
  let delta = now - ts;
  if (delta < 0) delta = 0;
  if (delta < 60) return 'just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
};

// Placeholder titles brclientd derives for posts whose body is only embeds;
// media-first cards suppress these instead of captioning the image with them.
const isPlaceholderTitle = (t: string): boolean =>
  t === '' || t === '(untitled post)' || t === '(image)' || t === '(attachment)';

const FeedCardMedia = ({
  uid,
  pid,
  index,
  alt,
  extra,
}: {
  uid: string;
  pid: string;
  index: number;
  alt?: string;
  extra: number;
}) => {
  const [broken, setBroken] = useState(false);
  if (broken) return null;
  return (
    <div className="relative -mx-4 sm:mx-0 sm:rounded-lg overflow-hidden bg-muted/20 border-y sm:border border-border/40">
      <img
        src={bisonrelayPostEmbedUrl(uid, pid, index)}
        alt={alt || ''}
        loading="lazy"
        decoding="async"
        onError={() => setBroken(true)}
        className="block w-full max-h-80 object-cover"
      />
      {extra > 0 && (
        <span className="absolute bottom-2 right-2 inline-flex items-center gap-1 rounded-md bg-background/80 backdrop-blur-sm px-2 py-0.5 text-[11px] text-foreground border border-border/50">
          <Images className="h-3.5 w-3.5" />
          +{extra}
        </span>
      )}
    </div>
  );
};

// PaidEmbedChip summarizes a file-transfer embed without any way to start the
// (potentially paid) download; that flow stays in the post detail view.
const PaidEmbedChip = ({ embed }: { embed: BisonrelayPostEmbedMeta }) => {
  const name = embed.filename || embed.alt || 'file';
  const isImg = isImageMime(embed.mime);
  const meta = [
    embed.size ? formatDownloadBytes(embed.size) : '',
    embed.cost && embed.cost > 0 ? `${formatDcrFromAtoms(embed.cost)} DCR` : 'free',
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <div className="inline-flex items-center gap-2 rounded-lg border border-border/50 bg-background/40 px-3 py-2 text-xs text-muted-foreground max-w-full">
      {isImg ? <Image className="h-4 w-4 shrink-0" /> : <Paperclip className="h-4 w-4 shrink-0" />}
      <span className="truncate min-w-0 font-mono text-foreground/90">{name}</span>
      <span className="opacity-50">·</span>
      <span className="tabular-nums whitespace-nowrap">{meta}</span>
      <Lock className="h-3.5 w-3.5 shrink-0 opacity-70" />
    </div>
  );
};

export const FeedCard = ({
  post,
  hasActivity,
  avatarB64,
  ownUid,
  onOpen,
}: {
  post: BisonrelayPostSummary;
  hasActivity: boolean;
  avatarB64?: string;
  ownUid: string;
  onOpen: () => void;
}) => {
  const nick = post.author_nick || post.author_id.slice(0, 12);
  const isRelayed = post.relayed ?? (!!post.from && post.from !== post.author_id);
  const relayerNick = post.relayer_nick || (isRelayed ? post.from.slice(0, 12) : '');
  const publishedTs = post.published || post.date;
  const fullDate = publishedTs ? new Date(publishedTs * 1000).toLocaleString() : '';
  const title = post.title || '(untitled post)';

  // The snippet starts with the same first line the title was derived from;
  // drop that prefix so the card reads as headline + remaining body.
  let snippet = (post.snippet ?? '').trim();
  if (post.title && snippet.startsWith(post.title)) {
    snippet = snippet.slice(post.title.length).trim();
  }

  const img = post.first_image && post.first_image.has_data ? post.first_image : null;
  const imageEmbeds = (post.embeds ?? []).filter(
    (e) => isImageMime(e.mime) && e.has_data,
  );
  const extraImages = img ? Math.max(0, imageEmbeds.length - 1) : 0;
  const paidEmbed = (post.embeds ?? []).find((e) => e.download && !e.has_data);
  const mediaFirst = !!img && !snippet && isPlaceholderTitle(post.title || '');

  const hearts = post.hearts_count ?? 0;
  const comments = post.comments_count ?? 0;
  const receipts = post.receipt_count ?? 0;
  const isOwn = !!ownUid && post.author_id === ownUid;

  return (
    <article className="relative rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden transition-colors hover:border-primary/30 focus-within:border-primary/40">
      <button
        type="button"
        onClick={onOpen}
        aria-label={`Open post: ${title}`}
        className="absolute inset-0 z-0 w-full rounded-xl focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/50"
      />
      {/* Content paints above the stretched button; pointer-events-none lets
          every click fall through to it (the whole card opens the post). */}
      <div className="relative z-10 p-4 space-y-3 pointer-events-none">
        <div className="flex items-center gap-3">
          <AuthorAvatar uid={post.author_id} nick={nick} avatarB64={avatarB64} size="md" />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5 flex-wrap">
              <span className="text-sm font-semibold text-foreground truncate max-w-full">{nick}</span>
              {isRelayed && relayerNick && (
                <span className="inline-flex items-center gap-1 text-[11px] text-muted-foreground">
                  <Repeat2 className="h-3.5 w-3.5 shrink-0" />
                  relayed by {relayerNick}
                </span>
              )}
            </div>
            <div className="text-[11px] text-muted-foreground" title={fullDate}>
              {relativeTime(publishedTs)}
            </div>
          </div>
          {hasActivity && (
            <span
              className="h-2 w-2 rounded-full bg-primary shrink-0"
              aria-hidden
              title="New activity since you last opened this post"
            />
          )}
        </div>

        {!mediaFirst && (
          <h3 className="text-base font-semibold text-foreground leading-snug line-clamp-2 break-words">
            {title}
          </h3>
        )}

        {post.description && post.description.trim() && (
          <p className="text-xs text-muted-foreground/90 line-clamp-1 break-words">
            {post.description}
          </p>
        )}

        {snippet && (
          <p className="text-sm text-foreground/80 line-clamp-3 break-words">{snippet}</p>
        )}

        {img && (
          <FeedCardMedia
            uid={post.author_id}
            pid={post.id}
            index={img.index}
            alt={img.alt}
            extra={extraImages}
          />
        )}

        {paidEmbed && <PaidEmbedChip embed={paidEmbed} />}

        <div className="flex items-center gap-4 pt-0.5 text-[11px] min-w-0">
          <span
            className={`inline-flex items-center gap-1.5 shrink-0 ${
              post.hearted_by_me ? 'text-primary' : 'text-muted-foreground'
            }`}
            title={post.hearted_by_me ? 'You atomed this post' : 'Atoms'}
          >
            <Atom className="h-4 w-4" strokeWidth={post.hearted_by_me ? 2.5 : 2} />
            <span className="tabular-nums">{hearts}</span>
          </span>
          <span
            className="inline-flex items-center gap-1.5 shrink-0 text-muted-foreground"
            title="Comments"
          >
            <MessageCircle className="h-4 w-4" />
            <span className="tabular-nums">{comments}</span>
          </span>
          {isOwn && receipts > 0 && (
            <span
              className="inline-flex items-center gap-1.5 shrink-0 text-muted-foreground"
              title={`Seen by ${receipts} subscriber${receipts === 1 ? '' : 's'}`}
            >
              <Eye className="h-4 w-4" />
              <span className="tabular-nums">{receipts}</span>
            </span>
          )}
          {hasActivity && post.last_status_ts ? (
            <span className="text-primary/80 truncate min-w-0">
              Last activity {relativeTime(post.last_status_ts)}
            </span>
          ) : comments > 0 && post.last_comment_nick && post.last_comment_ts ? (
            <span className="text-muted-foreground truncate min-w-0">
              {post.last_comment_nick} commented {relativeTime(post.last_comment_ts)}
            </span>
          ) : null}
        </div>
      </div>
    </article>
  );
};

export const FeedCardSkeleton = () => (
  <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-4 space-y-3 animate-pulse">
    <div className="flex items-center gap-3">
      <div className="h-10 w-10 rounded-full bg-muted/40" />
      <div className="flex-1 space-y-2">
        <div className="h-3 w-32 rounded bg-muted/40" />
        <div className="h-2.5 w-20 rounded bg-muted/30" />
      </div>
    </div>
    <div className="h-4 w-3/4 rounded bg-muted/40" />
    <div className="space-y-2">
      <div className="h-3 w-full rounded bg-muted/30" />
      <div className="h-3 w-5/6 rounded bg-muted/30" />
    </div>
    <div className="flex gap-4">
      <div className="h-3 w-10 rounded bg-muted/30" />
      <div className="h-3 w-10 rounded bg-muted/30" />
    </div>
  </div>
);
