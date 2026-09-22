package desiredstate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// ComponentParser handles parsing of components from desiredstate YAML files.
type ComponentParser struct {
	manager              *ComponentManager
	isResolveToCommit    bool
	isFailOnUnableToLock bool
}

// NewComponentParser creates a new component parser.
func NewComponentParser(isResolveToCommit, isFailOnUnableToLock bool) *ComponentParser {
	return &ComponentParser{
		manager:              NewComponentManager(),
		isResolveToCommit:    isResolveToCommit,
		isFailOnUnableToLock: isFailOnUnableToLock,
	}
}

// GetManager returns the component manager.
func (cp *ComponentParser) GetManager() *ComponentManager {
	return cp.manager
}

// ParseFromYAML parses components from a desiredstate YAML structure.
func (cp *ComponentParser) ParseFromYAML(yamlData map[string]interface{}, partFilePath string) error {
	logging.Debug("ParseFromYAML called with partFilePath: %s", partFilePath)

	// Navigate to desiredstate.content.components
	desiredstate, ok := yamlData["desiredstate"].(map[string]interface{})
	if !ok {
		logging.Debug("No desiredstate section found, checking for components at root level")
		// No desiredstate section - might be a parts file
		return cp.parseComponentsSection(yamlData, partFilePath)
	}

	logging.Debug("Found desiredstate section")
	content, ok := desiredstate["content"].(map[string]interface{})
	if !ok {
		logging.Debug("No content section found in desiredstate")
		// No content section
		return nil
	}

	logging.Debug("Found content section, parsing components")
	return cp.parseComponentsSection(content, partFilePath)
}

// parseComponentsSection parses the components section of a YAML file.
func (cp *ComponentParser) parseComponentsSection(content map[string]interface{}, partFilePath string) error {
	components, ok := content["components"].(map[string]interface{})
	if !ok {
		logging.Debug("No components section found in content")
		// No components section
		return nil
	}

	logging.Debug("Found components section with %d keys", len(components))

	// Parse sourcecode components
	if sourcecode, ok := components["sourcecode"].(map[string]interface{}); ok {
		logging.Debug("Found sourcecode section with %d components", len(sourcecode))
		if err := cp.parseSourcecodeComponents(sourcecode, partFilePath); err != nil {
			return err
		}
	} else {
		logging.Debug("No sourcecode section found")
	}

	// Parse artifacts components
	if artifacts, ok := components["artifacts"].(map[string]interface{}); ok {
		logging.Debug("Found artifacts section with %d artifacts", len(artifacts))
		if err := cp.parseArtifactsComponents(artifacts, partFilePath); err != nil {
			return err
		}
	} else {
		logging.Debug("No artifacts section found")
	}

	return nil
}

// parseSourcecodeComponents parses sourcecode components (Git repositories).
func (cp *ComponentParser) parseSourcecodeComponents(sourcecode map[string]interface{}, partFilePath string) error {
	for componentID, componentData := range sourcecode {
		data, ok := componentData.(map[string]interface{})
		if !ok {
			logging.Warn("Invalid sourcecode component data for %s", componentID)
			continue
		}

		component := &VersionedComponent{
			PartFile:             partFilePath,
			PartID:               fmt.Sprintf("sourcecode.%s", componentID),
			Type:                 ComponentTypeSourcecode,
			IsResolveToCommit:    cp.isResolveToCommit,
			IsFailOnUnableToLock: cp.isFailOnUnableToLock,
			VersionSeparator:     ":",
			LatestIdentifier:     "latest",
			IsVersioned:          true,
		}

		// Extract Git fields
		if url, ok := data["url"].(string); ok {
			component.URL = url
		}
		if branch, ok := data["branch"].(string); ok {
			component.Branch = branch
			component.Version = branch
			// Branch references are unlocked by default
			component.IsLocked = (branch == "")
		}
		if tag, ok := data["tag"].(string); ok {
			component.Tag = tag
			if tag != "" {
				component.Version = tag
				// Tags are considered locked
				component.IsLocked = true
			}
		}
		if path, ok := data["path"].(string); ok {
			component.Path = path
		}

		// Set YAML paths for locking/unlocking
		component.YAMLPathToLock = fmt.Sprintf("desiredstate.content.components.sourcecode.%s.tag", componentID)
		component.YAMLPathToUnlock = fmt.Sprintf("desiredstate.content.components.sourcecode.%s.branch", componentID)

		if err := cp.manager.AddComponent(component); err != nil {
			return err
		}

		logging.Debug("Parsed sourcecode component: %s (branch: %s, tag: %s, locked: %t)",
			componentID, component.Branch, component.Tag, component.IsLocked)
	}

	return nil
}

