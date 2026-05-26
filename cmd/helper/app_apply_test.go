package helper

import (
	"strings"
	"testing"

	"github.com/fprl/simple-vps/internal/config"
)

func TestRenderEnvFileEmitsSortedKeyValuePairs(t *testing.T) {
	vals := map[string]string{
		"LOG_LEVEL": "info",
		"DEBUG":     "false",
		"PORT":      "3000",
	}
	got := renderEnvFile(vals)
	want := "DEBUG=false\nLOG_LEVEL=info\nPORT=3000\n"
	if got != want {
		t.Fatalf("renderEnvFile mismatch:\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderEnvFileEmptyMapProducesEmptyString(t *testing.T) {
	if got := renderEnvFile(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := renderEnvFile(map[string]string{}); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestRenderAppCaddyfileProxyRoute(t *testing.T) {
	port := 3000
	ctx := &config.AppContext{
		Services: map[string]config.Service{
			"web": {Port: &port},
		},
		Routes: map[string]config.Route{
			"app": {
				Host:    "api.example.com",
				Type:    "proxy",
				Service: "web",
			},
		},
	}
	got, err := renderAppCaddyfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "api.example.com {") {
		t.Fatalf("expected host block, got:\n%s", got)
	}
	if !strings.Contains(got, "reverse_proxy 127.0.0.1:3000") {
		t.Fatalf("expected reverse_proxy directive, got:\n%s", got)
	}
}

func TestRenderAppCaddyfileRedirectRoute(t *testing.T) {
	ctx := &config.AppContext{
		Routes: map[string]config.Route{
			"r": {
				Host: "old.example.com",
				Type: "redirect",
				To:   "https://new.example.com",
			},
		},
	}
	got, err := renderAppCaddyfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "old.example.com {") {
		t.Fatalf("expected host block, got:\n%s", got)
	}
	if !strings.Contains(got, "redir https://new.example.com") {
		t.Fatalf("expected redir directive, got:\n%s", got)
	}
}

func TestRenderAppCaddyfileSkipsStaticRoutes(t *testing.T) {
	// Static apps land in a follow-up verb; the container deploy verb
	// shouldn't emit Caddy directives for them.
	ctx := &config.AppContext{
		Routes: map[string]config.Route{
			"site": {
				Host: "site.example.com",
				Type: "static",
			},
		},
	}
	got, err := renderAppCaddyfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "site.example.com") {
		t.Fatalf("static route should be skipped, got:\n%s", got)
	}
}

func TestRenderAppCaddyfileRejectsProxyWithoutServicePort(t *testing.T) {
	ctx := &config.AppContext{
		Services: map[string]config.Service{
			"worker": {}, // no port
		},
		Routes: map[string]config.Route{
			"r": {
				Host:    "x.example.com",
				Type:    "proxy",
				Service: "worker",
			},
		},
	}
	if _, err := renderAppCaddyfile(ctx); err == nil {
		t.Fatal("expected error for proxy route pointing at portless service")
	}
}

func TestRenderAppCaddyfileRejectsUnknownRouteType(t *testing.T) {
	ctx := &config.AppContext{
		Routes: map[string]config.Route{
			"r": {
				Host: "x.example.com",
				Type: "weirdo",
			},
		},
	}
	if _, err := renderAppCaddyfile(ctx); err == nil {
		t.Fatal("expected error for unknown route type")
	}
}

func TestValidateAppEnvAcceptsCanonicalNames(t *testing.T) {
	if err := validateAppEnv("api", "production"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateAppEnv("multi-word-app", "stage-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateAppEnvRejectsLeadingDigitOrPunctuation(t *testing.T) {
	for _, name := range []string{"1bad", "-bad", "bad name", "BAD"} {
		if err := validateAppEnv(name, "production"); err == nil {
			t.Fatalf("expected error for app=%q", name)
		}
		if err := validateAppEnv("good", name); err == nil {
			t.Fatalf("expected error for env=%q", name)
		}
	}
}
