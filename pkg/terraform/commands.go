package terraform

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/spf13/cobra"
)

const defaultAssembleEnvironment = "NON_EXISTING_ENVIRONMENT"

// NewTerraformCommand creates the terraform command group.
func NewTerraformCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tf",
		Aliases: []string{"terraform"},
		Short:   "Wrap Terraform commands",
		Long:    `Terraform wrapper commands for GitOps operations.`,
	}

	// Add subcommands
	cmd.AddCommand(newAssembleCommand())
	cmd.AddCommand(newInitCommand())
	cmd.AddCommand(newPlanCommand())
	cmd.AddCommand(newProvisionCommand())
	cmd.AddCommand(newDestroyCommand())
	cmd.AddCommand(newOutputCommand())
	cmd.AddCommand(newGraphCommand())
	cmd.AddCommand(newUnlockCommand())
	cmd.AddCommand(newImportCommand())
	cmd.AddCommand(newCostsCommand())

	return cmd
}

// Common flags structure shared across terraform subcommands
type commonFlags struct {
	awsProfile        string // -p, --aws-profile
	awsRegion         string // -r, --aws-region
	workspace         string // -w, --workspace
	desiredstateRoot  string // -d, --desiredstate-root
	terraformSource   string // -s, --sourcecode-repo-workdir (alias: --terraform-source)
	configurationRoot string // -c, --configuration-root
	configRepoWorkdir string // --configuration-repo-workdir
	environment       string // -e, --environment
}

