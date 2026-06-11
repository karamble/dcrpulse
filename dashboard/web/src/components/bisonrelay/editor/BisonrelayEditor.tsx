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
  FileSymlink,
  FileText,
  FormInput,
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
  SquareStack,
  Strikethrough,
} from 'lucide-react';
import {
  BisonrelayPageFormField,
  BisonrelayPageSegment,
  BisonrelayPostBodySegment,
  renderBisonrelayPageBody,
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
import { ImageAttachModal, ImageAttachResult, isCompressibleImage } from './ImageAttachModal';
import { blobToDataB64 } from './imageCompress';

// MAX_INLINE_BYTES is the per-attachment ceiling for inline embeds. Files
// above this should use the "Link to shared content" flow instead (which
// references the bytes by FID rather than carrying them in the post).
const MAX_INLINE_BYTES = 512 * 1024;

// BR post wire-size guidance. Soft warning + hard cap.
const SOFT_WARN_BYTES = 700 * 1024;
const HARD_CAP_BYTES = 1024 * 1024;

// EditorFeatures toggles individual toolbar groups so the same editor can
// host different compose surfaces. Examples:
//   - posts / Politeia (default): everything on, pageBlocks off.
//   - comment field: disable `attach`, `linkContent`, `preview` to get
//     just markdown helpers.
//   - chat composer (future migration): disable `linkContent`, `preview`,
//     `sizeFooter`; attach + markdown helpers stay.
//   - Bison Relay pages: enable `pageBlocks` for the page-only --form--,
//     --section-- and subpage-link inserts (irrelevant to posts/Politeia).
export interface EditorFeatures {
  attach?: boolean;
  linkContent?: boolean;
  markdownHelpers?: boolean;
  preview?: boolean;
  sizeFooter?: boolean;
  pageBlocks?: boolean;
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
  // Page-only constructs are off by default; only the Bison Relay pages
  // editor opts in. Posts and Politeia proposals never use them.
  pageBlocks: false,
};

// Templates the pageBlocks toolbar inserts at the cursor. The --section--
// marker keeps BR's exact "--section id=ID --" spelling (trailing space before
// the closing dashes); the form mirrors bruig's field syntax.
const FORM_SNIPPET = `
--form--
type="action" value="/submit"
type="txtinput" label="Your name" name="name"
type="submit" label="Submit"
--/form--
`;

const SECTION_SNIPPET = `
--section id=section1 --
Section content.
--/section--
`;

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
  const [pendingImage, setPendingImage] = useState<File | null>(null);
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
  const addInlineEmbed = (embed: EditorEmbed) => {
    const id = newEmbedId(embeds);
    onEmbedsChange({ ...embeds, [id]: embed });
    spliceAtCursor(placeholderFor(id));
  };

  const handleFilePicked = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    // Reset before any async work so re-picking the same file re-fires.
    e.target.value = '';
    if (!f) return;
    setErr(null);
    // Images go through the preview/compress modal, which also makes
    // over-cap photos attachable via compression instead of rejecting them.
    if (isCompressibleImage(f.type)) {
      setPendingImage(f);
      return;
    }
    if (f.size > MAX_INLINE_BYTES) {
      setErr(
        `${f.name} is ${formatBytes(f.size)}. Inline attachments must be ${formatBytes(
          MAX_INLINE_BYTES,
        )} or smaller; use "Link to shared content" for larger files.`,
      );
      return;
    }
    try {
      const dataB64 = await blobToDataB64(f);
      addInlineEmbed({
        displayName: f.name,
        name: f.name,
        mime: f.type || 'application/octet-stream',
        dataB64,
      });
    } catch (e: any) {
      setErr(e?.message || 'Could not read file');
    }
  };

  const handleImageAttach = (r: ImageAttachResult) => {
    addInlineEmbed({
      displayName: r.displayName,
      name: r.name,
      mime: r.mime,
      dataB64: r.dataB64,
      alt: r.alt,
    });
    setPendingImage(null);
  };

  const handleSharedFilePicked = (embed: EditorEmbed) => {
    addInlineEmbed(embed);
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
          {feat.markdownHelpers && feat.pageBlocks && <ToolbarSeparator />}
          {feat.pageBlocks && (
            <>
              <ToolbarButton
                icon={FormInput}
                label="Insert form block"
                disabled={disabled}
                onClick={() => spliceAtCursor(FORM_SNIPPET)}
              />
              <ToolbarButton
                icon={SquareStack}
                label="Insert section block"
                disabled={disabled}
                onClick={() => spliceAtCursor(SECTION_SNIPPET)}
              />
              <ToolbarButton
                icon={FileSymlink}
                label="Insert page link"
                disabled={disabled}
                onClick={() => {
                  const target = window.prompt('Page path (e.g. about.md) or br://<uid>/<path>:');
                  if (!target) return;
                  wrapSelection('[', `](${target})`, 'link text');
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
        <PreviewPane displayBody={value} embeds={embeds} page={feat.pageBlocks} />
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
      {pendingImage && (
        <ImageAttachModal
          file={pendingImage}
          maxInlineBytes={MAX_INLINE_BYTES}
          onCancel={() => setPendingImage(null)}
          onAttach={handleImageAttach}
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
        <span className="pointer-events-none absolute right-0 bottom-full mb-1 w-72 p-2 rounded-md bg-background border border-border/50 shadow-lg text-xs text-foreground/90 opacity-0 group-hover:opacity-100 transition-opacity z-10">
          BR posts ride over the same wire as chat messages; the cap is{' '}
          {formatBytes(HARD_CAP_BYTES)} per post. Inline embed bytes count
          toward the total. Larger files should be shared separately and
          referenced via the "Link to shared content" button.
        </span>
      </span>
    </div>
  );
};

// Embed download costs are in atoms (1 DCR = 1e8), not the milli-atoms used
// for payment/tip records.
function formatCost(atoms: number): string {
  const dcr = atoms / 1e8;
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
  page,
}: {
  displayBody: string;
  embeds: EditorEmbedMap;
  page?: boolean;
}) => {
  const [postSegs, setPostSegs] = useState<BisonrelayPostBodySegment[] | null>(null);
  const [pageSegs, setPageSegs] = useState<BisonrelayPageSegment[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Recompose + re-render whenever the source changes. Debounce 300ms so
  // typing in the Write tab doesn't bombard the server. A pages editor renders
  // through SplitAndRenderBRPage (forms/sections/br:// links); everything else
  // through the post renderer, so each Preview matches its published surface.
  useEffect(() => {
    let cancelled = false;
    const wire = composeBRBody(displayBody, embeds);
    if (!wire.trim()) {
      setPostSegs(null);
      setPageSegs(null);
      return () => {
        cancelled = true;
      };
    }
    const handle = window.setTimeout(async () => {
      setLoading(true);
      setErr(null);
      try {
        if (page) {
          const body = await renderBisonrelayPageBody(wire);
          if (!cancelled) setPageSegs(body.segments ?? []);
        } else {
          const body = await renderBisonrelayPostBody(wire);
          if (!cancelled) setPostSegs(body.segments ?? []);
        }
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
  }, [displayBody, embeds, page]);

  const hasContent = page
    ? !!pageSegs && pageSegs.length > 0
    : !!postSegs && postSegs.length > 0;

  return (
    <div className="px-4 py-3 min-h-[280px] space-y-3">
      {err ? (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span className="break-words">{err}</span>
        </div>
      ) : !displayBody.trim() ? (
        <p className="text-xs text-muted-foreground italic">Nothing to preview yet.</p>
      ) : loading && !hasContent ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          <span>Rendering preview…</span>
        </div>
      ) : page && pageSegs && pageSegs.length > 0 ? (
        <PreviewPageSegments segments={pageSegs} />
      ) : !page && postSegs && postSegs.length > 0 ? (
        <PreviewSegments segments={postSegs} />
      ) : null}
    </div>
  );
};

// PreviewPageSegments mirrors the Pages viewer's rendering but non-interactive:
// it shows how forms, sections and embeds will look, without initiating real
// downloads or in-app navigation.
const PreviewPageSegments = ({ segments }: { segments: BisonrelayPageSegment[] }) => (
  <div className="space-y-3">
    {segments.map((seg, i) => {
      if (seg.kind === 'text' && seg.html) {
        return (
          <div key={i} className={BR_PROSE_CLASSES} dangerouslySetInnerHTML={{ __html: seg.html }} />
        );
      }
      if (seg.kind === 'embed' && seg.data_b64) {
        if (seg.mime && seg.mime.startsWith('image/')) {
          return (
            <img
              key={i}
              src={`data:${seg.mime};base64,${seg.data_b64}`}
              alt={seg.alt || seg.name || ''}
              className="rounded-lg border border-border/40 max-w-full h-auto"
            />
          );
        }
        return (
          <a
            key={i}
            href={`data:${seg.mime || 'application/octet-stream'};base64,${seg.data_b64}`}
            download={seg.name || 'attachment'}
            className="inline-block text-xs text-primary underline hover:no-underline"
          >
            {seg.name || 'attachment'} ({seg.mime || 'binary'})
          </a>
        );
      }
      if (seg.kind === 'embed' && seg.download) {
        const label = seg.filename || seg.name || 'file';
        const price = seg.cost ? `${(seg.cost / 1e8).toFixed(8).replace(/\.?0+$/, '')} DCR` : 'free';
        return (
          <div
            key={i}
            className="rounded-lg border border-border/50 bg-background/40 px-3 py-2 text-xs text-muted-foreground"
          >
            Download: {label} ({price})
          </div>
        );
      }
      if (seg.kind === 'form' && seg.fields) {
        return <PreviewForm key={i} fields={seg.fields} />;
      }
      return null;
    })}
  </div>
);

// PreviewForm renders a page form's controls disabled, for visual preview only.
const PreviewForm = ({ fields }: { fields: BisonrelayPageFormField[] }) => {
  const submit = fields.find((f) => f.type === 'submit');
  return (
    <div className="space-y-3 rounded-lg border border-border/50 bg-background/40 p-4">
      {fields.map((f, i) =>
        f.type === 'txtinput' || f.type === 'intinput' ? (
          <label key={i} className="block">
            {f.label && <span className="text-xs text-muted-foreground">{f.label}</span>}
            <input
              type={f.type === 'intinput' ? 'number' : 'text'}
              placeholder={f.hint}
              defaultValue={f.value}
              disabled
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm opacity-70"
            />
          </label>
        ) : null,
      )}
      <button
        type="button"
        disabled
        className="px-4 py-1.5 rounded-md bg-primary/20 text-primary text-sm font-semibold opacity-70"
      >
        {submit?.label || 'Submit'}
      </button>
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
