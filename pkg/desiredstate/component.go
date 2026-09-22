package desiredstate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/jira"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
)

// ComponentType represents the type of component artifact
type ComponentType string

const (
	ComponentTypeDocker     ComponentType = "docker"
	ComponentTypeS3         ComponentType = "s3"
	ComponentTypeSourcecode ComponentType = "sourcecode"
	ComponentTypeGitHub     ComponentType = "github"
	ComponentTypeAMI        ComponentType = "ami"
	ComponentTypeUnknown    ComponentType = "unknown"
)

// VersionedComponent represents a versioned artifact in the desiredstate.
type VersionedComponent struct {
	PartFile   string        // Path to the part file containing this component
	PartID     string        // Unique identifier within the part file
	Type       ComponentType // Type of component (docker, s3, sourcecode, etc.)
	URL        string        // Component URL (repo URL, ECR image, S3 path, etc.)
	OldURL     string        // Previous URL (for tracking changes)
	Version    string        // Current version (tag, commit, digest, etc.)
	OldVersion string        // Previous version (for tracking changes)

	// Version control fields
	Branch           string // Git branch (for sourcecode components)
	Tag              string // Git tag (for sourcecode components)
	Path             string // Path within repository
	VersionSeparator string // Separator between URL and version (typically ":")
	LatestIdentifier string // Identifier for "latest" version (typically "latest")

	// Lock management
	IsVersioned      bool   // Whether this component has versioning
	IsLocked         bool   // Whether version is locked to specific value
	YAMLPathToLock   string // YAML path where version is stored
	YAMLPathToUnlock string // YAML path where version can be unlocked

	// Metadata
	JiraIssueNumber      string // Associated Jira issue
	IsResolveToCommit    bool   // Whether to resolve refs to commit SHAs
	IsFailOnUnableToLock bool   // Whether to fail if locking fails

	// AWS/Region specific (for multi-region artifacts)
	Region string // AWS region for this component

	// Handler-specific metadata (for custom component types)
	// Handlers can store arbitrary data here without modifying core struct
	Metadata map[string]interface{}
}

// ComponentManager manages collections of versioned components.
type ComponentManager struct {
	// partFiles maps part file paths to their components
	// partFiles[partFile][partID] = *VersionedComponent
	partFiles map[string]map[string]*VersionedComponent

	// Track if any components have been modified
	IsUpdated bool

	// AWS managers for version resolution
	ecrManager *aws.ECRManager
	s3Manager  *aws.S3Manager
}

// NewComponentManager creates a new ComponentManager instance.
func NewComponentManager() *ComponentManager {
	return &ComponentManager{
		partFiles: make(map[string]map[string]*VersionedComponent),
		IsUpdated: false,
	}
}

// SetAWSManagers sets the AWS ECR and S3 managers.
// Also updates handlers in the registry with these managers.
func (cm *ComponentManager) SetAWSManagers(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) {
	cm.ecrManager = ecrManager
	cm.s3Manager = s3Manager

	// Update Docker handler with ECR manager if available
	if ecrManager != nil {
		if handler, err := GetComponentHandler(ComponentTypeDocker); err == nil {
			if dockerHandler, ok := handler.(*DockerHandler); ok {
				dockerHandler.SetECRManager(ecrManager)
				logging.Debug("Set ECR manager on Docker handler")
			}
		}
	}

	// Update S3 handler with S3 manager if available
	if s3Manager != nil {
		if handler, err := GetComponentHandler(ComponentTypeS3); err == nil {
			if s3Handler, ok := handler.(*S3Handler); ok {
				s3Handler.SetS3Manager(s3Manager)
				logging.Debug("Set S3 manager on S3 handler")
			}
		}
	}
}

