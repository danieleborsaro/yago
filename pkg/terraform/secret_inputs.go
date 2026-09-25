package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awsmanager "github.com/danieleborsaro/yago/pkg/aws"
	"gopkg.in/yaml.v3"
)

const secretManifestName = "terraform-secrets.json"
const planSecretSuffix = ".secrets.json"

// secretInputManifest contains references only; resolved values never enter the cache.
type secretInputManifest struct {
	Version    int                              `json:"version"`
	AWSProfile string                           `json:"aws_profile,omitempty"`
	AWSRegion  string                           `json:"aws_region,omitempty"`
	Variables  map[string]SecretVariableBinding `json:"variables"`
}

func cacheSecretInputs(configPath, buildDir, profile, region string, generateJSON bool) error {
	manifest := secretInputManifest{Version: 1, AWSProfile: profile, AWSRegion: region}
	if configPath != "" {
		data, err := os.ReadFile(filepath.Clean(configPath))
		if err != nil {
			return err
		}
		var doc map[string]interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("cannot parse assembled Terraform configuration: %w", err)
		}
		if raw, exists := doc["secret_variables"]; exists {
			encoded, err := yaml.Marshal(raw)
			if err != nil {
				return fmt.Errorf("invalid secret_variables configuration: %w", err)
			}
			decoder := yaml.NewDecoder(bytes.NewReader(encoded))
			decoder.KnownFields(true)
			if err := decoder.Decode(&manifest.Variables); err != nil {
				return fmt.Errorf("secret_variables must map Terraform variable names to secret_id, json_key, version_id, or version_stage: %w", err)
			}
			if err := validateSecretBindings(manifest.Variables); err != nil {
				return err
			}
			for name := range manifest.Variables {
				if _, exists := doc[name]; exists {
					return fmt.Errorf("the Terraform variable %q is defined in both configuration and secret_variables", name)
				}
			}
			delete(doc, "secret_variables")
			if generateJSON {
				data, err = json.MarshalIndent(doc, "", "  ")
			} else {
				data, err = yaml.Marshal(doc)
			}
			if err != nil {
				return fmt.Errorf("cannot encode assembled Terraform configuration: %w", err)
			}
			if err := os.WriteFile(configPath, data, 0600); err != nil {
				return err
			}
		}
	}
	// Always replace the manifest, including when the new configuration has no bindings.
	return writeSecretManifest(filepath.Join(buildDir, secretManifestName), manifest)
}

func writeSecretManifest(path string, manifest secretInputManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func readSecretManifest(path string) (secretInputManifest, error) {
	manifest := secretInputManifest{Version: 1}
	data, err := os.ReadFile(filepath.Clean(path))
	if os.IsNotExist(err) {
		return manifest, nil
	}
	if err != nil {
		return manifest, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Version != 1 {
		return manifest, fmt.Errorf("invalid Terraform secret reference manifest; run yago tf assemble again")
	}
	return manifest, validateSecretBindings(manifest.Variables)
}

func defaultSecretManifest(workingDir string) string {
	return filepath.Join(workingDir, ".gitops", secretManifestName)
}

func terraformPlanPath(workingDir, planFile string) string {
	if filepath.IsAbs(planFile) {
		return planFile
	}
	return filepath.Join(workingDir, planFile)
}

func (s *Service) secretEnvironment(manifestPath string) (map[string]string, map[string]SecretVariableBinding, error) {
	manifest, err := readSecretManifest(manifestPath)
	if err != nil || len(manifest.Variables) == 0 {
		return nil, nil, err
	}
	profile, region := s.awsProfile, s.awsRegion
	if profile == "" {
		profile = manifest.AWSProfile
	}
	if region == "" {
		region = manifest.AWSRegion
	}
	reader := s.secretReader
	if reader == nil {
		manager, err := awsmanager.NewSecretManager(profile, region, false)
		if err != nil {
			return nil, nil, fmt.Errorf("cannot initialize AWS Secrets Manager for Terraform inputs: %w", err)
		}
		reader = manager
		defer func() { _ = manager.Close() }()
	}
	return resolveSecretVariables(manifest.Variables, reader)
}

func needsSecretVariables(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "plan", "apply", "destroy", "import", "refresh", "console":
		return true
	default:
		return false
	}
}

func mergeCommandEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, entry)
		}
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}
