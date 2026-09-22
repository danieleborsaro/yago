package core

import "testing"

func TestGetDesiredStatePathWithFallback_UsesCustom420SchemaKey_Phase3(t *testing.T) {
	doc := NewGitOpsDocument()
	doc.schemaVersion = "4.2.0"
	doc.assembledMeta.Data = map[string]interface{}{
		"namespace": "legacy",
	}

	path := doc.getDesiredStatePathWithFallback("masterPipelineRoot", "fallback.path")
	if path != "desiredstate.meta.master_pipeline" {
		t.Fatalf("expected custom 4.2 path, got %q", path)
	}
}

func TestGetDesiredStatePathWithFallback_UsesYago200SchemaKey_Phase3(t *testing.T) {
	doc := NewGitOpsDocument()
	doc.schemaVersion = "2.0.0"
	doc.assembledMeta.Data = map[string]interface{}{
		"namespace": "yago",
	}

	path := doc.getDesiredStatePathWithFallback("masterPipelineRoot", "fallback.path")
	if path != "desiredstate.meta.pipelines_as_code" {
		t.Fatalf("expected yago 2.0 path, got %q", path)
	}
}

func TestGetDesiredStatePathWithFallback_ReturnsFallbackForMissingKey_Phase3(t *testing.T) {
	doc := NewGitOpsDocument()
	doc.schemaVersion = "4.2.0"
	doc.assembledMeta.Data = map[string]interface{}{
		"namespace": "legacy",
	}

	path := doc.getDesiredStatePathWithFallback("thisKeyDoesNotExist", "fallback.path")
	if path != "fallback.path" {
		t.Fatalf("expected fallback path, got %q", path)
	}
}
