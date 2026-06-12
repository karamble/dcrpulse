// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Check, Edit2, Lock, Trash2, X } from 'lucide-react';
import {
  ARCHIVED_GROUP_ID,
  assignBisonrelayContactGroup,
  createBisonrelayContactGroup,
  deleteBisonrelayContactGroup,
  renameBisonrelayContactGroup,
  setBisonrelayContactGroupSettings,
} from '../../services/bisonrelayApi';
import { useBisonrelayLive } from './BisonrelayLiveProvider';

const inputCls =
  'w-full px-2 py-1.5 rounded-lg bg-background border border-border/50 text-sm focus:outline-none focus:border-primary/50';
const primaryBtnCls =
  'px-3 py-1.5 rounded-lg bg-gradient-primary text-primary-foreground text-xs font-semibold disabled:opacity-50 disabled:cursor-not-allowed';
const mutedBtnCls =
  'px-3 py-1.5 rounded-lg bg-muted/30 border border-border text-xs font-semibold hover:bg-muted/50';

const errMsg = (e: any): string => {
  const body = e?.response?.data;
  return typeof body === 'string' ? body : e?.message || 'Action failed';
};

// ContactGroupModal moves a single contact (keyed by uid, never by nick)
// into a group: the regular contact list, a custom group, or Archived with
// an optional pin that keeps it archived when new messages arrive.
export const ContactGroupModal = ({
  uid,
  nick,
  onClose,
}: {
  uid: string;
  nick: string;
  onClose: () => void;
}) => {
  const { contactGroups, refreshContactGroups } = useBisonrelayLive();
  const current = contactGroups?.contacts?.[uid];
  const [group, setGroup] = useState(current?.group ?? '');
  const [pinned, setPinned] = useState(!!current?.pinned);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await assignBisonrelayContactGroup(uid, group, group === ARCHIVED_GROUP_ID && pinned);
      refreshContactGroups();
      onClose();
    } catch (e: any) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4" onClick={onClose}>
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-sm rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold pr-4">Contact group for {nick}</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <select className={inputCls} value={group} onChange={(e) => setGroup(e.target.value)}>
          <option value="">Contact list</option>
          {(contactGroups?.groups ?? []).map((g) => (
            <option key={g.id} value={g.id}>
              {g.name}
            </option>
          ))}
          <option value={ARCHIVED_GROUP_ID}>Archived</option>
        </select>
        <label
          className={`flex items-center gap-2 text-xs ${
            group === ARCHIVED_GROUP_ID ? 'text-foreground' : 'text-muted-foreground opacity-50'
          }`}
        >
          <input
            type="checkbox"
            checked={pinned}
            disabled={group !== ARCHIVED_GROUP_ID}
            onChange={(e) => setPinned(e.target.checked)}
          />
          Keep archived even when new messages arrive (pin)
        </label>
        <p className="text-xs text-muted-foreground">
          Archived contacts still receive messages but produce no unread bubbles.
        </p>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
        <div className="flex gap-2 pt-1">
          <button type="button" onClick={onClose} className={mutedBtnCls}>
            Cancel
          </button>
          <button type="button" onClick={save} disabled={busy} className={primaryBtnCls}>
            Save
          </button>
        </div>
      </div>
    </div>
  );
};