// SetGitClient sets the GitClient for resolving Git references.
// Also updates the Git handler in the registry with this client.
func (cm *ComponentManager) SetGitClient(gitClient *GitClient) {
	// Update Git handler with GitClient if available
	if gitClient != nil {
		if handler, err := GetComponentHandler(ComponentTypeSourcecode); err == nil {
			if gitHandler, ok := handler.(*GitHandler); ok {
				gitHandler.SetGitClient(gitClient)
				logging.Debug("Set GitClient on Git handler")
			}
		}
	}
}

// SetVersionCache sets the version cache for all handlers.
// This enables caching of resolved versions to reduce external API/Git calls.
func (cm *ComponentManager) SetVersionCache(cache *VersionCache) {
	if cache == nil {
		logging.Debug("Version cache not configured")
		return
	}

	// Update Git handler with cache
	if handler, err := GetComponentHandler(ComponentTypeSourcecode); err == nil {
		if gitHandler, ok := handler.(*GitHandler); ok {
			gitHandler.SetCache(cache)
			logging.Debug("Set VersionCache on Git handler")
		}
	}

	// Update Docker handler with cache
	if handler, err := GetComponentHandler(ComponentTypeDocker); err == nil {
		if dockerHandler, ok := handler.(*DockerHandler); ok {
			dockerHandler.SetCache(cache)
			logging.Debug("Set VersionCache on Docker handler")
		}
	}

	// Update S3 handler with cache
	if handler, err := GetComponentHandler(ComponentTypeS3); err == nil {
		if s3Handler, ok := handler.(*S3Handler); ok {
			s3Handler.SetCache(cache)
			logging.Debug("Set VersionCache on S3 handler")
		}
	}

	stats := cache.Stats()
	logging.Info("Version cache configured: %s", stats.String())
}

// AddComponent adds a component to the manager.
func (cm *ComponentManager) AddComponent(component *VersionedComponent) error {
	if component.PartFile == "" {
		return errors.NewParamError("component must have a PartFile")
	}
	if component.PartID == "" {
		return errors.NewParamError("component must have a PartID")
	}

	// Ensure the part file map exists
	if cm.partFiles[component.PartFile] == nil {
		cm.partFiles[component.PartFile] = make(map[string]*VersionedComponent)
	}

	// Add the component
	cm.partFiles[component.PartFile][component.PartID] = component

	return nil
}

// GetComponent retrieves a component by part file and part ID.
func (cm *ComponentManager) GetComponent(partFile, partID string) (*VersionedComponent, error) {
	if cm.partFiles[partFile] == nil {
		return nil, fmt.Errorf("no components found for part file: %s", partFile)
	}

	component, exists := cm.partFiles[partFile][partID]
	if !exists {
		return nil, fmt.Errorf("component %s not found in part file %s", partID, partFile)
	}

	return component, nil
}

// GetAllComponents returns all components across all part files.
func (cm *ComponentManager) GetAllComponents() []*VersionedComponent {
	var components []*VersionedComponent

	for _, partComponents := range cm.partFiles {
		for _, component := range partComponents {
			components = append(components, component)
		}
	}

	return components
}

// GetComponentsByPartFile returns all components for a specific part file.
func (cm *ComponentManager) GetComponentsByPartFile(partFile string) []*VersionedComponent {
	var components []*VersionedComponent

	if partComponents, exists := cm.partFiles[partFile]; exists {
		for _, component := range partComponents {
			components = append(components, component)
		}
	}

	return components
}

// CountComponents returns total, locked, and unlocked component counts.
func (cm *ComponentManager) CountComponents() (total, locked, unlocked int) {
	for _, partComponents := range cm.partFiles {
		for _, component := range partComponents {
			total++
			if component.IsLocked {
				locked++
			} else {
				unlocked++
			}
		}
	}
	return
}

// GetPartFiles returns all part file paths.
func (cm *ComponentManager) GetPartFiles() []string {
	var files []string
	for partFile := range cm.partFiles {
		files = append(files, partFile)
	}
	return files
}

