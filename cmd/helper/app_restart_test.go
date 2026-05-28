package helper

import (
	"strings"
	"testing"
)

func TestPickRestartTargetsAllProcessesSorted(t *testing.T) {
	// Whole-app restart: every labelled process comes back, sorted by
	// process name so the rolling order is deterministic.
	processes := containersToProcesses([]containerEntry{
		{
			Names:  []string{"svps-a8f9b2-worker-abc1234"},
			State:  "running",
			Labels: map[string]string{"simple-vps.app": "api", "simple-vps.env": "production", "simple-vps.process": "worker"},
		},
		{
			Names:  []string{"svps-a8f9b2-web-abc1234"},
			State:  "running",
			Labels: map[string]string{"simple-vps.app": "api", "simple-vps.env": "production", "simple-vps.process": "web"},
		},
	})
	targets, err := pickRestartTargets("api", "production", "", processes)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(targets), targets)
	}
	if targets[0].Process != "web" || targets[1].Process != "worker" {
		t.Fatalf("expected sorted [web, worker], got [%s, %s]", targets[0].Process, targets[1].Process)
	}
}

func TestPickRestartTargetsSingleProcess(t *testing.T) {
	processes := []processStatus{
		{Process: "web", Container: "svps-a8f9b2-web-abc1234", State: "running"},
		{Process: "worker", Container: "svps-a8f9b2-worker-abc1234", State: "running"},
	}
	targets, err := pickRestartTargets("api", "production", "worker", processes)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Process != "worker" {
		t.Fatalf("expected [worker], got %+v", targets)
	}
}

func TestPickRestartTargetsUnknownProcess(t *testing.T) {
	processes := []processStatus{
		{Process: "web", Container: "svps-a8f9b2-web-abc1234", State: "running"},
	}
	if _, err := pickRestartTargets("api", "production", "worker", processes); err == nil {
		t.Fatal("expected error for unknown process, got nil")
	} else if !strings.Contains(err.Error(), `no process "worker"`) {
		t.Fatalf("expected unknown-process error, got %v", err)
	}
}

func TestPickRestartTargetsNothingRunning(t *testing.T) {
	if _, err := pickRestartTargets("api", "production", "", nil); err == nil {
		t.Fatal("expected error when nothing is running")
	} else if !strings.Contains(err.Error(), "no processes running") {
		t.Fatalf("expected no-processes-running error, got %v", err)
	}
}

func TestPostRestartStateMissingWhenContainerGone(t *testing.T) {
	entries := []containerEntry{
		{Names: []string{"svps-a8f9b2-web-abc1234"}, State: "running"},
	}
	if got := postRestartState(entries, "svps-a8f9b2-worker-abc1234"); got != "missing" {
		t.Fatalf("expected missing, got %s", got)
	}
	if got := postRestartState(entries, "svps-a8f9b2-web-abc1234"); got != "running" {
		t.Fatalf("expected running, got %s", got)
	}
}

func TestRenderRestartTextNonEmpty(t *testing.T) {
	results := []processStatus{
		{Process: "web", State: "running"},
		{Process: "worker", State: "running"},
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
