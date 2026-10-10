package terraform

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	formatTable = "table"
	formatJSON  = "json"
	formatYAML  = "yaml"
)

type stateBucket interface {
	Name() (string, error)
	PlanCreate() (*aws.StateBucketPlan, error)
	PlanUpdate() (*aws.StateBucketPlan, error)
	Apply(plan *aws.StateBucketPlan) (*aws.StateBucketResult, error)
	EnsureDeletable() error
	Delete() error
}

var newStateBucket = func(spec aws.StateBucketSpec, awsProfile string) (stateBucket, error) {
	return aws.NewStateBucket(spec, awsProfile)
}

func newBackendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backend",
		Short: "Show the Terraform backends of a configuration and manage their state buckets",
		Long: `Show the Terraform backends a configuration resolves to, and create, update or delete
the S3 bucket and KMS key a backend keeps its state in.`,
	}
	cmd.AddCommand(newBackendListCommand())
	cmd.AddCommand(newBackendDescribeCommand())
	cmd.AddCommand(newBackendProvisionCommand("create",
		"Create the state bucket, versioned, private, encrypted, protected and tagged",
		`Create the state bucket of the region's backend, with versioning on, public access blocked, ACLs off, encryption,
a lifecycle rule for old versions, a bucket policy that refuses plain HTTP and deleting state, and tags.
A bucket the configuration doesn't name yet is named the way the awstagging provider names an S3 bucket,
and the name and key to add to the configuration are printed. Stops if the bucket already exists.
Everything it will do is shown first and has to be agreed to, -A skips the question.`,
		stateBucket.PlanCreate))
	cmd.AddCommand(newBackendProvisionCommand("update",
		"Put versioning, the public access block, the lifecycle rule, the policy and tags back on the state bucket",
		`Put versioning, the public access block, ACLs off, the lifecycle rule, the bucket policy and tags back on a state
bucket that exists. Lifecycle rules, policy statements and tags the bucket has of its own are kept.
The encryption of a bucket that has a KMS key is left as it is, and a configuration that asks for another is refused.
A bucket without a KMS key of its own is moved to what the configuration says.
Everything it will do, a change of encryption included, is shown first and has to be agreed to, -A skips the question.`,
		stateBucket.PlanUpdate))
	cmd.AddCommand(newBackendDeleteCommand())
	return cmd
}

func newBackendListCommand() *cobra.Command {
	flags := &commonFlags{}
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print the bucket and state key of every region this configuration has a backend for",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(format); err != nil {
				return err
			}
			set, err := loadBackendSet(flags)
			if err != nil {
				return err
			}
			regions := set.Regions()
			if len(regions) == 0 {
				return fmt.Errorf("this configuration has no backend for any region")
			}
			backends := make([]map[string]interface{}, 0, len(regions))
			for _, region := range regions {
				backend, err := set.Backend(region)
				if err != nil {
					return err
				}
				backends = append(backends, withRegion(backend, region))
			}
			return writeBackends(cmd.OutOrStdout(), format, backends, backends)
		},
	}
	addBackendFlags(cmd, flags, false, "aws-region")
	addFormatFlag(cmd, &format, formatTable)
	return cmd
}

func newBackendDescribeCommand() *cobra.Command {
	flags := &commonFlags{}
	var format string
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Print the backend this configuration resolves to for the region",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(format); err != nil {
				return err
			}
			set, err := loadBackendSet(flags)
			if err != nil {
				return err
			}
			backend, err := set.Backend(flags.awsRegion)
			if err != nil {
				return err
			}
			// the table needs a region to show, the JSON and YAML stay exactly what init gets
			return writeBackends(cmd.OutOrStdout(), format, backend, []map[string]interface{}{withRegion(backend, flags.awsRegion)})
		},
	}
	addBackendFlags(cmd, flags, true)
	addFormatFlag(cmd, &format, formatJSON)
	return cmd
}

