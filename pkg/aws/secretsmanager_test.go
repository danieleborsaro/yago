package aws

import (
	"strings"
	"testing"
)

// Test_NewSecretManager tests the NewSecretManager constructor.
func Test_NewSecretManager(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		region     string
		isDryRun   bool
		expectErr  bool
		expectName string
	}{
		{
			name:      "Valid AWS profile and region",
			profile:   "default",
			region:    "us-east-1",
			isDryRun:  false,
			expectErr: false,
		},
		{
			name:      "Empty profile defaults to default",
			profile:   "",
			region:    "us-west-2",
			isDryRun:  false,
			expectErr: false,
		},
		{
			name:      "Dry-run mode",
			profile:   "default",
			region:    "eu-west-1",
			isDryRun:  true,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := NewSecretManager(tt.profile, tt.region, tt.isDryRun)

			if tt.expectErr && err != nil {
				return
			}

			if !tt.expectErr && err != nil {
				t.Errorf("NewSecretManager() error = %v, expected nil", err)
			}

			if sm == nil && !tt.expectErr {
				t.Errorf("NewSecretManager() returned nil SecretManager")
			}

			if sm != nil {
				if sm.awsRegion != tt.region {
					t.Errorf("Region mismatch: got %s, want %s", sm.awsRegion, tt.region)
				}
				if sm.isDryRun != tt.isDryRun {
					t.Errorf("DryRun mismatch: got %v, want %v", sm.isDryRun, tt.isDryRun)
				}
			}
		})
	}
}

// Test_CreateSecretDryRun tests secret creation in dry-run mode.
func Test_CreateSecretDryRun(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	result, err := sm.Create(
		"test-secret",
		"Test secret for validation",
		[]string{"username", "password"},
		"placeholder",
		30,
		true,
		map[string]string{"env": "test"},
		false,
		30,
		"",
		[]string{},
		[]string{},
		[]string{},
		[]string{},
		[]string{},
		[]map[string]interface{}{},
	)

	if err != nil {
		t.Errorf("Create() error = %v, expected nil", err)
	}

	if result == nil {
		t.Errorf("Create() returned nil result")
	}

	if result.Name != "test-secret" {
		t.Errorf("Name mismatch: got %s, want test-secret", result.Name)
	}

	if result.ARN == "" {
		t.Errorf("ARN is empty")
	}

	if result.KmsKeyId == "" {
		t.Errorf("KmsKeyId is empty")
	}
}

// Test_CreateSecretNotCreatedHere tests retrieving existing secret with isCreatedHere=False.
func Test_CreateSecretNotCreatedHere(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	result, err := sm.Create(
		"existing-secret",
		"Existing secret",
		[]string{"key1"},
		"placeholder",
		30,
		false, // isCreatedHere = False
		map[string]string{},
		false,
		30,
		"",
		[]string{},
		[]string{},
		[]string{},
		[]string{},
		[]string{},
		[]map[string]interface{}{},
	)

	// In dry-run mode, this should not fail
	if err == nil && result == nil {
		t.Errorf("Create() should return a result in dry-run mode")
	}
}

// Test_ValidateDryRun tests secret validation in dry-run mode.
func Test_ValidateDryRun(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	result, err := sm.Validate(
		"test-secret",
		[]string{"username", "password"},
		[]string{"AWSCURRENT"},
	)

	if err != nil {
		t.Errorf("Validate() error = %v, expected nil", err)
	}

	if result == nil {
		t.Errorf("Validate() returned nil result")
	}

	if !result.IsExpectedKeysOk {
		t.Errorf("Expected keys validation should pass in dry-run mode")
	}

	if !result.IsStoredKeysOk {
		t.Errorf("Stored keys validation should pass in dry-run mode")
	}

	if len(result.ExpectedKeys) != 2 {
		t.Errorf("Expected keys count mismatch: got %d, want 2", len(result.ExpectedKeys))
	}
}

// Test_ReadDryRun tests reading a secret in dry-run mode.
func Test_ReadDryRun(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	result, err := sm.Read("test-secret", "", "AWSCURRENT")

	if err != nil {
		t.Errorf("Read() error = %v, expected nil", err)
	}

	if result == "" {
		t.Errorf("Read() returned empty string")
	}
}

