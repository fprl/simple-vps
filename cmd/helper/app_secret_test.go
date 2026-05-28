package helper

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSecretListPayloadKeepsEmptyKeysAsArray(t *testing.T) {
	payload := secretListPayloadFor("api", "production", nil)
	if payload.App != "api" || payload.Env != "production" {
		t.Fatalf("unexpected identity: %+v", payload)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"keys":[]`) {
		t.Fatalf("empty keys should encode as [], got: %s", raw)
	}
}
