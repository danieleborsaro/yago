package terraform

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"gopkg.in/yaml.v3"
)

const (
	exampleProjectKeyVars = `---
terraform_backend:
  key_vars:
    resource_set: shared
    project: example
    component: cluster
`
	exampleListedBackends = `
      backends:
        eu-central-1: example/cluster/backends/eu-central-1.tfvars.json`
	exampleListedBackendPath = "example/cluster/backends/eu-central-1.tfvars.json"
	exampleListedBackendFile = `{"bucket": "example-dev-statefile-euc1", "key": "old/terraform.tfstate"}`
	exampleGeneratedName     = "foo-m-management-inception-state--dev-euw1-b74d7cd3c2d40d0494c3"
	exampleKeyARN            = "arn:aws:kms:eu-west-1:123456789012:key/00000000-0000-0000-0000-000000000000"
)

type fakeStateBucket struct {
	name         string
	result       *aws.StateBucketResult
	deletableErr error
	calls        []string
	// the plan it handed out and the one it was told to apply, which have to be the same one
	planned, applied *aws.StateBucketPlan
}

func (f *fakeStateBucket) Name() (string, error) { return f.name, nil }

func (f *fakeStateBucket) PlanCreate() (*aws.StateBucketPlan, error) {
	f.calls = append(f.calls, "PlanCreate")
	f.planned = &aws.StateBucketPlan{Changes: []string{"create state bucket example", "create a KMS key", "tag it with\nName = example"}}
	return f.planned, nil
}

func (f *fakeStateBucket) PlanUpdate() (*aws.StateBucketPlan, error) {
	f.calls = append(f.calls, "PlanUpdate")
	f.planned = &aws.StateBucketPlan{Changes: []string{"update state bucket example", "CHANGE ITS ENCRYPTION", "tag it with\nName = example"}}
	return f.planned, nil
}

func (f *fakeStateBucket) Apply(plan *aws.StateBucketPlan) (*aws.StateBucketResult, error) {
	f.calls = append(f.calls, "Apply")
	f.applied = plan
	return f.result, nil
}

func (f *fakeStateBucket) EnsureDeletable() error {
	f.calls = append(f.calls, "EnsureDeletable")
	return f.deletableErr
}

func (f *fakeStateBucket) Delete() error {
	f.calls = append(f.calls, "Delete")
	return nil
}

// what the command handed to the AWS layer, so a test can check it without AWS
type stateBucketRequest struct {
	spec    aws.StateBucketSpec
	profile string
}

func useFakeStateBucket(t *testing.T, bucket *fakeStateBucket) *stateBucketRequest {
	t.Helper()
	request := &stateBucketRequest{}
	original := newStateBucket
	newStateBucket = func(spec aws.StateBucketSpec, awsProfile string) (stateBucket, error) {
		*request = stateBucketRequest{spec: spec, profile: awsProfile}
		return bucket, nil
	}
	t.Cleanup(func() { newStateBucket = original })
	return request
}

type backendCommandRun struct {
	stdout string
	prompt string
	err    error
}

func runBackendCommand(t *testing.T, configFiles map[string]string, stdin string, args ...string) backendCommandRun {
	t.Helper()
	return runBackendCommandWithDryRunEnv(t, "", configFiles, stdin, args...)
}

func writeBackendTestFiles(t *testing.T, configFiles map[string]string) (desiredState, configuration string) {
	t.Helper()
	configs, desiredStates := t.TempDir(), t.TempDir()
	writeTestFiles(t, configs, configFiles)
	writeTestFiles(t, desiredStates, map[string]string{
		"example/cluster/desiredstate.yaml": exampleBackendDesiredState,
		"example/cluster/terraform.yaml":    exampleBackendDesiredStatePart,
	})
	return filepath.Join(desiredStates, "example/cluster/desiredstate.yaml"), filepath.Join(configs, "example/cluster/configuration.yaml")
}

// no terraform source is passed, the backend commands assemble without one, and the two variables a run reads or
// exports are pinned so nothing leaks in from the shell or out into the next test
func runBackendCommandWithDryRunEnv(t *testing.T, isDryRunEnv string, configFiles map[string]string, stdin string, args ...string) backendCommandRun {
	t.Helper()
	t.Setenv("IS_DRY_RUN", isDryRunEnv)
	t.Setenv("AWS_PROFILE", "")
	desiredState, configuration := writeBackendTestFiles(t, configFiles)

	cmd := NewTerraformCommand()
	var stdout, prompt bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&prompt)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{"backend"}, append(args, "-e", "dev", "-d", desiredState, "-c", configuration)...))
	err := cmd.Execute()
	return backendCommandRun{stdout: stdout.String(), prompt: prompt.String(), err: err}
}

