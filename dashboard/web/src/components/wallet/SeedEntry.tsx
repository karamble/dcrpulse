import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, Check, Hash, ListOrdered } from 'lucide-react';
import api, { decodeSeed } from '../../services/api';

// Wordlist is fetched once from /api/wallet/seed-words, which sources from
// dcrwallet's pgpwordlist package (upstream source of truth).
let cachedWordlist: string[] | null = null;
let cachedWordlistLower: Set<string> | null = null;
const wordlistReady: Promise<void> = (async () => {
  try {
    const resp = await api.get<string[]>('/wallet/seed-words');
    cachedWordlist = resp.data;
    cachedWordlistLower = new Set(resp.data.map((w) => w.toLowerCase()));
  } catch {
    cachedWordlist = [];
    cachedWordlistLower = new Set();
  }
})();

const SEED_WORDS = 33;

const HEX_LENGTH = 64; // 32 bytes — Decred standard

const VALIDATE_DEBOUNCE_MS = 300;

interface Props {
  onValidSeedHex: (hex: string) => void;
  onInvalid: () => void;
}

type Mode = 'words' | 'hex';

const suggestionsFor = (prefix: string, limit = 8): string[] => {
  const p = prefix.trim().toLowerCase();
  if (!p || !cachedWordlist) return [];
  const matches: string[] = [];
  for (const w of cachedWordlist) {
    if (w.toLowerCase().startsWith(p)) {
      matches.push(w);
      if (matches.length >= limit) break;
    }
  }
  return matches;
};

const isKnownWord = (w: string): boolean =>
  cachedWordlistLower ? cachedWordlistLower.has(w.trim().toLowerCase()) : false;

