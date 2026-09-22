package parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/danieleborsaro/yago/internal/schema"
	"github.com/danieleborsaro/yago/internal/utils/errors"
	"gopkg.in/yaml.v3"
)

// marshalYAML serialises v to YAML using 2-space indentation.
func marshalYAML(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LookupFunction represents a custom lookup function
type LookupFunction func(content map[string]interface{}, baseDir string, isInterpolation bool, helpers map[string]interface{}) error

// YAMLHandler provides methods for handling YAML documents
type YAMLHandler struct {
	lookupFunctions map[string]LookupFunction
	baseDir         string
	isInterpolation bool
	schemaManager   *schema.SchemaManager
	// Track included files to detect circular dependencies
	includeStack []string
}

// NewYAMLHandler creates a new YAML handler instance.
// IMPORTANT: This always uses the main yago project's schema manager (auto-detected),
// NOT a schema manager for the baseDir. This ensures schemas are loaded only once
// from the main yago installation, not separately for each target directory.
func NewYAMLHandler(baseDir string) *YAMLHandler {
	// Always use the main yago schema manager, not one specific to baseDir
	// This prevents creating multiple schema manager instances and re-loading schemas
	schemaManager, err := schema.GetOrCreateSchemaManagerAuto()
	if err != nil {
		// Final fallback: nil schema manager (will need error handling)
		schemaManager = nil
	}

	handler := &YAMLHandler{
		lookupFunctions: make(map[string]LookupFunction),
		baseDir:         baseDir,
		isInterpolation: true,
		schemaManager:   schemaManager,
	}

	// Register default lookup functions
	handler.registerDefaultLookups()

	return handler
}

// NewYAMLHandlerWithSchemaManager creates a new YAML handler instance with an existing schema manager
func NewYAMLHandlerWithSchemaManager(baseDir string, schemaManager *schema.SchemaManager) *YAMLHandler {
	handler := &YAMLHandler{
		lookupFunctions: make(map[string]LookupFunction),
		baseDir:         baseDir,
		isInterpolation: true,
		schemaManager:   schemaManager,
	}

	// Register default lookup functions
	handler.registerDefaultLookups()

	return handler
}

// SetInterpolation enables or disables interpolation
func (h *YAMLHandler) SetInterpolation(enabled bool) {
	h.isInterpolation = enabled
}

// RegisterLookupFunction registers a custom lookup function
func (h *YAMLHandler) RegisterLookupFunction(name string, fn LookupFunction) {
	h.lookupFunctions[strings.ToUpper(name)] = fn
}

// LoadFile loads a single YAML file
func (h *YAMLHandler) LoadFile(filePath string, envVars map[string]string) (map[string]interface{}, error) {
	return h.LoadFiles([]string{filePath}, envVars)
}

// LoadFiles loads and merges multiple YAML files
func (h *YAMLHandler) LoadFiles(filePaths []string, envVars map[string]string) (map[string]interface{}, error) {
	var buffers []string

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Load individual files with !include support
	for _, filePath := range filePaths {
		buffer, err := h.loadSingleFileWithIncludes(filePath)
		if err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to load file %s", filePath)
		}
		buffers = append(buffers, buffer)
	}

	return h.LoadBuffers(buffers)
}

// LoadBuffers loads and merges multiple YAML buffers
func (h *YAMLHandler) LoadBuffers(buffers []string) (map[string]interface{}, error) {
	// Concatenate all buffers with document separators
	var combined strings.Builder
	for i, buffer := range buffers {
		if i > 0 {
			combined.WriteString("\n---\n")
		}
		// Ensure buffer starts with document separator
		if !strings.HasPrefix(strings.TrimSpace(buffer), "---") {
			combined.WriteString("---\n")
		}
		combined.WriteString(buffer)
	}

	// Parse the combined YAML stream
	content, err := h.parseYAMLStream(combined.String())
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "failed to parse YAML stream")
	}

	// Process lookups if interpolation is enabled
	if h.isInterpolation {
		if err := h.processLookups(content); err != nil {
			return nil, errors.Wrapf(errors.ErrParse, err, "failed to process lookups")
		}
	}

	return content, nil
}

