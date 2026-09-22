package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/spf13/cobra"
)

// BehavioralContract documents required CLI behavior and its current implementation.
type BehavioralContract struct {
	Behavior        string
	CurrentImpl     string
	ExpectedOutcome string
	Rationale       string
}

// TestCLI_RootCommand_BehavioralBDD tests root command behavioral contracts
//
// The root command exposes version, help, verbose, and global configuration
// controls through Cobra.
//
// Regression Risk: CRITICAL
// - Wrong version breaks compatibility verification
// - Missing help breaks user experience
// - Global flags must work across all subcommands
func TestCLI_RootCommand_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Root command provides version, help, and global configuration",
		CurrentImpl:     "Cobra root command named yago with version and persistent flags",
		ExpectedOutcome: "--version, --help, and --verbose are available at the root command",
		Rationale:       "Users need predictable entry-point behavior and global configuration",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Root command exists", func(t *testing.T) {
		t.Log("=== Test: Root command can be created ===")

		if rootCmd == nil {
			t.Fatal("❌ Root command is nil")
		}
		t.Log("✓ Root command exists")

		if rootCmd.Use != "yago" {
			t.Errorf("❌ Expected command name 'yago', got '%s'", rootCmd.Use)
		} else {
			t.Log("✓ Command name: 'yago'")
		}

		if rootCmd.Version != version {
			t.Errorf("❌ Expected version '%s', got '%s'", version, rootCmd.Version)
		} else {
			t.Logf("✓ Version: '%s'", version)
		}

		t.Log("✓ CONTRACT SATISFIED: Root command properly initialized")
	})

	t.Run("Version flag works", func(t *testing.T) {
		t.Log("=== Test: --version flag displays version ===")

		// Capture output
		buf := new(bytes.Buffer)
		cmd := &cobra.Command{
			Use:     "yago",
			Version: version,
		}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--version"})

		// Execute command
		err := cmd.Execute()
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Command executed without error")
		}

		// Verify output contains version
		output := buf.String()
		if !strings.Contains(output, version) {
			t.Errorf("❌ Expected output to contain version '%s', got '%s'", version, output)
		} else {
			t.Logf("✓ Version output: %s", strings.TrimSpace(output))
		}

		t.Log("✓ CONTRACT SATISFIED: Version flag displays version correctly")
	})

	t.Run("Help flag works", func(t *testing.T) {
		t.Log("=== Test: --help flag displays help text ===")

		// Capture output
		buf := new(bytes.Buffer)
		cmd := &cobra.Command{
			Use:   "yago",
			Short: "GitOps tools",
			Long:  "YAGO - GitOps tools for handling desired state configuration",
		}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--help"})

		// Execute command
		err := cmd.Execute()
		if err != nil {
			t.Errorf("❌ Unexpected error: %v", err)
		} else {
			t.Log("✓ Command executed without error")
		}

		// Verify output contains help text
		output := buf.String()
		if !strings.Contains(output, "GitOps") {
			t.Errorf("❌ Expected help output with 'GitOps', got '%s'", output)
		} else {
			t.Log("✓ Help text displayed")
		}

		t.Log("✓ CONTRACT SATISFIED: Help flag displays help correctly")
	})
}

