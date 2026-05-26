package helper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fprl/simple-vps/internal/store"
)

func TestStatusStateLinesReportsNotInstalledWithoutRawOpenError(t *testing.T) {
	stateStore := store.Store{Root: t.TempDir()}

	lines, err := statusStateLines(stateStore)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "state: not installed") {
		t.Fatalf("expected not installed status, got:\n%s", text)
	}
	if strings.Contains(text, "open ") {
		t.Fatalf("status leaked raw open error:\n%s", text)
	}
}

func TestDoctorStateFindingsReportMissingHostWithoutRawError(t *testing.T) {
	root := t.TempDir()
	stateStore := store.Store{Root: root}

	findings := doctorStateFindings(stateStore)
	if len(findings) != 1 || !strings.Contains(findings[0], "host is not installed") {
		t.Fatalf("unexpected missing host findings: %+v", findings)
	}
	if strings.Contains(findings[0], "open ") {
		t.Fatalf("doctor leaked raw open error: %s", findings[0])
	}
}

func TestDoctorStateFindingsClearsAfterValidHost(t *testing.T) {
	// apps.json / routes.json are no longer probed by doctor (the
	// container deploy flow doesn't write them). A host with valid
	// host.json should produce no doctor findings, even when the
	// legacy state files are absent.
	root := t.TempDir()
	stateStore := store.Store{Root: root}
	writeValidHost(t, stateStore.HostPath())

	findings := doctorStateFindings(stateStore)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for a valid host with absent legacy files, got: %+v", findings)
	}
}

func writeValidHost(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	raw := `{
  "version": 1,
  "desired": {
    "users": {"operator": "operator", "deploy": "deploy"},
    "ingress": {"expose": "private", "tunnel": "none"},
    "features": {"docker": false, "litestream": false, "runtimes": []},
    "packages": {}
  },
  "observed": {"packages": {}, "ingress": {}},
  "meta": {}
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
}