// GroupManagementModal creates, renames, and deletes the user-defined
// contact groups and sets the auto-archive threshold. Deleting a group
// returns its members to the regular contact list. The Archived group is
// builtin and locked.
export const GroupManagementModal = ({ onClose }: { onClose: () => void }) => {
  const { contactGroups, refreshContactGroups } = useBisonrelayLive();
  const [newName, setNewName] = useState('');
  const [renamingID, setRenamingID] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const [deletingID, setDeletingID] = useState<string | null>(null);
  const [days, setDays] = useState<number>(contactGroups?.auto_archive_days ?? 30);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (contactGroups?.auto_archive_days !== undefined) setDays(contactGroups.auto_archive_days);
  }, [contactGroups?.auto_archive_days]);

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      refreshContactGroups();
      return true;
    } catch (e: any) {
      setErr(errMsg(e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const memberCount = (id: string) =>
    Object.values(contactGroups?.contacts ?? {}).filter((a) => a.group === id).length;

  return (
    <div className="fixed inset-0 z-30 bg-black/60 flex items-center justify-center p-4" onClick={onClose}>
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-card border border-border/50 shadow-2xl p-5 space-y-4"
      >
        <div className="flex items-start justify-between">
          <h3 className="text-base font-semibold pr-4">Contact groups</h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1 -mt-1 -mr-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30 transition-colors"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-1.5">
          {(contactGroups?.groups ?? []).map((g) => (
            <div
              key={g.id}
              className="flex items-center gap-2 px-3 py-2 rounded-lg bg-muted/10 border border-border/50 text-sm"
            >
              {renamingID === g.id ? (
                <>
                  <input
                    className={inputCls}
                    value={renameValue}
                    autoFocus
                    onChange={(e) => setRenameValue(e.target.value)}
                  />
                  <button
                    type="button"
                    disabled={busy}
                    onClick={async () => {
                      if (await run(() => renameBisonrelayContactGroup(g.id, renameValue))) {
                        setRenamingID(null);
                      }
                    }}
                    className="p-1.5 rounded text-emerald-400 hover:bg-emerald-500/10"
                    title="Save name"
                  >
                    <Check className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setRenamingID(null)}
                    className="p-1.5 rounded text-muted-foreground hover:bg-muted/30"
                    title="Cancel"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </>
              ) : deletingID === g.id ? (
                <>
                  <span className="flex-1 truncate text-xs">
                    Delete "{g.name}"? Members return to the contact list.
                  </span>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={async () => {
                      if (await run(() => deleteBisonrelayContactGroup(g.id))) {
                        setDeletingID(null);
                      }
                    }}
                    className="px-2 py-1 rounded text-xs text-rose-400 hover:bg-rose-500/10 font-semibold"
                  >
                    Delete
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeletingID(null)}
                    className="px-2 py-1 rounded text-xs text-muted-foreground hover:bg-muted/30"
                  >
                    Cancel
                  </button>
                </>
              ) : (
                <>
                  <span className="flex-1 truncate">{g.name}</span>
                  <span className="text-[10px] text-muted-foreground tabular-nums">
                    {memberCount(g.id)}
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setRenamingID(g.id);
                      setRenameValue(g.name);
                    }}
                    className="p-1.5 rounded text-muted-foreground hover:text-foreground hover:bg-muted/30"
                    title="Rename group"
                  >
                    <Edit2 className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeletingID(g.id)}
                    className="p-1.5 rounded text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                    title="Delete group"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </>
              )}
            </div>
          ))}
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-muted/10 border border-border/50 text-sm">
            <span className="flex-1 truncate">Archived</span>
            <span className="text-[10px] text-muted-foreground tabular-nums">
              {memberCount(ARCHIVED_GROUP_ID)}
            </span>
            <Lock className="h-3.5 w-3.5 text-muted-foreground" aria-label="Builtin group" />
          </div>
        </div>

        <div className="flex gap-2">
          <input
            className={inputCls}
            placeholder="New group name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
          <button
            type="button"
            disabled={busy || !newName.trim()}
            onClick={async () => {
              if (await run(() => createBisonrelayContactGroup(newName.trim()))) {
                setNewName('');
              }
            }}
            className={primaryBtnCls}
          >
            Create
          </button>
        </div>

        <div className="flex items-center justify-between gap-3 pt-1 border-t border-border/30">
          <div className="text-xs text-muted-foreground pt-3">
            Auto-archive contacts unheard for
          </div>
          <div className="flex items-center gap-2 pt-3">
            <input
              type="number"
              min={0}
              max={3650}
              className={`${inputCls} w-20 text-right`}
              value={days}
              onChange={(e) => setDays(parseInt(e.target.value, 10) || 0)}
            />
            <span className="text-xs text-muted-foreground">days</span>
            <button
              type="button"
              disabled={busy}
              onClick={() => run(() => setBisonrelayContactGroupSettings(days))}
              className={mutedBtnCls}
            >
              Save
            </button>
          </div>
        </div>
        <p className="text-[10px] text-muted-foreground">
          0 disables auto-archiving. Auto-archived contacts return to the contact list when
          they message again; pinned ones stay archived.
        </p>
        {err && (
          <div className="flex items-start gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span className="break-words">{err}</span>
          </div>
        )}
      </div>
    </div>
  );
};
