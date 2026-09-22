package concourse

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/spf13/cobra"
)

const defaultInputEnvironment = "NON_EXISTING_ENVIRONMENT"

// NewConcourseCommand creates the cci command group.
func NewConcourseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cci",
		Aliases: []string{"concourse"},
		Short:   "Wrap Fly (Concourse) commands",
		Long:    `Concourse CI wrapper commands for GitOps pipeline operations.`,
	}

	cmd.AddCommand(newAssembleCommand())
	cmd.AddCommand(newSetPipelinesCommand())

	return cmd
}

// assembleFlags holds all flags for the assemble subcommand.
type assembleFlags struct {
	desiredstateRoot  string // -d, --desiredstate-root
	configurationRoot string // -c, --configuration-root
	configRepoWorkdir string // --configuration-repo-workdir
	environment       string // -e, --environment
	pipelinesWorkdir  string // -p, --pipelines-repo-workdir (alias: --pipelines-workdir)
	save              string // -s, --save (build output directory)
	isMaster          bool   // -M, --master-pipeline
	isSlave           bool   // -S, --slave-pipelines
	isLocal           bool   // -L, --local-pipelines
}

// setPipelinesFlags holds flags for setpipelines.
type setPipelinesFlags struct {
	desiredstateRoot  string
	configurationRoot string
	configRepoWorkdir string
	environment       string
	pipelinesWorkdir  string
	save              string
	isMaster          bool
	isSlave           bool
	isLocal           bool

	targetName     string
	concourseURL   string
	flyTeam        string
	flyUsername    string
	flyPasswordB64 string
	isForce        bool
}