// the backend file an assemble writes for init, which is what describe has to agree with
func assembleBackendForInit(t *testing.T, region, awsProfile string, configFiles map[string]string) map[string]interface{} {
	t.Helper()
	t.Setenv("AWS_PROFILE", "")
	desiredState, configuration := writeBackendTestFiles(t, configFiles)
	response, err := NewService(".", false).AssembleTerraform(TerraformAssembleRequest{
		DesiredStateFile: desiredState, ConfigFile: configuration, Environment: "dev",
		AWSProfile: awsProfile, AWSRegion: region, TerraformSource: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return readJSONFile(t, response.BackendFile)
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	logging.SetOutput(&logs)
	t.Cleanup(func() { logging.SetOutput(os.Stderr) })
	return &logs
}

func sharedBackendFiles() map[string]string {
	return map[string]string{
		"example/cluster/configuration.yaml": exampleBackendConfiguration(""),
		"shared/example-dev/backend.yaml":    exampleSharedBackend,
		"example/cluster/terraform.yaml":     exampleProjectKeyVars,
	}
}

// eu-west-1 generated from the shared declaration, eu-central-1 still on a backends file
func mixedBackendFiles() map[string]string {
	files := sharedBackendFiles()
	files["example/cluster/configuration.yaml"] = exampleBackendConfiguration(exampleListedBackends)
	files[exampleListedBackendPath] = exampleListedBackendFile
	return files
}

func TestTerraformBackend_ListCoversDeclaredAndListedRegions_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf backend list prints one row per region, whether its backend is generated or a listed backends file",
		CurrentImpl:     "BackendSet.Regions merges the terraform_backend regions with wrappers.terraform.backends, and each resolves as it would for init",
		ExpectedOutcome: "A row for the declared region with its rendered key, and one row for the region that is both listed and declared, with the listed file's bucket and key, and no -r needed",
		Rationale:       "While we migrate, a configuration has both kinds, and a list that only knew one of them would hide a bucket",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: eu-west-1 declared in a shared part, and eu-central-1 on a backends file as well as declared
	files := mixedBackendFiles()
	files["shared/example-dev/backend.yaml"] = exampleSharedBackend + "  eu-central-1:\n    bucket: example-dev-not-yet-euc1\n"

	// When: list runs without a region, as a table and as JSON
	table := runBackendCommand(t, files, "", "list")
	asJSON := runBackendCommand(t, files, "", "list", "--format", "json")

	// Then: both regions are there, in order, each with its own bucket and key
	if table.err != nil || asJSON.err != nil {
		t.Fatalf("table = %v, json = %v", table.err, asJSON.err)
	}
	lines := strings.Split(strings.TrimSpace(table.stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("output = %q, want a header and two rows", table.stdout)
	}
	wantRows := [][]string{
		{"eu-central-1", "example-dev-statefile-euc1", "old/terraform.tfstate"},
		{"eu-west-1", "example-dev-statefile-euw1", "example-dev/shared/example/cluster/terraform.tfstate"},
	}
	for i, want := range wantRows {
		if got := strings.Fields(lines[i+1]); !slices.Equal(got, want) {
			t.Errorf("row %d = %v, want %v", i+1, got, want)
		}
	}
	var backends []map[string]interface{}
	if err := json.Unmarshal([]byte(asJSON.stdout), &backends); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, asJSON.stdout)
	}
	if len(backends) != 2 || backends[0]["region"] != "eu-central-1" || backends[1]["kms_key_id"] != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("backends = %v", backends)
	}

	// a listed backends file that leaves the key out gets an empty cell, the table is for people to read
	files[exampleListedBackendPath] = `{"bucket": "example-dev-statefile-euc1"}`
	keyless := runBackendCommand(t, files, "", "list")
	if keyless.err != nil || strings.Contains(keyless.stdout, "<nil>") || !strings.Contains(keyless.stdout, "example-dev-statefile-euc1") {
		t.Errorf("err = %v, table = %q", keyless.err, keyless.stdout)
	}
}

