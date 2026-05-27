// Package caddy holds the single Caddyfile-quoting helper that the
// helper-side `app apply` uses when rendering per-app fragments. The
// pre-cutover render path (routes.json → simple-vps/routes.caddy →
// `caddy validate` against a host binary) is gone; per-app routes now
// live in /etc/caddy/conf.d/<app>-<env>.caddy, written directly by
// `server app apply` and validated inside the Caddy container.
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
