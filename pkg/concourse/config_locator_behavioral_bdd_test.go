package concourse

import (
	"testing"

	yamlparser "github.com/danieleborsaro/yago/internal/parser"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

func TestParser_ResolveConfigRepoLocator_SchemaDriven_BDD(t *testing.T) {
	p := NewParser("all", nil)

	dsMeta := map[string]interface{}{
		"namespace": "legacy",
	}
	dsContent := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"environments": map[string]interface{}{
					"all": map[string]interface{}{
						"configurations": map[string]interface{}{
							"concourse": map[string]interface{}{
								"url":  "git@github.com:example/config.git",
								"path": "tests/assets/4.2.0/configurations/concourse-cluster/configuration.yaml",
							},
						},
					},
				},
			},
		},
	}

	locator, err := p.resolveConfigRepoLocator(dsContent, dsMeta, "4.2.0")
	if err != nil {
		t.Fatalf("resolveConfigRepoLocator returned error: %v", err)
	}

	expected := "desiredstate.content.environments.all.configurations.concourse"
	if locator != expected {
		t.Fatalf("expected locator %q, got %q", expected, locator)
	}
}

func TestParser_ResolveConfigRepoLocator_ContractWithWrapperCandidates_BDD(t *testing.T) {
	p := NewParser("all", nil)

	dsMeta := map[string]interface{}{
		"namespace": "legacy",
	}

	candidates := wrapper.ConfigRepoLocatorsFromMeta("concourse", "all", dsMeta, "4.2.0")
	if len(candidates) < 2 {
		t.Fatalf("expected at least two locator candidates, got %v", candidates)
	}

	// Populate only the second candidate to prove parser selection follows
	// the same candidate ordering as wrapper.ConfigRepoLocatorsFromMeta.
	dsContent := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"environments": map[string]interface{}{
					"all": map[string]interface{}{
						"configurations": map[string]interface{}{
							"concourse": map[string]interface{}{
								"git": map[string]interface{}{
									"url":  "git@github.com:example/config.git",
									"path": "tests/assets/4.2.0/configurations/concourse-cluster/configuration.yaml",
								},
							},
						},
					},
				},
			},
		},
	}

	h := yamlparser.NewYAMLHandler("")
	expected := ""
	for _, candidate := range candidates {
		if _, err := h.GetValue(dsContent, candidate); err == nil {
			expected = candidate
			break
		}
	}
	if expected == "" {
		t.Fatalf("expected at least one resolvable candidate from %v", candidates)
	}

	resolved, err := p.resolveConfigRepoLocator(dsContent, dsMeta, "4.2.0")
	if err != nil {
		t.Fatalf("resolveConfigRepoLocator returned error: %v", err)
	}

	if resolved != expected {
		t.Fatalf("expected parser to resolve %q, got %q (candidates: %v)", expected, resolved, candidates)
	}
}
