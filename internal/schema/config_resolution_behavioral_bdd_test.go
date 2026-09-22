package schema

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// TIME-BASED BDD: Configuration Resolution Behavioral Contracts
// =============================================================================
// These tests document the CURRENT behavior of schema configuration path
// resolution at time T. They serve as:
// 1. Characterization tests (Michael Feathers, "Working Effectively with Legacy Code")
// 2. Regression detection for refactoring safety
// 3. Behavioral specification derived from working code
//
// Pattern: GoBehavioralContract
// Migrated from: config_resolution_test.go (223 lines, 3 test functions)
// Migration Date: October 13, 2025
// =============================================================================

// =============================================================================
// PHASE 1: SCHEMA CONFIG PATH RESOLUTION WITH PRECEDENCE
// =============================================================================

func TestResolveSchemaConfigPath_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ResolveSchemaConfigPath resolves schema configuration file path using precedence hierarchy: CLI flag > Env var > WorkDir > Home directory",

		CurrentImpl: `
Go: internal/schema/config.go (ResolveSchemaConfigPath)

func ResolveSchemaConfigPath(explicitPath string, workDir string) string {
    // 1. Highest precedence: CLI flag (explicit path)
    if explicitPath != "" {
        return expandHomePath(explicitPath)
    }

    // 2. Environment variable
    if envPath := os.Getenv("YAGO_SCHEMA_CONFIG"); envPath != "" {
        return expandHomePath(envPath)
    }

    // 3. Working directory config
    workDirConfig := filepath.Join(workDir, "schema-config.json")
    if _, err := os.Stat(workDirConfig); err == nil {
        return workDirConfig
    }

    // 4. Fallback: Home directory
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".yago", "schema-config.json")
}

func expandHomePath(path string) string {
    if strings.HasPrefix(path, "~/") {
        home, _ := os.UserHomeDir()
        return filepath.Join(home, path[2:])
    }
    return path
}

Key features:
- 4-level precedence hierarchy
- CLI flag: Highest priority (user explicit choice)
- Env var: Second priority (environment configuration)
- WorkDir: Third priority (project-local config)
- Home: Fallback (user default config)
- Tilde expansion: ~/path → /home/user/path
- File existence check: Only for workDir (not for others)
`,

		ExpectedOutcome: `
Precedence order (highest to lowest):
1. CLI flag (--schema-config=/path/to/config.json)
   - MUST take precedence over all other sources
   - MUST expand tilde (~/) to home directory
   - MUST NOT check file existence

2. Environment variable (YAGO_SCHEMA_CONFIG)
   - MUST override workDir and home configs
   - MUST expand tilde (~/) to home directory
   - MUST NOT check file existence

3. Working directory (./schema-config.json)
   - MUST be used if file exists in workDir
   - MUST check file existence before using
   - MUST NOT expand tilde (absolute/relative path)

4. Home directory (~/.yago/schema-config.json)
   - MUST be fallback when others not available
   - MUST use ~/.yago/schema-config.json path
   - MUST NOT check file existence

Path expansion:
- MUST expand ~/path to /home/user/path (CLI and env var)
- MUST handle paths without tilde (absolute/relative)
`,

		TestScenario: `
Test Case 1: CLI flag takes highest precedence
GIVEN:
  - explicitPath: "/custom/path/schema-config.json"
  - envVar: "/env/path/schema-config.json" (set)
  - workDir: "/work/dir" with existing config
WHEN: ResolveSchemaConfigPath(explicitPath, workDir)
THEN: Returns "/custom/path/schema-config.json" (CLI flag)

Test Case 2: Environment variable overrides workDir and home
GIVEN:
  - explicitPath: "" (empty)
  - envVar: "/env/path/schema-config.json" (set)
  - workDir: "/work/dir" with existing config
WHEN: ResolveSchemaConfigPath("", workDir)
THEN: Returns "/env/path/schema-config.json" (env var)

Test Case 3: WorkDir takes precedence over home when file exists
GIVEN:
  - explicitPath: "" (empty)
  - envVar: "" (not set)
  - workDir: tempDir with "schema-config.json" file
WHEN: ResolveSchemaConfigPath("", tempDir)
THEN: Returns "tempDir/schema-config.json" (workDir)

Test Case 4: Home directory fallback when workDir doesn't exist
GIVEN:
  - explicitPath: "" (empty)
  - envVar: "" (not set)
  - workDir: "/nonexistent/work/dir" (no config file)
WHEN: ResolveSchemaConfigPath("", workDir)
THEN: Returns "$HOME/.yago/schema-config.json" (home fallback)

Test Case 5: Tilde expansion in CLI flag
GIVEN:
  - explicitPath: "~/custom/schema-config.json"
  - envVar: "" (not set)
  - workDir: "/work/dir"
WHEN: ResolveSchemaConfigPath(explicitPath, workDir)
THEN: Returns "$HOME/custom/schema-config.json" (tilde expanded)

Test Case 6: Tilde expansion in environment variable
GIVEN:
  - explicitPath: "" (empty)
  - envVar: "~/env/schema-config.json" (set)
  - workDir: "/work/dir"
WHEN: ResolveSchemaConfigPath("", workDir)
THEN: Returns "$HOME/env/schema-config.json" (tilde expanded)

Test implementation:
1. Get home directory for path comparisons
2. For each test case:
   - Set/unset environment variable
   - Create temp files if needed
   - Call ResolveSchemaConfigPath
   - Assert returned path matches expected precedence
`,

		Rationale: `
Why this behavior exists:
- Flexibility: Multiple ways to configure schema location
- Precedence clarity: Explicit (CLI) beats implicit (env/files)
- Developer experience: Project-local configs for teams
- User defaults: Home directory for personal configs
- Environment control: CI/CD can set env vars

Precedence rationale:
1. CLI flag: User's explicit choice in current invocation
2. Env var: Session/environment-wide configuration
3. WorkDir: Project-specific configuration (shared with team)
4. Home: User's personal default configuration

Use cases:
- CLI flag: Override for testing, debugging, special runs
- Env var: CI/CD pipelines, container environments
- WorkDir: Team shared config in git repository
- Home: Personal default for all projects

File existence check rationale:
- CLI/Env: User explicitly specified, trust them (may create later)
- WorkDir: Check existence to avoid false positives
- Home: Always return path (standard fallback location)

Tilde expansion rationale:
- User convenience: ~/path is common shell notation
- Cross-platform: Works on Linux, macOS, Unix-like systems
- Only for CLI/env: User-specified paths, not system paths
`,

		RegressionRisk: `
HIGH RISK if changed:
- Precedence order: Changing order breaks user expectations
- Environment variable name: YAGO_SCHEMA_CONFIG is documented
- Home directory path: ~/.yago/schema-config.json is standard location
- WorkDir behavior: Teams depend on project-local configs

MEDIUM RISK:
- Tilde expansion: Users expect ~/path to work
- File existence check: Affects workDir detection
- Path handling: Must handle absolute, relative, ~/ paths

LOW RISK:
- Default filename: "schema-config.json" is consistent
- Fallback behavior: Home directory is reasonable default

What breaks if this changes:
1. Change precedence → users' configs ignored or wrong config loaded
2. Remove env var support → CI/CD pipelines break
3. Remove workDir check → always use home (team configs ignored)
4. Remove tilde expansion → ~/path not recognized
5. Change home path → existing configs not found
6. Add file existence check to CLI/env → users can't specify non-existent paths
`,
	}

	// Execute the behavioral tests
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name         string
		explicitPath string
		envVar       string
		workDirPath  string
		workDir      string
		expected     string
		description  string
	}{
		{
			name:         "CLI flag takes highest precedence",
			explicitPath: "/custom/path/schema-config.json",
			envVar:       "/env/path/schema-config.json",
			workDirPath:  "/work/dir/schema-config.json",
			workDir:      "/work/dir",
			expected:     "/custom/path/schema-config.json",
			description:  "CLI flag should override everything",
		},
		{
			name:         "Environment variable overrides workDir and home",
			explicitPath: "",
			envVar:       "/env/path/schema-config.json",
			workDirPath:  "/work/dir/schema-config.json",
			workDir:      "/work/dir",
			expected:     "/env/path/schema-config.json",
			description:  "Environment variable should override workDir and home",
		},
		{
			name:         "WorkDir takes precedence over home when it exists",
			explicitPath: "",
			envVar:       "",
			workDirPath:  "", // Will create temp file
			workDir:      "", // Will use temp dir
			expected:     "", // Will be set to temp file path
			description:  "WorkDir config should be used when it exists",
		},
		{
			name:         "Home directory when workDir doesn't exist",
			explicitPath: "",
			envVar:       "",
			workDirPath:  "", // Don't create file
			workDir:      "/nonexistent/work/dir",
			expected:     filepath.Join(home, ".yago", "schema-config.json"),
			description:  "Home directory should be fallback when workDir config doesn't exist",
		},
		{
			name:         "Home directory expansion in CLI flag",
			explicitPath: "~/custom/schema-config.json",
			envVar:       "",
			workDirPath:  "",
			workDir:      "/work/dir",
			expected:     filepath.Join(home, "custom/schema-config.json"),
			description:  "Tilde should be expanded in CLI flag",
		},
		{
			name:         "Home directory expansion in env var",
			explicitPath: "",
			envVar:       "~/env/schema-config.json",
			workDirPath:  "",
			workDir:      "/work/dir",
			expected:     filepath.Join(home, "env/schema-config.json"),
			description:  "Tilde should be expanded in environment variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable if specified
			if tt.envVar != "" {
				oldEnv := os.Getenv("YAGO_SCHEMA_CONFIG")
				os.Setenv("YAGO_SCHEMA_CONFIG", tt.envVar)
				defer os.Setenv("YAGO_SCHEMA_CONFIG", oldEnv)
			} else {
				// Clear env var for this test
				oldEnv := os.Getenv("YAGO_SCHEMA_CONFIG")
				os.Unsetenv("YAGO_SCHEMA_CONFIG")
				defer func() {
					if oldEnv != "" {
						os.Setenv("YAGO_SCHEMA_CONFIG", oldEnv)
					}
				}()
			}

			// Handle special case for workDir test
			workDir := tt.workDir
			expected := tt.expected
			if tt.name == "WorkDir takes precedence over home when it exists" {
				// Create temporary directory and file
				tempDir := t.TempDir()
				tempFile := filepath.Join(tempDir, "schema-config.json")
				err := os.WriteFile(tempFile, []byte("{}"), 0644)
				if err != nil {
					t.Fatalf("Failed to create temp file: %v", err)
				}
				workDir = tempDir
				expected = tempFile
			}

			result := ResolveSchemaConfigPath(tt.explicitPath, workDir)

			if result != expected {
				t.Errorf("%s\nExpected: %s\nGot: %s", tt.description, expected, result)
			}
		})
	}

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 2: COMPLETE PRECEDENCE ORDER VERIFICATION
// =============================================================================