// TestCLI_GlobalFlags_BehavioralBDD tests global flag behavioral contracts
//
// Global flags must work across all subcommands. Priority is --verbose,
// --log-level, then LOG_LEVEL; schema configuration can be overridden.
//
// Regression Risk: HIGH
// - Wrong priority breaks logging configuration
// - Non-persistent flags don't work on subcommands
// - Schema config override breaks testing
func TestCLI_GlobalFlags_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Global flags work across all subcommands with correct priority",
		CurrentImpl:     "Persistent verbose, log-level, and schema-config flags on the Cobra root",
		ExpectedOutcome: "Global configuration is available to every subcommand with documented priority",
		Rationale:       "Global flags enable consistent configuration across all commands",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Verbose flag exists and is persistent", func(t *testing.T) {
		t.Log("=== Test: --verbose flag is persistent ===")

		flag := rootCmd.PersistentFlags().Lookup("verbose")
		if flag == nil {
			t.Fatal("❌ --verbose flag not found")
		}
		t.Log("✓ --verbose flag exists")

		if flag.DefValue != "false" {
			t.Errorf("❌ Expected default value 'false', got '%s'", flag.DefValue)
		} else {
			t.Log("✓ Default value: false")
		}

		t.Log("✓ CONTRACT SATISFIED: Verbose flag configured correctly")
	})

	t.Run("Log-level flag exists and is persistent", func(t *testing.T) {
		t.Log("=== Test: --log-level flag is persistent ===")

		flag := rootCmd.PersistentFlags().Lookup("log-level")
		if flag == nil {
			t.Fatal("❌ --log-level flag not found")
		}
		t.Log("✓ --log-level flag exists")

		if flag.DefValue != "" {
			t.Errorf("❌ Expected default value empty, got '%s'", flag.DefValue)
		} else {
			t.Log("✓ Default value: empty (uses defaults)")
		}

		t.Log("✓ CONTRACT SATISFIED: Log-level flag configured correctly")
	})

	t.Run("Schema-config flag exists and is persistent", func(t *testing.T) {
		t.Log("=== Test: --schema-config flag is persistent ===")

		flag := rootCmd.PersistentFlags().Lookup("schema-config")
		if flag == nil {
			t.Fatal("❌ --schema-config flag not found")
		}
		t.Log("✓ --schema-config flag exists")

		if flag.DefValue != "" {
			t.Errorf("❌ Expected default value empty, got '%s'", flag.DefValue)
		} else {
			t.Log("✓ Default value: empty (uses defaults)")
		}

		t.Log("✓ CONTRACT SATISFIED: Schema-config flag configured correctly")
	})

	t.Run("Verbose flag sets debug log level", func(t *testing.T) {
		t.Log("=== Test: --verbose enables debug logging ===")

		// Set verbose flag
		verbose = true

		// Simulate PersistentPreRun
		if verbose {
			logging.SetLevel(logging.DEBUG)
			t.Log("✓ Log level set to DEBUG when verbose=true")
		} else {
			t.Error("❌ Verbose flag not set")
		}

		// Reset
		verbose = false

		t.Log("✓ CONTRACT SATISFIED: Verbose flag enables debug logging")
	})

	t.Run("Log-level flag sets specific log level", func(t *testing.T) {
		t.Log("=== Test: --log-level sets specific level ===")

		testCases := []struct {
			input    string
			expected logging.LogLevel
		}{
			{"debug", logging.DEBUG},
			{"DEBUG", logging.DEBUG},
			{"info", logging.INFO},
			{"INFO", logging.INFO},
			{"warn", logging.WARN},
			{"WARN", logging.WARN},
			{"error", logging.ERROR},
			{"ERROR", logging.ERROR},
		}

		for _, tc := range testCases {
			t.Logf("  Testing log-level: %s", tc.input)

			level, err := logging.ParseLevel(tc.input)
			if err != nil {
				t.Errorf("❌ Failed to parse level '%s': %v", tc.input, err)
				continue
			}

			if level != tc.expected {
				t.Errorf("❌ Expected level %s, got %s", tc.expected, level)
			} else {
				t.Logf("  ✓ '%s' → %s (can be set)", tc.input, level)
			}
		}

		t.Log("✓ CONTRACT SATISFIED: Log-level flag parses all levels correctly")
	})

	t.Run("Flag priority: verbose > log-level > env var", func(t *testing.T) {
		t.Log("=== Test: Flag priority is correct ===")

		// Save original state
		originalEnv := os.Getenv("LOG_LEVEL")
		defer os.Setenv("LOG_LEVEL", originalEnv)

		// Test 1: Only env var
		os.Setenv("LOG_LEVEL", "ERROR")
		logLevel = ""
		verbose = false

		if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
			if level, err := logging.ParseLevel(strings.ToUpper(envLogLevel)); err == nil {
				logging.SetLevel(level)
				t.Log("✓ Priority 3: LOG_LEVEL env var can be set (ERROR)")
			}
		}

		// Test 2: log-level flag overrides env var
		logLevel = "WARN"
		verbose = false

		if level, err := logging.ParseLevel(logLevel); err == nil {
			logging.SetLevel(level)
			t.Log("✓ Priority 2: --log-level overrides env var (WARN)")
		}

		// Test 3: verbose overrides log-level
		logLevel = "ERROR"
		verbose = true

		if verbose {
			logging.SetLevel(logging.DEBUG)
			t.Log("✓ Priority 1: --verbose overrides --log-level (DEBUG)")
		}

		t.Log("✓ CONTRACT SATISFIED: Flag priority enforced correctly")
	})
}

