// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useCallback, useEffect, useState } from 'react';
import { toYMDTime } from '../../utils/date';
import {
  ArrowDownRight,
  ArrowLeft,
  ArrowUpRight,
  Check,
  Clock,
  Coins,
  Copy,
  EyeOff,
  FileText,
  Folder,
  Loader2,
  MessageSquare,
  MoreHorizontal,
  RotateCw,
  Rss,
  Shield,
  Users,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayGC,
  BisonrelayLiveEvent,
  BisonrelayPayStatsUser,
  BisonrelayPostSummary,
  BisonrelayStatsContact,
  BisonrelayTipAttempt,
  getBisonrelayContacts,
  getBisonrelayStatsContacts,
  getBisonrelayStatsPayments,
  getBisonrelayTipAttempts,
  listBisonrelayGCs,
  subscribeBisonrelayPosts,
  tipBisonrelayContact,
  unsubscribeBisonrelayPosts,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';
import { AuthorAvatar } from './AuthorAvatar';
import { FeedCard } from './FeedCard';
import { TipModal, formatTipDcr, tipAttemptState } from './TipModal';
import { BisonrelayUserSubNav, ContentListModal } from './BisonrelayUserSubNav';
import {
  Detail,
  HeroCard,
  MiniBars,
  PaymentBreakdownDetail,
  RatchetHealth,
  SectionCard,
  backupBtnCls,
  formatDCR,
  isMeaningfulDate,
  ratchetHealth,
  relativeTime,
} from './BisonrelayStats';

const healthMeta: Record<RatchetHealth, { label: string; cls: string }> = {
  green: { label: 'Active', cls: 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10' },
  amber: { label: 'Idle', cls: 'border-amber-500/40 text-amber-400 bg-amber-500/10' },
  red: { label: 'Awaiting peer', cls: 'border-rose-500/40 text-rose-400 bg-rose-500/10' },
  idle: { label: 'Offline', cls: 'border-border text-muted-foreground bg-muted/10' },
};

const pillCls = 'inline-flex items-center gap-1 px-2 py-0.5 rounded-full border text-[10px] uppercase tracking-wide';

const guardedTime = (iso?: string): string => (isMeaningfulDate(iso) ? relativeTime(iso) : '-');

// UserProfileView is the per-contact profile page (#feed/user/<uid>): the
// aggregate of everything the daemon knows about one peer - identity, payment
// balance, tip history, their posts, ratchet health, shared content and the
// contact actions.
export const UserProfileView = ({
  uid,
  ownUid,
  posts,
  avatars,
  onBack,
}: {
  uid: string;
  ownUid: string;
  posts: BisonrelayPostSummary[] | null;
  avatars: Record<string, string>;
  onBack: () => void;
}) => {
  const [contacts, setContacts] = useState<BisonrelayContact[]>([]);
  const [contactsLoaded, setContactsLoaded] = useState(false);
  const [statsContact, setStatsContact] = useState<BisonrelayStatsContact | null>(null);
  const [payUser, setPayUser] = useState<BisonrelayPayStatsUser | null>(null);
  const [tips, setTips] = useState<BisonrelayTipAttempt[]>([]);
  const [gcs, setGCs] = useState<BisonrelayGC[]>([]);
  const [showTip, setShowTip] = useState(false);
  const [showActions, setShowActions] = useState(false);
  const [showContent, setShowContent] = useState(false);
  const [copied, setCopied] = useState(false);
  const [subBusy, setSubBusy] = useState(false);
  const [tipStatus, setTipStatus] = useState<{
    state: 'requesting' | 'paying' | 'sent' | 'failed';
    line: string;
  } | null>(null);
  const { addListener } = useBisonrelayLive();

  const contact = contacts.find((c) => c.id?.identity === uid);
  const isOwn = !!ownUid && uid === ownUid;
  const theirPosts = (posts ?? [])
    .filter((p) => p.author_id === uid)
    .sort((a, b) => {
      const ta = a.last_status_ts && a.last_status_ts > a.date ? a.last_status_ts : a.date;
      const tb = b.last_status_ts && b.last_status_ts > b.date ? b.last_status_ts : b.date;
      return tb - ta;
    });
  const nick = contact
    ? contact.nick_alias || contact.id?.nick || uid.slice(0, 12)
    : theirPosts[0]?.author_nick || uid.slice(0, 12);
  const avatarB64 = contact?.id?.avatar || avatars[uid];
  const realNick = contact?.id?.nick;
  const mutualGCs = gcs.filter((g) =>
    (g.members ?? []).some((m) => m.toLowerCase() === uid.toLowerCase()),
  );
  const totalHearts = theirPosts.reduce((s, p) => s + (p.hearts_count ?? 0), 0);
  const totalComments = theirPosts.reduce((s, p) => s + (p.comments_count ?? 0), 0);
  const health = statsContact ? ratchetHealth(statsContact) : 'idle';

  const refreshContacts = useCallback(async () => {
    try {
      setContacts(await getBisonrelayContacts());
    } catch {
      /* keep the last list; the degraded header still renders */
    } finally {
      setContactsLoaded(true);
    }
  }, []);

  const refreshTips = useCallback(async () => {
    try {
      const atts = await getBisonrelayTipAttempts(uid);
      atts.sort((a, b) => Date.parse(b.created) - Date.parse(a.created));
      setTips(atts);
    } catch {
      /* older brclientd without the endpoint; the card stays hidden */
    }
  }, [uid]);

  useEffect(() => {
    refreshContacts();
    refreshTips();
    getBisonrelayStatsContacts()
      .then((cs) => setStatsContact(cs.find((c) => c.uid === uid) ?? null))
      .catch(() => {});
    getBisonrelayStatsPayments()
      .then((p) => setPayUser(p.users.find((u) => u.uid === uid) ?? null))
      .catch(() => {});
    listBisonrelayGCs()
      .then(setGCs)
      .catch(() => {});
  }, [uid, refreshContacts, refreshTips]);

  useEffect(() => {
    return addListener((evt: BisonrelayLiveEvent) => {
      if (evt.type === 'tip-invoice-generated') {
        const payload = (evt.payload ?? {}) as Record<string, unknown>;
        if (String(payload.uid ?? '') !== uid) return;
        const evNick = String(payload.nick ?? '');
        setTipStatus((prev) =>
          prev && prev.state === 'requesting'
            ? { state: 'paying', line: `Invoice received, paying tip to ${evNick}...` }
            : prev,
        );
        return;
      }
      if (evt.type === 'tip-sent' || evt.type === 'tip-failed') {
        const payload = (evt.payload ?? {}) as Record<string, string>;
        if (payload.recipient !== uid || !payload.line) return;
        setTipStatus({ state: evt.type === 'tip-sent' ? 'sent' : 'failed', line: payload.line });
        refreshTips();
        return;
      }
      if (
        evt.type === 'posts-subscribed' ||
        evt.type === 'posts-unsubscribed' ||
        evt.type === 'profile-updated'
      ) {
        refreshContacts();
      }
    });
  }, [addListener, uid, refreshContacts, refreshTips]);

  const submitTip = (dcrAmount: number) => {
    setTipStatus({
      state: 'requesting',
      line: `Requesting invoice for ${dcrAmount} DCR to tip ${nick}...`,
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

  const toggleSubscribe = async () => {
    if (!contact || subBusy) return;
    setSubBusy(true);
    try {
      if (contact.posts_subscribed) await unsubscribeBisonrelayPosts(uid);
      else await subscribeBisonrelayPosts(uid);
    } catch {
      /* flag refreshes below either way */
    } finally {
      setSubBusy(false);
      refreshContacts();
    }
  };

  const copyUid = async () => {
    try {
      await navigator.clipboard.writeText(uid);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard needs a secure context; silently skip */
    }
  };

  const kxSince = statsContact?.first_created || contact?.first_created;
  const lastKx = statsContact?.last_completed_kx || contact?.last_completed_kx;

  return (
    <div className="relative min-h-[60vh] space-y-4">
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

      <header className="rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 p-5">
        {!contactsLoaded ? (
          <div className="flex items-center gap-4 animate-pulse">
            <div className="h-16 w-16 rounded-full bg-muted/40 shrink-0" />
            <div className="flex-1 space-y-2">
              <div className="h-4 w-40 rounded bg-muted/40" />
              <div className="h-3 w-64 rounded bg-muted/40" />
            </div>
          </div>
        ) : (
          <div className="flex flex-col items-center text-center gap-4 sm:flex-row sm:items-start sm:text-left">
            <AuthorAvatar uid={uid} nick={nick} avatarB64={avatarB64} size="lg" />
            <div className="min-w-0 flex-1 space-y-2">
              <h2 className="text-lg font-semibold text-foreground break-words">
                {nick}
                {realNick && contact?.nick_alias && realNick !== contact.nick_alias && (
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (aka {realNick})
                  </span>
                )}
              </h2>
              <div className="flex flex-wrap justify-center sm:justify-start gap-1.5">
                {isOwn && (
                  <span className={`${pillCls} border-primary/40 text-primary bg-primary/10`}>
                    This is you
                  </span>
                )}
                {!isOwn && !contact && (
                  <span className={`${pillCls} border-border text-muted-foreground bg-muted/10`}>
                    Not in your contacts
                  </span>
                )}
                {contact?.posts_subscribed && (
                  <span className={`${pillCls} border-emerald-500/40 text-emerald-400 bg-emerald-500/10`}>
                    <Rss className="h-3 w-3" />
                    Subscribed
                  </span>
                )}
                {contact?.ignored && (
                  <span className={`${pillCls} border-amber-500/40 text-amber-400 bg-amber-500/10`}>
                    <EyeOff className="h-3 w-3" />
                    Ignored
                  </span>
                )}
                {!isOwn && contact && (
                  <span className={`${pillCls} ${healthMeta[health].cls}`}>
                    {healthMeta[health].label}
                  </span>
                )}
              </div>
              <button
                type="button"
                onClick={copyUid}
                title="Copy identity"
                className="inline-flex items-center gap-1.5 max-w-full rounded px-2 py-1.5 bg-muted/20 hover:bg-muted/30 text-[10px] font-mono text-muted-foreground transition-colors"
              >
                {copied ? <Check className="h-3 w-3 shrink-0" /> : <Copy className="h-3 w-3 shrink-0" />}
                <span className="min-w-0 break-all text-left">{uid}</span>
              </button>
              {(isMeaningfulDate(kxSince) || isMeaningfulDate(lastKx)) && (
                <div className="text-[11px] text-muted-foreground">
                  KX since {guardedTime(kxSince)}
                  <span className="mx-1.5 opacity-50">·</span>
                  Last completed KX {guardedTime(lastKx)}
                  {statsContact?.ratchet?.last_dec_time &&
                    isMeaningfulDate(statsContact.ratchet.last_dec_time) && (
                      <>
                        <span className="mx-1.5 opacity-50">·</span>
                        Last activity {relativeTime(statsContact.ratchet.last_dec_time)}
                      </>
                    )}
                </div>
              )}
              {tipStatus && (
                <div
                  className={`flex items-center justify-center sm:justify-start gap-2 text-xs ${
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
            {!isOwn && contact && (
              <div className="flex flex-wrap justify-center gap-2 sm:justify-end sm:max-w-[12rem]">
                <button
                  type="button"
                  onClick={() => {
                    window.location.hash = `chat/${uid}`;
                  }}
                  className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-gradient-primary text-white text-xs font-semibold transition-all"
                >
                  <MessageSquare className="h-3.5 w-3.5" />
                  Message
                </button>
                <button type="button" onClick={() => setShowTip(true)} className={backupBtnCls}>
                  <Coins className="h-3.5 w-3.5" />
                  Pay tip
                </button>
                <button
                  type="button"
                  onClick={toggleSubscribe}
                  disabled={subBusy}
                  className={backupBtnCls}
                >
                  {subBusy ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Rss className="h-3.5 w-3.5" />
                  )}
                  {contact.posts_subscribed ? 'Unsubscribe' : 'Subscribe'}
                </button>
                <button
                  type="button"
                  onClick={() => setShowActions(true)}
                  className={backupBtnCls}
                >
                  <MoreHorizontal className="h-3.5 w-3.5" />
                  More
                </button>
              </div>
            )}
          </div>
        )}
      </header>

      {!isOwn && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <HeroCard
            icon={Coins}
            label="Sent to them"
            value={formatDCR(payUser?.sent_matoms ?? 0)}
            hint={
              payUser?.fees_matoms
                ? `DCR · ${formatDCR(payUser.fees_matoms)} in fees`
                : 'DCR, tips and paid content'
            }
            tone="amber"
          />
          <HeroCard
            icon={ArrowDownRight}
            label="Received from them"
            value={formatDCR(payUser?.received_matoms ?? 0)}
            hint="DCR over Lightning"
            tone="emerald"
          />
          <HeroCard
            icon={FileText}
            label="Posts"
            value={String(theirPosts.length)}
            hint={`${totalHearts} atoms · ${totalComments} comments`}
          />
          <HeroCard
            icon={Clock}
            label="Contact age"
            value={isMeaningfulDate(kxSince) ? relativeTime(kxSince) : '-'}
            hint={`${statsContact?.ratchet?.nb_saved_keys ?? 0} saved keys`}
          />
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="lg:col-span-2 space-y-4">
          <SectionCard
            title={`Posts by ${nick}`}
            icon={Rss}
            action={
              theirPosts.length > 5 ? (
                <button
                  type="button"
                  onClick={onBack}
                  className="text-xs text-muted-foreground hover:text-foreground"
                >
                  View all in feed
                </button>
              ) : undefined
            }
          >
            {theirPosts.length === 0 ? (
              <p className="text-xs text-muted-foreground italic">
                No posts from {nick} in your feed yet.
                {contact && !contact.posts_subscribed && !isOwn
                  ? ' Subscribe to their posts to receive new ones.'
                  : ''}
              </p>
            ) : (
              <div className="space-y-3">
                {theirPosts.slice(0, 5).map((p) => (
                  <FeedCard
                    key={`${p.author_id}-${p.id}`}
                    post={p}
                    hasActivity={false}
                    avatarB64={avatars[p.author_id]}
                    ownUid={ownUid}
                    onOpen={() => {
                      window.location.hash = `feed/post/${p.author_id}/${p.id}`;
                    }}
                  />
                ))}
              </div>
            )}
          </SectionCard>

          {!isOwn && (
            <SectionCard title="Payment activity" icon={Coins}>
              {payUser ? (
                <div className="space-y-4">
                  <div className="grid grid-cols-[1fr_auto] gap-3 items-center">
                    <div className="min-w-0">
                      <MiniBars sent={payUser.sent_matoms} received={payUser.received_matoms} />
                    </div>
                    <div className="text-right text-[11px] tabular-nums">
                      <div className="text-rose-400 flex items-center justify-end gap-1">
                        <ArrowUpRight className="h-3 w-3" />
                        {formatDCR(payUser.sent_matoms)}
                      </div>
                      <div className="text-emerald-400 flex items-center justify-end gap-1">
                        <ArrowDownRight className="h-3 w-3" />
                        {formatDCR(payUser.received_matoms)}
                      </div>
                    </div>
                  </div>
                  <PaymentBreakdownDetail breakdowns={payUser.breakdowns ?? []} />
                </div>
              ) : (
                <p className="text-xs text-muted-foreground italic">
                  No payment activity with {nick} yet.
                </p>
              )}
            </SectionCard>
          )}
        </div>

        <div className="space-y-4">
          {!isOwn && contact && (
            <SectionCard
              title="Connection health"
              icon={Shield}
              action={
                <span className={`${pillCls} ${healthMeta[health].cls}`}>
                  {healthMeta[health].label}
                </span>
              }
            >
              <div className="grid grid-cols-2 gap-3 text-xs">
                <Detail label="KX since">{guardedTime(kxSince)}</Detail>
                <Detail label="Last completed KX">{guardedTime(lastKx)}</Detail>
                <Detail label="Last handshake">
                  {guardedTime(statsContact?.last_handshake_attempt)}
                </Detail>
                <Detail label="Saved keys">
                  {statsContact?.ratchet ? `${statsContact.ratchet.nb_saved_keys}` : '-'}
                </Detail>
                <Detail label="Will ratchet">
                  {statsContact?.ratchet ? (statsContact.ratchet.will_ratchet ? 'Yes' : 'No') : '-'}
                </Detail>
                <Detail label="Last encrypted">
                  {guardedTime(statsContact?.ratchet?.last_enc_time)}
                </Detail>
                <Detail label="Last decrypted">
                  {guardedTime(statsContact?.ratchet?.last_dec_time)}
                </Detail>
              </div>
              <button
                type="button"
                onClick={() => setShowActions(true)}
                className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                <RotateCw className="h-3.5 w-3.5" />
                Request ratchet reset
              </button>
            </SectionCard>
          )}

          {!isOwn && contact && (
            <SectionCard title="Shared files & pages" icon={Folder}>
              <div className="flex flex-col items-stretch gap-2">
                <button type="button" onClick={() => setShowContent(true)} className={backupBtnCls}>
                  <Folder className="h-3.5 w-3.5" />
                  Browse shared content
                </button>
                <button
                  type="button"
                  onClick={() => {
                    window.location.hash = `pages/visit/${uid}/index.md`;
                  }}
                  className={backupBtnCls}
                >
                  <FileText className="h-3.5 w-3.5" />
                  View pages
                </button>
              </div>
            </SectionCard>
          )}

          {mutualGCs.length > 0 && (
            <SectionCard title="Group chats in common" icon={Users}>
              <div className="flex flex-wrap gap-2">
                {mutualGCs.map((g) => (
                  <span
                    key={g.id}
                    className="px-2.5 py-1 rounded-full bg-muted/20 text-xs text-foreground/90"
                  >
                    {g.alias || g.name}
                  </span>
                ))}
              </div>
            </SectionCard>
          )}

          {!isOwn && contact && (
            <SectionCard
              title="Tip history"
              icon={Coins}
              action={
                <button type="button" onClick={() => setShowTip(true)} className={backupBtnCls}>
                  <Coins className="h-3.5 w-3.5" />
                  Pay tip
                </button>
              }
            >
              {tips.length === 0 ? (
                <p className="text-xs text-muted-foreground italic">No tips sent to {nick} yet.</p>
              ) : (
                <div className="space-y-1">
                  {tips.slice(0, 10).map((a) => (
                    <div
                      key={`${a.tag}-${a.created}`}
                      className="flex items-center gap-2 text-[11px] text-muted-foreground"
                      title={a.last_invoice_error || undefined}
                    >
                      <span className="font-medium text-foreground/90 tabular-nums">
                        {formatTipDcr(a.amount_matoms)} DCR
                      </span>
                      <span className="opacity-50">·</span>
                      <span>{toYMDTime(new Date(a.created))}</span>
                      <span className="opacity-50">·</span>
                      <span
                        className={
                          tipAttemptState(a) === 'completed'
                            ? 'text-success/90'
                            : tipAttemptState(a) === 'failed'
                              ? 'text-destructive/90'
                              : undefined
                        }
                      >
                        {tipAttemptState(a)}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </SectionCard>
          )}
        </div>
      </div>

      {showTip && (
        <TipModal nick={nick} uid={uid} onClose={() => setShowTip(false)} onSubmit={submitTip} />
      )}
      {showContent && (
        <ContentListModal nick={nick} uid={uid} onClose={() => setShowContent(false)} />
      )}
      {showActions && contact && (
        <BisonrelayUserSubNav
          contact={contact}
          nick={nick}
          contacts={contacts}
          displayNick={(c) => c.nick_alias || c.id?.nick || (c.id?.identity ?? '').slice(0, 12)}
          onClose={() => setShowActions(false)}
          onSendFile={() => {
            // The chat page owns the file-attach flow; degrade to opening
            // the conversation.
            setShowActions(false);
            window.location.hash = `chat/${uid}`;
          }}
          onRenamed={() => refreshContacts()}
          onContactsChanged={() => refreshContacts()}
        />
      )}
    </div>
  );
};
