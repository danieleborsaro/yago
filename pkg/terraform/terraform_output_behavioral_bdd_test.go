package terraform

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type consoleWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	onWrite func(written string)
}

func (w *consoleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if w.onWrite != nil {
		w.onWrite(w.buf.String())
	}
	return len(p), nil
}

func (w *consoleWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type goneConsole struct {
	writes int
}

func (c *goneConsole) Write(p []byte) (int, error) {
	c.writes++
	return 0, io.ErrClosedPipe
}

func givenFakeTerraform(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "terraform"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("IS_DRY_RUN", "0")
	return dir
}

func givenSecretInputs(t *testing.T, dir string, values map[string]string) *Service {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".gitops"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := secretInputManifest{Version: 1, Variables: map[string]SecretVariableBinding{}}
	reader := &fakeSecretValueReader{values: map[secretReadCall]string{}}
	for variable, value := range values {
		manifest.Variables[variable] = SecretVariableBinding{SecretID: "example/" + variable}
		reader.values[secretReadCall{name: "example/" + variable}] = value
	}
	if err := writeSecretManifest(defaultSecretManifest(dir), manifest); err != nil {
		t.Fatal(err)
	}
	svc := NewService(dir, false)
	svc.secretReader = reader
	return svc
}

func showingOutput(svc *Service) (*consoleWriter, *consoleWriter) {
	stdout, stderr := &consoleWriter{}, &consoleWriter{}
	svc.stdout, svc.stderr = stdout, stderr
	return stdout, stderr
}

func capturing(t *testing.T, stream **os.File) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := *stream
	*stream = w
	var written bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&written, r)
		close(done)
	}()
	return func() string {
		*stream = original
		_ = w.Close()
		<-done
		_ = r.Close()
		return written.String()
	}
}

func TestTerraform_ShowsItsOutputWhileItRuns_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Show Terraform's output while it runs, without --verbose",
		CurrentImpl:     "runTerraformCommandWithSecrets passes Terraform's stdout and stderr, as one stream, a line at a time to yago's stdout",
		ExpectedOutcome: "Lines reach yago's stdout before Terraform exits, in the order Terraform wrote them",
		Rationale:       "Plans and applies must be readable, and their progress visible, without debug logging",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Terraform that waits until its first lines are shown
	dir := givenFakeTerraform(t, `#!/bin/sh
echo "Refreshing state..."
echo "Warning: deprecated argument" >&2
i=0
while [ ! -f "$YAGO_TEST_SHOWN" ]; do
  i=$((i + 1))
  [ "$i" -gt 200 ] && exit 3
  sleep 0.05
done
echo "No changes."
`)
	shown := filepath.Join(t.TempDir(), "shown")
	t.Setenv("YAGO_TEST_SHOWN", shown)
	svc := NewService(dir, false)
	stdout, stderr := showingOutput(svc)
	stdout.onWrite = func(written string) {
		if strings.Contains(written, "Refreshing state...\n") && strings.Contains(written, "Warning: deprecated argument\n") {
			_ = os.WriteFile(shown, nil, 0600)
		}
	}

	// When: yago plans
	resp, err := svc.Plan(PlanRequest{WorkingDir: dir})

	// Then: lines are shown while it runs, in order
	if err != nil {
		t.Fatalf("plan failed: %v (the fake Terraform exits 3 if its lines aren't shown while it runs)", err)
	}
	want := "Refreshing state...\nWarning: deprecated argument\nNo changes.\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want nothing", got)
	}
	if resp.Output != want {
		t.Errorf("returned output = %q, want %q", resp.Output, want)
	}
}

func TestTerraform_RedactsSecretsInTheOutputItShows_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Redact secret inputs in the Terraform output yago shows and returns",
		CurrentImpl:     "lineWriter redacts each whole line, however Terraform's writes split it",
		ExpectedOutcome: "Only [REDACTED] is shown and returned, never the secret value",
		Rationale:       "Secret values must never be printed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a secret input printed on both streams, and split across writes
	dir := givenFakeTerraform(t, `#!/bin/sh
printf 'token=%s\n' "$TF_VAR_auth"
printf 'token=%s\n' "$TF_VAR_auth" >&2
printf 'token=%s' "$(printf '%s' "$TF_VAR_auth" | cut -c1-7)"
sleep 0.1
printf '%s\n' "$(printf '%s' "$TF_VAR_auth" | cut -c8-)"
`)
	svc := givenSecretInputs(t, dir, map[string]string{"auth": "example-token-value"})
	stdout, _ := showingOutput(svc)

	// When: yago plans
	resp, err := svc.Plan(PlanRequest{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Then: it's redacted, as shown and as returned
	want := "token=[REDACTED]\ntoken=[REDACTED]\ntoken=[REDACTED]\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if resp.Output != want {
		t.Errorf("returned output = %q, want %q", resp.Output, want)
	}
}