// LoadString loads YAML from a string
func (h *YAMLHandler) LoadString(yamlString string) (map[string]interface{}, error) {
	return h.LoadBuffers([]string{yamlString})
}

// ToString converts a YAML object to string
func (h *YAMLHandler) ToString(content map[string]interface{}) (string, error) {
	data, err := marshalYAML(content)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to marshal YAML")
	}
	return string(data), nil
}

// ToFile writes YAML content to a file
func (h *YAMLHandler) ToFile(content map[string]interface{}, writer io.Writer) error {
	yamlStr, err := h.ToString(content)
	if err != nil {
		return err
	}

	// Add document separator
	if !strings.HasPrefix(yamlStr, "---") {
		yamlStr = "---\n" + yamlStr
	}

	if w, ok := writer.(io.StringWriter); ok {
		_, err = w.WriteString(yamlStr)
	} else {
		_, err = writer.Write([]byte(yamlStr))
	}
	return err
}

// loadSingleFile loads a single YAML file and returns its content as string
func (h *YAMLHandler) loadSingleFile(filePath string) (result string, err error) {
	file, openErr := os.Open(filePath)
	if openErr != nil {
		return "", openErr
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	// Parse and re-serialize to normalize the YAML
	var doc interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "invalid YAML in file %s", filePath)
	}

	normalized, err := marshalYAML(doc)
	if err != nil {
		return "", err
	}

	return string(normalized), nil
}

// processIncludeTags processes !include tags in YAML content
// This recursively loads included files and replaces the !include tags with their content
func (h *YAMLHandler) processIncludeTags(node *yaml.Node, baseDir string) error {
	if node == nil {
		return nil
	}

	// Handle !join: concatenate all child scalars into a single string.
	// e.g. !join ["Hello", " ", "World"] → "Hello World"
	// Alias nodes (e.g. *anchor_name) are resolved to their anchor's value.
	if node.Kind == yaml.SequenceNode && node.Tag == "!join" {
		var parts []string
		for _, child := range node.Content {
			resolved := child
			if child.Kind == yaml.AliasNode && child.Alias != nil {
				resolved = child.Alias
			}
			parts = append(parts, resolved.Value)
		}
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = strings.Join(parts, "")
		node.Content = nil
		return nil
	}

	// Check if this node is an !include tag
	if node.Kind == yaml.ScalarNode && node.Tag == "!include" {
		includePath := node.Value

		// Resolve relative to the CCI repo root (h.baseDir), not the including file's directory
		fullPath := includePath
		if !filepath.IsAbs(includePath) {
			fullPath = filepath.Join(h.baseDir, includePath)
		}

		// Normalize path
		fullPath = filepath.Clean(fullPath)

		// Check for circular dependencies
		for _, p := range h.includeStack {
			if p == fullPath {
				return errors.Newf(errors.ErrParse, "circular dependency detected: %s", fullPath)
			}
		}

		// Add to stack
		h.includeStack = append(h.includeStack, fullPath)
		defer func() {
			// Remove from stack when done
			h.includeStack = h.includeStack[:len(h.includeStack)-1]
		}()

		// Load the included file
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to load included file: %s", includePath)
		}

		// Parse the included content
		var includedNode yaml.Node
		if err := yaml.Unmarshal(data, &includedNode); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "invalid YAML in included file: %s", includePath)
		}

		// Process includes in the loaded content recursively (still using h.baseDir)
		if err := h.processIncludeTags(&includedNode, h.baseDir); err != nil {
			return err
		}

		// Replace the !include node with the content of the included file
		// The included file's document node content
		if includedNode.Kind == yaml.DocumentNode && len(includedNode.Content) > 0 {
			*node = *includedNode.Content[0]
		} else {
			*node = includedNode
		}

		return nil
	}

	// Recursively process child nodes
	for _, child := range node.Content {
		if err := h.processIncludeTags(child, baseDir); err != nil {
			return err
		}
	}

	return nil
}

