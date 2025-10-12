// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { Star } from 'lucide-react';
import { BookmarkModal } from './BookmarkModal';
import { getBookmark, isBookmarked } from '../../services/bookmarkService';

interface AddressBookmarkButtonProps {
  address: string;
}

export const AddressBookmarkButton = ({ address }: AddressBookmarkButtonProps) => {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [bookmarked, setBookmarked] = useState(false);

  useEffect(() => {
    setBookmarked(isBookmarked(address));
  }, [address]);

  const handleSave = () => {
    setBookmarked(isBookmarked(address));
  };

  const existingBookmark = bookmarked ? getBookmark(address) : null;

  return (
    <>
      <button
        onClick={() => setIsModalOpen(true)}
        className="p-2 hover:bg-muted/10 rounded-lg transition-colors group"
        title={bookmarked ? 'Edit bookmark' : 'Bookmark address'}
        aria-label={bookmarked ? 'Edit bookmark' : 'Bookmark address'}
      >
        <Star 
          className={`h-5 w-5 transition-colors ${
            bookmarked 
              ? 'fill-warning text-warning' 
              : 'text-muted-foreground group-hover:text-warning'
          }`}
        />
      </button>

      <BookmarkModal
        address={address}
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        existingBookmark={existingBookmark}
        onSave={handleSave}
      />
    </>
  );
};

