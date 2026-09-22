package wrapper

import "testing"

func TestDiscoverPropertyPathOrFallback_ReturnsResolvedPath_WhenAvailable(t *testing.T) {
	meta := map[string]interface{}{"namespace": "legacy"}
	fallback := "__fallback__"

	path := DiscoverPropertyPathOrFallback(meta, "4.2.0", false, "wrappers", fallback)
	if path == "" {
		t.Fatalf("expected non-empty resolved path")
	}
	if path == fallback {
		t.Fatalf("expected schema-resolved path, got fallback %q", fallback)
	}
}

func TestDiscoverPropertyPathOrFallback_ReturnsFallback_WhenUnavailable(t *testing.T) {
	fallback := "desiredstate.meta.ecosystem"
	path := DiscoverPropertyPathOrFallback(nil, "0.0.0-does-not-exist", true, "metaEcosystem", fallback)

	if path != fallback {
		t.Fatalf("expected fallback path %q, got %q", fallback, path)
	}
}