func TestTerraform_RedactsSecretsAsTerraformPrintsThem_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Redact secrets in the forms Terraform prints them in",
		CurrentImpl:     "newSecretRedactor adds JSON-escaped forms without HTML escaping, and each line of a multi-line secret",
		ExpectedOutcome: "An escaped string, an indented heredoc and a multi-line secret with short lines are all redacted",
		Rationale:       "Terraform escapes quotes and backslashes but not &, < and >, and prints multi-line strings as indented heredocs",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: secrets printed as Terraform prints them
	dir := givenFakeTerraform(t, `#!/bin/sh
esc=$(printf '\033')
printf '  ~ password = "%s"\n' "$(printf '%s' "$TF_VAR_password" | sed 's/\\/\\\\/g; s/"/\\"/g')"
echo '  ~ certificate = <<-EOT'
printf '%s\n' "$TF_VAR_certificate" | sed 's/^/        /'
echo '    EOT'
echo '  ~ pin = <<-EOT'
printf '%s\n' "$TF_VAR_pin" | sed 's/^/      - /'
printf '%s\n' "$TF_VAR_pin" | sed "s/^/      ${esc}[32m+${esc}[0m${esc}[0m /"
echo '    EOT'
printf '%s\n' "$TF_VAR_pin"
echo 'marked = <<-EOT'
printf '%s\n' "$TF_VAR_marked" | sed 's/^/      /'
echo 'EOT'
`)
	svc := givenSecretInputs(t, dir, map[string]string{
		"password":    `pa"ss&w<o>rd\1`,
		"certificate": "-----BEGIN EXAMPLE-----\nZXhhbXBsZSBjZXJ0aWZpY2F0ZQ==\n-----END EXAMPLE-----",
		"pin":         "pin\n123",
		"marked":      "+ a\n- b",
	})
	stdout, _ := showingOutput(svc)

	// When: yago plans
	if _, err := svc.Plan(PlanRequest{WorkingDir: dir}); err != nil {
		t.Fatal(err)
	}

	// Then: no secret is shown
	want := "  ~ password = \"[REDACTED]\"\n" +
		"  ~ certificate = <<-EOT\n        [REDACTED]\n        [REDACTED]\n        [REDACTED]\n    EOT\n" +
		"  ~ pin = <<-EOT\n      - [REDACTED]\n      - [REDACTED]\n      + [REDACTED]\n      + [REDACTED]\n    EOT\n" +
		"[REDACTED]\n[REDACTED]\n" +
		"marked = <<-EOT\n      [REDACTED]\n      [REDACTED]\nEOT\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestTerraform_AppliesASavedPlanWithTheSecretVersionsItWasMadeWith_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Apply a saved plan with the secret versions the plan was made with",
		CurrentImpl:     "Plan pins each secret input to the version it read in <plan>.secrets.json, and Apply reads those versions",
		ExpectedOutcome: "After the secret rotates, applying the plan still reads, and so redacts, the value the plan holds",
		Rationale:       "A saved plan keeps the values it was made with, and Terraform can print them while applying",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Terraform that prints the planned value when applying
	dir := givenFakeTerraform(t, `#!/bin/sh
case "$1" in
  plan)
    for arg in "$@"; do
      case "$arg" in -out=*) printf '%s' "$TF_VAR_token" > "${arg#-out=}" ;; esac
    done
    ;;
  apply) printf 'token = "%s"\n' "$(cat "$2")" ;;
esac
`)
	// Given: a saved plan
	svc := givenSecretInputs(t, dir, map[string]string{"token": "example-planned-value"})
	reader := svc.secretReader.(*fakeSecretValueReader)
	reader.versions = map[secretReadCall]string{{name: "example/token"}: "planned-version"}
	reader.values[secretReadCall{name: "example/token", version: "planned-version"}] = "example-planned-value"
	stdout, _ := showingOutput(svc)
	if _, err := svc.Plan(PlanRequest{WorkingDir: dir, OutFile: "tfplan"}); err != nil {
		t.Fatal(err)
	}

	// When: the secret rotates, then the plan is applied
	reader.values[secretReadCall{name: "example/token"}] = "example-rotated-value"
	reader.versions[secretReadCall{name: "example/token"}] = "rotated-version"
	if _, err := svc.Apply(ApplyRequest{WorkingDir: dir, PlanFile: "tfplan"}); err != nil {
		t.Fatal(err)
	}

	// Then: the planned version is read, and redacted
	if last := reader.calls[len(reader.calls)-1]; last != (secretReadCall{name: "example/token", version: "planned-version"}) {
		t.Errorf("apply read %+v, want the planned version", last)
	}
	if got := stdout.String(); got != "token = \"[REDACTED]\"\n" {
		t.Errorf("stdout = %q", got)
	}
}

