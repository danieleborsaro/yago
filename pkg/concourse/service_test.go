package concourse

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

func TestResolveConcourseBuildDir_Priority(t *testing.T) {
	t.Run("uses explicit save path", func(t *testing.T) {
		save := filepath.Join(t.TempDir(), "custom-build")
		got, err := resolveConcourseBuildDir(save, "")
		if err != nil {
			t.Fatalf("resolveConcourseBuildDir returned error: %v", err)
		}
		if got != save {
			t.Fatalf("expected build dir %q, got %q", save, got)
		}
	})

	t.Run("uses pipelines workdir when save is empty", func(t *testing.T) {
		pipelinesWorkdir := t.TempDir()
		got, err := resolveConcourseBuildDir("", pipelinesWorkdir)
		if err != nil {
			t.Fatalf("resolveConcourseBuildDir returned error: %v", err)
		}
		if !strings.HasPrefix(filepath.Base(got), ".gitops-") {
			t.Fatalf("expected temp build dir with .gitops- prefix, got %q", got)
		}
		if _, statErr := os.Stat(got); statErr != nil {
			t.Fatalf("expected temp build dir to exist, stat failed: %v", statErr)
		}
	})

	t.Run("defaults to temp dir when both paths are empty", func(t *testing.T) {
		got, err := resolveConcourseBuildDir("", "")
		if err != nil {
			t.Fatalf("resolveConcourseBuildDir returned error: %v", err)
		}
		if !strings.HasPrefix(filepath.Base(got), ".gitops-") {
			t.Fatalf("expected temp build dir with .gitops- prefix, got %q", got)
		}
		if _, statErr := os.Stat(got); statErr != nil {
			t.Fatalf("expected temp build dir to exist, stat failed: %v", statErr)
		}
	})
}

func TestAssembleConcourse_ValidatesModeSelection(t *testing.T) {
	svc := NewService(".", false)

	_, err := svc.AssembleConcourse(ConcourseAssembleRequest{})
	if err == nil {
		t.Fatalf("expected mode validation error when no mode is selected")
	}
	if !strings.Contains(err.Error(), "exactly one of master/slave/local") {
		t.Fatalf("expected mode selection error, got: %v", err)
	}

	_, err = svc.AssembleConcourse(ConcourseAssembleRequest{IsMaster: true, IsLocal: true})
	if err == nil {
		t.Fatalf("expected mode validation error when multiple modes are selected")
	}
	if !strings.Contains(err.Error(), "exactly one of master/slave/local") {
		t.Fatalf("expected mode selection error, got: %v", err)
	}
}

func TestAssembleConcourse_LocalMode_ProducesArtifacts(t *testing.T) {
	yagoRoot := yagoRootFromTestFile(t)
	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	desiredstateRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "desiredstate.yaml")
	configurationRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "configurations", "concourse-cluster", "configuration.yaml")

	pipelinesWorkdir := t.TempDir()
	templateFile := filepath.Join(pipelinesWorkdir, "pipelines", "check-workers-and-tools-oci.yaml")
	if err := os.MkdirAll(filepath.Dir(templateFile), 0o755); err != nil {
		t.Fatalf("failed to create pipeline template directory: %v", err)
	}
	if err := os.WriteFile(templateFile, []byte("resources: []\njobs: []\n"), 0o644); err != nil {
		t.Fatalf("failed to write pipeline template fixture: %v", err)
	}

	buildDir := t.TempDir()
	svc := NewService(".", false)
	resp, err := svc.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  desiredstateRoot,
		ConfigurationRoot: configurationRoot,
		Environment:       "all",
		PipelinesWorkdir:  pipelinesWorkdir,
		Save:              buildDir,
		IsLocal:           true,
	})
	if err != nil {
		t.Fatalf("AssembleConcourse returned error: %v", err)
	}

	if resp.BuildDirectory != buildDir {
		t.Fatalf("expected build directory %q, got %q", buildDir, resp.BuildDirectory)
	}
	if resp.TotalPipelines < 1 {
		t.Fatalf("expected at least one pipeline, got %d", resp.TotalPipelines)
	}
	if len(resp.Pipelines) != resp.TotalPipelines {
		t.Fatalf("expected %d pipeline artifacts, got %d", resp.TotalPipelines, len(resp.Pipelines))
	}

	for _, artifact := range resp.Pipelines {
		if artifact.PipelineName == "" {
			t.Fatalf("expected pipeline name in artifact: %+v", artifact)
		}
		if artifact.ConfigurationFile == "" || artifact.TemplateFile == "" {
			t.Fatalf("expected non-empty artifact file paths: %+v", artifact)
		}

		if _, statErr := os.Stat(artifact.ConfigurationFile); statErr != nil {
			t.Fatalf("configuration artifact missing: %v", statErr)
		}
		if _, statErr := os.Stat(artifact.TemplateFile); statErr != nil {
			t.Fatalf("template artifact missing: %v", statErr)
		}

		if !strings.HasPrefix(filepath.Clean(artifact.ConfigurationFile), filepath.Clean(buildDir)+string(filepath.Separator)) {
			t.Fatalf("configuration artifact should be under build dir %q, got %q", buildDir, artifact.ConfigurationFile)
		}
		if !strings.HasPrefix(filepath.Clean(artifact.TemplateFile), filepath.Clean(buildDir)+string(filepath.Separator)) {
			t.Fatalf("template artifact should be under build dir %q, got %q", buildDir, artifact.TemplateFile)
		}
	}
}
