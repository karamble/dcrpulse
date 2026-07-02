import { useEffect, useMemo, useState } from 'react';
import { QrCode, RefreshCw, AlertCircle } from 'lucide-react';
import { QRCodeSVG } from 'qrcode.react';
import { AccountInfo, getAccounts, getNextAddress } from '../../services/api';
import { AddressGroups } from '../AddressGroups';
import { CopyButton } from '../explorer/CopyButton';
import { useWalletReady } from '../../hooks/useWalletReady';

const MAX_DCR = 21_000_000;

const buildBip21Uri = (address: string, amount: string): string => {
  const trimmed = amount.trim();
  if (!trimmed) return `decred:${address}`;
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n <= 0) return `decred:${address}`;
  return `decred:${address}?amount=${trimmed}`;
};

const validateAmount = (raw: string): string | null => {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  if (!/^\d*\.?\d{0,8}$/.test(trimmed)) return 'Use a positive number with up to 8 decimals';
  const n = Number(trimmed);
  if (!Number.isFinite(n) || n < 0) return 'Amount must be zero or positive';
  if (n > MAX_DCR) return `Amount must be at most ${MAX_DCR.toLocaleString()} DCR`;
  return null;
};

export const ReceiveTab = () => {
  const { isWatchOnly } = useWalletReady();
  const [accounts, setAccounts] = useState<AccountInfo[]>([]);
  const [accountsError, setAccountsError] = useState<string | null>(null);
  const [selectedAccount, setSelectedAccount] = useState<number | null>(null);
  const [address, setAddress] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [generateError, setGenerateError] = useState<string | null>(null);
  const [amount, setAmount] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const data = await getAccounts();
        if (cancelled) return;
        const visible = data
          .filter((a) => a.accountName !== 'imported' && a.accountName !== 'mixed')
          .sort((a, b) => a.accountNumber - b.accountNumber);
        setAccounts(visible);
        if (visible.length > 0) setSelectedAccount(visible[0].accountNumber);
      } catch (err) {
        if (!cancelled) {
          console.error('Failed to load accounts:', err);
          setAccountsError('Failed to load accounts');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const amountError = useMemo(() => validateAmount(amount), [amount]);

  const handleGenerate = async () => {
    if (selectedAccount === null || generating) return;
    setGenerating(true);
    setGenerateError(null);
    try {
      const resp = await getNextAddress(selectedAccount);
      setAddress(resp.address);
    } catch (err: any) {
      const msg =
        err?.response?.data?.toString?.() ||
        err?.message ||
        'Failed to generate address';
      setGenerateError(msg);
      setAddress(null);
    } finally {
      setGenerating(false);
    }
  };

  const handleAccountChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    setSelectedAccount(Number(e.target.value));
    setAddress(null);
    setGenerateError(null);
  };

  const uri = address ? buildBip21Uri(address, amount) : '';

  return (
    <div className="space-y-6">
      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        <div className="flex items-center gap-3 mb-4">
          <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
            <QrCode className="h-5 w-5 text-primary" />
          </div>
          <div>
            <h2 className="text-xl font-semibold">Receive</h2>
            <p className="text-sm text-muted-foreground">
              Each payment request should use a fresh address to protect your privacy.
            </p>
          </div>
        </div>

        {accountsError ? (
          <div className="flex items-center gap-2 text-destructive text-sm">
            <AlertCircle className="h-4 w-4" />
            {accountsError}
          </div>
        ) : accounts.length === 0 ? (
          <div className="text-muted-foreground text-sm">No accounts available.</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm text-muted-foreground mb-1">
                Receiving account
              </label>
              <select
                value={selectedAccount ?? ''}
                onChange={handleAccountChange}
                className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground focus:outline-none focus:border-primary"
              >
                {accounts.map((a) => (
                  <option key={a.accountNumber} value={a.accountNumber}>
                    {a.accountName} ({a.totalBalance.toFixed(4)} DCR)
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm text-muted-foreground mb-1">
                Requested amount (optional)
              </label>
              <input
                type="text"
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                placeholder="0.00000000"
                className={`w-full px-3 py-2 rounded-lg bg-background border text-foreground focus:outline-none ${
                  amountError ? 'border-destructive' : 'border-border focus:border-primary'
                }`}
              />
              {amountError && (
                <p className="mt-1 text-xs text-destructive">{amountError}</p>
              )}
            </div>
          </div>
        )}
      </div>

      <div className="p-6 rounded-xl bg-gradient-card backdrop-blur-sm border border-border/50">
        {!address ? (
          <div className="text-center py-6">
            <div className="mb-4">
              <p className="text-muted-foreground">
                Click below to generate a fresh receiving address.
              </p>
              {isWatchOnly && (
                <p className="mt-1 text-xs text-warning">
                  *verify the receiving address on your hardware wallet
                </p>
              )}
            </div>
            {generateError && (
              <div className="mb-4 flex items-center justify-center gap-2 text-destructive text-sm">
                <AlertCircle className="h-4 w-4" />
                {generateError}
              </div>
            )}
            <button
              onClick={handleGenerate}
              disabled={selectedAccount === null || generating}
              className="px-6 py-3 rounded-lg bg-gradient-primary text-white font-semibold transition-all inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <RefreshCw className={`h-5 w-5 ${generating ? 'animate-spin' : ''}`} />
              {generating ? 'Generating...' : 'Generate new address'}
            </button>
          </div>
        ) : (
          <div className="flex flex-col md:flex-row items-center gap-6">
            <div className="flex-shrink-0 p-3 bg-white rounded-lg" aria-hidden="true">
              <QRCodeSVG value={uri} size={192} level="H" />
            </div>
            <div className="flex-1 min-w-0 w-full">
              <p className="text-sm text-muted-foreground mb-2">Address</p>
              <div className="p-3 rounded-lg bg-background border border-border mb-3">
                <AddressGroups value={address} className="text-base md:text-lg" />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <CopyButton text={address} label="Copy address" />
                {amount && !amountError && (
                  <CopyButton text={uri} label="Copy payment URI" />
                )}
                <button
                  onClick={handleGenerate}
                  disabled={generating}
                  className="ml-auto inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-muted/20 hover:bg-muted/30 transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  <RefreshCw className={`h-4 w-4 ${generating ? 'animate-spin' : ''}`} />
                  {generating ? 'Generating...' : 'New address'}
                </button>
              </div>
              {generateError && (
                <p className="mt-3 text-xs text-destructive flex items-center gap-1">
                  <AlertCircle className="h-3 w-3" />
                  {generateError}
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