func TestTerraformBackend_DescribePrintsTheBackendInitWouldUse_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf backend describe prints exactly the backend an assemble writes for init, for a generated region, a listed one, and a key that looks the AWS profile up",
		CurrentImpl:     "BackendSet.Backend and PostCache both go through resolveBackend, and Service.Backends exports the profile as an assemble for init does",
		ExpectedOutcome: "The JSON on stdout equals the backend file assemble writes for the same region and profile, YAML carries the same values, and the table always has a region",
		Rationale:       "The bucket commands act on what this resolves, so it can't be allowed to drift from what terraform init gets",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: one region generated from the declaration, one on a listed backends file that names no region,
	// and a configuration whose state key carries whatever profile it is run with
	keyedByProfile := sharedBackendFiles()
	keyedByProfile["example/cluster/terraform.yaml"] = strings.Replace(exampleProjectKeyVars, "resource_set: shared", `resource_set: "[[gitops.getEnvValue(AWS_PROFILE)]]"`, 1)

	cases := []struct {
		name    string
		files   map[string]string
		region  string
		profile string
		wantKey string
	}{
		{name: "generated", files: mixedBackendFiles(), region: "eu-west-1", wantKey: "example-dev/shared/example/cluster/terraform.tfstate"},
		{name: "listed", files: mixedBackendFiles(), region: "eu-central-1", wantKey: "old/terraform.tfstate"},
		{name: "keyed by profile", files: keyedByProfile, region: "eu-west-1", profile: "example-profile",
			wantKey: "example-dev/example-profile/example/cluster/terraform.tfstate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files, region := tc.files, tc.region
			args := []string{"describe", "-r", region}
			if tc.profile != "" {
				args = append(args, "-p", tc.profile)
			}

			// When: the region is described, and assembled for init
			described := runBackendCommand(t, files, "", args...)
			asYAML := runBackendCommand(t, files, "", append(args, "--format", "yaml")...)
			asTable := runBackendCommand(t, files, "", append(args, "--format", "table")...)
			want := assembleBackendForInit(t, region, tc.profile, files)

			// Then: describe and the backend file agree
			if described.err != nil || asYAML.err != nil || asTable.err != nil {
				t.Fatalf("describe = %v, yaml = %v, table = %v", described.err, asYAML.err, asTable.err)
			}
			if want["key"] != tc.wantKey {
				t.Fatalf("assembled key = %v, want %s", want["key"], tc.wantKey)
			}
			var fromJSON, fromYAML map[string]interface{}
			if err := json.Unmarshal([]byte(described.stdout), &fromJSON); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, described.stdout)
			}
			if err := yaml.Unmarshal([]byte(asYAML.stdout), &fromYAML); err != nil {
				t.Fatalf("stdout is not YAML: %v\n%s", err, asYAML.stdout)
			}
			if !reflect.DeepEqual(fromJSON, want) || !reflect.DeepEqual(fromYAML, want) {
				t.Errorf("json = %v, yaml = %v, backend file = %v", fromJSON, fromYAML, want)
			}
			if rows := strings.Split(strings.TrimSpace(asTable.stdout), "\n"); len(rows) != 2 || !strings.HasPrefix(rows[1], region) {
				t.Errorf("table = %q, want one row starting with %s", asTable.stdout, region)
			}
		})
	}
}

func TestTerraformBackend_DescribeAndListStopOnWhatInitCouldNotUse_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "describe and list stop on an unknown format, a region with no backend, and a declared region with no bucket pinned, but not on another region's broken entry",
		CurrentImpl:     "validateFormat runs before the assemble, BackendSet.Backend resolves one region by the rule generateBackend applies for init",
		ExpectedOutcome: "An error and no backend on stdout, and a healthy region still described next to a broken one",
		Rationale:       "A typo in --format shouldn't quietly print the default, and a backend init would refuse shouldn't be shown as if it were fine",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	unpinned := sharedBackendFiles()
	unpinned["shared/example-dev/backend.yaml"] = "---\nterraform_backend:\n  eu-west-1:\n    use_lockfile: true\n  key_vars:\n    ecosystem: example-dev\n"
	brokenNeighbour := sharedBackendFiles()
	brokenNeighbour["example/cluster/configuration.yaml"] = exampleBackendConfiguration("\n      backends:\n        eu-central-1: ''")

	cases := []struct {
		name    string
		files   map[string]string
		args    []string
		wantErr bool
	}{
		{name: "unknown format", files: sharedBackendFiles(), args: []string{"list", "--format", "xml"}, wantErr: true},
		{name: "region not declared", files: sharedBackendFiles(), args: []string{"describe", "-r", "ap-south-1"}, wantErr: true},
		{name: "bucket not pinned", files: unpinned, args: []string{"describe", "-r", "eu-west-1"}, wantErr: true},
		{name: "list with a bucket not pinned", files: unpinned, args: []string{"list"}, wantErr: true},
		{name: "list with a broken entry", files: brokenNeighbour, args: []string{"list"}, wantErr: true},
		{name: "healthy region next to a broken entry", files: brokenNeighbour, args: []string{"describe", "-r", "eu-west-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: that configuration
			// When: the command runs
			run := runBackendCommand(t, tc.files, "", tc.args...)

			// Then: a backend is only printed when init could use it, cobra's usage text is all stdout gets otherwise
			if (run.err != nil) != tc.wantErr || strings.Contains(run.stdout, "tfstate") == tc.wantErr {
				t.Errorf("err = %v, stdout = %q", run.err, run.stdout)
			}
		})
	}
}

