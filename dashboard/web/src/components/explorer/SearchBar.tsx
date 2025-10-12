// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Search, Loader2 } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { searchExplorer } from '../../services/explorerApi';

export const SearchBar = () => {
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const detectSearchType = (q: string): string => {
    const trimmed = q.trim();

    // Block height (1-7 digits)
    if (/^\d{1,7}$/.test(trimmed)) {
      return 'block_height';
    }

    // Transaction hash or block hash (64 hex chars)
    if (/^[0-9a-fA-F]{64}$/.test(trimmed)) {
      return 'tx_hash';
    }

    // Address (starts with D, 26-35 chars)
    if (/^D[0-9A-Za-z]{24,33}$/.test(trimmed)) {
      return 'address';
    }

    return 'unknown';
  };

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!query.trim()) {
      setError('Please enter a search query');
      return;
    }

    setLoading(true);
    setError('');

    try {
      const result = await searchExplorer(query.trim());

      if (!result.found) {
        setError(result.error || 'Not found');
        setLoading(false);
        return;
      }

      // Navigate based on result type
      switch (result.type) {
        case 'block':
          const blockData = result.data as any;
          navigate(`/explorer/block/${blockData.height}`);
          break;
        case 'transaction':
          const txData = result.data as any;
          navigate(`/explorer/tx/${txData.txid}`);
          break;
        case 'address':
          navigate(`/explorer/address/${query.trim()}`);
          break;
        default:
          setError('Unknown result type');
      }

      setLoading(false);
    } catch (err) {
      setError('Search failed. Please try again.');
      setLoading(false);
    }
  };

  const getPlaceholder = () => {
    const type = detectSearchType(query);
    switch (type) {
      case 'block_height':
        return 'Block height detected...';
      case 'tx_hash':
        return 'Transaction hash detected...';
      case 'address':
        return 'Address detected...';
      default:
        return 'Search block height, tx hash, block hash, or address...';
    }
  };

  return (
    <form onSubmit={handleSearch} className="w-full max-w-3xl">
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setError('');
          }}
          placeholder={getPlaceholder()}
          className="w-full px-4 py-3 pl-12 pr-4 text-lg rounded-lg bg-background border border-border focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 transition-all"
          disabled={loading}
        />
        <div className="absolute left-4 top-1/2 transform -translate-y-1/2">
          {loading ? (
            <Loader2 className="h-5 w-5 text-muted-foreground animate-spin" />
          ) : (
            <Search className="h-5 w-5 text-muted-foreground" />
          )}
        </div>
        <button
          type="submit"
          disabled={loading || !query.trim()}
          className="absolute right-2 top-1/2 transform -translate-y-1/2 px-4 py-1.5 bg-primary text-primary-foreground rounded-md hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          Search
        </button>
      </div>
      
      {error && (
        <div className="mt-2 text-sm text-red-500">
          {error}
        </div>
      )}
      
      <div className="mt-2 text-xs text-muted-foreground">
        Search by: Block height (e.g., 1000000) • Transaction hash • Block hash • Address
      </div>
    </form>
  );
};