// nothing happens to a bucket that wasn't shown first, the plan is printed, then agreed to, then applied as it is
func newBackendProvisionCommand(use, short, long string, plan func(stateBucket) (*aws.StateBucketPlan, error)) *cobra.Command {
	flags := &commonFlags{}
	var isDryRun, isAutoApprove bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := loadStateBucket(flags)
			if err != nil {
				return err
			}
			planned, err := plan(target.bucket)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "yago tf backend %s will\n", use)
			for _, change := range planned.Changes {
				// a change can run over several lines, its tags do, and they stay under their own bullet
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", strings.ReplaceAll(change, "\n", "\n      "))
			}
			if isDryRun {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "This is a dry run, nothing was changed")
				return nil
			}
			if !isAutoApprove {
				if err := confirmTyped(cmd.InOrStdin(), cmd.ErrOrStderr(), "Type yes to go ahead: ", "yes"); err != nil {
					return fmt.Errorf("nothing was changed, %w (use --auto-approve when there is nobody to ask)", err)
				}
			}
			result, err := target.bucket.Apply(planned)
			if err != nil {
				return err
			}
			return target.writePins(cmd.OutOrStdout(), result)
		},
	}
	addBackendFlags(cmd, flags, true)
	addDryRunFlag(cmd, &isDryRun)
	cmd.Flags().BoolVarP(&isAutoApprove, "auto-approve", "A", false, "Auto approve, do not ask for confirmation")
	return cmd
}

