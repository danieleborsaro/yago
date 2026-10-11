package terraform

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"

	"github.com/danieleborsaro/yago/internal/core"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
	"github.com/danieleborsaro/yago/pkg/wrapper"
	"gopkg.in/yaml.v3"
)

// eu-west-1, us-gov-west-1 and the like, which no setting of the declaration could be mistaken for
var awsRegionShape = regexp.MustCompile(`^[a-z]{2,}(-[a-z]+)+-[0-9]+$`)

// what an assembled configuration says about its backends, without any one region having to resolve
type BackendSet struct {
	configuration map[string]interface{}
	declaration   map[string]interface{}
	// the configuration document, its meta is where the backends files are listed
	document    *core.GitOpsDocument
	environment string
}

// the inputs awstagging takes per bucket, next to the project properties it takes for everything
type bucketNameInputs struct {
	RoleExtend        string   `yaml:"role_extend"`
	VersioningSources []string `yaml:"versioning_sources"`
}

// assembles in memory, so unlike an assemble for init it needs no terraform source and no backend that resolves, and writes nothing
func (s *Service) Backends(req TerraformAssembleRequest) (*BackendSet, error) {
	// an assemble for init exports the profile before it reads a part, a lookup of it has to resolve the same here
	if req.AWSProfile != "" {
		if err := os.Setenv("AWS_PROFILE", req.AWSProfile); err != nil {
			return nil, errors.Wrapf(errors.ErrFail, err, "failed to export AWS_PROFILE")
		}
	}
	response, err := s.Assemble(wrapper.AssembleRequest{
		DesiredStateFile:  req.DesiredStateFile,
		ConfigFile:        req.ConfigFile,
		ConfigRepoWorkdir: req.ConfigRepoWorkdir,
		Environment:       req.Environment,
		Wrapper:           "terraform",
	})
	if err != nil {
		return nil, err
	}
	configuration, ok := wrapper.NormalizeMapForJSON(response.AssembledConfigurationContent).(map[string]interface{})
	if !ok {
		return nil, errors.NewConfigurationMalformedError("the assembled configuration is not a map")
	}
	declaration, err := backendDeclarationOf(configuration)
	if err != nil {
		return nil, malformedBackend(err)
	}
	// init only ever looks up the region it is given, here a misspelt setting would be listed as a region and its
	// value quietly dropped, so we stop instead
	for _, name := range slices.Sorted(maps.Keys(declaration)) {
		if !slices.Contains(backendDeclarationSettings, name) && !awsRegionShape.MatchString(name) {
			return nil, malformedBackend(fmt.Errorf("%s.%s is neither a setting nor a region", backendDeclarationKey, name))
		}
	}
	return &BackendSet{
		configuration: configuration, declaration: declaration,
		document: s.GetConfigurationDocument(), environment: req.Environment,
	}, nil
}

func (b *BackendSet) Regions() []string {
	regions := listedBackendRegions(b.document)
	for name := range b.declaration {
		if !slices.Contains(backendDeclarationSettings, name) && !slices.Contains(regions, name) {
			regions = append(regions, name)
		}
	}
	slices.Sort(regions)
	return regions
}

// the backend init gets for the region, resolved by the same rule
func (b *BackendSet) Backend(region string) (map[string]interface{}, error) {
	listed, _, err := b.listedBackend(region)
	if err != nil {
		return nil, err
	}
	backend, err := resolveBackend(listed, b.declaration, region, b.environment)
	if err != nil {
		return nil, malformedBackend(err)
	}
	if backend == nil {
		return nil, errNoBackend()
	}
	return backend, nil
}

// nil when the region has no listed backends file, the path is where that file is
func (b *BackendSet) listedBackend(region string) (*BackendConfig, string, error) {
	path, err := listedBackendFile(b.document, region)
	if err != nil || path == "" {
		return nil, "", err
	}
	manager := NewBackendManager("")
	if err := manager.LoadBackendConfig(path, region); err != nil {
		return nil, "", err
	}
	return manager.GetConfig(), path, nil
}

