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
