package host

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureFileWritesOnlyWhenContentOrMetadataDiffers(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner}
	file := File{
		Path:    "/etc/simple-vps/host.json",
		Content: []byte("one\n"),
		Owner:   "root",
		Group:   "root",
		Mode:    0644,
	}

	changed, err := EnsureFile(apply, file)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected missing file to be changed")
	}
	assertWrites(t, runner, file.Path)

	changed, err = EnsureFile(apply, file)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected identical file to be unchanged")
	}
	assertWrites(t, runner, file.Path)

	file.Mode = 0600
	changed, err = EnsureFile(apply, file)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected mode drift to be changed")
	}
	if len(runner.writes) != 2 {
		t.Fatalf("expected two writes, got %d", len(runner.writes))
	}
}

func TestEnsureFileCheckModeReportsDriftWithoutWriting(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner, CheckMode: true}

	changed, err := EnsureFile(apply, File{
		Path:    "/etc/simple-vps/host.json",
		Content: []byte("{}\n"),
		Owner:   "root",
		Group:   "root",
		Mode:    0644,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected missing file to be reported as changed")
	}
	if len(runner.writes) != 0 {
		t.Fatalf("check mode wrote files: %+v", runner.writes)
	}
}

func TestEnsureFileRejectsMissingMode(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureFile(apply, File{
		Path:    "/etc/simple-vps/host.json",
		Content: []byte("{}\n"),
		Owner:   "root",
		Group:   "root",
	})
	if err == nil {
		t.Fatal("expected missing file mode to fail")
	}
	if changed {
		t.Fatal("missing file mode must not report changed")
	}
	if len(runner.writes) != 0 {
		t.Fatalf("missing file mode wrote files: %+v", runner.writes)
	}
}

func TestEnsureSudoersFileValidatesBeforeWriting(t *testing.T) {
	runner := newFakeRunner()
	runner.validateErr = errors.New("bad sudoers")
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureSudoersFile(apply, "simple-vps", []byte("deploy ALL=(root) NOPASSWD: /usr/local/bin/simple-vps"))
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if changed {
		t.Fatal("invalid sudoers content must not report changed")
	}
	if len(runner.writes) != 0 {
		t.Fatalf("invalid sudoers content wrote files: %+v", runner.writes)
	}

	runner.validateErr = nil
	changed, err = EnsureSudoersFile(apply, "simple-vps", []byte("deploy ALL=(root) NOPASSWD: /usr/local/bin/simple-vps"))
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected sudoers file to be written")
	}
	got := runner.files["/etc/sudoers.d/simple-vps"]
	if string(got.Content) != "deploy ALL=(root) NOPASSWD: /usr/local/bin/simple-vps\n" {
		t.Fatalf("unexpected sudoers content: %q", string(got.Content))
	}
	if got.Owner != "root" || got.Group != "root" || got.Mode != 0440 {
		t.Fatalf("unexpected sudoers metadata: %+v", got)
	}
}

func TestEnsureSudoersFileRejectsUnsafeName(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner}

	_, err := EnsureSudoersFile(apply, "../root", []byte("deploy ALL=(root) NOPASSWD: /usr/local/bin/simple-vps\n"))
	if err == nil {
		t.Fatal("expected unsafe sudoers name to fail")
	}
	if len(runner.validations) != 0 || len(runner.writes) != 0 {
		t.Fatalf("unsafe sudoers name touched runner: validations=%+v writes=%+v", runner.validations, runner.writes)
	}
}