// the region's bucket as far as the configuration describes it, and the listed backends file that came from if any,
// unlike Backend it needs neither a bucket name nor a state key, as that is how things stand before the bucket is made
func (b *BackendSet) BucketSpec(region string) (aws.StateBucketSpec, string, error) {
	entry, listedFile, err := b.bucketEntry(region)
	if err != nil {
		return aws.StateBucketSpec{}, "", err
	}
	for _, unused := range []string{"role_arn", "assume_role", "profile"} {
		if _, set := entry[unused]; set {
			logging.Warn("The backend for %s sets %s, the bucket commands don't use it, they run as --aws-profile", region, unused)
		}
	}

	properties, err := wrapper.MergeProperties(b.configuration, false, tagPropertiesParts...)
	if err != nil {
		return aws.StateBucketSpec{}, "", err
	}
	spec := aws.StateBucketSpec{Region: region, Properties: properties}
	// a pin we can't read must never count as no pin, the commands would go and work out another bucket or key
	for _, pin := range []struct {
		name string
		into *string
	}{{"bucket", &spec.Name}, {"kms_key_id", &spec.KMSKeyID}, {"region", &spec.Region}} {
		switch value := entry[pin.name].(type) {
		case nil:
		case string:
			if value != "" {
				*pin.into = value
			}
		default:
			return aws.StateBucketSpec{}, "", fmt.Errorf("the backend for %s sets %s to something that isn't a string, quote it", region, pin.name)
		}
	}

	if raw, set := b.declaration["encryption"]; set {
		encryption, ok := raw.(string)
		if !ok {
			return aws.StateBucketSpec{}, "", fmt.Errorf("%s.encryption must be %s or %s", backendDeclarationKey, aws.EncryptionKms, aws.EncryptionSse)
		}
		spec.Encryption = encryption
	}

	inputs := bucketNameInputs{}
	lifecycle, policy := aws.DefaultStateBucketLifecycle(), aws.DefaultStateBucketPolicy()
	for _, setting := range []struct {
		name string
		into interface{}
	}{{"bucket_name", &inputs}, {"lifecycle", &lifecycle}, {"policy", &policy}} {
		if err := b.setting(setting.name, setting.into); err != nil {
			return aws.StateBucketSpec{}, "", err
		}
	}
	spec.RoleExtend, spec.VersioningSources = inputs.RoleExtend, inputs.VersioningSources
	spec.Lifecycle, spec.Policy = &lifecycle, &policy
	return spec, listedFile, nil
}

func (b *BackendSet) bucketEntry(region string) (map[string]interface{}, string, error) {
	listed, listedFile, err := b.listedBackend(region)
	if err != nil {
		return nil, "", err
	}
	if listed != nil {
		return listed.raw, listedFile, nil
	}
	entry, err := backendEntry(b.declaration, region)
	if err != nil {
		return nil, "", malformedBackend(err)
	}
	return entry, "", nil
}

// roots hand the tagging provider these three scopes merged in this order with Terraform's merge(), so we name and tag
// from the same merge, and a configuration that keeps everything in one project_properties block has the last word
var tagPropertiesParts = []string{"project_properties_global", "project_properties_env", "project_properties_proj", "project_properties"}

// one setting of the declaration read onto its defaults, a bucket can't be renamed and a policy shouldn't be guessed,
// so an input we don't know or can't read stops here
func (b *BackendSet) setting(name string, into interface{}) error {
	content, err := yaml.Marshal(b.declaration[name])
	if err == nil {
		decoder := yaml.NewDecoder(bytes.NewReader(content))
		decoder.KnownFields(true)
		err = decoder.Decode(into)
	}
	if err != nil {
		return fmt.Errorf("%s.%s can't be read, %w", backendDeclarationKey, name, err)
	}
	return nil
}