// addCommonFlags adds common flags to a command
func addCommonFlags(cmd *cobra.Command, flags *commonFlags, requireDesiredstate bool, requireRegion bool) {
	cmd.Flags().StringVarP(&flags.awsProfile, "aws-profile", "p", "", "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&flags.awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&flags.workspace, "workspace", "w", "default", "Terraform workspace")
	cmd.Flags().StringVarP(&flags.desiredstateRoot, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().StringVarP(&flags.terraformSource, "sourcecode-repo-workdir", "s", "", "Terraform/sourcecode repository working directory (if not provided, sourcecode repo is resolved and cloned from desiredstate)")
	cmd.Flags().StringVar(&flags.terraformSource, "terraform-source", "", "Alias of --sourcecode-repo-workdir")
	cmd.Flags().StringVarP(&flags.configurationRoot, "configuration-root", "c", "", "Terraform configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVar(&flags.configRepoWorkdir, "configuration-repo-workdir", "", "Configuration repository working directory (used when configuration is cloned from desiredstate)")
	environmentDefault := flags.environment
	if environmentDefault == "" {
		environmentDefault = "all"
	}
	cmd.Flags().StringVarP(&flags.environment, "environment", "e", environmentDefault, "Environment to deploy")

	if requireDesiredstate {
		cmd.MarkFlagRequired("desiredstate-root")
	}
	if requireRegion {
		cmd.MarkFlagRequired("aws-region")
	}
}

// newAssembleCommand creates the assemble subcommand.
func newAssembleCommand() *cobra.Command {
	flags := &commonFlags{environment: defaultAssembleEnvironment}

	cmd := &cobra.Command{
		Use:     "assemble",
		Aliases: []string{"A"},
		Short:   "Assemble configuration as defined in desired state",
		Long:    `Assemble Terraform configuration from GitOps desired state and configuration files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssemble(flags)
		},
	}

	addCommonFlags(cmd, flags, true, true)
	cmd.MarkFlagRequired("environment")

	return cmd
}

// newInitCommand creates the init subcommand.
func newInitCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		isReset            bool
		isGetProviders     bool
		isUpgradeModules   bool
		isUseLocalBackend  bool
		isMigrateStatefile bool
	)

	cmd := &cobra.Command{
		Use:     "init",
		Aliases: []string{"i"},
		Short:   "Initialise terraform work directory",
		Long:    `Initialize Terraform working directory with backend and plugins.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(flags, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isMigrateStatefile)
		},
	}

	addCommonFlags(cmd, flags, true, true)

	// Init-specific flags
	cmd.Flags().BoolVarP(&isReset, "reset", "R", false, "Clear all local state files for Terraform and tflib")
	cmd.Flags().BoolVarP(&isGetProviders, "get-providers", "g", false, "Get modules and plugins")
	cmd.Flags().BoolVarP(&isUpgradeModules, "upgrade-modules", "u", false, "Upgrade modules and plugins")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend configuration")
	cmd.Flags().BoolVarP(&isMigrateStatefile, "migrate-statefile", "M", false, "Migrate statefile to new S3 location")

	return cmd
}

// newPlanCommand creates the plan subcommand.
func newPlanCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		isPlanForDestroy  bool
		isInit            bool
		isReset           bool
		isGetProviders    bool
		isUpgradeModules  bool
		isUseLocalBackend bool
		isEstimateCosts   bool
		isNoLock          bool
	)

	cmd := &cobra.Command{
		Use:     "plan",
		Aliases: []string{"p"},
		Short:   "Compute plan for the Terraform root module",
		Long:    `Generate an execution plan showing what Terraform will do.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlan(flags, isPlanForDestroy, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isEstimateCosts, isNoLock)
		},
	}

	addCommonFlags(cmd, flags, true, true)

	// Plan-specific flags
	cmd.Flags().BoolVarP(&isPlanForDestroy, "plan-for-destroy", "D", false, "Plan for destroy")
	cmd.Flags().BoolVarP(&isInit, "init", "i", false, "Initialise backend and plugins")
	cmd.Flags().BoolVarP(&isReset, "reset", "R", false, "Clear all local state files for Terraform and tflib")
	cmd.Flags().BoolVarP(&isGetProviders, "get-providers", "g", false, "Get modules and plugins")
	cmd.Flags().BoolVarP(&isUpgradeModules, "upgrade-modules", "u", false, "Upgrade modules and plugins")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend configuration")
	cmd.Flags().BoolVarP(&isEstimateCosts, "estimate-costs", "C", false, "Estimate costs for planned resources (requires INFRACOST_API_KEY)")
	cmd.Flags().BoolVarP(&isNoLock, "no-lock", "N", false, "Do not lock statefile")

	return cmd
}

// newProvisionCommand creates the provision subcommand.
func newProvisionCommand() *cobra.Command {
	flags := &commonFlags{}
	var isDryRun bool

	cmd := &cobra.Command{
		Use:     "provision",
		Aliases: []string{"a", "apply"},
		Short:   "Execute Terraform provisioning",
		Long:    `Apply Terraform configuration to provision infrastructure.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProvision(flags, isDryRun)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Provision-specific flags
	cmd.Flags().BoolVarP(&isDryRun, "dry-run", "n", false, "Disables command effect on target instance (env: IS_DRY_RUN)")

	return cmd
}

// newDestroyCommand creates the destroy subcommand.
func newDestroyCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		isAutoApprove     bool
		isInit            bool
		isReset           bool
		isGetProviders    bool
		isUpgradeModules  bool
		isUsePlanFile     bool
		isUseLocalBackend bool
		isDryRun          bool
	)

	cmd := &cobra.Command{
		Use:     "destroy",
		Aliases: []string{"d"},
		Short:   "Destroy Terraform-managed infrastructure",
		Long:    `Destroy all resources managed by this Terraform configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDestroy(flags, isAutoApprove, isInit, isReset, isGetProviders, isUpgradeModules, isUsePlanFile, isUseLocalBackend, isDryRun)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Destroy-specific flags
	cmd.Flags().BoolVarP(&isAutoApprove, "auto-approve", "A", false, "Auto approve, do not ask for confirmation")
	cmd.Flags().BoolVarP(&isInit, "init", "i", false, "Initialise backend and plugins")
	cmd.Flags().BoolVarP(&isReset, "reset", "R", false, "Clear all local state files for Terraform and tflib")
	cmd.Flags().BoolVarP(&isGetProviders, "get-providers", "g", false, "Get modules and plugins")
	cmd.Flags().BoolVarP(&isUpgradeModules, "upgrade-modules", "u", false, "Upgrade modules and plugins")
	cmd.Flags().BoolVarP(&isUsePlanFile, "destroy-with-planfile", "P", false, "Destroy using generated plan file")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend configuration")
	cmd.Flags().BoolVarP(&isDryRun, "dry-run", "n", false, "Disables command effect on target instance")

	return cmd
}

// newOutputCommand creates the output subcommand.
func newOutputCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		outputFile        string
		isInit            bool
		isReset           bool
		isGetProviders    bool
		isUpgradeModules  bool
		isUseLocalBackend bool
	)

	cmd := &cobra.Command{
		Use:     "output",
		Aliases: []string{"o"},
		Short:   "Show Terraform output values",
		Long:    `Read and display output values from Terraform state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOutput(flags, outputFile, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend)
		},
	}

	addCommonFlags(cmd, flags, true, true)

	// Output-specific flags
	cmd.Flags().StringVarP(&outputFile, "output-file", "o", "", "File where to save Terraform output")
	cmd.Flags().BoolVarP(&isInit, "init", "i", false, "Initialise backend and plugins")
	cmd.Flags().BoolVarP(&isReset, "reset", "R", false, "Clear all local state files for Terraform and tflib")
	cmd.Flags().BoolVarP(&isGetProviders, "get-providers", "g", false, "Get modules and plugins")
	cmd.Flags().BoolVarP(&isUpgradeModules, "upgrade-modules", "u", false, "Upgrade modules and plugins")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend configuration")

	return cmd
}

