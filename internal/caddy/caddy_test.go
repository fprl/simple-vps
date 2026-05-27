package caddy

import "testing"

func TestCaddyQuoteJSONEscapesAndPreservesValue(t *testing.T) {
	got, err := CaddyQuote("api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"api.example.com"` {
		t.Fatalf("expected JSON-quoted, got %q", got)
	}
}

func TestCaddyQuoteEscapesInternalQuotes(t *testing.T) {
	got, err := CaddyQuote(`he said "hi"`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `"he said \"hi\""` {
		t.Fatalf("unexpected escape: %q", got)
	}
}

func TestCaddyQuoteRejectsNewlines(t *testing.T) {
	for _, bad := range []string{"line1\nline2", "carriage\rreturn"} {
		if _, err := CaddyQuote(bad); err == nil {
			t.Fatalf("expected newline rejection for %q", bad)
		}
	}
}