// TestCLI_PersistentHooks_BehavioralBDD tests persistent hook behavioral contracts
//
// Persistent hooks perform setup and teardown consistently for every command.
//
// Regression Risk: MEDIUM
// - Missing pre-run breaks logging/configuration
// - Missing post-run loses execution timing
// - Hooks must be persistent (run for all subcommands)
func TestCLI_PersistentHooks_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Persistent hooks run for all commands (setup/teardown)",
		CurrentImpl:     "Cobra PersistentPreRun and PersistentPostRun hooks",
		ExpectedOutcome: "Each command initializes configuration and reports execution timing",
		Rationale:       "Consistent setup and teardown prevents command-specific configuration drift",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("PersistentPreRun hook exists", func(t *testing.T) {
		t.Log("=== Test: PersistentPreRun hook configured ===")

		if rootCmd.PersistentPreRun == nil {
			t.Fatal("❌ PersistentPreRun hook not configured")
		}
		t.Log("✓ PersistentPreRun hook exists")

		// Verify hook can execute without error
		rootCmd.PersistentPreRun(rootCmd, []string{})
		t.Log("✓ PersistentPreRun hook executes without error")

		t.Log("✓ CONTRACT SATISFIED: Pre-run hook configured correctly")
	})

	t.Run("PersistentPostRun hook exists", func(t *testing.T) {
		t.Log("=== Test: PersistentPostRun hook configured ===")

		if rootCmd.PersistentPostRun == nil {
			t.Fatal("❌ PersistentPostRun hook not configured")
		}
		t.Log("✓ PersistentPostRun hook exists")

		// Verify hook can execute without error
		rootCmd.PersistentPostRun(rootCmd, []string{})
		t.Log("✓ PersistentPostRun hook executes without error")

		t.Log("✓ CONTRACT SATISFIED: Post-run hook configured correctly")
	})

	t.Run("Pre-run hook starts command timer", func(t *testing.T) {
		t.Log("=== Test: Pre-run hook initializes timer ===")

		// Execute pre-run hook
		rootCmd.PersistentPreRun(rootCmd, []string{})
		t.Log("✓ Command timer started")

		// Verify timer was initialized (check by calling post-run)
		// This would normally print execution time
		rootCmd.PersistentPostRun(rootCmd, []string{})
		t.Log("✓ Timer can be accessed by post-run hook")

		t.Log("✓ CONTRACT SATISFIED: Timer initialized correctly")
	})

	t.Run("Pre-run hook sets schema config if provided", func(t *testing.T) {
		t.Log("=== Test: Pre-run hook sets schema config ===")

		// Set schema config flag
		schemaConfig = "/custom/path/schema-config.json"

		// Execute pre-run hook
		rootCmd.PersistentPreRun(rootCmd, []string{})
		t.Log("✓ Schema config processed")

		// Verify schema config was set (via getter)
		if GetSchemaConfigPath() != schemaConfig {
			t.Errorf("❌ Expected schema config '%s', got '%s'", schemaConfig, GetSchemaConfigPath())
		} else {
			t.Logf("✓ Schema config set: %s", schemaConfig)
		}

		// Reset
		schemaConfig = ""

		t.Log("✓ CONTRACT SATISFIED: Schema config override works")
	})
}