// Lock locks the component to its current version.
// This resolves "latest" or branch references to specific versions (commit SHA, digest, etc.).
// Now delegates to the component's handler for type-specific locking logic.
func (vc *VersionedComponent) Lock(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) error {
	if vc.IsLocked {
		logging.Debug("Component %s is already locked", vc.PartID)
		return nil
	}

	// Try to get handler from registry
	handler, err := GetComponentHandler(vc.Type)
	if err != nil {
		// Fall back to legacy behavior for unregistered types
		logging.Debug("No handler found for type %s, using legacy locking: %v", vc.Type, err)
		return vc.lockLegacy(ecrManager, s3Manager)
	}

	// Use handler to lock the version
	if err := handler.LockVersion(vc); err != nil {
		if vc.IsFailOnUnableToLock {
			return errors.Wrapf(errors.ErrFail, err, "failed to lock component %s", vc.PartID)
		}
		logging.Warn("Unable to lock component %s: %v", vc.PartID, err)
		return nil
	}

	logging.Debug("Locked component %s: %s -> %s", vc.PartID, vc.OldVersion, vc.Version)

	return nil
}

// lockLegacy provides fallback locking for components without handlers
func (vc *VersionedComponent) lockLegacy(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) error {
	// Get the locked version
	lockedVersion, err := vc.GetLock(ecrManager, s3Manager)
	if err != nil {
		if vc.IsFailOnUnableToLock {
			return errors.Wrapf(errors.ErrFail, err, "failed to lock component %s", vc.PartID)
		}
		logging.Warn("Unable to lock component %s: %v", vc.PartID, err)
		return nil
	}

	// Update version
	vc.OldVersion = vc.Version
	vc.Version = lockedVersion
	vc.IsLocked = true

	logging.Debug("Locked component %s (legacy): %s -> %s", vc.PartID, vc.OldVersion, vc.Version)

	return nil
}

// Unlock unlocks the component and updates it to the latest version.
// Now delegates to the component's handler for type-specific unlocking logic.
func (vc *VersionedComponent) Unlock(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) error {
	if !vc.IsLocked {
		logging.Debug("Component %s is already unlocked", vc.PartID)
		return nil
	}

	// Try to get handler from registry
	handler, err := GetComponentHandler(vc.Type)
	if err != nil {
		// Fall back to legacy behavior for unregistered types
		logging.Debug("No handler found for type %s, using legacy unlocking: %v", vc.Type, err)
		return vc.unlockLegacy(ecrManager, s3Manager)
	}

	// Use handler to unlock the version
	if err := handler.UnlockVersion(vc); err != nil {
		if vc.IsFailOnUnableToLock {
			return errors.Wrapf(errors.ErrFail, err, "failed to unlock component %s", vc.PartID)
		}
		logging.Warn("Unable to unlock component %s: %v", vc.PartID, err)
		return nil
	}

	logging.Debug("Unlocked component %s: %s -> %s", vc.PartID, vc.OldVersion, vc.Version)

	return nil
}

// unlockLegacy provides fallback unlocking for components without handlers
func (vc *VersionedComponent) unlockLegacy(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) error {
	// Get the latest version
	latestVersion, err := vc.GetLatestVersion(ecrManager, s3Manager)
	if err != nil {
		if vc.IsFailOnUnableToLock {
			return errors.Wrapf(errors.ErrFail, err, "failed to unlock component %s", vc.PartID)
		}
		logging.Warn("Unable to unlock component %s: %v", vc.PartID, err)
		return nil
	}

	// Update version
	vc.OldVersion = vc.Version
	vc.Version = latestVersion
	vc.IsLocked = false

	logging.Debug("Unlocked component %s (legacy): %s -> %s", vc.PartID, vc.OldVersion, vc.Version)

	return nil
}

