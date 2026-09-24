package secretsmanager

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	awssecretsmanager "github.com/danieleborsaro/yago/pkg/aws"
)

type secretManager interface {
	AccountID() (string, error)
	Create(req awssecretsmanager.CreateRequest) (*awssecretsmanager.SecretCreationResult, bool, error)
	Validate(secretName string, expectedKeys []string, versions []string) (*awssecretsmanager.SecretValidationResult, error)
	Destroy(secretName string) error
}

type Lib struct {
	awsRegion string
	isDryRun  bool
	secMan    secretManager

	configuration map[string]interface{}
	secrets       Secrets
	tagConfig     *awssecretsmanager.TagConfiguration
}

func NewLib(awsProfile, awsRegion string, isDryRun bool) (*Lib, error) {
	secMan, err := awssecretsmanager.NewSecretManager(awsProfile, awsRegion, isDryRun)
	if err != nil {
		return nil, err
	}

	return &Lib{awsRegion: awsRegion, isDryRun: isDryRun, secMan: secMan}, nil
}

func (l *Lib) LoadGitOpsFiles(desiredStateFile, configFile, environment string) error {
	response, err := NewService(".", false).AssembleSecrets(desiredStateFile, configFile, environment, "")
	if err != nil {
		return err
	}

	return l.SetConfiguration(response.AssembledConfigurationContent)
}

func (l *Lib) SetConfiguration(configuration map[string]interface{}) error {
	secrets, err := ParseSecrets(configuration)
	if err != nil {
		return errors.Wrap(errors.ErrParse, "invalid secrets configuration", err)
	}

	l.configuration = configuration
	l.secrets = secrets
	l.tagConfig = nil
	return nil
}

func (l *Lib) regionSecrets() (map[string]Secret, []string, bool) {
	secrets, keys, exists := l.secrets.Region(l.awsRegion)
	if !exists {
		logging.Warn("No secrets configured for region %s", l.awsRegion)
	}
	return secrets, keys, exists
}

func (l *Lib) Plan() error {
	logging.Info("Planning secret creation...")

	created, err := l.create(false)
	if err != nil {
		return err
	}

	for _, result := range created {
		logging.Snippet(describeCreatedSecret(result), "New secret:", "\n")
	}

	logging.Info("To create %d secrets", len(created))
	return nil
}

func (l *Lib) Create() error {
	logging.Info("Creating secret...")

	created, err := l.create(true)
	if err != nil {
		return err
	}

	if l.isDryRun {
		logging.Info("[Dry run] Created %d secrets", len(created))
	} else {
		logging.Info("Created %d secrets", len(created))
	}
	return nil
}

func (l *Lib) create(showNew bool) ([]*awssecretsmanager.SecretCreationResult, error) {
	created := []*awssecretsmanager.SecretCreationResult{}

	secrets, keys, exists := l.regionSecrets()
	if !exists {
		return created, nil
	}

	for _, key := range keys {
		secret := secrets[key]

		var secretTags, kmsKeyTags map[string]string
		if secret.IsCreatedHere {
			var err error
			secretTags, kmsKeyTags, err = l.tags(secret.Name)
			if err != nil {
				return nil, err
			}
		}

		result, isCreated, err := l.secMan.Create(secret.createRequest(secretTags, kmsKeyTags))
		if err != nil {
			return nil, err
		}

		if isCreated && result != nil {
			created = append(created, result)
			if showNew {
				logging.Snippet(describeCreatedSecret(result), "New secret:", "\n")
			}
		} else {
			logging.Spaces()
		}
	}

	return created, nil
}

func (l *Lib) Validate() error {
	secrets, keys, exists := l.regionSecrets()
	if !exists {
		return nil
	}

	invalid := []string{}
	for _, key := range keys {
		secret := secrets[key]
		logging.Debug("Validating secret %s...", secret.Name)

		result, err := l.secMan.Validate(secret.Name, secret.KeyNames(), secret.VersionValues())
		if err != nil {
			return err
		}

		switch {
		case !result.IsExpectedKeysOk:
			logging.Error("Secret %s is not valid: configured keys mismatch", secret.Name)
			invalid = append(invalid, secret.Name)
		case !result.IsStoredKeysOk:
			logging.Error("Secret %s is not valid: stored keys mismatch", secret.Name)
			invalid = append(invalid, secret.Name)
		default:
			logging.Info("Secret %s is valid", secret.Name)
			logging.Snippet(fmt.Sprintf("Secret:  %s\nKMS Key: %s\nAlias:   %s\nVersion: %s",
				result.ARN, result.KmsKeyId, result.KmsAliasName, strings.Join(result.VersionsValidated, ", ")),
				"Valid secret:", "\n")
		}
		if !result.IsExpectedKeysOk || !result.IsStoredKeysOk {
			logging.Debug("Configured keys: %v, stored keys: %v", result.ExpectedKeys, result.StoredKeys)
		}
	}

	if len(invalid) > 0 {
		return errors.Newf(errors.ErrFail, "%d of %d secrets are not valid: %s",
			len(invalid), len(keys), strings.Join(invalid, ", "))
	}
	return nil
}

