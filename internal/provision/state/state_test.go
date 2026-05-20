package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreWritesADR0002Files(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}

	desired := validHostDesired()
	desired.Ingress.Tunnel = TunnelCloudflare
	desired.Features.Litestream = true
	desired.Features.Runtimes = []string{"bun"}
	desired.Packages = map[string]DesiredPackage{
		"litestream": {Source: "github-release", Version: "0.5.8"},
		"caddy":      {Source: "caddy-apt", Track: "stable"},
	}
	observed := HostObserved{
		Packages: map[string]ObservedPackage{
			"caddy": {Version: "2.8.4"},
		},
	}

	if err := store.WriteHostDesired(desired); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteHostState(observed, HostMeta{}); err != nil {
		t.Fatal(err)
	}
	if got := store.HostPath(); got != filepath.Join(root, "host.json") {
		t.Fatalf("unexpected host path: %s", got)
	}

	data, err := os.ReadFile(store.HostPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`"version": 1`,
		`"desired": {`,
		`"observed": {`,
		`"meta": {`,
		`"expose": "private"`,
		`"tunnel": "cloudflare"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected host.json to contain %q:\n%s", want, text)
		}
	}
	packagesBlock := text[strings.Index(text, `"packages": {`):]
	if strings.Index(packagesBlock, `"caddy"`) > strings.Index(packagesBlock, `"litestream"`) {
		t.Fatalf("package keys are not stable-sorted:\n%s", text)
	}
	assertMode(t, store.HostPath(), 0644)

	loaded, err := store.ReadHost()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 || loaded.Desired.Users.Operator != "operator" {
		t.Fatalf("unexpected loaded host file: %+v", loaded)
	}

	if err := store.WriteCloudflare(CloudflareFile{
		Version:    CurrentVersion,
		AccountID:  "account-test",
		TunnelID:   "tunnel-test",
		TunnelName: "simple-vps-prod-1",
		DNSRecords: map[string]string{
			"api.example.com": "record-test",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.CloudflarePath(); got != filepath.Join(root, "providers", "cloudflare.json") {
		t.Fatalf("unexpected cloudflare path: %s", got)
	}
	assertMode(t, store.CloudflarePath(), 0600)

	if got := store.SecretsDir(); got != filepath.Join(root, "secrets") {
		t.Fatalf("unexpected secrets dir: %s", got)
	}
}

func TestWriteHostStatePreservesDesired(t *testing.T) {
	store := Store{Root: t.TempDir()}
	raw := `{
  "version": 1,
  "desired": {
      "users": {"operator":"operator", "deploy":"deploy"},
      "ingress": {"tunnel":"none", "expose":"private"},
      "features": {"runtimes":["node", "bun"], "litestream":true, "docker":false},
      "packages": {
        "litestream": {"version":"0.5.8", "source":"github-release"},
        "caddy": {"track":"stable", "source":"caddy-apt"}
      }
  },
  "observed": {
    "packages": {},
    "ingress": {}
  },
  "meta": {}
}`
	if err := os.WriteFile(store.HostPath(), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	before := hostDesiredRaw(t, store.HostPath())

	if err := store.WriteHostState(HostObserved{
		Packages: map[string]ObservedPackage{
			"caddy": {Version: "2.8.4"},
		},
		Ingress: HostIngressObserved{
			CloudflaredServiceActive: true,
		},
	}, HostMeta{
		SimpleVPSVersion: "0.3.0",
	}); err != nil {
		t.Fatal(err)
	}
	after := hostDesiredRaw(t, store.HostPath())

	if before != after {
		t.Fatalf("WriteHostState mutated desired:\nbefore: %s\nafter:  %s", before, after)
	}

	loaded, err := store.ReadHost()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Observed.Packages["caddy"].Version != "2.8.4" {
		t.Fatalf("observed package version was not written: %+v", loaded.Observed.Packages)
	}
	if loaded.Meta.SimpleVPSVersion != "0.3.0" {
		t.Fatalf("meta was not written: %+v", loaded.Meta)
	}
}

func TestStoreSortsSliceFieldsBeforeWrite(t *testing.T) {
	storeA := Store{Root: t.TempDir()}
	storeB := Store{Root: t.TempDir()}

	desiredA := validHostDesired()
	desiredA.Features.Runtimes = []string{"node", "bun"}
	desiredB := validHostDesired()
	desiredB.Features.Runtimes = []string{"bun", "node"}
	if err := storeA.WriteHostDesired(desiredA); err != nil {
		t.Fatal(err)
	}
	if err := storeB.WriteHostDesired(desiredB); err != nil {
		t.Fatal(err)
	}
	assertSameFile(t, storeA.HostPath(), storeB.HostPath())

	appsA := AppsFile{Version: CurrentVersion, Apps: map[string]App{"api": {Services: []string{"worker", "web"}}}}
	appsB := AppsFile{Version: CurrentVersion, Apps: map[string]App{"api": {Services: []string{"web", "worker"}}}}
	if err := storeA.WriteApps(appsA); err != nil {
		t.Fatal(err)
	}
	if err := storeB.WriteApps(appsB); err != nil {
		t.Fatal(err)
	}
	assertSameFile(t, storeA.AppsPath(), storeB.AppsPath())

	routesA := RoutesFile{Version: CurrentVersion, Routes: []Route{
		{Host: "z.example.com", Type: "proxy", App: "api", Service: "web", Port: 3000},
		{Host: "a.example.com", Type: "proxy", App: "api", Service: "web", Port: 3000},
	}}
	routesB := RoutesFile{Version: CurrentVersion, Routes: []Route{
		{Host: "a.example.com", Type: "proxy", App: "api", Service: "web", Port: 3000},
		{Host: "z.example.com", Type: "proxy", App: "api", Service: "web", Port: 3000},
	}}
	if err := storeA.WriteRoutes(routesA); err != nil {
		t.Fatal(err)
	}
	if err := storeB.WriteRoutes(routesB); err != nil {
		t.Fatal(err)
	}
	assertSameFile(t, storeA.RoutesPath(), storeB.RoutesPath())
}

func TestStoreRejectsInvalidHostVersions(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing",
			raw:  `{"version":0}`,
			want: "host.json version is required",
		},
		{
			name: "future",
			raw:  `{"version":2}`,
			want: "unsupported host.json version 2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := Store{Root: t.TempDir()}
			writeRawFile(t, store.HostPath(), tc.raw)

			_, err := store.ReadHost()
			if err == nil {
				t.Fatal("expected invalid host schema version to fail")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestStoreTracksHostInstalledByHostFilePresence(t *testing.T) {
	store := Store{Root: t.TempDir()}

	installed, err := store.HostInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("expected fresh store to report host not installed")
	}

	if err := store.WriteHostDesired(validHostDesired()); err != nil {
		t.Fatal(err)
	}
	installed, err = store.HostInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("expected host file to report installed")
	}
}

func TestStoreValidatesVersionsAcrossStateFiles(t *testing.T) {
	store := Store{Root: t.TempDir()}

	for _, tc := range []struct {
		name        string
		path        string
		read        func() error
		writeZero   func() error
		zeroRaw     string
		futureRaw   string
		required    string
		unsupported string
	}{
		{
			name:        "apps",
			path:        store.AppsPath(),
			read:        func() error { _, err := store.ReadApps(); return err },
			writeZero:   func() error { return store.WriteApps(AppsFile{}) },
			zeroRaw:     `{"version":0,"apps":{}}`,
			futureRaw:   `{"version":2,"apps":{}}`,
			required:    "apps.json version is required",
			unsupported: "unsupported apps.json version 2",
		},
		{
			name:        "routes",
			path:        store.RoutesPath(),
			read:        func() error { _, err := store.ReadRoutes(); return err },
			writeZero:   func() error { return store.WriteRoutes(RoutesFile{}) },
			zeroRaw:     `{"version":0,"routes":[]}`,
			futureRaw:   `{"version":2,"routes":[]}`,
			required:    "routes.json version is required",
			unsupported: "unsupported routes.json version 2",
		},
		{
			name:        "cloudflare",
			path:        store.CloudflarePath(),
			read:        func() error { _, err := store.ReadCloudflare(); return err },
			writeZero:   func() error { return store.WriteCloudflare(CloudflareFile{}) },
			zeroRaw:     `{"version":0,"dns_records":{}}`,
			futureRaw:   `{"version":2,"dns_records":{}}`,
			required:    "providers/cloudflare.json version is required",
			unsupported: "unsupported providers/cloudflare.json version 2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.writeZero(); err == nil || !strings.Contains(err.Error(), tc.required) {
				t.Fatalf("expected write error %q, got %v", tc.required, err)
			}
			writeRawFile(t, tc.path, tc.zeroRaw)
			if err := tc.read(); err == nil || !strings.Contains(err.Error(), tc.required) {
				t.Fatalf("expected read error %q, got %v", tc.required, err)
			}
			writeRawFile(t, tc.path, tc.futureRaw)
			if err := tc.read(); err == nil || !strings.Contains(err.Error(), tc.unsupported) {
				t.Fatalf("expected read error %q, got %v", tc.unsupported, err)
			}
		})
	}
}

func TestWriteHostStateRequiresExistingHostDesired(t *testing.T) {
	store := Store{Root: t.TempDir()}

	err := store.WriteHostState(HostObserved{}, HostMeta{})
	if err == nil {
		t.Fatal("expected missing host.json to fail")
	}
	if !strings.Contains(err.Error(), "host.json is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadHostRejectsInvalidDesiredValues(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*HostDesired)
		want   string
	}{
		{
			name:   "missing operator",
			mutate: func(d *HostDesired) { d.Users.Operator = "" },
			want:   "users.operator",
		},
		{
			name:   "missing deploy",
			mutate: func(d *HostDesired) { d.Users.Deploy = "" },
			want:   "users.deploy",
		},
		{
			name:   "invalid expose",
			mutate: func(d *HostDesired) { d.Ingress.Expose = "" },
			want:   "ingress.expose",
		},
		{
			name:   "invalid tunnel",
			mutate: func(d *HostDesired) { d.Ingress.Tunnel = "" },
			want:   "ingress.tunnel",
		},
		{
			name:   "empty runtime",
			mutate: func(d *HostDesired) { d.Features.Runtimes = []string{"bun", ""} },
			want:   "features.runtimes",
		},
		{
			name: "missing package source",
			mutate: func(d *HostDesired) {
				d.Packages["caddy"] = DesiredPackage{Track: "stable"}
			},
			want: "packages.caddy.source",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := Store{Root: t.TempDir()}
			desired := validHostDesired()
			tc.mutate(&desired)
			writeHostWithDesired(t, store, desired)

			_, err := store.ReadHost()
			if err == nil {
				t.Fatal("expected invalid desired to fail")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func validHostDesired() HostDesired {
	return HostDesired{
		Users: HostUsers{
			Operator: "operator",
			Deploy:   "deploy",
		},
		Ingress: HostIngressDesired{
			Expose: ExposePrivate,
			Tunnel: TunnelNone,
		},
		Features: HostFeatures{
			Runtimes: []string{},
		},
		Packages: map[string]DesiredPackage{},
	}
}

func hostDesiredRaw(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Desired json.RawMessage `json:"desired"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return string(raw.Desired)
}

func assertSameFile(t *testing.T, left string, right string) {
	t.Helper()
	leftData, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftData) != string(rightData) {
		t.Fatalf("files differ:\n%s:\n%s\n%s:\n%s", left, leftData, right, rightData)
	}
}

func writeHostWithDesired(t *testing.T, store Store, desired HostDesired) {
	t.Helper()
	data, err := json.MarshalIndent(HostFile{
		Version:  CurrentVersion,
		Desired:  desired,
		Observed: HostObserved{Packages: map[string]ObservedPackage{}},
		Meta:     HostMeta{},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeRawFile(t, store.HostPath(), string(data))
}

func writeRawFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("expected %s mode %o, got %o", path, want, got)
	}
}
