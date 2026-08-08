// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// apiError returns the error text of a failed request: the response body when
// the daemon sent one, else the body's message field, else the error's own
// message, else the fallback. Interpolating the raw body renders
// "[object Object]" when it is JSON or a Blob, which is what this exists to
// prevent.
export const apiError = (err: unknown, fallback: string): string => {
  const e = err as { response?: { data?: unknown }; message?: string } | null | undefined;
  const body = e?.response?.data;
  if (typeof body === 'string' && body.trim()) return body.trim();
  const bodyMsg = (body as { message?: unknown } | null | undefined)?.message;
  if (typeof bodyMsg === 'string' && bodyMsg) return bodyMsg;
  if (e?.message) return e.message;
  return fallback;
};
