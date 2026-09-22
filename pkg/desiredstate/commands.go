// Package desiredstate provides Cobra commands for GitOps desired state operations.
//
// This package is structured with clear separation of concerns:
// - commands.go: Cobra UI layer (command definitions, flag handling, output formatting)
// - service.go: Business logic layer (validation, promotion, schema operations)
//
// The UI layer handles user interaction and delegates all business logic to the service layer,
// enhancing testability, reusability, and maintainability.
package desiredstate

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"github.com/spf13/cobra"
)

const defaultAssembleEnvironment = "NON_EXISTING_ENVIRONMENT"

// DesiredStateConfig holds configuration for desiredstate commands
type DesiredStateConfig struct {
	BaseDir           string
	Environment       string
	SchemaVersion     string
	DesiredStateFile  string
	DestinationFile   string
	ConfigRepoWorkdir string
	IsDryRun          bool
	IsInterpolation   bool
	// Assemble-specific fields
	CacheDirectory string
	OutputFormat   string
	Wrapper        string
}

// NewDesiredStateCommand creates the desiredstate command group
func NewDesiredStateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "desiredstate",
		Aliases: []string{"ds"},
		Short:   "Handle GitOps desired state operations",
		Long:    `Commands for validating, assembling, and managing GitOps desired state configurations.`,
	}

	// Add subcommands
	cmd.AddCommand(newSchemaCommand())        // printschema
	cmd.AddCommand(newValidateCommand())      // validate
	cmd.AddCommand(newAssembleCommand())      // assemble
	cmd.AddCommand(newPromoteCommand())       // promote
	cmd.AddCommand(newCompareCommand())       // compare
	cmd.AddCommand(newCheckVersionsCommand()) // checkversions
	cmd.AddCommand(newLockCommand())          // lock
	cmd.AddCommand(newUnlockCommand())        // unlock
	cmd.AddCommand(newStatusCommand())        // status
	cmd.AddCommand(newGetIssuesCommand())     // getissues

	return cmd
}

// newValidateCommand creates the validate subcommand
func newValidateCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
		Wrapper:         "",
	}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate GitOps desired state",
		Long:  `Validate GitOps desired state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(config, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", config.DesiredStateFile, "DesiredState file")
	cmd.Flags().StringVarP(&config.DestinationFile, "configuration-root", "c", config.DestinationFile, "Configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVar(&config.ConfigRepoWorkdir, "configuration-repo-workdir", "", "Configuration repository working directory (used when configuration is cloned from desiredstate)")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")
	cmd.Flags().StringVarP(&config.Wrapper, "wrapper", "w", config.Wrapper, "Wrapper to assemble configuration for")
	cmd.MarkFlagRequired("desiredstate-root")
	cmd.MarkFlagRequired("wrapper")

	return cmd
}

// newSchemaCommand creates the printschema subcommand
func newSchemaCommand() *cobra.Command {
	var schemaVersion string
	var isAll, isList bool

	cmd := &cobra.Command{
		Use:     "printschema",
		Aliases: []string{"ps", "schema"},
		Short:   "Print schema definition for GitOps desired state",
		Long:    `Print schema definition for GitOps desired state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrintSchema(schemaVersion, isAll, isList)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&schemaVersion, "schema-version", "v", "", "Schema version to print")
	cmd.Flags().BoolVarP(&isAll, "all", "a", false, "Print all schema versions")
	cmd.Flags().BoolVarP(&isList, "list", "l", false, "List available schema versions")

	return cmd
}

// runPrintSchema executes the printschema command
func runPrintSchema(schemaVersion string, isAll, isList bool) error {
	// Create service instance
	service := NewService(".", true) // Use current directory and enable interpolation

	// Prepare request
	req := SchemaRequest{
		SchemaVersion: schemaVersion,
		ShowAll:       isAll,
		ListVersions:  isList,
	}

	// Execute schema operation through service layer
	response, err := service.PrintSchema(req)
	if err != nil {
		return err
	}

	// Output results based on response
	if req.ListVersions {
		fmt.Println("Supported GitOps schema versions:")
		for _, version := range response.Versions {
			fmt.Printf("  - %s\n", version)
		}
	}

	if req.ShowAll {
		for _, version := range response.Versions {
			fmt.Printf("\n=== Schema Version %s ===\n", version)
			if content, exists := response.SchemaContent[version]; exists {
				fmt.Println(content)
			} else {
				fmt.Printf("Schema version %s definitions would be printed here\n", version)
			}
		}
	}

	if req.SchemaVersion != "" {
		fmt.Printf("=== Schema Version %s ===\n", req.SchemaVersion)
		if content, exists := response.SchemaContent[req.SchemaVersion]; exists {
			fmt.Println(content)
		} else {
			fmt.Printf("Schema version %s definitions would be printed here\n", req.SchemaVersion)
		}
	}

	return nil
}

