package helper

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fprl/simple-vps/internal/config"
	"github.com/fprl/simple-vps/internal/identity"
	"github.com/fprl/simple-vps/internal/utils"
)

// appApplyCmd is the per-env, all-services deploy primitive. Given a
// streamed source tarball and a manifest, it:
//
//  1. Validates the manifest against the new shape rules.
//  2. Writes the resolved env file to <shared>/.env (literal values only
//     in this slice; @secret:KEY resolution against the per-(app, env,
//     key) store lands in a follow-up).
//  3. Extracts the tarball and runs `podman build` to tag a new image.
//  4. For each service: stops the running container, starts a fresh
//     one joined to the per-env network plus the shared `ingress`
//     network, waits for the healthcheck.
//  5. Synthesizes a Caddyfile fragment for the manifest's routes and
//     reloads Caddy.
//
// Atomicity in this slice is per-service (ADR-0006 Cut 1), but the
// blue/green rolling lifecycle is simplified to stop-old → start-new.
// Full -new/-old rename choreography lands in a follow-up.
type appApplyCmd struct {
	App      string `arg:"" help:"App name."`
	Env      string `arg:"" help:"Env name."`
	Tarball  string `name:"tarball" required:"" help:"Path to the streamed source tarball."`
	Manifest string `name:"manifest" required:"" help:"Path to the uploaded simple-vps.toml."`
	SHA      string `name:"sha" required:"" help:"Release identifier (short git SHA or dirty-<timestamp>)."`
}

func (c appApplyCmd) Run() error {
	if err := validateAppEnv(c.App, c.Env); err != nil {
		utils.Die(err.Error(), 1)
	}
	if err := requireFileUnder(c.Tarball, "/tmp/simple-vps-deploy"); err != nil {
		utils.Die(err.Error(), 1)
	}
	if err := requireFileUnder(c.Manifest, "/tmp/simple-vps-deploy"); err != nil {
		utils.Die(err.Error(), 1)
	}

	// Manifest sits in a temp dir created by the client; CheckManifest
	// reads the rest of the working tree it expects (Dockerfile) from
	// the SAME directory. So we extract the tarball alongside the
	// uploaded manifest into a context dir and run the validator there.
	ctxDir, err := os.MkdirTemp("/tmp/simple-vps-deploy", "ctx-")
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	defer os.RemoveAll(ctxDir)

	if _, err := utils.RunChecked("tar", []string{"-xf", c.Tarball, "-C", ctxDir}, ""); err != nil {
		utils.Die(fmt.Sprintf("extract tarball: %v", err), 1)
	}
	// The uploaded manifest is authoritative — overwrite any manifest
	// that might have been in the tarball.
	if _, err := utils.RunChecked("install", []string{"-m", "0644", c.Manifest, filepath.Join(ctxDir, "simple-vps.toml")}, ""); err != nil {
		utils.Die(fmt.Sprintf("install manifest: %v", err), 1)
	}

	checkErrors, _, err := config.CheckManifest(ctxDir, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	if len(checkErrors) > 0 {
		utils.Die(fmt.Sprintf("manifest invalid: %s", strings.Join(checkErrors, "; ")), 1)
	}
	app, err := config.LoadAppContext(ctxDir, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	if app.Shape != config.ShapeContainer {
		utils.Die(fmt.Sprintf("apply currently supports container apps only (got shape %q); static apps land in a follow-up", app.Shape), 1)
	}
	if len(app.Services) == 0 {
		utils.Die("manifest must declare at least one [services.<name>] block", 1)
	}

	// 2. Resolve env. Literal values only for now; @secret refs are
	// validated by CheckManifest above and will be resolved against the
	// per-(app, env, key) secret store in a follow-up.
	if err := writeEnvFile(c.App, c.Env, app.Env); err != nil {
		utils.Die(err.Error(), 1)
	}

	// 3. Build the image.
	imageTag := identity.ImageTag(c.App, c.Env, c.SHA)
	if _, err := utils.RunChecked("podman",
		[]string{"build",
			"-t", imageTag,
			"--label", "app=" + c.App,
			"--label", "env=" + c.Env,
			"--label", "simple_vps_release=" + c.SHA,
			"-f", filepath.Join(ctxDir, "Dockerfile"),
			ctxDir,
		}, "",
	); err != nil {
		utils.Die(fmt.Sprintf("podman build: %v", err), 1)
	}

	// 4. Start each service.
	for _, svcName := range sortedKeys(app.Services) {
		svc := app.Services[svcName]
		if err := startService(c.App, c.Env, svcName, svc, imageTag); err != nil {
			utils.Die(err.Error(), 1)
		}
	}

	// 5. Render Caddyfile and reload.
	if err := writeAppCaddyfile(c.App, c.Env, app); err != nil {
		utils.Die(err.Error(), 1)
	}
	if _, err := utils.RunChecked("caddy", []string{"reload", "--config", caddyfilePath(c.App, c.Env)}, ""); err != nil {
		// Best-effort: also try systemctl reload caddy (apt-packaged Caddy).
		if _, fallbackErr := utils.RunChecked("systemctl", []string{"reload", "caddy"}, ""); fallbackErr != nil {
			utils.Die(fmt.Sprintf("caddy reload: %v (fallback: %v)", err, fallbackErr), 1)
		}
	}

	fmt.Printf("Deployed %s (%s) at %s\n", c.App, c.Env, c.SHA)
	return nil
}

func requireFileUnder(path, base string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cleanBase, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(cleanBase, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%q must live under %q", path, base)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("%q: %v", path, err)
	}
	return nil
}

func renderEnvFile(vals map[string]string) string {
	var lines []string
	for _, k := range sortedKeys(vals) {
		lines = append(lines, fmt.Sprintf("%s=%s", k, vals[k]))
	}
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return content
}

func writeEnvFile(app, env string, vals map[string]string) error {
	path := identity.EnvFile(app, env)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(renderEnvFile(vals)), 0600); err != nil {
		return err
	}
	user := identity.SystemUser(app, env)
	if _, err := utils.RunChecked("chown", []string{user + ":" + user, path}, ""); err != nil {
		return fmt.Errorf("chown env file: %v", err)
	}
	return nil
}

func startService(app, env, svcName string, svc config.Service, imageTag string) error {
	containerName := identity.ContainerName(app, env, svcName)
	user := identity.SystemUser(app, env)
	envFile := identity.EnvFile(app, env)
	shared := identity.SharedDir(app, env)
	appNet := identity.Network(app, env)

	// Stop and remove any existing container of the same name.
	_, _ = utils.RunChecked("podman", []string{"rm", "-f", containerName}, "")

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--restart", "unless-stopped",
		"--user", user,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--network", appNet,
		"--network", "ingress",
		"-v", shared + ":" + shared + ":Z",
		"--label", "app=" + app,
		"--label", "env=" + env,
		"--label", "service=" + svcName,
	}
	if _, err := os.Stat(envFile); err == nil {
		args = append(args, "--env-file", envFile)
	}
	if svc.Port != nil {
		// host-loopback publish so the host Caddy can reverse-proxy
		// without joining the Podman network plane (ADR-0005 §16; Caddy-
		// in-container lands in a follow-up).
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1:%d:%d", *svc.Port, *svc.Port))
	}
	args = append(args, imageTag)
	if svc.Command != "" {
		// Override the image CMD via /bin/sh -c so users can write the
		// command as a single string (ADR-0005 §13).
		args = append(args, "/bin/sh", "-c", svc.Command)
	}

	if _, err := utils.RunChecked("podman", args, ""); err != nil {
		return fmt.Errorf("podman run %s: %v", containerName, err)
	}

	if svc.Port != nil && svc.Healthcheck != "" {
		if err := waitHealthy(*svc.Port, svc.Healthcheck, healthcheckTimeout(svc)); err != nil {
			// Surface logs on failure so the user can see why.
			out, _ := exec.Command("podman", "logs", "--tail", "50", containerName).CombinedOutput()
			os.Stderr.Write(out)
			return fmt.Errorf("healthcheck failed for %s: %v", svcName, err)
		}
	}
	return nil
}