// newGraphCommand creates the graph subcommand.
func newGraphCommand() *cobra.Command {
	flags := &commonFlags{}
	var graphType string
	var isInit bool
	var isReset bool
	var isGetModules bool
	var isUpgradeModules bool
	var isUseLocalBackend bool

	cmd := &cobra.Command{
		Use:     "graph",
		Aliases: []string{"g"},
		Short:   "Generate a Terraform dependency graph",
		Long:    `Generate a visual dependency graph of Terraform resources.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGraph(flags, graphType, isInit, isReset, isGetModules, isUpgradeModules, isUseLocalBackend)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Graph-specific flags
	cmd.Flags().StringVarP(&graphType, "graph-type", "T", "plan", "Type of graph to produce (plan, plan-refresh-only, plan-destroy, apply)")
	cmd.Flags().BoolVarP(&isInit, "init", "i", false, "Run terraform init before graph")
	cmd.Flags().BoolVarP(&isReset, "reset", "R", false, "Reset terraform state before running")
	cmd.Flags().BoolVarP(&isGetModules, "get", "g", false, "Download missing modules during init")
	cmd.Flags().BoolVarP(&isUpgradeModules, "upgrade", "u", false, "Upgrade modules to latest versions during init")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend instead of remote")

	return cmd
}

// newUnlockCommand creates the unlock subcommand.
func newUnlockCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		lockId          string
		isConfirmUnlock bool
	)

	cmd := &cobra.Command{
		Use:     "unlock",
		Aliases: []string{"u"},
		Short:   "Force unlock Terraform state",
		Long:    `Manually unlock the Terraform state if locking failed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlock(flags, lockId, isConfirmUnlock)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Unlock-specific flags
	cmd.Flags().StringVarP(&lockId, "lock-id", "L", "", "Statefile lock ID to force-unlock")
	cmd.Flags().BoolVarP(&isConfirmUnlock, "confirm-unlock", "Y", false, "Unlock statefile non-interactively")

	// Lock ID is required for unlock
	cmd.MarkFlagRequired("lock-id")

	return cmd
}

// newImportCommand creates the import subcommand.
func newImportCommand() *cobra.Command {
	flags := &commonFlags{}
	var (
		tfResourceId      string
		awsResourceId     string
		isUseLocalBackend bool
	)

	cmd := &cobra.Command{
		Use:     "import",
		Aliases: []string{"I"},
		Short:   "Import existing infrastructure into Terraform state",
		Long:    `Import existing resources into Terraform state management.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(flags, tfResourceId, awsResourceId, isUseLocalBackend)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Import-specific flags
	cmd.Flags().StringVarP(&tfResourceId, "tf-resource-id", "T", "", "Terraform resource ID, as identified by the resource hierarchy (see plan output)")
	cmd.Flags().StringVarP(&awsResourceId, "aws-resource-id", "A", "", "AWS resource ID to match Terraform resource ID (see Terraform documentation)")
	cmd.Flags().BoolVarP(&isUseLocalBackend, "local-backend", "L", false, "Use local backend instead of remote")

	// Resource identifiers are required for import
	cmd.MarkFlagRequired("tf-resource-id")
	cmd.MarkFlagRequired("aws-resource-id")
	cmd.MarkFlagRequired("aws-resource-id")

	return cmd
}

// newCostsCommand creates the costs subcommand.
func newCostsCommand() *cobra.Command {
	flags := &commonFlags{}
	var isUsePlanFile bool

	cmd := &cobra.Command{
		Use:     "costs",
		Aliases: []string{"c"},
		Short:   "Estimate infrastructure costs with infracost",
		Long:    `Estimate infrastructure costs using infracost tool (requires INFRACOST_API_KEY).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCosts(flags, isUsePlanFile)
		},
	}

	addCommonFlags(cmd, flags, false, true)

	// Costs-specific flags
	cmd.Flags().BoolVarP(&isUsePlanFile, "use-planfile", "P", true, "Use plan file for cost estimation")

	return cmd
}

// Implementation functions (stubs for now - these will call the service methods)

func runAssemble(flags *commonFlags) error {
	if err := validateAssembleEnvironment(flags.environment); err != nil {
		return err
	}
	if err := validateAssembleEnvironmentExistsInDesiredState(flags.desiredstateRoot, flags.environment); err != nil {
		return err
	}

	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Spaces()

	service := NewService(".", false)
	assembleResp, err := service.AssembleTerraform(TerraformAssembleRequest{
		DesiredStateFile:  flags.desiredstateRoot,
		ConfigFile:        flags.configurationRoot,
		ConfigRepoWorkdir: flags.configRepoWorkdir,
		Environment:       flags.environment,
		AWSProfile:        flags.awsProfile,
		AWSRegion:         flags.awsRegion,
		TerraformSource:   flags.terraformSource,
		OutputFormat:      "json",
	})
	if err != nil {
		return fmt.Errorf("failed to assemble terraform configuration: %w", err)
	}

	// Output results
	logging.Info("Assembled desiredstate: '%s'", assembleResp.DesiredStateFile)
	logging.Info("Assembled config:       '%s'", assembleResp.ConfigurationFile)
	logging.Info("Assembled backend file: '%s'", assembleResp.BackendFile)

	return nil
}

func prepareTerraformAssembledInputs(flags *commonFlags) (*TerraformAssembleResponse, error) {
	service := NewService(".", false)
	resp, err := service.AssembleTerraform(TerraformAssembleRequest{
		DesiredStateFile:  flags.desiredstateRoot,
		ConfigFile:        flags.configurationRoot,
		ConfigRepoWorkdir: flags.configRepoWorkdir,
		Environment:       flags.environment,
		AWSProfile:        flags.awsProfile,
		AWSRegion:         flags.awsRegion,
		TerraformSource:   flags.terraformSource,
		OutputFormat:      "json",
	})
	if err != nil {
		return nil, err
	}

	logging.Info("Assembled desiredstate: '%s'", resp.DesiredStateFile)
	logging.Info("Assembled config:       '%s'", resp.ConfigurationFile)
	logging.Info("Assembled backend file: '%s'", resp.BackendFile)
	logging.Info("Source code dir:        '%s'", resp.CodeDirectory)
	logging.Info("")

	return resp, nil
}

func validateAssembleEnvironment(environment string) error {
	env := strings.TrimSpace(environment)
	if env == "" || env == defaultAssembleEnvironment {
		return fmt.Errorf("--environment is mandatory and must be a valid environment value")
	}

	return nil
}