func TestTerraform_KeepsOutputYagoReadsOffTheConsole_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Don't show the output of terraform output and terraform graph, unless they fail",
		CurrentImpl:     "showsTerraformOutput excludes output and graph, whose output yago prints or converts itself",
		ExpectedOutcome: "Their output is only returned, and a failure's output is shown on stderr",
		Rationale:       "yago tf output prints the JSON itself, and graph's output is DOT for Graphviz",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Terraform with outputs and a graph
	dir := givenFakeTerraform(t, `#!/bin/sh
if [ "$1" = output ] && [ "$2" = -json ]; then echo '{"id":{"value":"example"}}'; exit 0; fi
if [ "$1" = graph ]; then echo 'digraph {}'; exit 0; fi
echo "Error: example failure" >&2
exit 1
`)
	svc := NewService(dir, false)
	stdout, stderr := showingOutput(svc)

	// When: yago reads the outputs and the graph
	outputs, err := svc.Output(OutputRequest{WorkingDir: dir, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := svc.Graph(GraphRequest{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Then: they're returned, and nothing is shown
	if outputs.Output != "{\"id\":{\"value\":\"example\"}}\n" || graph.Output != "digraph {}\n" {
		t.Errorf("returned output = %q and %q", outputs.Output, graph.Output)
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Errorf("shown output = %q and %q", stdout.String(), stderr.String())
	}

	// When: reading the outputs fails
	if _, err := svc.Output(OutputRequest{WorkingDir: dir}); err == nil {
		t.Fatal("expected terraform output to fail")
	}

	// Then: Terraform's error is shown
	if got := stderr.String(); got != "Error: example failure\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestTerraform_KeepsRunningWhenTheConsoleGoesAway_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Let Terraform finish when yago's output can no longer be written",
		CurrentImpl:     "lineWriter stops writing to the console after its first failure, and keeps reading Terraform's output",
		ExpectedOutcome: "The plan succeeds, its whole output is returned, and the console is written to only once",
		Rationale:       "An apply stopped halfway, for example by | head exiting, can leave state unsaved and its lock held",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a console whose reader has gone
	dir := givenFakeTerraform(t, `#!/bin/sh
for i in 1 2 3 4 5; do echo "line $i"; done
`)
	svc := NewService(dir, false)
	console := &goneConsole{}
	svc.stdout = console

	// When: yago plans
	resp, err := svc.Plan(PlanRequest{WorkingDir: dir})

	// Then: the plan completes, and nothing is lost
	if err != nil {
		t.Fatal(err)
	}
	if want := "line 1\nline 2\nline 3\nline 4\nline 5\n"; resp.Output != want {
		t.Errorf("returned output = %q, want %q", resp.Output, want)
	}
	if console.writes != 1 {
		t.Errorf("console written to %d times, want 1", console.writes)
	}
}

func TestTerraform_OutputCommandKeepsStdoutForTheJSON_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Keep yago tf output's stdout for the outputs' JSON",
		CurrentImpl:     "runOutput sends the output of init, providers lock and workspace select to stderr",
		ExpectedOutcome: "stdout is only the JSON, and Terraform's other output is on stderr",
		Rationale:       "yago tf output is piped into tools such as jq",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: Terraform with outputs
	sourceDir := givenFakeTerraform(t, `#!/bin/sh
case "$1" in
  --version) echo "Terraform v1.16.3" ;;
  init) echo "Initializing the backend..." ;;
  providers) echo "Success! Terraform has validated the lock file." ;;
  workspace) [ "$2" = list ] && echo "* default" ;;
  output) echo '{"id":{"value":"example"}}' ;;
esac
exit 0
`)
	// Given: a desiredstate and its configuration
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
	flags := &commonFlags{
		awsRegion:         "eu-west-1",
		workspace:         "default",
		desiredstateRoot:  filepath.Join(desiredStates, "example/deploy/desiredstate.yaml"),
		terraformSource:   sourceDir,
		configurationRoot: filepath.Join(configs, "example/deploy/configuration.yaml"),
		environment:       "all",
	}

	// When: yago tf output runs with init
	stdout := capturing(t, &os.Stdout)
	stderr := capturing(t, &os.Stderr)
	err := runOutput(flags, "", true, false, false, false, false)
	gotStdout, gotStderr := stdout(), stderr()

	// Then: stdout is only the JSON
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(gotStdout)) || strings.TrimSpace(gotStdout) != `{"id":{"value":"example"}}` {
		t.Errorf("stdout = %q, want only the JSON", gotStdout)
	}
	for _, line := range []string{"Initializing the backend...", "Success! Terraform has validated the lock file."} {
		if !strings.Contains(gotStderr, line) {
			t.Errorf("stderr is missing %q: %q", line, gotStderr)
		}
	}
}
