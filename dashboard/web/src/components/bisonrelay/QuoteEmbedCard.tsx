// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Loader2, Quote } from 'lucide-react';
import {
  BisonrelayPostBodySegment,
  bisonrelayPostEmbedUrl,
  fetchBisonrelayUserPost,
  getBisonrelayPostBody,
} from '../../services/bisonrelayApi';
import { isImageMime, parseEmbeds } from './embedParser';

// QuoteEmbedCard renders a quote-by-reference embed
// (--embed[type=quote,from=,post=]--, docs/features/bison-relay-quote-embed.md)
// as a compact quoted-post card. Post bodies arrive pre-resolved from the
// backend (resolved prop); comments and chat resolve here against the LOCAL
// store only. An unavailable quote never invents content: it shows the alt
// text and offers a user-initiated fetch, never an automatic one.
export interface ResolvedQuote {
  available: boolean;
  author_nick?: string;
  title?: string;
  snippet?: string;
  image_index?: number;
  image_mime?: string;
}

// firstImageIndex finds the quoted post's first inline image and its index
// in the post's embed order (the index the embed-data endpoint serves).
// Only embed segments count toward the index, matching the backend.
const firstImageIndex = (markdown: string): { index: number; mime: string } | null => {
  let ordinal = 0;
  for (const seg of parseEmbeds(markdown)) {
    if (seg.kind !== 'embed') continue;
    if (isImageMime(seg.mime) && seg.dataB64) return { index: ordinal, mime: seg.mime };
    ordinal++;
  }
  return null;
};

// stripEmbedTags reduces a quoted body to plain text: quote depth is one,
// so nothing nested is ever resolved or rendered.
const stripEmbedTags = (s: string): string =>
  s
    .replace(/--(embed|download)\[.*?\]--/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

// FeedCardQuote shows a quoting post's quoted content on the feed
// OVERVIEW card. The summary meta only flags that a quote embed exists
// (mime "quote"), so the quoting post's body is fetched once - the backend
// returns it with the quote segments already resolved from the local
// store - and the first quote renders as a card. Renders nothing while
// loading or when the body has no valid quote reference.
export const FeedCardQuote = ({ uid, pid }: { uid: string; pid: string }) => {
  const [seg, setSeg] = useState<BisonrelayPostBodySegment | null>(null);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const body = await getBisonrelayPostBody(uid, pid);
        if (!alive) return;
        const quote = (body.segments ?? []).find((s) => s.quote_from && s.quote_post);
        if (quote) setSeg(quote);
      } catch {
        // Leave the card without a preview; the detail view reports errors.
      }
    })();
    return () => {
      alive = false;
    };
  }, [uid, pid]);

  if (!seg?.quote_from || !seg?.quote_post) return null;
  return (
    <div className="pointer-events-auto">
      <QuoteEmbedCard
        from={seg.quote_from}
        post={seg.quote_post}
        alt={seg.alt}
        resolved={seg.quote ?? { available: false }}
      />
    </div>
  );
};

export const QuoteEmbedCard = ({
  from,
  post,
  alt,
  resolved,
}: {
  from: string;
  post: string;
  alt?: string;
  resolved?: ResolvedQuote | null;
}) => {
  const [selfResolved, setSelfResolved] = useState<ResolvedQuote | null>(null);
  const [loading, setLoading] = useState(!resolved);
  const [fetchState, setFetchState] = useState<'idle' | 'busy' | 'requested'>('idle');
  const [fetchErr, setFetchErr] = useState<string | null>(null);

  useEffect(() => {
    if (resolved) return;
    let alive = true;
    (async () => {
      try {
        const body = await getBisonrelayPostBody(from, post);
        if (!alive) return;
        const img = firstImageIndex(body.markdown || '');
        setSelfResolved({
          available: true,
          author_nick: body.attributes?.from_nick,
          title: body.title,
          snippet: stripEmbedTags(body.markdown || '').slice(0, 240),
          image_index: img?.index,
          image_mime: img?.mime,
        });
      } catch {
        if (alive) setSelfResolved({ available: false });
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [from, post, resolved]);

  const q = resolved ?? selfResolved;
  const openQuoted = () => {
    window.location.hash = `feed/post/${from}/${post}`;
  };
  const requestFetch = async () => {
    setFetchState('busy');
    setFetchErr(null);
    try {
      await fetchBisonrelayUserPost(from, post);
      setFetchState('requested');
    } catch (e: any) {
      // Fails immediately when the author is not a KX'd contact; a
      // delivered request that never answers (author offline, or the
      // author blocked us - indistinguishable on BR by design) stays in
      // the requested state instead.
      const body = e?.response?.data;
      setFetchErr(typeof body === 'string' ? body : e?.message || 'Could not request the post');
      setFetchState('idle');
    }
  };

  if (loading) {
    return (
      <div className="border-l-2 border-primary/70 bg-primary/10 rounded-md px-2 py-1.5 flex items-center gap-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        <span>Loading quoted post...</span>
      </div>
    );
  }

  if (!q?.available) {
    return (
      <div className="border-l-2 border-primary/40 bg-muted/10 rounded-md px-2 py-1.5">
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Quote className="h-3.5 w-3.5 shrink-0" />
          <span className="italic truncate">{alt || 'Quoted post'} - not available</span>
        </div>
        <div className="mt-1">
          {fetchState === 'requested' ? (
            <span className="text-[11px] text-muted-foreground">
              Requested. It arrives if and when the author delivers it.
            </span>
          ) : (
            <button
              type="button"
              onClick={requestFetch}
              disabled={fetchState === 'busy'}
              className="text-[11px] text-primary hover:underline disabled:opacity-50"
            >
              {fetchState === 'busy' ? 'Requesting...' : 'Fetch from author'}
            </button>
          )}
          {fetchErr && <p className="mt-0.5 text-[11px] text-destructive break-words">{fetchErr}</p>}
        </div>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={openQuoted}
      className="block w-full text-left border-l-2 border-primary/70 bg-primary/10 hover:bg-primary/15 transition-colors rounded-md px-2 py-1.5 cursor-pointer"
    >
      <div className="flex items-center gap-1.5 min-w-0">
        <Quote className="h-3.5 w-3.5 shrink-0 text-primary" />
        <span className="text-xs font-semibold text-primary truncate">
          {q.author_nick || 'unknown'}
        </span>
        {q.title ? (
          <span className="text-xs text-muted-foreground truncate">{q.title}</span>
        ) : null}
      </div>
      {q.snippet ? (
        <p className="mt-0.5 text-xs text-foreground/90 line-clamp-3 break-words">{q.snippet}</p>
      ) : null}
      {q.image_index !== undefined && q.image_index !== null ? (
        <img
          src={bisonrelayPostEmbedUrl(from, post, q.image_index)}
          alt=""
          loading="lazy"
          className="mt-1.5 rounded-md border border-border/40 max-h-44 max-w-full object-cover"
        />
      ) : null}
    </button>
  );
};