func validateAssembleEnvironmentExistsInDesiredState(desiredstateRoot, environment string) error {
	doc := core.NewGitOpsDocument()
	if err := doc.LoadGitOpsFile(desiredstateRoot, true, nil); err != nil {
		return fmt.Errorf("failed to validate environment from desiredstate: %w", err)
	}

	content := doc.GetContent().Data
	meta := doc.GetMeta().Data
	envs, ok := extractEnvironments(content)
	if !ok {
		envs, ok = extractEnvironments(meta)
	}
	if !ok {
		return fmt.Errorf("desiredstate does not define desiredstate.content.environments")
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
		return fmt.Errorf("environment '%s' is not defined: desiredstate.content.environments is empty", environment)
	}

	return fmt.Errorf("environment '%s' is not defined in desiredstate.content.environments (available: %s)",
		environment, strings.Join(available, ", "))
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

func runInit(flags *commonFlags, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isMigrateStatefile bool) error {
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Reset:          '%v'", isReset)
	logging.Info("Migrate state:  '%v'", isMigrateStatefile)
	logging.Spaces()

	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory
	backendFile := assembleResp.BackendFile
	logging.Spaces()

	// Check Terraform version
	service := NewService(codeDir, false)
	if err := service.CheckDependencies(); err != nil {
		return fmt.Errorf("terraform version check failed: %w", err)
	}
	logging.Spaces()

	// Reset state if requested (-R flag)
	if isReset {
		logging.Info("Clearing state...")
		if err := service.Reset(codeDir); err != nil {
			return fmt.Errorf("failed to reset terraform state: %w", err)
		}
	}

	// Run terraform init

	// Prepare backend config - only use if not local backend
	backendConfig := ""
	if !isUseLocalBackend {
		logging.Info("Backend:        '%s'", backendFile)
		backendConfig = backendFile
	} else {
		logging.Info("Using local backend (no remote state)")
	}

	initReq := InitRequest{
		WorkingDir:        codeDir,
		BackendConfigFile: backendConfig,
		Reconfigure:       true, // Always use -reconfigure to avoid interactive prompts
		Upgrade:           isUpgradeModules,
		MigrateState:      isMigrateStatefile,
		NoBackend:         isUseLocalBackend,
		Get:               true, // Always get modules by default (terraform init behavior)
		LockProviders:     false,
	}

	logging.Info("Terraform init...")
	logging.Spaces()

	_, err = service.Init(initReq)
	if err != nil {
		return fmt.Errorf("terraform init failed: %w", err)
	}

	return nil
}
func runPlan(flags *commonFlags, isPlanForDestroy, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isEstimateCosts, isNoLock bool) error {
	// Log all parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Reset:          '%v'", isReset)
	logging.Info("For destroy:    '%v'", isPlanForDestroy)
	logging.Info("Estimate costs: '%v'", isEstimateCosts)
	logging.Info("")

	if isGetProviders || isUpgradeModules {
		isInit = true
	}

	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory
	dsPath := assembleResp.DesiredStateFile
	cfgPath := assembleResp.ConfigurationFile

	logging.Info("")

	// Create service
	service := NewService(codeDir, false)

	// Check terraform version
	err = service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Info("")

	// For now, we'll use the flag directly
	if isInit || assembleResp.SourceCodeCloned {
		// Reset state if requested
		if isReset {
			err := service.Reset(codeDir)
			if err != nil {
				return fmt.Errorf("failed to reset terraform state: %w", err)
			}
		}

		// Run terraform init
		var backendConfig string
		if !isUseLocalBackend {
			backendConfig = assembleResp.BackendFile
		}

		initReq := InitRequest{
			WorkingDir:        codeDir,
			BackendConfigFile: backendConfig,
			Reconfigure:       true,
			NoBackend:         isUseLocalBackend,
			Get:               true, // Always download modules
			Upgrade:           isUpgradeModules,
		}

		_, err = service.Init(initReq)
		if err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}

		// Providers lock
		lockReq := ProvidersLockRequest{
			WorkingDir: codeDir,
		}
		_, err = service.ProvidersLock(lockReq)
		if err != nil {
			logging.Info("Failed to lock providers: %v", err)
		}

		// Set workspace
		wsReq := WorkspaceRequest{
			WorkingDir: codeDir,
			Name:       flags.workspace,
			Operation:  "select",
		}
		_, err = service.Workspace(wsReq)
		if err != nil {
			return fmt.Errorf("failed to set workspace: %w", err)
		}
		logging.Info("")
	}

	// Validate
	validateReq := ValidateRequest{
		WorkingDir: codeDir,
	}
	_, err = service.ValidateTerraform(validateReq)
	if err != nil {
		return fmt.Errorf("terraform validate failed: %w", err)
	}
	logging.Info("")

	// Run terraform plan
	planFile := "tfplan"
	if isPlanForDestroy {
		planFile = "tfplan-destroy"
	}

	planReq := PlanRequest{
		WorkingDir:       codeDir,
		DesiredStateVar:  dsPath,
		ConfigurationVar: cfgPath,
		AwsRegion:        flags.awsRegion,
		Environment:      flags.environment,
		OutFile:          planFile,
		Destroy:          isPlanForDestroy,
		NoLock:           isNoLock,
	}

	_, err = service.Plan(planReq)
	if err != nil {
		return fmt.Errorf("terraform plan failed: %w", err)
	}

	// Estimate costs if requested
	if isEstimateCosts {
		logging.Info("Cost estimation not yet implemented")
		// TODO: Implement cost estimation using infracost
	}

	logging.Info("Working directory: '%s'", codeDir)

	return nil
}

