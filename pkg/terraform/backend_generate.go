package terraform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	backendDeclarationKey    = "terraform_backend"
	defaultBackendKeyPattern = "{ecosystem}/{resource_set}/{project}/{component}/terraform.tfstate"
)

var backendKeyPlaceholder = regexp.MustCompile(`\{[^{}]+\}`)

// terraform complains about variables it doesn't know, so we take the declaration out of the var file
func takeBackendDeclaration(configPath string, generateJSON bool) (map[string]interface{}, error) {
	if configPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse assembled Terraform configuration: %w", err)
	}
	raw, exists := doc[backendDeclarationKey]
	if !exists {
		return nil, nil
	}
	declaration, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be a map of regions", backendDeclarationKey)
	}

	delete(doc, backendDeclarationKey)
	if generateJSON {
		data, err = json.MarshalIndent(doc, "", "  ")
	} else {
		data, err = yaml.Marshal(doc)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot encode assembled Terraform configuration: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return nil, err
	}
	return declaration, nil
}

// nothing back means the region simply isn't declared, the caller still fails if there's no backend at all
func generateBackend(declaration map[string]interface{}, region, environment string) (map[string]interface{}, error) {
	raw, exists := declaration[region]
	if !exists {
		return nil, nil
	}
	entry, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s.%s must be a map with at least a bucket", backendDeclarationKey, region)
	}
	if bucket, _ := entry["bucket"].(string); bucket == "" {
		return nil, fmt.Errorf("%s.%s.bucket is required", backendDeclarationKey, region)
	}

	key, err := backendKey(declaration, region, environment)
	if err != nil {
		return nil, err
	}

	backend := map[string]interface{}{"region": region, "encrypt": true}
	for name, value := range entry {
		backend[name] = value
	}
	backend["key"] = key
	return backend, nil
}

func backendKey(declaration map[string]interface{}, region, environment string) (string, error) {
	if raw, set := declaration["key"]; set {
		key, _ := raw.(string)
		if key == "" {
			return "", fmt.Errorf("%s.key must be a non-empty string", backendDeclarationKey)
		}
		return key, nil
	}

	pattern := defaultBackendKeyPattern
	if raw, set := declaration["key_pattern"]; set {
		pattern, _ = raw.(string)
		if pattern == "" {
			return "", fmt.Errorf("%s.key_pattern must be a non-empty string", backendDeclarationKey)
		}
	}

	vars := map[string]string{"region": region, "environment": environment}
	if raw, set := declaration["key_vars"]; set {
		declared, ok := raw.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("%s.key_vars must be a map of strings", backendDeclarationKey)
		}
		for name, value := range declared {
			text, ok := value.(string)
			if !ok {
				return "", fmt.Errorf("%s.key_vars.%s must be a string", backendDeclarationKey, name)
			}
			vars[name] = text
		}
	}

	missing := map[string]bool{}
	key := backendKeyPlaceholder.ReplaceAllStringFunc(pattern, func(placeholder string) string {
		name := placeholder[1 : len(placeholder)-1]
		if vars[name] == "" {
			missing[name] = true
		}
		return vars[name]
	})
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", fmt.Errorf("%s key pattern %q has no value for %s, set them in %s.key_vars or set %s.key",
			backendDeclarationKey, pattern, strings.Join(names, ", "), backendDeclarationKey, backendDeclarationKey)
	}
	return key, nil
}
