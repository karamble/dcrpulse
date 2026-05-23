// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { ComponentType, useEffect, useMemo, useRef, useState } from 'react';
import {
  AlertCircle,
  Bold,
  Code,
  Eye,
  FileCode,
  FileText,
  Heading1,
  Image as ImageIcon,
  Info,
  Italic,
  Link2,
  List,
  ListOrdered,
  Loader2,
  Pencil,
  Quote,
  Strikethrough,
} from 'lucide-react';
import {
  BisonrelayPostBody,
  BisonrelayPostBodySegment,
  renderBisonrelayPostBody,
} from '../../../services/bisonrelayApi';
import { BR_PROSE_CLASSES } from '../bisonrelayProse';
import {
  EditorEmbed,
  EditorEmbedMap,
  composeBRBody,
  estimatedWireBytes,
  newEmbedId,
  placeholderFor,
} from './brEmbedBuilder';
import { SharedFilePickerModal } from './SharedFilePickerModal';

// MAX_INLINE_BYTES is the per-attachment ceiling for inline embeds. Files
// above this should use the "Link to shared content" flow instead (which
// references the bytes by FID rather than carrying them in the post).
const MAX_INLINE_BYTES = 512 * 1024;

// BR post wire-size guidance. Soft warning + hard cap.
const SOFT_WARN_BYTES = 700 * 1024;
const HARD_CAP_BYTES = 1024 * 1024;

// EditorFeatures toggles individual toolbar groups so the same editor can
// host different compose surfaces. All default to true. Examples:
//   - posts: leave everything on (default).
//   - comment field: disable `attach`, `linkContent`, `preview` to get
//     just markdown helpers.
//   - chat composer (future migration): disable `linkContent`, `preview`,
//     `sizeFooter`; attach + markdown helpers stay.
export interface EditorFeatures {
  attach?: boolean;
  linkContent?: boolean;
  markdownHelpers?: boolean;
  preview?: boolean;
  sizeFooter?: boolean;
}

interface Props {
  value: string;
  onChange: (value: string) => void;
  embeds: EditorEmbedMap;
  onEmbedsChange: (embeds: EditorEmbedMap) => void;
  placeholder?: string;
  disabled?: boolean;
  minRows?: number;
  features?: EditorFeatures;
}

type Mode = 'write' | 'preview';

const DEFAULT_FEATURES: Required<EditorFeatures> = {
  attach: true,
  linkContent: true,
  markdownHelpers: true,
  preview: true,
  sizeFooter: true,
};