func runProvision(flags *commonFlags, isDryRun bool) error {
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("")

	// Create environment variables map
	envVars := map[string]string{
		"AWS_PROFILE": flags.awsProfile,
		"AWS_REGION":  flags.awsRegion,
	}

	// Create parser with environment variables
	parser := NewParser(flags.environment, envVars)
	parser.SetConfigurationWorkdir(flags.configRepoWorkdir)

	// Load GitOps files (desiredstate/configuration are optional for provision)
	err := parser.LoadGitOpsFilesExtended(
		flags.desiredstateRoot,
		flags.awsRegion,
		flags.configurationRoot,
		flags.terraformSource,
	)
	if err != nil {
		return fmt.Errorf("failed to load GitOps files: %w", err)
	}

	// Determine code directory
	codeDir := flags.terraformSource
	if parser.IsClonedSourceCode() {
		codeDir = parser.GetSourceCodeDir()
	}
	if codeDir == "" {
		return fmt.Errorf("terraform source directory not specified and could not be determined from desired state")
	}

	// Create service
	service := NewService(codeDir, false)

	// Check terraform version
	err = service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Info("")

	// Find plan file (default: tfplan)
	planFile := "tfplan"
	planPath := fmt.Sprintf("%s/%s", codeDir, planFile)
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		// Try .gitops/tfplan
		planPath = fmt.Sprintf("%s/.gitops/%s", codeDir, planFile)
		if _, err := os.Stat(planPath); os.IsNotExist(err) {
			return fmt.Errorf("plan file not found: %s", planPath)
		}
	}
	logging.Info("Using plan file: '%s'", planPath)

	// Run terraform apply with plan file
	applyReq := ApplyRequest{
		WorkingDir:  codeDir,
		PlanFile:    planPath,
		AutoApprove: true,
		NoLock:      false,
	}

	_, err = service.Apply(applyReq)
	if err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	logging.Info("Provisioning complete for directory: '%s'", codeDir)
	return nil
}

func runDestroy(flags *commonFlags, isAutoApprove, isInit, isReset, isGetProviders, isUpgradeModules, isUsePlanFile, isUseLocalBackend, isDryRun bool) error {
	// Log parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Destroy w/plan: '%v'", isUsePlanFile)
	logging.Info("")

	if isUsePlanFile {
		return runDestroyWithPlan(flags, isDryRun)
	}

	return runDestroyWithoutPlan(flags, isAutoApprove, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isDryRun)
}

// runDestroyWithPlan destroys infrastructure using a pre-generated destroy plan file
func runDestroyWithPlan(flags *commonFlags, isDryRun bool) error {
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Dry run:        '%v'", isDryRun)
	logging.Info("")

	// Determine code directory
	codeDir := flags.terraformSource
	if codeDir == "" {
		return fmt.Errorf("terraform source directory not specified")
	}

	// Create service
	service := NewService(codeDir, false)

	// Check terraform version
	err := service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Info("")

	// Find destroy plan file (default: tfplan-destroy)
	planFile := "tfplan-destroy"
	planPath := fmt.Sprintf("%s/%s", codeDir, planFile)
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		// Try .gitops/tfplan-destroy
		planPath = fmt.Sprintf("%s/.gitops/%s", codeDir, planFile)
		if _, err := os.Stat(planPath); os.IsNotExist(err) {
			return fmt.Errorf("destroy plan file not found: %s", planPath)
		}
	}
	logging.Info("Using destroy plan file: '%s'", planPath)

	// Run terraform apply with destroy plan file
	applyReq := ApplyRequest{
		WorkingDir:  codeDir,
		PlanFile:    planPath,
		AutoApprove: true, // Plan file doesn't need approval
		NoLock:      false,
	}

	_, err = service.Apply(applyReq)
	if err != nil {
		return fmt.Errorf("terraform apply (destroy) failed: %w", err)
	}

	logging.Info("Working directory: '%s'", codeDir)
	return nil
}

// runDestroyWithoutPlan destroys infrastructure directly without a plan file
func runDestroyWithoutPlan(flags *commonFlags, isAutoApprove, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend, isDryRun bool) error {
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Reset:          '%v'", isReset)
	logging.Info("Auto approve:   '%v'", isAutoApprove)
	logging.Info("Dry run:        '%v'", isDryRun)
	logging.Info("")

	if isGetProviders || isUpgradeModules {
		isInit = true
	}

	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory
	dsPath := assembleResp.DesiredStateFile
	cfgPath := assembleResp.ConfigurationFile

	logging.Info("")

	// Create service
	service := NewService(codeDir, false)

	// Check terraform version
	err = service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Info("")

	if isInit || assembleResp.SourceCodeCloned {
		// Reset if requested
		if isReset {
			err := service.Reset(codeDir)
			if err != nil {
				return fmt.Errorf("failed to reset terraform state: %w", err)
			}
		}

		// Run terraform init
		var backendConfig string
		if !isUseLocalBackend {
			backendConfig = assembleResp.BackendFile
		}

		initReq := InitRequest{
			WorkingDir:        codeDir,
			BackendConfigFile: backendConfig,
			Reconfigure:       true,
			NoBackend:         isUseLocalBackend,
			Get:               true,
			Upgrade:           isUpgradeModules,
		}

		_, err = service.Init(initReq)
		if err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}

		// Providers lock
		lockReq := ProvidersLockRequest{
			WorkingDir: codeDir,
		}
		_, err = service.ProvidersLock(lockReq)
		if err != nil {
			logging.Info("Failed to lock providers: %v", err)
		}

		// Set workspace
		wsReq := WorkspaceRequest{
			WorkingDir: codeDir,
			Name:       flags.workspace,
			Operation:  "select",
		}
		_, err = service.Workspace(wsReq)
		if err != nil {
			return fmt.Errorf("failed to set workspace: %w", err)
		}
		logging.Info("")
	}

	// Validate
	validateReq := ValidateRequest{
		WorkingDir: codeDir,
	}
	_, err = service.ValidateTerraform(validateReq)
	if err != nil {
		return fmt.Errorf("terraform validate failed: %w", err)
	}
	logging.Info("")

	// Run terraform destroy without plan file
	destroyReq := DestroyRequest{
		WorkingDir:       codeDir,
		DesiredStateVar:  dsPath,
		ConfigurationVar: cfgPath,
		AwsRegion:        flags.awsRegion,
		Environment:      flags.environment,
		AutoApprove:      isAutoApprove,
		NoLock:           false,
	}

	_, err = service.Destroy(destroyReq)
	if err != nil {
		return fmt.Errorf("terraform destroy failed: %w", err)
	}

	logging.Info("Working directory: '%s'", codeDir)
	return nil
}

