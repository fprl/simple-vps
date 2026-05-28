package helper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fprl/simple-vps/internal/caddy"
	"github.com/fprl/simple-vps/internal/config"
	"github.com/fprl/simple-vps/internal/host"
	"github.com/fprl/simple-vps/internal/identity"
	"github.com/fprl/simple-vps/internal/secrets"
	"github.com/fprl/simple-vps/internal/utils"
)

// appApplyCmd is the per-env deploy primitive. Given a
// streamed source tarball and a manifest, it:
//
//  1. Validates the manifest.
//  2. Resolves vars/secrets into runtime/.env for container apps.
//  3. Builds the image or snapshots static assets.
//  4. Starts new versioned process containers and verifies web health.
//  5. Synthesizes a Caddyfile fragment, validates, reloads, and only
//     then removes old routed containers.
type appApplyCmd struct {
	App      string `arg:"" help:"App name."`
	Env      string `arg:"" help:"Env name."`
	Tarball  string `name:"tarball" required:"" help:"Path to the streamed source tarball."`
	Manifest string `name:"manifest" required:"" help:"Path to the uploaded simple-vps.toml."`
	SHA      string `name:"sha" required:"" help:"Release identifier (short git SHA or dirty-<timestamp>)."`
	Rebuild  bool   `name:"rebuild" help:"Pass --no-cache --pull=always to podman build."`
}

func (c appApplyCmd) Run() error {
	if err := validateAppEnv(c.App, c.Env); err != nil {
		utils.Die(err.Error(), 1)
	}
	withAppEnvLock(c.App, c.Env, func() {
		c.runLocked()
	})
	return nil
}

