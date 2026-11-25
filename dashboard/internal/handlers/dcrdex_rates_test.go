// Copyright (c) 2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"dcrpulse/internal/services"
)

// rateRoutes stands in for the rate-source steps and records what the routes
// save and apply.
type rateRoutes struct {
	saved   []map[string]bool
	applies int
}

func withRateRoutes(t *testing.T, ratesOn bool, choice map[string]bool, saveErr, applyErr error) *rateRoutes {
	t.Helper()
	prevChoice, prevSave, prevApply, prevOn := dexRateSourceChoice, saveDexRateSources, applyDexRateSources, exchangeRatesEnabled
	t.Cleanup(func() {
		dexRateSourceChoice, saveDexRateSources, applyDexRateSources, exchangeRatesEnabled = prevChoice, prevSave, prevApply, prevOn
	})
	rr := &rateRoutes{}
	dexRateSourceChoice = func(context.Context) (map[string]bool, error) { return choice, nil }
	saveDexRateSources = func(_ context.Context, update map[string]bool) error {
		if saveErr != nil {
			return saveErr
		}
		rr.saved = append(rr.saved, update)
		return nil
	}
	applyDexRateSources = func(context.Context) error {
		rr.applies++
		return applyErr
	}
	exchangeRatesEnabled = func() bool { return ratesOn }
	return rr
}

func TestGetDcrdexRateSourcesListsTheChoosableSourcesInOrder(t *testing.T) {
	withRateRoutes(t, false, map[string]bool{"Coinpaprika": true, "dcrdata": false}, nil, nil)
	rec := httptest.NewRecorder()
	GetDcrdexRateSourcesHandler(rec, httptest.NewRequest(http.MethodGet, "/api/dcrdex/rate-sources", nil))
	want := `{"exchangeRates":false,"sources":[{"name":"Coinpaprika","enabled":true},{"name":"dcrdata","enabled":false}]}`
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("%d %s, want %s", rec.Code, rec.Body.String(), want)
	}
}

func TestSaveDcrdexRateSourcesStoresThenApplies(t *testing.T) {
	for _, tc := range []struct {
		applyErr error
		want     string
	}{
		{nil, `{"applied":true}`},
		{errors.New("not logged in"), `{"applied":false}`},
	} {
		rr := withRateRoutes(t, true, nil, nil, tc.applyErr)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/dcrdex/rate-sources", strings.NewReader(`{"sources":{"dcrdata":false}}`))
		SaveDcrdexRateSourcesHandler(rec, req)
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != tc.want {
			t.Fatalf("apply %v: %d %s, want %s", tc.applyErr, rec.Code, rec.Body.String(), tc.want)
		}
		if !reflect.DeepEqual(rr.saved, []map[string]bool{{"dcrdata": false}}) || rr.applies != 1 {
			t.Fatalf("saved %v, applied %d times", rr.saved, rr.applies)
		}
	}
}

func TestSaveDcrdexRateSourcesRefusesWhatItCannotStore(t *testing.T) {
	for _, tc := range []struct {
		body    string
		saveErr error
		code    int
	}{
		{`{"sources":{}}`, nil, http.StatusBadRequest},
		{`{"sources":{"Messari":true}}`, fmt.Errorf("%w: %q", services.ErrUnknownDexRateSource, "Messari"), http.StatusBadRequest},
		{`{"sources":{"dcrdata":true}}`, errors.New("disk full"), http.StatusInternalServerError},
	} {
		rr := withRateRoutes(t, true, nil, tc.saveErr, nil)
		rec := httptest.NewRecorder()
		SaveDcrdexRateSourcesHandler(rec, httptest.NewRequest(http.MethodPost, "/api/dcrdex/rate-sources", strings.NewReader(tc.body)))
		if rec.Code != tc.code || rr.applies != 0 {
			t.Fatalf("%s: %d, applied %d times; want %d and no apply", tc.body, rec.Code, rr.applies, tc.code)
		}
	}
}