func runOutput(flags *commonFlags, outputFile string, isInit, isReset, isGetProviders, isUpgradeModules, isUseLocalBackend bool) error {
	// Log all parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("Reset:          '%v'", isReset)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("TF output:      '%s'", outputFile)
	logging.Info("")

	if isGetProviders || isUpgradeModules {
		isInit = true
	}

	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory

	logging.Info("")

	// Create service
	service := NewService(codeDir, false)

	// Check terraform version
	err = service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Info("")

	if isInit || assembleResp.SourceCodeCloned {
		// Reset if requested
		if isReset {
			err := service.Reset(codeDir)
			if err != nil {
				return fmt.Errorf("failed to reset terraform state: %w", err)
			}
		}

		// Run terraform init
		var backendConfig string
		if !isUseLocalBackend {
			backendConfig = assembleResp.BackendFile
		}

		initReq := InitRequest{
			WorkingDir:        codeDir,
			BackendConfigFile: backendConfig,
			Reconfigure:       true,
			NoBackend:         isUseLocalBackend,
			Get:               true,
			Upgrade:           isUpgradeModules,
		}

		_, err = service.Init(initReq)
		if err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}

		// Providers lock
		lockReq := ProvidersLockRequest{
			WorkingDir: codeDir,
		}
		_, err = service.ProvidersLock(lockReq)
		if err != nil {
			logging.Info("Failed to lock providers: %v", err)
		}

		// Set workspace
		wsReq := WorkspaceRequest{
			WorkingDir: codeDir,
			Name:       flags.workspace,
			Operation:  "select",
		}
		_, err = service.Workspace(wsReq)
		if err != nil {
			return fmt.Errorf("failed to set workspace: %w", err)
		}
		logging.Info("")
	}

	// Run terraform output
	outputReq := OutputRequest{
		WorkingDir: codeDir,
		JSON:       true, // always request JSON output
	}

	resp, err := service.Output(outputReq)
	if err != nil {
		return fmt.Errorf("terraform output failed: %w", err)
	}

	// Display the output
	fmt.Println(resp.Output)

	// Save to file if requested
	if outputFile != "" {
		err = os.WriteFile(outputFile, []byte(resp.Output), 0644)
		if err != nil {
			return fmt.Errorf("failed to write output to file %s: %w", outputFile, err)
		}
		logging.Info("Output saved to: '%s'", outputFile)
	}

	logging.Info("Working directory: '%s'", codeDir)
	return nil
}