// parseArtifactsComponents parses artifact components (Docker images, S3 objects, etc.).
func (cp *ComponentParser) parseArtifactsComponents(artifacts map[string]interface{}, partFilePath string) error {
	for artifactID, artifactData := range artifacts {
		data, ok := artifactData.(map[string]interface{})
		if !ok {
			logging.Warn("Invalid artifact component data for %s", artifactID)
			continue
		}

		// Check if this is a Docker artifact
		if docker, ok := data["docker"].(map[string]interface{}); ok {
			if err := cp.parseDockerArtifact(artifactID, docker, partFilePath); err != nil {
				return err
			}
			continue
		}

		// Check if this is an S3 artifact
		if s3, ok := data["s3"].(map[string]interface{}); ok {
			if err := cp.parseS3Artifact(artifactID, s3, partFilePath); err != nil {
				return err
			}
			continue
		}
	}

	return nil
}

// parseDockerArtifact parses Docker image artifacts (multi-region) using the component handler.
func (cp *ComponentParser) parseDockerArtifact(artifactID string, docker map[string]interface{}, partFilePath string) error {
	// Get the Docker handler from the registry
	handler, err := GetComponentHandler(ComponentTypeDocker)
	if err != nil {
		return errors.Wrapf(errors.ErrParse, err, "failed to get Docker component handler")
	}

	// Docker artifacts are typically multi-region
	for region, regionData := range docker {
		data, ok := regionData.(map[string]interface{})
		if !ok {
			logging.Warn("Invalid Docker region data for %s/%s", artifactID, region)
			continue
		}

		// Build the part ID for this Docker component
		partID := fmt.Sprintf("artifacts.%s.docker.%s", artifactID, region)

		// Use the handler to parse the component
		component, err := handler.ParseComponent(data, partID, partFilePath)
		if err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to parse Docker component %s", partID)
		}

		// Set additional parser-specific fields
		component.Region = region
		component.IsResolveToCommit = cp.isResolveToCommit
		component.IsFailOnUnableToLock = cp.isFailOnUnableToLock

		// Set YAML paths for locking/unlocking
		component.YAMLPathToLock = fmt.Sprintf("desiredstate.content.components.artifacts.%s.docker.%s.tag", artifactID, region)
		component.YAMLPathToUnlock = component.YAMLPathToLock

		if err := cp.manager.AddComponent(component); err != nil {
			return err
		}

		logging.Debug("Parsed Docker component: %s/%s (image: %s, tag: %s, locked: %t)",
			artifactID, region, component.URL, component.Version, component.IsLocked)
	}

	return nil
}

// parseS3Artifact parses S3 object artifacts (multi-region).
func (cp *ComponentParser) parseS3Artifact(artifactID string, s3 map[string]interface{}, partFilePath string) error {
	// Check if this is region-based S3 (similar to Docker multi-region structure)
	// If the first level contains region names, iterate through them
	hasRegions := false
	for _, value := range s3 {
		if _, ok := value.(map[string]interface{}); ok {
			// This looks like a region-based structure
			hasRegions = true
			break
		}
	}

	if hasRegions {
		// Region-based S3 artifact structure (like Docker)
		for region, regionData := range s3 {
			data, ok := regionData.(map[string]interface{})
			if !ok {
				logging.Warn("Invalid S3 region data for %s/%s", artifactID, region)
				continue
			}

			component := &VersionedComponent{
				PartFile:             partFilePath,
				PartID:               fmt.Sprintf("artifacts.%s.s3.%s", artifactID, region),
				Type:                 ComponentTypeS3,
				Region:               region,
				IsResolveToCommit:    cp.isResolveToCommit,
				IsFailOnUnableToLock: cp.isFailOnUnableToLock,
				VersionSeparator:     "/",
				LatestIdentifier:     "latest",
				IsVersioned:          true,
			}

			// Extract S3 fields
			if bucket, ok := data["bucket"].(string); ok {
				if key, ok := data["key"].(string); ok {
					component.URL = fmt.Sprintf("s3://%s/%s", bucket, key)
				}
			}

			// Check for version_id or version field
			if versionID, ok := data["version_id"].(string); ok {
				component.Version = versionID
				component.IsLocked = (versionID != "latest" && versionID != "")
			} else if version, ok := data["version"].(string); ok {
				component.Version = version
				component.IsLocked = (version != "latest" && version != "")
			}

			// Set YAML paths
			component.YAMLPathToLock = fmt.Sprintf("desiredstate.content.components.artifacts.%s.s3.%s.version_id", artifactID, region)
			component.YAMLPathToUnlock = component.YAMLPathToLock

			if err := cp.manager.AddComponent(component); err != nil {
				return err
			}

			logging.Debug("Parsed S3 component (region-based): %s/%s (url: %s, version: %s, locked: %t)",
				artifactID, region, component.URL, component.Version, component.IsLocked)
		}
	} else {
		// Single S3 artifact (no regions)
		component := &VersionedComponent{
			PartFile:             partFilePath,
			PartID:               fmt.Sprintf("artifacts.%s.s3", artifactID),
			Type:                 ComponentTypeS3,
			IsResolveToCommit:    cp.isResolveToCommit,
			IsFailOnUnableToLock: cp.isFailOnUnableToLock,
			VersionSeparator:     "/",
			LatestIdentifier:     "latest",
			IsVersioned:          true,
		}

		// Extract S3 fields
		if bucket, ok := s3["bucket"].(string); ok {
			if key, ok := s3["key"].(string); ok {
				component.URL = fmt.Sprintf("s3://%s/%s", bucket, key)
			}
		}
		if version, ok := s3["version"].(string); ok {
			component.Version = version
			component.IsLocked = (version != "latest" && version != "")
		}

		// Set YAML paths
		component.YAMLPathToLock = fmt.Sprintf("desiredstate.content.components.artifacts.%s.s3.version", artifactID)
		component.YAMLPathToUnlock = component.YAMLPathToLock

		if err := cp.manager.AddComponent(component); err != nil {
			return err
		}

		logging.Debug("Parsed S3 component: %s (url: %s, version: %s, locked: %t)",
			artifactID, component.URL, component.Version, component.IsLocked)
	}

	return nil
}

