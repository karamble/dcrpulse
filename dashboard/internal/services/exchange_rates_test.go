// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC license.
package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"dcrpulse/pkg/exchangerate"
)

// withRates turns exchange rates on or off for one test.
func withRates(t *testing.T, on bool) {
	t.Helper()
	old := ExchangeRatesEnabled
	ExchangeRatesEnabled = func() bool { return on }
	t.Cleanup(func() { ExchangeRatesEnabled = old })
}

// fakeDaemons records what ApplyExchangeRates tells each daemon.
func fakeDaemons(t *testing.T, brErr, dexErr error) (br *[]bool, dex *[]string) {
	t.Helper()
	oldBR, oldDex := setBrclientdExchangeRates, setDexRateSource
	t.Cleanup(func() { setBrclientdExchangeRates, setDexRateSource = oldBR, oldDex })
	br, dex = &[]bool{}, &[]string{}
	setBrclientdExchangeRates = func(_ context.Context, on bool) error {
		*br = append(*br, on)
		return brErr
	}
	setDexRateSource = func(_ context.Context, source string, disable bool) error {
		*dex = append(*dex, fmt.Sprintf("%s disable=%v", source, disable))
		return dexErr
	}
	return br, dex
}

// withChoice stands in for the active wallet's stored rate-source choice and
// counts how often it is read.
func withChoice(t *testing.T, stored map[string]bool, err error) *int {
	t.Helper()
	old := readDexRateSources
	t.Cleanup(func() { readDexRateSources = old })
	reads := new(int)
	readDexRateSources = func(context.Context) (map[string]bool, error) {
		*reads++
		return stored, err
	}
	return reads
}

func TestApplyExchangeRatesTellsBothDaemons(t *testing.T) {
	for _, on := range []bool{false, true} {
		withRates(t, on)
		withChoice(t, nil, nil)
		br, dex := fakeDaemons(t, nil, nil)
		if missed := ApplyExchangeRates(context.Background()); len(missed) != 0 {
			t.Fatalf("on=%v missed %v", on, missed)
		}
		want := []string{
			fmt.Sprintf("Coinpaprika disable=%v", !on),
			fmt.Sprintf("dcrdata disable=%v", !on),
			"Messari disable=true",
		}
		if !reflect.DeepEqual(*br, []bool{on}) || !reflect.DeepEqual(*dex, want) {
			t.Fatalf("on=%v: brclientd %v, dcrdex %v", on, *br, *dex)
		}
	}
}

func TestApplyExchangeRatesNamesWhatItCouldNotReach(t *testing.T) {
	withRates(t, false)
	_, dex := fakeDaemons(t, errors.New("brclientd down"), errors.New("dex locked"))
	if missed := ApplyExchangeRates(context.Background()); !reflect.DeepEqual(missed, []string{"brclientd", "dcrdex"}) {
		t.Fatalf("missed %v", missed)
	}
	if len(*dex) != 1 {
		t.Fatalf("kept asking a locked DCRDEX: %v", *dex)
	}
}

func TestApplyDexRateSourcesKeepsAChosenOffSourceOff(t *testing.T) {
	withRates(t, true)
	withChoice(t, map[string]bool{"dcrdata": false}, nil)
	_, dex := fakeDaemons(t, nil, nil)
	if err := ApplyDexRateSources(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"Coinpaprika disable=false", "dcrdata disable=true", "Messari disable=true"}
	if !reflect.DeepEqual(*dex, want) {
		t.Fatalf("dcrdex told %v, want %v", *dex, want)
	}
}

func TestApplyDexRateSourcesAcceptsADaemonThatNoLongerKnowsMessari(t *testing.T) {
	withRates(t, true)
	withChoice(t, nil, nil)
	for _, tc := range []struct {
		messariErr error
		wantErr    bool
	}{
		{errors.New("error disabling/enabling rate source: cannot disable unknown fiat rate source"), false},
		{errors.New("not logged in"), true},
	} {
		_, dex := fakeDaemons(t, nil, nil)
		setDexRateSource = func(_ context.Context, source string, disable bool) error {
			*dex = append(*dex, source)
			if source == "Messari" {
				return tc.messariErr
			}
			return nil
		}
		err := ApplyDexRateSources(context.Background())
		if (err != nil) != tc.wantErr {
			t.Fatalf("Messari answering %q: err %v", tc.messariErr, err)
		}
		if len(*dex) != 3 {
			t.Fatalf("dcrdex asked about %v", *dex)
		}
	}
}

func TestApplyExchangeRatesTogglesNothingWhenTheChoiceIsUnreadable(t *testing.T) {
	withRates(t, true)
	withChoice(t, nil, errors.New("parse config"))
	_, dex := fakeDaemons(t, nil, nil)
	if missed := ApplyExchangeRates(context.Background()); !reflect.DeepEqual(missed, []string{"dcrdex"}) {
		t.Fatalf("missed %v", missed)
	}
	if len(*dex) != 0 {
		t.Fatalf("dcrdex told %v from an unreadable choice", *dex)
	}
}

func TestApplyExchangeRatesOffNeverReadsTheChoice(t *testing.T) {
	withRates(t, false)
	reads := withChoice(t, nil, errors.New("parse config"))
	_, dex := fakeDaemons(t, nil, nil)
	if missed := ApplyExchangeRates(context.Background()); len(missed) != 0 {
		t.Fatalf("missed %v", missed)
	}
	want := []string{"Coinpaprika disable=true", "dcrdata disable=true", "Messari disable=true"}
	if *reads != 0 || !reflect.DeepEqual(*dex, want) {
		t.Fatalf("%d reads, dcrdex told %v", *reads, *dex)
	}
}

func TestDexRateSourceChoiceTurnsAnAbsentSourceOn(t *testing.T) {
	withChoice(t, map[string]bool{"dcrdata": false, "Messari": true}, nil)
	got, err := DexRateSourceChoice(context.Background())
	want := map[string]bool{"Coinpaprika": true, "dcrdata": false}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("choice %v (%v), want %v", got, err, want)
	}
}

func TestSaveDexRateSourcesStoresOnlyChoosableSources(t *testing.T) {
	old := writeDexRateSources
	t.Cleanup(func() { writeDexRateSources = old })
	var written []map[string]bool
	writeDexRateSources = func(_ context.Context, update map[string]bool) error {
		written = append(written, update)
		return nil
	}
	for _, name := range []string{"Messari", "Kraken"} {
		err := SaveDexRateSources(context.Background(), map[string]bool{"dcrdata": false, name: true})
		if !errors.Is(err, ErrUnknownDexRateSource) {
			t.Fatalf("%s: err %v", name, err)
		}
	}
	if len(written) != 0 {
		t.Fatalf("stored %v from a refused save", written)
	}
	if err := SaveDexRateSources(context.Background(), map[string]bool{"dcrdata": false}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, []map[string]bool{{"dcrdata": false}}) {
		t.Fatalf("stored %v", written)
	}
}

type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("no network in tests")
}

func TestDeviceBalanceRateAsksNobodyWhenRatesAreOff(t *testing.T) {
	withRates(t, false)
	rt := &countingTransport{}
	old := deviceRates
	deviceRates = exchangerate.New(rt)
	t.Cleanup(func() { deviceRates = old })
	if r := deviceBalanceRate(context.Background()); r != 0 || rt.calls.Load() != 0 {
		t.Fatalf("rate %v after %d requests", r, rt.calls.Load())
	}
	withRates(t, true)
	deviceBalanceRate(context.Background())
	if rt.calls.Load() == 0 {
		t.Fatal("rates on made no request")
	}
}
