package cli

import (
	"os"
	"strings"

	"github.com/danieleborsaro/yago/internal/buildinfo"
	coreRepo "github.com/danieleborsaro/yago/internal/repo"
	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	_ "github.com/danieleborsaro/yago/pkg/concourse"    // Import to trigger factory registration
	_ "github.com/danieleborsaro/yago/pkg/desiredstate" // Import to trigger factory registration
	_ "github.com/danieleborsaro/yago/pkg/terraform"    // Import to trigger factory registration
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

var (
	version             = buildinfo.Version
	logLevel            string
	verbose             bool
	schemaConfig        string
	namespace           string
	useFixedRepoBaseDir bool
	repoBaseDir         string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "yago",
	Short:   "Yet Another GitOps framework in Go - Tools for handling GitOps based desired state configuration",
	Long:    `YAGO (Yet Another GitOps framework in Go) - GitOps tools for handling desired state configuration in the context of a delivery pipeline.`,
	Version: buildinfo.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Start command execution timer
		logging.StartCommandTimer()

		// Set global schema config path if provided
		if schemaConfig != "" {
			schema.SetSchemaConfigPath(schemaConfig)
		}

		// Set global namespace override if provided
		if namespace != "" {
			schema.SetNamespaceOverride(namespace)
		}

		// Configure repo clone base directory strategy.
		// Default: temp directory (parallel-safe). Opt-in fixed mode for stable paths.
		coreRepo.SetCloneBaseDirMode(useFixedRepoBaseDir, repoBaseDir)

		// Set up logging based on flags and environment variables
		// Priority: --verbose flag > --log-level flag > LOG_LEVEL env var
		if verbose {
			logging.SetLevel(logging.DEBUG)
		} else if logLevel != "" {
			if level, err := logging.ParseLevel(logLevel); err == nil {
				logging.SetLevel(level)
			}
		} else if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
			// Handle LOG_LEVEL environment variable (case-insensitive)
			if level, err := logging.ParseLevel(strings.ToUpper(envLogLevel)); err == nil {
				logging.SetLevel(level)
			}
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Print execution time at the end of command
		logging.PrintExecutionTime()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

// GetSchemaConfigPath returns the schema config path from the global flag
func GetSchemaConfigPath() string {
	return schemaConfig
}

func init() {
	// Add global flags (-v is reserved for version)
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "Set log level (debug, info, warn, error)")
	// Note: -v is reserved for version (and schema-version in subcommands), so verbose uses different flag
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging (debug level)")
	rootCmd.PersistentFlags().StringVar(&schemaConfig, "schema-config", "", "Path to schema-config.json (overrides YAGO_SCHEMA_CONFIG and default locations)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Override the schema namespace (default: value in document, fallback: yago)")
	rootCmd.PersistentFlags().BoolVar(&useFixedRepoBaseDir, "use-fixed-repo-basedir", false, "Use fixed base directory for cloned repositories (default uses a unique temp dir)")
	rootCmd.PersistentFlags().StringVar(&repoBaseDir, "repo-basedir", "/tmp/gitops-repo", "Base directory for cloned repositories when --use-fixed-repo-basedir is enabled")

	// Auto-discover and add wrapper commands
	// Each wrapper that implements WrapperFactory will auto-register in its init()
	// We then discover and add all registered wrappers as subcommands
	for _, name := range wrapper.ListWrappers() {
		factory, err := wrapper.GetWrapper(name)
		if err != nil {
			logging.Warn("Failed to load wrapper '%s': %v", name, err)
			continue
		}
		rootCmd.AddCommand(factory.CreateCLICommand())
	}

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
}
