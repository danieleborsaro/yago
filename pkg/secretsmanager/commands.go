package secretsmanager

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

const logLevelHelp = `
LOG_LEVEL={DEBUG,INFO,WARNING,ERROR,FATAL}  Controls output verbosity, default is INFO`

type commonFlags struct {
	awsProfile        string
	awsRegion         string
	desiredstateRoot  string
	configurationRoot string
	environment       string
}

// NewSecretManagerCommand creates the root secretsmanager command and its subcommands.
func NewSecretManagerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sm",
		Aliases: []string{"secretsmanager"},
		Short:   "Wrap AWS SecretsManager commands",
	}

	cmd.AddCommand(newAssembleCommand())
	cmd.AddCommand(newPlanCommand())
	cmd.AddCommand(newCreateCommand())
	cmd.AddCommand(newValidateCommand())
	cmd.AddCommand(newDestroyCommand())

	return cmd
}

func addCommonFlags(cmd *cobra.Command, flags *commonFlags) {
	cmd.Flags().StringVarP(&flags.awsProfile, "aws-profile", "p", os.Getenv("AWS_PROFILE"), "AWS profile as configured in the AWS CLI auth helper")
	cmd.Flags().StringVarP(&flags.awsRegion, "aws-region", "r", "", "AWS target region")
	cmd.Flags().StringVarP(&flags.desiredstateRoot, "desiredstate-root", "d", "", "DesiredState file")
	cmd.Flags().StringVarP(&flags.configurationRoot, "configuration-root", "c", "", "Sm configuration file, if not provided it will be cloned as per desiredstate")
	cmd.Flags().StringVarP(&flags.environment, "environment", "e", "all", "Environment to deploy")

	_ = cmd.MarkFlagRequired("aws-region")
	_ = cmd.MarkFlagRequired("desiredstate-root")
}

func checkCommonFlags(flags *commonFlags) error {
	if flags.awsRegion == "" {
		return errors.NewParamError("AWS Region is not specified")
	}
	if info, err := os.Stat(flags.desiredstateRoot); err != nil || info.IsDir() {
		return errors.NewParamError(fmt.Sprintf("DesiredState file '%s' is not readable", flags.desiredstateRoot))
	}
	if flags.configurationRoot != "" {
		if info, err := os.Stat(flags.configurationRoot); err != nil || info.IsDir() {
			return errors.NewParamError(fmt.Sprintf("Config file '%s' is not readable", flags.configurationRoot))
		}
	}
	return nil
}

func logCommonFlags(flags *commonFlags) {
	logging.Info("AWS profile:                   '%s'", flags.awsProfile)
	logging.Info("AWS region:                    '%s'", flags.awsRegion)
	logging.Info("Environment:                   '%s'", flags.environment)
	logging.Info("DesiredState:                  '%s'", flags.desiredstateRoot)
	logging.Info("Config:                        '%s'", flags.configurationRoot)
}

func newLib(flags *commonFlags, isDryRun bool) (*Lib, error) {
	lib, err := NewLib(flags.awsProfile, flags.awsRegion, isDryRun)
	if err != nil {
		return nil, err
	}
	if err := lib.LoadGitOpsFiles(flags.desiredstateRoot, flags.configurationRoot, flags.environment); err != nil {
		return nil, err
	}
	return lib, nil
}

func newAssembleCommand() *cobra.Command {
	flags := &commonFlags{}
	var cacheDir string

	cmd := &cobra.Command{
		Use:     "assemble",
		Aliases: []string{"a"},
		Short:   "Assemble configuration as defined in desired state.",
		Long:    "Assemble configuration as defined in desired state.\n" + logLevelHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCommonFlags(flags); err != nil {
				return err
			}
			if info, err := os.Stat(cacheDir); err != nil || !info.IsDir() {
				return errors.NewParamError(fmt.Sprintf("Cache directory '%s' does not exist or is not a directory", cacheDir))
			}

			logging.Info("AWS profile:    '%s'", flags.awsProfile)
			logging.Info("AWS region:     '%s'", flags.awsRegion)
			logging.Info("Environment:    '%s'", flags.environment)
			logging.Info("DesiredState:   '%s'", flags.desiredstateRoot)
			logging.Info("Config:         '%s'", flags.configurationRoot)
			logging.Info("Cache dir:      '%s'", cacheDir)
			logging.Spaces()

			response, err := NewService(".", false).AssembleSecrets(flags.desiredstateRoot, flags.configurationRoot, flags.environment, cacheDir)
			if err != nil {
				return err
			}

			logging.Info("Assembled desiredstate: '%s'", response.DesiredStateFile)
			logging.Info("Assembled config:       '%s'", response.ConfigurationFile)
			return nil
		},
	}

	addCommonFlags(cmd, flags)
	cmd.Flags().StringVarP(&cacheDir, "cache-dir", "C", "", "Cache assembled configuration to this directory")
	_ = cmd.MarkFlagRequired("cache-dir")

	return cmd
}