func (c appApplyCmd) runLocked() {
	// host.ValidateDeployTmpSource resolves symlinks, ensures the
	// path is a regular file under the deploy tmp root, and (if invoked
	// via sudo) verifies the file is owned by the deploying user — so a
	// malicious local user can't leave a file behind for the helper to
	// pick up.
	tarball, err := host.ValidateDeployTmpSource(c.Tarball)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	manifestPath, err := host.ValidateDeployTmpSource(c.Manifest)
	if err != nil {
		utils.Die(err.Error(), 1)
	}

	// Manifest sits in a temp dir created by the client; CheckManifest
	// reads the rest of the working tree it expects (Dockerfile) from
	// the SAME directory. So we extract the tarball alongside the
	// uploaded manifest into a context dir and run the validator there.
	ctxDir, err := os.MkdirTemp(host.DeployTmpDir(), "ctx-")
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	defer os.RemoveAll(ctxDir)

	if _, err := utils.RunChecked("tar", []string{"-xf", tarball, "-C", ctxDir}, ""); err != nil {
		utils.Die(fmt.Sprintf("extract tarball: %v", err), 1)
	}
	// The uploaded manifest is authoritative — overwrite any manifest
	// that might have been in the tarball.
	if _, err := utils.RunChecked("install", []string{"-m", "0644", manifestPath, filepath.Join(ctxDir, "simple-vps.toml")}, ""); err != nil {
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
	if err := writeEnvIdentity(c.App, c.Env); err != nil {
		utils.Die(err.Error(), 1)
	}

	var oldWebContainers []string
	var newWebContainers []string
	var staticSnapshot *staticCurrentSnapshot
	switch app.Shape {
	case config.ShapeContainer:
		oldWebContainers, newWebContainers = c.applyContainer(ctxDir, app)
	case config.ShapeStatic:
		snapshot, err := snapshotStaticCurrent(c.App, c.Env)
		if err != nil {
			utils.Die(fmt.Sprintf("snapshot static current: %v", err), 1)
		}
		staticSnapshot = &snapshot
		if err := c.applyStatic(ctxDir, app); err != nil {
			utils.Die(err.Error(), 1)
		}
	default:
		utils.Die(fmt.Sprintf("unsupported app shape %q", app.Shape), 1)
	}

	// 6. Write the per-app Caddyfile fragment (`reverse_proxy
	// http://<container>:<process-port>`), validate the full Caddyfile
	// inside the Caddy container, then reload Caddy in place. The
	// fragment lives under `/etc/caddy/conf.d/` which the main Caddyfile
	// imports; we never `caddy reload --config <fragment>` because that
	// would *replace* the active config with just this app.
	//
	// Snapshot the previous fragment first: if validate rejects the new
	// one we restore the old. A previously-healthy app would otherwise
	// lose its route on the next reload from anywhere.
	caddyPath := caddyfilePath(c.App, c.Env)
	prevFragment, prevExisted, err := snapshotCaddyFragment(caddyPath)
	if err != nil {
		utils.Die(fmt.Sprintf("snapshot existing fragment: %v", err), 1)
	}
	if err := writeAppCaddyfile(c.App, c.Env, app, c.SHA); err != nil {
		if staticSnapshot != nil {
			_ = restoreStaticCurrent(c.App, c.Env, *staticSnapshot)
		}
		utils.Die(err.Error(), 1)
	}
	if _, err := utils.RunChecked("podman", []string{"exec", "caddy", "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"}, ""); err != nil {
		if restoreErr := restoreCaddyFragment(caddyPath, prevFragment, prevExisted); restoreErr != nil {
			utils.Die(fmt.Sprintf("caddy validate rejected the fragment AND restore failed (manual fix required at %s): %v (restore: %v)", caddyPath, err, restoreErr), 1)
		}
		if staticSnapshot != nil {
			_ = restoreStaticCurrent(c.App, c.Env, *staticSnapshot)
		}
		utils.Die(fmt.Sprintf("caddy validate rejected the fragment, restored previous: %v", err), 1)
	}
	if _, err := utils.RunChecked("podman", []string{"exec", "caddy", "caddy", "reload", "--config", "/etc/caddy/Caddyfile"}, ""); err != nil {
		_ = restoreCaddyFragment(caddyPath, prevFragment, prevExisted)
		if staticSnapshot != nil {
			_ = restoreStaticCurrent(c.App, c.Env, *staticSnapshot)
		}
		utils.Die(fmt.Sprintf("caddy reload: %v", err), 1)
	}
	if err := persistAppliedManifest(c.App, c.Env, filepath.Join(ctxDir, "simple-vps.toml")); err != nil {
		utils.Die(err.Error(), 1)
	}
	for _, name := range oldWebContainers {
		_, _ = utils.RunChecked("podman", []string{"rm", "-f", name}, "")
	}
	_ = newWebContainers

	fmt.Printf("Deployed %s (%s) at %s\n", c.App, c.Env, c.SHA)
}

func persistAppliedManifest(app, env, manifestPath string) error {
	dst := identity.ManifestFile(app, env)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("mkdir applied manifest dir: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read applied manifest: %v", err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return fmt.Errorf("write applied manifest: %v", err)
	}
	return nil
}

func (c appApplyCmd) applyContainer(ctxDir string, app *config.AppContext) ([]string, []string) {
	if len(app.Processes) == 0 {
		utils.Die("manifest must declare at least one [processes.<name>] block", 1)
	}
	resolved, err := resolveEnv(c.App, c.Env, app.Vars, app.SecretRefs)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	if err := writeEnvFile(c.App, c.Env, resolved); err != nil {
		utils.Die(err.Error(), 1)
	}

	userID, groupID, err := hostUserIDs(identity.SystemUser(c.App, c.Env))
	if err != nil {
		utils.Die(err.Error(), 1)
	}

	imageTag := identity.ImageTag(c.App, c.Env, c.SHA)
	buildArgs := podmanBuildArgs(c.App, c.Env, imageTag, c.SHA, filepath.Join(ctxDir, "Dockerfile"), ctxDir, c.Rebuild)
	if _, err := utils.RunChecked("podman", buildArgs, ""); err != nil {
		utils.Die(fmt.Sprintf("podman build: %v", err), 1)
	}

	if app.Deploy.Release != "" {
		if err := runReleaseCommand(c.App, c.Env, app.Deploy.Release, imageTag, userID, groupID, c.SHA); err != nil {
			utils.Die(err.Error(), 1)
		}
	}

	existing, err := podmanPSContainers(c.App, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	var oldWeb []string
	var nextWeb []string
	for _, processName := range sortedKeys(app.Processes) {
		proc := app.Processes[processName]
		if proc.Port == nil {
			for _, old := range processContainers(existing, processName, "") {
				_, _ = utils.RunChecked("podman", []string{"rm", "-f", old}, "")
			}
		}
		next := identity.ContainerName(c.App, c.Env, processName, c.SHA)
		if err := startProcess(c.App, c.Env, processName, proc, imageTag, userID, groupID, c.SHA); err != nil {
			utils.Die(err.Error(), 1)
		}
		if proc.Port != nil {
			oldWeb = append(oldWeb, processContainers(existing, processName, c.SHA)...)
			nextWeb = append(nextWeb, next)
		}
	}
	return oldWeb, nextWeb
}

func (c appApplyCmd) applyStatic(ctxDir string, app *config.AppContext) error {
	releaseDir := filepath.Join(identity.StaticDir(c.App, c.Env), "releases", c.SHA)
	if err := os.RemoveAll(releaseDir); err != nil {
		return err
	}
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		return err
	}
	for _, routeName := range sortedKeys(app.Routes) {
		route := app.Routes[routeName]
		if route.Serve == "" {
			continue
		}
		src := filepath.Join(ctxDir, route.Serve)
		dst := filepath.Join(releaseDir, routeName)
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		if _, err := utils.RunChecked("cp", []string{"-a", src + "/.", dst + "/"}, ""); err != nil {
			return fmt.Errorf("copy static route %s: %v", routeName, err)
		}
	}
	current := filepath.Join(identity.StaticDir(c.App, c.Env), "current")
	_ = os.Remove(current)
	if err := os.Symlink(releaseDir, current); err != nil {
		return fmt.Errorf("update static current symlink: %v", err)
	}
	return nil
}

type staticCurrentSnapshot struct {
	Target  string
	Existed bool
}

func snapshotStaticCurrent(app, env string) (staticCurrentSnapshot, error) {
	path := filepath.Join(identity.StaticDir(app, env), "current")
	return snapshotStaticCurrentAt(path)
}

func snapshotStaticCurrentAt(path string) (staticCurrentSnapshot, error) {
	target, err := os.Readlink(path)
	if err == nil {
		return staticCurrentSnapshot{Target: target, Existed: true}, nil
	}
	if os.IsNotExist(err) {
		return staticCurrentSnapshot{}, nil
	}
	return staticCurrentSnapshot{}, err
}

func restoreStaticCurrent(app, env string, snapshot staticCurrentSnapshot) error {
	path := filepath.Join(identity.StaticDir(app, env), "current")
	return restoreStaticCurrentAt(path, snapshot)
}

func restoreStaticCurrentAt(path string, snapshot staticCurrentSnapshot) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if snapshot.Existed {
		return os.Symlink(snapshot.Target, path)
	}
	return nil
}