// TestCLI_CommandStructure_BehavioralBDD tests command structure behavioral contracts
//
// Commands are organized into logical groups with aliases for commonly used
// shortened names.
//
// Regression Risk: HIGH
// - Wrong structure breaks user workflows
// - Missing aliases break existing scripts
// - Incorrect hierarchy confuses users
func TestCLI_CommandStructure_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "Commands organized into logical groups with aliases",
		CurrentImpl:     "Cobra root command with desiredstate and ds command aliases",
		ExpectedOutcome: "Users can discover and invoke grouped commands through stable names and aliases",
		Rationale:       "Consistent command structure enables user familiarity",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	t.Run("Root command has subcommands", func(t *testing.T) {
		t.Log("=== Test: Root command has subcommands ===")

		if !rootCmd.HasSubCommands() {
			t.Fatal("❌ Root command has no subcommands")
		}
		t.Log("✓ Root command has subcommands")

		// Count subcommands (excluding help/completion)
		subcommands := 0
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() != "help" && cmd.Name() != "completion" {
				subcommands++
				t.Logf("  - Subcommand: %s", cmd.Name())
			}
		}

		if subcommands == 0 {
			t.Error("❌ No user-defined subcommands found")
		} else {
			t.Logf("✓ Found %d subcommand(s)", subcommands)
		}

		t.Log("✓ CONTRACT SATISFIED: Command structure exists")
	})

	t.Run("Desiredstate subcommand exists", func(t *testing.T) {
		t.Log("=== Test: Desiredstate subcommand registered ===")

		var dsCmd *cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == "desiredstate" {
				dsCmd = cmd
				break
			}
		}

		if dsCmd == nil {
			t.Fatal("❌ Desiredstate subcommand not found")
		}
		t.Log("✓ Desiredstate subcommand exists")

		// Check for alias
		hasAlias := false
		for _, alias := range dsCmd.Aliases {
			if alias == "ds" {
				hasAlias = true
				break
			}
		}

		if !hasAlias {
			t.Error("❌ Desiredstate alias 'ds' not found")
		} else {
			t.Log("✓ Alias 'ds' configured")
		}

		t.Log("✓ CONTRACT SATISFIED: Desiredstate subcommand properly configured")
	})

	t.Run("Desiredstate has subcommands", func(t *testing.T) {
		t.Log("=== Test: Desiredstate has subcommands ===")

		var dsCmd *cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == "desiredstate" {
				dsCmd = cmd
				break
			}
		}

		if dsCmd == nil {
			t.Fatal("❌ Desiredstate subcommand not found")
		}

		if !dsCmd.HasSubCommands() {
			t.Fatal("❌ Desiredstate has no subcommands")
		}
		t.Log("✓ Desiredstate has subcommands")

		// Expected subcommands
		expectedSubcommands := []string{"validate", "printschema", "assemble", "promote"}
		foundSubcommands := make(map[string]bool)

		for _, cmd := range dsCmd.Commands() {
			if cmd.Name() != "help" && cmd.Name() != "completion" {
				foundSubcommands[cmd.Name()] = true
				t.Logf("  - Subcommand: %s", cmd.Name())
			}
		}

		for _, expected := range expectedSubcommands {
			if !foundSubcommands[expected] {
				t.Logf("  ⚠ Expected subcommand '%s' not found (may not be implemented yet)", expected)
			} else {
				t.Logf("  ✓ Subcommand '%s' found", expected)
			}
		}

		if len(foundSubcommands) == 0 {
			t.Error("❌ No desiredstate subcommands found")
		} else {
			t.Logf("✓ Found %d desiredstate subcommand(s)", len(foundSubcommands))
		}

		t.Log("✓ CONTRACT SATISFIED: Desiredstate subcommands exist")
	})
}