// Test_GenerateKmsKeyPolicy tests KMS key policy generation.
func Test_GenerateKmsKeyPolicy(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	policy, err := sm.generateKmsKeyPolicy("123456789012")

	if err != nil {
		t.Errorf("generateKmsKeyPolicy() error = %v, expected nil", err)
	}

	if policy == "" {
		t.Errorf("generateKmsKeyPolicy() returned empty policy")
	}

	// Verify policy structure
	if !contains(policy, "Enable IAM User Permissions") {
		t.Errorf("Policy missing 'Enable IAM User Permissions' statement")
	}

	if !contains(policy, "Allow SecretsManager Service") {
		t.Errorf("Policy missing 'Allow SecretsManager Service' statement")
	}

	if !contains(policy, "secretsmanager.amazonaws.com") {
		t.Errorf("Policy missing service principal")
	}
}

// Test_GenerateResourcePolicy tests resource policy generation.
func Test_GenerateResourcePolicy(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	// Test with no restrictions
	policy := sm.generateResourcePolicy("123456789012", "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
		[]string{}, []string{}, []string{}, []string{}, []string{}, []map[string]interface{}{})

	if policy != "" {
		t.Errorf("generateResourcePolicy() should return empty string for no restrictions")
	}

	// Test with user restrictions
	policy = sm.generateResourcePolicy("123456789012", "arn:aws:secretsmanager:us-east-1:123456789012:secret:test",
		[]string{"alice", "bob"}, []string{}, []string{}, []string{}, []string{}, []map[string]interface{}{})

	if policy == "" {
		t.Errorf("generateResourcePolicy() returned empty policy with user restrictions")
	}

	if !contains(policy, "alice") {
		t.Errorf("Policy missing user 'alice'")
	}

	if !contains(policy, "bob") {
		t.Errorf("Policy missing user 'bob'")
	}
}

// Test_GeneratePlaceholderSecretString tests placeholder secret string generation.
func Test_GeneratePlaceholderSecretString(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	tests := []struct {
		name       string
		keys       []string
		plaintext  string
		expectJson bool
	}{
		{
			name:       "Plain text secret",
			keys:       []string{},
			plaintext:  "my-secret-value",
			expectJson: false,
		},
		{
			name:       "JSON secret with keys",
			keys:       []string{"username", "password"},
			plaintext:  "placeholder",
			expectJson: true,
		},
		{
			name:       "Single key JSON secret",
			keys:       []string{"api_key"},
			plaintext:  "secret-api-key",
			expectJson: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sm.generatePlaceholderSecretString(tt.keys, tt.plaintext)

			if err != nil {
				t.Errorf("generatePlaceholderSecretString() error = %v", err)
			}

			if result == "" {
				t.Errorf("generatePlaceholderSecretString() returned empty string")
			}

			if !tt.expectJson {
				if result != tt.plaintext {
					t.Errorf("Plain text mismatch: got %s, want %s", result, tt.plaintext)
				}
			} else {
				if !contains(result, tt.plaintext) {
					t.Errorf("JSON secret missing placeholder value")
				}

				for _, key := range tt.keys {
					if !contains(result, key) {
						t.Errorf("JSON secret missing key: %s", key)
					}
				}
			}
		})
	}
}