// ParseMultipleFiles parses components from multiple part files.
func (cp *ComponentParser) ParseMultipleFiles(files map[string]map[string]interface{}) error {
	for filePath, yamlData := range files {
		if err := cp.ParseFromYAML(yamlData, filePath); err != nil {
			return errors.Wrapf(errors.ErrParse, err, "failed to parse components from %s", filePath)
		}
	}

	return nil
}

// LoadComponentsFromDesiredState loads all components from a desiredstate including part files.
func LoadComponentsFromDesiredState(parser *Parser, isResolveToCommit, isFailOnUnableToLock bool) (*ComponentManager, error) {
	componentParser := NewComponentParser(isResolveToCommit, isFailOnUnableToLock)

	// Get the desiredstate content
	dsWrapper := parser.GetDesiredState().GetDocument().GetContent()
	if dsWrapper == nil {
		return nil, errors.New(errors.ErrParse, "desiredstate content is nil")
	}

	dsContent := dsWrapper.Data
	if dsContent == nil {
		return nil, errors.New(errors.ErrParse, "desiredstate content data is nil")
	}

	// Get the main desiredstate file path
	// TODO: Add method to get file path if needed, for now use empty string
	mainFilePath := ""

	// Parse main desiredstate file
	if err := componentParser.ParseFromYAML(dsContent, mainFilePath); err != nil {
		return nil, err
	}

	// Get part files from meta.parts
	partFiles := extractPartFiles(dsContent)

	// For each part file, we would need to load and parse it
	// For now, we'll just log them
	for partName, partPath := range partFiles {
		logging.Debug("Found part file: %s -> %s", partName, partPath)
		// TODO: Load and parse part files
		// This would require:
		// 1. Resolve relative path to absolute
		// 2. Load the YAML file
		// 3. Parse components from it
	}

	return componentParser.GetManager(), nil
}

// extractPartFiles extracts part file paths from desiredstate meta.
func extractPartFiles(yamlData map[string]interface{}) map[string]string {
	partFiles := make(map[string]string)

	// Navigate to desiredstate.meta.parts
	desiredstate, ok := yamlData["desiredstate"].(map[string]interface{})
	if !ok {
		return partFiles
	}

	meta, ok := desiredstate["meta"].(map[string]interface{})
	if !ok {
		return partFiles
	}

	parts, ok := meta["parts"].(map[string]interface{})
	if !ok {
		return partFiles
	}

	// Extract each part file path
	for partName, partPath := range parts {
		if pathStr, ok := partPath.(string); ok {
			partFiles[partName] = pathStr
		}
	}

	return partFiles
}

// LoadPartFile loads and parses a part file.
func (cp *ComponentParser) LoadPartFile(filePath string) error {
	// Get the absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return errors.Wrapf(errors.ErrParam, err, "failed to resolve part file path: %s", filePath)
	}

	// TODO: Load the YAML file
	// This would use the existing YAML loading infrastructure

	logging.Debug("Loading part file: %s", absPath)

	return nil
}

// FilterComponentsByEnvironment filters components by environment name.
// If environment is "all", returns all components.
func FilterComponentsByEnvironment(components []*VersionedComponent, environment string) []*VersionedComponent {
	if environment == "all" || environment == "" {
		return components
	}

	var filtered []*VersionedComponent
	for _, component := range components {
		// Check if component PartID contains environment-specific info
		// This is a simplified approach; full implementation would need more sophisticated filtering
		if strings.Contains(component.PartID, environment) || component.Region == environment {
			filtered = append(filtered, component)
		} else {
			// For now, include all components
			// TODO: Implement proper environment filtering based on desiredstate structure
			filtered = append(filtered, component)
		}
	}

	return filtered
}
