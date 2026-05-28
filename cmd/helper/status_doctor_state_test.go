package helper

import (
	"encoding/json"
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
	root := t.TempDir()
	stateStore := store.Store{Root: root}
	writeValidHost(t, stateStore.HostPath())

	findings := doctorStateFindings(stateStore)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for a valid host, got: %+v", findings)
	}
}

func TestHostStatusReportUsesInjectedChecks(t *testing.T) {
	stateStore := store.Store{Root: t.TempDir()}
	writeValidHost(t, stateStore.HostPath())

	report, err := hostStatusReportFor(
		stateStore,
		func(service string) string { return "service:" + service },
		func(tool string) string { return "tool:" + tool },
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.State.Installed || report.State.Status != "installed" {
		t.Fatalf("unexpected state: %+v", report.State)
	}
	if report.Services["caddy"] != "service:caddy" {
		t.Fatalf("unexpected services: %+v", report.Services)
	}
	if report.Tools["podman"] != "tool:podman" {
		t.Fatalf("unexpected tools: %+v", report.Tools)
	}
}

func TestDoctorReportJSONShape(t *testing.T) {
	report := doctorReportFor([]string{"host is not installed"}, nil)
	if report.Healthy {
		t.Fatal("expected degraded report")
	}
	if report.State.Status != "degraded" || report.Identity.Status != "healthy" {
		t.Fatalf("unexpected statuses: %+v", report)
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"findings":[]`) {
		t.Fatalf("empty findings should encode as [], got: %s", raw)
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
    "features": {"docker": false, "litestream": false},
    "packages": {}
  },
  "observed": {"packages": {}, "ingress": {}},
  "meta": {}
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
}