func TestTerraformBackend_CreateHandsTheDeclarationToTheBucket_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf backend create works for a region whose bucket isn't pinned yet, with no state key to render, and prints what to pin once the bucket exists",
		CurrentImpl:     "BackendSet.BucketSpec reads the region entry without requiring a bucket or a key, bucketTarget.writePins prints what the spec left open",
		ExpectedOutcome: "The bucket gets the naming inputs, the encryption and the merged project properties, and stdout is the terraform_backend entry to add, or nothing for a dry run",
		Rationale:       "Before the bucket exists the configuration can't name it, and init only ever reads the name from the configuration",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a declaration with naming inputs, no entry for the region and no key_vars, and properties split by scope
	files := map[string]string{
		"example/cluster/configuration.yaml": exampleBackendConfiguration(""),
		"example/cluster/terraform.yaml":     "---\nexample: value\n",
		"shared/example-dev/backend.yaml": `---
terraform_backend:
  encryption: kms
  bucket_name:
    role_extend: dev
    versioning_sources: [v2]
  lifecycle:
    noncurrent_version_retention_days: 30
    noncurrent_versions_retained: 3
    abort_incomplete_upload_days: 2
  policy:
    deny_insecure_transport: false
    deny_state_deletion: false
    deny_version_deletion: false
project_properties_global:
  company_name_short: foo
  owner: team@example.com
  custom_tags:
    global: example
project_properties_proj:
  owner: project@example.com
  project_name_short: inception
  custom_tags:
    proj: example
`,
	}
	wantSpec := aws.StateBucketSpec{
		Region: "eu-west-1", Encryption: aws.EncryptionKms, RoleExtend: "dev", VersioningSources: []string{"v2"},
		// merged the way a root's merge() does it, so a map set in two scopes is the narrower one's, whole
		Properties: map[string]interface{}{"company_name_short": "foo", "owner": "project@example.com", "project_name_short": "inception",
			"custom_tags": map[string]interface{}{"proj": "example"}},
		Lifecycle: &aws.StateBucketLifecycle{NoncurrentVersionRetentionDays: 30, NoncurrentVersionsRetained: 3, AbortIncompleteUploadDays: 2},
		Policy:    &aws.StateBucketPolicy{},
	}

	// two spaces, so the entry can be pasted under a terraform_backend that is written like every other YAML file here
	cases := []struct {
		name       string
		args       []string
		result     aws.StateBucketResult
		wantStdout string
	}{
		{name: "name and key worked out", args: []string{"-A"}, result: aws.StateBucketResult{Name: exampleGeneratedName, KMSKeyID: exampleKeyARN},
			wantStdout: "terraform_backend:\n  eu-west-1:\n    bucket: " + exampleGeneratedName + "\n    kms_key_id: " + exampleKeyARN + "\n"},
		{name: "no key, as with sse", args: []string{"-A"}, result: aws.StateBucketResult{Name: exampleGeneratedName},
			wantStdout: "terraform_backend:\n  eu-west-1:\n    bucket: " + exampleGeneratedName + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bucket := &fakeStateBucket{result: &tc.result}
			request := useFakeStateBucket(t, bucket)

			// When: create runs for that region
			run := runBackendCommand(t, files, "", append([]string{"create", "-r", "eu-west-1", "-p", "example-dev"}, tc.args...)...)

			// Then: the bucket was given what the configuration says, and the entry to pin is printed
			if run.err != nil {
				t.Fatal(run.err)
			}
			if !reflect.DeepEqual(request.spec, wantSpec) || request.profile != "example-dev" {
				t.Errorf("request = %+v, want spec %+v", request, wantSpec)
			}
			if !slices.Equal(bucket.calls, []string{"PlanCreate", "Apply"}) {
				t.Errorf("calls = %v", bucket.calls)
			}
			if run.stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", run.stdout, tc.wantStdout)
			}
		})
	}
}