// loadSingleFileWithIncludes loads a YAML file and processes !include tags
func (h *YAMLHandler) loadSingleFileWithIncludes(filePath string) (result string, err error) {
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to read file: %s", filePath)
	}

	// Parse as yaml.Node to preserve structure and tags
	var rootNode yaml.Node
	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "invalid YAML in file: %s", filePath)
	}

	// Process !include tags — paths always resolve relative to h.baseDir (CCI repo root)
	h.includeStack = []string{filepath.Clean(filePath)}
	if err := h.processIncludeTags(&rootNode, h.baseDir); err != nil {
		return "", err
	}
	h.includeStack = nil

	// Convert back to string
	processed, err := marshalYAML(&rootNode)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to marshal processed YAML")
	}

	return string(processed), nil
}

// parseYAMLStream parses a multi-document YAML stream and merges documents
func (h *YAMLHandler) parseYAMLStream(yamlStream string) (map[string]interface{}, error) {
	decoder := yaml.NewDecoder(strings.NewReader(yamlStream))

	var merged map[string]interface{}

	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if doc == nil {
			continue
		}

		if merged == nil {
			merged = doc
		} else {
			merged = h.mergeMaps(merged, doc)
		}
	}

	if merged == nil {
		merged = make(map[string]interface{})
	}

	return merged, nil
}

// mergeMaps recursively merges two maps, with values from the second map taking precedence
func (h *YAMLHandler) mergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		return src
	}
	if src == nil {
		return dst
	}

	result := make(map[string]interface{})

	// Copy all values from dst
	for k, v := range dst {
		result[k] = v
	}

	// Merge values from src
	for k, v := range src {
		if dstVal, exists := result[k]; exists {
			// If both values are maps, merge them recursively
			if dstMap, dstOk := dstVal.(map[string]interface{}); dstOk {
				if srcMap, srcOk := v.(map[string]interface{}); srcOk {
					result[k] = h.mergeMaps(dstMap, srcMap)
					continue
				}
			}
		}
		// Otherwise, src value takes precedence
		result[k] = v
	}

	return result
}

// processLookups processes all custom lookup functions in the content
func (h *YAMLHandler) processLookups(content map[string]interface{}) error {
	// Convert to string to find all lookups
	yamlStr, err := h.ToString(content)
	if err != nil {
		return err
	}

	// Find all [[...]] patterns
	lookups := h.findInnermostLookups(yamlStr)

	for _, lookup := range lookups {
		if err := h.processLookup(content, lookup); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to process lookup %s", lookup)
		}
	}

	return nil
}

// findInnermostLookups finds all innermost [[...]] patterns in a string
func (h *YAMLHandler) findInnermostLookups(buffer string) []string {
	// Use regex to find all [[...]] patterns
	re := regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)
	matches := re.FindAllString(buffer, -1)

	var innermost []string
	for _, match := range matches {
		// Check if this match doesn't contain nested [[ or ]]
		content := match[2 : len(match)-2] // Remove [[ and ]]
		if !strings.Contains(content, "[[") && !strings.Contains(content, "]]") {
			innermost = append(innermost, match)
		}
	}

	return innermost
}

// processLookup processes a single lookup pattern
func (h *YAMLHandler) processLookup(content map[string]interface{}, lookup string) error {
	// Extract function name from lookup pattern
	// Pattern: [[functionName(args)]]
	inner := lookup[2 : len(lookup)-2] // Remove [[ and ]]

	// Parse function name and arguments
	re := regexp.MustCompile(`^([^(]+)\((.*)\)$`)
	matches := re.FindStringSubmatch(inner)
	if len(matches) != 3 {
		return errors.Newf(errors.ErrParse, "invalid lookup pattern: %s", lookup)
	}

	functionName := strings.ToUpper(strings.TrimSpace(matches[1]))

	// Find the lookup function
	lookupFn, exists := h.lookupFunctions[functionName]
	if !exists {
		return errors.Newf(errors.ErrParam, "unknown lookup function: %s", functionName)
	}

	// Execute the lookup function
	helpers := map[string]interface{}{
		"lookup":    lookup,
		"arguments": strings.TrimSpace(matches[2]),
	}

	return lookupFn(content, h.baseDir, h.isInterpolation, helpers)
}