// GetLock resolves the current version to a locked version.
// For "latest", this queries AWS or Git to get the specific version.
// Now delegates to handlers when available.
func (vc *VersionedComponent) GetLock(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) (string, error) {
	// Try to get handler from registry
	handler, err := GetComponentHandler(vc.Type)
	if err != nil {
		// Fall back to legacy behavior for unregistered types
		logging.Debug("No handler found for type %s, using legacy GetLock: %v", vc.Type, err)
		return vc.getLockLegacy(ecrManager, s3Manager)
	}

	// Create resolve context
	ctx := &ResolveContext{
		AWSProfile: "", // TODO: Get from config
		AWSRegion:  vc.Region,
		Options: map[string]interface{}{
			"ecrManager": ecrManager,
			"s3Manager":  s3Manager,
		},
	}

	// Use handler to resolve version
	lockedVersion, err := handler.ResolveVersion(vc, ctx)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "handler failed to resolve version for %s", vc.PartID)
	}

	return lockedVersion, nil
}

// getLockLegacy provides fallback version resolution for components without handlers
func (vc *VersionedComponent) getLockLegacy(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) (string, error) {
	// If version is already locked (not "latest"), return it
	if vc.Version != vc.LatestIdentifier && vc.Version != "" {
		return vc.Version, nil
	}

	// Otherwise, we need to resolve "latest" based on component type
	switch vc.Type {
	case ComponentTypeDocker:
		return vc.lockDockerImage(ecrManager)
	case ComponentTypeS3:
		return vc.lockS3Object(s3Manager)
	case ComponentTypeSourcecode, ComponentTypeGitHub:
		return vc.lockGitReference()
	default:
		return "", fmt.Errorf("unsupported component type for locking: %s", vc.Type)
	}
}

// GetLatestVersion gets the latest version for this component.
// Now delegates to handlers when available.
func (vc *VersionedComponent) GetLatestVersion(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) (string, error) {
	// Try to get handler from registry
	handler, err := GetComponentHandler(vc.Type)
	if err != nil {
		// Fall back to legacy behavior for unregistered types
		logging.Debug("No handler found for type %s, using legacy GetLatestVersion: %v", vc.Type, err)
		return vc.getLatestVersionLegacy(ecrManager, s3Manager)
	}

	// Create resolve context for "latest"
	tempVersion := vc.Version
	vc.Version = "latest" // Temporarily set to latest for resolution

	ctx := &ResolveContext{
		AWSProfile: "", // TODO: Get from config
		AWSRegion:  vc.Region,
		Options: map[string]interface{}{
			"ecrManager": ecrManager,
			"s3Manager":  s3Manager,
		},
	}

	// Use handler to resolve latest version
	latestVersion, err := handler.ResolveVersion(vc, ctx)
	vc.Version = tempVersion // Restore original version

	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "handler failed to resolve latest version for %s", vc.PartID)
	}

	return latestVersion, nil
}

// getLatestVersionLegacy provides fallback latest version resolution for components without handlers
func (vc *VersionedComponent) getLatestVersionLegacy(ecrManager *aws.ECRManager, s3Manager *aws.S3Manager) (string, error) {
	switch vc.Type {
	case ComponentTypeDocker:
		return vc.getLatestDockerImage(ecrManager)
	case ComponentTypeS3:
		return vc.getLatestS3Object(s3Manager)
	case ComponentTypeSourcecode, ComponentTypeGitHub:
		return vc.getLatestGitReference()
	default:
		return "", fmt.Errorf("unsupported component type for latest version: %s", vc.Type)
	}
}

