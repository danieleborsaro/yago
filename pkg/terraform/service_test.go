package terraform

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

func TestAssembleTerraform_RequiresDesiredStateFile(t *testing.T) {
	svc := NewService(".", false)
	_, err := svc.AssembleTerraform(TerraformAssembleRequest{})
	if err == nil {
		t.Fatalf("expected error when desiredstate file is missing")
	}
	if !strings.Contains(err.Error(), "desiredstate file must be specified") {
		t.Fatalf("expected desiredstate required error, got: %v", err)
	}
}

func TestAssembleTerraform_FixtureAssemble_CachesBackendAndDocuments(t *testing.T) {
	yagoRoot := yagoRootFromTestFile(t)
	chdirForTest(t, yagoRoot)

	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	desiredstateRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "desiredstate.yaml")
	configurationRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "configurations", "concourse-cluster", "configuration.yaml")
	terraformSource := t.TempDir()

	svc := NewService(".", false)
	resp, err := svc.AssembleTerraform(TerraformAssembleRequest{
		DesiredStateFile: desiredstateRoot,
		ConfigFile:       configurationRoot,
		Environment:      "all",
		AWSRegion:        "eu-west-1",
		TerraformSource:  terraformSource,
		OutputFormat:     "",
	})
	if err != nil {
		t.Fatalf("AssembleTerraform returned error: %v", err)
	}

	expectedBuildDir := filepath.Join(terraformSource, ".gitops")
	if resp.BuildDirectory != expectedBuildDir {
		t.Fatalf("expected build directory %q, got %q", expectedBuildDir, resp.BuildDirectory)
	}
	if resp.CodeDirectory != terraformSource {
		t.Fatalf("expected code directory %q, got %q", terraformSource, resp.CodeDirectory)
	}
	if resp.SourceCodeCloned {
		t.Fatalf("expected local terraform source to avoid cloning")
	}

	if resp.AssembleResponse == nil {
		t.Fatalf("expected embedded assemble response")
	}
	if !resp.DesiredStateAssembled || !resp.ConfigurationAssembled {
		t.Fatalf("expected desiredstate and configuration assembled, got ds=%v cfg=%v", resp.DesiredStateAssembled, resp.ConfigurationAssembled)
	}

	for _, path := range []string{resp.DesiredStateFile, resp.ConfigurationFile, resp.BackendFile} {
		if path == "" {
			t.Fatalf("expected non-empty artifact path")
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("artifact not found at %q: %v", path, statErr)
		}
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(expectedBuildDir)+string(filepath.Separator)) {
			t.Fatalf("artifact path should be under build dir %q, got %q", expectedBuildDir, path)
		}
	}

	if !strings.HasSuffix(resp.DesiredStateFile, ".tfvars.json") {
		t.Fatalf("expected desiredstate artifact to use json extension, got %q", resp.DesiredStateFile)
	}
	if !strings.HasSuffix(resp.ConfigurationFile, ".tfvars.json") {
		t.Fatalf("expected configuration artifact to use json extension, got %q", resp.ConfigurationFile)
	}
	if !strings.HasSuffix(resp.BackendFile, DefaultBackendFileSuffix) {
		t.Fatalf("expected backend artifact suffix %q, got %q", DefaultBackendFileSuffix, resp.BackendFile)
	}
}
