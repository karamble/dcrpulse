// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { AlertCircle, Image as ImageIcon, Loader2, X } from 'lucide-react';
import { formatBytes, isImageMime } from '../embedParser';
import { CompressResult, blobToDataB64, compressImageToJpeg } from './imageCompress';

export interface ImageAttachResult {
  // The original File for the as-is choice, or the re-encoded JPEG blob.
  blob: Blob;
  dataB64: string;
  mime: string;
  name: string;
  displayName: string;
  alt?: string;
  size: number;
}

type Choice = 'original' | 'compressed';

// ImageAttachModal mirrors bruig's "Compress Image" flow: preview the picked
// image, show original vs compressed sizes and let the user opt into the
// JPEG re-encode (which shrinks the payload and strips all EXIF metadata).
// It is embed/staging agnostic: callers receive the chosen bytes via
// onAttach and map them into their own structures.
export const ImageAttachModal = ({
  file,
  maxInlineBytes,
  allowOversized,
  oversizedHint,
  showAlt = true,
  onCancel,
  onAttach,
}: {
  file: File;
  maxInlineBytes: number;
  // When set, choices over maxInlineBytes stay attachable (chat falls back
  // to file transfer); the hint explains what happens.
  allowOversized?: boolean;
  oversizedHint?: string;
  showAlt?: boolean;
  onCancel: () => void;
  onAttach: (r: ImageAttachResult) => void;
}) => {
  const originalTooLarge = file.size > maxInlineBytes;
  const [computing, setComputing] = useState(true);
  const [compressed, setCompressed] = useState<CompressResult | null>(null);
  const [compressErr, setCompressErr] = useState<string | null>(null);
  const [attaching, setAttaching] = useState(false);
  const [alt, setAlt] = useState('');
  const [origUrl] = useState(() => URL.createObjectURL(file));
  const [choice, setChoice] = useState<Choice>('compressed');

  useEffect(() => {
    let cancelled = false;
    compressImageToJpeg(file, maxInlineBytes)
      .then((r) => {
        if (cancelled) {
          URL.revokeObjectURL(r.previewUrl);
          return;
        }
        setCompressed(r);
      })
      .catch((e: any) => {
        if (!cancelled) {
          setCompressErr(e?.message || 'unknown error');
          setChoice('original');
        }
      })
      .finally(() => {
        if (!cancelled) setComputing(false);
      });
    return () => {
      cancelled = true;
    };
  }, []); // mount-only: one compression pass per picked file

  useEffect(
    () => () => {
      URL.revokeObjectURL(origUrl);
    },
    [origUrl],
  );
  useEffect(
    () => () => {
      if (compressed) URL.revokeObjectURL(compressed.previewUrl);
    },
    [compressed],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  const isGif = file.type === 'image/gif';
  const compressedSavings = !!compressed && compressed.size < file.size;
  const previewUrl = choice === 'compressed' && compressed ? compressed.previewUrl : origUrl;
  const canAttach =
    !computing &&
    !attaching &&
    (choice === 'original'
      ? !originalTooLarge || !!allowOversized
      : !!compressed && (compressed.fitsCap || !!allowOversized));

  const submit = async () => {
    if (!canAttach) return;
    setAttaching(true);
    try {
      const trimmedAlt = alt.trim() || undefined;
      if (choice === 'compressed' && compressed) {
        onAttach({
          blob: compressed.blob,
          dataB64: compressed.dataB64,
          mime: 'image/jpeg',
          name: file.name,
          displayName: file.name,
          alt: trimmedAlt,
          size: compressed.size,
        });
      } else {
        onAttach({
          blob: file,
          dataB64: await blobToDataB64(file),
          mime: file.type || 'application/octet-stream',
          name: file.name,
          displayName: file.name,
          alt: trimmedAlt,
          size: file.size,
        });
      }
    } catch (e: any) {
      setCompressErr(e?.message || 'Could not read file');
      setAttaching(false);
    }
  };

  const choiceRow = (
    id: Choice,
    label: string,
    sizeText: string,
    over: boolean,
    disabled: boolean,
    caption?: string,
  ) => (
    <label
      className={`flex items-start gap-2.5 rounded-lg border px-3 py-2 transition-colors ${
        choice === id ? 'border-primary/50 bg-primary/10' : 'border-border/50 bg-background/40'
      } ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer hover:border-primary/30'}`}
    >
      <input
        type="radio"
        name="br-image-variant"
        checked={choice === id}
        disabled={disabled}
        onChange={() => setChoice(id)}
        className="mt-0.5 accent-[hsl(var(--primary))]"
      />
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5 text-sm text-foreground">
          {label}{' '}
          <span className={`tabular-nums ${over ? 'text-destructive' : 'text-muted-foreground'}`}>
            {sizeText}
          </span>
        </span>
        {caption && (
          <span className="block text-[10px] text-muted-foreground mt-0.5">{caption}</span>
        )}
        {over && !allowOversized && (
          <span className="block text-[10px] text-destructive mt-0.5">
            Still over the {formatBytes(maxInlineBytes)} attachment limit.
          </span>
        )}
        {over && allowOversized && oversizedHint && (
          <span className="block text-[10px] text-muted-foreground mt-0.5">{oversizedHint}</span>
        )}
      </span>
    </label>
  );

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4"
      onClick={onCancel}
    >
      <div
        className="w-full max-w-md max-h-[85vh] overflow-y-auto rounded-xl bg-card border border-border/50 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between p-5 pb-3">
          <h3 className="flex items-center gap-2 text-base font-semibold text-foreground">
            <ImageIcon className="h-4 w-4 text-primary" />
            Attach image
          </h3>
          <button
            type="button"
            onClick={onCancel}
            aria-label="Close"
            className="text-muted-foreground hover:text-foreground transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="px-5">
          <div className="relative rounded-lg border border-border/40 bg-muted/10 overflow-hidden flex items-center justify-center">
            <img
              src={previewUrl}
              alt=""
              className="max-h-64 w-auto max-w-full object-contain"
            />
            {computing && choice === 'compressed' && (
              <div className="absolute inset-0 flex items-center justify-center bg-background/60">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            )}
          </div>
        </div>

        <div className="px-5 pt-4 space-y-2">
          {choiceRow(
            'original',
            'Original File:',
            formatBytes(file.size),
            originalTooLarge,
            originalTooLarge && !allowOversized,
            originalTooLarge && !allowOversized ? 'Too large to attach uncompressed.' : undefined,
          )}
          {choiceRow(
            'compressed',
            'Compressed File:',
            computing
              ? 'computing...'
              : compressed
                ? formatBytes(compressed.size)
                : 'unavailable',
            !!compressed && !compressed.fitsCap,
            computing || !compressed,
            compressed
              ? [
                  `${compressed.width}x${compressed.height}, JPEG`,
                  isGif ? 'Animation is removed (first frame only).' : '',
                  !compressedSavings ? 'No size savings' : '',
                ]
                  .filter(Boolean)
                  .join(' - ')
              : undefined,
          )}
        </div>

        {showAlt && (
          <div className="px-5 pt-3">
            <input
              type="text"
              value={alt}
              onChange={(e) => setAlt(e.target.value)}
              placeholder="Alt text (optional)"
              className="w-full px-3 py-2 rounded-lg bg-background border border-border/50 text-sm text-foreground focus:outline-none focus:border-primary/50"
            />
            <p className="text-[10px] text-muted-foreground mt-1">
              Describe the image for screen readers and link previews.
            </p>
          </div>
        )}

        {compressErr && (
          <div className="px-5 pt-3 flex items-start gap-2 text-xs text-destructive">
            <AlertCircle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span className="break-words">Could not compress this image. {compressErr}</span>
          </div>
        )}

        <div className="flex justify-end gap-2 p-5 pt-4">
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 text-sm text-foreground transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={!canAttach}
            className="px-4 py-2 rounded-lg bg-gradient-primary text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
          >
            {attaching && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            Attach
          </button>
        </div>
      </div>
    </div>
  );
};

// svg has no reliable intrinsic raster size and rasterizing a vector to
// lossy JPEG is the wrong trade; callers route it through their plain
// attach path instead of this modal.
export const isCompressibleImage = (mime: string): boolean =>
  isImageMime(mime) && mime !== 'image/svg+xml';
