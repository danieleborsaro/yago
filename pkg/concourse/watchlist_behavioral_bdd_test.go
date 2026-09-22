package concourse

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/danieleborsaro/yago/internal/schema"
	"gopkg.in/yaml.v3"
)

func mustWriteFile(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", filePath, err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", filePath, err)
	}
}

func toStringMapForWatch(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	m, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", value)
	}
	return m
}

func extractWatchListFromAssembledConfig(t *testing.T, filePath string) []string {
	t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read assembled config: %v", err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("failed to parse assembled config yaml: %v", err)
	}

	desiredstate := toStringMapForWatch(t, root["desiredstate"])
	meta := toStringMapForWatch(t, desiredstate["meta"])
	repo := toStringMapForWatch(t, meta["repo"])

	raw, ok := repo["watch"].([]interface{})
	if !ok {
		t.Fatalf("expected desiredstate.meta.repo.watch as []interface{}, got %T", repo["watch"])
	}

	watch := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("expected watch item as string, got %T", item)
		}
		watch = append(watch, s)
	}

	return watch
}

func assertWatchContains(t *testing.T, watch []string, expected string) {
	t.Helper()
	for _, item := range watch {
		if item == expected {
			return
		}
	}
	t.Fatalf("expected watchlist to contain %q, got %v", expected, watch)
}

func assertWatchNotContains(t *testing.T, watch []string, forbidden string) {
	t.Helper()
	for _, item := range watch {
		if item == forbidden {
			t.Fatalf("expected watchlist to not contain %q, got %v", forbidden, watch)
		}
	}
}

func extractChildEcosystemWatchList(t *testing.T, filePath string, mountPoint string) []string {
	t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read assembled config: %v", err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("failed to parse assembled config yaml: %v", err)
	}

	desiredstate := toStringMapForWatch(t, root["desiredstate"])
	ecosystem := toStringMapForWatch(t, desiredstate["ecosystem"])
	mount := toStringMapForWatch(t, ecosystem[mountPoint])
	childDesiredstate := toStringMapForWatch(t, mount["desiredstate"])
	childMeta := toStringMapForWatch(t, childDesiredstate["meta"])
	childRepo := toStringMapForWatch(t, childMeta["repo"])

	raw, ok := childRepo["watch"].([]interface{})
	if !ok {
		t.Fatalf("expected desiredstate.ecosystem.%s.desiredstate.meta.repo.watch as []interface{}, got %T", mountPoint, childRepo["watch"])
	}

	watch := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("expected child watch item as string, got %T", item)
		}
		watch = append(watch, s)
	}

	return watch
}

func createWatchlistFixture(t *testing.T) (string, string, string, string, string, string, string) {
	t.Helper()
	yagoRoot := yagoRootFromTestFile(t)
	fixtureRoot, err := os.MkdirTemp(filepath.Join(yagoRoot, "tests", "assets"), "watchlist-fixture-")
	if err != nil {
		t.Fatalf("failed to create watchlist fixture dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(fixtureRoot)
	})

	parentDS := filepath.Join(fixtureRoot, "desiredstate-parent.yaml")
	childDS := filepath.Join(fixtureRoot, "desiredstate-child.yaml")
	unusedCfg := filepath.Join(fixtureRoot, "unused-configuration.yaml")

	parentDSRel, err := filepath.Rel(yagoRoot, parentDS)
	if err != nil {
		t.Fatalf("failed to derive relative path for parent desiredstate: %v", err)
	}
	childDSRel, err := filepath.Rel(yagoRoot, childDS)
	if err != nil {
		t.Fatalf("failed to derive relative path for child desiredstate: %v", err)
	}
	unusedCfgRel, err := filepath.Rel(yagoRoot, unusedCfg)
	if err != nil {
		t.Fatalf("failed to derive relative path for unused configuration: %v", err)
	}

	parentWatch := "parent/{**,.}/*"
	ecoWatch := "ecosystem/{**,.}/*"
	slaveWatch := "slave/{**,.}/*"

	terraformPart := filepath.Join("tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "terraform", "terraform.yaml")
	concoursePart := filepath.Join("tests", "assets", "4.2.0", "desiredstates", "concourse-cluster", "concourse", "concourse.yaml")
	configurationRoot := filepath.Join(yagoRoot, "tests", "assets", "4.2.0", "configurations", "concourse-cluster", "configuration.yaml")

	childContent := fmt.Sprintf(`---
schema: 4.2.0
desiredstate:
  meta:
    repo:
      url: git@github.com:example/child.git
      branch: main
      tag: ''
      watch:
        - child-own/{**,.}/*
    parts:
      self: %s
  content:
    environments:
      all:
        configurations:
          concourse:
            url: git@github.com:example/config.git
            branch: main
            tag: ''
            path: %s
            watch: []
`, childDSRel, unusedCfgRel)

	parentContent := fmt.Sprintf(`---
schema: 4.2.0
desiredstate:
  meta:
    repo:
      url: git@github.com:example/parent.git
      branch: main
      tag: ''
      watch:
        - %s
    parts:
      self: %s
      terraform: %s
      concourse: %s
    ecosystem:
      child-eco:
        url: git@github.com:example/child.git
        branch: main
        tag: ''
        path: %s
        watch:
          - %s
    master_pipeline:
      master:
        is_self_updating: false
      slaves:
        - url: git@github.com:example/child.git
          branch: main
          tag: ''
          path: %s
          watch:
            - %s
`, parentWatch, parentDSRel, terraformPart, concoursePart, childDSRel, ecoWatch, childDSRel, slaveWatch)

	mustWriteFile(t, unusedCfg, "dummy: true\n")
	mustWriteFile(t, childDS, childContent)
	mustWriteFile(t, parentDS, parentContent)

	return parentDS, configurationRoot, parentWatch, ecoWatch, slaveWatch, yagoRoot, fixtureRoot
}

func TestAssembleConcourse_Watchlist_EcosystemAndSlaveUnion_BDD(t *testing.T) {
	parentDS, configurationRoot, parentWatch, ecoWatch, slaveWatch, yagoRoot, _ := createWatchlistFixture(t)
	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	buildDir := t.TempDir()
	pipelinesWorkdir := t.TempDir()
	templateFile := filepath.Join(pipelinesWorkdir, "pipelines", "check-workers-and-tools-oci.yaml")
	mustWriteFile(t, templateFile, "resources: []\njobs: []\n")

	svc := NewService(".", false)
	resp, err := svc.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  parentDS,
		ConfigurationRoot: configurationRoot,
		Environment:       "all",
		PipelinesWorkdir:  pipelinesWorkdir,
		Save:              buildDir,
		IsLocal:           true,
	})
	if err != nil {
		t.Fatalf("AssembleConcourse returned error: %v", err)
	}
	if len(resp.Pipelines) == 0 {
		t.Fatalf("expected at least one pipeline artifact")
	}

	watch := extractWatchListFromAssembledConfig(t, resp.Pipelines[0].ConfigurationFile)
	assertWatchContains(t, watch, parentWatch)
	assertWatchContains(t, watch, ecoWatch)
	assertWatchContains(t, watch, slaveWatch)
}