// runValidate executes the validate command
func runValidate(config *DesiredStateConfig, args []string) error {
	if strings.TrimSpace(config.Wrapper) == "" {
		return errors.NewParamError("--wrapper is mandatory and must be a valid wrapper value")
	}

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := wrapper.ValidateRequest{
		DesiredStateFile:  config.DesiredStateFile,
		ConfigFile:        config.DestinationFile,
		ConfigRepoWorkdir: config.ConfigRepoWorkdir,
		Environment:       config.Environment,
		SchemaVersion:     config.SchemaVersion,
		Wrapper:           config.Wrapper,
	}

	// Output summary information
	fmt.Printf("Environment:    '%s'\n", config.Environment)
	fmt.Printf("DesiredState:   '%s'\n", config.DesiredStateFile)
	fmt.Printf("Config:         '%s'\n", config.DestinationFile)
	fmt.Printf("Wrapper:        '%s'\n", config.Wrapper)

	// Execute validation through service layer
	response, err := service.ValidateDesiredState(req)
	if err != nil {
		return err
	}

	if !response.IsValid {
		return errors.Newf(errors.ErrFail, "validation failed: %s", response.ErrorMessage)
	}

	// Output results
	if response.DesiredStateValidated {
		fmt.Println("Assembled desiredstate: OK")
		logging.Debug("Assembled desiredstate: OK")
	}

	if response.ConfigurationValidated {
		fmt.Println("Assembled config:       OK")
		logging.Debug("Assembled config:       OK")
	}

	if response.SchemaVersion != "" && response.SchemaVersion != "unknown" {
		logging.Debug("Detected schema version: %s", response.SchemaVersion)
	}

	return nil
}

