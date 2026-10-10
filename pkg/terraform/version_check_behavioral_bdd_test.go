package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func terraformReporting(t *testing.T, version string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"Terraform v" + version + "\"\necho \"on linux_amd64\"\n"
	if err := os.WriteFile(filepath.Join(bin, "terraform"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake terraform: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestService_CheckDependencies_VersionPin_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The installed terraform has to match every part of the version the desiredstate asks for",
		CurrentImpl:     "versionSatisfies compares the required parts against the installed ones, part by part",
		ExpectedOutcome: "A partial version like 1 or 1.6 accepts any release under it, a full version only accepts itself",
		Rationale:       "An exact pin that silently runs another patch release can plan or apply with a different terraform than the one reviewed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	scenarios := []struct {
		name      string
		installed string
		required  string
		wantErr   bool
	}{
		{"major only accepts any release under it", "1.6.5", "1", false},
		{"major and minor accepts any patch", "1.6.5", "1.6", false},
		{"exact version accepts itself", "1.6.3", "1.6.3", false},
		{"leading v is ignored", "1.6.3", "v1.6.3", false},
		{"exact version rejects another patch", "1.6.5", "1.6.3", true},
		{"exact version rejects extra parts", "1.6.3.4", "1.6.3", true},
		{"minor is compared as a number part, not a prefix", "1.60.0", "1.6", true},
		{"another minor is rejected", "1.7.0", "1.6", true},
		{"another major is rejected", "2.0.0", "1", true},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Given: a terraform binary on PATH and a version asked for by the desiredstate
			terraformReporting(t, scenario.installed)
			service := NewService(t.TempDir(), false)
			service.SetTerraformVersion(scenario.required)

			// When: the dependency check runs
			err := service.CheckDependencies()

			// Then: it fails only when a part the desiredstate spelled out differs
			if scenario.wantErr {
				if err == nil {
					t.Fatalf("installed %s passed a check for %s", scenario.installed, scenario.required)
				}
				for _, want := range []string{scenario.installed, scenario.required} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q doesn't name %s", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("installed %s failed a check for %s: %v", scenario.installed, scenario.required, err)
			}
		})
	}
}

func TestTerraformCommands_HoldTerraformToTheDesiredStateVersion_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A command checks terraform against the version in the desiredstate it loaded",
		CurrentImpl:     "Assembling hands the desiredstate version to the service the command builds",
		ExpectedOutcome: "A desiredstate pinned to 1.16.3 stops the command when 1.16.5 is installed",
		Rationale:       "The version used to stop at the parser, so the check only ever saw the default",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: terraform 1.16.5 and a desiredstate pinned to 1.16.3
	terraformReporting(t, "1.16.5")
	sourceDir := t.TempDir()
	configs := t.TempDir()
	writeTestFiles(t, configs, map[string]string{
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
      backends:
        eu-west-1: example/deploy/terraform/backends/eu-west-1.tfvars.json
      parts:
        terraform: example/deploy/terraform/terraform.yaml
`,
		"example/deploy/terraform/terraform.yaml":                 "---\nexample: value\n",
		"example/deploy/terraform/backends/eu-west-1.tfvars.json": `{"bucket": "example-state", "key": "example/terraform.tfstate", "region": "eu-west-1"}` + "\n",
	})
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
  tools:
    terraform: !!str 1.16.3
  configuration:
    all:
      terraform:
        git:
          url: git@example.com:example/configurations.git
          branch: main
          tag: ''
          path: example/deploy/configuration.yaml
`,
	})
	commands := map[string]func(*commonFlags) error{
		"output": func(f *commonFlags) error {
			return runOutput(f, "", true, false, false, false, false)
		},
		"destroy with a saved plan": func(f *commonFlags) error {
			return runDestroy(f, false, false, false, false, false, true, false, false)
		},
	}

	for name, run := range commands {
		t.Run(name, func(t *testing.T) {
			// When: the command runs
			err := run(&commonFlags{
				awsRegion:         "eu-west-1",
				workspace:         "default",
				desiredstateRoot:  filepath.Join(desiredStates, "example/deploy/desiredstate.yaml"),
				terraformSource:   sourceDir,
				configurationRoot: filepath.Join(configs, "example/deploy/configuration.yaml"),
				environment:       "all",
			})

			// Then: it stops on the version before running terraform
			if err == nil || !strings.Contains(err.Error(), "installed 1.16.5, required 1.16.3") {
				t.Fatalf("error = %v, want a version mismatch", err)
			}
		})
	}
}

func TestService_CheckDependencies_UnpinnedDefault_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A service that was never given a version accepts any terraform 1.x",
		CurrentImpl:     "DefaultTerraformVersion is the major only",
		ExpectedOutcome: "Commands run without a desiredstate version aren't held to a patch release nobody chose",
		Rationale:       "The default used to be 1.5.1 checked by major only, an exact check on it would break every unpinned run",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: terraform 1.9.8 and a service with no version set
	terraformReporting(t, "1.9.8")
	service := NewService(t.TempDir(), false)

	// When: the dependency check runs
	err := service.CheckDependencies()

	// Then: it passes
	if err != nil {
		t.Fatalf("unpinned check failed: %v", err)
	}
}
