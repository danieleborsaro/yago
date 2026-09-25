package terraform

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheSecretInputsSeparatesReferencesAndClearsOldBindings(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "configuration.tfvars.json")
	input := `{"workspace":"example","secret_variables":{"auth":{"secret_id":"existing","json_key":"token"}}}`
	if err := os.WriteFile(config, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cacheSecretInputs(config, dir, "example", "eu-west-1", true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var vars map[string]interface{}
	if err := json.Unmarshal(data, &vars); err != nil {
		t.Fatal(err)
	}
	if len(vars) != 1 || vars["workspace"] != "example" {
		t.Fatalf("unexpected Terraform inputs: %v", vars)
	}
	manifest, err := readSecretManifest(filepath.Join(dir, secretManifestName))
	if err != nil || manifest.Variables["auth"].SecretID != "existing" || manifest.AWSProfile != "example" || manifest.AWSRegion != "eu-west-1" {
		t.Fatalf("unexpected reference manifest: %+v, %v", manifest, err)
	}
	if err := cacheSecretInputs(config, dir, "", "", true); err != nil {
		t.Fatal(err)
	}
	manifest, err = readSecretManifest(filepath.Join(dir, secretManifestName))
	if err != nil || len(manifest.Variables) != 0 {
		t.Fatalf("stale secret references survived reassembly: %+v, %v", manifest, err)
	}
}

func TestCacheSecretInputsRejectsMalformedAndConflictingBindings(t *testing.T) {
	for _, input := range []string{
		`{"secret_variables":{"auth":{"secret_id":"existing","typo":"field"}}}`,
		`{"secret_variables":{"auth":{"json_key":"token"}}}`,
		`{"secret_variables":"existing"}`,
		`{"auth":"static-value","secret_variables":{"auth":{"secret_id":"existing"}}}`,
	} {
		dir := t.TempDir()
		config := filepath.Join(dir, "config.json")
		if err := os.WriteFile(config, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if err := cacheSecretInputs(config, dir, "", "", true); err == nil {
			t.Fatalf("invalid bindings accepted: %s", input)
		}
	}
}

func fakeTerraformForSecrets(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
set -eu
if [ "$1" = validate ]; then exit 0; fi
[ "$TF_VAR_auth" = '{"token":"runtime-only-token"}' ]
[ "$AWS_PROFILE" = review-profile ]
[ "$AWS_REGION" = eu-west-1 ]
printf '%s\n' "$TF_VAR_auth" 'runtime-only-token'
`
	if err := os.WriteFile(filepath.Join(dir, "terraform"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TF_VAR_auth", "parent-value")
	t.Setenv("IS_DRY_RUN", "0")
	return dir
}

func TestTerraformSecretEnvironmentAndSavedPlanReferences(t *testing.T) {
	dir := fakeTerraformForSecrets(t)
	if err := os.Mkdir(filepath.Join(dir, ".gitops"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := secretInputManifest{Version: 1, Variables: map[string]SecretVariableBinding{"auth": {SecretID: "original"}}}
	if err := writeSecretManifest(defaultSecretManifest(dir), manifest); err != nil {
		t.Fatal(err)
	}
	reader := &fakeSecretValueReader{values: map[secretReadCall]string{{name: "original"}: `{"token":"runtime-only-token"}`}}
	svc := NewService(dir, false)
	svc.SetAWSProfile("review-profile")
	svc.SetAWSRegion("eu-west-1")
	svc.secretReader = reader
	var shown bytes.Buffer
	svc.stdout, svc.stderr = &shown, &shown
	resp, err := svc.Plan(PlanRequest{WorkingDir: dir, OutFile: "saved-plan"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resp.Output, "runtime-only-token") || !strings.Contains(resp.Output, "[REDACTED]") {
		t.Fatalf("Terraform output was not redacted: %s", resp.Output)
	}
	if strings.Contains(shown.String(), "runtime-only-token") || !strings.Contains(shown.String(), "[REDACTED]") {
		t.Fatalf("shown Terraform output was not redacted: %s", shown.String())
	}
	if os.Getenv("TF_VAR_auth") != "parent-value" {
		t.Fatal("secret resolution changed parent process environment")
	}
	data, err := os.ReadFile(filepath.Join(dir, "saved-plan"+planSecretSuffix))
	if err != nil || strings.Contains(string(data), "runtime-only-token") {
		t.Fatalf("plan references missing or contain resolved values: %v", err)
	}
	manifest.Variables["auth"] = SecretVariableBinding{SecretID: "other-environment"}
	if err := writeSecretManifest(defaultSecretManifest(dir), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ApplyRequest{WorkingDir: dir, PlanFile: "saved-plan"}); err != nil {
		t.Fatal(err)
	}
	if len(reader.calls) != 2 || reader.calls[1].name != "original" {
		t.Fatalf("apply did not resolve the plan's original secret references: %+v", reader.calls)
	}
	if _, err := svc.Apply(ApplyRequest{WorkingDir: dir, PlanFile: "legacy-plan"}); err == nil {
		t.Fatal("apply should reject a plan missing its secret references")
	}
	reader.calls = nil
	svc.SetDryRun(true)
	if _, err := svc.Plan(PlanRequest{WorkingDir: dir, OutFile: "dry-plan"}); err != nil {
		t.Fatal(err)
	}
	if len(reader.calls) != 0 {
		t.Fatal("dry run contacted the secret reader")
	}
	if _, err := os.Stat(filepath.Join(dir, "dry-plan"+planSecretSuffix)); !os.IsNotExist(err) {
		t.Fatal("dry run wrote plan references")
	}
	svc.SetDryRun(false)
	if _, err := svc.ValidateTerraform(ValidateRequest{WorkingDir: dir}); err != nil {
		t.Fatal(err)
	}
	if len(reader.calls) != 0 {
		t.Fatal("validation resolved secrets")
	}
}
