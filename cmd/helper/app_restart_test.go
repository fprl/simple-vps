package helper

import (
	"strings"
	"testing"
)

func TestPickRestartTargetsAllServicesSorted(t *testing.T) {
	// Whole-app restart: every labelled service comes back, sorted by
	// service name so the rolling order is deterministic.
	services := containersToServices([]containerEntry{
		{
			Names: []string{"app-api-production-worker"},
			State: "running",
			Labels: map[string]string{"app": "api", "env": "production", "service": "worker"},
		},
		{
			Names: []string{"app-api-production-web"},
			State: "running",
			Labels: map[string]string{"app": "api", "env": "production", "service": "web"},
		},
	})
	targets, err := pickRestartTargets("api", "production", "", services)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(targets), targets)
	}
	if targets[0].Service != "web" || targets[1].Service != "worker" {
		t.Fatalf("expected sorted [web, worker], got [%s, %s]", targets[0].Service, targets[1].Service)
	}
}

func TestPickRestartTargetsSingleService(t *testing.T) {
	services := []serviceStatus{
		{Service: "web", Container: "app-api-production-web", State: "running"},
		{Service: "worker", Container: "app-api-production-worker", State: "running"},
	}
	targets, err := pickRestartTargets("api", "production", "worker", services)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Service != "worker" {
		t.Fatalf("expected [worker], got %+v", targets)
	}
}

func TestPickRestartTargetsUnknownService(t *testing.T) {
	services := []serviceStatus{
		{Service: "web", Container: "app-api-production-web", State: "running"},
	}
	if _, err := pickRestartTargets("api", "production", "worker", services); err == nil {
		t.Fatal("expected error for unknown service, got nil")
	} else if !strings.Contains(err.Error(), `no service "worker"`) {
		t.Fatalf("expected unknown-service error, got %v", err)
	}
}

func TestPickRestartTargetsNothingRunning(t *testing.T) {
	if _, err := pickRestartTargets("api", "production", "", nil); err == nil {
		t.Fatal("expected error when nothing is running")
	} else if !strings.Contains(err.Error(), "no services running") {
		t.Fatalf("expected no-services-running error, got %v", err)
	}
}

func TestPostRestartStateMissingWhenContainerGone(t *testing.T) {
	entries := []containerEntry{
		{Names: []string{"app-api-production-web"}, State: "running"},
	}
	if got := postRestartState(entries, "app-api-production-worker"); got != "missing" {
		t.Fatalf("expected missing, got %s", got)
	}
	if got := postRestartState(entries, "app-api-production-web"); got != "running" {
		t.Fatalf("expected running, got %s", got)
	}
}

func TestRenderRestartTextNonEmpty(t *testing.T) {
	results := []serviceStatus{
		{Service: "web", State: "running"},
		{Service: "worker", State: "running"},
	}
	out := renderRestartText("api", "production", results)
	if !strings.Contains(out, "api (production)") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "web") || !strings.Contains(out, "restarted (running)") {
		t.Fatalf("missing web row:\n%s", out)
	}
	if !strings.Contains(out, "worker") {
		t.Fatalf("missing worker row:\n%s", out)
	}
}
