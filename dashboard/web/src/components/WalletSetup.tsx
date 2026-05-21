// Copyright (c) 2015-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useState } from 'react';
import { Copy, Check, AlertCircle, Lock, Key, Shield, CheckCircle, Eye, EyeOff, Sprout, RotateCcw } from 'lucide-react';
import { generateSeed, createWallet } from '../services/api';
import { SeedEntry } from './wallet/SeedEntry';

type WizardMode = 'create' | 'restore';
type WizardStep =
  | 'choose'
  | 'welcome'
  | 'generate'
  | 'restore-welcome'
  | 'restore-seed'
  | 'passphrases'
  | 'confirm'
  | 'creating'
  | 'success';

export const WalletSetup = () => {
  const [step, setStep] = useState<WizardStep>('choose');
  const [mode, setMode] = useState<WizardMode>('create');
  const [seedMnemonic, setSeedMnemonic] = useState('');
  const [seedHex, setSeedHex] = useState('');
  const [seedHexValid, setSeedHexValid] = useState(false);
  const [publicPassphrase, setPublicPassphrase] = useState('');
  const [privatePassphrase, setPrivatePassphrase] = useState('');
  const [confirmPublicPass, setConfirmPublicPass] = useState('');
  const [confirmPrivatePass, setConfirmPrivatePass] = useState('');
  const [seedBackupConfirmed, setSeedBackupConfirmed] = useState(false);
  const [confirmWords, setConfirmWords] = useState<Record<number, string>>({});
  const [randomWordIndices, setRandomWordIndices] = useState<number[]>([]);
  const [copied, setCopied] = useState(false);
  const [showPublicPass, setShowPublicPass] = useState(false);
  const [showPrivatePass, setShowPrivatePass] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Generate seed when entering generate step
  const handleGenerateSeed = async () => {
    try {
      setError(null);
      const response = await generateSeed(0);
      setSeedMnemonic(response.seedMnemonic);
      setSeedHex(response.seedHex);
      setStep('generate');
      
      // Select 3 random words to verify later
      const words = response.seedMnemonic.split(' ');
      const indices: number[] = [];
      while (indices.length < 3) {
        const idx = Math.floor(Math.random() * words.length);
        if (!indices.includes(idx)) {
          indices.push(idx);
        }
      }
      setRandomWordIndices(indices.sort((a, b) => a - b));
    } catch (err) {
      setError('Failed to generate seed. Please try again.');
      console.error(err);
    }
  };

  const copySeedToClipboard = () => {
    navigator.clipboard.writeText(seedMnemonic);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const getPassphraseStrength = (pass: string): { strength: string; color: string; width: string } => {
    if (pass.length === 0) return { strength: '', color: 'bg-muted', width: 'w-0' };
    if (pass.length < 8) return { strength: 'Weak', color: 'bg-red-500', width: 'w-1/4' };
    if (pass.length < 12) return { strength: 'Fair', color: 'bg-orange-500', width: 'w-2/4' };
    if (pass.length < 16) return { strength: 'Good', color: 'bg-yellow-500', width: 'w-3/4' };
    return { strength: 'Strong', color: 'bg-green-500', width: 'w-full' };
  };

  const validatePassphrases = (): boolean => {
    // Public passphrase is optional
    if (publicPassphrase && publicPassphrase !== confirmPublicPass) {
      setError('Public passphrases do not match');
      return false;
    }
    // Private passphrase is mandatory
    if (!privatePassphrase) {
      setError('Private passphrase is required');
      return false;
    }
    if (privatePassphrase !== confirmPrivatePass) {
      setError('Private passphrases do not match');
      return false;
    }
    return true;
  };

  const validateSeedConfirmation = (): boolean => {
    const words = seedMnemonic.split(' ');
    for (const idx of randomWordIndices) {
      if (confirmWords[idx]?.toLowerCase().trim() !== words[idx].toLowerCase()) {
        setError(`Word #${idx + 1} does not match. Please check your backup.`);
        return false;
      }
    }
    return true;
  };

  const handleCreateWallet = async () => {
    try {
      setError(null);
      
      const response = await createWallet({
        publicPassphrase,
        privatePassphrase,
        seedHex,
        discoverAccounts: mode === 'restore',
      });

      if (response.success) {
        setStep('success');
        // Reload page after 2 seconds to load wallet dashboard
        setTimeout(() => {
          window.location.reload();
        }, 2000);
      } else {
        setError(response.message || 'Failed to create wallet');
        setStep(mode === 'restore' ? 'passphrases' : 'confirm');
      }
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to create wallet. Please try again.');
      setStep(mode === 'restore' ? 'passphrases' : 'confirm');
      console.error(err);
    }
  };

  const words = seedMnemonic ? seedMnemonic.split(' ') : [];
  const publicStrength = getPassphraseStrength(publicPassphrase);
  const privateStrength = getPassphraseStrength(privatePassphrase);

  // Live passphrase-match indicators. Empty confirm field = no indicator.
  const publicMatch = publicPassphrase.length > 0 && confirmPublicPass.length > 0
    ? publicPassphrase === confirmPublicPass
    : null;
  const privateMatch = privatePassphrase.length > 0 && confirmPrivatePass.length > 0
    ? privatePassphrase === confirmPrivatePass
    : null;
  const canSubmitPassphrases =
    privatePassphrase.length > 0 &&
    privatePassphrase === confirmPrivatePass &&
    (publicPassphrase.length === 0 || publicPassphrase === confirmPublicPass);

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-gradient-to-br from-background via-background to-muted/20">
      <div className="w-full max-w-3xl">
        <div className="bg-gradient-card backdrop-blur-sm border border-border/50 rounded-xl shadow-xl p-8 animate-fade-in">
          {/* Chooser Step */}
          {step === 'choose' && (
            <div className="text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 rounded-full bg-primary/10 border border-primary/20">
                  <Shield className="h-12 w-12 text-primary" />
                </div>
              </div>
              <h1 className="text-3xl font-bold">Welcome to DCRPulse</h1>
              <p className="text-lg text-muted-foreground max-w-xl mx-auto">
                Get started by creating a new Decred wallet, or restore an existing one from its seed phrase.
              </p>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 pt-2">
                <button
                  onClick={() => {
                    setMode('create');
                    setError(null);
                    setStep('welcome');
                  }}
                  className="p-6 rounded-xl border border-border bg-background/50 hover:border-primary/50 hover:bg-background transition-colors text-left space-y-2"
                >
                  <div className="flex items-center gap-3">
                    <Sprout className="h-6 w-6 text-primary" />
                    <h3 className="text-lg font-semibold">Create new wallet</h3>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Generate a fresh seed phrase, set passphrases, and start with an empty wallet.
                  </p>
                </button>
                <button
                  onClick={() => {
                    setMode('restore');
                    setError(null);
                    setStep('restore-welcome');
                  }}
                  className="p-6 rounded-xl border border-border bg-background/50 hover:border-primary/50 hover:bg-background transition-colors text-left space-y-2"
                >
                  <div className="flex items-center gap-3">
                    <RotateCcw className="h-6 w-6 text-primary" />
                    <h3 className="text-lg font-semibold">Restore from seed</h3>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    Recover an existing wallet by entering its 33-word seed phrase or raw hex.
                  </p>
                </button>
              </div>
            </div>
          )}

          {/* Restore Welcome Step */}
          {step === 'restore-welcome' && (
            <div className="text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 rounded-full bg-primary/10 border border-primary/20">
                  <RotateCcw className="h-12 w-12 text-primary" />
                </div>
              </div>
              <h1 className="text-3xl font-bold">Restore your wallet</h1>
              <p className="text-lg text-muted-foreground max-w-xl mx-auto">
                Enter the seed phrase from your existing Decred wallet. The dashboard will rescan the blockchain to rebuild your address history.
              </p>

              <div className="bg-warning/10 border border-warning/20 rounded-lg p-4 text-left space-y-2">
                <div className="flex items-start gap-2">
                  <AlertCircle className="h-5 w-5 text-warning flex-shrink-0 mt-0.5" />
                  <div className="space-y-2 text-sm">
                    <p className="font-semibold text-warning">What to expect</p>
                    <ul className="space-y-1 text-muted-foreground">
                      <li>• Decred seeds are 33 words. Or paste the raw hex (32 bytes) if you have that instead.</li>
                      <li>• After restore, dcrwallet rescans the chain from genesis - this can take a while.</li>
                      <li>• Keep the dashboard running during the rescan.</li>
                    </ul>
                  </div>
                </div>
              </div>

              <div className="flex gap-4">
                <button
                  onClick={() => {
                    setError(null);
                    setStep('choose');
                  }}
                  className="flex-1 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => {
                    setError(null);
                    setStep('restore-seed');
                  }}
                  className="flex-1 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors font-semibold"
                >
                  Continue
                </button>
              </div>
            </div>
          )}

          {/* Restore Seed Entry Step */}
          {step === 'restore-seed' && (
            <div className="space-y-6">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
                  <Key className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold">Enter your seed</h2>
                  <p className="text-sm text-muted-foreground">Words or hex - paste supported.</p>
                </div>
              </div>

              <SeedEntry
                onValidSeedHex={(hex) => {
                  setSeedHex(hex);
                  setSeedHexValid(true);
                }}
                onInvalid={() => setSeedHexValid(false)}
              />

              <div className="flex gap-4">
                <button
                  onClick={() => {
                    setError(null);
                    setSeedHexValid(false);
                    setStep('restore-welcome');
                  }}
                  className="flex-1 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => {
                    setError(null);
                    setStep('passphrases');
                  }}
                  disabled={!seedHexValid}
                  className="flex-1 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-semibold"
                >
                  Continue
                </button>
              </div>
            </div>
          )}

          {/* Welcome Step */}
          {step === 'welcome' && (
            <div className="text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 rounded-full bg-primary/10 border border-primary/20">
                  <Shield className="h-12 w-12 text-primary" />
                </div>
              </div>
              
              <h1 className="text-3xl font-bold">Welcome to DCRPulse</h1>
              <p className="text-lg text-muted-foreground max-w-xl mx-auto">
                Let's create your Decred wallet. You'll receive a 33-word seed phrase that can recover your wallet.
              </p>

              <div className="bg-warning/10 border border-warning/20 rounded-lg p-4 text-left space-y-2">
                <div className="flex items-start gap-2">
                  <AlertCircle className="h-5 w-5 text-warning flex-shrink-0 mt-0.5" />
                  <div className="space-y-2 text-sm">
                    <p className="font-semibold text-warning">Important Security Information</p>
                    <ul className="space-y-1 text-muted-foreground">
                      <li>• Your seed phrase is the ONLY way to recover your wallet</li>
                      <li>• Write it down on paper and store it securely offline</li>
                      <li>• Never share your seed phrase with anyone</li>
                      <li>• You'll need to confirm your backup before proceeding</li>
                    </ul>
                  </div>
                </div>
              </div>

              <div className="flex flex-col sm:flex-row gap-3 justify-center">
                <button
                  onClick={() => {
                    setError(null);
                    setStep('choose');
                  }}
                  className="px-6 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={handleGenerateSeed}
                  className="px-8 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors font-semibold text-lg"
                >
                  Create New Wallet
                </button>
              </div>
            </div>
          )}

          {/* Generate Seed Step */}
          {step === 'generate' && (
            <div className="space-y-6">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
                  <Key className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold">Your Seed Phrase</h2>
                  <p className="text-sm text-muted-foreground">33-word recovery phrase</p>
                </div>
              </div>

              <div className="bg-muted/20 border border-border rounded-lg p-6">
                <div className="grid grid-cols-3 gap-3">
                  {words.map((word, idx) => (
                    <div key={idx} className="flex items-center gap-2 bg-background/50 rounded p-2">
                      <span className="text-xs text-muted-foreground w-6">{idx + 1}.</span>
                      <span className="font-mono font-semibold">{word}</span>
                    </div>
                  ))}
                </div>
              </div>

              <button
                onClick={copySeedToClipboard}
                className="w-full flex items-center justify-center gap-2 py-2 px-4 border border-border rounded-lg hover:bg-background/50 transition-colors"
              >
                {copied ? (
                  <>
                    <Check className="h-4 w-4 text-green-500" />
                    <span>Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy className="h-4 w-4" />
                    <span>Copy to Clipboard</span>
                  </>
                )}
              </button>

              <div className="flex items-start gap-3 p-4 bg-warning/10 border border-warning/20 rounded-lg">
                <input
                  type="checkbox"
                  checked={seedBackupConfirmed}
                  onChange={(e) => setSeedBackupConfirmed(e.target.checked)}
                  className="mt-1"
                />
                <label className="text-sm text-muted-foreground cursor-pointer" onClick={() => setSeedBackupConfirmed(!seedBackupConfirmed)}>
                  I have written down all 33 words of my seed phrase and stored them in a secure location. I understand that without this seed phrase, I cannot recover my wallet.
                </label>
              </div>

              <div className="flex gap-4">
                <button
                  onClick={() => setStep('welcome')}
                  className="flex-1 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => {
                    if (seedBackupConfirmed) {
                      setError(null);
                      setStep('passphrases');
                    } else {
                      setError('Please confirm you have backed up your seed phrase');
                    }
                  }}
                  disabled={!seedBackupConfirmed}
                  className="flex-1 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-semibold"
                >
                  Continue
                </button>
              </div>
            </div>
          )}

          {/* Set Passphrases Step */}
          {step === 'passphrases' && (
            <div className="space-y-6">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
                  <Lock className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold">Set Passphrases</h2>
                  <p className="text-sm text-muted-foreground">Encrypt your wallet with strong passphrases</p>
                </div>
              </div>

              <div className="bg-muted/20 border border-border/30 rounded-lg p-4 mb-4">
                <div className="flex items-start gap-2">
                  <AlertCircle className="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
                  <div className="space-y-1 text-sm">
                    <p className="font-semibold">Two Types of Passphrases</p>
                    <p className="text-muted-foreground">
                      <strong>Public passphrase</strong> (optional): Encrypts wallet for viewing. You can leave this empty.
                    </p>
                    <p className="text-muted-foreground">
                      <strong>Private passphrase</strong> (required): Encrypts private keys. Required for all spending operations.
                    </p>
                  </div>
                </div>
              </div>

              <div className="space-y-4">
                {/* Public Passphrase */}
                <div className="space-y-2">
                  <label className="text-sm font-medium">Public Passphrase <span className="text-muted-foreground">(Optional)</span></label>
                  <p className="text-xs text-muted-foreground">Encrypts wallet database for viewing. Leave empty for no encryption.</p>
                  <div className="relative">
                    <input
                      type={showPublicPass ? 'text' : 'password'}
                      value={publicPassphrase}
                      onChange={(e) => setPublicPassphrase(e.target.value)}
                      className="w-full px-4 py-2 pr-10 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                      placeholder="Enter public passphrase (optional)"
                    />
                    <button
                      onClick={() => setShowPublicPass(!showPublicPass)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      {showPublicPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                  {publicPassphrase && (
                    <div className="space-y-1">
                      <div className="h-2 bg-muted rounded-full overflow-hidden">
                        <div className={`h-full ${publicStrength.color} ${publicStrength.width} transition-all`} />
                      </div>
                      <p className="text-xs text-muted-foreground">Strength: {publicStrength.strength}</p>
                    </div>
                  )}
                  {publicPassphrase && (
                    <>
                      <input
                        type={showPublicPass ? 'text' : 'password'}
                        value={confirmPublicPass}
                        onChange={(e) => setConfirmPublicPass(e.target.value)}
                        className={`w-full px-4 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary ${
                          publicMatch === false ? 'border-red-500/50' : publicMatch === true ? 'border-green-500/50' : 'border-border'
                        }`}
                        placeholder="Confirm public passphrase"
                      />
                      {publicMatch === true && (
                        <p className="text-xs text-green-500 flex items-center gap-1">
                          <Check className="h-3 w-3" /> Public passphrases match
                        </p>
                      )}
                      {publicMatch === false && (
                        <p className="text-xs text-red-500 flex items-center gap-1">
                          <AlertCircle className="h-3 w-3" /> Public passphrases do not match
                        </p>
                      )}
                    </>
                  )}
                </div>

                {/* Private Passphrase */}
                <div className="space-y-2">
                  <label className="text-sm font-medium">Private Passphrase <span className="text-red-500">*</span></label>
                  <p className="text-xs text-muted-foreground">Required: Encrypts private keys for sending/signing transactions</p>
                  <div className="relative">
                    <input
                      type={showPrivatePass ? 'text' : 'password'}
                      value={privatePassphrase}
                      onChange={(e) => setPrivatePassphrase(e.target.value)}
                      className="w-full px-4 py-2 pr-10 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                      placeholder="Enter private passphrase"
                    />
                    <button
                      onClick={() => setShowPrivatePass(!showPrivatePass)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                    >
                      {showPrivatePass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                    </button>
                  </div>
                  {privatePassphrase && (
                    <div className="space-y-1">
                      <div className="h-2 bg-muted rounded-full overflow-hidden">
                        <div className={`h-full ${privateStrength.color} ${privateStrength.width} transition-all`} />
                      </div>
                      <p className="text-xs text-muted-foreground">Strength: {privateStrength.strength}</p>
                    </div>
                  )}
                  <input
                    type={showPrivatePass ? 'text' : 'password'}
                    value={confirmPrivatePass}
                    onChange={(e) => setConfirmPrivatePass(e.target.value)}
                    className={`w-full px-4 py-2 bg-background border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary ${
                      privateMatch === false ? 'border-red-500/50' : privateMatch === true ? 'border-green-500/50' : 'border-border'
                    }`}
                    placeholder="Confirm private passphrase"
                  />
                  {privateMatch === true && (
                    <p className="text-xs text-green-500 flex items-center gap-1">
                      <Check className="h-3 w-3" /> Private passphrases match
                    </p>
                  )}
                  {privateMatch === false && (
                    <p className="text-xs text-red-500 flex items-center gap-1">
                      <AlertCircle className="h-3 w-3" /> Private passphrases do not match
                    </p>
                  )}
                </div>
              </div>

              <div className="flex gap-4">
                <button
                  onClick={() => setStep(mode === 'restore' ? 'restore-seed' : 'generate')}
                  className="flex-1 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => {
                    if (validatePassphrases()) {
                      setError(null);
                      if (mode === 'restore') {
                        setStep('creating');
                        handleCreateWallet();
                      } else {
                        setStep('confirm');
                      }
                    }
                  }}
                  disabled={!canSubmitPassphrases}
                  className="flex-1 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors font-semibold disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {mode === 'restore' ? 'Restore Wallet' : 'Continue'}
                </button>
              </div>
            </div>
          )}

          {/* Confirm Seed Step */}
          {step === 'confirm' && (
            <div className="space-y-6">
              <div className="flex items-center gap-3">
                <div className="p-2 rounded-lg bg-primary/10 border border-primary/20">
                  <CheckCircle className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <h2 className="text-2xl font-bold">Confirm Your Seed</h2>
                  <p className="text-sm text-muted-foreground">Enter the following words from your seed phrase</p>
                </div>
              </div>

              <div className="space-y-4">
                {randomWordIndices.map((idx) => (
                  <div key={idx} className="space-y-2">
                    <label className="text-sm font-medium">Word #{idx + 1}</label>
                    <input
                      type="text"
                      value={confirmWords[idx] || ''}
                      onChange={(e) => {
                        setConfirmWords({ ...confirmWords, [idx]: e.target.value });
                        // Clear error when user starts typing
                        if (error) setError(null);
                      }}
                      className="w-full px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:ring-2 focus:ring-primary"
                      placeholder={`Enter word #${idx + 1}`}
                      autoComplete="off"
                    />
                  </div>
                ))}
              </div>

              <div className="flex gap-4">
                <button
                  onClick={() => setStep('passphrases')}
                  className="flex-1 py-3 border border-border rounded-lg hover:bg-background/50 transition-colors"
                >
                  Back
                </button>
                <button
                  onClick={() => {
                    if (validateSeedConfirmation()) {
                      setError(null);
                      setStep('creating');
                      handleCreateWallet();
                    }
                  }}
                  className="flex-1 py-3 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors font-semibold"
                >
                  Create Wallet
                </button>
              </div>
            </div>
          )}

          {/* Creating Step */}
          {step === 'creating' && (
            <div className="text-center space-y-6">
              <div className="flex justify-center">
                <div className="animate-spin rounded-full h-16 w-16 border-b-2 border-primary"></div>
              </div>
              <h2 className="text-2xl font-bold">Creating Your Wallet...</h2>
              <p className="text-muted-foreground">Please wait while we securely create your wallet</p>
            </div>
          )}

          {/* Success Step */}
          {step === 'success' && (
            <div className="text-center space-y-6">
              <div className="flex justify-center">
                <div className="p-4 rounded-full bg-green-500/10 border border-green-500/20">
                  <CheckCircle className="h-12 w-12 text-green-500" />
                </div>
              </div>
              <h2 className="text-2xl font-bold text-green-500">Wallet Created Successfully!</h2>
              <p className="text-muted-foreground">Redirecting to your dashboard...</p>
            </div>
          )}

          {/* Error Display */}
          {error && step !== 'creating' && (
            <div className="mt-4 p-4 bg-red-500/10 border border-red-500/20 rounded-lg flex items-start gap-2">
              <AlertCircle className="h-5 w-5 text-red-500 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-500">{error}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