// lockDockerImage resolves a Docker image to its digest.
// This will use ECR manager when available.
// Enhanced in Phase 6 with validation and existence checks.
func (vc *VersionedComponent) lockDockerImage(ecrManager *aws.ECRManager) (string, error) {
	if ecrManager == nil {
		return "", errors.New(errors.ErrFail, "ECR manager not initialized - cannot lock Docker image")
	}

	// Validate URL
	if vc.URL == "" {
		return "", errors.New(errors.ErrParam, "Docker component must have an image URL")
	}

	// If already a digest, validate and return it
	if strings.HasPrefix(vc.Version, "sha256:") {
		if err := validateDockerDigest(vc.Version); err != nil {
			return "", errors.Wrapf(errors.ErrParam, err, "invalid Docker digest format")
		}
		logging.Debug("Component %s already locked to digest: %s", vc.PartID, vc.Version)
		return vc.Version, nil
	}

	// Verify image exists before getting digest
	exists, err := ecrManager.ImageExists(vc.URL, vc.Version)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to verify image existence: %s:%s", vc.URL, vc.Version)
	}
	if !exists {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("image not found in ECR: %s:%s", vc.URL, vc.Version))
	}

	// Use ECR manager to get image digest
	digest, err := ecrManager.GetImageDigest(vc.URL, vc.Version)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get image digest for %s:%s", vc.URL, vc.Version)
	}

	// Validate returned digest
	if err := validateDockerDigest(digest); err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "ECR returned invalid digest format")
	}

	logging.Debug("Locked Docker image %s:%s -> %s", vc.URL, vc.Version, digest)
	return digest, nil
}

// validateDockerDigest validates a Docker image digest format
func validateDockerDigest(digest string) error {
	if !strings.HasPrefix(digest, "sha256:") {
		return fmt.Errorf("digest must start with 'sha256:', got: %s", digest)
	}

	// Check that the hash part is 64 hex characters
	hashPart := strings.TrimPrefix(digest, "sha256:")
	if len(hashPart) != 64 {
		return fmt.Errorf("digest hash must be 64 characters, got: %d", len(hashPart))
	}

	// Verify it's all hex characters
	for _, c := range hashPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("digest hash contains invalid character: %c", c)
		}
	}

	return nil
}

// getLatestDockerImage gets the latest Docker image version.
// Enhanced in Phase 6 with validation.
func (vc *VersionedComponent) getLatestDockerImage(ecrManager *aws.ECRManager) (string, error) {
	if ecrManager == nil {
		return "", errors.New(errors.ErrFail, "ECR manager not initialized - cannot get latest Docker image")
	}

	// Validate URL
	if vc.URL == "" {
		return "", errors.New(errors.ErrParam, "Docker component must have an image URL")
	}

	// Use ECR manager to get latest tag
	latestTag, err := ecrManager.GetLatestImageTag(vc.URL)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get latest tag for %s", vc.URL)
	}

	// Validate tag is not empty
	if latestTag == "" {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("ECR returned empty tag for %s", vc.URL))
	}

	logging.Debug("Got latest Docker tag for %s: %s", vc.URL, latestTag)
	return latestTag, nil
}

// lockS3Object resolves an S3 object to its version ID.
// This will use S3 manager when available.
// Enhanced in Phase 6 with validation and existence checks.
func (vc *VersionedComponent) lockS3Object(s3Manager *aws.S3Manager) (string, error) {
	if s3Manager == nil {
		return "", errors.New(errors.ErrFail, "S3 manager not initialized - cannot lock S3 object")
	}

	// Validate URL
	if vc.URL == "" {
		return "", errors.New(errors.ErrParam, "S3 component must have a URL")
	}

	// Parse S3 URL
	bucket, key, err := aws.ParseS3URL(vc.URL)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParam, err, "invalid S3 URL format")
	}

	// If already a version ID (not "latest"), validate and optionally verify existence
	if vc.Version != "" && vc.Version != "latest" && vc.Version != vc.LatestIdentifier {
		if err := validateS3VersionID(vc.Version); err != nil {
			return "", errors.Wrapf(errors.ErrParam, err, "invalid S3 version ID format")
		}

		// Verify version exists
		exists, err := s3Manager.ObjectExists(bucket, key, vc.Version)
		if err != nil {
			logging.Warn("Could not verify S3 object version existence: %v", err)
		} else if !exists {
			return "", errors.New(errors.ErrFail, fmt.Sprintf("S3 object version not found: s3://%s/%s (version: %s)", bucket, key, vc.Version))
		}

		logging.Debug("Component %s already locked to S3 version: %s", vc.PartID, vc.Version)
		return vc.Version, nil
	}

	// Verify object exists
	exists, err := s3Manager.ObjectExists(bucket, key, "")
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to verify S3 object existence: s3://%s/%s", bucket, key)
	}
	if !exists {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("S3 object not found: s3://%s/%s", bucket, key))
	}

	// Use S3 manager to get object version
	version, err := s3Manager.GetObjectVersion(bucket, key)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get S3 object version for s3://%s/%s", bucket, key)
	}

	// Validate returned version ID
	if version != "" {
		if err := validateS3VersionID(version); err != nil {
			return "", errors.Wrapf(errors.ErrFail, err, "S3 returned invalid version ID format")
		}
	}

	logging.Debug("Locked S3 object s3://%s/%s -> version: %s", bucket, key, version)
	return version, nil
}

