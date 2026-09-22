package desiredstate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
)

func yagoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read current working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change working directory to %q: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}

func TestRunValidate_RequiresWrapper(t *testing.T) {
	config := &DesiredStateConfig{
		Environment:      "all",
		DesiredStateFile: "unused.yaml",
		Wrapper:          "",
	}

	err := runValidate(config, nil)
	if err == nil {
		t.Fatalf("expected wrapper validation error")
	}
	if !strings.Contains(err.Error(), "--wrapper is mandatory") {
		t.Fatalf("expected wrapper mandatory error, got: %v", err)
	}
}

func TestRunAssemble_RequiresWrapper(t *testing.T) {
	config := &DesiredStateConfig{
		Environment:      "all",
		DesiredStateFile: "unused.yaml",
		Wrapper:          "",
	}

	err := runAssemble(config, nil)
	if err == nil {
		t.Fatalf("expected wrapper validation error")
	}
	if !strings.Contains(err.Error(), "--wrapper is mandatory") {
		t.Fatalf("expected wrapper mandatory error, got: %v", err)
	}
}

func TestValidateAssembleEnvironmentExistsInDesiredState_WithFixture(t *testing.T) {
	yagoRoot := yagoRootFromTestFile(t)
	chdirForTest(t, yagoRoot)

	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	desiredstateRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "desiredstate.yaml")

	if err := validateAssembleEnvironmentExistsInDesiredState(desiredstateRoot, "all"); err != nil {
		t.Fatalf("expected environment 'all' to be valid, got: %v", err)
	}

	err := validateAssembleEnvironmentExistsInDesiredState(desiredstateRoot, "dev")
	if err == nil {
		t.Fatalf("expected error for undefined environment")
	}
	if !strings.Contains(err.Error(), "available: all") {
		t.Fatalf("expected available environments hint, got: %v", err)
	}
}