func podmanBuildArgs(app, env, imageTag, release, dockerfile, ctxDir string, rebuild bool) []string {
	args := []string{"build"}
	if rebuild {
		args = append(args, "--no-cache", "--pull=always")
	}
	args = append(args,
		"-t", imageTag,
		"--label", "simple-vps.app="+app,
		"--label", "simple-vps.env="+env,
		"--label", "simple-vps.infra_id="+identity.InfraID(app, env),
		"--label", "simple-vps.release="+release,
		"-f", dockerfile,
		ctxDir,
	)
	return args
}

// hostUserIDs looks up the uid:gid for the per-env Linux account. We
// pass these numerically to podman so `--user` doesn't try to resolve
// the name inside the container image.
func hostUserIDs(name string) (string, string, error) {
	uidOut, err := utils.RunChecked("id", []string{"-u", name}, "")
	if err != nil {
		return "", "", fmt.Errorf("looking up uid for %s: %v", name, err)
	}
	gidOut, err := utils.RunChecked("id", []string{"-g", name}, "")
	if err != nil {
		return "", "", fmt.Errorf("looking up gid for %s: %v", name, err)
	}
	uid := strings.TrimSpace(string(uidOut))
	gid := strings.TrimSpace(string(gidOut))
	if uid == "" || gid == "" {
		return "", "", fmt.Errorf("empty id output for %s", name)
	}
	return uid, gid, nil
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

// resolveEnv merges literal manifest env values with the runtime
// values pulled from the per-(app, env, key) secret store. A missing
// secret fails the whole resolution — no half-applied env file
// reaches the container, and no manifest-vs-store conflict is
// silently chosen for the user.
//
// Manifest literals and secret refs are guaranteed disjoint by
// `config.splitEnvBlock` (a value either *is* a secret ref or is a
// literal; never both). Returning a fresh map keeps the caller's
// `app.Vars` intact for any future reuse.
func resolveEnv(app, env string, literals map[string]string, refs map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(literals)+len(refs))
	for k, v := range literals {
		out[k] = v
	}
	// Sorted for deterministic error messages when multiple refs miss.
	keys := make([]string, 0, len(refs))
	for k := range refs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var missing []string
	for _, envKey := range keys {
		secretKey := refs[envKey]
		val, err := secrets.Get(app, env, secretKey)
		if errors.Is(err, secrets.ErrNotFound) {
			missing = append(missing, fmt.Sprintf("%s (looks up @secret:%s)", envKey, secretKey))
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read secret %s: %w", secretKey, err)
		}
		out[envKey] = string(val)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unresolved @secret references: %s — run `simple-vps secret set %s <key>` for each", strings.Join(missing, ", "), env)
	}
	return out, nil
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

// buildPodmanRunArgs is the pure-function core of startProcess:
// produces the `podman run` argv for one process. Extracted so it can
// be unit-tested without shelling out.
//
// The initial hardening subset from ADR-0005 §7 is always present:
// per-env Linux user, --cap-drop=ALL, --security-opt no-new-privileges,
// --pids-limit, --read-only with a default 64 MiB tmpfs at /tmp.
// No --publish: Caddy reaches the process over the shared `ingress`
// network by container DNS. Manifest-declared memory and CPU limits
// render to the closed set of runtime flags.
func buildPodmanRunArgs(app, env, processName string, proc config.Process, imageTag, userID, groupID, release string, envFileExists bool) []string {
	containerName := identity.ContainerName(app, env, processName, release)
	dataDir := identity.DataDir(app, env)
	appNet := identity.Network(app, env)
	envFile := identity.EnvFile(app, env)

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--restart", "unless-stopped",
		"--user", userID + ":" + groupID,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--read-only",
		// mode=1777 (sticky world-writable) so the per-env container
		// user (--user above) can actually write here. Without it,
		// the tmpfs is owned by root and the unprivileged container
		// process fails with EACCES.
		"--tmpfs", "/tmp:size=64m,mode=1777",
		"--network", appNet,
		"--network", "ingress",
		"-v", dataDir + ":/data:Z",
		"--label", "simple-vps.app=" + app,
		"--label", "simple-vps.env=" + env,
		"--label", "simple-vps.process=" + processName,
		"--label", "simple-vps.infra_id=" + identity.InfraID(app, env),
		"--label", "simple-vps.release=" + release,
	}
	if proc.Resources.Memory != nil {
		args = append(args, "--memory", *proc.Resources.Memory)
	}
	if proc.Resources.CPUs != nil {
		args = append(args, "--cpus", strconv.FormatFloat(*proc.Resources.CPUs, 'f', -1, 64))
	}
	if envFileExists {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, imageTag)
	if proc.Command != "" {
		// Override the image CMD via /bin/sh -c so users can write the
		// command as a single string (ADR-0005 §13).
		args = append(args, "/bin/sh", "-c", proc.Command)
	}
	return args
}

