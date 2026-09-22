package desiredstate

import (
	"testing"
)

// TestComponentParser_DockerHandlerIntegration verifies that the parser correctly
// uses the Docker handler from the registry to parse Docker components.
func TestComponentParser_DockerHandlerIntegration(t *testing.T) {
	parser := NewComponentParser(false, false)

	// Test data simulating a Docker component in YAML
	yamlData := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"components": map[string]interface{}{
					"artifacts": map[string]interface{}{
						"my-app": map[string]interface{}{
							"docker": map[string]interface{}{
								"eu-west-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.eu-west-1.amazonaws.com/my-app",
									"tag":   "1.2.3",
								},
							},
						},
					},
				},
			},
		},
	}

	// Parse the component
	err := parser.ParseFromYAML(yamlData, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse Docker component: %v", err)
	}

	// Verify the component was parsed
	components := parser.GetManager().GetAllComponents()
	if len(components) != 1 {
		t.Fatalf("Expected 1 component, got %d", len(components))
	}

	comp := components[0]

	// Verify component details
	if comp.Type != ComponentTypeDocker {
		t.Errorf("Expected type %s, got %s", ComponentTypeDocker, comp.Type)
	}
	if comp.URL != "123456789.dkr.ecr.eu-west-1.amazonaws.com/my-app" {
		t.Errorf("Expected URL '123456789.dkr.ecr.eu-west-1.amazonaws.com/my-app', got '%s'", comp.URL)
	}
	if comp.Version != "1.2.3" {
		t.Errorf("Expected version '1.2.3', got '%s'", comp.Version)
	}
	if comp.Region != "eu-west-1" {
		t.Errorf("Expected region 'eu-west-1', got '%s'", comp.Region)
	}
	if !comp.IsLocked {
		t.Error("Expected component to be locked with specific version")
	}
	if comp.PartID != "artifacts.my-app.docker.eu-west-1" {
		t.Errorf("Expected PartID 'artifacts.my-app.docker.eu-west-1', got '%s'", comp.PartID)
	}

	// Verify handler-specific metadata was set
	if comp.Metadata == nil {
		t.Error("Expected metadata to be set by handler")
	}
}

// TestComponentParser_DockerDigestIntegration verifies that Docker components
// with digests are correctly parsed and locked.
func TestComponentParser_DockerDigestIntegration(t *testing.T) {
	parser := NewComponentParser(false, false)

	// Test data with digest
	yamlData := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"components": map[string]interface{}{
					"artifacts": map[string]interface{}{
						"my-app": map[string]interface{}{
							"docker": map[string]interface{}{
								"us-east-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/my-app",
									"tag":   "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
								},
							},
						},
					},
				},
			},
		},
	}

	// Parse the component
	err := parser.ParseFromYAML(yamlData, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse Docker component with digest: %v", err)
	}

	// Verify the component
	components := parser.GetManager().GetAllComponents()
	if len(components) != 1 {
		t.Fatalf("Expected 1 component, got %d", len(components))
	}

	comp := components[0]

	// Verify digest parsing
	if comp.Version != "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Errorf("Expected digest version, got '%s'", comp.Version)
	}
	if !comp.IsLocked {
		t.Error("Expected component with digest to be locked")
	}

	// Verify is_digest metadata flag
	if comp.Metadata == nil {
		t.Fatal("Expected metadata to be set")
	}
	isDigest, ok := comp.Metadata["is_digest"].(bool)
	if !ok || !isDigest {
		t.Error("Expected is_digest metadata to be true")
	}
}

// TestComponentParser_DockerLatestIntegration verifies that Docker components
// with "latest" tag are correctly parsed as unlocked.
func TestComponentParser_DockerLatestIntegration(t *testing.T) {
	parser := NewComponentParser(false, false)

	// Test data with latest tag
	yamlData := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"components": map[string]interface{}{
					"artifacts": map[string]interface{}{
						"my-app": map[string]interface{}{
							"docker": map[string]interface{}{
								"ap-southeast-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.ap-southeast-1.amazonaws.com/my-app",
									"tag":   "latest",
								},
							},
						},
					},
				},
			},
		},
	}

	// Parse the component
	err := parser.ParseFromYAML(yamlData, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse Docker component with latest tag: %v", err)
	}

	// Verify the component
	components := parser.GetManager().GetAllComponents()
	if len(components) != 1 {
		t.Fatalf("Expected 1 component, got %d", len(components))
	}

	comp := components[0]

	// Verify latest tag parsing
	if comp.Version != "latest" {
		t.Errorf("Expected version 'latest', got '%s'", comp.Version)
	}
	if comp.IsLocked {
		t.Error("Expected component with 'latest' tag to be unlocked")
	}
}

// TestComponentParser_DockerMultiRegionIntegration verifies that multi-region
// Docker components are correctly parsed.
func TestComponentParser_DockerMultiRegionIntegration(t *testing.T) {
	parser := NewComponentParser(false, false)

	// Test data with multiple regions
	yamlData := map[string]interface{}{
		"desiredstate": map[string]interface{}{
			"content": map[string]interface{}{
				"components": map[string]interface{}{
					"artifacts": map[string]interface{}{
						"my-app": map[string]interface{}{
							"docker": map[string]interface{}{
								"eu-west-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.eu-west-1.amazonaws.com/my-app",
									"tag":   "1.0.0",
								},
								"us-east-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.us-east-1.amazonaws.com/my-app",
									"tag":   "1.0.0",
								},
								"ap-southeast-1": map[string]interface{}{
									"image": "123456789.dkr.ecr.ap-southeast-1.amazonaws.com/my-app",
									"tag":   "1.0.0",
								},
							},
						},
					},
				},
			},
		},
	}

	// Parse the components
	err := parser.ParseFromYAML(yamlData, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse multi-region Docker components: %v", err)
	}

	// Verify all regions were parsed
	components := parser.GetManager().GetAllComponents()
	if len(components) != 3 {
		t.Fatalf("Expected 3 components (one per region), got %d", len(components))
	}

	// Verify each region
	regions := make(map[string]bool)
	for _, comp := range components {
		if comp.Type != ComponentTypeDocker {
			t.Errorf("Expected type %s, got %s", ComponentTypeDocker, comp.Type)
		}
		regions[comp.Region] = true
	}

	expectedRegions := []string{"eu-west-1", "us-east-1", "ap-southeast-1"}
	for _, region := range expectedRegions {
		if !regions[region] {
			t.Errorf("Expected region '%s' to be parsed", region)
		}
	}
}
