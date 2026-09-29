package terraform

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danieleborsaro/yago/internal/schema"
)

// stub terraform on PATH, it answers version checks and logs every other call so
// a test can see exactly what yago ran, it creates the file a plan is saved to, and
// an infracost stub logs its calls to the same log
func fakeTerraform(t *testing.T) (logPath string) {
	t.Helper()
	bin := t.TempDir()
	logPath = filepath.Join(bin, "calls.log")
	script := `#!/bin/sh
if [ "$1" = "--version" ] || [ "$1" = "version" ]; then
  echo "Terraform v` + DefaultTerraformVersion + `"
  exit 0
fi
echo "$*" >> "` + logPath + `"
for arg in "$@"; do
  case "$arg" in -out=*) : > "${arg#-out=}" ;; esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "terraform"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake terraform: %v", err)
	}
	infracost := "#!/bin/sh\necho \"infracost $*\" >> \"" + logPath + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "infracost"), []byte(infracost), 0o755); err != nil {
		t.Fatalf("writing fake infracost: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("IS_DRY_RUN", "")
	return logPath
}

func terraformCalls(t *testing.T, logPath string) []string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading terraform call log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func writePlan(t *testing.T, codeDir, planPath string) string {
	t.Helper()
	plan := filepath.Join(codeDir, planPath)
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan, []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestTerraformCommands_DryRunRunsNothing_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf provision and tf destroy with --dry-run don't run terraform",
		CurrentImpl:     "runProvision and runDestroy call SetDryRun on the service when --dry-run is set",
		ExpectedOutcome: "No terraform command runs, only what would run is logged",
		Rationale:       "--dry-run used to be ignored, so destroy --dry-run with a plan file really destroyed",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name string
		plan string
		run  func(*commonFlags) error
	}{
		{name: "provision", plan: provisionPlanName, run: func(f *commonFlags) error { return runProvision(f, true) }},
		{name: "destroy with plan file", plan: destroyPlanName, run: func(f *commonFlags) error {
			return runDestroy(f, false, false, false, false, false, true, false, true)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a code directory with a saved plan
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			writePlan(t, codeDir, tt.plan)

			// When: the command runs with --dry-run
			err := tt.run(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir})

			// Then: terraform was never called
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if calls := terraformCalls(t, log); len(calls) != 0 {
				t.Errorf("--dry-run ran terraform: %q", calls)
			}
		})
	}
}

func TestTerraformProvision_AppliesSavedPlan_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf provision applies the saved plan under the python name, or under a name yago used before",
		CurrentImpl:     "runProvision gets the plan from findSavedPlan and runs terraform apply on it",
		ExpectedOutcome: "Exactly one terraform apply of the plan that was found",
		Rationale:       "Provision must apply the reviewed plan and nothing else, whichever tool saved it",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	for _, planPath := range []string{provisionPlanName, "tfplan", filepath.Join(".gitops", "tfplan")} {
		t.Run(planPath, func(t *testing.T) {
			// Given: a plan at this path and no desired state
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			plan := writePlan(t, codeDir, planPath)

			// When: provision runs with only -s
			err := runProvision(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir}, false)

			// Then: that plan is applied
			if err != nil {
				t.Fatalf("runProvision: %v", err)
			}
			if calls := terraformCalls(t, log); len(calls) != 1 || calls[0] != "apply "+plan {
				t.Errorf("terraform calls = %q, want [%q]", calls, "apply "+plan)
			}
		})
	}
}

func TestTerraformSavedPlans_PythonNameWins_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "When a plan exists under both names, the python name is the one applied",
		CurrentImpl:     "findSavedPlan tries the python name before the names yago used before",
		ExpectedOutcome: "provision applies gitops.tf-provision.tfplan and destroy applies gitops.tf-destroy.tfplan",
		Rationale:       "New plans get the python names, so an old plan left next to one must not win",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name          string
		wanted, stale string
		run           func(*commonFlags) error
	}{
		{name: "provision", wanted: provisionPlanName, stale: "tfplan", run: func(f *commonFlags) error { return runProvision(f, false) }},
		{name: "destroy", wanted: destroyPlanName, stale: "tfplan-destroy", run: func(f *commonFlags) error {
			return runDestroy(f, false, false, false, false, false, true, false, false)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a code directory holding a plan under each name
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			plan := writePlan(t, codeDir, tt.wanted)
			writePlan(t, codeDir, tt.stale)

			// When: the command runs
			err := tt.run(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir})

			// Then: the python named plan is applied
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if calls := terraformCalls(t, log); len(calls) != 1 || calls[0] != "apply "+plan {
				t.Errorf("terraform calls = %q, want [%q]", calls, "apply "+plan)
			}
		})
	}
}

func TestTerraformSavedPlans_RelativeSourceDir_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A saved plan is found and applied when -s is a relative path",
		CurrentImpl:     "findSavedPlan returns an absolute path, terraform runs inside the code directory",
		ExpectedOutcome: "terraform apply gets the absolute path of the plan",
		Rationale:       "A relative -s used to be prefixed twice, once by yago and again by terraform's working directory",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a plan in infra, next to where yago runs
	log := fakeTerraform(t)
	workdir := t.TempDir()
	chdirForTest(t, workdir)
	writePlan(t, filepath.Join(workdir, "infra"), provisionPlanName)

	// When: provision runs with -s infra
	err := runProvision(&commonFlags{awsRegion: "eu-west-1", terraformSource: "infra"}, false)

	// Then: terraform is given the plan's absolute path
	if err != nil {
		t.Fatalf("runProvision: %v", err)
	}
	// macOS reports the temp dir through /private once it's the working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := "apply " + filepath.Join(cwd, "infra", provisionPlanName)
	if calls := terraformCalls(t, log); len(calls) != 1 || calls[0] != want {
		t.Errorf("terraform calls = %q, want [%q]", calls, want)
	}
}

func TestTerraformSavedPlans_SidecarFollowsThePlan_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A saved plan is applied with the secret sidecar next to it, whichever name the plan has",
		CurrentImpl:     "Apply reads planSecretManifest of the path findSavedPlan returned",
		ExpectedOutcome: "An unpinned sidecar next to the plan stops the apply before terraform runs",
		Rationale:       "The sidecar pins the secret versions the plan was made with, reading another file could apply different secrets",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	for _, planName := range []string{provisionPlanName, "tfplan"} {
		t.Run(planName, func(t *testing.T) {
			// Given: a plan whose own sidecar has a secret without a pinned version
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			plan := writePlan(t, codeDir, planName)
			unpinned := secretInputManifest{Version: 1, Variables: map[string]SecretVariableBinding{
				"password": {SecretID: "example/password"},
			}}
			if err := writeSecretManifest(plan+planSecretSuffix, unpinned); err != nil {
				t.Fatal(err)
			}

			// When: provision runs
			err := runProvision(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir}, false)

			// Then: that sidecar was read, so the apply stops before terraform runs
			if err == nil || !strings.Contains(err.Error(), "isn't pinned to a version") {
				t.Fatalf("error = %v, want the unpinned secret in %s to stop the apply", err, plan+planSecretSuffix)
			}
			if calls := terraformCalls(t, log); len(calls) != 0 {
				t.Errorf("terraform ran: %q", calls)
			}
		})
	}
}

func TestTerraformSavedPlans_PlanThenUse_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "A plan saved by tf plan is the one provision, destroy and costs pick up",
		CurrentImpl:     "runPlan saves to the python names and provision, destroy and costs all use findSavedPlan",
		ExpectedOutcome: "terraform plan writes gitops.tf-provision.tfplan with its sidecar, then apply and infracost read that same file",
		Rationale:       "costs looked for gitops.tf-provision.tfplan while plan wrote tfplan, so costs never found a plan yago made",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	root := yagoRootFromTestFile(t)
	chdirForTest(t, root)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() { schema.SetNamespaceOverride(prevNamespace) })

	tests := []struct {
		name     string
		destroy  bool
		planName string
		use      func(*commonFlags) error
		wantUse  string // the call that must use the saved plan, %s is its path
	}{
		{name: "provision", planName: provisionPlanName, use: func(f *commonFlags) error { return runProvision(f, false) }, wantUse: "apply %s"},
		{name: "costs", planName: provisionPlanName, use: func(f *commonFlags) error { return runCosts(f, true) }, wantUse: "infracost breakdown --path=%s"},
		{name: "destroy", destroy: true, planName: destroyPlanName, use: func(f *commonFlags) error {
			return runDestroy(f, false, false, false, false, false, true, false, false)
		}, wantUse: "apply %s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a plan made by tf plan from a desired state
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			flags := &commonFlags{
				awsRegion:         "eu-west-1",
				environment:       "all",
				desiredstateRoot:  filepath.Join("tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "desiredstate.yaml"),
				configurationRoot: filepath.Join("tests", "assets", "4.2.0", "configurations", "concourse-cluster", "configuration.yaml"),
				terraformSource:   codeDir,
			}
			if err := runPlan(flags, tt.destroy, false, false, false, false, false, false, false); err != nil {
				t.Fatalf("runPlan: %v", err)
			}
			plan := filepath.Join(codeDir, tt.planName)
			if _, err := os.Stat(plan + planSecretSuffix); err != nil {
				t.Errorf("no secret sidecar next to the plan: %v", err)
			}

			// When: the plan is used
			err := tt.use(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir})

			// Then: terraform planned to the python name and the plan was used from there
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			calls := terraformCalls(t, log)
			planned := false
			for _, call := range calls {
				planned = planned || (strings.HasPrefix(call, "plan ") && strings.Contains(call, "-out="+tt.planName))
			}
			if !planned {
				t.Errorf("no terraform plan to %s in %q", tt.planName, calls)
			}
			if want := fmt.Sprintf(tt.wantUse, plan); calls[len(calls)-1] != want {
				t.Errorf("last call = %q, want %q", calls[len(calls)-1], want)
			}
		})
	}
}

func TestTerraformProvision_MissingInputs_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf provision explains what's missing instead of running terraform",
		CurrentImpl:     "runProvision checks for a code directory and a saved plan before applying",
		ExpectedOutcome: "A clear error and no terraform call",
		Rationale:       "Without -d it used to fail trying to read the current directory as a desired state",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a stub terraform
	log := fakeTerraform(t)

	// When: provision runs without -s or -d
	err := runProvision(&commonFlags{awsRegion: "eu-west-1"}, false)

	// Then: it asks for the source directory
	if err == nil || !strings.Contains(err.Error(), "terraform source directory not specified") {
		t.Errorf("error = %v, want it to ask for the source directory", err)
	}

	// When: provision runs on a directory with no plan
	err = runProvision(&commonFlags{awsRegion: "eu-west-1", terraformSource: t.TempDir()}, false)

	// Then: it says the plan is missing
	if err == nil || !strings.Contains(err.Error(), "plan file not found") {
		t.Errorf("error = %v, want plan file not found", err)
	}
	if calls := terraformCalls(t, log); len(calls) != 0 {
		t.Errorf("terraform ran despite the errors: %q", calls)
	}
}

func TestTerraformDestroy_AppliesDestroyPlan_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf destroy --destroy-with-planfile applies the saved destroy plan, python name or the name yago used before",
		CurrentImpl:     "runDestroyWithPlan gets the destroy plan from findSavedPlan and runs terraform apply on it",
		ExpectedOutcome: "Exactly one terraform apply of the destroy plan that was found",
		Rationale:       "Destroy with a plan must apply the reviewed destroy plan and nothing else",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	for _, planName := range []string{destroyPlanName, "tfplan-destroy"} {
		t.Run(planName, func(t *testing.T) {
			// Given: a code directory with a destroy plan
			log := fakeTerraform(t)
			codeDir := t.TempDir()
			plan := writePlan(t, codeDir, planName)

			// When: destroy runs with the plan file
			err := runDestroy(&commonFlags{awsRegion: "eu-west-1", terraformSource: codeDir},
				false, false, false, false, false, true, false, false)

			// Then: that plan is applied
			if err != nil {
				t.Fatalf("runDestroy: %v", err)
			}
			if calls := terraformCalls(t, log); len(calls) != 1 || calls[0] != "apply "+plan {
				t.Errorf("terraform calls = %q, want [%q]", calls, "apply "+plan)
			}
		})
	}
}

// copied from a real terraform 1.13.3 apply
const colouredPrompt = "  \x1b[1mEnter a value:\x1b[0m \x1b[0m"

func TestTerraformConfirmation_PromptShowsBeforeTerraformReads_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Terraform's confirmation prompt is on screen while it waits, and the answer comes from yago's stdin",
		CurrentImpl:     "runTerraformCommandWithSecrets passes s.stdin through and lineWriter shows a line that is only the prompt",
		ExpectedOutcome: "The user sees the coloured prompt, types yes, and terraform gets it",
		Rationale:       "Go gives a child an empty stdin, and a prompt without a newline stayed buffered, so the user could never answer",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	// Given: a terraform that prints the real coloured prompt and waits for an answer
	dir := givenFakeTerraform(t, `#!/bin/sh
printf 'Only '"'"'yes'"'"' will be accepted to confirm.\n\n`+strings.ReplaceAll(colouredPrompt, "\x1b", `\033`)+`'
read answer
echo "$answer" > answer.txt
`)
	service := NewService(dir, false)
	answer, typing := io.Pipe()
	service.stdin = answer
	promptShown := make(chan struct{})
	var once sync.Once
	service.stdout = &consoleWriter{onWrite: func(written string) {
		if strings.HasSuffix(written, colouredPrompt) {
			once.Do(func() { close(promptShown) })
		}
	}}

	// Given: a user who only types once the prompt is on screen
	go func() {
		select {
		case <-promptShown:
			_, _ = typing.Write([]byte("yes\n"))
		case <-time.After(10 * time.Second):
		}
		_ = typing.Close()
	}()

	// When: an unconfirmed unlock runs
	_, err := service.Unlock(UnlockRequest{WorkingDir: dir, LockID: "abc-123"})

	// Then: the prompt was shown and terraform got the answer typed after it
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	select {
	case <-promptShown:
	default:
		t.Fatal("the prompt never reached the screen while terraform waited")
	}
	got, err := os.ReadFile(filepath.Join(dir, "answer.txt"))
	if err != nil || strings.TrimSpace(string(got)) != "yes" {
		t.Errorf("terraform read %q (%v), want yes", got, err)
	}
}

func TestTerraformOutput_ShowsOnlyASafePromptEarly_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "An unfinished line is shown early only when it is nothing but terraform's prompt",
		CurrentImpl:     "lineWriter.showPrompt strips colour, needs the line to be the prompt, and skips it when a secret could start there",
		ExpectedOutcome: "Plain and coloured prompts show at once, anything else waits for its newline and stays redacted",
		Rationale:       "Showing part of a line early could print the first half of a secret the redactor would have caught whole",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name      string
		secret    string
		writes    []string
		wantEarly string // on screen after every write but the last
		wantFinal string
	}{
		{
			name:      "plain prompt",
			writes:    []string{"Plan: 0 to add, 0 to destroy.\n\n  Enter a value: ", "yes\n"},
			wantEarly: "Plan: 0 to add, 0 to destroy.\n\n  Enter a value: ",
			wantFinal: "Plan: 0 to add, 0 to destroy.\n\n  Enter a value: yes\n",
		},
		{
			name:      "coloured prompt",
			writes:    []string{colouredPrompt, "\n"},
			wantEarly: colouredPrompt,
			wantFinal: colouredPrompt + "\n",
		},
		{
			name:      "any other unfinished line waits",
			writes:    []string{"Plan: 0 to add", ", 0 to destroy.\n"},
			wantFinal: "Plan: 0 to add, 0 to destroy.\n",
		},
		{
			name:      "prompt text inside a secret",
			secret:    "token Enter a value: suffix",
			writes:    []string{"token Enter a value: ", "suffix\n"},
			wantFinal: "[REDACTED]\n",
		},
		{
			name:      "secret starting inside the prompt",
			secret:    "value: suffix",
			writes:    []string{"  Enter a value: ", "suffix\n"},
			wantFinal: "  Enter a [REDACTED]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a writer showing terraform's output, maybe with a secret to hide
			var screen strings.Builder
			env := map[string]string{}
			if tt.secret != "" {
				env["TF_VAR_example"] = tt.secret
			}
			w := &lineWriter{out: &screen, redactor: newSecretRedactor(env)}

			// When: terraform's output arrives in pieces
			for _, chunk := range tt.writes[:len(tt.writes)-1] {
				_, _ = w.Write([]byte(chunk))
			}

			// Then: only a safe prompt has been shown so far
			if screen.String() != tt.wantEarly {
				t.Errorf("shown early = %q, want %q", screen.String(), tt.wantEarly)
			}

			// When: the rest arrives
			_, _ = w.Write([]byte(tt.writes[len(tt.writes)-1]))
			w.flush()

			// Then: the whole output is shown with the secret hidden
			if screen.String() != tt.wantFinal {
				t.Errorf("shown = %q, want %q", screen.String(), tt.wantFinal)
			}
		})
	}
}

func TestTerraformUnlock_Arguments_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "tf unlock force unlocks the given lock ID",
		CurrentImpl:     "runUnlock validates its flags and calls terraform force-unlock",
		ExpectedOutcome: "force-unlock with -force only when confirmed, and no call when flags are missing",
		Rationale:       "Unlocking the wrong state, or unlocking without being asked, can corrupt it",
	}
	t.Logf("BEHAVIORAL CONTRACT: %s", contract.Behavior)

	tests := []struct {
		name      string
		lockID    string
		confirm   bool
		hasSource bool
		want      string
		wantErr   string
	}{
		{name: "confirmed", lockID: "abc-123", confirm: true, hasSource: true, want: "force-unlock -force abc-123"},
		{name: "not confirmed", lockID: "abc-123", hasSource: true, want: "force-unlock abc-123"},
		{name: "missing lock id", hasSource: true, wantErr: "lock ID is required"},
		{name: "missing source", lockID: "abc-123", wantErr: "terraform source directory is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a stub terraform and these flags
			log := fakeTerraform(t)
			flags := &commonFlags{awsRegion: "eu-west-1"}
			if tt.hasSource {
				flags.terraformSource = t.TempDir()
			}

			// When: unlock runs
			err := runUnlock(flags, tt.lockID, tt.confirm)

			// Then: it calls force-unlock as expected, or fails without calling terraform
			calls := terraformCalls(t, log)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				if len(calls) != 0 {
					t.Errorf("terraform ran despite the error: %q", calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("runUnlock: %v", err)
			}
			if len(calls) != 1 || calls[0] != tt.want {
				t.Errorf("terraform calls = %q, want [%q]", calls, tt.want)
			}
		})
	}
}
