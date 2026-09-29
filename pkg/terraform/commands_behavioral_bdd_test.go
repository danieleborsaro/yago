package terraform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
