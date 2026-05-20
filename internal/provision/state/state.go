package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const CurrentVersion = 1

type ExposeMode string

const (
	ExposePublic  ExposeMode = "public"
	ExposePrivate ExposeMode = "private"
)

type TunnelMode string

const (
	TunnelNone            TunnelMode = "none"
	TunnelCloudflare      TunnelMode = "cloudflare"
	TunnelTailscaleFunnel TunnelMode = "tailscale-funnel"
)

type Store struct {
	Root string
}

type HostFile struct {
	Version  int          `json:"version"`
	Desired  HostDesired  `json:"desired"`
	Observed HostObserved `json:"observed"`
	Meta     HostMeta     `json:"meta"`
}

type HostDesired struct {
	Users    HostUsers                 `json:"users"`
	Ingress  HostIngressDesired        `json:"ingress"`
	Features HostFeatures              `json:"features"`
	Packages map[string]DesiredPackage `json:"packages"`
}

type HostUsers struct {
	Operator string `json:"operator"`
	Deploy   string `json:"deploy"`
}

type HostIngressDesired struct {
	Expose ExposeMode `json:"expose"`
	Tunnel TunnelMode `json:"tunnel"`
}

type HostFeatures struct {
	Docker     bool     `json:"docker"`
	Litestream bool     `json:"litestream"`
	Runtimes   []string `json:"runtimes"`
}

type DesiredPackage struct {
	Source  string `json:"source"`
	Track   string `json:"track,omitempty"`
	Version string `json:"version,omitempty"`
}

type HostObserved struct {
	Packages map[string]ObservedPackage `json:"packages"`
	Ingress  HostIngressObserved        `json:"ingress"`
}

type ObservedPackage struct {
	Version string `json:"version"`
}

type HostIngressObserved struct {
	UFW80443Allowed          bool `json:"ufw_80_443_allowed"`
	CloudflaredServiceActive bool `json:"cloudflared_service_active"`
}

type HostMeta struct {
	InstalledAt      string     `json:"installed_at,omitempty"`
	SimpleVPSVersion string     `json:"simple_vps_version,omitempty"`
	LastApply        *ApplyMeta `json:"last_apply,omitempty"`
}

type ApplyMeta struct {
	ID                string `json:"id"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at"`
	Status            string `json:"status"`
	OperationsChanged int    `json:"operations_changed"`
}

type AppsFile struct {
	Version int            `json:"version"`
	Apps    map[string]App `json:"apps"`
}

type App struct {
	Path           string   `json:"path"`
	Services       []string `json:"services"`
	CurrentRelease string   `json:"current_release"`
}

type RoutesFile struct {
	Version int     `json:"version"`
	Routes  []Route `json:"routes"`
}

type Route struct {
	App     string `json:"app"`
	Host    string `json:"host"`
	Type    string `json:"type"`
	Service string `json:"service,omitempty"`
	Port    int    `json:"port,omitempty"`
}

type CloudflareFile struct {
	Version    int               `json:"version"`
	AccountID  string            `json:"account_id,omitempty"`
	TunnelID   string            `json:"tunnel_id,omitempty"`
	TunnelName string            `json:"tunnel_name,omitempty"`
	DNSRecords map[string]string `json:"dns_records"`
}

type hostFileRaw struct {
	Version  int             `json:"version"`
	Desired  json.RawMessage `json:"desired"`
	Observed HostObserved    `json:"observed"`
	Meta     HostMeta        `json:"meta"`
}

func DefaultRoot() string {
	if root := os.Getenv("SIMPLE_VPS_PROVISION_STATE_DIR"); root != "" {
		return root
	}
	return "/etc/simple-vps"
}

func (s Store) root() string {
	if s.Root != "" {
		return s.Root
	}
	return DefaultRoot()
}

func (s Store) HostPath() string {
	return filepath.Join(s.root(), "host.json")
}

func (s Store) AppsPath() string {
	return filepath.Join(s.root(), "apps.json")
}

func (s Store) RoutesPath() string {
	return filepath.Join(s.root(), "routes.json")
}

func (s Store) CloudflarePath() string {
	return filepath.Join(s.root(), "providers", "cloudflare.json")
}

func (s Store) SecretsDir() string {
	return filepath.Join(s.root(), "secrets")
}