func newBackendDeleteCommand() *cobra.Command {
	flags := &commonFlags{}
	var isDryRun, isForce bool
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete the state bucket, only if it is empty",
		Long: `Delete the state bucket of the region's backend, only if it holds nothing, old versions and delete markers included.
The bucket name has to be typed back first. The KMS key is left alone.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := loadStateBucket(flags)
			if err != nil {
				return err
			}
			// checked before the question, so nobody is asked to confirm a delete that would be refused anyway
			if err := target.bucket.EnsureDeletable(); err != nil {
				return err
			}
			name, err := target.bucket.Name()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "yago tf backend delete will\n  - delete the empty state bucket %s, its KMS key is left alone\n", name)
			if isDryRun {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "This is a dry run, nothing was changed")
				return nil
			}
			if !isForce {
				// a y/N gets answered by habit, typing the name makes us read which bucket it is
				if err := confirmTyped(cmd.InOrStdin(), cmd.ErrOrStderr(), "Type the bucket name to delete it: ", name); err != nil {
					return fmt.Errorf("state bucket %s was not deleted, %w (use --force when there is nobody to ask)", name, err)
				}
			}
			return target.bucket.Delete()
		},
	}
	addBackendFlags(cmd, flags, true)
	addDryRunFlag(cmd, &isDryRun)
	cmd.Flags().BoolVarP(&isForce, "force", "f", false, "Skip the confirmation, a bucket that isn't empty is still refused")
	return cmd
}

// the backend commands assemble in memory, so the flags that only matter to a terraform run are there but hidden,
// the profile stays even where AWS is never called, a configuration can look it up
func addBackendFlags(cmd *cobra.Command, flags *commonFlags, requireRegion bool, unused ...string) {
	addCommonFlags(cmd, flags, true, requireRegion)
	for _, name := range append(unused, "sourcecode-repo-workdir", "terraform-source", "workspace") {
		// only fails for a flag that isn't there, and these all come from addCommonFlags
		_ = cmd.Flags().MarkHidden(name)
	}
}

func addFormatFlag(cmd *cobra.Command, format *string, defaultFormat string) {
	cmd.Flags().StringVar(format, "format", defaultFormat, fmt.Sprintf("Output format, one of %s, %s or %s", formatTable, formatJSON, formatYAML))
}

func addDryRunFlag(cmd *cobra.Command, isDryRun *bool) {
	cmd.Flags().BoolVarP(isDryRun, "dry-run", "n", os.Getenv("IS_DRY_RUN") == "1", "Disables command effect on target instance (env: IS_DRY_RUN)")
}

func loadBackendSet(flags *commonFlags) (*BackendSet, error) {
	if err := validateAssembleEnvironment(flags.environment); err != nil {
		return nil, err
	}
	set, err := NewService(".", false).Backends(flags.assembleRequest())
	if err != nil {
		return nil, fmt.Errorf("failed to assemble terraform configuration: %w", err)
	}
	return set, nil
}

// a state bucket and where what it works out has to be written down
type bucketTarget struct {
	bucket     stateBucket
	spec       aws.StateBucketSpec
	region     string
	listedFile string
}

func loadStateBucket(flags *commonFlags) (*bucketTarget, error) {
	set, err := loadBackendSet(flags)
	if err != nil {
		return nil, err
	}
	spec, listedFile, err := set.BucketSpec(flags.awsRegion)
	if err != nil {
		return nil, err
	}
	bucket, err := newStateBucket(spec, flags.awsProfile)
	if err != nil {
		return nil, err
	}
	return &bucketTarget{bucket: bucket, spec: spec, region: flags.awsRegion, listedFile: listedFile}, nil
}

// init only reads the bucket and its key from the configuration, so what was worked out here has to be written down there,
// which for a region on a listed backends file means that file, as the declaration's entry for the region isn't read then
func (t *bucketTarget) writePins(out io.Writer, result *aws.StateBucketResult) error {
	pins := map[string]interface{}{}
	if t.spec.Name == "" {
		pins["bucket"] = result.Name
	}
	if t.spec.KMSKeyID == "" && result.KMSKeyID != "" {
		pins["kms_key_id"] = result.KMSKeyID
	}
	if len(pins) == 0 {
		return nil
	}
	if t.listedFile != "" {
		logging.Info("Add this to %s, init reads the bucket and its key from there", t.listedFile)
		// in the file's own format, a backends file is JSON or YAML
		format := formatYAML
		if strings.HasSuffix(t.listedFile, ".json") {
			format = formatJSON
		}
		return writeBackends(out, format, pins, nil)
	}
	logging.Info("Add this to the configuration, init reads the bucket and its key from there")
	return writeBackends(out, formatYAML, map[string]interface{}{backendDeclarationKey: map[string]interface{}{t.region: pins}}, nil)
}

func confirmTyped(input io.Reader, prompt io.Writer, question, want string) error {
	_, _ = fmt.Fprint(prompt, question)
	answer, _ := bufio.NewReader(input).ReadString('\n')
	if strings.TrimSpace(answer) != want {
		return fmt.Errorf("what was typed is not %q", want)
	}
	return nil
}

func validateFormat(format string) error {
	switch format {
	case formatTable, formatJSON, formatYAML:
		return nil
	default:
		return fmt.Errorf("unknown format %q, use %s, %s or %s", format, formatTable, formatJSON, formatYAML)
	}
}

// a listed backends file doesn't have to say its region, and a row nobody can place is no use
func withRegion(backend map[string]interface{}, region string) map[string]interface{} {
	if _, named := backend["region"]; named {
		return backend
	}
	placed := maps.Clone(backend)
	placed["region"] = region
	return placed
}

func writeBackends(out io.Writer, format string, value interface{}, rows []map[string]interface{}) error {
	switch format {
	case formatTable:
		table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(table, "REGION\tBUCKET\tKEY")
		for _, row := range rows {
			// a listed backends file may leave one of these out, and an empty cell reads better than <nil>
			cells := make([]string, 0, 3)
			for _, name := range []string{"region", "bucket", "key"} {
				cell, _ := row[name].(string)
				cells = append(cells, cell)
			}
			_, _ = fmt.Fprintln(table, strings.Join(cells, "\t"))
		}
		return table.Flush()
	case formatJSON:
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	default:
		// two spaces, as every YAML file here is written, so what create prints can be pasted in as it is
		encoder := yaml.NewEncoder(out)
		encoder.SetIndent(2)
		if err := encoder.Encode(value); err != nil {
			return err
		}
		return encoder.Close()
	}
}
