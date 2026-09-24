package secretsmanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	awssecretsmanager "github.com/danieleborsaro/yago/pkg/aws"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	logging.SetOutput(&logs)
	t.Cleanup(func() { logging.SetOutput(os.Stderr) })
	return &logs
}

func TestSmCommandTree(t *testing.T) {
	cmd := NewSecretManagerCommand()
	if cmd.Use != "sm" || strings.Join(cmd.Aliases, ",") != "secretsmanager" {
		t.Fatalf("unexpected command: %s %v", cmd.Use, cmd.Aliases)
	}

	got := map[string]string{}
	for _, sub := range cmd.Commands() {
		got[sub.Name()] = strings.Join(sub.Aliases, ",")
	}
	want := map[string]string{"assemble": "a", "plan": "p", "create": "c", "validate": "v", "destroy": "Sdest"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got subcommands %v, want %v", got, want)
	}
}

func TestFactoryRegistration(t *testing.T) {
	factory := &SecretManagerFactory{}
	if factory.Name() != "secretsmanager" {
		t.Fatalf("expected 'secretsmanager', got '%s'", factory.Name())
	}
	if factory.CreateParser("all", nil) == nil || factory.CreateService(".", false) == nil || factory.CreateCLICommand() == nil {
		t.Fatal("expected the factory to create a parser, a service and a command")
	}
}

func TestSmFlags(t *testing.T) {
	common := map[string]string{
		"aws-profile": "p", "aws-region": "r", "desiredstate-root": "d", "configuration-root": "c", "environment": "e",
	}
	extra := map[string]map[string]string{
		"assemble": {"cache-dir": "C"},
		"plan":     {},
		"create":   {"dry-run": "n"},
		"validate": {},
		"destroy":  {"force": "f", "dry-run": ""},
	}

	for _, sub := range NewSecretManagerCommand().Commands() {
		t.Run(sub.Name(), func(t *testing.T) {
			want := map[string]string{}
			for name, short := range common {
				want[name] = short
			}
			for name, short := range extra[sub.Name()] {
				want[name] = short
			}

			for name, short := range want {
				flag := sub.Flags().Lookup(name)
				if flag == nil || flag.Shorthand != short {
					t.Errorf("expected flag --%s with shorthand %q, got %+v", name, short, flag)
				}
			}
			usages := strings.Split(strings.TrimRight(sub.Flags().FlagUsages(), "\n"), "\n")
			if len(usages) != len(want) {
				t.Errorf("expected %d flags, got:\n%s", len(want), sub.Flags().FlagUsages())
			}
		})
	}
}

func TestSmFlagDefaults(t *testing.T) {
	t.Setenv("AWS_PROFILE", "example")
	t.Setenv("IS_DRY_RUN", "1")

	cmd := NewSecretManagerCommand()
	create, _, _ := cmd.Find([]string{"create"})
	if value := create.Flag("aws-profile").DefValue; value != "example" {
		t.Errorf("expected the profile to default to AWS_PROFILE, got %q", value)
	}
	if value := create.Flag("dry-run").DefValue; value != "true" {
		t.Errorf("expected dry-run to default to IS_DRY_RUN=1, got %q", value)
	}
	if value := create.Flag("environment").DefValue; value != "all" {
		t.Errorf("expected the environment to default to all, got %q", value)
	}

	destroy, _, _ := cmd.Find([]string{"destroy"})
	if value := destroy.Flag("dry-run").DefValue; value != "false" {
		t.Errorf("destroy's dry run doesn't read IS_DRY_RUN, got %q", value)
	}
}

