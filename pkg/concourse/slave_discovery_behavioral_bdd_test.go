package concourse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
)

func mustWriteSlaveFixtureFile(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", filePath, err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture file %s: %v", filePath, err)
	}
}

func TestParser_LoadDesiredStates_SlavesDeclaredInPart_BDD(t *testing.T) {
	yagoRoot := yagoRootFromTestFile(t)
	fixtureRoot, err := os.MkdirTemp(filepath.Join(yagoRoot, "tests", "assets"), "slave-discovery-fixture-")
	if err != nil {
		t.Fatalf("failed to create fixture dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixtureRoot)
	})

	parentDS := filepath.Join(fixtureRoot, "desiredstate-parent.yaml")
	parentPipelinesPart := filepath.Join(fixtureRoot, "pipelines.yaml")

	parentDSRel, err := filepath.Rel(yagoRoot, parentDS)
	if err != nil {
		t.Fatalf("failed to derive relative path for parent desiredstate: %v", err)
	}
	parentPipelinesPartRel, err := filepath.Rel(yagoRoot, parentPipelinesPart)
	if err != nil {
		t.Fatalf("failed to derive relative path for parent part: %v", err)
	}

	slaveDSRel := "tests/assets/4.2.0/desiredstates/concourse-cluster/desiredstate.yaml"
	configurationRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "configurations", "concourse-cluster", "configuration.yaml")

	parentDSContent := "---\n" +
		"schema: 4.2.0\n" +
		"desiredstate:\n" +
		"  meta:\n" +
		"    repo:\n" +
		"      url: git@github.com:example/parent.git\n" +
		"      branch: main\n" +
		"      tag: ''\n" +
		"      watch:\n" +
		"        - " + parentDSRel + "\n" +
		"    parts:\n" +
		"      self: " + parentDSRel + "\n" +
		"      terraform: tests/assets/4.2.0/desiredstates/concourse-cluster/terraform/terraform.yaml\n" +
		"      concourse: tests/assets/4.2.0/desiredstates/concourse-cluster/concourse/concourse.yaml\n" +
		"      pipelines: " + parentPipelinesPartRel + "\n"

	parentPipelinesPartContent := "---\n" +
		"desiredstate:\n" +
		"  meta:\n" +
		"    master_pipeline:\n" +
		"      master:\n" +
		"        is_self_updating: true\n" +
		"      slaves:\n" +
		"        - url: git@github.com:example/slave.git\n" +
		"          branch: main\n" +
		"          tag: ''\n" +
		"          path: " + slaveDSRel + "\n" +
		"          watch:\n" +
		"            - " + slaveDSRel + "\n"

	mustWriteSlaveFixtureFile(t, parentDS, parentDSContent)
	mustWriteSlaveFixtureFile(t, parentPipelinesPart, parentPipelinesPartContent)

	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	p := NewParser("all", nil)
	if err := p.LoadGitOpsFilesExtended(parentDS, configurationRoot, "", filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "pipelines"), false); err != nil {
		t.Fatalf("LoadGitOpsFilesExtended failed: %v", err)
	}

	slaves, err := p.getSlavePipelinesRepoListForCurrentDesiredState()
	if err != nil {
		t.Fatalf("getSlavePipelinesRepoListForCurrentDesiredState failed: %v", err)
	}
	if len(slaves) == 0 {
		t.Fatalf("expected at least one slave entry from part-declared master_pipeline.slaves, got %d", len(slaves))
	}
}
