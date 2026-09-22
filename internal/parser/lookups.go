package parser

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
)

// getEnvValueLookup implements the gitops.getEnvValue lookup function
// Pattern: [[gitops.getEnvValue(ENV_VAR_NAME)]]
func (h *YAMLHandler) getEnvValueLookup(content map[string]interface{}, baseDir string, isInterpolation bool, helpers map[string]interface{}) error {
	yamlStr, err := h.ToString(content)
	if err != nil {
		return err
	}

	// Find all environment variable lookups
	re := regexp.MustCompile(`\[\[gitops\.getEnvValue\(([^)]+)\)\]\]`)
	matches := re.FindAllStringSubmatch(yamlStr, -1)

	for _, match := range matches {
		if len(match) != 2 {
			continue
		}

		fullMatch := match[0]
		envVarName := strings.TrimSpace(match[1])

		// Get environment variable value
		envValue := os.Getenv(envVarName)
		if envValue == "" {
			return errors.Newf(errors.ErrParse, "environment variable '%s' not found or empty", envVarName)
		}

		// Replace in content
		if err := h.replaceStringInContent(content, fullMatch, envValue); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to replace env lookup %s", fullMatch)
		}
	}

	return nil
}

// getYamlValueLookup implements the gitops.getYamlValue lookup function
// Patterns:
//   - [[gitops.getYamlValue(path.to.value)]]
//   - [[gitops.getYamlValue(path.to.value, path/to/file.yaml)]]
func (h *YAMLHandler) getYamlValueLookup(content map[string]interface{}, baseDir string, isInterpolation bool, helpers map[string]interface{}) error {
	yamlStr, err := h.ToString(content)
	if err != nil {
		return err
	}

	// Find all YAML value lookups with nested parentheses support
	re := regexp.MustCompile(`\[\[gitops\.getYamlValue\(([^)]+)\)\]\]`)
	matches := re.FindAllStringSubmatch(yamlStr, -1)

	for _, match := range matches {
		if len(match) != 2 {
			continue
		}

		fullMatch := match[0]
		argsStr := strings.TrimSpace(match[1])

		// Parse arguments (path and optional file)
		args := h.parseCommaArgs(argsStr)
		if len(args) == 0 {
			return errors.Newf(errors.ErrParse, "getYamlValue requires at least one argument: %s", fullMatch)
		}

		yamlPath := strings.TrimSpace(args[0])

		var value interface{}

		if len(args) >= 2 {
			// Load value from external file
			filePath := strings.TrimSpace(args[1])
			if !strings.HasPrefix(filePath, "/") {
				filePath = baseDir + "/" + filePath
			}

			// Load the external file
			externalContent, err := h.LoadFile(filePath, make(map[string]string))
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to load external file %s", filePath)
			}

			// Get value from external content
			value, err = h.GetValue(externalContent, yamlPath)
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to get value %s from file %s", yamlPath, filePath)
			}
		} else {
			// Get value from current content
			value, err = h.GetValue(content, yamlPath)
			if err != nil {
				return errors.Wrapf(errors.ErrParse, err, "failed to get value %s from current content", yamlPath)
			}
		}

		// Convert value to string for replacement
		valueStr := h.valueToString(value)

		// Replace in content
		if err := h.replaceStringInContent(content, fullMatch, valueStr); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to replace yaml lookup %s", fullMatch)
		}
	}

	return nil
}

// parseCommaArgs parses comma-separated arguments, handling nested structures
func (h *YAMLHandler) parseCommaArgs(argsStr string) []string {
	var args []string
	var current strings.Builder
	depth := 0
	inQuotes := false
	var quoteChar rune

	for _, char := range argsStr {
		switch char {
		case '"', '\'':
			if !inQuotes {
				inQuotes = true
				quoteChar = char
			} else if char == quoteChar {
				inQuotes = false
			}
			current.WriteRune(char)
		case '(':
			if !inQuotes {
				depth++
			}
			current.WriteRune(char)
		case ')':
			if !inQuotes {
				depth--
			}
			current.WriteRune(char)
		case ',':
			if !inQuotes && depth == 0 {
				args = append(args, current.String())
				current.Reset()
			} else {
				current.WriteRune(char)
			}
		default:
			current.WriteRune(char)
		}
	}

	// Add the last argument
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// valueToString converts any value to its string representation
func (h *YAMLHandler) valueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		// For complex types, marshal to YAML
		if yamlBytes, err := h.ToString(map[string]interface{}{"value": v}); err == nil {
			// Extract just the value part
			lines := strings.Split(yamlBytes, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "value: ") {
					return strings.TrimPrefix(line, "value: ")
				}
			}
		}
		// Fallback to fmt representation
		return fmt.Sprintf("%v", v)
	}
}

// replaceStringInContent replaces all occurrences of oldStr with newStr in the content
func (h *YAMLHandler) replaceStringInContent(content map[string]interface{}, oldStr, newStr string) error {
	return h.replaceInContentRecursive(content, oldStr, newStr)
}

// replaceInContentRecursive recursively replaces strings in the content structure
func (h *YAMLHandler) replaceInContentRecursive(value interface{}, oldStr, newStr string) error {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, val := range v {
			// Replace in the value
			if err := h.replaceInContentRecursive(val, oldStr, newStr); err != nil {
				return err
			}

			// If the value is a string, replace it directly
			if strVal, ok := val.(string); ok {
				if strings.Contains(strVal, oldStr) {
					v[key] = strings.ReplaceAll(strVal, oldStr, newStr)
				}
			}
		}
	case map[interface{}]interface{}:
		for key, val := range v {
			// Replace in the value
			if err := h.replaceInContentRecursive(val, oldStr, newStr); err != nil {
				return err
			}

			// If the value is a string, replace it directly
			if strVal, ok := val.(string); ok {
				if strings.Contains(strVal, oldStr) {
					v[key] = strings.ReplaceAll(strVal, oldStr, newStr)
				}
			}
		}
	case []interface{}:
		for i, val := range v {
			// Replace in the value
			if err := h.replaceInContentRecursive(val, oldStr, newStr); err != nil {
				return err
			}

			// If the value is a string, replace it directly
			if strVal, ok := val.(string); ok {
				if strings.Contains(strVal, oldStr) {
					v[i] = strings.ReplaceAll(strVal, oldStr, newStr)
				}
			}
		}
	case string:
		// This case is handled by the parent when it detects a string value
		return nil
	}

	return nil
}
