package helper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestImageReleasesFromEntriesUsesPodmanLabels(t *testing.T) {
	entries := []imageEntry{
		{
			Names: []string{"localhost/simple-vps/svps-de70a215abfd:new"},
			Labels: map[string]string{
				"simple-vps.app":      "hello",
				"simple-vps.env":      "production",
				"simple-vps.infra_id": "svps-de70a215abfd",
				"simple-vps.release":  "new",
			},
		},
		{
			Names: []string{"localhost/simple-vps/svps-de70a215abfd:old"},
			Labels: map[string]string{
				"simple-vps.app":      "hello",
				"simple-vps.env":      "production",
				"simple-vps.infra_id": "svps-de70a215abfd",
				"simple-vps.release":  "old",
			},
		},
		{
			Names: []string{"localhost/simple-vps/svps-other:ignored"},
			Labels: map[string]string{
				"simple-vps.app":      "hello",
				"simple-vps.env":      "production",
				"simple-vps.infra_id": "svps-other",
				"simple-vps.release":  "ignored",
			},
		},
	}

	got := imageReleasesFromEntries("hello", "production", entries)
	if len(got) != 2 || got[0].Release != "new" || got[1].Release != "old" {
		t.Fatalf("unexpected releases: %+v", got)
	}
}

func TestStaticReleasesAtOrdersNewestFirst(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	new := filepath.Join(root, "new")
	if err := os.Mkdir(old, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(new, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(new, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	got, err := staticReleasesAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Release != "new" || got[1].Release != "old" {
		t.Fatalf("unexpected static release order: %+v", got)
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