export const BisonrelayEditor = ({
  value,
  onChange,
  embeds,
  onEmbedsChange,
  placeholder,
  disabled,
  minRows = 14,
  features,
}: Props) => {
  const feat = { ...DEFAULT_FEATURES, ...(features ?? {}) };
  const [mode, setMode] = useState<Mode>('write');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  const wireBytes = useMemo(() => estimatedWireBytes(value, embeds), [value, embeds]);
  const overSoft = wireBytes >= SOFT_WARN_BYTES;
  const overHard = wireBytes >= HARD_CAP_BYTES;

  // -----------------------------------------------------------------
  // Cursor splicing helpers
  // -----------------------------------------------------------------
  const spliceAtCursor = (insert: string) => {
    const el = textareaRef.current;
    if (!el) {
      onChange(value + insert);
      return;
    }
    const start = el.selectionStart ?? value.length;
    const end = el.selectionEnd ?? value.length;
    const next = value.slice(0, start) + insert + value.slice(end);
    onChange(next);
    // Place cursor after the inserted run on next tick (after rerender).
    queueMicrotask(() => {
      const pos = start + insert.length;
      el.focus();
      el.setSelectionRange(pos, pos);
    });
  };

  // wrapSelection wraps the current selection with (left, right). If
  // nothing is selected, inserts `left + placeholder + right` and selects
  // the placeholder so the user can type over it.
  const wrapSelection = (left: string, right: string, placeholder = 'text') => {
    const el = textareaRef.current;
    if (!el) {
      onChange(value + left + placeholder + right);
      return;
    }
    const start = el.selectionStart ?? value.length;
    const end = el.selectionEnd ?? value.length;
    const selected = value.slice(start, end);
    const content = selected || placeholder;
    const insert = left + content + right;
    const next = value.slice(0, start) + insert + value.slice(end);
    onChange(next);
    queueMicrotask(() => {
      el.focus();
      const innerStart = start + left.length;
      const innerEnd = innerStart + content.length;
      el.setSelectionRange(innerStart, innerEnd);
    });
  };

  // prefixSelectedLines applies `prefix` to every line touched by the
  // current selection (or just the cursor line if no selection). Used by
  // list / quote helpers.
  const prefixSelectedLines = (prefix: string | ((i: number) => string)) => {
    const el = textareaRef.current;
    if (!el) return;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    const lineStart = value.lastIndexOf('\n', start - 1) + 1;
    let lineEnd = value.indexOf('\n', end);
    if (lineEnd === -1) lineEnd = value.length;
    const block = value.slice(lineStart, lineEnd);
    const lines = block.split('\n');
    const prefixed = lines
      .map((ln, i) => `${typeof prefix === 'function' ? prefix(i) : prefix}${ln}`)
      .join('\n');
    const next = value.slice(0, lineStart) + prefixed + value.slice(lineEnd);
    onChange(next);
    queueMicrotask(() => {
      el.focus();
      el.setSelectionRange(lineStart, lineStart + prefixed.length);
    });
  };

  // cycleHeading toggles a heading prefix on the current line: none → # → ## → ### → none.
  const cycleHeading = () => {
    const el = textareaRef.current;
    if (!el) return;
    const caret = el.selectionStart ?? 0;
    const lineStart = value.lastIndexOf('\n', caret - 1) + 1;
    let lineEnd = value.indexOf('\n', caret);
    if (lineEnd === -1) lineEnd = value.length;
    const line = value.slice(lineStart, lineEnd);
    const m = line.match(/^(#{1,6})\s/);
    let next: string;
    if (!m) {
      next = `# ${line}`;
    } else if (m[1].length < 3) {
      next = '#'.repeat(m[1].length + 1) + ' ' + line.slice(m[0].length);
    } else {
      next = line.slice(m[0].length);
    }
    const updated = value.slice(0, lineStart) + next + value.slice(lineEnd);
    onChange(updated);
    queueMicrotask(() => {
      el.focus();
      const newCaret = lineStart + next.length;
      el.setSelectionRange(newCaret, newCaret);
    });
  };

  // -----------------------------------------------------------------
  // Attach (inline file → embed placeholder)
  // -----------------------------------------------------------------
  const handleFilePicked = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    setErr(null);
    if (f.size > MAX_INLINE_BYTES) {
      setErr(
        `${f.name} is ${formatBytes(f.size)}. Inline attachments must be ${formatBytes(
          MAX_INLINE_BYTES,
        )} or smaller; use "Link to shared content" for larger files.`,
      );
      return;
    }
    try {
      const buf = await f.arrayBuffer();
      const bytes = new Uint8Array(buf);
      let binStr = '';
      const chunk = 0x8000;
      for (let i = 0; i < bytes.length; i += chunk) {
        binStr += String.fromCharCode.apply(
          null,
          bytes.subarray(i, i + chunk) as unknown as number[],
        );
      }
      const dataB64 = btoa(binStr);
      const id = newEmbedId(embeds);
      const embed: EditorEmbed = {
        displayName: f.name,
        name: f.name,
        mime: f.type || 'application/octet-stream',
        dataB64,
      };
      onEmbedsChange({ ...embeds, [id]: embed });
      spliceAtCursor(placeholderFor(id));
    } catch (e: any) {
      setErr(e?.message || 'Could not read file');
    }
  };

  const handleSharedFilePicked = (embed: EditorEmbed) => {
    const id = newEmbedId(embeds);
    onEmbedsChange({ ...embeds, [id]: embed });
    spliceAtCursor(placeholderFor(id));
  };

  const removeEmbed = (id: string) => {
    const placeholder = placeholderFor(id);
    onChange(value.split(placeholder).join(''));
    const next = { ...embeds };
    delete next[id];
    onEmbedsChange(next);
  };

  // -----------------------------------------------------------------
  // Rendering
  // -----------------------------------------------------------------
  return (
    <div className="rounded-xl border border-border/50 bg-background overflow-hidden">
      <div className="flex items-center justify-between gap-2 px-2 py-1.5 border-b border-border/40 bg-muted/20">
        <div className="flex items-center gap-0.5 flex-wrap">
          {feat.attach && (
            <ToolbarButton
              icon={ImageIcon}
              label="Attach image / file"
              disabled={disabled}
              onClick={() => fileInputRef.current?.click()}
            />
          )}
          {feat.linkContent && (
            <ToolbarButton
              icon={Link2}
              label="Link to shared content"
              disabled={disabled}
              onClick={() => setPickerOpen(true)}
            />
          )}
          {(feat.attach || feat.linkContent) && feat.markdownHelpers && <ToolbarSeparator />}
          {feat.markdownHelpers && (
            <>
              <ToolbarButton
                icon={Bold}
                label="Bold"
                disabled={disabled}
                onClick={() => wrapSelection('**', '**')}
              />
              <ToolbarButton
                icon={Italic}
                label="Italic"
                disabled={disabled}
                onClick={() => wrapSelection('*', '*')}
              />
              <ToolbarButton
                icon={Strikethrough}
                label="Strikethrough"
                disabled={disabled}
                onClick={() => wrapSelection('~~', '~~')}
              />
              <ToolbarButton
                icon={Heading1}
                label="Heading"
                disabled={disabled}
                onClick={cycleHeading}
              />
              <ToolbarButton
                icon={List}
                label="Bulleted list"
                disabled={disabled}
                onClick={() => prefixSelectedLines('- ')}
              />
              <ToolbarButton
                icon={ListOrdered}
                label="Numbered list"
                disabled={disabled}
                onClick={() => prefixSelectedLines((i) => `${i + 1}. `)}
              />
              <ToolbarButton
                icon={Quote}
                label="Blockquote"
                disabled={disabled}
                onClick={() => prefixSelectedLines('> ')}
              />
              <ToolbarButton
                icon={Code}
                label="Inline code"
                disabled={disabled}
                onClick={() => wrapSelection('`', '`', 'code')}
              />
              <ToolbarButton
                icon={FileCode}
                label="Code block"
                disabled={disabled}
                onClick={() => wrapSelection('```\n', '\n```', 'code')}
              />
              <ToolbarButton
                icon={Link2}
                label="Hyperlink"
                disabled={disabled}
                onClick={() => {
                  const url = window.prompt('Link URL:');
                  if (!url) return;
                  wrapSelection('[', `](${url})`, 'link');
                }}
              />
            </>
          )}
        </div>
        {feat.preview && (
          <div className="flex items-center gap-1 shrink-0">
            <ModeButton active={mode === 'write'} icon={Pencil} onClick={() => setMode('write')}>
              Write
            </ModeButton>
            <ModeButton active={mode === 'preview'} icon={Eye} onClick={() => setMode('preview')}>
              Preview
            </ModeButton>
          </div>
        )}
      </div>

      {feat.preview && mode === 'preview' ? (
        <PreviewPane displayBody={value} embeds={embeds} />
      ) : (
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          disabled={disabled}
          rows={minRows}
          className="w-full px-3 py-2 bg-transparent text-foreground text-sm font-mono focus:outline-none disabled:opacity-50 resize-y min-h-[280px]"
        />
      )}

      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        onChange={handleFilePicked}
      />
      {pickerOpen && (
        <SharedFilePickerModal
          onClose={() => setPickerOpen(false)}
          onSubmit={handleSharedFilePicked}
        />
      )}

      {feat.sizeFooter && (
        <div className="px-3 py-2 border-t border-border/40 bg-muted/20 flex items-center justify-between gap-3 text-[11px]">
          <div className="flex items-center gap-2 flex-wrap min-w-0">
            {Object.entries(embeds).map(([id, e]) => (
              <EmbedChip key={id} id={id} embed={e} onRemove={() => removeEmbed(id)} />
            ))}
            {Object.keys(embeds).length === 0 && (
              <span className="text-muted-foreground">No attachments yet.</span>
            )}
          </div>
          <SizeIndicator wireBytes={wireBytes} overSoft={overSoft} overHard={overHard} />
        </div>
      )}
      {err && (
        <div className="px-3 py-2 border-t border-destructive/30 bg-destructive/10 flex items-start gap-2 text-xs text-destructive">
          <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      )}
    </div>
  );
};

// isEditorOverHardCap is exported so NewPostView can disable Publish when
// the editor would emit a post over the BR wire limit.
export const isEditorOverHardCap = (value: string, embeds: EditorEmbedMap): boolean =>
  estimatedWireBytes(value, embeds) >= HARD_CAP_BYTES;

const ToolbarButton = ({
  icon: Icon,
  label,
  disabled,
  onClick,
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  disabled?: boolean;
  onClick: () => void;
}) => (
  <button
    type="button"
    title={label}
    aria-label={label}
    disabled={disabled}
    onClick={onClick}
    className="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
  >
    <Icon className="h-4 w-4" />
  </button>
);

const ToolbarSeparator = () => <span className="mx-1 inline-block w-px h-5 bg-border/60" />;

const ModeButton = ({
  active,
  icon: Icon,
  children,
  onClick,
}: {
  active: boolean;
  icon: ComponentType<{ className?: string }>;
  children: React.ReactNode;
  onClick: () => void;
}) => (
  <button
    type="button"
    onClick={onClick}
    className={`px-2.5 py-1 rounded-md text-xs inline-flex items-center gap-1.5 transition-colors ${
      active
        ? 'bg-primary/20 text-primary font-semibold'
        : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
    }`}
  >
    <Icon className="h-3.5 w-3.5" />
    {children}
  </button>
);

const EmbedChip = ({
  id,
  embed,
  onRemove,
}: {
  id: string;
  embed: EditorEmbed;
  onRemove: () => void;
}) => {
  const isDownload = !!embed.download;
  const label = embed.displayName || embed.name || embed.filename || id.slice(0, 6);
  const extra = isDownload
    ? embed.cost && embed.cost > 0
      ? `pay ${formatCost(embed.cost)}`
      : 'shared link'
    : embed.mime || 'inline';
  return (
    <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-muted/40 border border-border/40 text-muted-foreground">
      {isDownload ? <Link2 className="h-3 w-3" /> : <FileText className="h-3 w-3" />}
      <span className="truncate max-w-[14ch] text-foreground">{label}</span>
      <span className="opacity-60">·</span>
      <span>{extra}</span>
      <button
        type="button"
        onClick={onRemove}
        title="Remove attachment"
        className="ml-0.5 text-muted-foreground/70 hover:text-destructive transition-colors"
      >
        ×
      </button>
    </span>
  );
};

const SizeIndicator = ({
  wireBytes,
  overSoft,
  overHard,
}: {
  wireBytes: number;
  overSoft: boolean;
  overHard: boolean;
}) => {
  const pct = Math.min(999, Math.round((wireBytes / HARD_CAP_BYTES) * 100));
  const tone = overHard
    ? 'text-destructive font-medium'
    : overSoft
      ? 'text-warning'
      : 'text-muted-foreground';
  return (
    <div className="flex items-center gap-1.5 shrink-0 whitespace-nowrap">
      <span className={tone} title="Estimated wire size of the composed post">
        {pct}% used
      </span>
      <span className="text-muted-foreground/60">·</span>
      <span className={tone}>{formatBytes(wireBytes)}</span>
      <span className="relative group inline-flex">
        <Info className="h-3.5 w-3.5 text-muted-foreground/60 hover:text-muted-foreground cursor-help" />
        <span className="pointer-events-none absolute right-0 bottom-full mb-1 w-72 p-2 rounded-md bg-popover border border-border/50 shadow-lg text-xs text-foreground/90 opacity-0 group-hover:opacity-100 transition-opacity z-10">
          BR posts ride over the same wire as chat messages; the cap is{' '}
          {formatBytes(HARD_CAP_BYTES)} per post. Inline embed bytes count
          toward the total. Larger files should be shared separately and
          referenced via the "Link to shared content" button.
        </span>
      </span>
    </div>
  );
};

function formatCost(matoms: number): string {
  const dcr = matoms / 1e11;
  return `${dcr.toFixed(8).replace(/\.?0+$/, '')} DCR`;
}

function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
}

