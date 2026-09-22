package secretsmanager

import (
	"fmt"

	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// SecretSpec defines the specification for a single secret.
type SecretSpec struct {
	Name                 string            `yaml:"name"`
	Description          string            `yaml:"description,omitempty"`
	SecretString         string            `yaml:"secretString,omitempty"`
	SecretBinary         string            `yaml:"secretBinary,omitempty"`
	KmsKeyId             string            `yaml:"kmsKeyId,omitempty"`
	Tags                 map[string]string `yaml:"tags,omitempty"`
	AutoRotationEnabled  bool              `yaml:"autoRotationEnabled,omitempty"`
	RotationIntervalDays int32             `yaml:"rotationIntervalDays,omitempty"`
	LambdaArn            string            `yaml:"lambdaArn,omitempty"`
	ResourcePolicy       string            `yaml:"resourcePolicy,omitempty"`
	AddReplicaRegions    []string          `yaml:"addReplicaRegions,omitempty"`
	RemoveReplicaRegions []string          `yaml:"removeReplicaRegions,omitempty"`
	IsCreatedHere        bool              `yaml:"isCreatedHere,omitempty"` // Flag to mark secrets eligible for destruction
}

// ValidateSecretSpec validates a secret specification.
func ValidateSecretSpec(spec *SecretSpec) error {
	logging.Debug("Validating secret spec: %s", spec.Name)

	if spec.Name == "" {
		return fmt.Errorf("secret name is required")
	}

	// Secret value is required (either string or binary)
	if spec.SecretString == "" && spec.SecretBinary == "" {
		return fmt.Errorf("secret %s: either secretString or secretBinary is required", spec.Name)
	}

	// Both shouldn't be set
	if spec.SecretString != "" && spec.SecretBinary != "" {
		return fmt.Errorf("secret %s: cannot specify both secretString and secretBinary", spec.Name)
	}

	// Validate rotation settings
	if spec.AutoRotationEnabled {
		if spec.LambdaArn == "" {
			return fmt.Errorf("secret %s: lambdaArn is required when autoRotationEnabled is true", spec.Name)
		}
		if spec.RotationIntervalDays <= 0 {
			return fmt.Errorf("secret %s: rotationIntervalDays must be > 0", spec.Name)
		}
	}

	logging.Debug("Secret spec %s validation successful", spec.Name)
	return nil
}

// SecretManagerDesiredStateSpec defines the top-level spec for secretsmanager desired state.
type SecretManagerDesiredStateSpec struct {
	Secrets []SecretSpec `yaml:"secrets"`
}

// SecretManagerConfigSpec defines the configuration section.
type SecretManagerConfigSpec struct {
	DefaultKmsKeyId string            `yaml:"defaultKmsKeyId,omitempty"`
	DefaultTags     map[string]string `yaml:"defaultTags,omitempty"`
	DefaultRegions  []string          `yaml:"defaultRegions,omitempty"`
}

// ValidateDesiredStateSpec validates the entire desired state specification.
func ValidateDesiredStateSpec(spec *SecretManagerDesiredStateSpec) error {
	logging.Debug("Validating desired state spec with %d secrets", len(spec.Secrets))

	if len(spec.Secrets) == 0 {
		return fmt.Errorf("at least one secret must be defined")
	}

	for _, secret := range spec.Secrets {
		if err := ValidateSecretSpec(&secret); err != nil {
			return err
		}
	}

	logging.Debug("Desired state spec validation successful")
	return nil
}