func TestTerraformBackend_NothingChangesThatWasNotShownAndAgreed_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "create and update show every change they are about to make and only make it once we typed yes, passed --auto-approve, and didn't ask for a dry run",
		CurrentImpl:     "newBackendProvisionCommand prints StateBucketPlan.Changes, stops there for a dry run, asks with confirmTyped unless auto approved, and only then calls Apply with that plan",
		ExpectedOutcome: "The plan is on stderr every time, a change of encryption included, Apply is only reached after yes or -A, and a dry run or IS_DRY_RUN=1 never reaches it",
		Rationale:       "A command that quietly creates a key or moves a bucket's encryption is one we stop trusting, so there is no invisible functionality",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name        string
		command     string
		args        []string
		stdin       string
		dryRunEnv   string
		wantApplied bool
		wantAsked   bool
		wantErr     bool
	}{
		{name: "yes typed", command: "create", stdin: "yes\n", wantApplied: true, wantAsked: true},
		{name: "y is not yes", command: "create", stdin: "y\n", wantAsked: true, wantErr: true},
		{name: "nobody to ask", command: "update", stdin: "", wantAsked: true, wantErr: true},
		{name: "auto approved", command: "update", args: []string{"--auto-approve"}, wantApplied: true},
		{name: "dry run", command: "create", args: []string{"-n"}},
		{name: "auto approved dry run", command: "update", args: []string{"-n", "-A"}},
		{name: "IS_DRY_RUN set and no flag", command: "update", args: []string{"-A"}, dryRunEnv: "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a bucket whose plan creates a key or changes the encryption
			bucket := &fakeStateBucket{result: &aws.StateBucketResult{Name: "example-dev-statefile-euw1"}}
			useFakeStateBucket(t, bucket)

			// When: the command runs with that answer
			run := runBackendCommandWithDryRunEnv(t, tc.dryRunEnv, sharedBackendFiles(), tc.stdin, append([]string{tc.command, "-r", "eu-west-1"}, tc.args...)...)

			// Then: the plan was shown, and applied only when it was agreed to
			if (run.err != nil) != tc.wantErr {
				t.Errorf("err = %v", run.err)
			}
			wantChange := map[string]string{"create": "  - create a KMS key", "update": "  - CHANGE ITS ENCRYPTION"}[tc.command]
			if !strings.Contains(run.prompt, wantChange) {
				t.Errorf("stderr = %q, want the plan with %q", run.prompt, wantChange)
			}
			if applied := slices.Contains(bucket.calls, "Apply"); applied != tc.wantApplied {
				t.Errorf("applied = %v, want %v, calls = %v", applied, tc.wantApplied, bucket.calls)
			}
			if tc.wantApplied && (bucket.applied != bucket.planned || len(bucket.calls) != 2) {
				t.Errorf("calls = %v, the one plan that was shown is the one that has to be applied", bucket.calls)
			}
			if !strings.Contains(run.prompt, "  - tag it with\n      Name = example\n") {
				t.Errorf("stderr = %q, want the tags indented under their own bullet", run.prompt)
			}
			if asked := strings.Contains(run.prompt, "Type yes to go ahead"); asked != tc.wantAsked {
				t.Errorf("asked = %v, want %v", asked, tc.wantAsked)
			}
		})
	}
}

