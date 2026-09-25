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

func TestApplyExchangeRatesTellsBothDaemons(t *testing.T) {
	for _, on := range []bool{false, true} {
		withRates(t, on)
		br, dex := fakeDaemons(t, nil, nil)
		if missed := ApplyExchangeRates(context.Background()); len(missed) != 0 {
			t.Fatalf("on=%v missed %v", on, missed)
		}
		want := []string{
			fmt.Sprintf("Messari disable=%v", !on),
			fmt.Sprintf("Coinpaprika disable=%v", !on),
			fmt.Sprintf("dcrdata disable=%v", !on),
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
