package wrapper

import (
	"testing"
)

func TestConfigRepoLocatorsFromMeta_IncludesSchemaAndFallbackPaths(t *testing.T) {
	meta := map[string]interface{}{
		"namespace": "legacy",
	}
	locators := ConfigRepoLocatorsFromMeta("concourse", "all", meta, "4.2.0")
	if len(locators) == 0 {
		t.Fatalf("expected non-empty locator candidates")
	}

	expectedFirst := "desiredstate.content.environments.all.configurations.concourse"
	if locators[0] != expectedFirst {
		t.Fatalf("expected first locator %q, got %q", expectedFirst, locators[0])
	}

	contains := func(target string) bool {
		for _, locator := range locators {
			if locator == target {
				return true
			}
		}
		return false
	}

	if !contains("desiredstate.content.environments.all.configurations.concourse.git") {
		t.Fatalf("expected .git locator candidate in %v", locators)
	}
	if !contains("desiredstate.configuration.all.concourse") {
		t.Fatalf("expected legacy locator candidate in %v", locators)
	}
	if !contains("desiredstate.configuration.all.concourse.git") {
		t.Fatalf("expected legacy .git locator candidate in %v", locators)
	}
}