func TestTerraformBackend_BucketCommandsStopOnInputsTheyCannotRead_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "create stops on a naming input, setting or pin it can't read, a setting written inside the region entry, and a setting that is misspelt",
		CurrentImpl:     "BackendSet.BucketSpec decodes bucket_name with known fields only and reads the pins as strings or not at all, backendEntry rejects settings found in the entry, Service.Backends rejects a key that is neither a setting nor a region",
		ExpectedOutcome: "An error before any bucket is asked for",
		Rationale:       "A bucket can't be renamed and a key costs money, so a setting that is quietly dropped is worse than one that stops us",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := map[string]string{
		"role_extend that isn't a string":        "terraform_backend:\n  bucket_name:\n    role_extend: [dev]\n",
		"versioning_sources that isn't a list":   "terraform_backend:\n  bucket_name:\n    versioning_sources: v2\n",
		"a naming input we don't know":           "terraform_backend:\n  bucket_name:\n    role_extended: dev\n",
		"encryption that isn't a string":         "terraform_backend:\n  encryption: [sse]\n",
		"encryption written in the region entry": "terraform_backend:\n  eu-west-1:\n    encryption: sse\n",
		"bucket_name written in the entry":       "terraform_backend:\n  eu-west-1:\n    bucket_name:\n      role_extend: dev\n",
		"a bucket name YAML reads as a number":   "terraform_backend:\n  eu-west-1:\n    bucket: 20261010\n",
		"kms_key_id that isn't a string":         "terraform_backend:\n  eu-west-1:\n    bucket: example-dev-statefile-euw1\n    kms_key_id: [example]\n",
		"region that isn't a string":             "terraform_backend:\n  eu-west-1:\n    bucket: example-dev-statefile-euw1\n    region: 1\n",
		"encryption misspelt":                    "terraform_backend:\n  encyption: sse\n",
		"bucket_name misspelt":                   "terraform_backend:\n  bucket_names:\n    role_extend: dev\n",
		"a lifecycle setting we don't know":      "terraform_backend:\n  lifecycle:\n    retention_days: 30\n",
		"a lifecycle setting that isn't numeric": "terraform_backend:\n  lifecycle:\n    abort_incomplete_upload_days: soon\n",
		"a policy statement we don't know":       "terraform_backend:\n  policy:\n    deny_everything: true\n",
		"policy written in the region entry":     "terraform_backend:\n  eu-west-1:\n    policy:\n      deny_state_deletion: false\n",
	}
	for name, declaration := range cases {
		t.Run(name, func(t *testing.T) {
			// Given: that declaration
			files := sharedBackendFiles()
			files["shared/example-dev/backend.yaml"] = "---\n" + declaration
			bucket := &fakeStateBucket{result: &aws.StateBucketResult{}}
			useFakeStateBucket(t, bucket)

			// When: create runs
			run := runBackendCommand(t, files, "", "create", "-r", "eu-west-1")

			// Then: it stops before the bucket is touched
			if run.err == nil || len(bucket.calls) > 0 {
				t.Errorf("err = %v, calls = %v", run.err, bucket.calls)
			}
		})
	}
}

