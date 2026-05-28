// Package caddy holds Caddyfile helpers used when rendering generated
// per-env fragments under /etc/caddy/conf.d/.
package caddy

import (
	"encoding/json"
	"errors"
	"strings"
)

// CaddyQuote produces a JSON-quoted string safe to use as a value
// inside a Caddyfile directive (block selector, redir target, etc.).
// Rejects values containing carriage returns or newlines so a hostile
// value can't inject extra directives.
func CaddyQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("Caddyfile values cannot contain newlines")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