// registerDefaultLookups registers the default lookup functions
func (h *YAMLHandler) registerDefaultLookups() {
	h.RegisterLookupFunction("gitops.getEnvValue", h.getEnvValueLookup)
	h.RegisterLookupFunction("gitops.getYamlValue", h.getYamlValueLookup)
	// Note: AWS Secrets Manager lookup would be implemented separately
}

// GetValue retrieves a value from the content using a dot-separated path
func (h *YAMLHandler) GetValue(content map[string]interface{}, path string) (interface{}, error) {
	if path == "" || path == "/" {
		return content, nil
	}

	parts := strings.Split(path, ".")
	current := interface{}(content)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, errors.Newf(errors.ErrParam, "key '%s' not found in path '%s'", part, path)
			}
		case map[interface{}]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, errors.Newf(errors.ErrParam, "key '%s' not found in path '%s'", part, path)
			}
		default:
			return nil, errors.Newf(errors.ErrParse, "cannot navigate into non-map value at '%s' in path '%s'", part, path)
		}
	}

	return current, nil
}

// UpdateValue updates a value in the content using a dot-separated path
func (h *YAMLHandler) UpdateValue(content map[string]interface{}, path string, value interface{}) error {
	if path == "" {
		return errors.New(errors.ErrParam, "empty path not supported for updates")
	}

	parts := strings.Split(path, ".")
	current := content

	// Navigate to the parent of the target key
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]

		if next, exists := current[part]; exists {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return errors.Newf(errors.ErrParse, "cannot navigate into non-map value at '%s'", part)
			}
		} else {
			// Create intermediate maps as needed
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}

	// Set the final value
	finalKey := parts[len(parts)-1]
	current[finalKey] = value

	return nil
}

// FindValueByKey finds all paths in the content that contain a specific key name.
// It returns a slice of dot-separated paths to all occurrences of the key.
//
// Example:
//
//	content := map[string]interface{}{
//	    "deployment": map[string]interface{}{
//	        "spec": map[string]interface{}{
//	            "template": map[string]interface{}{
//	                "spec": map[string]interface{}{
//	                    "containers": []interface{}{
//	                        map[string]interface{}{"image": "nginx:1.19", "name": "web"},
//	                        map[string]interface{}{"image": "redis:6", "name": "cache"},
//	                    },
//	                },
//	            },
//	        },
//	    },
//	}
//	paths := handler.FindValueByKey(content, "image")
//	// Returns: ["deployment.spec.template.spec.containers[0].image",
//	//           "deployment.spec.template.spec.containers[1].image"]
func (h *YAMLHandler) FindValueByKey(content map[string]interface{}, key string) []string {
	var paths []string
	h.findValueByKeyRecursive(content, key, "", &paths)
	return paths
}

// findValueByKeyRecursive recursively searches for all paths containing the specified key
func (h *YAMLHandler) findValueByKeyRecursive(
	content map[string]interface{},
	targetKey string,
	currentPath string,
	paths *[]string,
) {
	for k, v := range content {
		// Build the current path
		path := k
		if currentPath != "" {
			path = currentPath + "." + k
		}

		// Check if this key matches the target
		if k == targetKey {
			*paths = append(*paths, path)
		}

		// Recurse into nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			h.findValueByKeyRecursive(nestedMap, targetKey, path, paths)
		}

		// Recurse into arrays
		if arr, ok := v.([]interface{}); ok {
			for i, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arrayPath := path + "[" + fmt.Sprintf("%d", i) + "]"
					h.findValueByKeyRecursive(itemMap, targetKey, arrayPath, paths)
				}
			}
		}
	}
}

