// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Canvas-based image recompression for inline BR embeds. Re-encoding to
// JPEG drops every metadata block (EXIF, GPS, XMP) and the downscale ladder
// brings phone photos under BR's small inline caps. bruig compresses at a
// fixed JPEG quality 40 with no dimension cap, but its payload budget is
// 10 MiB; ours is 512 KiB (posts) / 800 KiB (chat), so quality alone is not
// enough and each rung trades resolution for bytes.
const COMPRESS_LADDER: ReadonlyArray<{ maxEdge: number; quality: number }> = [
  { maxEdge: 1920, quality: 0.4 },
  { maxEdge: 1280, quality: 0.4 },
  { maxEdge: 1024, quality: 0.35 },
];

const JPEG_MIME = 'image/jpeg';

// blobToDataB64 turns a Blob/File into base64. Chunked because a single
// String.fromCharCode.apply over megabytes of bytes overflows the
// call-stack argument limit.
export async function blobToDataB64(blob: Blob): Promise<string> {
  const buf = await blob.arrayBuffer();
  const bytes = new Uint8Array(buf);
  let binStr = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binStr += String.fromCharCode.apply(
      null,
      bytes.subarray(i, i + chunk) as unknown as number[],
    );
  }
  return btoa(binStr);
}

type Drawable = ImageBitmap | HTMLImageElement;

// decodeImage returns a drawable with EXIF orientation already baked into
// the pixels, so the re-encoded JPEG is upright even though its orientation
// tag is gone. Falls back to an <img> element where createImageBitmap is
// unavailable or rejects the input.
const decodeImage = async (
  file: File,
): Promise<{ src: Drawable; w: number; h: number }> => {
  if (typeof createImageBitmap === 'function') {
    try {
      const bmp = await createImageBitmap(file, { imageOrientation: 'from-image' });
      return { src: bmp, w: bmp.width, h: bmp.height };
    } catch {
      /* fall through to <img> decode */
    }
  }
  const url = URL.createObjectURL(file);
  try {
    const img = await new Promise<HTMLImageElement>((resolve, reject) => {
      const el = new Image();
      el.onload = () => resolve(el);
      el.onerror = () => reject(new Error('image decode failed'));
      el.src = url;
    });
    return { src: img, w: img.naturalWidth, h: img.naturalHeight };
  } finally {
    URL.revokeObjectURL(url);
  }
};

export interface CompressResult {
  blob: Blob;
  dataB64: string;
  size: number;
  width: number;
  height: number;
  // Object URL for previewing the compressed result; caller must revoke.
  previewUrl: string;
  fitsCap: boolean;
}

// compressImageToJpeg walks the ladder until the JPEG output fits maxBytes,
// returning the first fitting rung; if none fit it returns the smallest
// result produced with fitsCap=false so the caller can show the size and
// keep the attach action disabled. Throws only when decoding or every
// encode attempt fails outright.
export async function compressImageToJpeg(
  file: File,
  maxBytes: number,
  ladder: ReadonlyArray<{ maxEdge: number; quality: number }> = COMPRESS_LADDER,
): Promise<CompressResult> {
  const { src, w, h } = await decodeImage(file);
  let smallest: { blob: Blob; w: number; h: number } | null = null;

  for (const rung of ladder) {
    const scale = Math.min(1, rung.maxEdge / Math.max(w, h));
    const dw = Math.max(1, Math.round(w * scale));
    const dh = Math.max(1, Math.round(h * scale));

    const canvas = document.createElement('canvas');
    canvas.width = dw;
    canvas.height = dh;
    const ctx = canvas.getContext('2d');
    if (!ctx) break;
    // JPEG has no alpha channel; matte transparent pixels white so PNG/GIF
    // transparency does not flatten to black.
    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, dw, dh);
    ctx.drawImage(src as CanvasImageSource, 0, 0, dw, dh);

    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, JPEG_MIME, rung.quality),
    );
    if (!blob) continue;
    if (!smallest || blob.size < smallest.blob.size) {
      smallest = { blob, w: dw, h: dh };
    }
    if (blob.size <= maxBytes) break;
  }

  if (typeof (src as ImageBitmap).close === 'function') {
    (src as ImageBitmap).close();
  }
  if (!smallest) throw new Error('compression produced no output');

  const dataB64 = await blobToDataB64(smallest.blob);
  return {
    blob: smallest.blob,
    dataB64,
    size: smallest.blob.size,
    width: smallest.w,
    height: smallest.h,
    previewUrl: URL.createObjectURL(smallest.blob),
    fitsCap: smallest.blob.size <= maxBytes,
  };
}