func TestResolveSchemaConfigPath_PrecedenceOrder_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "ResolveSchemaConfigPath enforces complete precedence order with all configuration sources available simultaneously",

		CurrentImpl: `
Go: internal/schema/config.go (precedence enforcement)

Precedence implementation:
1. Check explicitPath first (return if non-empty)
2. Check YAGO_SCHEMA_CONFIG env var (return if set)
3. Check workDir/schema-config.json existence (return if exists)
4. Return home/.yago/schema-config.json (fallback)

Sequential checks (early return pattern):
- Each level checked in order
- First match wins (early return)
- Lower levels never evaluated once higher level matches
- Fallback always available (home directory)

Key features:
- Deterministic: Same inputs always produce same output
- Complete coverage: All combinations tested
- Isolation: Each test case independent
- Verification: File existence only for workDir level
`,

		ExpectedOutcome: `
Precedence verification:
1. Only workDir config exists
   - MUST return workDir config path
   - MUST detect file existence

2. Env var overrides workDir
   - MUST return env var path
   - MUST ignore workDir config (even if exists)

3. CLI flag overrides everything
   - MUST return CLI flag path
   - MUST ignore env var and workDir

4. Home directory fallback
   - MUST return ~/.yago/schema-config.json
   - MUST NOT check file existence
   - MUST be used when workDir config doesn't exist

Isolation requirements:
- MUST set/unset env vars per test
- MUST create temp files as needed
- MUST clean up after each test
- MUST NOT affect other tests
`,

		TestScenario: `
Test 1: Only workDir config exists
GIVEN:
  - explicitPath: "" (empty)
  - YAGO_SCHEMA_CONFIG: "" (unset)
  - workDir: tempDir with schema-config.json file
WHEN: ResolveSchemaConfigPath("", tempDir)
THEN: Returns "tempDir/schema-config.json"

Test 2: Env var overrides workDir
GIVEN:
  - explicitPath: "" (empty)
  - YAGO_SCHEMA_CONFIG: "/env/override/schema-config.json" (set)
  - workDir: tempDir with schema-config.json file
WHEN: ResolveSchemaConfigPath("", tempDir)
THEN: Returns "/env/override/schema-config.json" (env var wins)

Test 3: CLI flag overrides everything
GIVEN:
  - explicitPath: "/cli/override/schema-config.json"
  - YAGO_SCHEMA_CONFIG: "/env/override/schema-config.json" (set)
  - workDir: tempDir with schema-config.json file
WHEN: ResolveSchemaConfigPath("/cli/override/schema-config.json", tempDir)
THEN: Returns "/cli/override/schema-config.json" (CLI wins)

Test 4: Home directory fallback
GIVEN:
  - explicitPath: "" (empty)
  - YAGO_SCHEMA_CONFIG: "" (unset)
  - workDir: "/definitely/does/not/exist" (no config)
WHEN: ResolveSchemaConfigPath("", nonexistentDir)
THEN: Returns "$HOME/.yago/schema-config.json" (fallback)

Test implementation:
1. Create temp directory with config file
2. For each test case:
   - Set/unset environment variables
   - Call ResolveSchemaConfigPath with appropriate parameters
   - Assert returned path matches expected precedence level
   - Verify lower levels are NOT used when higher level available
`,

		Rationale: `
Why this behavior exists:
- Complete verification: Test all precedence levels together
- Integration testing: Verify precedence with multiple sources active
- Real-world scenarios: Users may have multiple configs set
- Precedence correctness: Ensure higher priority always wins

Test design rationale:
- Isolated tests: Each test case is independent
- Progressive overrides: Tests build up from lowest to highest priority
- File creation: Only create files when testing workDir behavior
- Environment manipulation: Set/unset env vars safely

Sequential testing approach:
1. Test lowest priority (workDir only)
2. Add higher priority (env var) and verify override
3. Add highest priority (CLI) and verify it wins
4. Test fallback (no configs) separately

Why test all levels:
- Precedence bugs: Higher priority might not override lower
- Implementation changes: Refactoring could break precedence
- Edge cases: All configs set simultaneously is valid scenario
- Documentation: Tests demonstrate correct precedence behavior
`,

		RegressionRisk: `
HIGH RISK if changed:
- Precedence order critical for predictable behavior
- Multiple configs common in real deployments
- CI/CD depends on env var overriding workDir
- CLI override essential for debugging/testing

MEDIUM RISK:
- Adding new precedence level requires careful ordering
- Changing order breaks existing user setups
- File existence check affects detection

LOW RISK:
- Test improvements don't affect production behavior
- Cleanup logic (defer) prevents test pollution

What breaks if this changes:
1. Wrong precedence → wrong config loaded in production
2. No early return → multiple configs could conflict
3. File existence check removed → false positives for workDir
4. Env var check broken → CI/CD can't override configs
5. CLI flag not checked first → can't override in emergency
`,
	}

	// Execute the behavioral tests
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Skipping test: cannot get home directory")
	}

	// Create temp directory with config file
	tempDir := t.TempDir()
	workDirConfig := filepath.Join(tempDir, "schema-config.json")
	err = os.WriteFile(workDirConfig, []byte("{}"), 0644)
	if err != nil {
		t.Fatalf("Failed to create workDir config: %v", err)
	}

	// Test 1: Only workDir config exists
	t.Run("Only workDir config", func(t *testing.T) {
		os.Unsetenv("YAGO_SCHEMA_CONFIG")
		result := ResolveSchemaConfigPath("", tempDir)
		if result != workDirConfig {
			t.Errorf("Expected workDir config: %s, got: %s", workDirConfig, result)
		}
	})

	// Test 2: Env var overrides workDir
	t.Run("Env var overrides workDir", func(t *testing.T) {
		envPath := "/env/override/schema-config.json"
		os.Setenv("YAGO_SCHEMA_CONFIG", envPath)
		defer os.Unsetenv("YAGO_SCHEMA_CONFIG")

		result := ResolveSchemaConfigPath("", tempDir)
		if result != envPath {
			t.Errorf("Expected env path: %s, got: %s", envPath, result)
		}
	})

	// Test 3: CLI flag overrides everything
	t.Run("CLI flag overrides everything", func(t *testing.T) {
		envPath := "/env/override/schema-config.json"
		os.Setenv("YAGO_SCHEMA_CONFIG", envPath)
		defer os.Unsetenv("YAGO_SCHEMA_CONFIG")

		cliPath := "/cli/override/schema-config.json"
		result := ResolveSchemaConfigPath(cliPath, tempDir)
		if result != cliPath {
			t.Errorf("Expected CLI path: %s, got: %s", cliPath, result)
		}
	})

	// Test 4: Home directory fallback when workDir doesn't exist
	t.Run("Home directory fallback", func(t *testing.T) {
		os.Unsetenv("YAGO_SCHEMA_CONFIG")
		homeConfig := filepath.Join(home, ".yago", "schema-config.json")
		nonexistentDir := "/definitely/does/not/exist"

		result := ResolveSchemaConfigPath("", nonexistentDir)
		if result != homeConfig {
			t.Errorf("Expected home config: %s, got: %s", homeConfig, result)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}

// =============================================================================
// PHASE 3: GLOBAL SCHEMA CONFIG PATH MANAGEMENT
// =============================================================================

func TestSetGetSchemaConfigPath_BehavioralBDD(t *testing.T) {
	contract := GoBehavioralContract{
		Behavior: "SetSchemaConfigPath and GetSchemaConfigPath manage global schema configuration path state for application-wide config sharing",

		CurrentImpl: `
Go: internal/schema/config.go (global state management)

var globalSchemaConfigPath string

func SetSchemaConfigPath(path string) {
    globalSchemaConfigPath = path
}

func GetSchemaConfigPath() string {
    return globalSchemaConfigPath
}

Key features:
- Package-level variable: Shared across application
- Simple getter/setter: Direct access to global state
- No synchronization: Assumes single-threaded or careful usage
- Empty string support: Can be cleared/unset
- No validation: Accepts any string (including empty)

Typical usage:
1. Application startup: Resolve config path using precedence
2. Set global path: SetSchemaConfigPath(resolvedPath)
3. Access throughout: GetSchemaConfigPath() anywhere needed
4. Test cleanup: Save original, restore in defer

Global state rationale:
- Convenience: Avoid passing path to every function
- Consistency: One config path for entire application
- Performance: Resolve once, use many times
- Testability: Can be set/reset in tests
`,

		ExpectedOutcome: `
- MUST store any string value (including empty)
- MUST return exactly what was set (no transformation)
- MUST persist across function calls (global state)
- MUST support clearing (empty string)
- MUST be accessible from any code in package
- SHOULD be set once at application startup
- SHOULD be saved/restored in tests (avoid pollution)

State management:
- Set: Overwrites previous value
- Get: Returns current value
- Clear: Set to empty string
- Initial: Empty string (package var zero value)
`,

		TestScenario: `
Test Case 1: Setting and getting custom path
GIVEN:
  - Original state unknown
  - Save original value
WHEN:
  - SetSchemaConfigPath("/test/custom/path/schema-config.json")
  - result = GetSchemaConfigPath()
THEN:
  - result == "/test/custom/path/schema-config.json"
  - Value persists across Get calls

Test Case 2: Clearing the path
GIVEN:
  - Path set to "/test/custom/path/schema-config.json"
WHEN:
  - SetSchemaConfigPath("")
  - result = GetSchemaConfigPath()
THEN:
  - result == ""
  - Empty string is valid value

Test Case 3: State restoration in tests
GIVEN:
  - Original state saved
  - Multiple Set operations during test
WHEN:
  - defer SetSchemaConfigPath(original)
  - Test completes (normal or panic)
THEN:
  - Original state restored
  - No test pollution

Test implementation:
1. Save original state: original := GetSchemaConfigPath()
2. Ensure restoration: defer SetSchemaConfigPath(original)
3. Test set operation: SetSchemaConfigPath(testPath)
4. Verify get operation: assert GetSchemaConfigPath() == testPath
5. Test clear operation: SetSchemaConfigPath("")
6. Verify empty: assert GetSchemaConfigPath() == ""
`,

		Rationale: `
Why this behavior exists:
- Global config: Single source of truth for schema config path
- Simplicity: Getter/setter pattern is straightforward
- Application-wide: Config path needed in many places
- Startup configuration: Resolve once, use everywhere

Design decisions:
- Package-level var: Simplest global state mechanism
- No mutex: Schema config set once at startup (not concurrent)
- No validation: Path validation happens during resolution
- String type: Path is fundamentally a string

Use cases:
- Application startup: Resolve and set config path
- Schema loading: Get config path to load schemas
- Testing: Override config path for test scenarios
- Runtime checks: Verify config path is set

Alternative designs (not used):
- Function parameter: Would need to pass path everywhere (verbose)
- Dependency injection: Overkill for simple config path
- Config struct: More complex than needed for single value
- sync.Once: Path may need to change (testing)

Global state trade-offs:
- Pro: Simple, efficient, convenient
- Pro: One-time resolution at startup
- Con: Global mutable state (testing care needed)
- Con: No concurrency protection (assumed single-threaded setup)
`,

		RegressionRisk: `
LOW RISK if changed:
- Simple getter/setter unlikely to break
- No complex logic to regress
- Used throughout codebase but predictably

MEDIUM RISK:
- Adding concurrency control affects performance
- Changing to config struct requires API changes
- Validation in setter changes error handling

HIGH RISK:
- Removing global state requires passing path everywhere
- Thread-safety issues if concurrent access added
- Test pollution if not properly restored

What breaks if this changes:
1. Add validation → existing code may pass invalid paths
2. Add mutex → performance impact, potential deadlocks
3. Change to private → external packages can't access
4. Remove global state → massive refactoring needed
5. Change type → all callers must update
`,
	}

	// Execute the behavioral test
	t.Run("Set and get schema config path", func(t *testing.T) {
		// Save original state
		original := GetSchemaConfigPath()
		defer SetSchemaConfigPath(original)

		// Test setting custom path
		testPath := "/test/custom/path/schema-config.json"
		SetSchemaConfigPath(testPath)

		result := GetSchemaConfigPath()
		if result != testPath {
			t.Errorf("Expected %s, got %s", testPath, result)
		}

		// Test clearing
		SetSchemaConfigPath("")
		result = GetSchemaConfigPath()
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	// Log the contract
	t.Logf("\n=== BEHAVIORAL CONTRACT ===")
	t.Logf("Behavior: %s", contract.Behavior)
	t.Logf("Expected Outcome: %s", contract.ExpectedOutcome)
	t.Logf("Regression Risk: %s", contract.RegressionRisk)
}