// -----------------------------------------------------------------
// Preview tab
// -----------------------------------------------------------------
const PreviewPane = ({
  displayBody,
  embeds,
}: {
  displayBody: string;
  embeds: EditorEmbedMap;
}) => {
  const [rendered, setRendered] = useState<BisonrelayPostBody | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Recompose + re-render whenever the source changes. Debounce 300ms so
  // typing in the Write tab doesn't bombard the server.
  useEffect(() => {
    let cancelled = false;
    const wire = composeBRBody(displayBody, embeds);
    if (!wire.trim()) {
      setRendered(null);
      return () => {
        cancelled = true;
      };
    }
    const handle = window.setTimeout(async () => {
      setLoading(true);
      setErr(null);
      try {
        const body = await renderBisonrelayPostBody(wire);
        if (cancelled) return;
        setRendered(body);
      } catch (e: any) {
        if (cancelled) return;
        const body = e?.response?.data;
        setErr(typeof body === 'string' ? body : e?.message || 'Could not render preview');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(handle);
    };
  }, [displayBody, embeds]);

  return (
    <div className="px-4 py-3 min-h-[280px] space-y-3">
      {err ? (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      ) : !displayBody.trim() ? (
        <p className="text-xs text-muted-foreground italic">Nothing to preview yet.</p>
      ) : loading && !rendered ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>Rendering preview…</span>
        </div>
      ) : rendered && rendered.segments && rendered.segments.length > 0 ? (
        <PreviewSegments segments={rendered.segments} />
      ) : null}
    </div>
  );
};

const PreviewSegments = ({ segments }: { segments: BisonrelayPostBodySegment[] }) => (
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
