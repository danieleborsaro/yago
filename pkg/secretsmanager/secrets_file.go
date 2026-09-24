package secretsmanager

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"

	awssecretsmanager "github.com/danieleborsaro/yago/pkg/aws"
)

var secretNameRe = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)

type Secrets map[string]map[string]Secret

type Secret struct {
	IsCreatedHere bool                          `yaml:"is_created_here"`
	Name          string                        `yaml:"name"`
	Description   string                        `yaml:"description"`
	Encryption    string                        `yaml:"encryption"`
	KmsKeyId      string                        `yaml:"kms_key_id"`
	Versions      map[string]string             `yaml:"versions"`
	Keys          map[string]string             `yaml:"keys"`
	Permissions   awssecretsmanager.Permissions `yaml:"permissions"`
}

func (s Secret) KeyNames() []string {
	names := make([]string, 0, len(s.Keys))
	for _, name := range s.Keys {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s Secret) VersionValues() []string {
	values := make([]string, 0, len(s.Versions))
	for _, label := range sortedKeys(s.Versions) {
		values = append(values, s.Versions[label])
	}
	return values
}

func (s Secret) createsKmsKey() bool {
	return s.IsCreatedHere && s.Encryption != awssecretsmanager.EncryptionSse && s.KmsKeyId == ""
}

func (s Secret) createRequest(secretTags, kmsKeyTags map[string]string) awssecretsmanager.CreateRequest {
	return awssecretsmanager.CreateRequest{
		Name:                 s.Name,
		Description:          s.Description,
		PlaceholderKeys:      s.KeyNames(),
		PlaintextPlaceholder: awssecretsmanager.PlaceholderValue,
		IsCreatedHere:        s.IsCreatedHere,
		Encryption:           s.Encryption,
		KmsKeyId:             s.KmsKeyId,
		SecretTags:           secretTags,
		KmsKeyTags:           kmsKeyTags,
		Permissions:          s.Permissions,
	}
}

func ParseSecrets(configuration map[string]interface{}) (Secrets, error) {
	block, exists := configuration["secrets"]
	if !exists || block == nil {
		return Secrets{}, nil
	}

	content, err := yaml.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to read the secrets block: %w", err)
	}

	secrets := Secrets{}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&secrets); err != nil {
		return nil, fmt.Errorf("failed to parse the secrets block: %w", err)
	}

	if err := secrets.Validate(); err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s Secrets) Validate() error {
	for _, region := range sortedKeys(s) {
		secrets := s[region]

		names := map[string]string{}
		aliases := map[string]string{}
		for _, key := range sortedKeys(secrets) {
			secret := secrets[key]
			if secret.Name == "" {
				return fmt.Errorf("secret %s in %s: name is required", key, region)
			}
			if !secretNameRe.MatchString(secret.Name) {
				return fmt.Errorf("secret %s in %s: name %q may only use letters, digits and /_+=.@- (up to 512 characters)",
					key, region, secret.Name)
			}
			if other, exists := names[secret.Name]; exists {
				return fmt.Errorf("secret %s in %s: name %q is also used by %s", key, region, secret.Name, other)
			}
			names[secret.Name] = key

			switch secret.Encryption {
			case "", awssecretsmanager.EncryptionKms:
			case awssecretsmanager.EncryptionSse:
				if secret.KmsKeyId != "" {
					return fmt.Errorf("secret %s in %s: kms_key_id can't be used with encryption: %s", key, region, secret.Encryption)
				}
			default:
				return fmt.Errorf("secret %s in %s: encryption must be %s or %s, not %q",
					key, region, awssecretsmanager.EncryptionKms, awssecretsmanager.EncryptionSse, secret.Encryption)
			}

			if secret.createsKmsKey() {
				if err := awssecretsmanager.ValidateKmsAlias(secret.Name); err != nil {
					return fmt.Errorf("secret %s in %s: %w", key, region, err)
				}
				alias := awssecretsmanager.KmsAliasName(secret.Name)
				if other, exists := aliases[alias]; exists {
					return fmt.Errorf("secret %s in %s: its KMS alias %q is also the alias of %s", key, region, alias, other)
				}
				aliases[alias] = key
			}

			if err := secret.Permissions.Validate(); err != nil {
				return fmt.Errorf("secret %s in %s: %w", key, region, err)
			}

			fields := map[string]string{}
			for _, label := range sortedKeys(secret.Keys) {
				field := secret.Keys[label]
				if field == "" {
					return fmt.Errorf("secret %s in %s: key %s has no field name", key, region, label)
				}
				if other, exists := fields[field]; exists {
					return fmt.Errorf("secret %s in %s: keys %s and %s both use field %q", key, region, other, label, field)
				}
				fields[field] = label
			}

			for _, label := range sortedKeys(secret.Versions) {
				if secret.Versions[label] == "" {
					return fmt.Errorf("secret %s in %s: version %s is empty", key, region, label)
				}
			}
		}
	}
	return nil
}

func (s Secrets) Region(region string) (map[string]Secret, []string, bool) {
	secrets, exists := s[region]
	if !exists {
		return nil, nil, false
	}
	return secrets, sortedKeys(secrets), true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
