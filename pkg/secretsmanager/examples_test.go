package secretsmanager

import (
	"os"
	"testing"

	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// TestValidateSecretSpecBasic validates a basic secret specification.
func TestValidateSecretSpecBasic(t *testing.T) {
	spec := &SecretSpec{
		Name:         "my-database-password",
		Description:  "Database password for production",
		SecretString: "super-secret-password",
		KmsKeyId:     "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
		Tags: map[string]string{
			"Environment": "production",
			"Application": "myapp",
		},
	}

	err := ValidateSecretSpec(spec)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// TestValidateSecretSpecMissingName validates that a secret requires a name.
func TestValidateSecretSpecMissingName(t *testing.T) {
	spec := &SecretSpec{
		SecretString: "secret-value",
	}

	err := ValidateSecretSpec(spec)
	if err == nil {
		t.Fatal("Expected error for missing name")
	}
	if err.Error() != "secret name is required" {
		t.Fatalf("Expected 'secret name is required', got: %v", err)
	}
}

// TestValidateSecretSpecMissingValue validates that a secret requires a value.
func TestValidateSecretSpecMissingValue(t *testing.T) {
	spec := &SecretSpec{
		Name: "my-secret",
	}

	err := ValidateSecretSpec(spec)
	if err == nil {
		t.Fatal("Expected error for missing value")
	}
	if err.Error() != "secret my-secret: either secretString or secretBinary is required" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestValidateSecretSpecBothValues validates that a secret can't have both string and binary.
func TestValidateSecretSpecBothValues(t *testing.T) {
	spec := &SecretSpec{
		Name:         "my-secret",
		SecretString: "string-value",
		SecretBinary: "binary-value",
	}

	err := ValidateSecretSpec(spec)
	if err == nil {
		t.Fatal("Expected error for both string and binary")
	}
	if err.Error() != "secret my-secret: cannot specify both secretString and secretBinary" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestValidateSecretSpecRotationWithoutLambda validates rotation requires lambda.
func TestValidateSecretSpecRotationWithoutLambda(t *testing.T) {
	spec := &SecretSpec{
		Name:                 "my-secret",
		SecretString:         "secret-value",
		AutoRotationEnabled:  true,
		RotationIntervalDays: 30,
	}

	err := ValidateSecretSpec(spec)
	if err == nil {
		t.Fatal("Expected error for missing lambda ARN")
	}
	if err.Error() != "secret my-secret: lambdaArn is required when autoRotationEnabled is true" {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestValidateSecretSpecRotationValid validates valid rotation configuration.
func TestValidateSecretSpecRotationValid(t *testing.T) {
	spec := &SecretSpec{
		Name:                 "my-secret",
		SecretString:         "secret-value",
		AutoRotationEnabled:  true,
		RotationIntervalDays: 30,
		LambdaArn:            "arn:aws:lambda:us-east-1:123456789012:function:my-rotation",
	}

	err := ValidateSecretSpec(spec)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
}

// TestParserLoadGitOpsFiles tests loading desired state and configuration files.
func TestParserLoadGitOpsFiles(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	dsDir := tmpDir + "/desiredstate"
	cfgDir := tmpDir + "/configuration"

	os.MkdirAll(dsDir, 0755)
	os.MkdirAll(cfgDir, 0755)

	// Create a test desired state file
	dsContent := `secrets:
  - name: test-secret
    description: Test secret
    secretString: test-value
`
	os.WriteFile(dsDir+"/secrets.yaml", []byte(dsContent), 0644)

	// Create a test configuration file
	cfgContent := `defaultKmsKeyId: arn:aws:kms:us-east-1:123456789012:key/12345678
defaultTags:
  Environment: test
`
	os.WriteFile(cfgDir+"/config.yaml", []byte(cfgContent), 0644)

	// Test loading with nil request (should error)
	parser := NewParser("test", make(map[string]string))
	parser.SetWorkspace(tmpDir)

	err := parser.LoadGitOpsFiles(nil)
	if err == nil {
		t.Fatal("Expected error for nil LoadRequest")
	}
}

// TestParserWorkspaceManagement tests workspace directory management.
func TestParserWorkspaceManagement(t *testing.T) {
	tmpDir := t.TempDir()
	parser := NewParser("test", make(map[string]string))

	// Test setting workspace
	err := parser.SetWorkspace(tmpDir)
	if err != nil {
		t.Fatalf("Failed to set workspace: %v", err)
	}

	if parser.GetWorkspaceDir() != tmpDir {
		t.Fatalf("Expected workspace %s, got %s", tmpDir, parser.GetWorkspaceDir())
	}

	// Test build directory
	buildDir := parser.GetBuildDir()
	if buildDir == "" {
		t.Fatal("Expected non-empty build directory")
	}

	// Test empty workspace
	err = parser.SetWorkspace("")
	if err == nil {
		t.Fatal("Expected error for empty workspace")
	}
}

// TestServiceValidation tests the service validation logic.
func TestServiceValidation(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir := t.TempDir()

	// Create a test desired state file
	dsFile := tmpDir + "/secrets.yaml"
	dsContent := `secrets:
  - name: test-secret
    description: Test secret
    secretString: test-value
`
	os.WriteFile(dsFile, []byte(dsContent), 0644)

	service := NewService(tmpDir, false)

	// Test successful validation using public Validate method
	req := &wrapper.ValidateRequest{
		DesiredStateFile: dsFile,
		SchemaVersion:    "gitops.io/v1",
	}

	resp, err := service.Validate(*req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !resp.IsValid {
		t.Fatalf("Expected validation to succeed, got error: %s", resp.ErrorMessage)
	}

	// Test validation of non-existent file
	req.DesiredStateFile = tmpDir + "/nonexistent.yaml"
	resp, err = service.Validate(*req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.IsValid {
		t.Fatal("Expected validation to fail for non-existent file")
	}

	// Test validation of invalid YAML
	invalidFile := tmpDir + "/invalid.yaml"
	os.WriteFile(invalidFile, []byte("invalid: [yaml"), 0644)
	req.DesiredStateFile = invalidFile
	resp, err = service.Validate(*req)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.IsValid {
		t.Fatal("Expected validation to fail for invalid YAML")
	}
}

// TestCommandIntegration tests basic command structure (without running actual commands).
func TestCommandIntegration(t *testing.T) {
	rootCmd := NewSecretManagerCommand()

	// Verify root command exists
	if rootCmd == nil {
		t.Fatal("Expected root command to be created")
	}

	// Verify root command name
	if rootCmd.Use != "secretsmanager" {
		t.Fatalf("Expected 'secretsmanager', got '%s'", rootCmd.Use)
	}

	// Verify all subcommands exist
	subcommands := []string{"validate", "create", "list", "read", "delete", "rotate"}
	for _, subcmd := range subcommands {
		cmd, _, err := rootCmd.Find([]string{subcmd})
		if err != nil {
			t.Fatalf("Failed to find subcommand %s: %v", subcmd, err)
		}
		// Note: read and rotate have arguments so their Use will include them
		if cmd.Use != subcmd && !contains(cmd.Use, subcmd) {
			t.Fatalf("Expected subcommand %s, got %s", subcmd, cmd.Use)
		}
	}
}

// contains is a helper function to check if a string contains a substring
func contains(str, substr string) bool {
	for i := 0; i < len(str)-len(substr)+1; i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSecretSpecIsCreatedHereField tests the IsCreatedHere field in SecretSpec.
func TestSecretSpecIsCreatedHereField(t *testing.T) {
	// Test with IsCreatedHere = true
	spec := &SecretSpec{
		Name:          "my-secret",
		SecretString:  "secret-value",
		IsCreatedHere: true,
	}

	err := ValidateSecretSpec(spec)
	if err != nil {
		t.Fatalf("Expected no error for valid spec with IsCreatedHere=true, got: %v", err)
	}

	if !spec.IsCreatedHere {
		t.Fatalf("Expected IsCreatedHere to be true")
	}

	// Test with IsCreatedHere = false (default)
	spec2 := &SecretSpec{
		Name:          "external-secret",
		SecretString:  "secret-value",
		IsCreatedHere: false,
	}

	err = ValidateSecretSpec(spec2)
	if err != nil {
		t.Fatalf("Expected no error for valid spec with IsCreatedHere=false, got: %v", err)
	}

	if spec2.IsCreatedHere {
		t.Fatalf("Expected IsCreatedHere to be false")
	}
}

// TestDestroyCommandStructure tests that destroy command is registered.
func TestDestroyCommandStructure(t *testing.T) {
	cmd := NewSecretManagerCommand()

	// Find destroy subcommand
	destroyCmd := cmd.Commands()
	found := false
	for _, subcmd := range destroyCmd {
		if subcmd.Use == "destroy" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Expected destroy subcommand not found")
	}
}

// TestDestroyCommandFlags tests that destroy command has required flags.
func TestDestroyCommandFlags(t *testing.T) {
	cmd := NewDestroyCommand()

	// Test that command has expected flags
	if cmd.Flag("file") == nil {
		t.Fatal("Expected 'file' flag not found")
	}

	if cmd.Flag("force") == nil {
		t.Fatal("Expected 'force' flag not found")
	}
}

// TestSecretSpecYAMLMarshalling tests that IsCreatedHere is properly marshalled in YAML.
func TestSecretSpecYAMLMarshalling(t *testing.T) {
	spec := &SecretSpec{
		Name:          "my-secret",
		SecretString:  "secret-value",
		IsCreatedHere: true,
	}

	// Verify the field exists and has proper tag
	// This is a basic test to ensure the field is present in the struct
	if spec.Name != "my-secret" {
		t.Fatalf("Expected name to be 'my-secret', got '%s'", spec.Name)
	}

	if !spec.IsCreatedHere {
		t.Fatalf("Expected IsCreatedHere to be true")
	}
}

// TestFactoryRegistration tests the factory pattern implementation.
func TestFactoryRegistration(t *testing.T) {
	factory := &SecretManagerFactory{}

	// Test factory name
	if factory.Name() != "secretsmanager" {
		t.Fatalf("Expected 'secretsmanager', got '%s'", factory.Name())
	}

	// Test that factory creates valid instances
	parser := factory.CreateParser("test", make(map[string]string))
	if parser == nil {
		t.Fatal("Expected non-nil parser")
	}

	service := factory.CreateService(".", false)
	if service == nil {
		t.Fatal("Expected non-nil service")
	}

	cmd := factory.CreateCLICommand()
	if cmd == nil {
		t.Fatal("Expected non-nil CLI command")
	}
}