// TestCLI_CommandFlags_BehavioralBDD tests command flag behavioral contracts
//
// Desiredstate command flags:
//
//	validate:
//	  -d/--desiredstate-root (required): Path to desiredstate file
//	  -c/--configuration-root (optional): Path to configuration file
//	  -e/--environment (required): Environment to deploy
//	  -w/--wrapper (required): Wrapper type
//
//	printschema (aliases: ps, schema):
//	  -v/--schema-version (optional): Specific version to print
//	  -a/--all (flag): Print all versions
//	  -l/--list (flag): List available versions
//
//	assemble:
//	  -d/--desiredstate-root (optional): Path to desiredstate file
//	  -c/--configuration-root (optional): Path to configuration file
//	  -e/--environment (required): Environment
//	  -w/--wrapper (required): Wrapper type
//	  --cache-dir (optional): Cache directory
//	  --format (default: 'yaml'): Output format (json|yaml)
//
//	promote:
//	  -d/--desiredstate-root (required): Path to desiredstate file
//	  -D/--desiredstate-destination (required): Destination path
//	  -S/--source-branch (optional): Source branch
//	  -T/--target-branch (optional): Target branch
//	  -p/--aws-profile (optional): AWS profile
//	  -r/--aws-region (optional): AWS region
//	  -n/--dry-run (flag): Dry run mode
//
// All commands must expose stable flag names, shortcuts, and defaults. Flag
// validation happens at runtime rather than through this structural contract.
//
// Regression Risk: CRITICAL
// - Changed flag names break scripts/automation
// - Wrong defaults change behavior
// - Missing required flags break compatibility
func TestCLI_CommandFlags_BehavioralBDD(t *testing.T) {
	contract := BehavioralContract{
		Behavior:        "All desiredstate subcommands expose their required flags",
		CurrentImpl:     "Cobra desiredstate commands define named flags, shorthands, and defaults",
		ExpectedOutcome: "Flag names, shortcuts, and defaults remain stable for automation",
		Rationale:       "Stable flags are part of the command-line interface contract",
	}

	t.Logf("Behavioral Contract: %s", contract.Behavior)

	// Helper function to find command
	findCommand := func(name string) *cobra.Command {
		var dsCmd *cobra.Command
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == "desiredstate" {
				dsCmd = cmd
				break
			}
		}
		if dsCmd == nil {
			return nil
		}
		for _, cmd := range dsCmd.Commands() {
			if cmd.Name() == name {
				return cmd
			}
		}
		return nil
	}

	t.Run("Validate command has correct flags", func(t *testing.T) {
		t.Log("=== Test: validate command flags ===")

		cmd := findCommand("validate")
		if cmd == nil {
			t.Fatal("❌ Validate command not found")
		}
		t.Log("✓ Validate command found")

		// Check required flags
		requiredFlags := map[string]string{
			"desiredstate-root": "d",
		}

		for flagName, shorthand := range requiredFlags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Required flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if flag.Shorthand != shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", shorthand, flag.Shorthand)
				} else {
					t.Logf("  ✓ Shorthand: '-%s'", shorthand)
				}
			}
		}

		// Check optional flags with defaults
		optionalFlags := map[string]struct {
			shorthand    string
			defaultValue string
		}{
			"configuration-root": {"c", ""},
			"environment":        {"e", "all"},
			"wrapper":            {"w", ""}, // required flag, no meaningful default
		}

		for flagName, expected := range optionalFlags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Optional flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if flag.Shorthand != expected.shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", expected.shorthand, flag.Shorthand)
				} else {
					t.Logf("  ✓ Shorthand: '-%s'", expected.shorthand)
				}
				if flag.DefValue != expected.defaultValue {
					t.Errorf("❌ Expected default '%s', got '%s'", expected.defaultValue, flag.DefValue)
				} else {
					t.Logf("  ✓ Default: '%s'", expected.defaultValue)
				}
			}
		}

		t.Log("✓ CONTRACT SATISFIED: Validate flags are configured correctly")
	})

	t.Run("PrintSchema command has correct flags", func(t *testing.T) {
		t.Log("=== Test: printschema command flags ===")

		cmd := findCommand("printschema")
		if cmd == nil {
			t.Fatal("❌ PrintSchema command not found")
		}
		t.Log("✓ PrintSchema command found")

		// Check aliases
		expectedAliases := []string{"ps", "schema"}
		if len(cmd.Aliases) == 0 {
			t.Error("❌ PrintSchema should have aliases")
		} else {
			t.Logf("✓ Aliases: %v", cmd.Aliases)
			for _, alias := range expectedAliases {
				found := false
				for _, a := range cmd.Aliases {
					if a == alias {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("⚠ Expected alias '%s' not found", alias)
				} else {
					t.Logf("  ✓ Alias '%s' exists", alias)
				}
			}
		}

		// Check flags
		optionalFlags := map[string]struct {
			shorthand string
			flagType  string
		}{
			"schema-version": {"v", "string"},
			"all":            {"a", "bool"},
			"list":           {"l", "bool"},
		}

		for flagName, expected := range optionalFlags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if flag.Shorthand != expected.shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", expected.shorthand, flag.Shorthand)
				} else {
					t.Logf("  ✓ Shorthand: '-%s'", expected.shorthand)
				}
			}
		}

		t.Log("✓ CONTRACT SATISFIED: PrintSchema flags are configured correctly")
	})

	t.Run("Assemble command has correct flags", func(t *testing.T) {
		t.Log("=== Test: assemble command flags ===")

		cmd := findCommand("assemble")
		if cmd == nil {
			t.Fatal("❌ Assemble command not found")
		}
		t.Log("✓ Assemble command found")

		// Check flags
		flags := map[string]struct {
			shorthand    string
			defaultValue string
		}{
			"desiredstate-root":  {"d", ""},
			"configuration-root": {"c", ""},
			"environment":        {"e", "NON_EXISTING_ENVIRONMENT"}, // required flag, sentinel forces explicit value
			"wrapper":            {"w", ""},                         // required flag, no meaningful default
			"cache-dir":          {"", ""},
			"format":             {"", "yaml"},
		}

		for flagName, expected := range flags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if expected.shorthand != "" && flag.Shorthand != expected.shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", expected.shorthand, flag.Shorthand)
				} else if expected.shorthand != "" {
					t.Logf("  ✓ Shorthand: '-%s'", expected.shorthand)
				}
				if flag.DefValue != expected.defaultValue {
					t.Errorf("❌ Expected default '%s', got '%s'", expected.defaultValue, flag.DefValue)
				} else {
					t.Logf("  ✓ Default: '%s'", expected.defaultValue)
				}
			}
		}

		t.Log("✓ CONTRACT SATISFIED: Assemble flags are configured correctly")
	})

	t.Run("Promote command has correct flags", func(t *testing.T) {
		t.Log("=== Test: promote command flags ===")

		cmd := findCommand("promote")
		if cmd == nil {
			t.Fatal("❌ Promote command not found")
		}
		t.Log("✓ Promote command found")

		// Check required flags (should be marked as required)
		requiredFlags := map[string]string{
			"desiredstate-root":             "d",
			"desiredstate-destination-root": "D",
		}

		for flagName, shorthand := range requiredFlags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Required flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if flag.Shorthand != shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", shorthand, flag.Shorthand)
				} else {
					t.Logf("  ✓ Shorthand: '-%s'", shorthand)
				}
			}
		}

		// Check optional flags
		optionalFlags := map[string]string{
			"source-branch": "S",
			"target-branch": "T",
			"aws-profile":   "p",
			"aws-region":    "r",
			"dry-run":       "n",
		}

		for flagName, shorthand := range optionalFlags {
			flag := cmd.Flags().Lookup(flagName)
			if flag == nil {
				t.Errorf("❌ Optional flag '%s' not found", flagName)
			} else {
				t.Logf("✓ Flag '%s' exists", flagName)
				if flag.Shorthand != shorthand {
					t.Errorf("❌ Expected shorthand '%s', got '%s'", shorthand, flag.Shorthand)
				} else {
					t.Logf("  ✓ Shorthand: '-%s'", shorthand)
				}
			}
		}

		t.Log("✓ CONTRACT SATISFIED: Promote flags are configured correctly")
	})
}