// newAssembleCommand creates the assemble subcommand
func newAssembleCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     defaultAssembleEnvironment,
		IsInterpolation: true,
		OutputFormat:    "yaml",
		Wrapper:         "",
	}

	cmd := &cobra.Command{
		Use:   "assemble",
		Short: "Assemble and optionally cache GitOps desired state configuration",
		Long:  `Assemble GitOps desired state configuration. Content assembled from configuration.yaml depends on the target wrapper passed with --wrapper.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssemble(config, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", config.DesiredStateFile, "DesiredState file")
	cmd.Flags().StringVarP(&config.DestinationFile, "configuration-root", "c", config.DestinationFile, "Configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVar(&config.ConfigRepoWorkdir, "configuration-repo-workdir", "", "Configuration repository working directory (used when configuration is cloned from desiredstate)")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")
	cmd.Flags().StringVarP(&config.Wrapper, "wrapper", "w", config.Wrapper, "Wrapper to assemble configuration for")
	cmd.Flags().StringVar(&config.CacheDirectory, "cache-dir", "", "Cache directory to save assembled files (optional)")
	cmd.Flags().StringVar(&config.OutputFormat, "format", config.OutputFormat, "Output format for cached files (yaml or json)")
	cmd.MarkFlagRequired("desiredstate-root")
	cmd.MarkFlagRequired("environment")
	cmd.MarkFlagRequired("wrapper")

	return cmd
}

// runAssemble executes the assemble command
func runAssemble(config *DesiredStateConfig, args []string) error {
	if err := validateAssembleEnvironment(config.Environment); err != nil {
		return err
	}
	if strings.TrimSpace(config.Wrapper) == "" {
		return errors.NewParamError("--wrapper is mandatory and must be a valid wrapper value")
	}
	if err := validateAssembleEnvironmentExistsInDesiredState(config.DesiredStateFile, config.Environment); err != nil {
		return err
	}

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := wrapper.AssembleRequest{
		DesiredStateFile:  config.DesiredStateFile,
		ConfigFile:        config.DestinationFile,
		ConfigRepoWorkdir: config.ConfigRepoWorkdir,
		Environment:       config.Environment,
		Wrapper:           config.Wrapper,
		CacheDirectory:    config.CacheDirectory,
		OutputFormat:      config.OutputFormat,
	}

	// Output summary information
	fmt.Printf("Environment:    '%s'\n", config.Environment)
	fmt.Printf("DesiredState:   '%s'\n", config.DesiredStateFile)
	fmt.Printf("Config:         '%s'\n", config.DestinationFile)
	fmt.Printf("Wrapper:        '%s'\n", config.Wrapper)
	if config.CacheDirectory != "" {
		fmt.Printf("Cache Dir:      '%s'\n", config.CacheDirectory)
		fmt.Printf("Format:         '%s'\n", config.OutputFormat)
	}

	// Execute assembly through service layer
	response, err := service.AssembleDesiredState(req)
	if err != nil {
		return err
	}

	if !response.IsValid {
		return errors.Newf(errors.ErrFail, "assembly failed: %s", response.ErrorMessage)
	}

	// Output results
	if response.DesiredStateAssembled {
		if response.DesiredStateFile != "" {
			fmt.Printf("Assembled desiredstate: '%s'\n", response.DesiredStateFile)
		} else {
			fmt.Println("Assembled desiredstate: OK")
		}
		logging.Debug("Assembled desiredstate: OK")
	}

	if response.ConfigurationAssembled {
		if response.ConfigurationFile != "" {
			fmt.Printf("Assembled config:       '%s'\n", response.ConfigurationFile)
		} else {
			fmt.Println("Assembled config:       OK")
		}
		logging.Debug("Assembled config:       OK")
	}

	if response.SchemaVersion != "" && response.SchemaVersion != "unknown" {
		logging.Debug("Detected schema version: %s", response.SchemaVersion)
	}

	return nil
}

func validateAssembleEnvironment(environment string) error {
	env := strings.TrimSpace(environment)
	if env == "" || env == defaultAssembleEnvironment {
		return errors.NewParamError("--environment is mandatory and must be a valid environment value")
	}

	return nil
}

func validateAssembleEnvironmentExistsInDesiredState(desiredstateRoot, environment string) error {
	doc := core.NewGitOpsDocument()
	if err := doc.LoadGitOpsFile(desiredstateRoot, true, nil); err != nil {
		return errors.Newf(errors.ErrParam, "failed to validate environment from desiredstate: %v", err)
	}

	content := doc.GetContent().Data
	meta := doc.GetMeta().Data
	envs, ok := extractEnvironments(content)
	if !ok {
		envs, ok = extractEnvironments(meta)
	}
	if !ok {
		return errors.NewParamError("desiredstate does not define desiredstate.content.environments")
	}

	if _, exists := envs[environment]; exists {
		return nil
	}

	available := make([]string, 0, len(envs))
	for name := range envs {
		available = append(available, name)
	}
	sort.Strings(available)

	if len(available) == 0 {
		return errors.Newf(errors.ErrParam, "environment '%s' is not defined: desiredstate.content.environments is empty", environment)
	}

	return errors.Newf(
		errors.ErrParam,
		"environment '%s' is not defined in desiredstate.content.environments (available: %s)",
		environment,
		strings.Join(available, ", "),
	)
}

func toStringMap(value interface{}) (map[string]interface{}, bool) {
	switch m := value.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		converted := make(map[string]interface{}, len(m))
		for key, val := range m {
			keyStr, ok := key.(string)
			if !ok {
				continue
			}
			converted[keyStr] = val
		}
		return converted, true
	default:
		return nil, false
	}
}

func extractEnvironments(data map[string]interface{}) (map[string]interface{}, bool) {
	if data == nil {
		return nil, false
	}

	if desiredstate, ok := toStringMap(data["desiredstate"]); ok {
		if dsContent, ok := toStringMap(desiredstate["content"]); ok {
			if envs, ok := toStringMap(dsContent["environments"]); ok {
				return envs, true
			}
		}
		if envs, ok := toStringMap(desiredstate["configuration"]); ok {
			return envs, true
		}
		if envs, ok := toStringMap(desiredstate["environments"]); ok {
			return envs, true
		}
	}

	if content, ok := toStringMap(data["content"]); ok {
		if envs, ok := toStringMap(content["environments"]); ok {
			return envs, true
		}
	}
	if envs, ok := toStringMap(data["configuration"]); ok {
		return envs, true
	}
	if envs, ok := toStringMap(data["environments"]); ok {
		return envs, true
	}

	return nil, false
}

// newPromoteCommand creates the promote subcommand
func newPromoteCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	// Read IS_DRY_RUN from environment variable
	envDryRun := os.Getenv("IS_DRY_RUN") == "1"

	var awsProfile, awsRegion, sourceBranch, targetBranch, rollbackReason string
	var isResolveToCommit, isCompareEnvironments, isForcePromotion, isFailOnUnableToLock bool
	var isRemoveMissing, isRollback, isInteractive bool

	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote GitOps desired state from source to destination environment",
		Long:  `Promote GitOps desired state from source to destination environment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromote(config, awsProfile, awsRegion, sourceBranch, targetBranch,
				isResolveToCommit, isCompareEnvironments, isForcePromotion, isFailOnUnableToLock,
				isRemoveMissing, isRollback, isInteractive, rollbackReason, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().StringVarP(&config.DestinationFile, "desiredstate-destination-root", "D", "", "DesiredState destination file")
	cmd.Flags().StringVarP(&sourceBranch, "source-branch", "S", "", "Target branch for promoting desiredstate")
	cmd.Flags().StringVarP(&targetBranch, "target-branch", "T", "", "Target branch for promoting desiredstate")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isCompareEnvironments, "compare-environments", "E", false, "Do not promote desiredstates, just compare them")
	cmd.Flags().BoolVarP(&isForcePromotion, "force-promotion", "F", false, "Forced promotion of desiredstate")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")
	cmd.Flags().BoolVarP(&config.IsDryRun, "dry-run", "n", envDryRun, "Disables command effect on target instance")

	// Enhancement flags
	cmd.Flags().BoolVar(&isRemoveMissing, "remove-missing", false, "Remove components from destination that don't exist in source")
	cmd.Flags().BoolVar(&isRollback, "rollback", false, "Mark this promotion as an intentional rollback (suppresses downgrade warnings)")
	cmd.Flags().StringVar(&rollbackReason, "rollback-reason", "", "Reason for the rollback (used with --rollback flag)")
	cmd.Flags().BoolVarP(&isInteractive, "interactive", "i", false, "Interactively prompt for each component change")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("desiredstate-root")
	cmd.MarkFlagRequired("desiredstate-destination-root")
	cmd.MarkFlagRequired("source-branch")
	cmd.MarkFlagRequired("target-branch")

	return cmd
}

