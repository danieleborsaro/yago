package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SecretVariableBinding holds references only; secret payloads are resolved when Terraform runs.
type SecretVariableBinding struct {
	SecretID     string `json:"secret_id" yaml:"secret_id"`
	JSONKey      string `json:"json_key,omitempty" yaml:"json_key,omitempty"`
	VersionID    string `json:"version_id,omitempty" yaml:"version_id,omitempty"`
	VersionStage string `json:"version_stage,omitempty" yaml:"version_stage,omitempty"`
}

type secretValueReader interface {
	Read(name, version, stage string) (string, error)
}

func secretBindingNames(bindings map[string]SecretVariableBinding) []string {
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validSecretVariableName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func validateSecretBindings(bindings map[string]SecretVariableBinding) error {
	for _, name := range secretBindingNames(bindings) {
		if !validSecretVariableName(name) {
			return fmt.Errorf("secret binding has an invalid Terraform variable name")
		}
		binding := bindings[name]
		if strings.TrimSpace(binding.SecretID) == "" {
			return fmt.Errorf("secret binding for Terraform variable %q requires secret_id", name)
		}
		if binding.VersionID != "" && binding.VersionStage != "" {
			return fmt.Errorf("secret binding for Terraform variable %q cannot specify both version_id and version_stage", name)
		}
	}
	return nil
}

func resolveSecretVariables(bindings map[string]SecretVariableBinding, reader secretValueReader) (map[string]string, error) {
	if err := validateSecretBindings(bindings); err != nil {
		return nil, err
	}
	env := make(map[string]string, len(bindings))
	if len(bindings) == 0 {
		return env, nil
	}
	if reader == nil {
		return nil, fmt.Errorf("secret value reader is not configured")
	}

	type secretVersion struct {
		id, version, stage string
	}
	cache := make(map[secretVersion]string)
	for _, name := range secretBindingNames(bindings) {
		binding := bindings[name]
		key := secretVersion{binding.SecretID, binding.VersionID, binding.VersionStage}
		value, cached := cache[key]
		if !cached {
			var err error
			value, err = reader.Read(binding.SecretID, binding.VersionID, binding.VersionStage)
			if err != nil {
				// Reader errors may contain secret values, so do not wrap or log them.
				return nil, fmt.Errorf("failed to read secret for Terraform variable %q", name)
			}
			cache[key] = value
		}
		if binding.JSONKey != "" {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(value), &fields); err != nil || fields == nil {
				return nil, fmt.Errorf("secret for Terraform variable %q must be a JSON object", name)
			}
			field, found := fields[binding.JSONKey]
			if !found || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return nil, fmt.Errorf("secret for Terraform variable %q has a missing or null JSON field", name)
			}
			if len(field) > 0 && field[0] == '"' {
				if err := json.Unmarshal(field, &value); err != nil {
					return nil, fmt.Errorf("secret for Terraform variable %q has an invalid JSON string field", name)
				}
			} else {
				var compact bytes.Buffer
				if err := json.Compact(&compact, field); err != nil {
					return nil, fmt.Errorf("secret for Terraform variable %q has an invalid JSON field", name)
				}
				value = compact.String()
			}
		}
		if strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("secret for Terraform variable %q contains a NUL byte that cannot be passed through the environment", name)
		}
		env["TF_VAR_"+name] = value
	}
	return env, nil
}

func redactSecretOutput(output string, env map[string]string) string {
	values := make(map[string]struct{})
	add := func(value string) {
		if value == "" {
			return
		}
		values[value] = struct{}{}
		// Terraform may render strings with JSON escapes rather than literal bytes.
		encoded, err := json.Marshal(value)
		if err == nil && len(encoded) > 2 {
			values[string(encoded[1:len(encoded)-1])] = struct{}{}
		}
	}
	var collectStrings func(interface{})
	collectStrings = func(value interface{}) {
		switch v := value.(type) {
		case string:
			add(v)
		case []interface{}:
			for _, item := range v {
				collectStrings(item)
			}
		case map[string]interface{}:
			for _, item := range v {
				collectStrings(item)
			}
		}
	}
	for _, value := range env {
		add(value)
		var decoded interface{}
		if json.Unmarshal([]byte(value), &decoded) == nil {
			collectStrings(decoded)
		}
	}
	if len(values) == 0 {
		return output
	}

	ordered := make([]string, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i]) != len(ordered[j]) {
			return len(ordered[i]) > len(ordered[j])
		}
		return ordered[i] < ordered[j]
	})
	replacements := make([]string, 0, len(ordered)*2)
	for _, value := range ordered {
		replacements = append(replacements, value, "[REDACTED]")
	}
	return strings.NewReplacer(replacements...).Replace(output)
}
