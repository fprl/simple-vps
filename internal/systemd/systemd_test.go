package systemd

import (
	"strings"
	"testing"
)

func TestDeployTmpDirDefaultMatchesServerAPI(t *testing.T) {
	t.Setenv("SIMPLE_VPS_DEPLOY_TMP_DIR", "")
	if got := DeployTmpDir(); got != "/tmp/simple-vps-deploy" {
		t.Fatalf("unexpected deploy temp dir: %s", got)
	}
}

func TestPathIsRelativeToAllowsNamesStartingWithDotDot(t *testing.T) {
	if !PathIsRelativeTo("/srv/app/..data/file", "/srv/app") {
		t.Fatal("expected path under base to be accepted")
	}
	if PathIsRelativeTo("/srv/app-other/file", "/srv/app") {
		t.Fatal("expected sibling path to be rejected")
	}
}

func TestValidateEnvironmentContentRejectsShellSyntax(t *testing.T) {
	valid := "API_KEY=secret\nEMPTY=\n# comment\n"
	if err := ValidateEnvironmentContent(valid); err != nil {
		t.Fatalf("expected valid env content, got %v", err)
	}

	tests := []string{
		"export API_KEY=secret\n",
		"API KEY=secret\n",
		"API_KEY =secret\n",
		"API_KEY='secret'\n",
		"API_KEY=secret # comment\n",
		"not-a-pair\n",
	}
	for _, content := range tests {
		t.Run(strings.TrimSpace(content), func(t *testing.T) {
			if err := ValidateEnvironmentContent(content); err == nil {
				t.Fatal("expected env content to be rejected")
			}
		})
	}
}