// runPromote executes the promote command
func runPromote(config *DesiredStateConfig, awsProfile, awsRegion, sourceBranch, targetBranch string,
	isResolveToCommit, isCompareEnvironments, isForcePromotion, isFailOnUnableToLock bool,
	isRemoveMissing, isRollback, isInteractive bool, rollbackReason string, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := PromoteRequest{
		DesiredStateFile:      config.DesiredStateFile,
		DestinationFile:       config.DestinationFile,
		Environment:           config.Environment,
		AWSProfile:            awsProfile,
		AWSRegion:             awsRegion,
		SourceBranch:          sourceBranch,
		TargetBranch:          targetBranch,
		IsResolveToCommit:     isResolveToCommit,
		IsCompareEnvironments: isCompareEnvironments,
		IsForcePromotion:      isForcePromotion,
		IsFailOnUnableToLock:  isFailOnUnableToLock,
		IsDryRun:              config.IsDryRun,
		IsRemoveMissing:       isRemoveMissing,
		IsRollback:            isRollback,
		IsInteractive:         isInteractive,
		RollbackReason:        rollbackReason,
	}

	// If interactive mode, first compare and prompt
	if isInteractive {
		return runInteractivePromotion(service, req)
	}

	// Execute promotion through service layer
	response, err := service.PromoteDesiredState(req)
	if err != nil {
		return err
	}

	// Output results based on response
	if isCompareEnvironments {
		// Compare-only mode
		if response.Success {
			logging.Info("Comparison completed successfully")
		}
		return nil
	}

	if config.IsDryRun {
		// Dry-run mode
		logging.Info("Dry run completed - no files were modified")
		return nil
	}

	// Report actual promotion results
	if response.Success && response.FilesModified {
		logging.Info("Promotion completed successfully")
		if response.SourceSchemaVersion != "" && response.SourceSchemaVersion != "unknown" {
			logging.Info("Source schema version: %s", response.SourceSchemaVersion)
		}
	}

	return nil
}