// FindKeyByValue finds all paths in the content where values match the specified pattern.
// It supports both exact and partial (substring) matching.
//
// Parameters:
//   - content: The YAML content to search
//   - pattern: The value pattern to search for
//   - isPartialMatch: If true, matches substrings; if false, requires exact match
//
// Returns a slice of dot-separated paths where the value was found.
//
// Example:
//
//	content := map[string]interface{}{
//	    "deployment": map[string]interface{}{
//	        "image": "nginx:1.19",
//	        "sidecar": map[string]interface{}{
//	            "image": "nginx:1.19",
//	        },
//	    },
//	}
//	paths := handler.FindKeyByValue(content, "nginx:1.19", false)
//	// Returns: ["deployment.image", "deployment.sidecar.image"]
func (h *YAMLHandler) FindKeyByValue(content map[string]interface{}, pattern string, isPartialMatch bool) []string {
	var paths []string
	h.findKeyByValueRecursive(content, pattern, isPartialMatch, "", &paths)
	return paths
}

// findKeyByValueRecursive recursively searches for all paths containing the specified value
func (h *YAMLHandler) findKeyByValueRecursive(
	content interface{},
	pattern string,
	isPartialMatch bool,
	currentPath string,
	paths *[]string,
) {
	switch v := content.(type) {
	case map[string]interface{}:
		// Recurse into map
		for k, val := range v {
			path := k
			if currentPath != "" {
				path = currentPath + "." + k
			}
			h.findKeyByValueRecursive(val, pattern, isPartialMatch, path, paths)
		}

	case map[interface{}]interface{}:
		// Handle map[interface{}]interface{} (some YAML parsers produce this)
		for k, val := range v {
			keyStr := fmt.Sprintf("%v", k)
			path := keyStr
			if currentPath != "" {
				path = currentPath + "." + keyStr
			}
			h.findKeyByValueRecursive(val, pattern, isPartialMatch, path, paths)
		}

	case []interface{}:
		// Recurse into array
		for i, item := range v {
			arrayPath := currentPath + "[" + fmt.Sprintf("%d", i) + "]"
			h.findKeyByValueRecursive(item, pattern, isPartialMatch, arrayPath, paths)
		}

	default:
		// Leaf node - check if value matches
		valueStr := fmt.Sprintf("%v", v)

		var matches bool
		if isPartialMatch {
			// Substring match (case-insensitive)
			matches = strings.Contains(strings.ToLower(valueStr), strings.ToLower(pattern))
		} else {
			// Exact match (case-insensitive)
			matches = strings.EqualFold(valueStr, pattern)
		}

		if matches {
			*paths = append(*paths, currentPath)
		}
	}
}

// ReplaceInContent replaces all occurrences of oldValue with newValue in the content
func (h *YAMLHandler) ReplaceInContent(content map[string]interface{}, oldValue, newValue string) error {
	return h.replaceInValue(content, oldValue, newValue)
}

// replaceInValue recursively replaces values in any data structure
func (h *YAMLHandler) replaceInValue(value interface{}, oldValue, newValue string) error {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, val := range v {
			if err := h.replaceInValue(val, oldValue, newValue); err != nil {
				return err
			}
		}
	case map[interface{}]interface{}:
		for _, val := range v {
			if err := h.replaceInValue(val, oldValue, newValue); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, val := range v {
			if err := h.replaceInValue(val, oldValue, newValue); err != nil {
				return err
			}
		}
	case string:
		if parent, ok := value.(*string); ok {
			*parent = strings.ReplaceAll(*parent, oldValue, newValue)
		}
	}

	return nil
}

// ValidateSchema validates content against a specific schema version and type
func (h *YAMLHandler) ValidateSchema(content map[string]interface{}, version schema.SchemaVersion, schemaType schema.SchemaType) error {
	if h.schemaManager == nil {
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}
	return h.schemaManager.ValidateSchema(content, version, schemaType)
}

// ValidateSchemaAutoDetect validates content with auto-detected schema version
func (h *YAMLHandler) ValidateSchemaAutoDetect(content map[string]interface{}, schemaType schema.SchemaType) error {
	if h.schemaManager == nil {
		return errors.New(errors.ErrFail, "schema manager not initialized")
	}
	return h.schemaManager.ValidateSchemaAutoDetect(content, schemaType)
}