func TestTerraformBackend_UpdateUsesWhatIsPinned_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf backend update hands the bucket what is pinned for the region, from the declaration or from a listed backends file, with the settings both share, and says where a missing pin goes",
		CurrentImpl:     "BackendSet.BucketSpec reads bucket, kms_key_id and region from the entry init would read and encryption and bucket_name from the declaration, writePins prints bare keys for a listed backends file",
		ExpectedOutcome: "The spec carries the pinned bucket and its own region, the shared settings reach a listed region too, a role the commands won't assume is warned about, and for a listed file the key comes out as JSON to add to that file",
		Rationale:       "Once the entry is pinned the commands must act on exactly that bucket, in the region init uses, and a pin written where init doesn't read it changes nothing",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a declaration whose settings both regions share, one region pinned in it, with the bucket kept in another
	// region and a role to assume, and one region on a listed backends file
	files := mixedBackendFiles()
	files["shared/example-dev/backend.yaml"] = `---
terraform_backend:
  encryption: sse
  bucket_name:
    role_extend: dev
  lifecycle:
    abort_incomplete_upload_days: 3
  policy:
    deny_version_deletion: false
  eu-west-1:
    bucket: example-dev-statefile-use1
    region: us-east-1
    role_arn: arn:aws:iam::123456789012:role/example
  eu-west-2:
    bucket: example-dev-statefile-euw2
    kms_key_id: example-key
  key_vars:
    ecosystem: example-dev
`
	cases := []struct {
		name       string
		region     string
		result     aws.StateBucketResult
		wantSpec   aws.StateBucketSpec
		wantStdout string
		wantLog    string
	}{
		{name: "pinned in the declaration", region: "eu-west-1", result: aws.StateBucketResult{Name: "example-dev-statefile-use1"},
			wantSpec: aws.StateBucketSpec{Name: "example-dev-statefile-use1", Region: "us-east-1"}, wantLog: "sets role_arn"},
		{name: "bucket and key both pinned, nothing left to print", region: "eu-west-2",
			result:   aws.StateBucketResult{Name: "example-dev-statefile-euw2", KMSKeyID: "example-key"},
			wantSpec: aws.StateBucketSpec{Name: "example-dev-statefile-euw2", KMSKeyID: "example-key", Region: "eu-west-2"}},
		{name: "listed backends file without a key", region: "eu-central-1",
			result:     aws.StateBucketResult{Name: "example-dev-statefile-euc1", KMSKeyID: exampleKeyARN},
			wantSpec:   aws.StateBucketSpec{Name: "example-dev-statefile-euc1", Region: "eu-central-1"},
			wantStdout: "{\n  \"kms_key_id\": \"" + exampleKeyARN + "\"\n}\n", wantLog: exampleListedBackendPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bucket := &fakeStateBucket{result: &tc.result}
			request := useFakeStateBucket(t, bucket)
			logs := captureLogs(t)
			wantSpec := tc.wantSpec
			wantSpec.Encryption, wantSpec.RoleExtend, wantSpec.Properties = aws.EncryptionSse, "dev", map[string]interface{}{}
			// the one field the declaration sets goes on top of the defaults, the others keep theirs
			wantSpec.Lifecycle = &aws.StateBucketLifecycle{NoncurrentVersionRetentionDays: 90, NoncurrentVersionsRetained: 10, AbortIncompleteUploadDays: 3}
			wantSpec.Policy = &aws.StateBucketPolicy{DenyInsecureTransport: true, DenyStateDeletion: true}

			// When: update runs for the region
			run := runBackendCommand(t, files, "", "update", "-r", tc.region, "-A")

			// Then: the pinned bucket was updated where it lives, and only what the entry lacks is printed
			if run.err != nil {
				t.Fatal(run.err)
			}
			if !reflect.DeepEqual(request.spec, wantSpec) {
				t.Errorf("request = %+v, want spec %+v", request, wantSpec)
			}
			if !slices.Equal(bucket.calls, []string{"PlanUpdate", "Apply"}) {
				t.Errorf("calls = %v", bucket.calls)
			}
			if run.stdout != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", run.stdout, tc.wantStdout)
			}
			if !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("logs = %q, want them to mention %q", logs.String(), tc.wantLog)
			}
		})
	}
}

func TestTerraformBackend_PinsForAListedFileComeInItsOwnFormat_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "What has to be added to a listed backends file is printed as YAML for a YAML file and as JSON for a JSON one",
		CurrentImpl:     "bucketTarget.writePins picks the format from the listed file's extension",
		ExpectedOutcome: "Bare keys that can be pasted into the file as they are",
		Rationale:       "A backends file is either, and JSON pasted into a YAML file of plain keys is one more thing to tidy by hand",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	for file, want := range map[string]string{
		"backends/eu-west-1.tfvars.yaml": "kms_key_id: example-key\n",
		"backends/eu-west-1.tfvars.json": "{\n  \"kms_key_id\": \"example-key\"\n}\n",
	} {
		t.Run(file, func(t *testing.T) {
			// Given: a bucket whose listed backends file names the bucket and no key
			target := &bucketTarget{spec: aws.StateBucketSpec{Name: "example-dev-statefile-euw1"}, region: "eu-west-1", listedFile: file}
			var out bytes.Buffer

			// When: the key that was worked out is printed
			err := target.writePins(&out, &aws.StateBucketResult{Name: "example-dev-statefile-euw1", KMSKeyID: "example-key"})

			// Then: it is in the format of that file
			if err != nil || out.String() != want {
				t.Errorf("err = %v, stdout = %q, want %q", err, out.String(), want)
			}
		})
	}
}