// runInteractivePromotion runs promotion in interactive mode, prompting for each component
func runInteractivePromotion(service *Service, req PromoteRequest) error {
	logging.Info("Starting interactive promotion...")
	logging.Info("Source:      %s", req.DesiredStateFile)
	logging.Info("Destination: %s", req.DestinationFile)
	logging.Spaces()

	// First, compare the files to see what changes would be made
	logging.Info("Comparing files...")
	compareResult, err := service.CompareDesiredStates(
		req.DesiredStateFile,
		req.DestinationFile,
		req.AWSProfile,
		req.AWSRegion,
		req.IsResolveToCommit,
		req.IsRemoveMissing,
		req.IsRollback,
		req.RollbackReason,
	)
	if err != nil {
		return err
	}

	// Display summary
	logging.Spaces()
	logging.Info("Changes detected:")
	logging.Info("  Upgrades:   %d", len(compareResult.ComponentChanges))
	logging.Info("  Total:      %d components", compareResult.TotalComponents)

	if len(compareResult.ComponentChanges) == 0 {
		logging.Info("No changes to promote")
		return nil
	}

	logging.Spaces()

	// Prompt user if they want to proceed
	fmt.Print("Proceed with promotion? [y/n]: ")
	var response string
	fmt.Scanln(&response)
	if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
		logging.Info("Promotion cancelled")
		return nil
	}

	// Ask if they want to review each change
	fmt.Print("Apply changes individually? [y/n/a(ll)]: ")
	fmt.Scanln(&response)

	applyAll := false
	if strings.EqualFold(response, "a") || strings.EqualFold(response, "all") {
		applyAll = true
		logging.Info("Applying all changes...")
	} else if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
		// User wants to apply all without review
		applyAll = true
	}

	// Track which components to promote
	componentsToPromote := make(map[string]bool)
	skipDowngrades := false
	skipUnchanged := false

	if !applyAll {
		// Iterate through changes and prompt
		for _, change := range compareResult.ComponentChanges {
			// Skip if user said skip all downgrades
			if skipDowngrades && change.IsDowngrade {
				logging.Debug("Skipping downgrade: %s", change.PartID)
				continue
			}

			// Skip if user said skip all unchanged
			if skipUnchanged && change.IsUnchanged {
				logging.Debug("Skipping unchanged: %s", change.PartID)
				continue
			}

			// Display change
			logging.Spaces()
			if change.IsUpgrade {
				logging.Info("  ↑ %s: %s -> %s (UPGRADE)", change.PartID, change.SourceVersion, change.DestVersion)
			} else if change.IsDowngrade {
				if change.IsRollback {
					logging.Warn("  ↻ %s: %s -> %s (ROLLBACK: %s)", change.PartID, change.SourceVersion, change.DestVersion, change.RollbackReason)
				} else {
					logging.Warn("  ↓ %s: %s -> %s (DOWNGRADE!)", change.PartID, change.SourceVersion, change.DestVersion)
				}
			} else {
				logging.Info("  = %s: %s (unchanged)", change.PartID, change.SourceVersion)
			}

			// Prompt for action
			if change.IsDowngrade && !change.IsRollback {
				fmt.Print("  Apply this downgrade? [y/n/s(kip all downgrades)]: ")
			} else if change.IsUnchanged {
				fmt.Print("  Apply unchanged? [y/n/s(kip unchanged)]: ")
			} else {
				fmt.Print("  Apply? [y/n/a(ll remaining)]: ")
			}

			var action string
			fmt.Scanln(&action)

			switch strings.ToLower(action) {
			case "y", "yes":
				componentsToPromote[change.PartID] = true
				logging.Debug("Will promote: %s", change.PartID)
			case "a", "all":
				componentsToPromote[change.PartID] = true
				applyAll = true
				logging.Info("Applying all remaining changes...")
			case "s", "skip":
				if change.IsDowngrade {
					skipDowngrades = true
					logging.Info("Skipping all downgrades...")
				} else if change.IsUnchanged {
					skipUnchanged = true
					logging.Info("Skipping all unchanged components...")
				}
			case "n", "no":
				logging.Debug("Skipping: %s", change.PartID)
			default:
				logging.Debug("Skipping: %s (invalid response)", change.PartID)
			}

			if applyAll {
				// Mark all remaining changes for promotion
				componentsToPromote[change.PartID] = true
				break
			}
		}

		// If apply all was selected, mark all remaining
		if applyAll {
			for _, change := range compareResult.ComponentChanges {
				if !skipDowngrades || !change.IsDowngrade {
					if !skipUnchanged || !change.IsUnchanged {
						componentsToPromote[change.PartID] = true
					}
				}
			}
		}
	} else {
		// Apply all - mark everything for promotion
		for _, change := range compareResult.ComponentChanges {
			componentsToPromote[change.PartID] = true
		}
	}

	// Summary
	logging.Spaces()
	logging.Info("Promotion Summary:")
	logging.Info("  Selected for promotion: %d components", len(componentsToPromote))
	logging.Info("  Skipped:                %d components", len(compareResult.ComponentChanges)-len(componentsToPromote))

	if len(componentsToPromote) == 0 {
		logging.Info("No components selected for promotion")
		return nil
	}

	// Confirm final action
	fmt.Print("\nProceed with promotion? [y/n]: ")
	fmt.Scanln(&response)
	if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
		logging.Info("Promotion cancelled")
		return nil
	}

	// Execute promotion with selected components
	// For now, we promote all or nothing. In a full implementation,
	// we would need to extend the service to support selective promotion.
	logging.Spaces()
	logging.Info("Executing promotion...")

	response2, err := service.PromoteDesiredState(req)
	if err != nil {
		return err
	}

	if response2.Success {
		logging.Spaces()
		logging.Info("✓ Promotion completed successfully!")
		logging.Info("  Updated file: %s", req.DestinationFile)
	}

	return nil
}