export const SeedEntry = ({ onValidSeedHex, onInvalid }: Props) => {
  const [mode, setMode] = useState<Mode>('words');
  const [words, setWords] = useState<string[]>(() => new Array(SEED_WORDS).fill(''));
  const [hex, setHex] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [activeSuggestionFor, setActiveSuggestionFor] = useState<number | null>(null);
  const [highlightedSuggestion, setHighlightedSuggestion] = useState(0);
  const [wordlistLoaded, setWordlistLoaded] = useState(cachedWordlist !== null);

  useEffect(() => {
    if (!wordlistLoaded) {
      wordlistReady.then(() => setWordlistLoaded(true));
    }
  }, [wordlistLoaded]);

  const wordRefs = useRef<Array<HTMLInputElement | null>>([]);
  const debounceRef = useRef<number | null>(null);

  const handlePaste = useCallback(
    (e: React.ClipboardEvent, slotIdx: number) => {
      const text = e.clipboardData.getData('text');
      const tokens = text
        .split(/\s+/)
        .map((t) => t.trim())
        .filter((t) => t.length > 0);
      if (tokens.length !== SEED_WORDS) {
        // Not a full-seed paste; let it fall through to the normal input
        return;
      }
      e.preventDefault();
      const next = new Array(SEED_WORDS).fill('');
      for (let i = 0; i < SEED_WORDS; i++) next[i] = tokens[i];
      setWords(next);
      setInfo('Make sure you also have a physical, written-down copy of your seed.');
      setActiveSuggestionFor(null);
      slotIdx; // unused; preserved for signature parity
    },
    [],
  );

  const updateWord = (idx: number, value: string) => {
    const next = words.slice();
    next[idx] = value;
    setWords(next);
    setHighlightedSuggestion(0);
    setActiveSuggestionFor(value.trim().length > 0 ? idx : null);
  };

  const acceptSuggestion = (slotIdx: number, suggestion: string) => {
    const next = words.slice();
    next[slotIdx] = suggestion;
    setWords(next);
    setActiveSuggestionFor(null);
    setHighlightedSuggestion(0);
    if (slotIdx + 1 < words.length) {
      wordRefs.current[slotIdx + 1]?.focus();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>, slotIdx: number) => {
    if (activeSuggestionFor !== slotIdx) {
      // Plain navigation: support arrow keys between slots
      if (e.key === 'ArrowLeft' && slotIdx > 0 && e.currentTarget.selectionStart === 0) {
        wordRefs.current[slotIdx - 1]?.focus();
      } else if (e.key === 'ArrowRight' && slotIdx + 1 < words.length && e.currentTarget.selectionStart === e.currentTarget.value.length) {
        wordRefs.current[slotIdx + 1]?.focus();
      }
      return;
    }
    const sugg = suggestionsFor(words[slotIdx]);
    if (sugg.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setHighlightedSuggestion((h) => Math.min(h + 1, sugg.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setHighlightedSuggestion((h) => Math.max(h - 1, 0));
    } else if (e.key === 'Enter' || e.key === 'Tab' || e.key === ' ') {
      e.preventDefault();
      acceptSuggestion(slotIdx, sugg[highlightedSuggestion]);
    } else if (e.key === 'Escape') {
      setActiveSuggestionFor(null);
    }
  };

  // Debounced validation — sends current words (joined) or hex to /decode-seed
  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    setError(null);

    const userInput =
      mode === 'words' ? words.map((w) => w.trim()).filter((w) => w.length > 0).join(' ') : hex.trim();

    if (mode === 'words') {
      const filled = words.filter((w) => w.trim().length > 0).length;
      if (filled !== words.length) {
        onInvalid();
        return;
      }
      // Quick client-side check: every word in the wordlist
      const badIdx = words.findIndex((w) => !isKnownWord(w));
      if (badIdx !== -1) {
        setError(`Word #${badIdx + 1} ("${words[badIdx]}") is not in the wordlist.`);
        onInvalid();
        return;
      }
    } else {
      if (!userInput) {
        onInvalid();
        return;
      }
      if (!/^[0-9a-fA-F]+$/.test(userInput)) {
        setError('Hex contains non-hex characters.');
        onInvalid();
        return;
      }
      if (userInput.length !== HEX_LENGTH) {
        setError(`Hex must be exactly ${HEX_LENGTH} characters (32 bytes).`);
        onInvalid();
        return;
      }
    }

    debounceRef.current = window.setTimeout(async () => {
      try {
        const resp = await decodeSeed(userInput);
        onValidSeedHex(resp.seedHex);
        setError(null);
      } catch (err: any) {
        const body = err?.response?.data;
        const msg = typeof body === 'string' ? body : err?.message || 'Invalid seed';
        setError(msg);
        onInvalid();
      }
    }, VALIDATE_DEBOUNCE_MS);

    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
    };
  }, [mode, words, hex, onValidSeedHex, onInvalid]);

  const switchMode = (next: Mode) => {
    if (next === mode) return;
    setMode(next);
    setError(null);
    setInfo(null);
    setActiveSuggestionFor(null);
  };

  const hexLooksOdd = useMemo(
    () => mode === 'hex' && hex.trim().length > 0 && hex.trim().length !== HEX_LENGTH,
    [mode, hex],
  );

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-1 p-1 rounded-lg bg-background border border-border w-fit">
        <button
          type="button"
          onClick={() => switchMode('words')}
          className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-md text-sm transition-colors ${
            mode === 'words'
              ? 'bg-primary/20 text-primary font-semibold'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <ListOrdered className="h-3.5 w-3.5" />
          Words
        </button>
        <button
          type="button"
          onClick={() => switchMode('hex')}
          className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-md text-sm transition-colors ${
            mode === 'hex'
              ? 'bg-primary/20 text-primary font-semibold'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <Hash className="h-3.5 w-3.5" />
          Hex
        </button>
      </div>

      {mode === 'words' ? (
        <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
          {words.map((w, i) => {
            const known = w.trim().length > 0 && isKnownWord(w);
            const unknown = w.trim().length > 0 && !known;
            const showSuggestions = activeSuggestionFor === i && !known;
            const sugg = showSuggestions ? suggestionsFor(w) : [];
            return (
              <div key={i} className="relative">
                <div
                  className={`flex items-center gap-2 px-2 py-1.5 rounded-lg bg-background border ${
                    unknown ? 'border-destructive' : known ? 'border-success/50' : 'border-border'
                  } focus-within:border-primary`}
                >
                  <span className="text-xs text-muted-foreground w-6 text-right shrink-0">{i + 1}.</span>
                  <input
                    ref={(el) => {
                      wordRefs.current[i] = el;
                    }}
                    type="text"
                    value={w}
                    onChange={(e) => updateWord(i, e.target.value)}
                    onPaste={(e) => handlePaste(e, i)}
                    onFocus={() => setActiveSuggestionFor(w.trim().length > 0 ? i : null)}
                    onBlur={() => window.setTimeout(() => setActiveSuggestionFor((cur) => (cur === i ? null : cur)), 150)}
                    onKeyDown={(e) => handleKeyDown(e, i)}
                    autoComplete="off"
                    autoCorrect="off"
                    spellCheck={false}
                    className="flex-1 bg-transparent text-sm focus:outline-none"
                  />
                  {known && <Check className="h-3.5 w-3.5 text-success shrink-0" />}
                </div>
                {sugg.length > 0 && (
                  <ul className="absolute left-0 right-0 top-full mt-1 z-10 rounded-lg bg-card border border-border shadow-lg text-sm overflow-hidden">
                    {sugg.map((s, sidx) => (
                      <li
                        key={s}
                        onMouseDown={(e) => {
                          e.preventDefault();
                          acceptSuggestion(i, s);
                        }}
                        className={`px-3 py-1 cursor-pointer ${
                          sidx === highlightedSuggestion ? 'bg-primary/15 text-primary' : 'hover:bg-muted/20'
                        }`}
                      >
                        {s}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <div>
          <label className="block text-sm text-muted-foreground mb-1">Seed hex</label>
          <textarea
            value={hex}
            onChange={(e) => setHex(e.target.value)}
            placeholder="64 hex characters (32 bytes)"
            rows={3}
            className="w-full px-3 py-2 rounded-lg bg-background border border-border text-foreground font-mono text-xs focus:outline-none focus:border-primary"
          />
          {hexLooksOdd && (
            <p className="mt-1 text-xs text-destructive">
              Hex must be exactly {HEX_LENGTH} characters (32 bytes).
            </p>
          )}
        </div>
      )}

      {info && (
        <p className="text-xs text-muted-foreground">{info}</p>
      )}

      {error && (
        <div className="flex items-start gap-2 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
};