func (s Store) HostInstalled() (bool, error) {
	if _, err := os.Stat(s.HostPath()); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ReadHost returns os.IsNotExist(err) when host.json has not been created yet.
func (s Store) ReadHost() (*HostFile, error) {
	var file HostFile
	if err := readJSON(s.HostPath(), &file); err != nil {
		return nil, err
	}
	if err := validateVersion("host.json", file.Version); err != nil {
		return nil, err
	}
	normalizeHostFile(&file)
	if err := validateHostDesired(file.Desired); err != nil {
		return nil, fmt.Errorf("invalid host.json desired: %w", err)
	}
	return &file, nil
}

func (s Store) WriteHostDesired(desired HostDesired) error {
	normalizeHostDesired(&desired)
	if err := validateHostDesired(desired); err != nil {
		return fmt.Errorf("invalid host desired: %w", err)
	}

	file, err := s.readHostForDesiredWrite()
	if err != nil {
		return err
	}
	file.Desired = desired
	normalizeHostFile(&file)
	return writeHostFile(s.HostPath(), file)
}

func (s Store) WriteHostState(observed HostObserved, meta HostMeta) error {
	file, err := s.readHostRaw()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("host.json is required before writing host state")
		}
		return err
	}

	var desired HostDesired
	if err := json.Unmarshal(file.Desired, &desired); err != nil {
		return fmt.Errorf("invalid host.json desired: %w", err)
	}
	normalizeHostDesired(&desired)
	if err := validateHostDesired(desired); err != nil {
		return fmt.Errorf("invalid host.json desired: %w", err)
	}

	normalizeHostObserved(&observed)
	return writeHostState(s.HostPath(), file.Desired, observed, meta)
}

func (s Store) ReadApps() (*AppsFile, error) {
	var file AppsFile
	if err := readJSON(s.AppsPath(), &file); err != nil {
		if os.IsNotExist(err) {
			return &AppsFile{Version: CurrentVersion, Apps: map[string]App{}}, nil
		}
		return nil, err
	}
	if err := validateVersion("apps.json", file.Version); err != nil {
		return nil, err
	}
	file.Version = CurrentVersion
	if file.Apps == nil {
		file.Apps = map[string]App{}
	}
	return &file, nil
}

func (s Store) WriteApps(file AppsFile) error {
	if err := validateVersion("apps.json", file.Version); err != nil {
		return err
	}
	normalizeAppsFile(&file)
	return writeJSON(s.AppsPath(), file, 0644)
}

func (s Store) ReadRoutes() (*RoutesFile, error) {
	var file RoutesFile
	if err := readJSON(s.RoutesPath(), &file); err != nil {
		if os.IsNotExist(err) {
			return &RoutesFile{Version: CurrentVersion, Routes: []Route{}}, nil
		}
		return nil, err
	}
	if err := validateVersion("routes.json", file.Version); err != nil {
		return nil, err
	}
	file.Version = CurrentVersion
	if file.Routes == nil {
		file.Routes = []Route{}
	}
	return &file, nil
}

func (s Store) WriteRoutes(file RoutesFile) error {
	if err := validateVersion("routes.json", file.Version); err != nil {
		return err
	}
	normalizeRoutesFile(&file)
	return writeJSON(s.RoutesPath(), file, 0644)
}

func (s Store) ReadCloudflare() (*CloudflareFile, error) {
	var file CloudflareFile
	if err := readJSON(s.CloudflarePath(), &file); err != nil {
		if os.IsNotExist(err) {
			return &CloudflareFile{Version: CurrentVersion, DNSRecords: map[string]string{}}, nil
		}
		return nil, err
	}
	if err := validateVersion("providers/cloudflare.json", file.Version); err != nil {
		return nil, err
	}
	file.Version = CurrentVersion
	if file.DNSRecords == nil {
		file.DNSRecords = map[string]string{}
	}
	return &file, nil
}

func (s Store) WriteCloudflare(file CloudflareFile) error {
	if err := validateVersion("providers/cloudflare.json", file.Version); err != nil {
		return err
	}
	file.Version = CurrentVersion
	if file.DNSRecords == nil {
		file.DNSRecords = map[string]string{}
	}
	return writeJSON(s.CloudflarePath(), file, 0600)
}

func newHostFile() *HostFile {
	file := HostFile{Version: CurrentVersion}
	normalizeHostFile(&file)
	return &file
}

func normalizeHostFile(file *HostFile) {
	file.Version = CurrentVersion
	normalizeHostDesired(&file.Desired)
	normalizeHostObserved(&file.Observed)
}

func normalizeHostDesired(desired *HostDesired) {
	if desired.Packages == nil {
		desired.Packages = map[string]DesiredPackage{}
	}
	if desired.Features.Runtimes == nil {
		desired.Features.Runtimes = []string{}
	}
	sort.Strings(desired.Features.Runtimes)
}

