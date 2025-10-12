// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

const STORAGE_KEY = 'decred-pulse-bookmarks';
const STORAGE_VERSION = 1;

export interface AddressBookmark {
  address: string;
  name: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
}

interface BookmarkStorage {
  version: number;
  bookmarks: AddressBookmark[];
}

// Initialize storage if it doesn't exist
const initStorage = (): void => {
  const existing = localStorage.getItem(STORAGE_KEY);
  if (!existing) {
    const initial: BookmarkStorage = {
      version: STORAGE_VERSION,
      bookmarks: [],
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(initial));
  }
};

// Get storage object
const getStorage = (): BookmarkStorage => {
  try {
    initStorage();
    const data = localStorage.getItem(STORAGE_KEY);
    if (!data) {
      return { version: STORAGE_VERSION, bookmarks: [] };
    }
    const parsed = JSON.parse(data) as BookmarkStorage;
    return parsed;
  } catch (error) {
    console.error('Failed to parse bookmark storage:', error);
    return { version: STORAGE_VERSION, bookmarks: [] };
  }
};

// Save storage object
const saveStorage = (storage: BookmarkStorage): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(storage));
  } catch (error) {
    console.error('Failed to save bookmarks:', error);
    throw new Error('Failed to save bookmarks. Storage quota may be exceeded.');
  }
};

// Validate address format (basic Decred address validation)
const isValidAddress = (address: string): boolean => {
  // Decred addresses start with D (mainnet) or T (testnet) and are 26-35 chars
  return /^[DT][a-km-zA-HJ-NP-Z1-9]{25,34}$/.test(address);
};

// Get all bookmarks
export const getBookmarks = (): AddressBookmark[] => {
  const storage = getStorage();
  return storage.bookmarks.sort((a, b) => 
    new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
  );
};

// Get single bookmark by address
export const getBookmark = (address: string): AddressBookmark | null => {
  const storage = getStorage();
  const bookmark = storage.bookmarks.find(b => b.address === address);
  return bookmark || null;
};

// Check if address is bookmarked
export const isBookmarked = (address: string): boolean => {
  return getBookmark(address) !== null;
};

// Add new bookmark
export const addBookmark = (data: Omit<AddressBookmark, 'createdAt' | 'updatedAt'>): void => {
  const { address, name, notes } = data;

  // Validation
  if (!address || !isValidAddress(address)) {
    throw new Error('Invalid Decred address format');
  }
  if (!name || name.trim().length === 0) {
    throw new Error('Name is required');
  }
  if (name.length > 50) {
    throw new Error('Name must be 50 characters or less');
  }
  if (notes.length > 500) {
    throw new Error('Notes must be 500 characters or less');
  }

  const storage = getStorage();

  // Check for duplicates
  if (storage.bookmarks.some(b => b.address === address)) {
    throw new Error('This address is already bookmarked');
  }

  const now = new Date().toISOString();
  const bookmark: AddressBookmark = {
    address,
    name: name.trim(),
    notes: notes.trim(),
    createdAt: now,
    updatedAt: now,
  };

  storage.bookmarks.push(bookmark);
  saveStorage(storage);
};

// Update existing bookmark
export const updateBookmark = (
  address: string,
  updates: Partial<Omit<AddressBookmark, 'address' | 'createdAt' | 'updatedAt'>>
): void => {
  const storage = getStorage();
  const index = storage.bookmarks.findIndex(b => b.address === address);

  if (index === -1) {
    throw new Error('Bookmark not found');
  }

  // Validation
  if (updates.name !== undefined) {
    if (updates.name.trim().length === 0) {
      throw new Error('Name cannot be empty');
    }
    if (updates.name.length > 50) {
      throw new Error('Name must be 50 characters or less');
    }
  }
  if (updates.notes !== undefined && updates.notes.length > 500) {
    throw new Error('Notes must be 500 characters or less');
  }

  const bookmark = storage.bookmarks[index];
  storage.bookmarks[index] = {
    ...bookmark,
    name: updates.name !== undefined ? updates.name.trim() : bookmark.name,
    notes: updates.notes !== undefined ? updates.notes.trim() : bookmark.notes,
    updatedAt: new Date().toISOString(),
  };

  saveStorage(storage);
};

// Delete bookmark
export const deleteBookmark = (address: string): void => {
  const storage = getStorage();
  const index = storage.bookmarks.findIndex(b => b.address === address);

  if (index === -1) {
    throw new Error('Bookmark not found');
  }

  storage.bookmarks.splice(index, 1);
  saveStorage(storage);
};

// Export bookmarks as JSON string
export const exportBookmarks = (): string => {
  const storage = getStorage();
  return JSON.stringify(storage, null, 2);
};

// Import bookmarks from JSON string (skip duplicates)
export const importBookmarks = (json: string): { success: boolean; count: number; skipped: number; error?: string } => {
  try {
    const imported = JSON.parse(json) as BookmarkStorage;

    // Validate structure
    if (!imported.bookmarks || !Array.isArray(imported.bookmarks)) {
      return { success: false, count: 0, skipped: 0, error: 'Invalid JSON format' };
    }

    const storage = getStorage();
    const existingAddresses = new Set(storage.bookmarks.map(b => b.address));
    
    let importedCount = 0;
    let skippedCount = 0;

    for (const bookmark of imported.bookmarks) {
      // Validate bookmark structure
      if (!bookmark.address || !bookmark.name) {
        skippedCount++;
        continue;
      }

      // Skip if already exists
      if (existingAddresses.has(bookmark.address)) {
        skippedCount++;
        continue;
      }

      // Validate address
      if (!isValidAddress(bookmark.address)) {
        skippedCount++;
        continue;
      }

      // Add to storage
      storage.bookmarks.push({
        address: bookmark.address,
        name: bookmark.name,
        notes: bookmark.notes || '',
        createdAt: bookmark.createdAt || new Date().toISOString(),
        updatedAt: bookmark.updatedAt || new Date().toISOString(),
      });

      importedCount++;
      existingAddresses.add(bookmark.address);
    }

    saveStorage(storage);
    return { success: true, count: importedCount, skipped: skippedCount };
  } catch (error) {
    return { 
      success: false, 
      count: 0, 
      skipped: 0, 
      error: error instanceof Error ? error.message : 'Invalid JSON format' 
    };
  }
};