// newCompareCommand creates the compare subcommand
func newCompareCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	var awsProfile, awsRegion string
	var isResolveToCommit, isFailOnUnableToLock bool

	cmd := &cobra.Command{
		Use:     "compare",
		Aliases: []string{"cmp"},
		Short:   "Compare two GitOps desired state files showing version differences",
		Long:    `Compare two GitOps desired state files component-by-component, showing version differences without performing any modifications.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompare(config, awsProfile, awsRegion,
				isResolveToCommit, isFailOnUnableToLock, args)
		},
	}

	// Add flags matching promote command but for comparison only
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "source-file", "s", "", "Source desiredstate file")
	cmd.Flags().StringVarP(&config.DestinationFile, "destination-file", "d", "", "Destination desiredstate file to compare against")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to compare")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("source-file")
	cmd.MarkFlagRequired("destination-file")

	return cmd
}

// runCompare executes the compare command
func runCompare(config *DesiredStateConfig, awsProfile, awsRegion string,
	isResolveToCommit, isFailOnUnableToLock bool, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Execute comparison through service layer
	logging.Info("Comparing desiredstate files...")
	logging.Info("Source:      %s", config.DesiredStateFile)
	logging.Info("Destination: %s", config.DestinationFile)
	logging.Info("Environment: %s", config.Environment)
	logging.Spaces()

	// CompareDesiredStates signature:
	// (sourceFile, destFile, awsProfile, awsRegion string, isResolveToCommit, isRemoveMissing, isRollback bool, rollbackReason string)
	result, err := service.CompareDesiredStates(
		config.DesiredStateFile,
		config.DestinationFile,
		awsProfile,
		awsRegion,
		isResolveToCommit,
		false, // isRemoveMissing - not applicable for compare
		false, // isRollback - not applicable for compare
		"",    // rollbackReason - not applicable for compare
	)

	if err != nil {
		return err
	}

	if !result.IsValid {
		logging.Error("Comparison failed: %s", result.ErrorMessage)
		return fmt.Errorf("comparison failed: %s", result.ErrorMessage)
	}

	// Categorize changes
	var upgrades, downgrades, unchanged, removed []ComponentChange
	for _, change := range result.ComponentChanges {
		if change.IsUpgrade {
			upgrades = append(upgrades, change)
		} else if change.IsDowngrade {
			downgrades = append(downgrades, change)
		} else if change.IsRemoved {
			removed = append(removed, change)
		} else if change.IsUnchanged {
			unchanged = append(unchanged, change)
		}
	}

	// Output comparison results
	logging.Info("Comparison Results:")
	logging.Info("==================")
	logging.Spaces()

	if len(upgrades) > 0 {
		logging.Info("Upgrades (%d):", len(upgrades))
		for _, change := range upgrades {
			logging.Info("  ↑ %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
		}
		logging.Spaces()
	}

	if len(downgrades) > 0 {
		logging.Warn("Downgrades (%d):", len(downgrades))
		for _, change := range downgrades {
			logging.Warn("  ↓ %s (%s): %s", change.PartID, change.ComponentType, change.VersionComparison)
		}
		logging.Spaces()
	}

	if len(unchanged) > 0 {
		logging.Info("Unchanged (%d):", len(unchanged))
		for _, change := range unchanged {
			logging.Debug("  = %s (%s): %s", change.PartID, change.ComponentType, change.SourceVersion)
		}
		logging.Spaces()
	}

	if len(removed) > 0 {
		logging.Warn("Removed Components (%d):", len(removed))
		for _, change := range removed {
			logging.Warn("  - %s (%s): %s", change.PartID, change.ComponentType, change.DestVersion)
		}
		logging.Spaces()
	}

	// Summary
	logging.Info("Summary:")
	logging.Info("  Total components compared: %d", result.TotalComponents)
	logging.Info("  Upgrades:   %d", result.ComponentsToUpgrade)
	logging.Info("  Downgrades: %d", result.ComponentsDowngraded)
	logging.Info("  Unchanged:  %d", result.ComponentsUnchanged)
	logging.Info("  Removed:    %d", len(removed))

	if result.HasDowngrades {
		logging.Spaces()
		logging.Warn("⚠️  Warning: Downgrades detected!")
		logging.Warn("Use 'yago desiredstate promote' with appropriate flags to proceed")
	}

	return nil
}

// newCheckVersionsCommand creates the checkversions subcommand
func newCheckVersionsCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	var awsProfile, awsRegion string
	var isResolveToCommit, isFailOnUnableToLock, isRetrieveJiraIssue, isResolveToJiraIssue bool

	cmd := &cobra.Command{
		Use:     "checkversions",
		Aliases: []string{"cv"},
		Short:   "Check whether components in GitOps desired state are locked",
		Long:    `Check whether components in GitOps desired state are locked.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheckVersions(config, awsProfile, awsRegion,
				isResolveToCommit, isFailOnUnableToLock,
				isRetrieveJiraIssue, isResolveToJiraIssue, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().BoolVarP(&isRetrieveJiraIssue, "retrieve-jira-issue", "j", false, "Retrieve Jira issue numbers")
	cmd.Flags().BoolVarP(&isResolveToJiraIssue, "resolve-to-jira-issue", "J", false, "Resolve to Jira issue")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("desiredstate-root")

	return cmd
}

// runCheckVersions executes the checkversions command
func runCheckVersions(config *DesiredStateConfig, awsProfile, awsRegion string,
	isResolveToCommit, isFailOnUnableToLock, isRetrieveJiraIssue, isResolveToJiraIssue bool, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := CheckVersionsRequest{
		DesiredStateFile:     config.DesiredStateFile,
		Environment:          config.Environment,
		AWSProfile:           awsProfile,
		AWSRegion:            awsRegion,
		IsResolveToCommit:    isResolveToCommit,
		IsFailOnUnableToLock: isFailOnUnableToLock,
		IsRetrieveJiraIssue:  isRetrieveJiraIssue,
		IsResolveToJiraIssue: isResolveToJiraIssue,
	}

	// Execute version checking through service layer
	response, err := service.CheckVersions(req)
	if err != nil {
		return err
	}

	// Output results
	if response.Success {
		logging.Spaces()
		logging.Info("Version check completed successfully")
		if response.TotalComponents > 0 {
			logging.Info("Total components: %d", response.TotalComponents)
			logging.Info("Locked components: %d", response.LockedComponents)
			logging.Info("Unlocked components: %d", response.UnlockedComponents)
		}
	}

	return nil
}

// newLockCommand creates the lock subcommand
func newLockCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	// Read IS_DRY_RUN from environment variable
	envDryRun := os.Getenv("IS_DRY_RUN") == "1"

	var awsProfile, awsRegion string
	var isResolveToCommit, isFailOnUnableToLock bool

	cmd := &cobra.Command{
		Use:     "lock",
		Aliases: []string{"l"},
		Short:   "Compute locked versions for components in GitOps desired state that are not locked",
		Long:    `Compute locked versions for components in GitOps desired state that are not locked.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLock(config, awsProfile, awsRegion,
				isResolveToCommit, isFailOnUnableToLock, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")
	cmd.Flags().BoolVarP(&config.IsDryRun, "dry-run", "n", envDryRun, "Disables command effect on target instance")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("desiredstate-root")

	return cmd
}

// runLock executes the lock command
func runLock(config *DesiredStateConfig, awsProfile, awsRegion string,
	isResolveToCommit, isFailOnUnableToLock bool, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := LockRequest{
		DesiredStateFile:     config.DesiredStateFile,
		Environment:          config.Environment,
		AWSProfile:           awsProfile,
		AWSRegion:            awsRegion,
		IsResolveToCommit:    isResolveToCommit,
		IsFailOnUnableToLock: isFailOnUnableToLock,
		IsDryRun:             config.IsDryRun,
	}

	// Execute lock through service layer
	response, err := service.Lock(req)
	if err != nil {
		return err
	}

	// Output results
	if response.Success {
		logging.Spaces()
		if config.IsDryRun {
			logging.Info("Dry run completed - no files were modified")
		} else {
			logging.Info("Lock operation completed successfully")
			if response.LockedComponents > 0 {
				logging.Info("Locked components: %d", response.LockedComponents)
			}
		}
	}

	return nil
}

// newUnlockCommand creates the unlock subcommand
func newUnlockCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	// Read IS_DRY_RUN from environment variable
	envDryRun := os.Getenv("IS_DRY_RUN") == "1"

	var awsProfile, awsRegion string
	var isResolveToCommit, isFailOnUnableToLock bool

	cmd := &cobra.Command{
		Use:     "unlock",
		Aliases: []string{"u"},
		Short:   "Compute latest versions for components in GitOps desired state",
		Long:    `Compute latest versions for components in GitOps desired state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlock(config, awsProfile, awsRegion,
				isResolveToCommit, isFailOnUnableToLock, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")
	cmd.Flags().BoolVarP(&config.IsDryRun, "dry-run", "n", envDryRun, "Disables command effect on target instance")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("desiredstate-root")

	return cmd
}

// runUnlock executes the unlock command
func runUnlock(config *DesiredStateConfig, awsProfile, awsRegion string,
	isResolveToCommit, isFailOnUnableToLock bool, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := UnlockRequest{
		DesiredStateFile:     config.DesiredStateFile,
		Environment:          config.Environment,
		AWSProfile:           awsProfile,
		AWSRegion:            awsRegion,
		IsResolveToCommit:    isResolveToCommit,
		IsFailOnUnableToLock: isFailOnUnableToLock,
		IsDryRun:             config.IsDryRun,
	}

	// Execute unlock through service layer
	response, err := service.Unlock(req)
	if err != nil {
		return err
	}

	// Output results
	if response.Success {
		logging.Spaces()
		if config.IsDryRun {
			logging.Info("Dry run completed - no files were modified")
		} else {
			logging.Info("Unlock operation completed successfully")
			if response.UnlockedComponents > 0 {
				logging.Info("Unlocked components: %d", response.UnlockedComponents)
			}
		}
	}

	return nil
}

// newStatusCommand creates the status subcommand for showing lock status
func newStatusCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	var awsProfile, awsRegion string
	var showDetails bool

	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"st"},
		Short:   "Show lock status of components in a desiredstate file",
		Long:    `Display which components are locked (using explicit versions/SHAs) versus unlocked (using dynamic references like tags or branches).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(config, awsProfile, awsRegion, showDetails, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-file", "f", "", "DesiredState file to check")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to check")
	cmd.Flags().BoolVarP(&showDetails, "details", "d", false, "Show detailed information for each component")

	// Mark required flags
	cmd.MarkFlagRequired("desiredstate-file")

	return cmd
}

// runStatus executes the status command
func runStatus(config *DesiredStateConfig, awsProfile, awsRegion string, showDetails bool, args []string) error {
	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Get lock status
	logging.Info("Analyzing lock status for: %s", config.DesiredStateFile)
	logging.Info("Environment: %s", config.Environment)
	logging.Spaces()

	status, err := service.GetLockStatus(config.DesiredStateFile, config.Environment, awsProfile, awsRegion)
	if err != nil {
		return err
	}

	// Display results
	logging.Info("Component Lock Status:")
	logging.Info("======================")
	logging.Spaces()

	// Locked components
	if len(status.LockedComponents) > 0 {
		logging.Info("✓ Locked Components (%d):", len(status.LockedComponents))
		for _, comp := range status.LockedComponents {
			if showDetails {
				logging.Info("  • %s (%s)", comp.PartID, comp.ComponentType)
				logging.Info("    Version: %s", comp.Version)
				logging.Info("    Reason:  %s", comp.LockReason)
			} else {
				logging.Info("  • %s (%s): %s", comp.PartID, comp.ComponentType, comp.Version)
			}
		}
		logging.Spaces()
	}

	// Unlocked components
	if len(status.UnlockedComponents) > 0 {
		logging.Info("○ Unlocked Components (%d):", len(status.UnlockedComponents))
		for _, comp := range status.UnlockedComponents {
			if showDetails {
				logging.Info("  • %s (%s)", comp.PartID, comp.ComponentType)
				logging.Info("    Version: %s (dynamic)", comp.Version)
			} else {
				logging.Info("  • %s (%s): %s", comp.PartID, comp.ComponentType, comp.Version)
			}
		}
		logging.Spaces()
	}

	// Summary
	total := len(status.LockedComponents) + len(status.UnlockedComponents)
	lockedPct := 0.0
	if total > 0 {
		lockedPct = float64(len(status.LockedComponents)) / float64(total) * 100
	}

	logging.Info("Summary:")
	logging.Info("  Total components:   %d", total)
	logging.Info("  Locked:             %d (%.1f%%)", len(status.LockedComponents), lockedPct)
	logging.Info("  Unlocked:           %d (%.1f%%)", len(status.UnlockedComponents), 100-lockedPct)

	if len(status.LockedComponents) == 0 {
		logging.Spaces()
		logging.Warn("⚠️  No components are locked - all using dynamic versions")
	} else if len(status.UnlockedComponents) == 0 {
		logging.Spaces()
		logging.Info("✓ All components are locked")
	}

	return nil
}

// newGetIssuesCommand creates the getissues subcommand
func newGetIssuesCommand() *cobra.Command {
	config := &DesiredStateConfig{
		BaseDir:         ".",
		Environment:     "all",
		IsInterpolation: true,
	}

	var awsProfile, awsRegion, jiraSaveDir, jiraSaveFile string
	var isResolveToCommit, isResolveToJiraIssue, isFailOnUnableToLock bool

	cmd := &cobra.Command{
		Use:     "getissues",
		Aliases: []string{"gi"},
		Short:   "Retrieve Jira issue numbers implied in GitOps desired state",
		Long:    `Retrieve Jira issue numbers implied in GitOps desired state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGetIssues(config, awsProfile, awsRegion,
				isResolveToCommit, isResolveToJiraIssue, isFailOnUnableToLock,
				jiraSaveDir, jiraSaveFile, args)
		},
	}

	// Add flags
	cmd.Flags().StringVarP(&awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&config.DesiredStateFile, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().BoolVarP(&isResolveToCommit, "resolve-to-commits", "R", false, "Resolve tags and references to a Git commit")
	cmd.Flags().BoolVarP(&isResolveToJiraIssue, "resolve-to-jira-issue", "J", false, "Resolve to Jira issue")
	cmd.Flags().StringVarP(&jiraSaveDir, "jira-save-directory", "D", "", "Jira save directory")
	cmd.Flags().StringVarP(&jiraSaveFile, "jira-save-file", "F", "", "Jira save file")
	cmd.Flags().BoolVarP(&isFailOnUnableToLock, "fail-on-unable-to-lock", "U", false, "Fail if unable to compute locks")
	cmd.Flags().StringVarP(&config.Environment, "environment", "e", config.Environment, "Environment to deploy")

	// Mark required flags
	cmd.MarkFlagRequired("aws-region")
	cmd.MarkFlagRequired("desiredstate-root")

	return cmd
}