func newPlanCommand() *cobra.Command {
	flags := &commonFlags{}

	cmd := &cobra.Command{
		Use:     "plan",
		Aliases: []string{"p"},
		Short:   "Plan for secret creation.",
		Long:    "Plan for secret creation.\n" + logLevelHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCommonFlags(flags); err != nil {
				return err
			}

			logCommonFlags(flags)
			logging.Spaces()

			lib, err := newLib(flags, true)
			if err != nil {
				return err
			}
			return lib.Plan()
		},
	}

	addCommonFlags(cmd, flags)

	return cmd
}

func newCreateCommand() *cobra.Command {
	flags := &commonFlags{}
	var isDryRun bool

	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"c"},
		Short:   "Creates secret structure with custom KMS key.",
		Long:    "Creates secret structure with custom KMS key.\n" + logLevelHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCommonFlags(flags); err != nil {
				return err
			}

			logCommonFlags(flags)
			logging.Info("Dry run:                       '%v'", isDryRun)
			logging.Spaces()

			lib, err := newLib(flags, isDryRun)
			if err != nil {
				return err
			}
			return lib.Create()
		},
	}

	addCommonFlags(cmd, flags)
	cmd.Flags().BoolVarP(&isDryRun, "dry-run", "n", os.Getenv("IS_DRY_RUN") == "1", "Disables command effect on target instance (env: IS_DRY_RUN)")

	return cmd
}

func newValidateCommand() *cobra.Command {
	flags := &commonFlags{}

	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"v"},
		Short:   "Validates secret structure with custom KMS key.",
		Long:    "Validates secret structure with custom KMS key.\n" + logLevelHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCommonFlags(flags); err != nil {
				return err
			}

			logCommonFlags(flags)
			logging.Spaces()

			lib, err := newLib(flags, false)
			if err != nil {
				return err
			}
			return lib.Validate()
		},
	}

	addCommonFlags(cmd, flags)

	return cmd
}

func newDestroyCommand() *cobra.Command {
	flags := &commonFlags{}
	var isForce, isDryRun bool

	cmd := &cobra.Command{
		Use:     "destroy",
		Aliases: []string{"Sdest"},
		Short:   "Destroy (delete) secrets created by yago.",
		Long: `Destroy (delete) secrets created by yago.

Only deletes secrets marked with is_created_here=true in configuration, which yago created (they have its
isCreatedHere tag). Includes deletion of associated KMS keys and aliases.

Uses 7-day deletion schedule for KMS keys (AWS requirement).
` + logLevelHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCommonFlags(flags); err != nil {
				return err
			}

			logCommonFlags(flags)
			logging.Info("Dry-run mode:                  '%s'", enabled(isDryRun))
			logging.Info("Force (skip confirmation):     '%s'", enabled(isForce))
			logging.Spaces()

			lib, err := newLib(flags, isDryRun)
			if err != nil {
				return err
			}
			return destroySecrets(lib, cmd.InOrStdin(), isForce, isDryRun)
		},
	}

	addCommonFlags(cmd, flags)
	cmd.Flags().BoolVarP(&isForce, "force", "f", false, "Skip confirmation and force destruction without prompting")
	cmd.Flags().BoolVar(&isDryRun, "dry-run", false, "Show what would be destroyed without actually deleting")

	return cmd
}

func destroySecrets(lib *Lib, input io.Reader, isForce, isDryRun bool) error {
	secretsToDestroy := lib.SecretsCreatedHere()

	if len(secretsToDestroy) == 0 {
		logging.Warn("No secrets marked with is_created_here=true found in configuration")
		return nil
	}

	logging.Info("Found %d secret(s) to destroy:", len(secretsToDestroy))
	for _, name := range secretsToDestroy {
		logging.Info("  - %s", name)
	}

	if !isForce && !isDryRun {
		logging.Spaces()
		confirmed, err := confirm(input, fmt.Sprintf("About to destroy %d secret(s). Continue? [y/N]: ", len(secretsToDestroy)))
		if err != nil {
			return err
		}
		if !confirmed {
			logging.Warn("Destruction cancelled by user")
			return nil
		}
	}

	logging.Spaces()

	failed := []string{}
	for _, name := range secretsToDestroy {
		logging.Info("Destroying secret: %s", name)
		if err := lib.Destroy(name); err != nil {
			logging.Error("✗ Failed to destroy %s: %v", name, err)
			failed = append(failed, name)
			continue
		}
		if isDryRun {
			logging.Info("[Dry-Run] Would destroy: %s", name)
		} else {
			logging.Info("✓ Successfully destroyed: %s", name)
		}
	}

	logging.Spaces()
	if isDryRun {
		logging.Info("[Dry-Run] Destruction summary: %d/%d would succeed", len(secretsToDestroy)-len(failed), len(secretsToDestroy))
	} else {
		logging.Info("Destruction summary: %d/%d successful", len(secretsToDestroy)-len(failed), len(secretsToDestroy))
	}

	if len(failed) > 0 {
		logging.Error("Failed to destroy the following secrets:")
		for _, name := range failed {
			logging.Error("  - %s", name)
		}
		return errors.Newf(errors.ErrFail, "failed to destroy %d of %d secrets", len(failed), len(secretsToDestroy))
	}
	return nil
}

func confirm(input io.Reader, question string) (bool, error) {
	fmt.Fprint(os.Stderr, question)

	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && answer == "" {
		return false, errors.New(errors.ErrParam, "destruction needs confirmation, but there is no input to read it from (use --force)")
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func enabled(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}