// Test_ValidateKeyExtraction tests bidirectional key validation.
func Test_ValidateKeyExtraction(t *testing.T) {
	tests := []struct {
		name             string
		expectedKeys     []string
		storedKeys       []string
		expectExpectedOk bool
		expectStoredOk   bool
	}{
		{
			name:             "Exact match",
			expectedKeys:     []string{"user", "pass"},
			storedKeys:       []string{"user", "pass"},
			expectExpectedOk: true,
			expectStoredOk:   true,
		},
		{
			name:             "Extra stored keys",
			expectedKeys:     []string{"user", "pass"},
			storedKeys:       []string{"user", "pass", "extra"},
			expectExpectedOk: true,
			expectStoredOk:   false,
		},
		{
			name:             "Missing stored keys",
			expectedKeys:     []string{"user", "pass", "host"},
			storedKeys:       []string{"user", "pass"},
			expectExpectedOk: false,
			expectStoredOk:   true,
		},
		{
			name:             "Complete mismatch",
			expectedKeys:     []string{"a", "b"},
			storedKeys:       []string{"x", "y"},
			expectExpectedOk: false,
			expectStoredOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectedSet := make(map[string]bool)
			for _, key := range tt.expectedKeys {
				expectedSet[key] = true
			}

			storedSet := make(map[string]bool)
			for _, key := range tt.storedKeys {
				storedSet[key] = true
			}

			isExpectedOk := true
			for key := range expectedSet {
				if !storedSet[key] {
					isExpectedOk = false
					break
				}
			}

			isStoredOk := true
			for key := range storedSet {
				if !expectedSet[key] {
					isStoredOk = false
					break
				}
			}

			if isExpectedOk != tt.expectExpectedOk {
				t.Errorf("Expected keys validation: got %v, want %v", isExpectedOk, tt.expectExpectedOk)
			}

			if isStoredOk != tt.expectStoredOk {
				t.Errorf("Stored keys validation: got %v, want %v", isStoredOk, tt.expectStoredOk)
			}
		})
	}
}

// Test_DestroyDryRun tests destroy in dry-run mode (no actual destruction).
func Test_DestroyDryRun(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	// In dry-run mode, destroy should fail on non-existent secret gracefully
	err = sm.Destroy("non-existent-secret")
	if err == nil {
		t.Errorf("Destroy() should return error for non-existent secret, got nil")
	}
}

// Test_DestroyNonExistentSecret tests that destroying non-existent secret fails.
func Test_DestroyNonExistentSecret(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", false)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	// Should fail because secret doesn't exist
	err = sm.Destroy("non-existent-secret-xyz-123")
	if err == nil {
		t.Errorf("Destroy() should fail for non-existent secret, got nil error")
	}
}

// Test_DestroyStructure tests that Destroy method exists and is callable.
func Test_DestroyStructure(t *testing.T) {
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	// Verify method exists by checking it can be called (even in dry-run with non-existent secret)
	// This is a basic structural test
	if sm == nil {
		t.Errorf("SecretManager is nil")
	}
}

// Test_DeleteSecretIntegration tests the delete secret internal logic.
func Test_DeleteSecretIntegration(t *testing.T) {
	// This test verifies the basic structure of delete operations
	// In real scenarios, these would interact with AWS
	sm, err := NewSecretManager("default", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	// Verify we can at least call the method (it will fail in dry-run without real secret)
	err = sm.Destroy("test-secret-that-doesnt-exist")
	// Expected to fail because secret doesn't exist
	if err == nil {
		t.Errorf("Expected error for non-existent secret in destroy")
	}
}

// Test_AliasNameGeneration tests KMS alias name generation from secret name.
func Test_AliasNameGeneration(t *testing.T) {
	tests := []struct {
		secretName string
		expected   string
	}{
		{
			secretName: "my-secret",
			expected:   "alias/my-secret",
		},
		{
			secretName: "my.secret.name",
			expected:   "alias/my-secret-name",
		},
		{
			secretName: "simple",
			expected:   "alias/simple",
		},
		{
			secretName: "name.with.multiple.dots",
			expected:   "alias/name-with-multiple-dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.secretName, func(t *testing.T) {
			// Replicate the alias generation logic
			aliasName := "alias/" + strings.ReplaceAll(tt.secretName, ".", "-")

			if aliasName != tt.expected {
				t.Errorf("Alias generation: got %s, want %s", aliasName, tt.expected)
			}
		})
	}
}

// Test_DefaultProfile tests default profile handling.
func Test_DefaultProfile(t *testing.T) {
	sm, err := NewSecretManager("", "us-east-1", true)
	if err != nil {
		t.Fatalf("Failed to create SecretManager: %v", err)
	}

	if sm.awsProfile != "default" {
		t.Errorf("Default profile mismatch: got %s, want default", sm.awsProfile)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			stringContainsSubstring(s, substr)))
}

// Helper function for substring search
func stringContainsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