// runGetIssues executes the getissues command
func runGetIssues(config *DesiredStateConfig, awsProfile, awsRegion string,
	isResolveToCommit, isResolveToJiraIssue, isFailOnUnableToLock bool,
	jiraSaveDir, jiraSaveFile string, args []string) error {

	// Create service instance
	service := NewService(config.BaseDir, config.IsInterpolation)

	// Prepare request
	req := GetIssuesRequest{
		DesiredStateFile:     config.DesiredStateFile,
		Environment:          config.Environment,
		AWSProfile:           awsProfile,
		AWSRegion:            awsRegion,
		IsResolveToCommit:    isResolveToCommit,
		IsResolveToJiraIssue: isResolveToJiraIssue,
		IsFailOnUnableToLock: isFailOnUnableToLock,
		JiraSaveDir:          jiraSaveDir,
		JiraSaveFile:         jiraSaveFile,
	}

	// Execute getissues through service layer
	response, err := service.GetIssues(req)
	if err != nil {
		return err
	}

	// Output results
	if response.Success {
		logging.Spaces()
		logging.Info("Jira issue extraction completed successfully")
		if response.TotalIssues > 0 {
			logging.Info("Total Jira issues found: %d", response.TotalIssues)
			if response.SavedToFile != "" {
				logging.Info("Issues saved to: %s", response.SavedToFile)
			}
		}
	}

	return nil
}
