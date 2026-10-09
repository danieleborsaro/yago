package terraform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleBackendDesiredState = `---
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
      self: example/cluster/desiredstate.yaml
      terraform: example/cluster/terraform.yaml
`

const exampleBackendDesiredStatePart = `---
desiredstate:
  tools:
    terraform: !!str 1.16.3
`

const exampleSharedBackend = `---
terraform_backend:
  eu-west-1:
    bucket: example-dev-statefile-euw1
    dynamodb_table: example-dev-statefile-euw1
    kms_key_id: 00000000-0000-0000-0000-000000000000
  key_vars:
    ecosystem: example-dev
`

func exampleBackendConfiguration(backends string) string {
	return `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/cluster/configuration.yaml
  wrappers:
    terraform:` + backends + `
      parts:
        eco: shared/example-dev/backend.yaml
        terraform: example/cluster/terraform.yaml
`
}

func assembleExampleBackend(t *testing.T, region string, configFiles map[string]string) (*TerraformAssembleResponse, error) {
	t.Helper()
	configs := t.TempDir()
	writeTestFiles(t, configs, configFiles)
	desiredStates := t.TempDir()
	writeTestFiles(t, desiredStates, map[string]string{
		"example/cluster/desiredstate.yaml": exampleBackendDesiredState,
		"example/cluster/terraform.yaml":    exampleBackendDesiredStatePart,
	})

	service := NewService(".", false)
	return service.AssembleTerraform(TerraformAssembleRequest{
		DesiredStateFile: filepath.Join(desiredStates, "example/cluster/desiredstate.yaml"),
		ConfigFile:       filepath.Join(configs, "example/cluster/configuration.yaml"),
		Environment:      "dev",
		AWSRegion:        region,
		TerraformSource:  t.TempDir(),
	})
}

func readJSONFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func TestTerraformBackend_GeneratedFromSharedDeclaration_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Generate the backend file from a terraform_backend declaration shared between configurations",
		CurrentImpl:     "Service.PostCache takes terraform_backend out of the assembled configuration and renders the state key",
		ExpectedOutcome: "The backend file has the region's bucket and the rendered key, and the var file has no terraform_backend",
		Rationale:       "Hand written backends files repeat the bucket and KMS key in every configuration, and a typo points Terraform at the wrong state",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a shared part with the region's bucket, and a configuration that only names itself
	files := map[string]string{
		"example/cluster/configuration.yaml": exampleBackendConfiguration(""),
		"shared/example-dev/backend.yaml":    exampleSharedBackend,
		"example/cluster/terraform.yaml": `---
example: value
terraform_backend:
  key_vars:
    resource_set: shared
    project: example
    component: cluster
`,
	}

	// When: the configuration is assembled for that region
	resp, err := assembleExampleBackend(t, "eu-west-1", files)

	// Then: the backend file is generated with the default key pattern
	if err != nil {
		t.Fatal(err)
	}
	backend := readJSONFile(t, resp.BackendFile)
	want := map[string]interface{}{
		"bucket":         "example-dev-statefile-euw1",
		"dynamodb_table": "example-dev-statefile-euw1",
		"kms_key_id":     "00000000-0000-0000-0000-000000000000",
		"key":            "example-dev/shared/example/cluster/terraform.tfstate",
		"region":         "eu-west-1",
		"encrypt":        true,
	}
	if len(backend) != len(want) {
		t.Errorf("backend = %v, want %v", backend, want)
	}
	for name, value := range want {
		if backend[name] != value {
			t.Errorf("backend %s = %v, want %v", name, backend[name], value)
		}
	}

	// Then: Terraform never sees the declaration as a variable
	config := readJSONFile(t, resp.ConfigurationFile)
	if _, exists := config["terraform_backend"]; exists {
		t.Error("terraform_backend is still in the var file")
	}
	if config["example"] != "value" {
		t.Errorf("configuration lost its own values: %v", config)
	}
}

func TestTerraformBackend_ListedBackendsFileWins_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Keep using a listed backends file when a terraform_backend declaration also exists",
		CurrentImpl:     "Service.applyBackendDeclaration skips generation when the parser already loaded a backends file for the region",
		ExpectedOutcome: "The backend file is the hand written one, and terraform_backend is still taken out of the var file",
		Rationale:       "Adding the shared declaration must not move the state of a configuration that hasn't migrated",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a configuration that still lists a backends file with a key the pattern can't make
	files := map[string]string{
		"example/cluster/configuration.yaml": exampleBackendConfiguration(`
      backends:
        eu-west-1: example/cluster/backends/eu-west-1.tfvars.json`),
		"shared/example-dev/backend.yaml":                exampleSharedBackend,
		"example/cluster/terraform.yaml":                 "---\nexample: value\n",
		"example/cluster/backends/eu-west-1.tfvars.json": `{"bucket": "example-legacy-state", "key": "legacy.tfstate", "region": "eu-west-1"}`,
	}

	// When: the configuration is assembled
	resp, err := assembleExampleBackend(t, "eu-west-1", files)

	// Then: the listed file is used as it is
	if err != nil {
		t.Fatal(err)
	}
	backend := readJSONFile(t, resp.BackendFile)
	if backend["bucket"] != "example-legacy-state" || backend["key"] != "legacy.tfstate" {
		t.Errorf("backend = %v, want the listed backends file", backend)
	}
	if _, exists := readJSONFile(t, resp.ConfigurationFile)["terraform_backend"]; exists {
		t.Error("terraform_backend is still in the var file")
	}
}