func executeSm(args ...string) error {
	cmd := NewSecretManagerCommand()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

func TestSmChecksItsInputs(t *testing.T) {
	captureLogs(t)
	dir := t.TempDir()
	desiredState := filepath.Join(dir, "desiredstate.yaml")
	if err := os.WriteFile(desiredState, []byte("---\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"region is required", []string{"plan", "-d", desiredState}, `"aws-region" not set`},
		{"desired state is required", []string{"plan", "-r", "eu-west-1"}, `"desiredstate-root" not set`},
		{"empty region", []string{"plan", "-r", "", "-d", desiredState}, "AWS Region is not specified"},
		{"missing desired state", []string{"validate", "-r", "eu-west-1", "-d", filepath.Join(dir, "missing.yaml")}, "is not readable"},
		{"missing configuration", []string{"create", "-r", "eu-west-1", "-d", desiredState, "-c", filepath.Join(dir, "missing.yaml")}, "Config file"},
		{"cache dir is required", []string{"assemble", "-r", "eu-west-1", "-d", desiredState}, `"cache-dir" not set`},
		{"missing cache dir", []string{"assemble", "-r", "eu-west-1", "-d", desiredState, "-C", filepath.Join(dir, "missing")}, "Cache directory"},
		{"no removed commands", []string{"list"}, "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := executeSm(tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

type fakeSecretManager struct {
	existing    map[string]bool
	validations map[string]*awssecretsmanager.SecretValidationResult
	destroyErrs map[string]error

	created   []awssecretsmanager.CreateRequest
	destroyed []string
}

func (f *fakeSecretManager) AccountID() (string, error) {
	return "123456789012", nil
}

func (f *fakeSecretManager) Create(req awssecretsmanager.CreateRequest) (*awssecretsmanager.SecretCreationResult, bool, error) {
	if !req.IsCreatedHere {
		if !f.existing[req.Name] {
			return nil, false, errors.New("secret '" + req.Name + "' does not exist but isCreatedHere=False")
		}
		return &awssecretsmanager.SecretCreationResult{Name: req.Name}, false, nil
	}
	if f.existing[req.Name] {
		return nil, false, nil
	}
	f.created = append(f.created, req)
	return &awssecretsmanager.SecretCreationResult{Name: req.Name, ARN: "arn:" + req.Name}, true, nil
}

func (f *fakeSecretManager) Validate(secretName string, expectedKeys []string, versions []string) (*awssecretsmanager.SecretValidationResult, error) {
	if result, exists := f.validations[secretName]; exists {
		return result, nil
	}
	return &awssecretsmanager.SecretValidationResult{IsExpectedKeysOk: true, IsStoredKeysOk: true, ARN: "arn:" + secretName}, nil
}

func (f *fakeSecretManager) Destroy(secretName string) error {
	if err := f.destroyErrs[secretName]; err != nil {
		return err
	}
	f.destroyed = append(f.destroyed, secretName)
	return nil
}

const libConfiguration = `---
project_properties:
  company_name_short: foo
  accounts_coding:
    "123456789012":
      class: management
      name: management
      name_canonical: Management
      name_encoded: mgmt
  infra_environment: management
  project_name_long: example project
  resource_set_long: example resource set
  role: secrets
  owner: team@example.com
  cost_centre: cc-1234
  compliance: internal
  custom_tags_verbatim:
    ManagedBy: yago
secrets:
  eu-west-1:
    new_one:
      is_created_here: true
      name: example/new
      keys:
        token_path: token
    existing_one:
      is_created_here: true
      name: example/existing
    read_only:
      is_created_here: false
      name: example/read-only
`

func newTestLib(t *testing.T, fake *fakeSecretManager, isDryRun bool) *Lib {
	t.Helper()
	lib := &Lib{awsRegion: "eu-west-1", isDryRun: isDryRun, secMan: fake}
	if err := lib.SetConfiguration(parseConfiguration(t, libConfiguration)); err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestLib_CreateOnlyCreatesMissingSecretsCreatedHere(t *testing.T) {
	logs := captureLogs(t)
	fake := &fakeSecretManager{existing: map[string]bool{"example/existing": true, "example/read-only": true}}

	if err := newTestLib(t, fake, false).Create(); err != nil {
		t.Fatal(err)
	}

	if len(fake.created) != 1 || fake.created[0].Name != "example/new" {
		t.Fatalf("expected only example/new to be created, got %+v", fake.created)
	}
	request := fake.created[0]
	if request.SecretTags["Foo:Environment:ResourceType"] != ":AWS::SecretsManager::Secret" ||
		request.KmsKeyTags["Foo:Environment:ResourceType"] != ":AWS::KMS::Key" ||
		request.SecretTags["Name"] != "example/new" || request.SecretTags["ManagedBy"] != "yago" {
		t.Fatalf("unexpected tags:\n%s\n\n%s", awssecretsmanager.FormatTags(request.SecretTags), awssecretsmanager.FormatTags(request.KmsKeyTags))
	}
	if !strings.Contains(logs.String(), "Created 1 secrets") {
		t.Fatalf("expected the count in the logs:\n%s", logs.String())
	}
}

func TestLib_CreateFailsForAMissingSecretNotCreatedHere(t *testing.T) {
	captureLogs(t)
	fake := &fakeSecretManager{existing: map[string]bool{"example/existing": true}}

	err := newTestLib(t, fake, false).Create()
	if err == nil || !strings.Contains(err.Error(), "example/read-only") {
		t.Fatalf("expected an error for the missing read-only secret, got %v", err)
	}
}

func TestLib_PlanCountsTheSecretsToCreate(t *testing.T) {
	logs := captureLogs(t)
	fake := &fakeSecretManager{existing: map[string]bool{"example/existing": true, "example/read-only": true}}

	if err := newTestLib(t, fake, true).Plan(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "To create 1 secrets") || !strings.Contains(logs.String(), "arn:example/new") {
		t.Fatalf("expected the plan to list example/new:\n%s", logs.String())
	}
}

func TestLib_CreateNeedsTaggingProperties(t *testing.T) {
	captureLogs(t)
	lib := &Lib{awsRegion: "eu-west-1", secMan: &fakeSecretManager{}}
	configuration := parseConfiguration(t, libConfiguration)
	delete(configuration["project_properties"].(map[string]interface{}), "owner")
	if err := lib.SetConfiguration(configuration); err != nil {
		t.Fatal(err)
	}

	err := lib.Create()
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected an error for the missing owner, got %v", err)
	}
}

func TestLib_NoSecretsForTheRegion(t *testing.T) {
	logs := captureLogs(t)
	fake := &fakeSecretManager{}
	lib := newTestLib(t, fake, false)
	lib.awsRegion = "us-east-1"

	if err := lib.Create(); err != nil {
		t.Fatal(err)
	}
	if err := lib.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 0 || !strings.Contains(logs.String(), "No secrets configured for region us-east-1") {
		t.Fatalf("expected a warning and nothing created:\n%s", logs.String())
	}
}

func TestLib_ValidateFailsForInvalidSecrets(t *testing.T) {
	logs := captureLogs(t)
	fake := &fakeSecretManager{validations: map[string]*awssecretsmanager.SecretValidationResult{
		"example/new": {IsExpectedKeysOk: true, IsStoredKeysOk: false},
	}}

	err := newTestLib(t, fake, false).Validate()
	if err == nil || !strings.Contains(err.Error(), "1 of 3 secrets are not valid: example/new") {
		t.Fatalf("expected the invalid secret to fail validation, got %v", err)
	}
	if !strings.Contains(logs.String(), "Secret example/new is not valid: stored keys mismatch") ||
		!strings.Contains(logs.String(), "Secret example/existing is valid") {
		t.Fatalf("unexpected logs:\n%s", logs.String())
	}
}

func TestDestroySecrets(t *testing.T) {
	for _, tc := range []struct {
		name          string
		input         string
		isForce       bool
		isDryRun      bool
		destroyErrs   map[string]error
		wantErr       string
		wantDestroyed []string
	}{
		{name: "confirmed", input: "y\n", wantDestroyed: []string{"example/existing", "example/new"}},
		{name: "declined", input: "n\n"},
		{name: "default is no", input: "\n"},
		{name: "no input", input: "", wantErr: "needs confirmation"},
		{name: "forced", isForce: true, wantDestroyed: []string{"example/existing", "example/new"}},
		{name: "dry run skips the prompt", isDryRun: true, wantDestroyed: []string{"example/existing", "example/new"}},
		{name: "failures are reported", isForce: true, destroyErrs: map[string]error{"example/new": errors.New("destruction blocked")},
			wantErr: "failed to destroy 1 of 2 secrets", wantDestroyed: []string{"example/existing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			fake := &fakeSecretManager{destroyErrs: tc.destroyErrs}

			err := destroySecrets(newTestLib(t, fake, tc.isDryRun), strings.NewReader(tc.input), tc.isForce, tc.isDryRun)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected an error containing %q, got %v", tc.wantErr, err)
			}

			sort.Strings(fake.destroyed)
			if strings.Join(fake.destroyed, ",") != strings.Join(tc.wantDestroyed, ",") {
				t.Fatalf("destroyed %v, want %v", fake.destroyed, tc.wantDestroyed)
			}

			if tc.isDryRun && (strings.Contains(logs.String(), "Successfully destroyed") || !strings.Contains(logs.String(), "2/2 would succeed")) {
				t.Fatalf("unexpected dry run logs:\n%s", logs.String())
			}
		})
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

const smDesiredState = `---
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
        watch:
          - example/secrets/create/{**,.}/*
    parts:
      self: example/secrets/create/desiredstate.yaml
  configuration:
    all:
      terraform:
        git:
          url: git@example.com:example/configurations.git
          branch: main
          tag: ''
          path: example/secrets/create/configuration.yaml
          watch:
            - example/{**,.}/*
`

func writeSecretsFixture(t *testing.T, configurationFiles map[string]string) (string, string) {
	t.Helper()
	desiredStates := t.TempDir()
	configurations := t.TempDir()
	writeFiles(t, desiredStates, map[string]string{"example/secrets/create/desiredstate.yaml": smDesiredState})
	writeFiles(t, configurations, configurationFiles)
	return filepath.Join(desiredStates, "example/secrets/create/desiredstate.yaml"),
		filepath.Join(configurations, "example/secrets/create/configuration.yaml")
}

func TestSmCommandsDryRun(t *testing.T) {
	captureLogs(t)
	t.Setenv("IS_DRY_RUN", "")
	desiredState, configuration := writeSecretsFixture(t, map[string]string{
		"example/secrets/create/configuration.yaml": `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/secrets/create/configuration.yaml
  wrappers:
    terraform:
      parts:
        terraform: example/secrets/create/terraform/terraform.yaml
`,
		"example/secrets/create/terraform/terraform.yaml": strings.Replace(libConfiguration, "project_properties:", "project_properties_env:", 1),
	})

	isDryRun := []bool{}
	original := newSecretManager
	t.Cleanup(func() { newSecretManager = original })
	newSecretManager = func(awsProfile, awsRegion string, dryRun bool) (secretManager, error) {
		isDryRun = append(isDryRun, dryRun)
		return &fakeSecretManager{existing: map[string]bool{"example/read-only": true}}, nil
	}

	for _, tc := range []struct {
		args         []string
		wantIsDryRun bool
	}{
		{args: []string{"plan"}, wantIsDryRun: true},
		{args: []string{"create"}, wantIsDryRun: false},
		{args: []string{"create", "-n"}, wantIsDryRun: true},
		{args: []string{"validate"}, wantIsDryRun: false},
		{args: []string{"destroy", "-f"}, wantIsDryRun: false},
		{args: []string{"destroy", "--dry-run"}, wantIsDryRun: true},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			isDryRun = nil
			args := append(tc.args, "-r", "eu-west-1", "-d", desiredState, "-c", configuration)
			if err := executeSm(args...); err != nil {
				t.Fatal(err)
			}
			if len(isDryRun) != 1 || isDryRun[0] != tc.wantIsDryRun {
				t.Fatalf("expected one secret manager with dry run %v, got %v", tc.wantIsDryRun, isDryRun)
			}
		})
	}
}

func TestAssembleSecrets(t *testing.T) {
	captureLogs(t)
	desiredState, configuration := writeSecretsFixture(t, map[string]string{
		"example/secrets/create/configuration.yaml": `---
schema: 2.0.0
namespace: yago
kind: Configuration
configuration:
  meta:
    parts:
      self: example/secrets/create/configuration.yaml
  wrappers:
    terraform:
      parts:
        global: shared/global/project-properties.yaml
        project: example/shared/project-properties.yaml
        terraform: example/secrets/create/terraform/terraform.yaml
        secrets: example/shared/secrets.yaml
`,
		"shared/global/project-properties.yaml": `---
project_properties_global:
  company_name_short: foo
  custom_tags_verbatim:
    ManagedBy: terraform
`,
		"example/shared/project-properties.yaml": `---
project_properties_proj:
  owner: team@example.com
  role: app
`,
		"example/secrets/create/terraform/terraform.yaml": `---
project_properties_env:
  role: secrets
  custom_tags_verbatim:
    ManagedBy: yago
`,
		"example/shared/secrets.yaml": `---
secrets:
  eu-west-1:
    database:
      is_created_here: true
      name: example/app/database
`,
	})

	cacheDir := t.TempDir()
	response, err := NewService(".", false).AssembleSecrets(desiredState, configuration, "all", cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]interface{}{
		"company_name_short":   "foo",
		"owner":                "team@example.com",
		"role":                 "secrets",
		"custom_tags_verbatim": map[string]interface{}{"ManagedBy": "yago"},
	}
	if got := response.AssembledConfigurationContent["project_properties"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got project_properties %v, want %v", got, want)
	}

	cached, err := os.ReadFile(filepath.Join(cacheDir, "configuration.tfvars.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cachedConfiguration map[string]interface{}
	if err := json.Unmarshal(cached, &cachedConfiguration); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cachedConfiguration["project_properties"], want) || cachedConfiguration["secrets"] == nil {
		t.Fatalf("unexpected cached configuration: %s", cached)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "desiredstate.tfvars.json")); err != nil {
		t.Fatalf("the desired state was not cached: %v", err)
	}

	secrets, err := ParseSecrets(response.AssembledConfigurationContent)
	if err != nil || secrets["eu-west-1"]["database"].Name != "example/app/database" {
		t.Fatalf("unexpected secrets: %+v, %v", secrets, err)
	}
}
