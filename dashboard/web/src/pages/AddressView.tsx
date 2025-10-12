// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { ArrowLeft, AlertCircle, Ticket, CheckCircle, XCircle, Info } from 'lucide-react';
import { CopyButton } from '../components/explorer/CopyButton';
import { AddressBookmarkButton } from '../components/explorer/AddressBookmarkButton';
import { getAddressInfo, AddressInfo } from '../services/explorerApi';

export const AddressView = () => {
  const { address } = useParams<{ address: string }>();
  const navigate = useNavigate();
  const [info, setInfo] = useState<AddressInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (address) {
      fetchAddressInfo();
    }
  }, [address]);

  const fetchAddressInfo = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await getAddressInfo(address!);
      setInfo(data);
    } catch (err) {
      console.error('Failed to fetch address info:', err);
      setError('Failed to load address information');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-6">
        <button
          onClick={() => navigate('/explorer')}
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Explorer
        </button>
        <div className="flex justify-center items-center py-12">
          <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      </div>
    );
  }

  if (error || !info) {
    return (
      <div className="space-y-6">
        <button
          onClick={() => navigate('/explorer')}
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Explorer
        </button>
        <div className="p-8 rounded-lg bg-destructive/10 border border-destructive/20">
          <div className="flex items-center gap-3">
            <AlertCircle className="h-6 w-6 text-destructive" />
            <p className="text-destructive">{error || 'Address not found'}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Navigation */}
      <button
        onClick={() => navigate('/explorer')}
        className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Explorer
      </button>

      {/* Address Header */}
      <div className="p-6 rounded-lg bg-gradient-to-br from-background to-muted/20 border border-border">
        <div className="flex items-start justify-between mb-4">
          <div className="flex-1">
            <h1 className="text-3xl font-bold mb-2">Address</h1>
            <div className="flex items-center gap-2 flex-wrap">
              <code className="text-sm sm:text-base font-mono bg-muted/50 px-3 py-1.5 rounded break-all">
                {address}
              </code>
              <CopyButton text={address || ''} />
              <AddressBookmarkButton address={address || ''} />
            </div>
          </div>
        </div>

        {/* Address Status */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-6">
          <div className="flex items-center gap-3 p-3 rounded-lg bg-background/50">
            {info.isValid ? (
              <CheckCircle className="h-5 w-5 text-success flex-shrink-0" />
            ) : (
              <XCircle className="h-5 w-5 text-destructive flex-shrink-0" />
            )}
            <div>
              <p className="text-sm text-muted-foreground">Validity</p>
              <p className="font-medium">{info.isValid ? 'Valid Address' : 'Invalid Address'}</p>
            </div>
          </div>

          <div className="flex items-center gap-3 p-3 rounded-lg bg-background/50">
            {info.exists ? (
              <CheckCircle className="h-5 w-5 text-success flex-shrink-0" />
            ) : (
              <Info className="h-5 w-5 text-muted-foreground flex-shrink-0" />
            )}
            <div>
              <p className="text-sm text-muted-foreground">On-Chain Status</p>
              <p className="font-medium">{info.exists ? 'Used on blockchain' : 'Never used'}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Tickets Section */}
      {info.tickets && info.tickets.length > 0 && (
        <div className="p-6 rounded-lg bg-gradient-card backdrop-blur-sm border border-border/50">
          <div className="flex items-center gap-2 mb-6">
            <Ticket className="h-5 w-5 text-warning" />
            <h2 className="text-xl font-semibold">Tickets Owned</h2>
            <span className="ml-auto text-sm text-muted-foreground">{info.tickets.length} total</span>
          </div>

          <div className="space-y-2 max-h-96 overflow-y-auto">
            {info.tickets.map((ticketHash, index) => (
              <Link
                key={index}
                to={`/explorer/tx/${ticketHash}`}
                className="flex items-center gap-3 p-3 rounded-lg bg-background/50 hover:bg-background transition-colors border border-border/30 hover:border-primary/30"
              >
                <Ticket className="h-4 w-4 text-warning flex-shrink-0" />
                <code className="text-sm font-mono flex-1 break-all">{ticketHash}</code>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* No Tickets */}
      {info.tickets && info.tickets.length === 0 && info.exists && (
        <div className="p-6 rounded-lg bg-muted/20 border border-border/50">
          <div className="flex items-center gap-3">
            <Ticket className="h-5 w-5 text-muted-foreground" />
            <p className="text-muted-foreground">No tickets found for this address</p>
          </div>
        </div>
      )}

      {/* Disclaimer at bottom */}
      <div className="p-6 rounded-lg bg-gradient-to-br from-yellow-500/10 to-orange-500/5 border border-yellow-500/20">
        <div className="flex items-start gap-4">
          <AlertCircle className="h-6 w-6 text-warning flex-shrink-0 mt-1" />
          <div className="flex-1">
            <h3 className="font-semibold mb-2 text-warning">Limited Address Information</h3>
            <p className="text-sm text-muted-foreground mb-4">
              This dashboard provides basic address information but does not store a local database of all transactions with an addressindex. 
              For full transaction history and balance information of any address, please visit the official Decred block explorer.
            </p>
            <p className="text-sm text-muted-foreground">
              <strong className="text-foreground">Current capabilities:</strong> Address validation, existence check, and ticket ownership lookup.
            </p>
            <p className="text-sm text-muted-foreground mt-3">
              <strong className="text-foreground">Full address details:</strong>{' '}
              <a 
                href={`https://dcrdata.decred.org/address/${address}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline"
              >
                dcrdata.decred.org/address/{address}
              </a>
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};