// validateS3VersionID validates an S3 version ID format
func validateS3VersionID(versionID string) error {
	if versionID == "" {
		return fmt.Errorf("version ID cannot be empty")
	}

	// S3 version IDs are typically alphanumeric with some special characters
	// Length varies but usually between 32-100 characters
	if len(versionID) < 10 {
		return fmt.Errorf("version ID too short: %d characters", len(versionID))
	}

	if len(versionID) > 200 {
		return fmt.Errorf("version ID too long: %d characters", len(versionID))
	}

	// Check for valid characters (alphanumeric, dash, underscore, dot)
	for _, c := range versionID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			c == '-' || c == '_' || c == '.') {
			return fmt.Errorf("version ID contains invalid character: %c", c)
		}
	}

	return nil
}

// getLatestS3Object gets the latest S3 object version.
// getLatestS3Object resolves the latest version tag for an S3 object.
// Enhanced in Phase 6 with validation and error handling.
func (vc *VersionedComponent) getLatestS3Object(s3Manager *aws.S3Manager) (string, error) {
	if s3Manager == nil {
		return "", errors.New(errors.ErrFail, "S3 manager not initialized - cannot get latest S3 version")
	}

	// Validate URL
	if vc.URL == "" {
		return "", errors.New(errors.ErrParam, "S3 component must have a URL")
	}

	// Parse S3 URL
	bucket, key, err := aws.ParseS3URL(vc.URL)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParam, err, "invalid S3 URL format")
	}

	// Verify object exists
	exists, err := s3Manager.ObjectExists(bucket, key, "")
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to verify S3 object existence: s3://%s/%s", bucket, key)
	}
	if !exists {
		return "", errors.New(errors.ErrFail, fmt.Sprintf("S3 object not found: s3://%s/%s", bucket, key))
	}

	// Use S3 manager to get latest version
	version, err := s3Manager.GetLatestObjectVersion(bucket, key)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get latest S3 object version for s3://%s/%s", bucket, key)
	}

	// Validate returned version if not empty
	if version != "" {
		if err := validateS3VersionID(version); err != nil {
			return "", errors.Wrapf(errors.ErrFail, err, "S3 returned invalid version ID format")
		}
	} else {
		logging.Warn("S3 returned empty version ID for s3://%s/%s", bucket, key)
	}

	logging.Debug("Latest S3 object s3://%s/%s -> version: %s", bucket, key, version)
	return version, nil
}

// lockGitReference resolves a Git branch/tag to a commit SHA.
func (vc *VersionedComponent) lockGitReference() (string, error) {
	if vc.Branch == "" && vc.Tag == "" {
		return "", errors.New(errors.ErrParam, "Git component must have branch or tag")
	}

	// TODO: Integrate with Git operations
	// For now, return the branch/tag as-is
	if vc.Tag != "" {
		return vc.Tag, nil
	}
	return vc.Branch, nil
}

// getLatestGitReference gets the latest Git reference.
func (vc *VersionedComponent) getLatestGitReference() (string, error) {
	// For Git, "latest" typically means the HEAD of the default branch
	if vc.Branch != "" {
		return vc.Branch, nil
	}
	return "main", nil // Default to main branch
}