func runGraph(flags *commonFlags, graphType string, isInit, isReset, isGetModules, isUpgradeModules, isUseLocalBackend bool) error {
	// Validate graph type
	validTypes := map[string]bool{
		"plan":              true,
		"plan-refresh-only": true,
		"plan-destroy":      true,
		"apply":             true,
	}
	if !validTypes[graphType] {
		return fmt.Errorf("invalid graph type '%s'. Valid types: plan, plan-refresh-only, plan-destroy, apply", graphType)
	}

	// Log parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("TF workspace:   '%s'", flags.workspace)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Reset:          '%v'", isReset)
	logging.Info("Graph type:     '%s'", graphType)
	logging.Spaces()

	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory
	backendFile := assembleResp.BackendFile

	logging.Spaces()

	// 3. Create terraform service
	service := NewService(codeDir, false)

	// 4. Check terraform version
	err = service.CheckDependencies()
	if err != nil {
		return fmt.Errorf("terraform dependency check failed: %w", err)
	}
	logging.Spaces()

	// 5. Determine if we should run init
	shouldInit := isInit || assembleResp.SourceCodeCloned || isGetModules || isUpgradeModules

	// 6. Optionally reset terraform state
	if isReset {
		err := service.Reset(codeDir)
		if err != nil {
			return fmt.Errorf("failed to reset terraform state: %w", err)
		}
	}

	// 7. Optionally run terraform init
	if shouldInit {
		// Prepare backend config
		var backendConfig string
		if !isUseLocalBackend {
			backendConfig = backendFile
		}

		initReq := InitRequest{
			WorkingDir:        codeDir,
			BackendConfigFile: backendConfig,
			Reconfigure:       true,
			NoBackend:         isUseLocalBackend,
			Get:               true, // Always download modules
			Upgrade:           isUpgradeModules,
		}

		_, err = service.Init(initReq)
		if err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}

		// 8. Lock providers
		lockReq := ProvidersLockRequest{
			WorkingDir: codeDir,
		}
		_, err = service.ProvidersLock(lockReq)
		if err != nil {
			logging.Info("Failed to lock providers: %v", err)
		}

		// 9. Select workspace
		wsReq := WorkspaceRequest{
			WorkingDir: codeDir,
			Name:       flags.workspace,
			Operation:  "select",
		}
		_, err = service.Workspace(wsReq)
		if err != nil {
			return fmt.Errorf("failed to set workspace: %w", err)
		}
		logging.Spaces()
	}

	// 10. Validate terraform configuration
	validateReq := ValidateRequest{
		WorkingDir: codeDir,
	}
	_, err = service.ValidateTerraform(validateReq)
	if err != nil {
		return fmt.Errorf("terraform validate failed: %w", err)
	}
	logging.Spaces()

	// 11. Run terraform graph command and pipe to dot
	logging.Info("Terraform graph...")

	// Default output file
	svgFile := filepath.Join(codeDir, "gitops.tf-provision.svg")

	// Build command: terraform graph -draw-cycles -type=<type> | dot -Tsvg -o <file>
	graphArgs := []string{
		"graph",
		"-draw-cycles",
		fmt.Sprintf("-type=%s", graphType),
	}

	// Check dry-run mode
	isDryRun := os.Getenv("IS_DRY_RUN") == "1"

	if isDryRun {
		logging.Info("[Dry-Run] Would execute: terraform %s | dot -Tsvg -o '%s'", strings.Join(graphArgs, " "), svgFile)
		logging.Info("[Dry-Run] Working directory: %s", codeDir)
	} else {
		// Run terraform graph
		tfCmd := exec.Command("terraform", graphArgs...)
		tfCmd.Dir = codeDir
		tfCmd.Env = append(os.Environ(),
			fmt.Sprintf("AWS_PROFILE=%s", flags.awsProfile),
			fmt.Sprintf("AWS_REGION=%s", flags.awsRegion),
		)

		// Run dot command
		dotCmd := exec.Command("dot", "-Tsvg", "-o", svgFile)
		dotCmd.Dir = codeDir

		// Pipe terraform output to dot
		var stderr bytes.Buffer
		dotCmd.Stderr = &stderr

		pipe, err := tfCmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to create pipe: %w", err)
		}
		dotCmd.Stdin = pipe

		// Start both commands
		if err := dotCmd.Start(); err != nil {
			return fmt.Errorf("failed to start dot command: %w", err)
		}

		if err := tfCmd.Run(); err != nil {
			return fmt.Errorf("terraform graph failed: %w", err)
		}

		if err := dotCmd.Wait(); err != nil {
			return fmt.Errorf("dot command failed: %w (stderr: %s)", err, stderr.String())
		}

		logging.Info("Graph saved to: '%s'", svgFile)
	}

	logging.Info("Working directory: '%s'", codeDir)

	return nil
}

func runUnlock(flags *commonFlags, lockId string, isConfirmUnlock bool) error {
	logging.Info("Running terraform unlock...")

	// Validate required parameters
	if lockId == "" {
		return fmt.Errorf("lock ID is required (use -L flag)")
	}

	// Validate terraform source directory
	if flags.terraformSource == "" {
		return fmt.Errorf("terraform source directory is required (use -s flag)")
	}

	// Log parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Lock ID:        '%s'", lockId)
	logging.Info("Confirm unlock: '%v'", isConfirmUnlock)
	logging.Spaces()

	codeDir := flags.terraformSource

	// Create terraform service
	service := NewService(codeDir, false)

	// Run terraform unlock
	logging.Info("Terraform unlock...")

	unlockReq := UnlockRequest{
		WorkingDir: codeDir,
		LockID:     lockId,
		Force:      isConfirmUnlock,
	}

	_, err := service.Unlock(unlockReq)
	if err != nil {
		return fmt.Errorf("terraform unlock failed: %w", err)
	}

	logging.Info("Working directory: '%s'", codeDir)
	logging.Spaces()

	return nil
}

