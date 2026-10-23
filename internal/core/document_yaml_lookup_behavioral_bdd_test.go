package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// a config repo whose terraform part reads an account id from a file that isn't a part
var yamlLookupConfigRepo = map[string]string{
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
        ecosystem: shared/ecosystems/example-dev/project-properties.yaml
        terraform: example/deploy/terraform/terraform.yaml
`,
	"shared/ecosystems/example-dev/project-properties.yaml": `---
project_properties_eco:
  aws_account_id: "111111111111"
`,
	"shared/ecosystems/example-prod/project-properties.yaml": `---
project_properties_eco:
  aws_account_id: "123456789012"
`,
	"example/deploy/terraform/terraform.yaml": `---
prod_account_id: "[[gitops.getYamlValue(project_properties_eco.aws_account_id, shared/ecosystems/example-prod/project-properties.yaml)]]"
own_account_id: "[[gitops.getYamlValue(project_properties_eco.aws_account_id)]]"
`,
}

func assertYamlLookupAccounts(t *testing.T, doc *GitOpsDocument) {
	t.Helper()
	content := doc.GetContent().Data
	if content["prod_account_id"] != "123456789012" {
		t.Errorf("prod_account_id = %v, want 123456789012 read from the file under the config repo root", content["prod_account_id"])
	}
	if content["own_account_id"] != "111111111111" {
		t.Errorf("own_account_id = %v, want 111111111111 read from the merged parts", content["own_account_id"])
	}
}

func TestGitOpsDocument_YamlLookupFileFromLocalConfig_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "getYamlValue with a file reads it from the config repo root when the config is passed with -c",
		CurrentImpl:     "LoadMetadata reads the root raw, computes the workdir from parts.self, then loads it and its parts with a handler based there",
		ExpectedOutcome: "shared/... resolves inside the config repo, not inside the directory yago runs from",
		Rationale:       "yago runs from the terraform root with -s ., so the file was looked up in the terraform root and wasn't found",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a config repo somewhere other than the directory the test runs from
	dir := t.TempDir()
	writePartOrderFiles(t, dir, yamlLookupConfigRepo)

	// When: the configuration is loaded from its path and its terraform parts assembled
	doc := NewGitOpsDocument()
	if err := doc.LoadGitOpsFile(filepath.Join(dir, "example/deploy/configuration.yaml"), false, nil); err != nil {
		t.Fatalf("LoadGitOpsFile failed: %v", err)
	}
	doc.SetWrapperForConfiguration("terraform")
	if err := doc.LoadParts(); err != nil {
		t.Fatalf("LoadParts failed: %v", err)
	}

	// Then: both lookup forms resolve
	assertYamlLookupAccounts(t, doc)
}

func TestGitOpsDocument_YamlLookupFileFromClonedConfig_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "getYamlValue with a file reads it from the config repo root when the config is cloned from the desiredstate",
		CurrentImpl:     "CloneRepoAndLoad goes through the same LoadMetadata, so the handler is based on the clone's root",
		ExpectedOutcome: "shared/... resolves inside the cloned config repo",
		Rationale:       "Pipelines clone the config instead of passing -c, both have to agree",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: the same config repo as a git origin, and a desiredstate pointing at it
	origin := t.TempDir()
	writePartOrderFiles(t, origin, yamlLookupConfigRepo)
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"commit", "-q", "-m", "config"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		// keeps the developer's own git config out, commit signing breaks this
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=yago", "GIT_AUTHOR_EMAIL=yago@example.com",
			"GIT_COMMITTER_NAME=yago", "GIT_COMMITTER_EMAIL=yago@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	desiredState := map[string]interface{}{
		"schema":    "2.0.0",
		"namespace": "yago",
		"kind":      "DesiredState",
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"environments": map[string]interface{}{
					"dev": map[string]interface{}{
						"configurations": map[string]interface{}{
							"terraform": map[string]interface{}{
								"git": map[string]interface{}{
									"url":    "file://" + origin,
									"branch": "main",
									"path":   "example/deploy/configuration.yaml",
								},
							},
						},
					},
				},
			},
		},
	}

	// When: the configuration is cloned and its terraform parts assembled
	doc := NewGitOpsDocument()
	err := doc.CloneRepoAndLoad(desiredState, "desiredstate.content.environments.dev.configurations.terraform.git",
		filepath.Join(t.TempDir(), "config"), false, "", nil)
	if err != nil {
		t.Fatalf("CloneRepoAndLoad failed: %v", err)
	}
	doc.SetWrapperForConfiguration("terraform")
	if err := doc.LoadParts(); err != nil {
		t.Fatalf("LoadParts failed: %v", err)
	}

	// Then: both lookup forms resolve
	assertYamlLookupAccounts(t, doc)
}
