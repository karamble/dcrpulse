// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  ArrowLeft,
  Atom,
  Coins,
  Edit,
  Loader2,
  Repeat2,
  Reply,
  Rss,
  Users,
  Send,
  FileText,
} from 'lucide-react';
import { BR_PROSE_CLASSES } from './bisonrelayProse';
import { useBrNotifPrefs } from './brNotifPrefs';
import { AuthorAvatar } from './AuthorAvatar';
import { FeedCard, FeedCardSkeleton } from './FeedCard';
import {
  BisonrelayEditor,
  EditorEmbedMap,
  composeBRBody,
  isEditorOverHardCap,
} from './editor';
import {
  BisonrelayContact,
  BisonrelayLiveEvent,
  BisonrelayPostBody,
  BisonrelayPostBodySegment,
  BisonrelayPostComment,
  BisonrelayPostSummary,
  BisonrelayReceiveReceipt,
  createBisonrelayPost,
  getBisonrelayContacts,
  getBisonrelayIdentity,
  getBisonrelayPostBody,
  getBisonrelayPostCommentReceipts,
  getBisonrelayPostComments,
  getBisonrelayPostHearts,
  getBisonrelayPostReceiveReceipts,
  getBisonrelayPosts,
  getBisonrelayStatsPosts,
  heartBisonrelayPost,
  postBisonrelayComment,
  relayBisonrelayPost,
  subscribeBisonrelayPosts,
  tipBisonrelayContact,
  unsubscribeBisonrelayPosts,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { DownloadEmbed } from './DownloadEmbed';
import { ImageViewerModal, ViewerImage } from './ImageViewerModal';
import { TipModal } from './TipModal';
import { UserProfileView } from './BisonrelayUserProfile';

type Section = 'list' | 'yours' | 'subs' | 'new' | 'detail' | 'user';

interface FeedTarget {
  uid: string;
  pid: string;
}

const activityTs = (p: BisonrelayPostSummary): number =>
  p.last_status_ts && p.last_status_ts > p.date ? p.last_status_ts : p.date;

const hasNewActivity = (p: BisonrelayPostSummary, seenTs: number | undefined): boolean => {
  if (!p.last_status_ts) return false;
  const watermark = seenTs && seenTs > p.date ? seenTs : p.date;
  return p.last_status_ts > watermark;
};

const FEED_SEEN_STORAGE = 'dcrpulse.br.feed-seen';

const FEED_PAGE_SIZE = 15;

const loadSeenMap = (): Record<string, number> => {
  try {
    const raw = localStorage.getItem(FEED_SEEN_STORAGE);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
};

const persistSeenMap = (m: Record<string, number>): void => {
  try {
    localStorage.setItem(FEED_SEEN_STORAGE, JSON.stringify(m));
  } catch {
    /* ignore */
  }
};

// brclientd's /public-identity returns the identity base64-encoded while the
// posts feed keys authors by hex; convert so own-post checks match.
const identityB64ToHex = (b64: string): string => {
  try {
    const bin = atob(b64);
    let hex = '';
    for (let i = 0; i < bin.length; i++) {
      hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    }
    return hex;
  } catch {
    return '';
  }
};

// readHashRoute parses the URL hash into a (section, target?) tuple.
// Hash format:
//   #feed                          -> list (default landing)
//   #feed/yours                    -> your-posts
//   #feed/subs                     -> subscriptions
//   #feed/new                      -> composer
//   #feed/post/<uid>/<pid>         -> post detail
const readHashRoute = (): { section: Section; target: FeedTarget | null } => {
  const h = window.location.hash.replace(/^#/, '');
  if (!h.startsWith('feed')) return { section: 'list', target: null };
  const rest = h.slice('feed'.length);
  if (rest === '' || rest === '/') return { section: 'list', target: null };
  if (rest === '/yours') return { section: 'yours', target: null };
  if (rest === '/subs') return { section: 'subs', target: null };
  if (rest === '/new') return { section: 'new', target: null };
  if (rest.startsWith('/post/')) {
    const parts = rest.slice('/post/'.length).split('/');
    if (parts.length === 2 && parts[0] && parts[1]) {
      return { section: 'detail', target: { uid: parts[0], pid: parts[1] } };
    }
  }
  if (rest.startsWith('/user/')) {
    const u = rest.slice('/user/'.length);
    if (u) return { section: 'user', target: { uid: u, pid: '' } };
  }
  return { section: 'list', target: null };
};

const navigateTo = (hash: string): void => {
  window.location.hash = hash;
};

export const BisonrelayFeed = () => {
  const [route, setRoute] = useState(readHashRoute);
  const [posts, setPosts] = useState<BisonrelayPostSummary[] | null>(null);
  const [postsErr, setPostsErr] = useState<string | null>(null);
  const [seen, setSeen] = useState<Record<string, number>>(loadSeenMap);
  const [ownUid, setOwnUid] = useState<string>('');
  const [avatars, setAvatars] = useState<Record<string, string>>({});
  const { addListener } = useBisonrelayLive();

  const markSeen = useCallback((key: string, ts: number) => {
    if (!ts) return;
    setSeen((prev) => {
      if ((prev[key] ?? 0) >= ts) return prev;
      const next = { ...prev, [key]: ts };
      persistSeenMap(next);
      return next;
    });
  }, []);

  const reload = useCallback(async () => {
    try {
      const [list, contacts, identity] = await Promise.all([
        getBisonrelayPosts(),
        getBisonrelayContacts().catch(() => [] as BisonrelayContact[]),
        getBisonrelayIdentity().catch(() => null),
      ]);
      list.sort((a, b) => activityTs(b) - activityTs(a));
      setPosts(list);
      setPostsErr(null);
      const map: Record<string, string> = {};
      for (const c of contacts) {
        const uid = c.id?.identity;
        const av = c.id?.avatar;
        if (uid && av) map[uid] = av;
      }
      // The local user is never in the contacts list, so own posts,
      // comments and atom/seen rows would always fall back to the initial
      // circle; merge the identity avatar under the hex uid the feed keys by.
      if (identity?.identity && identity.avatar) {
        const hex = identityB64ToHex(identity.identity);
        if (hex) map[hex] = identity.avatar;
      }
      setAvatars(map);
    } catch (e: any) {
      const body = e?.response?.data;
      setPostsErr(typeof body === 'string' ? body : e?.message || 'Could not load posts');
    }
  }, []);

  useEffect(() => {
    reload();
    return addListener((evt: BisonrelayLiveEvent) => {
      if (
        evt.type === 'post-received' ||
        evt.type === 'post-status-received' ||
        evt.type === 'post-heart-received' ||
        evt.type === 'profile-updated'
      ) {
        reload();
      }
    });
  }, [addListener, reload]);

  // Fetch our own BR identity once so YourPostsView can filter by author.
  useEffect(() => {
    getBisonrelayIdentity()
      .then((id) => {
        if (id.identity) setOwnUid(identityB64ToHex(id.identity));
      })
      .catch(() => {
        /* leave ownUid empty; Your Posts view will show an empty list */
      });
  }, []);


  useEffect(() => {
    const onHashChange = () => setRoute(readHashRoute());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const content = (() => {
    if (route.section === 'detail' && route.target) {
      const key = `${route.target.uid}-${route.target.pid}`;
      const summary = posts?.find(
        (p) => p.author_id === route.target!.uid && p.id === route.target!.pid,
      );
      return (
        <PostDetailView
          uid={route.target.uid}
          pid={route.target.pid}
          ownUid={ownUid}
          summary={summary}
          avatarB64={avatars[route.target.uid]}
          avatars={avatars}
          onBack={() => navigateTo('feed')}
          onMarkSeen={(ts) => markSeen(key, ts)}
        />
      );
    }
    if (route.section === 'user' && route.target) {
      return (
        <UserProfileView
          key={`feed-user-${route.target.uid}`}
          uid={route.target.uid}
          ownUid={ownUid}
          posts={posts}
          avatars={avatars}
          onBack={() => navigateTo('feed')}
        />
      );
    }
    if (route.section === 'yours') {
      return (
        <PostsListView
          key="feed-yours"
          posts={posts}
          err={postsErr}
          filter={(p) => !!ownUid && p.author_id === ownUid}
          seen={seen}
          avatars={avatars}
          ownUid={ownUid}
          emptyTitle="You haven't published any posts yet"
          emptyHint='Use "New Post" in the sidebar to write your first one.'
        />
      );
    }
    if (route.section === 'subs') {
      return <SubscriptionsView />;
    }
    if (route.section === 'new') {
      return <NewPostView />;
    }
    return (
      <PostsListView
        key="feed-all"
        posts={posts}
        err={postsErr}
        seen={seen}
        avatars={avatars}
        ownUid={ownUid}
        emptyTitle="No posts yet"
        emptyHint="Subscribe to a contact's posts from their sub-nav (click their avatar in Chat) and new posts will land here as they publish."
      />
    );
  })();

  return (
    <div className="flex flex-col md:flex-row gap-4">
      <FeedSidebar active={route.section} />
      <div className="flex-1 min-w-0">{content}</div>
    </div>
  );
};

const sidebarItems: { id: Section; label: string; hash: string; icon: typeof Rss }[] = [
  { id: 'list', label: 'Feed', hash: 'feed', icon: Rss },
  { id: 'yours', label: 'Your Posts', hash: 'feed/yours', icon: FileText },
  { id: 'subs', label: 'Subscriptions', hash: 'feed/subs', icon: Users },
  { id: 'new', label: 'New Post', hash: 'feed/new', icon: Edit },
];

const FeedSidebar = ({ active }: { active: Section }) => (
  <aside className="md:w-44 shrink-0 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-2 md:self-start">
    <nav className="flex md:flex-col gap-1 overflow-x-auto overflow-y-hidden md:overflow-visible">
      {sidebarItems.map((item) => {
        // Treat the post-detail and user-profile views as belonging to the
        // list section so the "Feed" entry stays highlighted inside them.
        const isActive =
          item.id === active ||
          (item.id === 'list' && (active === 'detail' || active === 'user'));
        const Icon = item.icon;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => navigateTo(item.hash)}
            className={`shrink-0 whitespace-nowrap md:w-full px-3 py-2 rounded-md text-sm flex items-center gap-2 text-left transition-colors ${
              isActive
                ? 'bg-primary/20 text-primary font-semibold'
                : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground'
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" />
            <span>{item.label}</span>
          </button>
        );
      })}
    </nav>
  </aside>
);

const PostsListView = ({
  posts,
  err,
  seen,
  avatars,
  ownUid,
  filter,
  emptyTitle,
  emptyHint,
}: {
  posts: BisonrelayPostSummary[] | null;
  err: string | null;
  seen: Record<string, number>;
  avatars: Record<string, string>;
  ownUid: string;
  filter?: (p: BisonrelayPostSummary) => boolean;
  emptyTitle: string;
  emptyHint: string;
}) => {
  const filtered = posts ? (filter ? posts.filter(filter) : posts) : null;
  // The BR notification switches gate the new-activity dots only; the seen
  // watermarks keep updating so re-enabling shows the true activity state.
  const notifPrefs = useBrNotifPrefs();
  // Window the list so long feeds don't render (and image-fetch) every card
  // at once; scrolling near the sentinel reveals the next page.
  const [visibleCount, setVisibleCount] = useState(FEED_PAGE_SIZE);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const hasMore = !!filtered && filtered.length > visibleCount;

  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore) return;
    // visibleCount in the deps re-arms the observer after each page; the
    // initial observe() callback fires immediately, so a sentinel still in
    // view keeps paging until it leaves the viewport or the list runs out.
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting)) {
          setVisibleCount((c) => c + FEED_PAGE_SIZE);
        }
      },
      { rootMargin: '200px' },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [hasMore, visibleCount]);

  return (
    <div className="space-y-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {filtered === null && !err ? (
        <>
          {Array.from({ length: 4 }).map((_, i) => (
            <FeedCardSkeleton key={i} />
          ))}
        </>
      ) : filtered && filtered.length === 0 ? (
        <EmptyState title={emptyTitle} hint={emptyHint} />
      ) : (
        filtered?.slice(0, visibleCount).map((p) => {
          const key = `${p.author_id}-${p.id}`;
          return (
            <FeedCard
              key={key}
              post={p}
              hasActivity={notifPrefs.feedPosts && hasNewActivity(p, seen[key])}
              avatarB64={avatars[p.author_id]}
              ownUid={ownUid}
              onOpen={() => navigateTo(`feed/post/${p.author_id}/${p.id}`)}
            />
          );
        })
      )}
      {hasMore && (
        <div
          ref={sentinelRef}
          className="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground"
        >
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading more posts…</span>
        </div>
      )}
    </div>
  );
};

const EmptyState = ({ title, hint }: { title: string; hint: string }) => (
  <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 flex items-start gap-3">
    <div className="p-2 rounded-lg bg-primary/10 border border-primary/20 shrink-0">
      <Rss className="h-5 w-5 text-primary" />
    </div>
    <div className="space-y-1">
      <h3 className="text-sm font-semibold">{title}</h3>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  </div>
);

type SubTab = 'subscribed' | 'unsubscribed';

const SubscriptionsView = () => {
  const [contacts, setContacts] = useState<BisonrelayContact[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busyUid, setBusyUid] = useState<string | null>(null);
  const [tab, setTab] = useState<SubTab>('subscribed');
  const { addListener } = useBisonrelayLive();

  const reload = useCallback(async () => {
    try {
      const list = await getBisonrelayContacts();
      setContacts(list);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load contacts');
    }
  }, []);

  useEffect(() => {
    reload();
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'posts-subscribed' || evt.type === 'posts-unsubscribed') {
        reload();
      }
      if (evt.type === 'posts-subscribe-error') {
        // The change the UI optimistically assumed has failed remotely;
        // surface the daemon's text and refetch the real state.
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        setErr(String(payload.text ?? 'Subscription change failed'));
        reload();
      }
    });
  }, [addListener, reload]);

  // subscribed: every contact with posts_subscribed=true (ignored or not;
  // the user may want to unsub from an ignored contact's feed).
  // unsubscribed: every other non-ignored contact.
  const subscribed = useMemo(
    () => (contacts ? contacts.filter((c) => c.posts_subscribed) : null),
    [contacts],
  );
  const unsubscribed = useMemo(
    () =>
      contacts ? contacts.filter((c) => !c.posts_subscribed && !c.ignored) : null,
    [contacts],
  );

  const handleToggle = async (uid: string, currentlySubscribed: boolean) => {
    if (busyUid) return;
    setBusyUid(uid);
    try {
      if (currentlySubscribed) {
        await unsubscribeBisonrelayPosts(uid);
      } else {
        await subscribeBisonrelayPosts(uid);
      }
      // Live posts-subscribed / posts-unsubscribed event triggers reload.
    } catch (e: any) {
      const body = e?.response?.data;
      const verb = currentlySubscribed ? 'Unsubscribe' : 'Subscribe';
      setErr(typeof body === 'string' ? body : e?.message || `${verb} failed`);
    } finally {
      setBusyUid(null);
    }
  };

  const list = tab === 'subscribed' ? subscribed : unsubscribed;
  const subCount = subscribed?.length ?? 0;
  const unsubCount = unsubscribed?.length ?? 0;

  return (
    <div className="space-y-3">
      <div className="inline-flex rounded-lg bg-muted/20 p-0.5 border border-border/40 text-xs">
        <button
          type="button"
          onClick={() => setTab('subscribed')}
          className={`px-3 py-1.5 rounded-md transition-colors inline-flex items-center gap-1.5 ${
            tab === 'subscribed'
              ? 'bg-primary/20 text-primary font-semibold'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Subscribed
          <span className="tabular-nums opacity-70">({subCount})</span>
        </button>
        <button
          type="button"
          onClick={() => setTab('unsubscribed')}
          className={`px-3 py-1.5 rounded-md transition-colors inline-flex items-center gap-1.5 ${
            tab === 'unsubscribed'
              ? 'bg-primary/20 text-primary font-semibold'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Not subscribed
          <span className="tabular-nums opacity-70">({unsubCount})</span>
        </button>
      </div>

      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {list === null && !err ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading contacts…</span>
        </div>
      ) : list && list.length === 0 ? (
        <EmptyState
          title={
            tab === 'subscribed'
              ? "You're not subscribed to anyone yet"
              : 'Every contact is already subscribed'
          }
          hint={
            tab === 'subscribed'
              ? 'Switch to Not subscribed to add some, or KX with a new contact (new contacts auto-subscribe).'
              : 'Newly KX\'d contacts are auto-subscribed; existing ones will appear here if you unsubscribe.'
          }
        />
      ) : (
        <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden divide-y divide-border/30">
          {list?.map((c) => {
            const uid = c.id?.identity ?? '';
            const nick = c.nick_alias || c.id?.nick || uid.slice(0, 12);
            const isSubscribed = !!c.posts_subscribed;
            return (
              <div
                key={uid}
                className="px-4 py-3 flex items-center gap-3 hover:bg-muted/20 transition-colors"
              >
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-foreground truncate">{nick}</div>
                  <div className="text-[10px] text-muted-foreground font-mono break-all">
                    {uid}
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => handleToggle(uid, isSubscribed)}
                  disabled={!uid || busyUid === uid}
                  className={`shrink-0 px-3 py-1.5 rounded-md text-xs inline-flex items-center gap-1.5 transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                    isSubscribed
                      ? 'border border-border/50 text-muted-foreground hover:text-foreground hover:bg-muted/30'
                      : 'bg-gradient-primary text-white font-semibold'
                  }`}
                >
                  {busyUid === uid ? <Loader2 className="h-3 w-3 animate-spin" /> : null}
                  {isSubscribed ? 'Unsubscribe' : 'Subscribe'}
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

const NewPostView = () => {
  const [body, setBody] = useState('');
  const [embeds, setEmbeds] = useState<EditorEmbedMap>({});
  const [descr, setDescr] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const overCap = isEditorOverHardCap(body, embeds);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = body.trim();
    if (!trimmed || submitting || overCap) return;
    setSubmitting(true);
    setErr(null);
    try {
      const wire = composeBRBody(body, embeds);
      const summ = await createBisonrelayPost(wire, descr.trim());
      navigateTo(`feed/post/${summ.author_id}/${summ.id}`);
    } catch (e: any) {
      const respBody = e?.response?.data;
      setErr(typeof respBody === 'string' ? respBody : e?.message || 'Could not publish post');
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-4">
        <div>
          <h3 className="text-base font-semibold">New post</h3>
          <p className="text-xs text-muted-foreground mt-1">
            Posts are shared with everyone currently subscribed to your posts.
            Use the toolbar to attach an inline image / file or link to a
            shared file with an optional price.
          </p>
        </div>
        <div>
          <label
            className="block text-xs text-muted-foreground mb-1"
            htmlFor="br-new-post-descr"
          >
            Description (optional)
          </label>
          <input
            id="br-new-post-descr"
            type="text"
            value={descr}
            onChange={(e) => setDescr(e.target.value)}
            disabled={submitting}
            maxLength={200}
            placeholder="Short summary shown in some clients"
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
          />
        </div>
        <div>
          <label className="block text-xs text-muted-foreground mb-1">Body</label>
          <BisonrelayEditor
            value={body}
            onChange={setBody}
            embeds={embeds}
            onEmbedsChange={setEmbeds}
            disabled={submitting}
            placeholder={'# My first post\n\nWrite your post here…'}
          />
        </div>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={() => navigateTo('feed')}
            disabled={submitting}
            className="px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={!body.trim() || submitting || overCap}
            title={overCap ? 'Post exceeds the BR wire size limit' : undefined}
            className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
            <Send className="h-4 w-4" />
            Publish
          </button>
        </div>
      </div>
    </form>
  );
};

const PostDetailView = ({
  uid,
  pid,
  ownUid,
  summary,
  avatarB64,
  avatars,
  onBack,
  onMarkSeen,
}: {
  uid: string;
  pid: string;
  ownUid: string;
  summary: BisonrelayPostSummary | undefined;
  avatarB64?: string;
  avatars: Record<string, string>;
  onBack: () => void;
  onMarkSeen?: (ts: number) => void;
}) => {
  const [body, setBody] = useState<BisonrelayPostBody | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [hearts, setHearts] = useState<number | null>(null);
  const [hearted, setHearted] = useState(false);
  const [hearting, setHearting] = useState(false);
  const [heartedBy, setHeartedBy] = useState<{ user: string; nick: string }[]>([]);
  const [receipts, setReceipts] = useState<BisonrelayReceiveReceipt[]>([]);
  // Relay-to-subscribers flow: confirm shows the live subscriber count
  // before anything is sent.
  const [relayState, setRelayState] = useState<'idle' | 'confirm' | 'busy' | 'done'>('idle');
  const [relayCount, setRelayCount] = useState(0);
  const [relayErr, setRelayErr] = useState<string | null>(null);
  const [showTip, setShowTip] = useState(false);
  const [tipStatus, setTipStatus] = useState<{
    state: 'requesting' | 'paying' | 'sent' | 'failed';
    line: string;
  } | null>(null);
  const isOwnPost = !!ownUid && uid === ownUid;
  const authorNick = summary?.author_nick || uid.slice(0, 12);
  const { addListener } = useBisonrelayLive();

  const startRelay = async () => {
    setRelayErr(null);
    try {
      const stats = await getBisonrelayStatsPosts();
      setRelayCount(stats.subscribers_count);
      setRelayState('confirm');
    } catch (e: any) {
      const body = e?.response?.data;
      setRelayErr(typeof body === 'string' ? body : e?.message || 'Could not load subscribers');
    }
  };

  const confirmRelay = async () => {
    setRelayState('busy');
    setRelayErr(null);
    try {
      await relayBisonrelayPost(uid, pid);
      setRelayState('done');
    } catch (e: any) {
      const body = e?.response?.data;
      setRelayErr(typeof body === 'string' ? body : e?.message || 'Relay failed');
      setRelayState('confirm');
    }
  };

  // Fire-and-forget like the chat page's tip flow; the live
  // tip-invoice-generated / tip-sent / tip-failed events advance the line.
  const submitTip = (dcrAmount: number) => {
    setTipStatus({
      state: 'requesting',
      line: `Requesting invoice for ${dcrAmount} DCR to tip ${authorNick}...`,
    });
    tipBisonrelayContact(uid, dcrAmount).catch((e: any) => {
      const body = e?.response?.data;
      const msg = typeof body === 'string' ? body : e?.message || 'Tip failed';
      setTipStatus({
        state: 'failed',
        line: `Tip attempt of ${dcrAmount} DCR failed due to ${msg}. Given up on attempting to tip.`,
      });
    });
  };

  const loadBody = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const b = await getBisonrelayPostBody(uid, pid);
      setBody(b);
    } catch (e: any) {
      const respBody = e?.response?.data;
      setErr(typeof respBody === 'string' ? respBody : e?.message || 'Could not load post body');
    } finally {
      setLoading(false);
    }
  }, [uid, pid]);

  const loadHearts = useCallback(async () => {
    try {
      const h = await getBisonrelayPostHearts(uid, pid);
      setHearts(h.count);
      setHearted(h.hearted_by_me);
      setHeartedBy(h.hearts ?? []);
    } catch {
      /* leave count hidden on error; heart button still works */
    }
  }, [uid, pid]);

  const loadReceipts = useCallback(async () => {
    if (!isOwnPost) return;
    try {
      setReceipts(await getBisonrelayPostReceiveReceipts(pid));
    } catch {
      /* older brclientd without the endpoint; keep the section hidden */
    }
  }, [isOwnPost, pid]);

  useEffect(() => {
    setBody(null);
    setErr(null);
    setHearts(null);
    setHearted(false);
    setHeartedBy([]);
    setReceipts([]);
    loadBody();
    loadHearts();
    loadReceipts();
  }, [loadBody, loadHearts, loadReceipts]);

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'receive-receipt') {
        // A peer acknowledged receiving this post; refresh Seen by live
        // (receipts do not fire post-status-received).
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        if (payload.domain === 'posts' && payload.id === pid) loadReceipts();
        return;
      }
      if (evt.type !== 'post-heart-received' && evt.type !== 'post-status-received') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.author !== uid || payload.pid !== pid) return;
      if (evt.type === 'post-heart-received') loadHearts();
      loadReceipts();
    });
  }, [addListener, uid, pid, loadHearts, loadReceipts]);

  // Tip progress mirrors the chat page's handling of the same events.
  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'tip-invoice-generated') {
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        if (String(payload.uid ?? '') !== uid) return;
        const nick = String(payload.nick ?? '');
        setTipStatus((prev) =>
          prev && prev.state === 'requesting'
            ? { state: 'paying', line: `Invoice received, paying tip to ${nick}...` }
            : prev,
        );
        return;
      }
      if (evt.type !== 'tip-sent' && evt.type !== 'tip-failed') return;
      const payload = (evt.payload ?? {}) as Record<string, string>;
      if (payload.recipient !== uid || !payload.line) return;
      setTipStatus({ state: evt.type === 'tip-sent' ? 'sent' : 'failed', line: payload.line });
    });
  }, [addListener, uid]);

  const toggleHeart = async () => {
    if (hearting) return;
    const next = !hearted;
    setHearting(true);
    setHearted(next);
    setHearts((prev) => (prev === null ? prev : prev + (next ? 1 : -1)));
    try {
      await heartBisonrelayPost(uid, pid, next);
    } catch {
      setHearted(!next);
      setHearts((prev) => (prev === null ? prev : prev + (next ? -1 : 1)));
    } finally {
      setHearting(false);
    }
  };

  useEffect(() => {
    if (!summary || !onMarkSeen) return;
    const ts =
      summary.last_status_ts && summary.last_status_ts > summary.date
        ? summary.last_status_ts
        : summary.date;
    onMarkSeen(ts);
  }, [summary, onMarkSeen]);

  const title = summary?.title || body?.title || '(untitled post)';
  const publishedTs = summary ? summary.published || summary.date : 0;
  const dateStr = publishedTs ? new Date(publishedTs * 1000).toLocaleString() : '';
  const waitingForArrival = !summary && (loading || err === null) && body === null;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to feed
        </button>
      </div>
      <header className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 flex items-start gap-3">
        <AuthorAvatar uid={uid} nick={authorNick} avatarB64={avatarB64} size="md" />
        <div className="min-w-0 flex-1 space-y-2">
          <h2 className="text-lg font-semibold text-foreground break-words">{title}</h2>
          <div className="text-xs text-muted-foreground">
            <button
              type="button"
              onClick={() => navigateTo(`feed/user/${uid}`)}
              title="View profile"
              className="hover:text-foreground hover:underline transition-colors"
            >
              {authorNick}
            </button>
            {dateStr && (
              <>
                <span className="mx-1.5 opacity-50">·</span>
                {dateStr}
              </>
            )}
          </div>
          <div className="flex items-center gap-4">
            <button
              type="button"
              onClick={toggleHeart}
              disabled={hearting}
              aria-pressed={hearted}
              title="Atom this post"
              className={`inline-flex items-center gap-1.5 text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                hearted ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Atom className="h-8 w-8" strokeWidth={hearted ? 2.5 : 2} />
              <span className="tabular-nums">{hearts ?? 0}</span>
            </button>
            {!isOwnPost && (
              <button
                type="button"
                onClick={() => setShowTip(true)}
                title="Pay a tip over Lightning"
                className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                <Coins className="h-4 w-4" />
                <span>Pay tip</span>
              </button>
            )}
            <button
              type="button"
              onClick={() => (relayState === 'idle' ? startRelay() : setRelayState('idle'))}
              disabled={relayState === 'busy'}
              title="Relay this post to your subscribers"
              className={`ml-auto inline-flex items-center gap-1.5 text-xs transition-colors disabled:opacity-50 ${
                relayState === 'done'
                  ? 'text-success'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Repeat2 className="h-4 w-4" />
              <span>{relayState === 'done' ? 'Relayed' : 'Relay'}</span>
            </button>
          </div>
          {relayState === 'confirm' && (
            <div className="text-xs space-y-1.5">
              {relayCount > 0 ? (
                <>
                  <div>
                    Relay this post to your{' '}
                    <span className="font-semibold text-foreground">{relayCount}</span>{' '}
                    subscriber{relayCount === 1 ? '' : 's'}?
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={confirmRelay}
                      className="px-3 py-1 rounded-md bg-primary/20 text-primary font-semibold hover:bg-primary/30 transition-colors"
                    >
                      Relay
                    </button>
                    <button
                      type="button"
                      onClick={() => setRelayState('idle')}
                      className="px-3 py-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/30"
                    >
                      Cancel
                    </button>
                  </div>
                </>
              ) : (
                <span className="text-muted-foreground">
                  You have no post subscribers yet, so there is nobody to relay to.
                </span>
              )}
            </div>
          )}
          {relayState === 'busy' && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 className="h-3 w-3 animate-spin" />
              <span>Relaying...</span>
            </div>
          )}
          {relayErr && <p className="text-xs text-destructive">{relayErr}</p>}
          {tipStatus && (
            <div
              className={`flex items-center gap-2 text-xs ${
                tipStatus.state === 'sent'
                  ? 'text-success'
                  : tipStatus.state === 'failed'
                    ? 'text-destructive'
                    : 'text-muted-foreground'
              }`}
            >
              {(tipStatus.state === 'requesting' || tipStatus.state === 'paying') && (
                <Loader2 className="h-3 w-3 animate-spin shrink-0" />
              )}
              <span className="min-w-0 break-words">{tipStatus.line}</span>
            </div>
          )}
        </div>
      </header>
      <article className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-3">
        {loading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span>{waitingForArrival ? 'Waiting for post to arrive…' : 'Loading body…'}</span>
          </div>
        ) : err ? (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        ) : body && body.segments && body.segments.length > 0 ? (
          <PostBodySegments segments={body.segments} uid={uid} />
        ) : body ? (
          <p className="text-sm text-muted-foreground italic">(Empty post)</p>
        ) : null}
      </article>
      {body && (
        <section className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5">
          <PostComments authorId={uid} pid={pid} avatars={avatars} isOwnPost={isOwnPost} />
        </section>
      )}
      {isOwnPost && (receipts.length > 0 || heartedBy.length > 0) && (
        <section className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5">
          <div
            className={`grid grid-cols-1 gap-4 ${
              receipts.length > 0 && heartedBy.length > 0 ? 'sm:grid-cols-2' : ''
            }`}
          >
            {receipts.length > 0 && (
              <div className="space-y-3">
                <h4 className="text-[11px] uppercase tracking-wide text-muted-foreground">
                  Seen by
                </h4>
                <div className="flex flex-wrap gap-2">
                  {receipts.map((r) => (
                    <span
                      key={r.user}
                      title={`${r.nick} - ${new Date(r.server_time).toLocaleString()}`}
                    >
                      <AuthorAvatar
                        uid={r.user}
                        nick={r.nick}
                        avatarB64={avatars[r.user]}
                        size="sm"
                      />
                    </span>
                  ))}
                </div>
              </div>
            )}
            {heartedBy.length > 0 && (
              <div
                className={`space-y-3 ${receipts.length > 0 ? 'sm:text-right' : ''}`}
              >
                <h4 className="text-[11px] uppercase tracking-wide text-muted-foreground">
                  Atomed by
                </h4>
                <div
                  className={`flex flex-wrap gap-2 ${
                    receipts.length > 0 ? 'sm:justify-end' : ''
                  }`}
                >
                  {heartedBy.map((h) => (
                    <span key={h.user} title={h.nick}>
                      <AuthorAvatar
                        uid={h.user}
                        nick={h.nick}
                        avatarB64={avatars[h.user]}
                        size="sm"
                      />
                    </span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </section>
      )}
      {showTip && (
        <TipModal
          nick={authorNick}
          uid={uid}
          onClose={() => setShowTip(false)}
          onSubmit={submitTip}
        />
      )}
    </div>
  );
};

const MAX_COMMENT_DEPTH = 50;
// Visual nesting cap: replies indent under a left rail up to this depth, after
// which indent growth eases off so long chains stay on-screen (the rail stays
// at every depth). This is purely cosmetic and separate from the much deeper
// render-safety cap above.
const MAX_VISUAL_DEPTH = 6;
// Shared empty ancestor set for the root render; CommentNode always clones
// before adding, so it is never mutated.
const EMPTY_SEEN: Set<string> = new Set();

type CommentTree = {
  roots: BisonrelayPostComment[];
  childrenOf: Map<string, BisonrelayPostComment[]>;
};

// Builds a parent/child tree from the flat comment list. Comments are keyed by
// status_id (the unique per-comment hash); a reply's `parent` holds the parent
// comment's status_id, so it nests under it. (`identifier` is the post id,
// shared by every comment on the post, so it cannot thread replies.) Comments
// whose parent isn't in the set (orphans) are promoted to the root so they're
// still visible. Pending optimistic comments slot in by their `parent` field
// too (children) or by absence of one (top-level).
const buildCommentTree = (list: BisonrelayPostComment[]): CommentTree => {
  const byId = new Map<string, BisonrelayPostComment>();
  for (const c of list) {
    if (c.status_id) byId.set(c.status_id, c);
  }
  const roots: BisonrelayPostComment[] = [];
  const childrenOf = new Map<string, BisonrelayPostComment[]>();
  for (const c of list) {
    if (c.parent && c.parent !== c.status_id && byId.has(c.parent)) {
      const arr = childrenOf.get(c.parent) ?? [];
      arr.push(c);
      childrenOf.set(c.parent, arr);
    } else {
      roots.push(c);
    }
  }
  const byTs = (a: BisonrelayPostComment, b: BisonrelayPostComment) =>
    a.timestamp - b.timestamp;
  roots.sort(byTs);
  childrenOf.forEach((arr) => arr.sort(byTs));
  return { roots, childrenOf };
};

const PostComments = ({
  authorId,
  pid,
  avatars,
  isOwnPost,
}: {
  authorId: string;
  pid: string;
  avatars: Record<string, string>;
  isOwnPost: boolean;
}) => {
  const [comments, setComments] = useState<BisonrelayPostComment[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [replyTargetId, setReplyTargetId] = useState<string | null>(null);
  // Receive receipts per comment status_id; only recorded on the post
  // author's node (it relays the comments), so loaded for own posts only.
  const [commentReceipts, setCommentReceipts] = useState<
    Record<string, BisonrelayReceiveReceipt[]>
  >({});
  const { addListener } = useBisonrelayLive();

  const loadCommentReceipts = useCallback(async () => {
    if (!isOwnPost) return;
    try {
      setCommentReceipts(await getBisonrelayPostCommentReceipts(pid));
    } catch {
      /* older brclientd without the endpoint; markers stay hidden */
    }
  }, [isOwnPost, pid]);

  const reload = useCallback(async () => {
    try {
      const list = await getBisonrelayPostComments(authorId, pid);
      setComments(list);
      setErr(null);
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Could not load comments');
    }
  }, [authorId, pid]);

  useEffect(() => {
    reload();
    loadCommentReceipts();
    return addListener((evt: BisonrelayLiveEvent) => {
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (evt.type === 'receive-receipt') {
        if (payload.domain === 'postcomments' && payload.id === pid) {
          loadCommentReceipts();
        }
        return;
      }
      if (evt.type !== 'post-status-received') return;
      if (payload.author !== authorId || payload.pid !== pid) return;
      reload();
    });
  }, [addListener, authorId, pid, reload, loadCommentReceipts]);

  const tree = useMemo(
    () => buildCommentTree(comments ?? []),
    [comments],
  );

  const submitComment = useCallback(
    async (text: string, parent?: string) => {
      const commentKey = `c-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
      setSubmitting(true);
      setComments((prev) => [
        ...(prev ?? []),
        {
          status_from: '',
          from_nick: '',
          comment: text,
          parent,
          timestamp: Math.floor(Date.now() / 1000),
          pending: true,
          commentKey,
        },
      ]);
      try {
        await postBisonrelayComment(authorId, pid, text, parent);
        // The send succeeded; reload replaces the optimistic row with the
        // server's unreplicated entry. If the reload itself failed, stop the
        // spinner on the surviving optimistic row and flag it instead.
        await reload();
        setComments((prev) => {
          if (!prev) return prev;
          const idx = prev.findIndex((c) => c.commentKey === commentKey);
          if (idx === -1) return prev;
          const updated = [...prev];
          updated[idx] = {
            ...updated[idx],
            pending: false,
            unreplicated: true,
            commentKey: undefined,
          };
          return updated;
        });
      } catch (e: any) {
        const body = e?.response?.data;
        const msg = typeof body === 'string' ? body : e?.message || 'Comment failed';
        setComments((prev) => {
          if (!prev) return prev;
          const idx = prev.findIndex((c) => c.commentKey === commentKey);
          if (idx === -1) return prev;
          const updated = [...prev];
          updated[idx] = {
            ...updated[idx],
            comment: `${text}  —  Failed: ${msg}`,
            pending: false,
            commentKey: undefined,
          };
          return updated;
        });
      } finally {
        setSubmitting(false);
      }
    },
    [authorId, pid, reload],
  );

  const handleTopLevelSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const text = draft.trim();
    if (!text || submitting) return;
    setDraft('');
    await submitComment(text);
  };

  return (
    <div className="space-y-3">
      <h4 className="text-[11px] uppercase tracking-wide text-muted-foreground">Comments</h4>
      {err && (
        <div className="flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {comments === null && !err ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          <span>Loading comments…</span>
        </div>
      ) : comments && comments.length === 0 ? (
        <p className="text-xs text-muted-foreground italic">No comments yet.</p>
      ) : (
        <div className="space-y-2">
          {tree.roots.map((c) => (
            <CommentNode
              key={c.status_id || c.commentKey || `${c.timestamp}-${c.status_from}`}
              comment={c}
              level={0}
              childrenOf={tree.childrenOf}
              seenIds={EMPTY_SEEN}
              avatars={avatars}
              seenReceipts={commentReceipts}
              replyTargetId={replyTargetId}
              setReplyTargetId={setReplyTargetId}
              onSubmitReply={submitComment}
              submitting={submitting}
            />
          ))}
        </div>
      )}
      <form onSubmit={handleTopLevelSubmit} className="flex items-end gap-2 pt-2">
        <textarea
          rows={2}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder="Add a comment…"
          disabled={submitting}
          className="flex-1 px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50 resize-y min-h-[44px]"
        />
        <button
          type="submit"
          disabled={!draft.trim() || submitting}
          className="px-3 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? 'Sending…' : 'Comment'}
        </button>
      </form>
    </div>
  );
};

const CommentNode = ({
  comment,
  level,
  childrenOf,
  seenIds,
  avatars,
  seenReceipts,
  replyTargetId,
  setReplyTargetId,
  onSubmitReply,
  submitting,
}: {
  comment: BisonrelayPostComment;
  level: number;
  childrenOf: Map<string, BisonrelayPostComment[]>;
  seenIds: Set<string>;
  avatars: Record<string, string>;
  seenReceipts: Record<string, BisonrelayReceiveReceipt[]>;
  replyTargetId: string | null;
  setReplyTargetId: (id: string | null) => void;
  onSubmitReply: (text: string, parent?: string) => Promise<void>;
  submitting: boolean;
}) => {
  const seen = (comment.status_id && seenReceipts[comment.status_id]) || [];
  // A comment's identity is its status_id (unique per-comment hash); replies
  // reference it as their `parent`. (`identifier` is the shared post id.)
  const id = comment.status_id;
  const nick =
    comment.from_nick ||
    (comment.status_from ? comment.status_from.slice(0, 12) : 'you');
  const isReplyTarget = !!id && replyTargetId === id;
  const canReply = !!id;
  const children = id ? childrenOf.get(id) ?? [] : [];
  // Guard the recursive render against cycles in the comment graph (e.g. a
  // self-referential reply): never descend into a node already on the path from
  // the root, and cap the depth. Without this such a comment renders forever and
  // exhausts memory.
  const childSeen = id ? new Set(seenIds).add(id) : seenIds;
  const safeChildren =
    level >= MAX_COMMENT_DEPTH
      ? []
      : children.filter((ch) => !(ch.status_id && childSeen.has(ch.status_id)));
  // Each reply level is inset under a left rail so the parent/child link reads
  // at a glance; hovering a thread lights up its rail. Indent is small on mobile
  // and roomier on sm+, and growth eases off past MAX_VISUAL_DEPTH so long
  // chains stay on-screen, while the rail stays at every depth.
  const indentWrap =
    'space-y-2 border-l-2 border-border/40 transition-colors hover:border-primary/40 ' +
    (level < MAX_VISUAL_DEPTH ? 'pl-2 ml-1 sm:pl-4 sm:ml-2' : 'pl-2 sm:pl-3');
  return (
    <div className="space-y-2">
      <div className="flex items-start gap-2 text-sm">
        <AuthorAvatar
          uid={comment.status_from || ''}
          nick={nick}
          avatarB64={comment.status_from ? avatars[comment.status_from] : undefined}
          size="sm"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
            {comment.pending && <Loader2 className="h-3 w-3 animate-spin" />}
            <span className="font-medium text-foreground/90">{nick}</span>
            <span className="opacity-50">·</span>
            <span className="opacity-70">
              {comment.timestamp ? new Date(comment.timestamp * 1000).toLocaleString() : ''}
            </span>
            {comment.unreplicated && (
              <span className="italic opacity-70">Not yet relayed by the post author</span>
            )}
            {seen.length > 0 && (
              <span
                className="opacity-70"
                title={seen.map((r) => r.nick).join(', ')}
              >
                seen {seen.length}
              </span>
            )}
            <button
              type="button"
              onClick={() => setReplyTargetId(isReplyTarget ? null : id ?? null)}
              disabled={!canReply}
              title={canReply ? undefined : 'Waiting for delivery'}
              className="inline-flex items-center gap-1 ml-2 text-[11px] text-muted-foreground hover:text-foreground transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              <Reply className="h-3 w-3" />
              <span>{isReplyTarget ? 'Cancel' : 'Reply'}</span>
            </button>
          </div>
          <p className="text-sm text-foreground/90 break-words whitespace-pre-wrap">
            {comment.comment}
          </p>
        </div>
      </div>
      {(isReplyTarget || safeChildren.length > 0) && (
        <div className={indentWrap}>
          {isReplyTarget && id && (
            <InlineReplyComposer
              submitting={submitting}
              onCancel={() => setReplyTargetId(null)}
              onSubmit={async (text) => {
                await onSubmitReply(text, id);
                setReplyTargetId(null);
              }}
            />
          )}
          {safeChildren.map((child) => (
            <CommentNode
              key={child.status_id || child.commentKey || `${child.timestamp}-${child.status_from}`}
              comment={child}
              level={level + 1}
              childrenOf={childrenOf}
              seenIds={childSeen}
              avatars={avatars}
              seenReceipts={seenReceipts}
              replyTargetId={replyTargetId}
              setReplyTargetId={setReplyTargetId}
              onSubmitReply={onSubmitReply}
              submitting={submitting}
            />
          ))}
        </div>
      )}
    </div>
  );
};

const InlineReplyComposer = ({
  submitting,
  onCancel,
  onSubmit,
}: {
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (text: string) => Promise<void>;
}) => {
  const [text, setText] = useState('');
  const handle = async (e: React.FormEvent) => {
    e.preventDefault();
    const t = text.trim();
    if (!t || submitting) return;
    setText('');
    await onSubmit(t);
  };
  return (
    <form onSubmit={handle} className="space-y-2">
      <textarea
        rows={2}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Write a reply…"
        disabled={submitting}
        autoFocus
        className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50 resize-y min-h-[44px]"
      />
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          disabled={submitting}
          className="px-3 py-1.5 rounded-lg text-xs text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={!text.trim() || submitting}
          className="px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-xs font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {submitting ? 'Sending…' : 'Reply'}
        </button>
      </div>
    </form>
  );
};

const PostBodySegments = ({ segments, uid }: { segments: BisonrelayPostBodySegment[]; uid: string }) => {
  const [viewer, setViewer] = useState<ViewerImage | null>(null);
  return (
    <div className="space-y-3">
      {segments.map((seg, i) => {
        if (seg.kind === 'text' && seg.html) {
          return (
            <div
              key={i}
              className={BR_PROSE_CLASSES}
              dangerouslySetInnerHTML={{ __html: seg.html }}
            />
          );
        }
        if (seg.kind === 'embed' && seg.download && !seg.data_b64) {
          return <DownloadEmbed key={i} seg={seg} uid={uid} />;
        }
        if (seg.kind === 'embed' && seg.data_b64) {
          const isImage = !!seg.mime && seg.mime.startsWith('image/');
          if (isImage) {
            const src = `data:${seg.mime};base64,${seg.data_b64}`;
            return (
              <button
                key={i}
                type="button"
                onClick={() => setViewer({ src, name: seg.name || seg.filename || 'image', mime: seg.mime || '' })}
                className="block p-0 border-0 bg-transparent cursor-zoom-in"
              >
                <img
                  src={src}
                  alt={seg.alt || seg.name || ''}
                  className="rounded-lg border border-border/40 max-w-full h-auto"
                />
              </button>
            );
          }
          const href = `data:${seg.mime || 'application/octet-stream'};base64,${seg.data_b64}`;
          return (
            <a
              key={i}
              href={href}
              download={seg.name || 'attachment'}
              className="inline-block max-w-full break-words text-xs text-primary underline hover:no-underline"
            >
              {seg.name || 'attachment'} ({seg.mime || 'binary'})
            </a>
          );
        }
        return null;
      })}
      {viewer && <ImageViewerModal image={viewer} onClose={() => setViewer(null)} />}
    </div>
  );
};