func startProcess(app, env, processName string, proc config.Process, imageTag, userID, groupID, release string) error {
	containerName := identity.ContainerName(app, env, processName, release)
	envFile := identity.EnvFile(app, env)

	_, _ = utils.RunChecked("podman", []string{"rm", "-f", containerName}, "")

	envFileExists := false
	if _, err := os.Stat(envFile); err == nil {
		envFileExists = true
	}
	args := buildPodmanRunArgs(app, env, processName, proc, imageTag, userID, groupID, release, envFileExists)

	if _, err := utils.RunChecked("podman", args, ""); err != nil {
		return fmt.Errorf("podman run %s: %v", containerName, err)
	}

	if proc.Port != nil && proc.Health != "" {
		if err := waitHealthy(containerName, *proc.Port, proc.Health, 30*time.Second); err != nil {
			// Surface logs on failure so the user can see why.
			out, _ := exec.Command("podman", "logs", "--tail", "50", containerName).CombinedOutput()
			os.Stderr.Write(out)
			return fmt.Errorf("health check failed for %s: %v", processName, err)
		}
	}
	return nil
}

// waitHealthy probes the app container's health path via Caddy on the
// shared `ingress` network. The helper itself runs on the host and is
// not a member of `ingress`, so it can't talk to the app container's
// DNS name directly. The Caddy container is on `ingress` by design and
// ships busybox `wget` — exec the probe inside it.
func waitHealthy(containerName string, port int, path string, timeout time.Duration) error {
	url := fmt.Sprintf("http://%s:%d%s", containerName, port, path)
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		cmd := exec.Command("podman", "exec", "caddy", "wget", "-q", "-O", "-", "--timeout=2", url)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		lastErr = fmt.Errorf("%s: %s", url, detail)
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out after %s", timeout)
	}
	return lastErr
}

