package core

import (
	"path/filepath"
	"runtime"
	"testing"
)

// yagoRootFromOrchestrationTestFile locates the yago repository root from this test file's path.
func yagoRootFromOrchestrationTestFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestGitOpsDocument_LoadEcosystem_Yago200_BehavioralBDD is a real, executable
// regression test (unlike TestGitOpsDocument_LoadEcosystem_BehavioralBDD, which is
// narrative-only) proving that orchestration ("ecosystem") loading actually works
// end-to-end against the yago 2.0.0 schema, using its "orchestration" property name.
//
// This closes a coverage gap: previously, the only functional (non-narrative)
// ecosystem test exercised schema 4.2.0 (namespace "legacy") exclusively, and the
// real fixture at tests/assets/2.0.0/desiredstates/concourse-cluster/ was unused
// by any test - so a schema-vs-fixture property name mismatch here would not have
// been caught.
func TestGitOpsDocument_LoadEcosystem_Yago200_BehavioralBDD(t *testing.T) {
	yagoRoot := yagoRootFromOrchestrationTestFile(t)
	desiredStatePath := filepath.Join(yagoRoot, "tests", "assets", "2.0.0", "desiredstates", "concourse-cluster", "desiredstate.yaml")

	doc := NewGitOpsDocument()
	if err := doc.LoadGitOpsFile(desiredStatePath, true, nil); err != nil {
		t.Fatalf("LoadGitOpsFile failed: %v", err)
	}

	if doc.GetSchemaVersion() != "2.0.0" {
		t.Fatalf("expected schema version 2.0.0, got %q", doc.GetSchemaVersion())
	}

	content := doc.GetContent()
	if content == nil || content.Data == nil {
		t.Fatal("expected assembled content to be loaded")
	}

	ds, ok := content.Data["desiredstate"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'desiredstate' key in assembled content, got: %v", content.Data)
	}

	orchestration, ok := ds["orchestration"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'orchestration' key mounted at top-level desiredstate, got keys: %v", mapKeys(ds))
	}

	for _, mountPoint := range []string{"foo", "foo2"} {
		mounted, ok := orchestration[mountPoint].(map[string]interface{})
		if !ok || len(mounted) == 0 {
			t.Errorf("expected non-empty mounted content for orchestration mount point %q, got: %v", mountPoint, orchestration[mountPoint])
			continue
		}
		if _, hasDesiredState := mounted["desiredstate"]; !hasDesiredState {
			t.Errorf("expected mounted content for %q to contain a 'desiredstate' key, got keys: %v", mountPoint, mapKeys(mounted))
		}
	}
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