func TestTerraformBackend_NoBackendForRegion_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Stop when neither a backends file nor a terraform_backend entry covers the region",
		CurrentImpl:     "generateBackend returns nothing for an undeclared region and BackendManager.Cache refuses to continue",
		ExpectedOutcome: "Assemble fails and says how to give the region a backend",
		Rationale:       "Running Terraform without a backend file would start a fresh local state",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a declaration that only covers one region
	files := map[string]string{
		"example/cluster/configuration.yaml": exampleBackendConfiguration(""),
		"shared/example-dev/backend.yaml":    exampleSharedBackend,
		"example/cluster/terraform.yaml":     "---\nterraform_backend:\n  key: example.tfstate\n",
	}

	// When: the configuration is assembled for another region
	_, err := assembleExampleBackend(t, "us-east-2", files)

	// Then: it fails and names both ways out
	if err == nil || !strings.Contains(err.Error(), "list a backends file or declare terraform_backend") {
		t.Fatalf("err = %v, want it to name both ways out", err)
	}
}

func TestTerraformBackend_StateKey_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Render the state key from key, key_pattern and key_vars",
		CurrentImpl:     "backendKey prefers key, then fills key_pattern or the default pattern from key_vars, region and environment",
		ExpectedOutcome: "An explicit key is used as it is, a pattern is filled in, and a placeholder without a value is an error",
		Rationale:       "A half rendered key would silently create a new empty state",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	region := map[string]interface{}{"bucket": "example-dev-statefile-euw1"}
	identity := map[string]interface{}{
		"ecosystem": "example-dev", "resource_set": "shared", "project": "example", "component": "state",
	}
	tests := []struct {
		name        string
		declaration map[string]interface{}
		wantKey     string
		wantErr     string
	}{
		{
			name:        "default pattern",
			declaration: map[string]interface{}{"eu-west-1": region, "key_vars": identity},
			wantKey:     "example-dev/shared/example/state/terraform.tfstate",
		},
		{
			name: "explicit key for a state that doesn't fit",
			declaration: map[string]interface{}{
				"eu-west-1": region, "key_vars": identity, "key": "00000000-0000-0000-0000-000000000000.tfstate",
			},
			wantKey: "00000000-0000-0000-0000-000000000000.tfstate",
		},
		{
			name: "custom pattern with region and environment",
			declaration: map[string]interface{}{
				"eu-west-1": region, "key_vars": identity, "key_pattern": "{region}/{environment}/{project}.tfstate",
			},
			wantKey: "eu-west-1/dev/example.tfstate",
		},
		{
			name: "placeholders without a value",
			declaration: map[string]interface{}{
				"eu-west-1": region, "key_vars": map[string]interface{}{"ecosystem": "example-dev", "project": ""},
			},
			wantErr: "no value for component, project, resource_set",
		},
		{
			name: "custom pattern with a placeholder nobody set",
			declaration: map[string]interface{}{
				"eu-west-1": region, "key_vars": identity, "key_pattern": "{ecosystem}/{typo}.tfstate",
			},
			wantErr: "no value for typo",
		},
		{
			name:        "key_vars value that isn't a string",
			declaration: map[string]interface{}{"eu-west-1": region, "key_vars": map[string]interface{}{"project": 1}},
			wantErr:     "key_vars.project must be a string",
		},
		{
			name:        "empty key",
			declaration: map[string]interface{}{"eu-west-1": region, "key": ""},
			wantErr:     "key must be a non-empty string",
		},
		{
			name:        "region without a bucket",
			declaration: map[string]interface{}{"eu-west-1": map[string]interface{}{}, "key": "example.tfstate"},
			wantErr:     "eu-west-1.bucket is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: the declaration from the assembled configuration

			// When: the backend is generated for that region in dev
			backend, err := generateBackend(tt.declaration, "eu-west-1", "dev")

			// Then: the key or the error is the expected one
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if backend["key"] != tt.wantKey {
				t.Errorf("key = %v, want %q", backend["key"], tt.wantKey)
			}
		})
	}
}
