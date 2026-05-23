// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import {
  AlertCircle,
  ArrowUpRight,
  Ban,
  Crown,
  Edit2,
  Loader2,
  LogOut,
  RefreshCw,
  Shield,
  Skull,
  Trash2,
  UserCheck,
  UserMinus,
  Users,
  X,
} from 'lucide-react';
import {
  BisonrelayContact,
  BisonrelayGC,
  aliasBisonrelayGC,
  blockInBisonrelayGC,
  getBisonrelayGCDetail,
  kickFromBisonrelayGC,
  killBisonrelayGC,
  modifyBisonrelayGCAdmins,
  modifyBisonrelayGCOwner,
  partBisonrelayGC,
  resendBisonrelayGCList,
  unblockInBisonrelayGC,
  upgradeBisonrelayGCVersion,
} from '../../../services/bisonrelayApi';

// GroupSubNav is the per-group sliding sidebar (mirror of
// BisonrelayUserSubNav for contacts). Admin-only actions are gated on
// gc.local_is_admin; owner-only actions on gc.local_is_owner.
export const GroupSubNav = ({
  gc,
  contactsByUid,
  onClose,
  onMutated,
  onPartedOrKilled,
}: {
  gc: BisonrelayGC;
  contactsByUid: Map<string, BisonrelayContact>;
  onClose: () => void;
  onMutated: () => void;
  onPartedOrKilled: () => void;
}) => {
  const [detail, setDetail] = useState<BisonrelayGC>(gc);
  const [err, setErr] = useState<string | null>(null);
  const [modal, setModal] = useState<
    | { kind: 'alias' }
    | { kind: 'part' }
    | { kind: 'kill' }
    | { kind: 'upgrade' }
    | { kind: 'kick' | 'block' | 'unblock' | 'promote' | 'demote' | 'transfer-owner'; uid: string; nick: string }
    | null
  >(null);

  // Refresh detail (including blocklist) on mount + whenever the parent
  // signals an underlying change via onMutated() (which re-renders this
  // with a fresh gc prop). Re-fetching here ensures blocklist is current.
  useEffect(() => {
    let cancelled = false;
    getBisonrelayGCDetail(gc.id)
      .then((d) => {
        if (!cancelled) setDetail(d);
      })
      .catch((e: any) => {
        if (cancelled) return;
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not load group');
      });
    return () => {
      cancelled = true;
    };
  }, [gc.id, gc.generation]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (modal) setModal(null);
        else onClose();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose, modal]);

  const nickFor = (uid: string): string => {
    const c = contactsByUid.get(uid);
    return c?.nick_alias || c?.id?.nick || uid.slice(0, 12);
  };

  const adminSet = new Set([detail.owner, ...(detail.extra_admins ?? [])]);
  const blockedSet = new Set(detail.blocked ?? []);
  const selfUid = detail.owner; // approximate; only used for "you" label fallback

  const memberRows = detail.members.map((uid) => ({
    uid,
    isOwner: uid === detail.owner,
    isAdmin: adminSet.has(uid),
    isBlocked: blockedSet.has(uid),
    isSelf: detail.local_is_owner && uid === selfUid,
  }));

  return (
    <>
      <div
        className="absolute inset-0 bg-black/20 backdrop-blur-[1px] z-10 rounded-xl"
        onClick={onClose}
        aria-hidden
      />
      <aside
        className="absolute right-0 top-0 bottom-0 w-72 flex flex-col rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 z-20 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="p-3 border-b border-border/50 flex items-start gap-2">
          <div className="h-10 w-10 rounded-full bg-primary/15 border border-primary/30 flex items-center justify-center shrink-0">
            <Users className="h-5 w-5 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-sm font-semibold truncate">{detail.alias || detail.name}</h3>
            <p className="text-[10px] text-muted-foreground font-mono break-all">
              {detail.id.slice(0, 24)}…
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </header>

        {err && (
          <div className="m-3 p-2 rounded-md bg-destructive/10 border border-destructive/30 text-xs text-destructive flex items-start gap-1.5">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-3 py-2 space-y-3">
          <section>
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground px-1 mb-1">
              Members ({memberRows.length})
            </div>
            <div className="space-y-1">
              {memberRows.map((m) => (
                <div
                  key={m.uid}
                  className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-muted/20"
                >
                  <span className="text-xs truncate flex-1">{nickFor(m.uid)}</span>
                  {m.isOwner ? (
                    <Crown className="h-3 w-3 text-amber-400" />
                  ) : m.isAdmin ? (
                    <Shield className="h-3 w-3 text-primary" />
                  ) : null}
                  {m.isBlocked && <Ban className="h-3 w-3 text-rose-400" />}
                  {detail.local_is_admin && !m.isOwner && !m.isSelf && (
                    <div className="flex items-center gap-0.5">
                      <button
                        type="button"
                        onClick={() =>
                          setModal({ kind: 'kick', uid: m.uid, nick: nickFor(m.uid) })
                        }
                        title="Kick from group"
                        className="p-1 rounded text-muted-foreground hover:text-rose-400 hover:bg-rose-500/10"
                      >
                        <UserMinus className="h-3 w-3" />
                      </button>
                      {m.isBlocked ? (
                        <button
                          type="button"
                          onClick={() =>
                            setModal({ kind: 'unblock', uid: m.uid, nick: nickFor(m.uid) })
                          }
                          title="Unblock (re-show their messages)"
                          className="p-1 rounded text-muted-foreground hover:text-emerald-400 hover:bg-emerald-500/10"
                        >
                          <UserCheck className="h-3 w-3" />
                        </button>
                      ) : (
                        <button
                          type="button"
                          onClick={() =>
                            setModal({ kind: 'block', uid: m.uid, nick: nickFor(m.uid) })
                          }
                          title="Block locally (hide their messages)"
                          className="p-1 rounded text-muted-foreground hover:text-rose-400 hover:bg-rose-500/10"
                        >
                          <Ban className="h-3 w-3" />
                        </button>
                      )}
                    </div>
                  )}
                  {detail.local_is_owner && !m.isOwner && (
                    <div className="flex items-center gap-0.5">
                      {detail.version >= 1 &&
                        (m.isAdmin ? (
                          <button
                            type="button"
                            onClick={() =>
                              setModal({ kind: 'demote', uid: m.uid, nick: nickFor(m.uid) })
                            }
                            title="Demote from admin"
                            className="p-1 rounded text-muted-foreground hover:text-amber-400 hover:bg-amber-500/10"
                          >
                            <Shield className="h-3 w-3" strokeDasharray="2" />
                          </button>
                        ) : (
                          <button
                            type="button"
                            onClick={() =>
                              setModal({ kind: 'promote', uid: m.uid, nick: nickFor(m.uid) })
                            }
                            title="Promote to admin"
                            className="p-1 rounded text-muted-foreground hover:text-primary hover:bg-primary/10"
                          >
                            <Shield className="h-3 w-3" />
                          </button>
                        ))}
                      <button
                        type="button"
                        onClick={() =>
                          setModal({ kind: 'transfer-owner', uid: m.uid, nick: nickFor(m.uid) })
                        }
                        title="Transfer ownership"
                        className="p-1 rounded text-muted-foreground hover:text-amber-400 hover:bg-amber-500/10"
                      >
                        <ArrowUpRight className="h-3 w-3" />
                      </button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </section>

          <section>
            <div className="text-[10px] uppercase tracking-wide text-muted-foreground px-1 mb-1">
              Actions
            </div>
            <ActionButton
              icon={Edit2}
              label="Rename locally (alias)"
              onClick={() => setModal({ kind: 'alias' })}
            />
            {detail.local_is_admin && (
              <ActionButton
                icon={RefreshCw}
                label="Resend member list"
                onClick={async () => {
                  try {
                    await resendBisonrelayGCList(detail.id);
                  } catch (e: any) {
                    const body = e?.response?.data;
                    setErr(typeof body === 'string' ? body : e?.message || 'Resend failed');
                  }
                }}
              />
            )}
            {detail.local_is_owner && detail.version < 1 && (
              <ActionButton
                icon={ArrowUpRight}
                label="Upgrade to v1 (enables admins)"
                onClick={() => setModal({ kind: 'upgrade' })}
              />
            )}
            {!detail.local_is_owner ? (
              <ActionButton
                icon={LogOut}
                label="Leave group"
                tone="rose"
                onClick={() => setModal({ kind: 'part' })}
              />
            ) : (
              <ActionButton
                icon={Skull}
                label="Dissolve group (owner)"
                tone="rose"
                onClick={() => setModal({ kind: 'kill' })}
              />
            )}
          </section>
        </div>
      </aside>

      {modal?.kind === 'alias' && (
        <PromptModal
          title="Rename locally"
          body="The alias is shown in your sidebar only. Other members still see the original name."
          initial={detail.alias}
          placeholder={detail.name}
          confirmLabel="Save"
          onClose={() => setModal(null)}
          onConfirm={async (val) => {
            await aliasBisonrelayGC(detail.id, val);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'part' && (
        <ConfirmModal
          title="Leave group?"
          body="You will no longer receive messages from this group. The other members are notified."
          confirmLabel="Leave"
          tone="rose"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await partBisonrelayGC(detail.id);
            onPartedOrKilled();
          }}
        />
      )}
      {modal?.kind === 'kill' && (
        <ConfirmModal
          title="Dissolve group?"
          body="All members are removed. The group is destroyed for everyone, not just you. This cannot be undone."
          confirmLabel="Dissolve"
          tone="rose"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await killBisonrelayGC(detail.id);
            onPartedOrKilled();
          }}
        />
      )}
      {modal?.kind === 'upgrade' && (
        <ConfirmModal
          title="Upgrade to v1?"
          body="v1 allows extra admins (in addition to the owner) and lets you transfer ownership. This is a one-way upgrade and all members must support v1."
          confirmLabel="Upgrade"
          tone="primary"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await upgradeBisonrelayGCVersion(detail.id, 1);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'kick' && (
        <ConfirmModal
          title={`Kick ${modal.nick}?`}
          body="They are removed from the group. Other members are notified. They can be re-invited later."
          confirmLabel="Kick"
          tone="rose"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await kickFromBisonrelayGC(detail.id, modal.uid);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'block' && (
        <ConfirmModal
          title={`Block ${modal.nick} locally?`}
          body="Their messages stop appearing in your view. Other members still see them; this is a client-side filter only."
          confirmLabel="Block"
          tone="rose"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await blockInBisonrelayGC(detail.id, modal.uid);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'unblock' && (
        <ConfirmModal
          title={`Unblock ${modal.nick}?`}
          body="Their messages will appear again in your view."
          confirmLabel="Unblock"
          tone="primary"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await unblockInBisonrelayGC(detail.id, modal.uid);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'promote' && (
        <ConfirmModal
          title={`Promote ${modal.nick} to admin?`}
          body="They will be able to invite, kick, and modify the group."
          confirmLabel="Promote"
          tone="primary"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            const next = [...(detail.extra_admins ?? []), modal.uid];
            await modifyBisonrelayGCAdmins(detail.id, next);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'demote' && (
        <ConfirmModal
          title={`Demote ${modal.nick}?`}
          body="They become a regular member and lose admin privileges."
          confirmLabel="Demote"
          tone="primary"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            const next = (detail.extra_admins ?? []).filter((u) => u !== modal.uid);
            await modifyBisonrelayGCAdmins(detail.id, next);
            onMutated();
          }}
        />
      )}
      {modal?.kind === 'transfer-owner' && (
        <ConfirmModal
          title={`Transfer ownership to ${modal.nick}?`}
          body="You become a regular member (still an admin). The new owner can dissolve the group. Cannot be undone without their cooperation."
          confirmLabel="Transfer"
          tone="rose"
          onClose={() => setModal(null)}
          onConfirm={async () => {
            await modifyBisonrelayGCOwner(detail.id, modal.uid);
            onMutated();
          }}
        />
      )}
    </>
  );
};

const ActionButton = ({
  icon: Icon,
  label,
  tone = 'muted',
  onClick,
}: {
  icon: typeof Trash2;
  label: string;
  tone?: 'muted' | 'primary' | 'rose';
  onClick: () => void;
}) => {
  const toneClass = {
    muted: 'text-muted-foreground hover:text-foreground hover:bg-muted/30',
    primary: 'text-primary hover:bg-primary/10',
    rose: 'text-rose-400 hover:bg-rose-500/10',
  }[tone];
  return (
    <button
      type="button"
      onClick={onClick}
      className={`w-full px-2 py-2 rounded-md text-left flex items-center gap-2 text-sm transition-colors ${toneClass}`}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span>{label}</span>
    </button>
  );
};

// PromptModal: single-input + Save / Cancel.
const PromptModal = ({
  title,
  body,
  initial,
  placeholder,
  confirmLabel,
  onClose,
  onConfirm,
}: {
  title: string;
  body: string;
  initial?: string;
  placeholder?: string;
  confirmLabel: string;
  onClose: () => void;
  onConfirm: (val: string) => Promise<void>;
}) => {
  const [val, setVal] = useState(initial ?? '');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setErr(null);
    try {
      await onConfirm(val);
      onClose();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Action failed');
      setBusy(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl"
      >
        <form onSubmit={submit}>
          <div className="p-5 pb-3 space-y-3">
            <h3 className="text-base font-semibold">{title}</h3>
            <p className="text-xs text-muted-foreground">{body}</p>
            <input
              type="text"
              autoFocus
              value={val}
              onChange={(e) => setVal(e.target.value)}
              placeholder={placeholder}
              disabled={busy}
              maxLength={64}
              className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground text-sm focus:outline-none focus:border-primary disabled:opacity-50"
            />
            {err && (
              <div className="flex items-start gap-2 text-xs text-destructive">
                <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span className="break-words">{err}</span>
              </div>
            )}
          </div>
          <div className="border-t border-border/40 p-3 flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={busy}
              className="px-3 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={busy}
              className="px-3 py-1.5 rounded-md text-xs bg-gradient-primary text-white font-semibold inline-flex items-center gap-1.5 disabled:opacity-50"
            >
              {busy && <Loader2 className="h-3 w-3 animate-spin" />}
              {confirmLabel}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};

// ConfirmModal: yes / no with a tone class on the confirm button.
const ConfirmModal = ({
  title,
  body,
  confirmLabel,
  tone = 'primary',
  onClose,
  onConfirm,
}: {
  title: string;
  body: string;
  confirmLabel: string;
  tone?: 'primary' | 'rose';
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (busy) return;
    setBusy(true);
    setErr(null);
    try {
      await onConfirm();
      onClose();
    } catch (e: any) {
      const body = e?.response?.data;
      setErr(typeof body === 'string' ? body : e?.message || 'Action failed');
      setBusy(false);
    }
  };

  const toneClass =
    tone === 'rose'
      ? 'bg-rose-500/20 text-rose-300 border border-rose-500/40'
      : 'bg-gradient-primary text-white';

  return (
    <div
      className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl"
      >
        <div className="p-5 pb-3 space-y-3">
          <h3 className="text-base font-semibold">{title}</h3>
          <p className="text-xs text-muted-foreground">{body}</p>
          {err && (
            <div className="flex items-start gap-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span className="break-words">{err}</span>
            </div>
          )}
        </div>
        <div className="border-t border-border/40 p-3 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={busy}
            className="px-3 py-1.5 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-muted/30 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className={`px-3 py-1.5 rounded-md text-xs font-semibold inline-flex items-center gap-1.5 disabled:opacity-50 ${toneClass}`}
          >
            {busy && <Loader2 className="h-3 w-3 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};
