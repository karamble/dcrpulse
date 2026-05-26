// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Command dexassetgen generates the DCRDEX supported-asset catalog (per-asset
// wallet definitions and config-option schemas) as static JSON for the
// dashboard frontend, sourced from the pinned bisonw asset drivers.
//
// The bisonw RPC does not expose this catalog (it is only available over the
// webserver, which dcrpulse runs with --noweb), so it is generated from
// decred.org/dcrdex at the version dcrpulse builds bisonw from. Regenerate
// after bumping that version (keep the require in go.mod in sync with
// DCRDEX_BUILD_CONTEXT):
//
//	cd dashboard/cmd/dexassetgen
//	go run . > ../../internal/dexassets/catalog.generated.json
package main

import (
	"encoding/json"
	"os"
	"sort"

	"decred.org/dcrdex/client/asset"
	_ "decred.org/dcrdex/client/asset/importall"
	"decred.org/dcrdex/dex"
)

type configOption struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Default     string `json:"default"`
	NoEcho      bool   `json:"noEcho"`
	IsBoolean   bool   `json:"isBoolean"`
	IsDate      bool   `json:"isDate"`
	Repeatable  string `json:"repeatable,omitempty"`
	Required    bool   `json:"required"`
}

type walletDefinition struct {
	Type        string         `json:"type"`
	Tab         string         `json:"tab"`
	Seeded      bool           `json:"seeded"`
	Description string         `json:"description"`
	ConfigPath  string         `json:"configPath,omitempty"`
	GuideLink   string         `json:"guideLink,omitempty"`
	NoAuth      bool           `json:"noAuth"`
	ConfigOpts  []configOption `json:"configOpts"`
}

type unitInfo struct {
	AtomicUnit       string `json:"atomicUnit"`
	ConventionalUnit string `json:"conventionalUnit"`
	ConversionFactor uint64 `json:"conversionFactor"`
}

type tokenDef struct {
	ID         uint32           `json:"id"`
	Symbol     string           `json:"symbol"`
	Name       string           `json:"name"`
	ParentID   uint32           `json:"parentID"`
	UnitInfo   unitInfo         `json:"unitInfo"`
	Definition walletDefinition `json:"definition"`
}

type assetDef struct {
	ID               uint32             `json:"id"`
	Symbol           string             `json:"symbol"`
	Name             string             `json:"name"`
	IsAccountBased   bool               `json:"isAccountBased"`
	UnitInfo         unitInfo           `json:"unitInfo"`
	AvailableWallets []walletDefinition `json:"availableWallets"`
	Tokens           []tokenDef         `json:"tokens,omitempty"`
}

func convOpts(opts []*asset.ConfigOption) []configOption {
	out := make([]configOption, 0, len(opts))
	for _, o := range opts {
		out = append(out, configOption{
			Key:         o.Key,
			DisplayName: o.DisplayName,
			Description: o.Description,
			Default:     o.DefaultValue,
			NoEcho:      o.NoEcho,
			IsBoolean:   o.IsBoolean,
			IsDate:      o.IsDate,
			Repeatable:  o.Repeatable,
			Required:    o.Required,
		})
	}
	return out
}

func convDef(d *asset.WalletDefinition) walletDefinition {
	if d == nil {
		return walletDefinition{}
	}
	return walletDefinition{
		Type:        d.Type,
		Tab:         d.Tab,
		Seeded:      d.Seeded,
		Description: d.Description,
		ConfigPath:  d.DefaultConfigPath,
		GuideLink:   d.GuideLink,
		NoAuth:      d.NoAuth,
		ConfigOpts:  convOpts(d.ConfigOpts),
	}
}

func convUnitInfo(ui dex.UnitInfo) unitInfo {
	return unitInfo{
		AtomicUnit:       ui.AtomicUnit,
		ConventionalUnit: ui.Conventional.Unit,
		ConversionFactor: ui.Conventional.ConversionFactor,
	}
}

func main() {
	registered := asset.Assets()
	out := make([]assetDef, 0, len(registered))
	for id, ra := range registered {
		if ra.Info == nil {
			continue
		}
		defs := make([]walletDefinition, 0, len(ra.Info.AvailableWallets))
		for _, d := range ra.Info.AvailableWallets {
			defs = append(defs, convDef(d))
		}
		tokens := make([]tokenDef, 0, len(ra.Tokens))
		for tokenID, t := range ra.Tokens {
			if t == nil || t.Token == nil {
				continue
			}
			tokens = append(tokens, tokenDef{
				ID:         tokenID,
				Symbol:     dex.BipIDSymbol(tokenID),
				Name:       t.Name,
				ParentID:   t.ParentID,
				UnitInfo:   convUnitInfo(t.UnitInfo),
				Definition: convDef(t.Definition),
			})
		}
		sort.Slice(tokens, func(i, j int) bool { return tokens[i].ID < tokens[j].ID })
		out = append(out, assetDef{
			ID:               id,
			Symbol:           ra.Symbol,
			Name:             ra.Info.Name,
			IsAccountBased:   ra.Info.IsAccountBased,
			UnitInfo:         convUnitInfo(ra.Info.UnitInfo),
			AvailableWallets: defs,
			Tokens:           tokens,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}
