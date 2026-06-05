// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  ArrowLeft,
  ChevronRight,
  Edit,
  Heart,
  Loader2,
  Reply,
  Rss,
  Users,
  Send,
  FileText,
} from 'lucide-react';
import { BR_PROSE_CLASSES } from './bisonrelayProse';
import { useBrNotifPrefs } from './brNotifPrefs';
import { avatarDataUrl, colorForUid } from './bisonrelayAvatar';
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
  getBisonrelayPostComments,
  getBisonrelayPostHearts,
  getBisonrelayPostReceiveReceipts,
  getBisonrelayPosts,
  heartBisonrelayPost,
  postBisonrelayComment,
  subscribeBisonrelayPosts,
  unsubscribeBisonrelayPosts,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { DownloadEmbed } from './DownloadEmbed';

type Section = 'list' | 'yours' | 'subs' | 'new' | 'detail';

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

const relativeTime = (ts: number): string => {
  if (!ts) return '';
  const now = Math.floor(Date.now() / 1000);
  let delta = now - ts;
  if (delta < 0) delta = 0;
  if (delta < 60) return 'just now';
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h ago`;
  return `${Math.floor(delta / 86400)}d ago`;
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
      const [list, contacts] = await Promise.all([
        getBisonrelayPosts(),
        getBisonrelayContacts().catch(() => [] as BisonrelayContact[]),
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
    if (route.section === 'yours') {
      return (
        <PostsListView
          posts={posts}
          err={postsErr}
          filter={(p) => !!ownUid && p.author_id === ownUid}
          seen={seen}
          avatars={avatars}
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
        posts={posts}
        err={postsErr}
        seen={seen}
        avatars={avatars}
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
    <nav className="flex md:flex-col gap-1 overflow-x-auto md:overflow-visible">
      {sidebarItems.map((item) => {
        // Treat the post-detail view as belonging to the list section so
        // the "Feed" entry stays highlighted when reading a post.
        const isActive = item.id === active || (item.id === 'list' && active === 'detail');
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
  filter,
  emptyTitle,
  emptyHint,
}: {
  posts: BisonrelayPostSummary[] | null;
  err: string | null;
  seen: Record<string, number>;
  avatars: Record<string, string>;
  filter?: (p: BisonrelayPostSummary) => boolean;
  emptyTitle: string;
  emptyHint: string;
}) => {
  const filtered = posts ? (filter ? posts.filter(filter) : posts) : null;
  // The BR notification switches gate the new-activity dots only; the seen
  // watermarks keep updating so re-enabling shows the true activity state.
  const notifPrefs = useBrNotifPrefs();

  return (
    <div className="space-y-3">
      {err && (
        <div className="rounded-lg bg-destructive/10 border border-destructive/30 p-3 flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
      {filtered === null && !err ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading posts…</span>
        </div>
      ) : filtered && filtered.length === 0 ? (
        <EmptyState title={emptyTitle} hint={emptyHint} />
      ) : (
        <div className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 overflow-hidden divide-y divide-border/30">
          {filtered?.map((p) => {
            const key = `${p.author_id}-${p.id}`;
            return (
              <FeedRow
                key={key}
                post={p}
                hasActivity={notifPrefs.feedPosts && hasNewActivity(p, seen[key])}
                avatarB64={avatars[p.author_id]}
                onOpen={() => navigateTo(`feed/post/${p.author_id}/${p.id}`)}
              />
            );
          })}
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

const FeedRow = ({
  post,
  hasActivity,
  avatarB64,
  onOpen,
}: {
  post: BisonrelayPostSummary;
  hasActivity: boolean;
  avatarB64?: string;
  onOpen: () => void;
}) => {
  const dateStr = new Date(post.date * 1000).toLocaleString();
  const nick = post.author_nick || post.author_id.slice(0, 12);
  return (
    <button
      type="button"
      onClick={onOpen}
      className="w-full px-4 py-3 flex items-center gap-3 text-left hover:bg-muted/20 transition-colors"
    >
      <AuthorAvatar uid={post.author_id} nick={nick} avatarB64={avatarB64} size="sm" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-semibold text-foreground truncate flex items-center gap-2">
          {hasActivity && (
            <span
              className="inline-block h-2 w-2 rounded-full bg-primary shrink-0"
              aria-hidden
              title="New activity since you last opened this post"
            />
          )}
          <span className="truncate">{post.title || '(untitled post)'}</span>
        </div>
        <div className="text-[11px] text-muted-foreground mt-0.5 truncate">
          {nick}
          <span className="mx-1.5 opacity-50">·</span>
          {dateStr}
          {hasActivity && post.last_status_ts && (
            <>
              <span className="mx-1.5 opacity-50">·</span>
              <span className="text-primary/80">
                Last comment {relativeTime(post.last_status_ts)}
              </span>
            </>
          )}
        </div>
      </div>
      <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" />
    </button>
  );
};

const AuthorAvatar = ({
  uid,
  nick,
  avatarB64,
  size,
}: {
  uid: string;
  nick: string;
  avatarB64?: string;
  size: 'sm' | 'md';
}) => {
  const dim = size === 'sm' ? 'h-7 w-7 text-[11px]' : 'h-10 w-10 text-sm';
  const dataUrl = avatarDataUrl(avatarB64);
  if (dataUrl) {
    return (
      <img
        src={dataUrl}
        alt=""
        className={`shrink-0 rounded-full object-cover bg-muted/30 ${dim}`}
      />
    );
  }
  const initial = nick.trim().charAt(0).toUpperCase() || '?';
  const bg = colorForUid(uid || nick);
  return (
    <span
      className={`shrink-0 rounded-full flex items-center justify-center font-semibold text-white ${bg} ${dim}`}
    >
      {initial}
    </span>
  );
};

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
  const [receipts, setReceipts] = useState<BisonrelayReceiveReceipt[]>([]);
  const isOwnPost = !!ownUid && uid === ownUid;
  const { addListener } = useBisonrelayLive();

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
    setReceipts([]);
    loadBody();
    loadHearts();
    loadReceipts();
  }, [loadBody, loadHearts, loadReceipts]);

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'post-heart-received' && evt.type !== 'post-status-received') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.author !== uid || payload.pid !== pid) return;
      if (evt.type === 'post-heart-received') loadHearts();
      loadReceipts();
    });
  }, [addListener, uid, pid, loadHearts, loadReceipts]);

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
  const authorNick = summary?.author_nick || uid.slice(0, 12);
  const dateStr = summary?.date ? new Date(summary.date * 1000).toLocaleString() : '';
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
            {authorNick}
            {dateStr && (
              <>
                <span className="mx-1.5 opacity-50">·</span>
                {dateStr}
              </>
            )}
          </div>
          <button
            type="button"
            onClick={toggleHeart}
            disabled={hearting}
            aria-pressed={hearted}
            className={`inline-flex items-center gap-1.5 text-xs transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
              hearted ? 'text-rose-500' : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <Heart
              className="h-4 w-4"
              fill={hearted ? 'currentColor' : 'none'}
              strokeWidth={hearted ? 0 : 2}
            />
            <span className="tabular-nums">{hearts ?? 0}</span>
          </button>
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
          <PostComments authorId={uid} pid={pid} avatars={avatars} />
        </section>
      )}
      {isOwnPost && receipts.length > 0 && (
        <section className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5 space-y-3">
          <h4 className="text-[11px] uppercase tracking-wide text-muted-foreground">Seen by</h4>
          <div className="flex flex-wrap gap-2">
            {receipts.map((r) => (
              <span
                key={r.user}
                title={`${r.nick} - ${new Date(r.server_time).toLocaleString()}`}
              >
                <AuthorAvatar uid={r.user} nick={r.nick} avatarB64={avatars[r.user]} size="sm" />
              </span>
            ))}
          </div>
        </section>
      )}
    </div>
  );
};

