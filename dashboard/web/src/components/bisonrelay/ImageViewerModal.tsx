// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Download, X } from 'lucide-react';

export interface ViewerImage {
  src: string;
  name: string;
  mime: string;
}

// ImageViewerModal is the shared image lightbox: fit-to-screen by default,
// clicking the image toggles a natural-size view with pan scrolling.
export const ImageViewerModal = ({
  image,
  onClose,
}: {
  image: ViewerImage;
  onClose: () => void;
}) => {
  const [zoomed, setZoomed] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-label={image.name}
    >
      {zoomed ? (
        <div className="absolute inset-0 overflow-auto" onClick={onClose}>
          <div
            className="flex min-h-full min-w-full w-max items-center justify-center p-4"
            onClick={onClose}
          >
            <img
              src={image.src}
              alt={image.name}
              onClick={(e) => {
                e.stopPropagation();
                setZoomed(false);
              }}
              className="max-w-none cursor-zoom-out"
            />
          </div>
        </div>
      ) : (
        <img
          src={image.src}
          alt={image.name}
          onClick={(e) => {
            e.stopPropagation();
            setZoomed(true);
          }}
          className="max-h-[92vh] max-w-[92vw] object-contain rounded shadow-2xl cursor-zoom-in"
        />
      )}
      <div className="absolute top-3 right-3 z-10 flex items-center gap-2">
        <a
          href={image.src}
          download={image.name}
          onClick={(e) => e.stopPropagation()}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-background/80 hover:bg-background text-foreground text-xs font-medium"
          title="Download"
        >
          <Download className="h-4 w-4" />
          <span>Download</span>
        </a>
        <button
          type="button"
          onClick={onClose}
          className="p-1.5 rounded-md bg-background/80 hover:bg-background text-foreground"
          title="Close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
};