func TestEnsureSystemdUnitWritesUnitReloadsDaemonThenRunsRequestedAction(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureSystemdUnit(apply, SystemdUnit{
		Name:    "caddy.service",
		Content: []byte("[Unit]\nDescription=Caddy\n"),
		Action:  Reloaded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected new unit and requested reload to report changed")
	}

	wantCommands := []Command{
		{Program: "systemctl", Args: []string{"daemon-reload"}},
		{Program: "systemctl", Args: []string{"reload", "caddy.service"}},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("unexpected commands:\nwant: %+v\n got: %+v", wantCommands, runner.commands)
	}

	runner.commands = nil
	changed, err = EnsureSystemdUnit(apply, SystemdUnit{
		Name:    "caddy.service",
		Content: []byte("[Unit]\nDescription=Caddy\n"),
		Action:  NoSystemdAction,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected unchanged unit with no action to report unchanged")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("unchanged unit ran commands: %+v", runner.commands)
	}
}

func TestEnsureSystemdUnitCheckModeDoesNotWriteOrRunCommands(t *testing.T) {
	runner := newFakeRunner()
	apply := Apply{Context: context.Background(), Runner: runner, CheckMode: true}

	changed, err := EnsureSystemdUnit(apply, SystemdUnit{
		Name:    "simple-vps.service",
		Content: []byte("[Service]\nExecStart=/usr/local/bin/simple-vps server\n"),
		Action:  Restarted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected check mode to report pending unit/action change")
	}
	if len(runner.writes) != 0 || len(runner.commands) != 0 {
		t.Fatalf("check mode touched host: writes=%+v commands=%+v", runner.writes, runner.commands)
	}
}

func TestEnsureSystemdUnitStartedUsesServiceState(t *testing.T) {
	runner := newFakeRunner()
	content := []byte("[Unit]\nDescription=Caddy\n")
	runner.files["/etc/systemd/system/caddy.service"] = FileState{
		Content: content,
		Owner:   "root",
		Group:   "root",
		Mode:    0644,
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureSystemdUnit(apply, SystemdUnit{
		Name:    "caddy.service",
		Content: content,
		Action:  Started,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected already-active service to be unchanged")
	}

	wantCommands := []Command{
		{Program: "systemctl", Args: []string{"is-active", "--quiet", "caddy.service"}},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("unexpected commands:\nwant: %+v\n got: %+v", wantCommands, runner.commands)
	}
}

func TestEnsureUserCorrectsExistingShellHomeAndPrimaryGroup(t *testing.T) {
	runner := newFakeRunner()
	runner.commandResults = map[string]CommandResult{
		"getent group deploy":  {Stdout: []byte("deploy:x:2000:\n")},
		"getent passwd deploy": {Stdout: []byte("deploy:x:1001:1001::/old:/bin/sh\n")},
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureUser(apply, User{
		Name:         "deploy",
		PrimaryGroup: "deploy",
		Shell:        "/bin/bash",
		Home:         "/home/deploy",
		CreateHome:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected user drift to be corrected")
	}

	wantCommands := []Command{
		{Program: "getent", Args: []string{"group", "deploy"}},
		{Program: "getent", Args: []string{"passwd", "deploy"}},
		{Program: "usermod", Args: []string{"--gid", "deploy", "--shell", "/bin/bash", "--home", "/home/deploy", "--move-home", "deploy"}},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("unexpected commands:\nwant: %+v\n got: %+v", wantCommands, runner.commands)
	}
}

func TestEnsureUserSkipsAlreadyConvergedUser(t *testing.T) {
	runner := newFakeRunner()
	runner.commandResults = map[string]CommandResult{
		"getent group deploy":  {Stdout: []byte("deploy:x:2000:\n")},
		"getent passwd deploy": {Stdout: []byte("deploy:x:1001:2000::/home/deploy:/bin/bash\n")},
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureUser(apply, User{
		Name:         "deploy",
		PrimaryGroup: "deploy",
		Shell:        "/bin/bash",
		CreateHome:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected converged user to be unchanged")
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected only getent probes, got %+v", runner.commands)
	}
}

func TestEnsureBlockInFileReplacesMarkedBlock(t *testing.T) {
	runner := newFakeRunner()
	runner.files["/etc/example.conf"] = FileState{
		Content: []byte("one\n# BEGIN Simple VPS\nold\n# END Simple VPS\nlast\n"),
		Owner:   "root",
		Group:   "root",
		Mode:    0644,
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureBlockInFile(apply, BlockInFile{
		Path:       "/etc/example.conf",
		MarkerName: "Simple VPS",
		Block:      "new",
		Owner:      "root",
		Group:      "root",
		Mode:       0644,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected block replacement to change file")
	}
	got := string(runner.files["/etc/example.conf"].Content)
	want := "one\n# BEGIN Simple VPS\nnew\n# END Simple VPS\nlast\n"
	if got != want {
		t.Fatalf("unexpected content:\nwant %q\n got %q", want, got)
	}

	changed, err = EnsureBlockInFile(apply, BlockInFile{
		Path:       "/etc/example.conf",
		MarkerName: "Simple VPS",
		Block:      "new",
		Owner:      "root",
		Group:      "root",
		Mode:       0644,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected identical block to be unchanged")
	}
}

func TestEnsureUfwRuleSkipsAlreadyAppliedRule(t *testing.T) {
	runner := newFakeRunner()
	runner.commandResults = map[string]CommandResult{
		"ufw status verbose": {Stdout: []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n22/tcp ALLOW IN Anywhere\n")},
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureUfwRule(apply, UfwRule{Rule: "allow 22/tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected existing ufw rule to be unchanged")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected only status probe, got %+v", runner.commands)
	}
}

func TestEnsureUfwRuleReportsMissingDeleteAsUnchanged(t *testing.T) {
	runner := newFakeRunner()
	runner.commandResults = map[string]CommandResult{
		"ufw status verbose": {Stdout: []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n")},
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureUfwRule(apply, UfwRule{Rule: "allow 80/tcp", Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected missing ufw rule delete to be unchanged")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected only status probe, got %+v", runner.commands)
	}
}

func TestEnsureUfwRuleRunsWhenRuleMissing(t *testing.T) {
	runner := newFakeRunner()
	runner.commandResults = map[string]CommandResult{
		"ufw status verbose": {Stdout: []byte("Status: active\nDefault: deny (incoming), allow (outgoing), disabled (routed)\n")},
	}
	apply := Apply{Context: context.Background(), Runner: runner}

	changed, err := EnsureUfwRule(apply, UfwRule{Rule: "allow 22/tcp"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected missing ufw rule to change")
	}
	wantCommands := []Command{
		{Program: "ufw", Args: []string{"status", "verbose"}},
		{Program: "ufw", Args: []string{"--force", "allow", "22/tcp"}},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("unexpected commands:\nwant: %+v\n got: %+v", wantCommands, runner.commands)
	}
}

type fakeRunner struct {
	files          map[string]FileState
	writes         []File
	validations    []Validation
	validateErr    error
	commands       []Command
	commandResults map[string]CommandResult
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{files: make(map[string]FileState)}
}

func (r *fakeRunner) ReadFile(_ context.Context, path string) (FileState, error) {
	file, ok := r.files[path]
	if !ok {
		return FileState{}, ErrNotExist
	}
	return file, nil
}

func (r *fakeRunner) WriteFile(_ context.Context, file File) error {
	r.writes = append(r.writes, file)
	r.files[file.Path] = FileState{
		Content: append([]byte(nil), file.Content...),
		Owner:   file.Owner,
		Group:   file.Group,
		Mode:    file.Mode,
	}
	return nil
}

func (r *fakeRunner) Validate(_ context.Context, validation Validation) error {
	r.validations = append(r.validations, validation)
	return r.validateErr
}

func (r *fakeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	r.commands = append(r.commands, command)
	if result, ok := r.commandResults[commandKey(command)]; ok {
		return result, nil
	}
	return CommandResult{}, nil
}

func commandKey(command Command) string {
	return command.Program + " " + strings.Join(command.Args, " ")
}

func assertWrites(t *testing.T, runner *fakeRunner, path string) {
	t.Helper()
	if len(runner.writes) != 1 {
		t.Fatalf("expected one write, got %d", len(runner.writes))
	}
	if runner.writes[0].Path != path {
		t.Fatalf("unexpected write path: %s", runner.writes[0].Path)
	}
}

var _ Runner = (*fakeRunner)(nil)
