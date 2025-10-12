// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { BookmarkCheck, Edit3, Download, Upload, X, Copy, Check, ChevronDown, ChevronUp } from 'lucide-react';
import { BookmarkModal } from '../explorer/BookmarkModal';
import { 
  getBookmarks, 
  exportBookmarks, 
  importBookmarks, 
  AddressBookmark 
} from '../../services/bookmarkService';

export const AddressBookmarksCard = () => {
  const navigate = useNavigate();
  const [bookmarks, setBookmarks] = useState<AddressBookmark[]>([]);
  const [showAll, setShowAll] = useState(false);
  const [isExportModalOpen, setIsExportModalOpen] = useState(false);
  const [isImportModalOpen, setIsImportModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [selectedBookmark, setSelectedBookmark] = useState<AddressBookmark | null>(null);
  const [copied, setCopied] = useState(false);
  const [importJson, setImportJson] = useState('');
  const [importError, setImportError] = useState('');
  const [importSuccess, setImportSuccess] = useState('');

  const loadBookmarks = () => {
    setBookmarks(getBookmarks());
  };

  useEffect(() => {
    loadBookmarks();
  }, []);

  const handleEdit = (bookmark: AddressBookmark) => {
    setSelectedBookmark(bookmark);
    setIsEditModalOpen(true);
  };

  const handleEditSave = () => {
    loadBookmarks();
  };

  const handleAddressClick = (address: string) => {
    navigate(`/explorer/address/${address}`);
  };

  const formatAddress = (address: string) => {
    if (address.length <= 16) return address;
    return `${address.substring(0, 8)}...${address.substring(address.length - 8)}`;
  };

  const handleExport = () => {
    setIsExportModalOpen(true);
    setCopied(false);
  };

  const handleCopyExport = () => {
    const json = exportBookmarks();
    navigator.clipboard.writeText(json);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleImport = () => {
    setImportJson('');
    setImportError('');
    setImportSuccess('');
    setIsImportModalOpen(true);
  };

  const handleImportSubmit = () => {
    setImportError('');
    setImportSuccess('');

    const result = importBookmarks(importJson);
    
    if (result.success) {
      const messages = [];
      if (result.count > 0) {
        messages.push(`${result.count} bookmark${result.count !== 1 ? 's' : ''} imported`);
      }
      if (result.skipped > 0) {
        messages.push(`${result.skipped} skipped (duplicates or invalid)`);
      }
      setImportSuccess(messages.join(', '));
      loadBookmarks();
      
      // Close modal after 2 seconds on success
      setTimeout(() => {
        setIsImportModalOpen(false);
        setImportJson('');
        setImportSuccess('');
      }, 2000);
    } else {
      setImportError(result.error || 'Failed to import bookmarks');
    }
  };

  const displayedBookmarks = showAll ? bookmarks : bookmarks.slice(0, 5);
  const hasMore = bookmarks.length > 5;

  return (
    <>
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-2">
            <BookmarkCheck className="h-5 w-5 text-warning" />
            <h2 className="text-xl font-semibold">Bookmarked Addresses</h2>
            <span className="text-sm text-muted-foreground">({bookmarks.length})</span>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleExport}
              disabled={bookmarks.length === 0}
              className="flex items-center gap-2 px-3 py-2 text-sm border border-border rounded-lg hover:bg-muted/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              title="Export bookmarks"
            >
              <Download className="h-4 w-4" />
              Export
            </button>
            <button
              onClick={handleImport}
              className="flex items-center gap-2 px-3 py-2 text-sm border border-border rounded-lg hover:bg-muted/10 transition-colors"
              title="Import bookmarks"
            >
              <Upload className="h-4 w-4" />
              Import
            </button>
          </div>
        </div>

        {bookmarks.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <BookmarkCheck className="h-12 w-12 mx-auto mb-3 opacity-50" />
            <p>No bookmarked addresses</p>
            <p className="text-sm mt-1">Bookmark addresses from the explorer to see them here</p>
          </div>
        ) : (
          <>
            <div className="space-y-2">
              {displayedBookmarks.map((bookmark) => (
                <div
                  key={bookmark.address}
                  className="flex items-center gap-3 p-3 rounded-lg bg-background/50 border border-border/30 hover:border-primary/30 transition-colors group"
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="font-medium text-foreground">{bookmark.name}</span>
                    </div>
                    <button
                      onClick={() => handleAddressClick(bookmark.address)}
                      className="font-mono text-sm text-muted-foreground hover:text-primary transition-colors"
                      title={bookmark.address}
                    >
                      {formatAddress(bookmark.address)}
                    </button>
                    {bookmark.notes && (
                      <p className="text-xs text-muted-foreground mt-1 line-clamp-1">
                        {bookmark.notes}
                      </p>
                    )}
                  </div>
                  <button
                    onClick={() => handleEdit(bookmark)}
                    className="p-2 opacity-0 group-hover:opacity-100 hover:bg-muted/10 rounded-lg transition-all"
                    title="Edit bookmark"
                  >
                    <Edit3 className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>

            {hasMore && (
              <button
                onClick={() => setShowAll(!showAll)}
                className="w-full mt-4 flex items-center justify-center gap-2 py-2 text-sm text-muted-foreground hover:text-foreground transition-colors"
              >
                {showAll ? (
                  <>
                    <ChevronUp className="h-4 w-4" />
                    Show less
                  </>
                ) : (
                  <>
                    <ChevronDown className="h-4 w-4" />
                    View all ({bookmarks.length - 5} more)
                  </>
                )}
              </button>
            )}
          </>
        )}
      </div>

      {/* Edit Modal */}
      {selectedBookmark && (
        <BookmarkModal
          address={selectedBookmark.address}
          isOpen={isEditModalOpen}
          onClose={() => {
            setIsEditModalOpen(false);
            setSelectedBookmark(null);
          }}
          existingBookmark={selectedBookmark}
          onSave={handleEditSave}
        />
      )}

      {/* Export Modal */}
      {isExportModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-background border border-border rounded-xl shadow-2xl max-w-2xl w-full">
            <div className="flex items-center justify-between p-6 border-b border-border">
              <h2 className="text-xl font-semibold">Export Bookmarks</h2>
              <button
                onClick={() => setIsExportModalOpen(false)}
                className="p-2 hover:bg-muted/10 rounded-lg transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="p-6">
              <p className="text-sm text-muted-foreground mb-4">
                Copy the JSON below to back up your bookmarks. You can import it later or on another device.
              </p>
              <textarea
                readOnly
                value={exportBookmarks()}
                className="w-full h-64 px-4 py-2 bg-muted/20 border border-border rounded-lg font-mono text-sm resize-none"
              />
            </div>

            <div className="flex justify-end gap-2 p-6 border-t border-border">
              <button
                onClick={() => setIsExportModalOpen(false)}
                className="px-6 py-2 border border-border rounded-lg hover:bg-muted/10 transition-colors"
              >
                Close
              </button>
              <button
                onClick={handleCopyExport}
                className="flex items-center gap-2 px-6 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors"
              >
                {copied ? (
                  <>
                    <Check className="h-4 w-4" />
                    Copied!
                  </>
                ) : (
                  <>
                    <Copy className="h-4 w-4" />
                    Copy to Clipboard
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Import Modal */}
      {isImportModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="bg-background border border-border rounded-xl shadow-2xl max-w-2xl w-full">
            <div className="flex items-center justify-between p-6 border-b border-border">
              <h2 className="text-xl font-semibold">Import Bookmarks</h2>
              <button
                onClick={() => setIsImportModalOpen(false)}
                className="p-2 hover:bg-muted/10 rounded-lg transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            <div className="p-6 space-y-4">
              <p className="text-sm text-muted-foreground">
                Paste your exported bookmarks JSON below. Duplicate addresses will be skipped.
              </p>
              <textarea
                value={importJson}
                onChange={(e) => setImportJson(e.target.value)}
                placeholder='{"version":1,"bookmarks":[...]}'
                className="w-full h-64 px-4 py-2 bg-background border border-border rounded-lg font-mono text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary"
              />

              {importError && (
                <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg">
                  <p className="text-sm text-destructive">{importError}</p>
                </div>
              )}

              {importSuccess && (
                <div className="p-3 bg-success/10 border border-success/20 rounded-lg">
                  <p className="text-sm text-success">{importSuccess}</p>
                </div>
              )}
            </div>

            <div className="flex justify-end gap-2 p-6 border-t border-border">
              <button
                onClick={() => setIsImportModalOpen(false)}
                className="px-6 py-2 border border-border rounded-lg hover:bg-muted/10 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleImportSubmit}
                disabled={!importJson.trim()}
                className="px-6 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Import
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
};