func TestAssembleConcourse_Watchlist_OverrideSelfUpdatingToTrue_BDD(t *testing.T) {
	parentDS, configurationRoot, parentWatch, _, _, yagoRoot, _ := createWatchlistFixture(t)
	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	buildDir := t.TempDir()
	pipelinesWorkdir := t.TempDir()
	templateFile := filepath.Join(pipelinesWorkdir, "pipelines", "check-workers-and-tools-oci.yaml")
	mustWriteFile(t, templateFile, "resources: []\njobs: []\n")

	svc := NewService(".", false)
	resp, err := svc.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  parentDS,
		ConfigurationRoot: configurationRoot,
		Environment:       "all",
		PipelinesWorkdir:  pipelinesWorkdir,
		Save:              buildDir,
		IsLocal:           true,
	})
	if err != nil {
		t.Fatalf("AssembleConcourse returned error: %v", err)
	}
	if len(resp.Pipelines) == 0 {
		t.Fatalf("expected at least one pipeline artifact")
	}

	watch := extractWatchListFromAssembledConfig(t, resp.Pipelines[0].ConfigurationFile)
	assertWatchContains(t, watch, parentWatch)
}

func TestAssembleConcourse_Watchlist_ChildEcosystemIsolation_BDD(t *testing.T) {
	parentDS, configurationRoot, _, ecoWatch, slaveWatch, yagoRoot, _ := createWatchlistFixture(t)
	chdirForTest(t, yagoRoot)
	prevNamespace := schema.GetNamespaceOverride()
	schema.SetNamespaceOverride("legacy")
	t.Cleanup(func() {
		schema.SetNamespaceOverride(prevNamespace)
	})

	buildDir := t.TempDir()
	pipelinesWorkdir := t.TempDir()
	templateFile := filepath.Join(pipelinesWorkdir, "pipelines", "check-workers-and-tools-oci.yaml")
	mustWriteFile(t, templateFile, "resources: []\njobs: []\n")

	svc := NewService(".", false)
	resp, err := svc.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  parentDS,
		ConfigurationRoot: configurationRoot,
		Environment:       "all",
		PipelinesWorkdir:  pipelinesWorkdir,
		Save:              buildDir,
		IsLocal:           true,
	})
	if err != nil {
		t.Fatalf("AssembleConcourse returned error: %v", err)
	}
	if len(resp.Pipelines) == 0 {
		t.Fatalf("expected at least one pipeline artifact")
	}

	childWatch := extractChildEcosystemWatchList(t, resp.Pipelines[0].ConfigurationFile, "child-eco")
	assertWatchContains(t, childWatch, "child-own/{**,.}/*")
	assertWatchNotContains(t, childWatch, ecoWatch)
	assertWatchNotContains(t, childWatch, slaveWatch)
}
