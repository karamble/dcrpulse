// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { Box, ArrowRightLeft } from 'lucide-react';
import { SearchBar } from '../components/explorer/SearchBar';
import { Pagination } from '../components/explorer/Pagination';
import { getRecentBlocksPaginated, BlockSummary } from '../services/explorerApi';
import { useNavigate } from 'react-router-dom';

export const ExplorerLanding = () => {
  const [recentBlocks, setRecentBlocks] = useState<BlockSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [totalBlocks, setTotalBlocks] = useState(0);
  const pageSize = 20;
  const navigate = useNavigate();

  useEffect(() => {
    fetchRecentBlocks(currentPage);
  }, [currentPage]);

  useEffect(() => {
    // Refresh every 30 seconds, but only if on first page
    if (currentPage === 1) {
      const interval = setInterval(() => fetchRecentBlocks(1), 30000);
      return () => clearInterval(interval);
    }
  }, [currentPage]);

  const fetchRecentBlocks = async (page: number) => {
    try {
      setLoading(true);
      const response = await getRecentBlocksPaginated(page, pageSize);
      setRecentBlocks(response.blocks);
      setTotalPages(response.totalPages);
      setTotalBlocks(response.totalBlocks);
      setLoading(false);
    } catch (err) {
      console.error('Failed to fetch recent blocks:', err);
      setLoading(false);
    }
  };

  const handlePageChange = (page: number) => {
    setCurrentPage(page);
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };

  const formatTimeAgo = (timestamp: string) => {
    const date = new Date(timestamp);
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
    
    if (seconds < 60) return `${seconds}s ago`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    return `${Math.floor(seconds / 86400)}d ago`;
  };

  const formatHash = (hash: string) => {
    return `${hash.substring(0, 8)}...${hash.substring(hash.length - 8)}`;
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-background via-background/95 to-background/90 p-6">
      <div className="max-w-7xl mx-auto space-y-8">
        {/* Search Bar */}
        <div className="flex justify-center">
          <SearchBar />
        </div>

        {/* Recent Blocks */}
        <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center gap-2">
              <Box className="h-5 w-5 text-primary" />
              <h2 className="text-xl font-semibold">Recent Blocks</h2>
            </div>
            <div className="text-sm text-muted-foreground">
              Total: {totalBlocks.toLocaleString()} blocks
            </div>
          </div>

          {loading ? (
            <div className="text-center py-12">
              <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <p className="mt-4 text-muted-foreground">Loading blocks...</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-border/50">
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Height</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Hash</th>
                    <th className="text-left py-3 px-4 text-sm font-medium text-muted-foreground">Time</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Txs</th>
                    <th className="text-right py-3 px-4 text-sm font-medium text-muted-foreground">Size</th>
                  </tr>
                </thead>
                <tbody>
                  {recentBlocks.map((block) => (
                    <tr
                      key={block.hash}
                      onClick={() => navigate(`/explorer/block/${block.height}`)}
                      className="border-b border-border/30 hover:bg-muted/5 cursor-pointer transition-colors"
                    >
                      <td className="py-3 px-4 font-mono text-primary font-semibold">
                        {block.height.toLocaleString()}
                      </td>
                      <td className="py-3 px-4 font-mono text-sm">
                        {formatHash(block.hash)}
                      </td>
                      <td className="py-3 px-4 text-sm text-muted-foreground">
                        {formatTimeAgo(block.timestamp)}
                      </td>
                      <td className="py-3 px-4 text-right">
                        <span className="inline-flex items-center gap-1">
                          <ArrowRightLeft className="h-3 w-3" />
                          {block.txCount}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-right text-sm text-muted-foreground">
                        {formatSize(block.size)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {/* Pagination */}
          {!loading && (
            <Pagination
              currentPage={currentPage}
              totalPages={totalPages}
              onPageChange={handlePageChange}
              loading={loading}
            />
          )}
        </div>
      </div>
    </div>
  );
};

