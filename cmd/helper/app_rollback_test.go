package helper

import (
	"strings"
	"testing"
)

func TestCurrentReleaseRejectsEmptyOrMixedProcesses(t *testing.T) {
	if _, err := currentRelease(nil); err == nil || !strings.Contains(err.Error(), "no processes running") {
		t.Fatalf("expected empty-processes error, got %v", err)
	}
	_, err := currentRelease([]processStatus{
		{Process: "web", Release: "aaa"},
		{Process: "worker", Release: "bbb"},
	})
	if err == nil || !strings.Contains(err.Error(), "different releases") {
		t.Fatalf("expected mixed-release error, got %v", err)
	}
}

func TestSelectRollbackRelease(t *testing.T) {
	images := []imageRelease{
		{Release: "new"},
		{Release: "old"},
		{Release: "older"},
	}
	got, err := selectRollbackRelease(images, "new", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != "old" {
		t.Fatalf("expected previous release old, got %+v", got)
	}

	got, err = selectRollbackRelease(images, "new", "older")
	if err != nil {
		t.Fatal(err)
	}
	if got.Release != "older" {
		t.Fatalf("expected requested release older, got %+v", got)
	}
}

func TestSelectRollbackReleaseErrors(t *testing.T) {
	_, err := selectRollbackRelease([]imageRelease{{Release: "new"}}, "new", "")
	if err == nil || !strings.Contains(err.Error(), "no previous release") {
		t.Fatalf("expected no previous release error, got %v", err)
	}
	_, err = selectRollbackRelease([]imageRelease{{Release: "new"}}, "new", "missing")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("expected missing release error, got %v", err)
	}
	_, err = selectRollbackRelease([]imageRelease{{Release: "new"}}, "new", "new")
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("expected already running error, got %v", err)
	}
}

func TestRenderRollbackText(t *testing.T) {
	out := renderRollbackText(rollbackPayload{
		App:       "api",
		Env:       "production",
		Previous:  "new",
		Release:   "old",
		Processes: []string{"web"},
	})
	if !strings.Contains(out, "Rolled back api (production) from new to old") {
		t.Fatalf("missing rollback summary:\n%s", out)
	}
	if !strings.Contains(out, "web") || !strings.Contains(out, "running") {
		t.Fatalf("missing process row:\n%s", out)
	}
}
