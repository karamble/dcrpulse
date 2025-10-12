// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { Server, Lock } from 'lucide-react';
import { useState } from 'react';
import { connectRPC } from '../services/api';

interface RPCConnectionProps {
  onConnect?: () => void;
}

export const RPCConnection = ({ onConnect }: RPCConnectionProps) => {
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState('');
  
  const [formData, setFormData] = useState({
    host: 'localhost',
    port: '9109',
    username: '',
    password: '',
  });

  const handleConnect = async () => {
    setIsConnecting(true);
    setError('');
    
    try {
      const response = await connectRPC(formData);
      
      if (response.success) {
        setIsConnected(true);
        if (onConnect) {
          onConnect();
        }
      } else {
        setError(response.message);
      }
    } catch (err: any) {
      setError(err.response?.data?.message || err.message || 'Connection failed');
    } finally {
      setIsConnecting(false);
    }
  };

  const handleDisconnect = () => {
    setIsConnected(false);
    setFormData({ ...formData, username: '', password: '' });
  };

  return (
    <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50 animate-fade-in">
      <div className="flex items-center gap-2 mb-6">
        <Server className="h-5 w-5 text-primary" />
        <h3 className="text-lg font-semibold">RPC Connection</h3>
      </div>
      
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">RPC Host</label>
            <input 
              className="w-full px-3 py-2 rounded-lg bg-muted border border-border text-foreground focus:outline-none focus:ring-2 focus:ring-primary/50"
              type="text" 
              placeholder="localhost" 
              value={formData.host}
              onChange={(e) => setFormData({ ...formData, host: e.target.value })}
              disabled={isConnected}
            />
          </div>
          <div className="space-y-2">
            <label className="text-sm font-medium">RPC Port</label>
            <input 
              className="w-full px-3 py-2 rounded-lg bg-muted border border-border text-foreground focus:outline-none focus:ring-2 focus:ring-primary/50"
              type="text" 
              placeholder="9109" 
              value={formData.port}
              onChange={(e) => setFormData({ ...formData, port: e.target.value })}
              disabled={isConnected}
            />
          </div>
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium">RPC Username</label>
          <input 
            className="w-full px-3 py-2 rounded-lg bg-muted border border-border text-foreground focus:outline-none focus:ring-2 focus:ring-primary/50"
            type="text" 
            placeholder="Enter username"
            value={formData.username}
            onChange={(e) => setFormData({ ...formData, username: e.target.value })}
            disabled={isConnected}
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium">RPC Password</label>
          <div className="flex items-center gap-2">
            <Lock className="h-4 w-4 text-muted-foreground" />
            <input 
              className="flex-1 px-3 py-2 rounded-lg bg-muted border border-border text-foreground focus:outline-none focus:ring-2 focus:ring-primary/50"
              type="password" 
              placeholder="Enter password"
              value={formData.password}
              onChange={(e) => setFormData({ ...formData, password: e.target.value })}
              disabled={isConnected}
            />
          </div>
        </div>

        {error && (
          <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20">
            <p className="text-sm text-red-500">{error}</p>
          </div>
        )}

        <button 
          onClick={isConnected ? handleDisconnect : handleConnect}
          disabled={isConnecting}
          className={`w-full px-4 py-2 rounded-lg font-semibold transition-all ${
            isConnected 
              ? 'bg-muted text-foreground hover:bg-muted/80' 
              : 'bg-primary text-primary-foreground hover:bg-primary/90'
          } disabled:opacity-50 disabled:cursor-not-allowed`}
        >
          {isConnecting ? 'Connecting...' : isConnected ? 'Disconnect' : 'Connect to Node'}
        </button>

        {isConnected && (
          <div className="p-3 rounded-lg bg-primary/10 border border-primary/20">
            <p className="text-sm text-primary font-medium">✓ Connected to dcrd node</p>
          </div>
        )}
      </div>
    </div>
  );
};