// DetectSchemaVersion detects the schema version from content
func (h *YAMLHandler) DetectSchemaVersion(content map[string]interface{}) (schema.SchemaVersion, error) {
	if h.schemaManager == nil {
		return "", errors.New(errors.ErrFail, "schema manager not initialized")
	}

	version, err := h.schemaManager.DetectSchemaVersion(content)
	if err != nil {
		return "", err
	}

	return schema.SchemaVersion(version), nil
}

// GetSupportedSchemaVersions returns all supported schema versions
func (h *YAMLHandler) GetSupportedSchemaVersions() []schema.SchemaVersion {
	if h.schemaManager == nil {
		return []schema.SchemaVersion{}
	}
	return h.schemaManager.GetSupportedSchemaVersions()
}

// GetSchemaContent returns the JSON schema definition for a specific version and schema type
// Defaults to "yago" namespace for backward compatibility
func (h *YAMLHandler) GetSchemaContent(version schema.SchemaVersion, schemaType schema.SchemaType) (map[string]interface{}, error) {
	if h.schemaManager == nil {
		return nil, errors.New(errors.ErrFail, "schema manager not initialized")
	}
	return h.schemaManager.GetSchemaContent("", version, schemaType) // Empty string defaults to "yago"
}

// GetPropertyPaths retrieves the property paths for a specific schema version and document type
// This is used to get schema-defined paths for parsing documents
// Uses default "yago" namespace - for custom namespaces use GetPropertyPathsWithNamespace
func (h *YAMLHandler) GetPropertyPaths(version schema.SchemaVersion, isDesiredState bool) (*schema.PropertyPaths, error) {
	if h.schemaManager == nil {
		return nil, errors.New(errors.ErrFail, "schema manager not initialized")
	}
	return h.schemaManager.DiscoverPropertyPaths("", version, isDesiredState)
}

// GetPropertyPathsWithNamespace retrieves the property paths for a specific namespace, schema version and document type
// This is used when the namespace is known from the document content
func (h *YAMLHandler) GetPropertyPathsWithNamespace(namespace string, version schema.SchemaVersion, isDesiredState bool) (*schema.PropertyPaths, error) {
	if h.schemaManager == nil {
		return nil, errors.New(errors.ErrFail, "schema manager not initialized")
	}
	return h.schemaManager.DiscoverPropertyPaths(namespace, version, isDesiredState)
}

// Repo field path methods - these delegate to the schema's PropertyPaths
// They provide a convenient API for getting schema-defined repository field paths

// GetPathToRepoUrl returns the full path to a repository's URL field
// repoPath is the path to the repo object (e.g., "metadata.repo")
// Returns the complete path to the URL field (e.g., "metadata.repo.url")
func (h *YAMLHandler) GetPathToRepoUrl(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoUrlPath(repoPath), nil
}

// GetPathToRepoRef returns the full path to a repository's ref field
func (h *YAMLHandler) GetPathToRepoRef(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoRefPath(repoPath), nil
}

// GetPathToRepoTag returns the full path to a repository's tag field
func (h *YAMLHandler) GetPathToRepoTag(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoTagPath(repoPath), nil
}

// GetPathToRepoBranch returns the full path to a repository's branch field
func (h *YAMLHandler) GetPathToRepoBranch(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoBranchPath(repoPath), nil
}

// GetPathToRepoPath returns the full path to a repository's path field (subdirectory)
func (h *YAMLHandler) GetPathToRepoPath(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoPathPath(repoPath), nil
}

// GetPathToRepoWatch returns the full path to a repository's watch list field
func (h *YAMLHandler) GetPathToRepoWatch(repoPath string, version schema.SchemaVersion, isDesiredState bool) (string, error) {
	paths, err := h.GetPropertyPaths(version, isDesiredState)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParse, err, "failed to get property paths")
	}
	return paths.GetRepoWatchPath(repoPath), nil
}