func runReleaseCommand(app, env, command, imageTag, userID, groupID, release string) error {
	name := identity.ContainerName(app, env, "release", release)
	_, _ = utils.RunChecked("podman", []string{"rm", "-f", name}, "")
	args := []string{
		"run", "--rm",
		"--name", name,
		"--user", userID + ":" + groupID,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--network", identity.Network(app, env),
		"-v", identity.DataDir(app, env) + ":/data:Z",
		"--label", "simple-vps.app=" + app,
		"--label", "simple-vps.env=" + env,
		"--label", "simple-vps.process=release",
		"--label", "simple-vps.infra_id=" + identity.InfraID(app, env),
		"--label", "simple-vps.release=" + release,
	}
	if _, err := os.Stat(identity.EnvFile(app, env)); err == nil {
		args = append(args, "--env-file", identity.EnvFile(app, env))
	}
	args = append(args, imageTag, "/bin/sh", "-c", command)
	if _, err := utils.RunChecked("podman", args, ""); err != nil {
		return fmt.Errorf("release command failed: %v", err)
	}
	return nil
}

func processContainers(entries []containerEntry, processName, excludeRelease string) []string {
	var names []string
	for _, e := range entries {
		if e.Labels["simple-vps.process"] != processName {
			continue
		}
		if excludeRelease != "" && e.Labels["simple-vps.release"] == excludeRelease {
			continue
		}
		if len(e.Names) > 0 {
			names = append(names, e.Names[0])
		}
	}
	sort.Strings(names)
	return names
}

func caddyfilePath(app, env string) string {
	return fmt.Sprintf("/etc/caddy/conf.d/simple-vps-%s-%s.caddy", app, env)
}

