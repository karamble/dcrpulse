package services

import (
	"encoding/json"
	"testing"
)

type signingMetadata string

func (m signingMetadata) Get(_ string, out any) (bool, error) {
	if m == "" {
		return false, nil
	}
	return true, json.Unmarshal([]byte(m), out)
}

func TestGamingSigningMetadataFailsClosed(t *testing.T) {
	for _, input := range []string{"", "null", "true", "\"false\"", "{", "0"} {
		t.Run(input, func(t *testing.T) {
			if err := requireGamingSigningMetadata(signingMetadata(input)); err == nil {
				t.Fatal("unproven signing wallet accepted")
			}
		})
	}
	if err := requireGamingSigningMetadata(signingMetadata("false")); err != nil {
		t.Fatal(err)
	}
}

// The mixer, Lightning, DEX and imported accounts are bound to by name, in any
// case: a game bound to one would spend funds another daemon relies on.
func TestTheDashboardReservesItsDaemonAccountsFromGames(t *testing.T) {
	w := GamingHost(nil).Wallet
	for _, name := range []string{"lightning", "dex", "mixed", "unmixed", "imported", "  Lightning "} {
		if !w.ReservedAccount(name) {
			t.Errorf("%q is not reserved", name)
		}
	}
	if w.ReservedAccount("games") {
		t.Error("an ordinary account is reserved")
	}
}