func normalizeHostObserved(observed *HostObserved) {
	if observed.Packages == nil {
		observed.Packages = map[string]ObservedPackage{}
	}
}

func normalizeAppsFile(file *AppsFile) {
	file.Version = CurrentVersion
	if file.Apps == nil {
		file.Apps = map[string]App{}
	}
	for name, app := range file.Apps {
		sort.Strings(app.Services)
		file.Apps[name] = app
	}
}

func normalizeRoutesFile(file *RoutesFile) {
	file.Version = CurrentVersion
	if file.Routes == nil {
		file.Routes = []Route{}
	}
	sort.Slice(file.Routes, func(i, j int) bool {
		left := file.Routes[i]
		right := file.Routes[j]
		if left.Host != right.Host {
			return left.Host < right.Host
		}
		if left.App != right.App {
			return left.App < right.App
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Service != right.Service {
			return left.Service < right.Service
		}
		return left.Port < right.Port
	})
}

func validateHostDesired(desired HostDesired) error {
	if strings.TrimSpace(desired.Users.Operator) == "" {
		return fmt.Errorf("users.operator is required")
	}
	if strings.TrimSpace(desired.Users.Deploy) == "" {
		return fmt.Errorf("users.deploy is required")
	}
	switch desired.Ingress.Expose {
	case ExposePublic, ExposePrivate:
	default:
		return fmt.Errorf("ingress.expose must be public or private")
	}
	switch desired.Ingress.Tunnel {
	case TunnelNone, TunnelCloudflare, TunnelTailscaleFunnel:
	default:
		return fmt.Errorf("ingress.tunnel must be none, cloudflare, or tailscale-funnel")
	}
	for _, runtime := range desired.Features.Runtimes {
		if strings.TrimSpace(runtime) == "" {
			return fmt.Errorf("features.runtimes cannot contain empty values")
		}
	}
	for name, pkg := range desired.Packages {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("packages cannot contain empty names")
		}
		if strings.TrimSpace(pkg.Source) == "" {
			return fmt.Errorf("packages.%s.source is required", name)
		}
	}
	return nil
}

func validateVersion(scope string, version int) error {
	if version == 0 {
		return fmt.Errorf("%s version is required", scope)
	}
	if version > CurrentVersion {
		return fmt.Errorf("unsupported %s version %d", scope, version)
	}
	return nil
}

func (s Store) readHostForDesiredWrite() (HostFile, error) {
	var file HostFile
	if err := readJSON(s.HostPath(), &file); err != nil {
		if os.IsNotExist(err) {
			file = *newHostFile()
			return file, nil
		}
		return HostFile{}, err
	}
	if err := validateVersion("host.json", file.Version); err != nil {
		return HostFile{}, err
	}
	normalizeHostFile(&file)
	return file, nil
}

func (s Store) readHostRaw() (hostFileRaw, error) {
	var file hostFileRaw
	if err := readJSON(s.HostPath(), &file); err != nil {
		return hostFileRaw{}, err
	}
	if err := validateVersion("host.json", file.Version); err != nil {
		return hostFileRaw{}, err
	}
	if len(bytes.TrimSpace(file.Desired)) == 0 {
		return hostFileRaw{}, fmt.Errorf("host.json desired is required")
	}
	return file, nil
}

func readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("invalid %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), mode)
}

func writeHostFile(path string, file HostFile) error {
	file.Version = CurrentVersion
	normalizeHostFile(&file)
	if err := validateHostDesired(file.Desired); err != nil {
		return fmt.Errorf("invalid host desired: %w", err)
	}
	return writeJSON(path, file, 0644)
}

// writeHostState preserves desired as raw JSON so apply cannot rewrite intent
// while recording observed host state and apply metadata.
func writeHostState(path string, desired json.RawMessage, observed HostObserved, meta HostMeta) error {
	observedData, err := json.MarshalIndent(observed, "  ", "  ")
	if err != nil {
		return err
	}
	metaData, err := json.MarshalIndent(meta, "  ", "  ")
	if err != nil {
		return err
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "{\n  \"version\": %d,\n", CurrentVersion)
	out.WriteString("  \"desired\": ")
	out.Write(bytes.TrimSpace(desired))
	out.WriteString(",\n  \"observed\": ")
	out.Write(observedData)
	out.WriteString(",\n  \"meta\": ")
	out.Write(metaData)
	out.WriteString("\n}\n")
	return atomicWrite(path, out.Bytes(), 0644)
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if _, err := os.Stat(tmpName); err == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
