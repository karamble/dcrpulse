// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { AlertCircle, ChevronDown, ChevronRight, Loader2, Rss } from 'lucide-react';
import {
  BisonrelayLiveEvent,
  BisonrelayPostBody,
  BisonrelayPostBodySegment,
  BisonrelayPostSummary,
  getBisonrelayPostBody,
  getBisonrelayPosts,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

// readHashFeedTarget parses the URL hash to extract a deep-linked
// (author_id, post_id) pair set by the PostsListModal when the user
// clicks a post to fetch + open it.
const readHashFeedTarget = (): { uid: string; pid: string } | null => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('feed/')) return null;
  const parts = h.slice('feed/'.length).split('/');
  if (parts.length !== 2 || !parts[0] || !parts[1]) return null;
  return { uid: parts[0], pid: parts[1] };
};

// BisonrelayFeed renders the aggregate list of posts received from
// subscribed-to users (and our own, once we author them). Bodies are
// fetched lazily on expand; the dashboard renders the markdown to
// sanitized HTML server-side using the same goldmark+bluemonday policy
// the Politeia proposal viewer uses.
export const BisonrelayFeed = () => {
  const [posts, setPosts] = useState<BisonrelayPostSummary[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [target, setTarget] = useState<{ uid: string; pid: string } | null>(readHashFeedTarget);
  const { addListener } = useBisonrelayLive();

  const reload = useCallback(async () => {
    try {
      const list = await getBisonrelayPosts();
      list.sort((a, b) => b.date - a.date);
      setPosts(list);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load posts');
    }
  }, []);

  useEffect(() => {
    reload();
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'post-received') return;
      reload();
    });
  }, [addListener, reload]);

  // React to hash changes (the modal navigates to feed/<uid>/<pid> when
  // the user clicks a post). Also covers manual URL edits.
  useEffect(() => {
    const onHashChange = () => setTarget(readHashFeedTarget());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const targetMissing =
    target !== null &&
    (posts === null || !posts.some((p) => p.author_id === target.uid && p.id === target.pid));

  return (
    <div className="space-y-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {targetMissing && (
        <div className="rounded-lg bg-muted/20 border border-border/40 px-3 py-2 text-xs text-muted-foreground flex items-center gap-2">
          <Loader2 className="h-3 w-3 animate-spin shrink-0" />
          <span>
            Waiting for the requested post to arrive… the author needs to be
            online for the body to be delivered.
          </span>
        </div>
      )}
      {posts === null && !err ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading posts…</span>
        </div>
      ) : posts && posts.length === 0 ? (
        <EmptyFeed />
      ) : (
        posts?.map((p) => (
          <FeedCard
            key={`${p.author_id}-${p.id}`}
            post={p}
            forceExpanded={
              target !== null && target.uid === p.author_id && target.pid === p.id
            }
          />
        ))
      )}
    </div>
  );
};

const EmptyFeed = () => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-start gap-3">
    <div className="p-2 rounded-lg bg-primary/10 border border-primary/20 shrink-0">
      <Rss className="h-5 w-5 text-primary" />
    </div>
    <div className="space-y-1">
      <h3 className="text-sm font-semibold">No posts yet</h3>
      <p className="text-xs text-muted-foreground">
        Subscribe to a contact's posts from their sub-nav (click their avatar in
        Chat) and new posts will land here as they publish.
      </p>
    </div>
  </div>
);

const FeedCard = ({
  post,
  forceExpanded,
}: {
  post: BisonrelayPostSummary;
  forceExpanded?: boolean;
}) => {
  const [expanded, setExpanded] = useState(!!forceExpanded);
  const [body, setBody] = useState<BisonrelayPostBody | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const loadBody = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const b = await getBisonrelayPostBody(post.author_id, post.id);
      setBody(b);
    } catch (e: any) {
      const respBody = e?.response?.data;
      setErr(typeof respBody === 'string' ? respBody : e?.message || 'Could not load post body');
    } finally {
      setLoading(false);
    }
  }, [post.author_id, post.id]);

  // When the parent transitions forceExpanded to true (deep-link
  // arrival), expand + fetch.
  useEffect(() => {
    if (forceExpanded && !expanded) {
      setExpanded(true);
    }
    if (forceExpanded && body === null && !loading) {
      loadBody();
    }
  }, [forceExpanded, expanded, body, loading, loadBody]);

  const onToggle = () => {
    const next = !expanded;
    setExpanded(next);
    if (next && body === null && !loading) {
      loadBody();
    }
  };

  const dateStr = new Date(post.date * 1000).toLocaleString();

  return (
    <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden">
      <button
        type="button"
        onClick={onToggle}
        className="w-full px-4 py-3 flex items-start gap-3 text-left hover:bg-muted/20 transition-colors"
      >
        <div className="mt-1 shrink-0 text-muted-foreground">
          {expanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </div>
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold text-foreground truncate">
            {post.title || '(untitled post)'}
          </div>
          <div className="text-[11px] text-muted-foreground mt-0.5">
            {post.author_nick || post.author_id.slice(0, 12)}
            <span className="mx-1.5 opacity-50">·</span>
            {dateStr}
          </div>
        </div>
      </button>
      {expanded && (
        <div className="px-4 pb-4 pt-1 border-t border-border/30">
          {loading ? (
            <div className="flex items-center gap-2 text-xs text-muted-foreground py-3">
              <Loader2 className="h-4 w-4 animate-spin" />
              <span>Loading body…</span>
            </div>
          ) : err ? (
            <div className="flex items-start gap-2 text-sm text-destructive py-3">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          ) : body && body.segments && body.segments.length > 0 ? (
            <PostBodySegments segments={body.segments} />
          ) : body ? (
            <p className="text-xs text-muted-foreground py-3 italic">
              (Empty post)
            </p>
          ) : null}
        </div>
      )}
    </div>
  );
};

const PROSE_CLASSES =
  'prose prose-sm prose-invert max-w-none text-sm text-foreground/90 break-words ' +
  '[&_a]:text-primary [&_a]:underline [&_a:hover]:no-underline ' +
  '[&_pre]:bg-muted/30 [&_pre]:rounded [&_pre]:p-3 [&_code]:font-mono ' +
  '[&_blockquote]:border-l-2 [&_blockquote]:border-border/60 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground';

const PostBodySegments = ({ segments }: { segments: BisonrelayPostBodySegment[] }) => (
  <div className="mt-2 space-y-3">
    {segments.map((seg, i) => {
      if (seg.kind === 'text' && seg.html) {
        return (
          <div
            key={i}
            className={PROSE_CLASSES}
            dangerouslySetInnerHTML={{ __html: seg.html }}
          />
        );
      }
      if (seg.kind === 'embed' && seg.data_b64) {
        const isImage = !!seg.mime && seg.mime.startsWith('image/');
        if (isImage) {
          return (
            <img
              key={i}
              src={`data:${seg.mime};base64,${seg.data_b64}`}
              alt={seg.alt || seg.name || ''}
              className="rounded-lg border border-border/40 max-w-full h-auto"
            />
          );
        }
        // Non-image embed: download link with a data: URL.
        const href = `data:${seg.mime || 'application/octet-stream'};base64,${seg.data_b64}`;
        return (
          <a
            key={i}
            href={href}
            download={seg.name || 'attachment'}
            className="inline-block text-xs text-primary underline hover:no-underline"
          >
            {seg.name || 'attachment'} ({seg.mime || 'binary'})
          </a>
        );
      }
      return null;
    })}
  </div>
);