// ApplyJSONPatch applies RFC 6902 JSON patch operations to YAML content
// This is a convenience method that uses the JSONPatchEngine to apply patches.
// Individual patch operations are applied sequentially.
// Returns the patched content or error if any patch fails.
//
// Example:
//
//	patches := []JSONPatchOperation{
//	    {Op: "replace", Path: "/metadata/version", Value: "2.0"},
//	    {Op: "add", Path: "/spec/replicas", Value: 3},
//	}
//	result, err := handler.ApplyJSONPatch(content, patches)
func (h *YAMLHandler) ApplyJSONPatch(content map[string]interface{}, patches []JSONPatchOperation) (map[string]interface{}, error) {
	engine := NewJSONPatchEngine()
	return engine.Apply(content, patches)
}

// ApplyJSONPatchStrict applies RFC 6902 JSON patch operations to YAML content atomically.
// This is a convenience method that uses the JSONPatchEngine in strict (atomic) mode.
// All patches are validated before any are applied; if any fails, none are applied.
// This ensures consistency and prevents partial updates.
// Returns the patched content or error without modifying the original.
//
// Example:
//
//	patches := []JSONPatchOperation{
//	    {Op: "test", Path: "/metadata/version", Value: "1.0"},
//	    {Op: "replace", Path: "/metadata/version", Value: "2.0"},
//	}
//	result, err := handler.ApplyJSONPatchStrict(content, patches)
//	// Either all patches apply or none do (atomic guarantee)
func (h *YAMLHandler) ApplyJSONPatchStrict(content map[string]interface{}, patches []JSONPatchOperation) (map[string]interface{}, error) {
	engine := NewJSONPatchEngine()
	return engine.ApplyStrict(content, patches)
}

// TestJSONPatchValue validates that a value exists and matches at a given JSON Pointer path.
// This is a convenience method for the "test" operation in RFC 6902.
// Returns true if the value matches, false otherwise.
//
// Example:
//
//	if handler.TestJSONPatchValue(content, "/metadata/version", "1.0") {
//	    // Version is 1.0, safe to proceed with upgrade
//	}
func (h *YAMLHandler) TestJSONPatchValue(content map[string]interface{}, path string, expectedValue interface{}) bool {
	engine := NewJSONPatchEngine()
	return engine.Test(content, path, expectedValue)
}

// StrategicMerge performs a field-aware merge of source into base using configured strategies.
// This enables intelligent merging that prevents data loss through specific merge strategies per path.
// Returns a new merged object without modifying the originals.
//
// Merge strategies:
//
//	REPLACE (default): Source values override base values (standard deep merge)
//	APPEND: Arrays are appended instead of replaced
//	UNION: Union of array values (unique only, no duplicates)
//	MERGE_BY_KEY:fieldname: Arrays are merged by matching a key field
//
// Example:
//
//	config := StrategicMergeConfig{
//	    "/spec/containers": "MERGE_BY_KEY:name",
//	    "/spec/volumes": "APPEND",
//	    "/labels": "UNION",
//	}
//	engine := NewStrategicMergeEngine(config)
//	result, err := handler.StrategicMerge(base, source, engine)
func (h *YAMLHandler) StrategicMerge(base map[string]interface{}, source map[string]interface{}, engine *StrategicMergeEngine) (map[string]interface{}, error) {
	if engine == nil {
		engine = NewStrategicMergeEngine(nil)
	}
	return engine.Merge(base, source)
}

// StrategicMergeWithConfig performs strategic merge with inline configuration.
// Convenience method that creates a StrategicMergeEngine from config and performs merge.
//
// Example:
//
//	result, err := handler.StrategicMergeWithConfig(base, source, StrategicMergeConfig{
//	    "/spec/containers": "MERGE_BY_KEY:name",
//	    "/env": "APPEND",
//	})
func (h *YAMLHandler) StrategicMergeWithConfig(base map[string]interface{}, source map[string]interface{}, config StrategicMergeConfig) (map[string]interface{}, error) {
	engine := NewStrategicMergeEngine(config)
	return engine.Merge(base, source)
}
