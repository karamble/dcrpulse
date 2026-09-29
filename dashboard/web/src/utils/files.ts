// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// downloadBlob saves data as a file. The anchor is attached before the click,
// which older Firefox needs, and the URL is revoked after the click has
// started the download, since revoking in the same tick can cancel a large one.
export const downloadBlob = (data: BlobPart, filename: string, type = 'application/octet-stream') => {
  const url = URL.createObjectURL(data instanceof Blob ? data : new Blob([data], { type }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
};
