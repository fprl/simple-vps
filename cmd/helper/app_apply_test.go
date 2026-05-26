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
	got, err := renderAppCaddyfile(ctx, map[string]int{"web": 33000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "api.example.com {") {
		t.Fatalf("expected host block, got:\n%s", got)
	}
	// The reverse_proxy must point at the ALLOCATED host port, not the
	// in-container service port. Two apps both declaring `port = 3000`
	// would otherwise collide on the host loopback bind.
	if !strings.Contains(got, "reverse_proxy 127.0.0.1:33000") {
		t.Fatalf("expected reverse_proxy at allocated host port, got:\n%s", got)
	}
	if strings.Contains(got, "reverse_proxy 127.0.0.1:3000") {
		t.Fatalf("rendered Caddyfile points at the in-container port instead of the allocated host port:\n%s", got)
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
	got, err := renderAppCaddyfile(ctx, nil)
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
	got, err := renderAppCaddyfile(ctx, nil)
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
	if _, err := renderAppCaddyfile(ctx, nil); err == nil {
		t.Fatal("expected error for proxy route pointing at portless service")
	}
}

func TestRenderAppCaddyfileRejectsProxyWithoutAllocatedPort(t *testing.T) {
	// Defense in depth: even if a port-having service slips into the
	// route table, render must refuse when the allocator hasn't given
	// it a host port.
	port := 3000
	ctx := &config.AppContext{
		Services: map[string]config.Service{
			"web": {Port: &port},
		},
		Routes: map[string]config.Route{
			"r": {
				Host:    "x.example.com",
				Type:    "proxy",
				Service: "web",
			},
		},
	}
	if _, err := renderAppCaddyfile(ctx, map[string]int{}); err == nil {
		t.Fatal("expected error when allocator returned no host port")
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
	if _, err := renderAppCaddyfile(ctx, nil); err == nil {
		t.Fatal("expected error for unknown route type")
	}
}

func TestPickHostPortReturnsLowestFreePort(t *testing.T) {
	used := map[int]bool{33000: true, 33001: true, 33003: true}
	got, err := pickHostPort(used, 33000, 34000)
	if err != nil {
		t.Fatal(err)
	}
	if got != 33002 {
		t.Fatalf("expected 33002, got %d", got)
	}
}

func TestPickHostPortReturnsFirstWhenAllFree(t *testing.T) {
	got, err := pickHostPort(map[int]bool{}, 33000, 34000)
	if err != nil {
		t.Fatal(err)
	}
	if got != 33000 {
		t.Fatalf("expected 33000, got %d", got)
	}
}

func TestPickHostPortErrorsWhenRangeExhausted(t *testing.T) {
	used := map[int]bool{}
	for p := 33000; p < 33003; p++ {
		used[p] = true
	}
	if _, err := pickHostPort(used, 33000, 33003); err == nil {
		t.Fatal("expected error when range is exhausted")
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
