// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState, useEffect } from 'react';
import { X, Trash2, AlertTriangle } from 'lucide-react';
import { CopyButton } from './CopyButton';
import { AddressBookmark, addBookmark, updateBookmark, deleteBookmark } from '../../services/bookmarkService';

interface BookmarkModalProps {
  address: string;
  isOpen: boolean;
  onClose: () => void;
  existingBookmark?: AddressBookmark | null;
  onSave: () => void;
}

export const BookmarkModal = ({ address, isOpen, onClose, existingBookmark, onSave }: BookmarkModalProps) => {
  const [name, setName] = useState('');
  const [notes, setNotes] = useState('');
  const [error, setError] = useState('');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const isEditing = !!existingBookmark;

  useEffect(() => {
    if (isOpen) {
      if (existingBookmark) {
        setName(existingBookmark.name);
        setNotes(existingBookmark.notes);
      } else {
        setName('');
        setNotes('');
      }
      setError('');
      setShowDeleteConfirm(false);
    }
  }, [isOpen, existingBookmark]);

  // Handle ESC key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen) {
        if (showDeleteConfirm) {
          setShowDeleteConfirm(false);
        } else {
          onClose();
        }
      }
    };
    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [isOpen, showDeleteConfirm, onClose]);

  const handleSave = () => {
    try {
      setError('');
      if (isEditing) {
        updateBookmark(address, { name, notes });
      } else {
        addBookmark({ address, name, notes });
      }
      onSave();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save bookmark');
    }
  };

  const handleDelete = () => {
    try {
      deleteBookmark(address);
      onSave();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete bookmark');
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-background border border-border rounded-xl shadow-2xl max-w-lg w-full max-h-[90vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-border">
          <h2 className="text-xl font-semibold">
            {isEditing ? 'Edit Bookmark' : 'Bookmark Address'}
          </h2>
          <button
            onClick={onClose}
            className="p-2 hover:bg-muted/10 rounded-lg transition-colors"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 space-y-4">
          {/* Address Display */}
          <div>
            <label className="block text-sm font-medium mb-2">Address</label>
            <div className="flex items-center gap-2 p-3 bg-muted/20 rounded-lg border border-border/50">
              <code className="text-sm font-mono flex-1 break-all">{address}</code>
              <CopyButton text={address} />
            </div>
          </div>

          {/* Name Input */}
          <div>
            <label htmlFor="bookmark-name" className="block text-sm font-medium mb-2">
              Name <span className="text-destructive">*</span>
            </label>
            <input
              id="bookmark-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={50}
              placeholder="e.g., My Mining Wallet"
              className="w-full px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
            />
            <div className="mt-1 text-xs text-muted-foreground text-right">
              {name.length}/50 characters
            </div>
          </div>

          {/* Notes Textarea */}
          <div>
            <label htmlFor="bookmark-notes" className="block text-sm font-medium mb-2">
              Notes
            </label>
            <textarea
              id="bookmark-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              maxLength={500}
              rows={4}
              placeholder="Add notes about this address..."
              className="w-full px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary resize-none"
            />
            <div className="mt-1 text-xs text-muted-foreground text-right">
              {notes.length}/500 characters
            </div>
          </div>

          {/* Error Message */}
          {error && (
            <div className="p-3 bg-destructive/10 border border-destructive/20 rounded-lg">
              <p className="text-sm text-destructive">{error}</p>
            </div>
          )}

          {/* Delete Confirmation */}
          {showDeleteConfirm && (
            <div className="p-4 bg-destructive/10 border border-destructive/20 rounded-lg">
              <div className="flex items-start gap-3">
                <AlertTriangle className="h-5 w-5 text-destructive flex-shrink-0 mt-0.5" />
                <div className="flex-1">
                  <p className="text-sm font-medium text-destructive mb-2">
                    Are you sure you want to delete this bookmark?
                  </p>
                  <p className="text-xs text-muted-foreground mb-3">
                    This action cannot be undone.
                  </p>
                  <div className="flex gap-2">
                    <button
                      onClick={handleDelete}
                      className="px-4 py-2 bg-destructive text-destructive-foreground rounded-lg hover:bg-destructive/90 transition-colors text-sm font-medium"
                    >
                      Delete
                    </button>
                    <button
                      onClick={() => setShowDeleteConfirm(false)}
                      className="px-4 py-2 bg-muted/20 rounded-lg hover:bg-muted/30 transition-colors text-sm"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between p-6 border-t border-border">
          <div>
            {isEditing && !showDeleteConfirm && (
              <button
                onClick={() => setShowDeleteConfirm(true)}
                className="flex items-center gap-2 px-4 py-2 text-destructive hover:bg-destructive/10 rounded-lg transition-colors"
              >
                <Trash2 className="h-4 w-4" />
                Delete
              </button>
            )}
          </div>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-6 py-2 border border-border rounded-lg hover:bg-muted/10 transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              disabled={!name.trim()}
              className="px-6 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isEditing ? 'Update' : 'Save'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