// GetJiraIssueNumber extracts the Jira issue number from component metadata.
// This method searches for Jira issues in the component's version string, URL, branch, or tag.
// If resolveToJira is true, it would validate the issue exists (not yet implemented).
func (vc *VersionedComponent) GetJiraIssueNumber(resolveToJira bool) (string, error) {
	// If we already have it cached, return it
	if vc.JiraIssueNumber != "" && vc.JiraIssueNumber != "NOT FOUND" {
		return vc.JiraIssueNumber, nil
	}

	var issueNumber string

	// Try to extract from version first (most common)
	if vc.Version != "" {
		issueNumber = jira.ParseIssueFromString(vc.Version)
	}

	// If not found, try branch name
	if issueNumber == "" && vc.Branch != "" {
		issueNumber = jira.ParseIssueFromString(vc.Branch)
	}

	// If not found, try tag
	if issueNumber == "" && vc.Tag != "" {
		issueNumber = jira.ParseIssueFromString(vc.Tag)
	}

	// If not found, try URL
	if issueNumber == "" && vc.URL != "" {
		issueNumber = jira.ParseIssueFromString(vc.URL)
	}

	// TODO: If resolveToJira is true, validate the issue exists via Jira API
	// This would require:
	// 1. Jira API client configuration
	// 2. Authentication (API token or OAuth)
	// 3. Query Jira REST API to verify issue exists
	// 4. Handle errors (issue not found, API timeout, etc.)
	//
	// For now, we only parse and don't validate
	if resolveToJira && issueNumber != "" {
		logging.Debug("Jira validation requested but not yet implemented for: %s", issueNumber)
	}

	// Cache the result
	if issueNumber == "" {
		vc.JiraIssueNumber = "NOT FOUND"
		return "NOT FOUND", nil
	}

	vc.JiraIssueNumber = issueNumber
	logging.Debug("Found Jira issue for component %s: %s", vc.PartID, issueNumber)

	return issueNumber, nil
}

// CacheLocks caches the current version as a lock.
// If setToLatest is true, sets version to "latest" identifier.
func (vc *VersionedComponent) CacheLocks(setToLatest bool) {
	if setToLatest {
		vc.OldVersion = vc.Version
		vc.Version = vc.LatestIdentifier
		vc.IsLocked = false
	} else {
		// Cache current version
		vc.OldVersion = vc.Version
	}
}

// DetectComponentType determines the component type from its properties.
func DetectComponentType(data map[string]interface{}) ComponentType {
	// Check for Docker-specific fields
	if _, hasImage := data["image"]; hasImage {
		return ComponentTypeDocker
	}
	if _, hasTag := data["tag"]; hasTag {
		if _, hasImage := data["image"]; hasImage {
			return ComponentTypeDocker
		}
	}

	// Check for sourcecode fields
	if _, hasURL := data["url"]; hasURL {
		if url, ok := data["url"].(string); ok {
			if strings.Contains(url, "git@") || strings.Contains(url, ".git") {
				return ComponentTypeSourcecode
			}
			if strings.Contains(url, "s3://") {
				return ComponentTypeS3
			}
		}
	}

	// Check for S3-specific fields
	if _, hasBucket := data["bucket"]; hasBucket {
		return ComponentTypeS3
	}

	return ComponentTypeUnknown
}

// ParseJiraIssue extracts Jira issue number from text using regex.
func ParseJiraIssue(text string, jiraProjects []string) string {
	if len(jiraProjects) == 0 {
		// Default Jira project pattern
		jiraProjects = []string{"[A-Z]+"}
	}

	// Build regex pattern: (PROJECT1|PROJECT2|...)-\d+
	projectPattern := strings.Join(jiraProjects, "|")
	pattern := fmt.Sprintf("(%s)-\\d+", projectPattern)

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)

	if len(matches) > 0 {
		return matches[0]
	}

	return "NOT FOUND"
}