func TestTerraformBackend_DeleteNeedsTheBucketNameTyped_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf backend delete says which bucket it will delete and only deletes it when the name is typed back or --force is set, never asks about a bucket it would refuse, and a dry run stops after the check",
		CurrentImpl:     "The delete command runs EnsureDeletable, prints what it will delete, returns for a dry run, asks with confirmTyped unless forced, then calls Delete",
		ExpectedOutcome: "A wrong answer or no input leaves the bucket, the name or --force deletes it, a bucket that isn't deletable is never asked about, and -n or IS_DRY_RUN=1 never reaches Delete, forced or not",
		Rationale:       "A y/N prompt gets answered by habit, typing the name makes us read which bucket it is, and a dry run skips the question so it must never get as far as the delete",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	cases := []struct {
		name         string
		stdin        string
		args         []string
		dryRunEnv    string
		deletableErr error
		wantCalls    []string
		wantPrompt   bool
		wantErr      bool
	}{
		{name: "IS_DRY_RUN set and no flag", dryRunEnv: "1", wantCalls: []string{"EnsureDeletable"}},
		{name: "yes is not the name", stdin: "yes\n", wantCalls: []string{"EnsureDeletable"}, wantPrompt: true, wantErr: true},
		{name: "nobody to ask", stdin: "", wantCalls: []string{"EnsureDeletable"}, wantPrompt: true, wantErr: true},
		{name: "name typed", stdin: "example-dev-statefile-euw1\n", wantCalls: []string{"EnsureDeletable", "Delete"}, wantPrompt: true},
		{name: "force", args: []string{"--force"}, wantCalls: []string{"EnsureDeletable", "Delete"}},
		{name: "dry run", args: []string{"--dry-run"}, wantCalls: []string{"EnsureDeletable"}},
		{name: "forced dry run", args: []string{"-f", "-n"}, wantCalls: []string{"EnsureDeletable"}},
		{name: "not empty, even with force", args: []string{"--force"}, deletableErr: errors.New("still holds objects"),
			wantCalls: []string{"EnsureDeletable"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a bucket, and that answer waiting on stdin
			bucket := &fakeStateBucket{name: "example-dev-statefile-euw1", deletableErr: tc.deletableErr}
			useFakeStateBucket(t, bucket)

			// When: delete runs
			run := runBackendCommandWithDryRunEnv(t, tc.dryRunEnv, sharedBackendFiles(), tc.stdin, append([]string{"delete", "-r", "eu-west-1"}, tc.args...)...)

			// Then: Delete is only reached when it was confirmed and this is no dry run
			if (run.err != nil) != tc.wantErr {
				t.Errorf("err = %v", run.err)
			}
			if !slices.Equal(bucket.calls, tc.wantCalls) {
				t.Errorf("calls = %v, want %v", bucket.calls, tc.wantCalls)
			}
			if asked := strings.Contains(run.prompt, "Type the bucket name"); asked != tc.wantPrompt {
				t.Errorf("asked = %v, want %v", asked, tc.wantPrompt)
			}
			if said := strings.Contains(run.prompt, "delete the empty state bucket example-dev-statefile-euw1"); said == (tc.deletableErr != nil) {
				t.Errorf("stderr = %q, the bucket should be named unless the delete is refused", run.prompt)
			}
		})
	}
}

func TestTerraformBackend_TagPropertiesMergeTheScopedBlocks_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "The bucket is named and tagged from project_properties_global, _env and _proj merged in that order, then project_properties",
		CurrentImpl:     "wrapper.MergeProperties merges the tagPropertiesParts blocks of the assembled configuration, a later block wins, and no other block is read",
		ExpectedOutcome: "Each scope overrides the wider ones before it, a block that isn't one of the four is ignored, and a configuration with none of them gives nothing",
		Rationale:       "Roots merge exactly these scopes before handing them to the tagging provider, so any other order would name a different bucket than Terraform",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: every scope setting its own key and the ones wider scopes set
	configuration := map[string]interface{}{
		"project_properties_global": map[string]interface{}{"global": "global", "env": "global", "proj": "global", "plain": "global"},
		"project_properties_env":    map[string]interface{}{"env": "env", "proj": "env", "plain": "env"},
		"project_properties_proj":   map[string]interface{}{"proj": "proj", "plain": "proj"},
		"project_properties":        map[string]interface{}{"plain": "plain"},
		"project_properties_eco":    map[string]interface{}{"global": "eco", "eco": "eco"},
		"example":                   "value",
	}

	// When: the tag properties are worked out
	properties, err := wrapper.MergeProperties(configuration, false, tagPropertiesParts...)
	none, noneErr := wrapper.MergeProperties(map[string]interface{}{"example": "value"}, false, tagPropertiesParts...)
	if err != nil || noneErr != nil {
		t.Fatal(err, noneErr)
	}

	// Then: every key holds the value of the narrowest scope that set it
	want := map[string]interface{}{"global": "global", "env": "env", "proj": "proj", "plain": "plain"}
	if !reflect.DeepEqual(properties, want) {
		t.Errorf("properties = %v, want %v", properties, want)
	}
	if len(none) != 0 {
		t.Errorf("properties without any block = %v", none)
	}
}