// newAssembleCommand creates the assemble subcommand.
// This is the concourse equivalent of "yago tf assemble": assembles GitOps files to disk
// without invoking fly. It generates per-pipeline configuration.yaml and pipeline.yaml
// in the build directory.
func newAssembleCommand() *cobra.Command {
	flags := &assembleFlags{}

	cmd := &cobra.Command{
		Use:     "assemble",
		Aliases: []string{"A"},
		Short:   "Assemble Concourse pipeline configuration files from GitOps desired state",
		Long: `Assemble Concourse pipeline configuration files from GitOps desired state and configuration.

For each pipeline instance defined in concourse.pipelines.instances, writes:
  <build-dir>/<pipeline-name>/configuration.yaml  — merged desiredstate + global config + instance config
  <build-dir>/<pipeline-name>/pipeline.yaml       — pipeline template

Exactly one of --master-pipeline, --slave-pipelines, or --local-pipelines must be specified.

Environment variables:
  LOG_LEVEL={DEBUG,INFO,WARNING,ERROR,CRITICAL}  Controls output verbosity (default: INFO)
  IS_DRY_RUN={0,1}                               Reserved for future use (default: 0)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssemble(flags)
		},
	}

	// CLI flags for the assemble command
	cmd.Flags().StringVarP(&flags.desiredstateRoot, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().StringVarP(&flags.configurationRoot, "configuration-root", "c", "", "Concourse configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVar(&flags.configRepoWorkdir, "configuration-repo-workdir", "", "Configuration repository working directory (used when configuration is cloned from desiredstate)")
	cmd.Flags().StringVarP(&flags.environment, "environment", "e", defaultInputEnvironment, "Environment to deploy")
	cmd.Flags().StringVarP(&flags.pipelinesWorkdir, "pipelines-repo-workdir", "p", "", "Pipelines repository working directory (if omitted, repo is resolved and cloned from desiredstate locator)")
	cmd.Flags().StringVar(&flags.pipelinesWorkdir, "pipelines-workdir", "", "Alias of --pipelines-repo-workdir")
	cmd.Flags().StringVarP(&flags.save, "save", "s", "", "Build output directory (default: <pipelines-workdir>/.gitops or <cwd>/.gitops)")
	cmd.Flags().BoolVarP(&flags.isMaster, "master-pipeline", "M", false, "Set master pipeline mode")
	cmd.Flags().BoolVarP(&flags.isSlave, "slave-pipelines", "S", false, "Set slave pipelines mode")
	cmd.Flags().BoolVarP(&flags.isLocal, "local-pipelines", "L", false, "Set local pipelines mode")

	cmd.MarkFlagRequired("desiredstate-root")
	cmd.MarkFlagRequired("environment")

	return cmd
}

// newSetPipelinesCommand creates the setpipelines subcommand.
// It mirrors gitops cci setpipelines plumbing while running fly lifecycle steps
// in dry-run mode for now.
func newSetPipelinesCommand() *cobra.Command {
	flags := &setPipelinesFlags{}

	cmd := &cobra.Command{
		Use:     "setpipelines",
		Aliases: []string{"p"},
		Short:   "Create or update pipelines (dry-run fly lifecycle for now)",
		Long: `Create or update Concourse pipelines.

This command performs the full setpipelines plumbing and lifecycle plan:
  1. sync fly binary
  2. login to target
  3. update pipelines (validate + apply)
  4. logout target

For this iteration, fly execution is dry-run only; no fly command is executed.
Pipeline configuration/template generation reuses the assemble orchestration.

Environment variables:
  LOG_LEVEL={DEBUG,INFO,WARNING,ERROR,CRITICAL}  Controls output verbosity (default: INFO)
  IS_DRY_RUN={0,1}                               Pretend running commands (default: 0)
  IS_MANUAL_RUN={0,1}                            Skip downloading fly from Concourse instance; use existing fly binary (default: 0)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetPipelines(flags)
		},
	}

	// Assemble-compatible inputs
	cmd.Flags().StringVarP(&flags.desiredstateRoot, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().StringVarP(&flags.configurationRoot, "configuration-root", "c", "", "Concourse configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVar(&flags.configRepoWorkdir, "configuration-repo-workdir", "", "Configuration repository working directory (used when configuration is cloned from desiredstate)")
	cmd.Flags().StringVarP(&flags.environment, "environment", "e", defaultInputEnvironment, "Environment to deploy")
	cmd.Flags().StringVarP(&flags.pipelinesWorkdir, "pipelines-repo-workdir", "p", "", "Pipelines repository working directory (if omitted, repo is resolved and cloned from desiredstate locator)")
	cmd.Flags().StringVar(&flags.pipelinesWorkdir, "pipelines-workdir", "", "Alias of --pipelines-repo-workdir")
	cmd.Flags().StringVarP(&flags.save, "save", "s", "", "Build output directory (default: <pipelines-workdir>/.gitops or <cwd>/.gitops)")
	cmd.Flags().BoolVarP(&flags.isMaster, "master-pipeline", "M", false, "Set master pipeline mode")
	cmd.Flags().BoolVarP(&flags.isSlave, "slave-pipelines", "S", false, "Set slave pipelines mode")
	cmd.Flags().BoolVarP(&flags.isLocal, "local-pipelines", "L", false, "Set local pipelines mode")

	// Fly plumbing inputs (parity)
	cmd.Flags().StringVarP(&flags.targetName, "target-name", "n", "", "Concourse target name")
	cmd.Flags().StringVarP(&flags.concourseURL, "concourse-url", "u", "", "Concourse URL")
	cmd.Flags().StringVarP(&flags.flyTeam, "fly-team", "T", "", "Fly team")
	cmd.Flags().StringVarP(&flags.flyUsername, "fly-username", "U", "", "Fly username (optional; omit to use pre-configured target or SSO)")
	cmd.Flags().StringVarP(&flags.flyPasswordB64, "fly-password-b64", "P", "", "Fly password, base64 encoded (required when --fly-username is set)")
	cmd.Flags().BoolVarP(&flags.isForce, "force", "F", false, "Force updating target pipelines regardless of self-updating configuration")

	cmd.MarkFlagRequired("desiredstate-root")
	cmd.MarkFlagRequired("environment")
	cmd.MarkFlagRequired("target-name")
	cmd.MarkFlagRequired("concourse-url")
	cmd.MarkFlagRequired("fly-team")

	return cmd
}

// runAssemble implements the assemble command logic.
func runAssemble(flags *assembleFlags) error {
	logging.Info("Environment:      '%s'", flags.environment)
	logging.Info("DesiredState:     '%s'", flags.desiredstateRoot)
	logging.Info("Config:           '%s'", flags.configurationRoot)
	logging.Info("Config workdir:   '%s'", flags.configRepoWorkdir)
	logging.Info("Pipelines:        '%s'", flags.pipelinesWorkdir)
	logging.Info("Master pipeline:  '%v'", flags.isMaster)
	logging.Info("Slave pipelines:  '%v'", flags.isSlave)
	logging.Info("Local pipelines:  '%v'", flags.isLocal)
	logging.Spaces()

	// Validate: exactly one pipeline mode must be selected
	modeCount := 0
	if flags.isMaster {
		modeCount++
	}
	if flags.isSlave {
		modeCount++
	}
	if flags.isLocal {
		modeCount++
	}
	if modeCount != 1 {
		return fmt.Errorf("exactly one of --master-pipeline, --slave-pipelines, or --local-pipelines must be specified (got %d)", modeCount)
	}

	// Validate desiredstate file exists
	if _, err := os.Stat(flags.desiredstateRoot); os.IsNotExist(err) {
		return fmt.Errorf("desiredstate file '%s' is not readable", flags.desiredstateRoot)
	}

	if err := validateEnvironment(flags.environment); err != nil {
		return err
	}
	if err := validateEnvironmentExistsInDesiredState(flags.desiredstateRoot, flags.environment); err != nil {
		return err
	}

	service := NewService(".", false)
	assembleResp, err := service.AssembleConcourse(ConcourseAssembleRequest{
		DesiredStateRoot:  flags.desiredstateRoot,
		ConfigurationRoot: flags.configurationRoot,
		ConfigRepoWorkdir: flags.configRepoWorkdir,
		Environment:       flags.environment,
		PipelinesWorkdir:  flags.pipelinesWorkdir,
		Save:              flags.save,
		IsMaster:          flags.isMaster,
		IsSlave:           flags.isSlave,
		IsLocal:           flags.isLocal,
	})
	if err != nil {
		return fmt.Errorf("failed to assemble concourse pipelines: %w", err)
	}

	for _, artifact := range assembleResp.Pipelines {
		logging.Info("Pipeline '%s':", artifact.PipelineName)
		logging.Info("  Config:   %s", artifact.ConfigurationFile)
		logging.Info("  Template: %s", artifact.TemplateFile)
	}

	logging.Spaces()
	logging.Info("Assembled %d pipeline(s) to: %s", assembleResp.TotalPipelines, assembleResp.BuildDirectory)
	return nil
}

func runSetPipelines(flags *setPipelinesFlags) error {
	isManualRun := os.Getenv("IS_MANUAL_RUN") == "1"

	logging.Info("Environment:      '%s'", flags.environment)
	logging.Info("Concourse URL:    '%s'", flags.concourseURL)
	logging.Info("Fly team:         '%s'", flags.flyTeam)
	logging.Info("Fly username:     '%s'", flags.flyUsername)
	logging.Info("DesiredState:     '%s'", flags.desiredstateRoot)
	logging.Info("Config:           '%s'", flags.configurationRoot)
	logging.Info("Pipelines:        '%s'", flags.pipelinesWorkdir)
	logging.Info("Master pipeline:  '%v'", flags.isMaster)
	logging.Info("Slave pipelines:  '%v'", flags.isSlave)
	logging.Info("Local pipelines:  '%v'", flags.isLocal)
	logging.Info("Force:            '%v'", flags.isForce)
	logging.Info("Dry run:          'true'")
	logging.Info("Manual run:       '%v'", isManualRun)
	logging.Spaces()

	if flags.flyUsername == "" {
		logging.Warn("Empty Concourse username. Relying on pre-configured concourse target (e.g. with SSO)")
	}
	if flags.flyUsername != "" && flags.flyPasswordB64 == "" {
		return fmt.Errorf("non valid Concourse password")
	}

	if err := validateEnvironment(flags.environment); err != nil {
		return err
	}
	if err := validateEnvironmentExistsInDesiredState(flags.desiredstateRoot, flags.environment); err != nil {
		return err
	}

	service := NewService(".", false)
	resp, err := service.SetPipelines(SetPipelinesRequest{
		DesiredStateRoot:  flags.desiredstateRoot,
		ConfigurationRoot: flags.configurationRoot,
		ConfigRepoWorkdir: flags.configRepoWorkdir,
		Environment:       flags.environment,
		PipelinesWorkdir:  flags.pipelinesWorkdir,
		Save:              flags.save,
		IsMaster:          flags.isMaster,
		IsSlave:           flags.isSlave,
		IsLocal:           flags.isLocal,
		TargetName:        flags.targetName,
		ConcourseURL:      flags.concourseURL,
		FlyTeam:           flags.flyTeam,
		FlyUsername:       flags.flyUsername,
		FlyPasswordB64:    flags.flyPasswordB64,
		IsForce:           flags.isForce,
		IsDryRun:          true,
		IsManualRun:       isManualRun,
	})
	if err != nil {
		return fmt.Errorf("failed to set pipelines: %w", err)
	}

	for _, artifact := range resp.AssembleResult.Pipelines {
		logging.Info("Pipeline '%s':", artifact.PipelineName)
		logging.Info("  Config:   %s", artifact.ConfigurationFile)
		logging.Info("  Template: %s", artifact.TemplateFile)
	}

	logging.Spaces()
	logging.Info("Dry-run fly lifecycle plan:")
	for _, op := range resp.Operations {
		if op.PipelineName == "" {
			logging.Info("  [%s] %s", op.Stage, op.Preview)
			continue
		}
		logging.Info("  [%s/%s] %s", op.Stage, op.PipelineName, op.Preview)
	}

	logging.Spaces()
	logging.Info("Setpipelines dry-run prepared %d pipeline(s) in: %s", resp.AssembleResult.TotalPipelines, resp.AssembleResult.BuildDirectory)

	return nil
}

// validateEnvironment enforces explicit, usable environment input.
func validateEnvironment(environment string) error {
	env := strings.TrimSpace(environment)
	if env == "" || env == defaultInputEnvironment {
		return fmt.Errorf("--environment is mandatory and must be a valid environment value")
	}

	return nil
}

func validateEnvironmentExistsInDesiredState(desiredstateRoot, environment string) error {
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

	// Most common shape: desiredstate.content.environments
	if desiredstate, ok := toStringMap(data["desiredstate"]); ok {
		if dsContent, ok := toStringMap(desiredstate["content"]); ok {
			if envs, ok := toStringMap(dsContent["environments"]); ok {
				return envs, true
			}
		}

		// Schema 2.x shape: desiredstate.configuration.<env>
		if envs, ok := toStringMap(desiredstate["configuration"]); ok {
			return envs, true
		}

		// Fallback shape: desiredstate.environments
		if envs, ok := toStringMap(desiredstate["environments"]); ok {
			return envs, true
		}
	}

	// Fallback shapes when content is already flattened
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