func renderAppCaddyfile(app, env string, ctx *config.AppContext, release string) (string, error) {
	type hostKey struct {
		host string
		tls  string
	}
	grouped := map[hostKey][]string{}
	for _, name := range sortedKeys(ctx.Routes) {
		route := ctx.Routes[name]
		grouped[hostKey{host: route.Host, tls: route.TLS}] = append(grouped[hostKey{host: route.Host, tls: route.TLS}], name)
	}

	keys := make([]hostKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].host != keys[j].host {
			return keys[i].host < keys[j].host
		}
		return keys[i].tls < keys[j].tls
	})

	var blocks []string
	for _, key := range keys {
		routeNames := grouped[key]
		sort.SliceStable(routeNames, func(i, j int) bool {
			left := ctx.Routes[routeNames[i]].Path
			right := ctx.Routes[routeNames[j]].Path
			if left == right {
				return routeNames[i] < routeNames[j]
			}
			if left == "" {
				return false
			}
			if right == "" {
				return true
			}
			return len(left) > len(right)
		})

		var prelude string
		if key.tls == "internal" {
			prelude = "\ttls internal\n"
		}
		var body strings.Builder
		useHandles := len(routeNames) > 1 || ctx.Routes[routeNames[0]].Path != ""
		for _, routeName := range routeNames {
			rendered, err := renderRouteBody(app, env, ctx, routeName, release, useHandles)
			if err != nil {
				return "", err
			}
			body.WriteString(rendered)
		}
		quotedHost, err := caddy.CaddyQuote(key.host)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, fmt.Sprintf("%s {\n%s%s}\n", quotedHost, prelude, body.String()))
	}
	return "# generated by simple-vps server app apply — do not edit\n" + strings.Join(blocks, "\n"), nil
}

func renderRouteBody(app, env string, ctx *config.AppContext, routeName string, release string, wrap bool) (string, error) {
	route := ctx.Routes[routeName]
	body, err := renderRouteDirectives(app, env, ctx, routeName, release)
	if err != nil {
		return "", err
	}
	if !wrap {
		return indent(body, 1), nil
	}
	if route.Path == "" {
		return "\thandle {\n" + indent(body, 2) + "\t}\n", nil
	}
	if route.Serve != "" {
		return fmt.Sprintf("\thandle %s {\n\t\trewrite * /\n%s\t}\n\thandle_path %s/* {\n%s\t}\n",
			route.Path, indent(body, 2), route.Path, indent(body, 2)), nil
	}
	return fmt.Sprintf("\thandle %s {\n%s\t}\n\thandle %s/* {\n%s\t}\n",
		route.Path, indent(body, 2), route.Path, indent(body, 2)), nil
}

func renderRouteDirectives(app, env string, ctx *config.AppContext, routeName string, release string) (string, error) {
	route := ctx.Routes[routeName]
	switch {
	case route.Process != "":
		proc, ok := ctx.Processes[route.Process]
		if !ok || proc.Port == nil {
			return "", fmt.Errorf("route %q references process %q with no port", routeName, route.Process)
		}
		upstream := identity.ContainerName(app, env, route.Process, release)
		return fmt.Sprintf("reverse_proxy http://%s:%d\n", upstream, *proc.Port), nil
	case route.Serve != "":
		root := filepath.Join(identity.StaticDir(app, env), "current", routeName)
		quotedRoot, err := caddy.CaddyQuote(root)
		if err != nil {
			return "", fmt.Errorf("route %q: %v", routeName, err)
		}
		return fmt.Sprintf("root * %s\nfile_server\n", quotedRoot), nil
	case route.Redirect != "":
		quotedTo, err := caddy.CaddyQuote(route.Redirect)
		if err != nil {
			return "", fmt.Errorf("route %q: %v", routeName, err)
		}
		return fmt.Sprintf("redir %s permanent\n", quotedTo), nil
	default:
		return "", fmt.Errorf("route %q has no target", routeName)
	}
}

func indent(s string, levels int) string {
	prefix := strings.Repeat("\t", levels)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func writeAppCaddyfile(app, env string, ctx *config.AppContext, release string) error {
	content, err := renderAppCaddyfile(app, env, ctx, release)
	if err != nil {
		return err
	}
	path := caddyfilePath(app, env)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// snapshotCaddyFragment reads the existing fragment so a failed deploy
// can restore it. Returns (contents, true, nil) when a fragment exists,
// (nil, false, nil) when nothing is there, or an error for anything
// other than ENOENT.
func snapshotCaddyFragment(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

// restoreCaddyFragment puts back what snapshotCaddyFragment captured.
// If there was no previous fragment, remove the new one so subsequent
// reloads don't trip on it.
func restoreCaddyFragment(path string, prev []byte, existed bool) error {
	if existed {
		return os.WriteFile(path, prev, 0644)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