func (l *Lib) SecretsCreatedHere() []string {
	logging.Debug("Retrieving secrets created here in region %s...", l.awsRegion)

	names := []string{}
	secrets, keys, exists := l.secrets.Region(l.awsRegion)
	if !exists {
		logging.Debug("No secrets configured for region %s", l.awsRegion)
		return names
	}

	for _, key := range keys {
		secret := secrets[key]
		if secret.IsCreatedHere {
			names = append(names, secret.Name)
			logging.Debug("Found secret created here: %s", secret.Name)
		} else {
			logging.Debug("Skipping secret not created here: %s", secret.Name)
		}
	}
	return names
}

func (l *Lib) Destroy(secretName string) error {
	return l.secMan.Destroy(secretName)
}

func (l *Lib) tags(secretName string) (map[string]string, map[string]string, error) {
	config, err := l.tagConfiguration()
	if err != nil {
		return nil, nil, err
	}

	secretTags, dropped, err := awssecretsmanager.Tags(config, awssecretsmanager.ResourceTypeSecret, secretName)
	if err != nil {
		return nil, nil, err
	}
	warnDroppedTags(secretName, dropped)

	kmsKeyTags, dropped, err := awssecretsmanager.Tags(config, awssecretsmanager.ResourceTypeKmsKey, secretName)
	if err != nil {
		return nil, nil, err
	}
	warnDroppedTags("the KMS key of "+secretName, dropped)

	logging.Debug("Tags for secret %s:\n%s", secretName, awssecretsmanager.FormatTags(secretTags))
	logging.Debug("Tags for the KMS key of %s:\n%s", secretName, awssecretsmanager.FormatTags(kmsKeyTags))

	return secretTags, kmsKeyTags, nil
}

func warnDroppedTags(resource string, dropped []string) {
	if len(dropped) > 0 {
		logging.Warn("AWS allows 50 tags, so %s doesn't get: %s", resource, strings.Join(dropped, ", "))
	}
}

func (l *Lib) tagConfiguration() (awssecretsmanager.TagConfiguration, error) {
	if l.tagConfig != nil {
		return *l.tagConfig, nil
	}

	accountId, err := l.secMan.AccountID()
	if err != nil {
		return awssecretsmanager.TagConfiguration{}, err
	}

	content, err := yaml.Marshal(l.configuration["project_properties"])
	if err != nil {
		return awssecretsmanager.TagConfiguration{}, errors.Wrap(errors.ErrParse, "failed to read project_properties", err)
	}

	var properties struct {
		CompanyNameShort   string                                     `yaml:"company_name_short"`
		AccountsCoding     map[string]awssecretsmanager.AccountCoding `yaml:"accounts_coding"`
		AppEcosystem       string                                     `yaml:"app_ecosystem"`
		AppEnvironment     string                                     `yaml:"app_environment"`
		InfraEnvironment   string                                     `yaml:"infra_environment"`
		ProjectNameLong    string                                     `yaml:"project_name_long"`
		ResourceSetLong    string                                     `yaml:"resource_set_long"`
		Role               string                                     `yaml:"role"`
		Owner              string                                     `yaml:"owner"`
		CostCentre         string                                     `yaml:"cost_centre"`
		Compliance         string                                     `yaml:"compliance"`
		Description        string                                     `yaml:"description"`
		CustomTags         map[string]string                          `yaml:"custom_tags"`
		CustomTagsVerbatim map[string]string                          `yaml:"custom_tags_verbatim"`
	}
	if err := yaml.Unmarshal(content, &properties); err != nil {
		return awssecretsmanager.TagConfiguration{}, errors.Wrap(errors.ErrParse, "failed to read project_properties", err)
	}

	l.tagConfig = &awssecretsmanager.TagConfiguration{
		Account:            accountId,
		AccountsCoding:     properties.AccountsCoding,
		CompanyNameShort:   properties.CompanyNameShort,
		AppEcosystem:       properties.AppEcosystem,
		AppEnvironment:     properties.AppEnvironment,
		InfraEnvironment:   properties.InfraEnvironment,
		ProjectNameLong:    properties.ProjectNameLong,
		ResourceSetLong:    properties.ResourceSetLong,
		Role:               properties.Role,
		Owner:              properties.Owner,
		CostCentre:         properties.CostCentre,
		Compliance:         properties.Compliance,
		Description:        properties.Description,
		CustomTags:         properties.CustomTags,
		CustomTagsVerbatim: properties.CustomTagsVerbatim,
	}
	return *l.tagConfig, nil
}

func describeCreatedSecret(result *awssecretsmanager.SecretCreationResult) string {
	return fmt.Sprintf("Secret:  %s\nKMS Key: %s\nAlias:   %s\nVersion: %s",
		result.ARN, result.KmsKeyId, result.KmsAliasName, result.VersionId)
}
