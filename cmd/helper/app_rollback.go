package helper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fprl/simple-vps/internal/config"
	"github.com/fprl/simple-vps/internal/identity"
	"github.com/fprl/simple-vps/internal/utils"
)

// appRollbackCmd swaps the running containers for one (app, env) back to an
// older local image tag. Container images are the release artifacts for
// container apps; the last applied manifest supplies the service and route
// shape.
type appRollbackCmd struct {
	App     string `arg:"" help:"App name."`
	Env     string `arg:"" help:"Env name."`
	Release string `arg:"" optional:"" help:"Release tag to run. Omitted = previous local image."`
	JSON    bool   `name:"json" help:"Emit structured JSON instead of the text summary."`
}

func (c appRollbackCmd) Run() error {
	if err := validateAppEnv(c.App, c.Env); err != nil {
		utils.Die(err.Error(), 1)
	}
	withAppEnvLock(c.App, c.Env, func() {
		c.runLocked()
	})
	return nil
}

func (c appRollbackCmd) runLocked() {
	containers, err := podmanPSContainers(c.App, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	current, err := currentRelease(containersToServices(containers))
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	images, err := podmanImages(c.App, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	target, err := selectRollbackRelease(images, current, c.Release)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	app, cleanup, err := loadAppliedAppContext(c.App, c.Env)
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	defer cleanup()

	userID, groupID, err := hostUserIDs(identity.SystemUser(c.App, c.Env))
	if err != nil {
		utils.Die(err.Error(), 1)
	}
	imageTag := identity.ImageTag(c.App, c.Env, target.Release)
	for _, svcName := range sortedKeys(app.Services) {
		svc := app.Services[svcName]
		if err := startService(c.App, c.Env, svcName, svc, imageTag, userID, groupID); err != nil {
			utils.Die(err.Error(), 1)
		}
	}
	if err := writeAppCaddyfile(c.App, c.Env, app); err != nil {
		utils.Die(err.Error(), 1)
	}
	if _, err := utils.RunChecked("podman", []string{"exec", "caddy", "caddy", "validate", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"}, ""); err != nil {
		utils.Die(fmt.Sprintf("caddy validate after rollback: %v", err), 1)
	}
	if _, err := utils.RunChecked("podman", []string{"exec", "caddy", "caddy", "reload", "--config", "/etc/caddy/Caddyfile"}, ""); err != nil {
		utils.Die(fmt.Sprintf("caddy reload after rollback: %v", err), 1)
	}

	result := rollbackPayload{
		App:      c.App,
		Env:      c.Env,
		Previous: current,
		Release:  target.Release,
		Services: serviceNames(app.Services),
	}
	if c.JSON {
		buf, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			utils.Die(err.Error(), 1)
		}
		fmt.Println(string(buf))
		return
	}
	fmt.Print(renderRollbackText(result))
}

type rollbackPayload struct {
	App      string   `json:"app"`
	Env      string   `json:"env"`
	Previous string   `json:"previous"`
	Release  string   `json:"release"`
	Services []string `json:"services"`
}

type imageRelease struct {
	Release string
	Image   string
}

type imageEntry struct {
	Repository string            `json:"Repository"`
	Tag        string            `json:"Tag"`
	Labels     map[string]string `json:"Labels"`
}

func podmanImages(app, env string) ([]imageRelease, error) {
	out, err := utils.RunChecked("podman", []string{"images", "--format", "json"}, "")
	if err != nil {
		return nil, fmt.Errorf("podman images: %v", err)
	}
	out = []byte(strings.TrimSpace(string(out)))
	if len(out) == 0 {
		return nil, nil
	}
	var entries []imageEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse podman images json: %v", err)
	}
	repo := identity.ImageRepo(app, env)
	var releases []imageRelease
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Repository != repo {
			continue
		}
		if e.Labels["app"] != app || e.Labels["env"] != env {
			continue
		}
		release := e.Labels["simple_vps_release"]
		if release == "" {
			release = e.Tag
		}
		if release == "" || release == "<none>" || seen[release] {
			continue
		}
		seen[release] = true
		releases = append(releases, imageRelease{Release: release, Image: repo + ":" + release})
	}
	return releases, nil
}

func currentRelease(services []serviceStatus) (string, error) {
	if len(services) == 0 {
		return "", fmt.Errorf("no services running; deploy before rollback")
	}
	current := services[0].Release
	if current == "" {
		return "", fmt.Errorf("running services do not expose a release label; cannot choose rollback target")
	}
	for _, svc := range services[1:] {
		if svc.Release != current {
			return "", fmt.Errorf("running services are on different releases; pass an explicit release")
		}
	}
	return current, nil
}

func selectRollbackRelease(images []imageRelease, current, requested string) (imageRelease, error) {
	if requested != "" {
		for _, img := range images {
			if img.Release == requested {
				if requested == current {
					return imageRelease{}, fmt.Errorf("%s is already running", requested)
				}
				return img, nil
			}
		}
		return imageRelease{}, fmt.Errorf("release %s is not available locally", requested)
	}
	for _, img := range images {
		if img.Release != current {
			return img, nil
		}
	}
	return imageRelease{}, fmt.Errorf("no previous release available locally")
}

func loadAppliedAppContext(app, env string) (*config.AppContext, func(), error) {
	manifestPath := identity.ManifestFile(app, env)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, func() {}, fmt.Errorf("applied manifest not found at %s; deploy once before rollback", manifestPath)
	}
	tmp, err := os.MkdirTemp("", "simple-vps-rollback-manifest-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "simple-vps.toml"), data, 0644); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	ctx, err := config.LoadAppContext(tmp, env)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if ctx.AppName != app {
		cleanup()
		return nil, func() {}, fmt.Errorf("applied manifest names app %s, expected %s", ctx.AppName, app)
	}
	if ctx.Shape != config.ShapeContainer {
		cleanup()
		return nil, func() {}, fmt.Errorf("rollback currently supports container apps only (got shape %q)", ctx.Shape)
	}
	return ctx, cleanup, nil
}

func serviceNames(services map[string]config.Service) []string {
	return sortedKeys(services)
}

func renderRollbackText(payload rollbackPayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Rolled back %s (%s) from %s to %s\n", payload.App, payload.Env, payload.Previous, payload.Release)
	for _, svc := range payload.Services {
		fmt.Fprintf(&b, "  %-12s running\n", svc)
	}
	return b.String()
}