func healthcheckTimeout(svc config.Service) time.Duration {
	if svc.HealthcheckTimeout != nil && *svc.HealthcheckTimeout > 0 {
		return time.Duration(*svc.HealthcheckTimeout) * time.Second
	}
	return 30 * time.Second
}

func waitHealthy(port int, path string, timeout time.Duration) error {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out after %s", timeout)
	}
	return lastErr
}

func caddyfilePath(app, env string) string {
	return fmt.Sprintf("/etc/caddy/conf.d/simple-vps-%s-%s.caddy", app, env)
}

func renderAppCaddyfile(ctx *config.AppContext) (string, error) {
	var blocks []string
	for _, name := range sortedKeys(ctx.Routes) {
		route := ctx.Routes[name]
		var body string
		switch route.Type {
		case "proxy":
			svc, ok := ctx.Services[route.Service]
			if !ok || svc.Port == nil {
				return "", fmt.Errorf("route %q references service %q with no port", name, route.Service)
			}
			body = fmt.Sprintf("\treverse_proxy 127.0.0.1:%d\n", *svc.Port)
		case "redirect":
			body = fmt.Sprintf("\tredir %s\n", route.To)
		case "static":
			// Static apps not in scope for this verb; skip.
			continue
		default:
			return "", fmt.Errorf("route %q: unsupported type %q", name, route.Type)
		}
		blocks = append(blocks, fmt.Sprintf("%s {\n%s}\n", route.Host, body))
	}
	return "# generated by simple-vps server app apply — do not edit\n" + strings.Join(blocks, "\n"), nil
}

func writeAppCaddyfile(app, env string, ctx *config.AppContext) error {
	content, err := renderAppCaddyfile(ctx)
	if err != nil {
		return err
	}
	path := caddyfilePath(app, env)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
