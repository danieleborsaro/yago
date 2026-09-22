package yaml

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// UpdateComponentVersion updates a component's version in a YAML file.
// This uses YAML path notation to navigate and update specific fields.
// Example path: "desiredstate.content.components.artifacts.web-app.docker.eu-west-1.tag"
func UpdateComponentVersion(yamlData map[string]interface{}, yamlPath, newVersion string) (bool, error) {
	if yamlPath == "" {
		return false, errors.NewParamError("YAML path must be specified")
	}

	logging.Debug("Updating YAML path '%s' to value '%s'", yamlPath, newVersion)

	// Parse the YAML path
	// Example: "desiredstate.content.components.sourcecode.api.tag"
	// Split by "." to get individual keys
	keys := parseYAMLPath(yamlPath)
	if len(keys) == 0 {
		return false, errors.New(errors.ErrParam, fmt.Sprintf("invalid YAML path: %s", yamlPath))
	}

	// Navigate to the parent of the target field
	current := yamlData
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]

		next, ok := current[key]
		if !ok {
			logging.Warn("YAML path key '%s' not found in path '%s'", key, yamlPath)
			return false, nil
		}

		nextMap, ok := next.(map[string]interface{})
		if !ok {
			logging.Warn("YAML path key '%s' is not a map in path '%s'", key, yamlPath)
			return false, nil
		}

		current = nextMap
	}

	// Update the final key
	finalKey := keys[len(keys)-1]
	oldValue, exists := current[finalKey]
	if !exists {
		logging.Warn("YAML path final key '%s' not found", finalKey)
		return false, nil
	}

	// Check if value actually changed
	if oldValue == newVersion {
		logging.Debug("Value unchanged for '%s': %v", yamlPath, newVersion)
		return false, nil
	}

	// Update the value
	current[finalKey] = newVersion
	logging.Debug("Updated '%s': '%v' -> '%s'", yamlPath, oldValue, newVersion)

	return true, nil
}

// SaveYAMLFile saves a YAML data structure to a file.
// This preserves the structure but may not preserve comments or formatting.
func SaveYAMLFile(filePath string, data map[string]interface{}, isDryRun bool) error {
	if filePath == "" {
		return errors.NewParamError("file path must be specified")
	}

	if isDryRun {
		logging.Info("[Dry-Run] Would save file: %s", filePath)
		return nil
	}

	logging.Debug("Saving YAML file: %s", filePath)

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to create directory: %s", dir)
	}

	// Marshal to YAML with 2-space indentation
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to marshal YAML data")
	}
	yamlBytes := buf.Bytes()

	// Write to file
	if err := os.WriteFile(filePath, yamlBytes, 0644); err != nil {
		return errors.Wrapf(errors.ErrFail, err, "failed to write YAML file: %s", filePath)
	}

	logging.Debug("Saved YAML file: %s (%d bytes)", filePath, len(yamlBytes))

	return nil
}

// parseYAMLPath splits a dot-separated YAML path into individual keys.
// Example: "desiredstate.content.components.tag" -> ["desiredstate", "content", "components", "tag"]
func parseYAMLPath(path string) []string {
	if path == "" {
		return nil
	}

	// Simple split by "."
	// Note: This doesn't handle escaped dots or array indices yet
	var keys []string
	current := ""

	for _, ch := range path {
		if ch == '.' {
			if current != "" {
				keys = append(keys, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		keys = append(keys, current)
	}

	return keys
}

// UpdateMultipleComponents updates multiple component versions in a YAML file.
// Returns the number of components actually updated.
func UpdateMultipleComponents(yamlData map[string]interface{}, updates map[string]string, isDryRun bool) (int, error) {
	updatedCount := 0

	for yamlPath, newVersion := range updates {
		if isDryRun {
			logging.Info("[Dry-Run] Would update '%s' to '%s'", yamlPath, newVersion)
			updatedCount++
			continue
		}

		wasUpdated, err := UpdateComponentVersion(yamlData, yamlPath, newVersion)
		if err != nil {
			return updatedCount, err
		}

		if wasUpdated {
			updatedCount++
		}
	}

	return updatedCount, nil
}

// BackupFile creates a backup of a file before modifying it.
// Returns the backup file path.
func BackupFile(filePath string, isDryRun bool) (string, error) {
	if isDryRun {
		backupPath := fmt.Sprintf("%s.backup", filePath)
		logging.Info("[Dry-Run] Would create backup: %s", backupPath)
		return backupPath, nil
	}

	// Read original file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to read file for backup: %s", filePath)
	}

	// Create backup with timestamp or .backup extension
	backupPath := fmt.Sprintf("%s.backup", filePath)

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to write backup file: %s", backupPath)
	}

	logging.Debug("Created backup: %s", backupPath)

	return backupPath, nil
}
