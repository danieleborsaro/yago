package terraform

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	settings := []string{
		"-c", "user.name=yago", "-c", "user.email=yago@example.com", "-c", "init.defaultBranch=main",
		"-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false",
	}
	cmd := exec.Command("git", append(settings, args...)...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func writeTestFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParser_ClonesTheConfigurationFromTheDesiredState_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Clone the configuration repository named by a 2.0.0 desiredstate when no configuration file is given",
		CurrentImpl:     "Parser.LoadGitOpsFilesExtended tries each locator from wrapper.UsableConfigRepoLocators",
		ExpectedOutcome: "The configuration is loaded from the repository at the desiredstate's tag",
		Rationale:       "Desiredstates pin their configuration by tag, and a plan must use exactly that version",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a configuration repository whose tag and main branch differ
	configRepo := t.TempDir()
	runGit(t, configRepo, "init", "--quiet")
	writeTestFiles(t, configRepo, map[string]string{
		"example/deploy/configuration.yaml": `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/deploy/configuration.yaml
  wrappers:
    terraform:
      parts:
        terraform: example/deploy/terraform/terraform.yaml
`,
		"example/deploy/terraform/terraform.yaml": "---\nworkspace: tagged\n",
	})
	runGit(t, configRepo, "add", "-A")
	runGit(t, configRepo, "commit", "--quiet", "-m", "Tagged configuration")
	runGit(t, configRepo, "tag", "2026-01-01.1")
	writeTestFiles(t, configRepo, map[string]string{"example/deploy/terraform/terraform.yaml": "---\nworkspace: main\n"})
	runGit(t, configRepo, "commit", "--quiet", "-am", "Later change")

	// Given: a desiredstate that pins the configuration at the tag, under configuration.all.terraform.git
	desiredStates := t.TempDir()
	writeTestFiles(t, desiredStates, map[string]string{
		"example/deploy/desiredstate.yaml": `---
schema: 2.0.0
namespace: yago
kind: DesiredState
desiredstate:
  meta:
    repo:
      git:
        url: git@example.com:example/desiredstates.git
        branch: main
        tag: ''
    parts:
      self: example/deploy/desiredstate.yaml
      terraform: example/deploy/terraform/terraform.yaml
`,
		"example/deploy/terraform/terraform.yaml": `---
desiredstate:
  configuration:
    all:
      terraform:
        git:
          url: ` + configRepo + `
          branch: ""
          tag: !!str 2026-01-01.1
          path: example/deploy/configuration.yaml
`,
	})

	// When: the desiredstate is loaded without a configuration file
	p := NewParser("all", nil)
	p.configWorkdir = t.TempDir()
	err := p.LoadGitOpsFilesExtended(filepath.Join(desiredStates, "example/deploy/desiredstate.yaml"), "", "", t.TempDir())

	// Then: the configuration comes from the tag, not from main
	if err != nil {
		t.Fatal(err)
	}
	content := p.GetConfiguration().GetDocument().GetContent().Data
	if content["workspace"] != "tagged" {
		t.Fatalf("expected the tagged configuration, got workspace %v", content["workspace"])
	}
}
