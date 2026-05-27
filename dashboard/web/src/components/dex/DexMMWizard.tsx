// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

import { useEffect, useState } from 'react';
import {
  getMMMarketReport,
  startMMBot,
  updateMMBotConfig,
  type DexMarket,
  type MMAllocation,
  type MMBotConfig,
  type MMBotStatus,
  type MMCexStatus,
  type MMMarketReport,
} from '../../services/dcrdexApi';
import { DexMMMarketSelector } from './DexMMMarketSelector';
import { DexMMBotTypeSelector } from './DexMMBotTypeSelector';
import { DexMMConfigStep } from './DexMMConfigStep';
import { DexMMFundingDialog } from './DexMMFundingDialog';
import { botTypeOf, defaultDraft, draftFromConfig, needsCex, type BotType, type ConfigDraft } from './dexMMConfig';

type Step = 'market' | 'type' | 'config';

// DexMMWizard guides bot setup as a three-step flow (market -> bot type ->
// configure), mirroring the v1.0.6 market-maker settings page. For editing an
// existing bot it jumps straight to the config step with market and type locked.
// Allocation is collected at start time via the funding dialog, not stored in
// the saved config.
export const DexMMWizard = ({
  host,
  markets,
  cexes,
  editBot,
  refresh,
  onCexSaved,
  onDone,
}: {
  host: string;
  markets: DexMarket[];
  cexes: Record<string, MMCexStatus>;
  editBot?: MMBotStatus;
  refresh: () => void;
  onCexSaved: () => void;
  onDone: () => void;
}) => {
  const editing = !!editBot;
  const editMarket = editBot
    ? markets.find((m) => m.baseID === editBot.config.baseID && m.quoteID === editBot.config.quoteID) ?? null
    : null;

  const [step, setStep] = useState<Step>(editing ? 'config' : 'market');
  const [market, setMarket] = useState<DexMarket | null>(editMarket);
  const [botType, setBotType] = useState<BotType>(editBot ? botTypeOf(editBot.config) : 'basicmm');
  const [cexName, setCexName] = useState<string | undefined>(editBot?.config.cexName);
  const [draft, setDraft] = useState<ConfigDraft>(
    editBot ? draftFromConfig(editBot.config) : defaultDraft('basicmm'),
  );
  const [report, setReport] = useState<MMMarketReport | null>(null);
  const [fundingCfg, setFundingCfg] = useState<MMBotConfig | null>(null);
  const [startBusy, setStartBusy] = useState(false);

  // Load the market report whenever a market is selected; it feeds the config
  // step's placements chart, oracle table, and lots-to-USD hints.
  useEffect(() => {
    if (!market) return;
    let live = true;
    setReport(null);
    getMMMarketReport(host, market.baseID, market.quoteID)
      .then((r) => {
        if (live) setReport(r);
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [host, market]);

  const handleMarketSelect = (m: DexMarket) => {
    setMarket(m);
    setStep('type');
  };

  const handleBotTypeContinue = (t: BotType, cex?: string) => {
    setBotType(t);
    setCexName(cex);
    setDraft(defaultDraft(t, cex));
    setStep('config');
  };

  const saveConfig = async (cfg: MMBotConfig) => {
    await updateMMBotConfig(cfg);
    refresh();
  };

  const handleSave = async (cfg: MMBotConfig) => {
    await saveConfig(cfg);
    onDone();
  };

  const handleSaveAndStart = async (cfg: MMBotConfig) => {
    await saveConfig(cfg);
    setFundingCfg(cfg);
  };

  const handleFundingConfirm = async (alloc: MMAllocation) => {
    if (!fundingCfg) return;
    setStartBusy(true);
    try {
      await startMMBot({ host, baseID: fundingCfg.baseID, quoteID: fundingCfg.quoteID, alloc });
      refresh();
      onDone();
    } catch {
      setStartBusy(false);
    }
  };

  if (step === 'market') {
    return <DexMMMarketSelector markets={markets} cexes={cexes} onSelect={handleMarketSelect} />;
  }

  if (step === 'type' && market) {
    return (
      <DexMMBotTypeSelector
        market={market}
        cexes={cexes}
        initialBotType={botType}
        initialCex={cexName}
        onContinue={handleBotTypeContinue}
        onChangeMarket={() => setStep('market')}
        onCexSaved={onCexSaved}
      />
    );
  }

  if (step === 'config' && market) {
    return (
      <>
        <DexMMConfigStep
          key={`${market.baseID}-${market.quoteID}-${botType}-${cexName ?? ''}`}
          host={host}
          market={market}
          initial={draft}
          editing={editing}
          report={report}
          onChangeMarket={() => setStep('market')}
          onChangeBotType={() => setStep('type')}
          onSave={handleSave}
          onSaveAndStart={handleSaveAndStart}
          onCancel={onDone}
        />
        {fundingCfg && (
          <DexMMFundingDialog
            market={market}
            needsCex={needsCex(botType)}
            busy={startBusy}
            onConfirm={handleFundingConfirm}
            onCancel={() => {
              setFundingCfg(null);
              onDone();
            }}
          />
        )}
      </>
    );
  }

  return null;
};
