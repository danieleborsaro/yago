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
			return fmt.Errorf("secret binding has an invalid Terraform variable name: %q", name)
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
				// Secrets Manager errors never contain secret values. The JSON errors below can, so they aren't wrapped.
				return nil, fmt.Errorf("failed to read secret for Terraform variable %q: %w", name, err)
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

// Shorter lines of a multiline secret only get redacted when they're a whole line of output
const minRedactedLineLength = 4

type secretRedactor struct {
	replacer *strings.Replacer
	lines    map[string]struct{}
}

func (r *secretRedactor) redact(output string) string {
	if r.replacer == nil {
		return output
	}
	output = r.replacer.Replace(output)
	var rebuilt strings.Builder
	changed := false
	copied, offset := 0, 0
	for line := range strings.SplitAfterSeq(output, "\n") {
		if redacted := r.redactWholeLine(line); redacted != line {
			if !changed {
				rebuilt.Grow(len(output))
				changed = true
			}
			rebuilt.WriteString(output[copied:offset])
			rebuilt.WriteString(redacted)
			copied = offset + len(line)
		}
		offset += len(line)
	}
	if !changed {
		return output
	}
	rebuilt.WriteString(output[copied:])
	return rebuilt.String()
}

func (r *secretRedactor) redactWholeLine(line string) string {
	body := strings.TrimRight(line, " \t\r\n")
	start := len(body) - len(strings.TrimLeft(body, " \t"))
	// A secret's own line can start with what looks like a diff marker, so try the whole line first
	if _, found := r.lines[body[start:]]; found {
		return line[:start] + "[REDACTED]" + line[len(body):]
	}
	if content := body[start:]; len(content) > 2 && strings.ContainsRune("+-~", rune(content[0])) && (content[1] == ' ' || content[1] == '\t') {
		start += 1 + len(content[1:]) - len(strings.TrimLeft(content[1:], " \t"))
		if _, found := r.lines[body[start:]]; found {
			return line[:start] + "[REDACTED]" + line[len(body):]
		}
	}
	return line
}

func newSecretRedactor(env map[string]string) *secretRedactor {
	values := make(map[string]struct{})
	lines := make(map[string]struct{})
	var add func(string)
	add = func(value string) {
		if value == "" {
			return
		}
		values[value] = struct{}{}
		// Terraform can print strings JSON escaped, with or without &, < and > escaped too
		for _, escapeHTML := range []bool{true, false} {
			var encoded bytes.Buffer
			encoder := json.NewEncoder(&encoded)
			encoder.SetEscapeHTML(escapeHTML)
			if encoder.Encode(value) == nil {
				if quoted := strings.TrimSuffix(encoded.String(), "\n"); len(quoted) > 2 {
					values[quoted[1:len(quoted)-1]] = struct{}{}
				}
			}
		}
		// Terraform prints multiline strings one indented line at a time
		if strings.Contains(value, "\n") {
			for _, line := range strings.Split(value, "\n") {
				if line = strings.TrimSpace(line); len(line) >= minRedactedLineLength {
					add(line)
				} else if line != "" {
					lines[line] = struct{}{}
				}
			}
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
	redactor := &secretRedactor{lines: lines}
	if len(values) == 0 {
		return redactor
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
	redactor.replacer = strings.NewReplacer(replacements...)
	return redactor
}