func runImport(flags *commonFlags, tfResourceId, awsResourceId string, isUseLocalBackend bool) error {
	logging.Info("Running terraform import...")

	// Validate required parameters
	if tfResourceId == "" {
		return fmt.Errorf("terraform resource ID is required (use -T flag)")
	}
	if awsResourceId == "" {
		return fmt.Errorf("AWS resource ID is required (use -A flag)")
	}

	// Log parameters
	logging.Info("AWS profile:     '%s'", flags.awsProfile)
	logging.Info("AWS region:      '%s'", flags.awsRegion)
	logging.Info("Environment:     '%s'", flags.environment)
	logging.Info("TF workspace:    '%s'", flags.workspace)
	logging.Info("DesiredState:    '%s'", flags.desiredstateRoot)
	logging.Info("Config:          '%s'", flags.configurationRoot)
	logging.Info("TF code:         '%s'", flags.terraformSource)
	logging.Info("TF resource ID:  '%s'", tfResourceId)
	logging.Info("AWS resource ID: '%s'", awsResourceId)
	logging.Spaces()

	// argTerraformResourceId = argTerraformResourceId.replace("|", "\\|")
	tfResourceId = strings.ReplaceAll(tfResourceId, "|", "\\|")

	logging.Info("Loading GitOps files...")
	assembleResp, err := prepareTerraformAssembledInputs(flags)
	if err != nil {
		return fmt.Errorf("failed to assemble terraform inputs: %w", err)
	}

	codeDir := assembleResp.CodeDirectory
	dsPath := assembleResp.DesiredStateFile
	cfgPath := assembleResp.ConfigurationFile
	backendFile := assembleResp.BackendFile

	logging.Spaces()

	// 3. Create terraform service
	service := NewService(codeDir, false)

	// 4. Check dependencies (terraform version)
	logging.Info("Checking terraform version...")
	if err := service.CheckDependencies(); err != nil {
		return fmt.Errorf("terraform version check failed: %w", err)
	}
	logging.Spaces()

	// 5. If source code was cloned, run init
	//           tflib.isInit = True
	//           tflib.isGetModules = True
	//           tflib.init(argIsUseLocalBackend)
	//           tflib.providersLock()
	if assembleResp.SourceCodeCloned {
		logging.Info("Source code was cloned, running terraform init...")

		initReq := InitRequest{
			WorkingDir:        codeDir,
			BackendConfigFile: backendFile,
			Upgrade:           false,
			MigrateState:      false,
			Get:               true, // Download modules
			NoBackend:         isUseLocalBackend,
		}

		_, err = service.Init(initReq)
		if err != nil {
			return fmt.Errorf("terraform init failed: %w", err)
		}
		logging.Spaces()

		// Lock providers
		logging.Info("Locking provider versions...")
		providersReq := ProvidersLockRequest{
			WorkingDir: codeDir,
			Platforms:  []string{"windows_amd64", "darwin_amd64", "linux_amd64"},
		}
		_, err = service.ProvidersLock(providersReq)
		if err != nil {
			return fmt.Errorf("failed to lock providers: %w", err)
		}
		logging.Spaces()
	}

	// 6. Select/create workspace
	logging.Info("Selecting terraform workspace '%s'...", flags.workspace)
	workspaceReq := WorkspaceRequest{
		WorkingDir: codeDir,
		Name:       flags.workspace,
		Operation:  "select", // Select or create workspace
	}
	_, err = service.Workspace(workspaceReq)
	if err != nil {
		return fmt.Errorf("failed to select workspace: %w", err)
	}
	logging.Spaces()

	// 7. Run terraform import
	// terraform import -var-file='<desiredstate>' -var-file='<config>' -var='aws_provider_region=<region>' '<tf-resource-id>' '<aws-resource-id>'
	logging.Info("Importing resource...")

	importReq := ImportRequest{
		WorkingDir:       codeDir,
		ResourceAddress:  tfResourceId,
		ResourceID:       awsResourceId,
		DesiredStateFile: dsPath,
		ConfigFile:       cfgPath,
		AwsRegion:        flags.awsRegion,
		Environment:      flags.environment,
	}

	_, err = service.Import(importReq)
	if err != nil {
		return fmt.Errorf("terraform import failed: %w", err)
	}

	logging.Info("Working directory: '%s'", codeDir)
	logging.Spaces()

	return nil
}

func runCosts(flags *commonFlags, isUsePlanFile bool) error {
	logging.Info("Running terraform costs...")

	// Log parameters
	logging.Info("AWS profile:    '%s'", flags.awsProfile)
	logging.Info("AWS region:     '%s'", flags.awsRegion)
	logging.Info("Environment:    '%s'", flags.environment)
	logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
	logging.Info("Config:         '%s'", flags.configurationRoot)
	logging.Info("TF code:        '%s'", flags.terraformSource)
	logging.Info("Plan file:      '%v'", isUsePlanFile)
	logging.Spaces()

	// Determine code directory
	codeDir := flags.terraformSource
	if codeDir == "" {
		return fmt.Errorf("terraform source directory is required (use -s flag)")
	}

	// Default plan file name
	planFile := filepath.Join(codeDir, "gitops.tf-provision.tfplan")

	var dsPath, cfgPath string

	// If not using plan file, load and assemble GitOps files
	if !isUsePlanFile {
		logging.Info("Loading GitOps files...")
		assembleResp, err := prepareTerraformAssembledInputs(flags)
		if err != nil {
			return fmt.Errorf("failed to assemble terraform inputs: %w", err)
		}

		codeDir = assembleResp.CodeDirectory
		dsPath = assembleResp.DesiredStateFile
		cfgPath = assembleResp.ConfigurationFile
	}

	logging.Info("Source code dir:        '%s'", codeDir)
	logging.Spaces()

	// 3. Create terraform service
	service := NewService(codeDir, false)

	// 4. Run infracost
	logging.Info("Estimating costs...")

	costsReq := CostsRequest{
		WorkingDir:       codeDir,
		PlanFile:         planFile,
		UsePlanFile:      isUsePlanFile,
		DesiredStateFile: dsPath,
		ConfigFile:       cfgPath,
		AwsRegion:        flags.awsRegion,
	}

	resp, err := service.Costs(costsReq)
	if err != nil {
		return fmt.Errorf("infracost failed: %w", err)
	}

	// Print the output
	if resp.Output != "" {
		fmt.Println(resp.Output)
	}

	logging.Info("Working directory: '%s'", codeDir)
	logging.Spaces()

	return nil
}