type CommentTree = {
  roots: BisonrelayPostComment[];
  childrenOf: Map<string, BisonrelayPostComment[]>;
};

// Builds a parent/child tree from the flat comment list. Comments whose
// parent isn't in the set (orphans) are promoted to the root so they're
// still visible. Pending optimistic comments slot in by their `parent`
// field too (children) or by absence of one (top-level).
const buildCommentTree = (list: BisonrelayPostComment[]): CommentTree => {
  const byId = new Map<string, BisonrelayPostComment>();
  for (const c of list) {
    if (c.identifier) byId.set(c.identifier, c);
  }
  const roots: BisonrelayPostComment[] = [];
  const childrenOf = new Map<string, BisonrelayPostComment[]>();
  for (const c of list) {
    if (c.parent && byId.has(c.parent)) {
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
}: {
  authorId: string;
  pid: string;
  avatars: Record<string, string>;
}) => {
  const [comments, setComments] = useState<BisonrelayPostComment[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [replyTargetId, setReplyTargetId] = useState<string | null>(null);
  const { addListener } = useBisonrelayLive();

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
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type !== 'post-status-received') return;
      const payload = (evt.payload ?? {}) as Record<string, unknown>;
      if (payload.author !== authorId || payload.pid !== pid) return;
      reload();
    });
  }, [addListener, authorId, pid, reload]);

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
              key={c.identifier || c.commentKey || `${c.timestamp}-${c.status_from}`}
              comment={c}
              level={0}
              childrenOf={tree.childrenOf}
              avatars={avatars}
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
  avatars,
  replyTargetId,
  setReplyTargetId,
  onSubmitReply,
  submitting,
}: {
  comment: BisonrelayPostComment;
  level: number;
  childrenOf: Map<string, BisonrelayPostComment[]>;
  avatars: Record<string, string>;
  replyTargetId: string | null;
  setReplyTargetId: (id: string | null) => void;
  onSubmitReply: (text: string, parent?: string) => Promise<void>;
  submitting: boolean;
}) => {
  const id = comment.identifier;
  const nick =
    comment.from_nick ||
    (comment.status_from ? comment.status_from.slice(0, 12) : 'you');
  const isReplyTarget = !!id && replyTargetId === id;
  const canReply = !!id;
  const children = id ? childrenOf.get(id) ?? [] : [];
  // Beyond level 5 we stop adding visual indent so deep threads don't push
  // off-screen; nesting semantics are preserved either way.
  const indentWrap = level < 5
    ? 'pl-4 ml-2 border-l border-border/40 space-y-2'
    : 'space-y-2';
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
      {(isReplyTarget || children.length > 0) && (
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
          {children.map((child) => (
            <CommentNode
              key={child.identifier || child.commentKey || `${child.timestamp}-${child.status_from}`}
              comment={child}
              level={level + 1}
              childrenOf={childrenOf}
              avatars={avatars}
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

const PostBodySegments = ({ segments, uid }: { segments: BisonrelayPostBodySegment[]; uid: string }) => (
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
          return (
            <img
              key={i}
              src={`data:${seg.mime};base64,${seg.data_b64}`}
              alt={seg.alt || seg.name || ''}
              className="rounded-lg border border-border/40 max-w-full h-auto"
            />
          );
        }
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
