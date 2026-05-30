package client

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fprl/simple-vps/internal/config"
)

func deployRemotePreflight(runner sshRunner, ctx *config.AppContext) error {
	if _, err := runSSHRequired(runner, ctx.Server, "true", fmt.Sprintf("SSH failed for %s", ctx.Server)); err != nil {
		return err
	}
	if _, err := runSSHRequired(runner, ctx.Server, "command -v simple-vps >/dev/null", "missing Simple VPS server API; run `simple-vps host install`"); err != nil {
		return err
	}
	if _, err := runSSHRequired(runner, ctx.Server, "command -v rsync >/dev/null", "missing required server tool: rsync"); err != nil {
		return err
	}
	stdout, stderr, code, err := runner.RunSSH(ctx.Server, serverAppPreflightJSONCommand(ctx.AppName, ctx.EnvName, secretRefKeys(ctx.SecretRefs)))
	if err == nil && code == 0 {
		if report, ok := parseRemotePreflightReport(stdout); ok && !report.Healthy {
			return fmt.Errorf("deploy preflight failed:\n%s", renderRemotePreflightFindings(report))
		}
		return nil
	}
	if report, ok := parseRemotePreflightReport(stdout); ok {
		return fmt.Errorf("deploy preflight failed:\n%s", renderRemotePreflightFindings(report))
	}
	detail := strings.TrimSpace(stdout)
	if detail == "" {
		detail = strings.TrimSpace(stderr)
	}
	if detail == "" {
		detail = "no error detail"
	}
	return fmt.Errorf("deploy preflight failed:\n%s", detail)
}

type remotePreflightReport struct {
	App      string   `json:"app"`
	Env      string   `json:"env"`
	Healthy  bool     `json:"healthy"`
	Findings []string `json:"findings"`
}

func parseRemotePreflightReport(out string) (remotePreflightReport, bool) {
	var report remotePreflightReport
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &report); err != nil {
		return remotePreflightReport{}, false
	}
	if report.App == "" || report.Env == "" {
		return remotePreflightReport{}, false
	}
	return report, true
}

func renderRemotePreflightFindings(report remotePreflightReport) string {
	if len(report.Findings) == 0 {
		if report.Healthy {
			return fmt.Sprintf("preflight for %s (%s) returned no findings", report.App, report.Env)
		}
		return fmt.Sprintf("preflight for %s (%s) failed without findings", report.App, report.Env)
	}
	var lines []string
	for _, finding := range report.Findings {
		lines = append(lines, "  - "+finding)
	}
	return strings.Join(lines, "\n")
}

func secretRefKeys(refs map[string]string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, key := range refs {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
