package desiredstate

import (
	"fmt"
	"strings"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
)

// DockerHandler implements ComponentTypeHandler for Docker/ECR images.
// This handler manages Docker container images stored in AWS ECR (Elastic Container Registry).
//
// Docker components support:
// - Multi-region deployments (separate images per region)
// - Version locking to specific tags or digests
// - Digest-based immutable references (sha256:...)
// - Latest tag resolution
//
// Example YAML structure:
//
//	components:
//	  artifacts:
//	    my-app:
//	      docker:
//	        eu-west-1:
//	          image: 1234567890.dkr.ecr.eu-west-1.amazonaws.com/my-app
//	          tag: 1.5.0
//	        us-east-1:
//	          image: 1234567890.dkr.ecr.us-east-1.amazonaws.com/my-app
//	          tag: latest
type DockerHandler struct {
	BaseComponentHandler
	ecrManager *aws.ECRManager
	cache      *VersionCache // Optional cache for resolved versions
}

// NewDockerHandler creates a new Docker component handler.
func NewDockerHandler(ecrManager *aws.ECRManager) *DockerHandler {
	return &DockerHandler{
		ecrManager: ecrManager,
	}
}

// Type returns the component type identifier for Docker components.
func (h *DockerHandler) Type() ComponentType {
	return ComponentTypeDocker
}

// ParseComponent extracts Docker component data from YAML structure.
// Docker components are typically nested under artifacts with multi-region support.
//
// Expected structure:
//
//	artifactID:
//	  docker:
//	    region:
//	      image: "ecr-image-url"
//	      tag: "version-or-digest"
func (h *DockerHandler) ParseComponent(data map[string]interface{}, partID, partFile string) (*VersionedComponent, error) {
	component := &VersionedComponent{
		PartFile:         partFile,
		PartID:           partID,
		Type:             ComponentTypeDocker,
		VersionSeparator: ":",
		LatestIdentifier: "latest",
		IsVersioned:      true,
		Metadata:         make(map[string]interface{}),
	}

	// Extract image URL
	image, err := ParseYAMLString(data, "image", true)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParam, err, "Docker component '%s' missing 'image' field", partID)
	}
	component.URL = image

	// Extract tag (version)
	tag, err := ParseYAMLString(data, "tag", false)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParam, err, "Docker component '%s' has invalid 'tag' field", partID)
	}
	if tag == "" {
		tag = "latest" // Default to latest if not specified
	}
	component.Version = tag

	// Determine if locked (not "latest" and not empty)
	component.IsLocked = IsVersionLocked(tag)

	// Store Docker-specific metadata
	component.Metadata["is_digest"] = strings.HasPrefix(tag, "sha256:")

	logging.Debug("Parsed Docker component: %s (image: %s, tag: %s, locked: %t)",
		partID, component.URL, component.Version, component.IsLocked)

	return component, nil
}

// ResolveVersion resolves "latest" or unlocked version to a specific version.
// For Docker, this queries ECR to get the latest tag or validates existing digests.
func (h *DockerHandler) ResolveVersion(comp *VersionedComponent, ctx *ResolveContext) (string, error) {
	// Validate URL
	if comp.URL == "" {
		return "", errors.New(errors.ErrParam, "Docker component must have an image URL")
	}

	// If already a digest, validate and return it (no ECR manager needed)
	if strings.HasPrefix(comp.Version, "sha256:") {
		if err := h.validateDockerDigest(comp.Version); err != nil {
			return "", errors.Wrapf(errors.ErrParam, err, "invalid Docker digest format")
		}
		logging.Debug("Docker image already has digest: %s", comp.Version)
		return comp.Version, nil
	}

	// If version is locked (not "latest"), return it (no ECR manager needed)
	if comp.IsLocked && comp.Version != comp.LatestIdentifier {
		return comp.Version, nil
	}

	// Check cache first
	cacheKey := MakeCacheKey(ComponentTypeDocker, comp.URL, comp.Version)
	if h.cache != nil {
		if cached, found := h.cache.Get(cacheKey); found {
			logging.Debug("Cache hit for Docker image: %s:%s -> %s", comp.URL, comp.Version, cached)
			return cached, nil
		}
	}

	// For resolving "latest" or unlocked versions, we need ECR manager
	if h.ecrManager == nil {
		return "", errors.New(errors.ErrFail, "ECR manager not initialized - cannot resolve Docker version")
	}

	// Query ECR for latest tag
	latestTag, err := h.ecrManager.GetLatestImageTag(comp.URL)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get latest tag for %s", comp.URL)
	}

	if latestTag == "" {
		return "", errors.Newf(errors.ErrFail, "ECR returned empty tag for %s", comp.URL)
	}

	// Store in cache
	if h.cache != nil {
		h.cache.Set(cacheKey, latestTag, ComponentTypeDocker)
	}

	logging.Debug("Resolved Docker image %s to latest tag: %s", comp.URL, latestTag)
	return latestTag, nil
}

// CompareVersions compares two Docker version strings.
// Docker versions can be:
// - Semantic versions (1.0.0, v2.1.3)
// - Timestamps (1688899028633)
// - Digests (sha256:abc123...)
//
// For digests, compares lexicographically.
// For versions, uses semantic or timestamp comparison.
func (h *DockerHandler) CompareVersions(v1, v2 string) (int, error) {
	// If both are digests, compare lexicographically
	if strings.HasPrefix(v1, "sha256:") && strings.HasPrefix(v2, "sha256:") {
		if v1 < v2 {
			return -1, nil
		} else if v1 > v2 {
			return 1, nil
		}
		return 0, nil
	}

	// If one is digest and other isn't, can't meaningfully compare
	if strings.HasPrefix(v1, "sha256:") || strings.HasPrefix(v2, "sha256:") {
		return 0, errors.Newf(errors.ErrParam, "cannot compare digest with non-digest version")
	}

	// Try semantic version comparison first
	result, err := CompareSemanticVersions(v1, v2)
	if err == nil {
		return result, nil
	}

	// Fallback to timestamp comparison
	return CompareTimestampVersions(v1, v2)
}

// LockVersion locks a Docker component to a specific digest.
// This converts a tag to a digest by querying ECR.
// Note: This method is called from Lock() which doesn't pass context yet,
// so it relies on the handler's ecrManager being set via SetECRManager().
// TODO: Refactor to accept ResolveContext parameter in future.
func (h *DockerHandler) LockVersion(comp *VersionedComponent) error {
	if h.ecrManager == nil {
		return errors.New(errors.ErrFail, "ECR manager not initialized - cannot lock Docker image")
	}

	// Validate URL
	if comp.URL == "" {
		return errors.New(errors.ErrParam, "Docker component must have an image URL")
	}

	// If already a digest, just mark as locked
	if strings.HasPrefix(comp.Version, "sha256:") {
		if err := h.validateDockerDigest(comp.Version); err != nil {
			return errors.Wrapf(errors.ErrParam, err, "invalid Docker digest format")
		}
		comp.IsLocked = true
		logging.Debug("Docker image already locked with digest: %s", comp.Version)
		return nil
	}

	// Check cache first for digest
	cacheKey := MakeCacheKey(ComponentTypeDocker, comp.URL, comp.Version)
	var digest string
	var err error

	if h.cache != nil {
		if cached, found := h.cache.Get(cacheKey); found {
			// Verify cached value is a digest
			if strings.HasPrefix(cached, "sha256:") {
				logging.Debug("Cache hit for Docker digest: %s:%s -> %s", comp.URL, comp.Version, cached)
				digest = cached
			}
		}
	}

	// If not in cache, query ECR
	if digest == "" {
		digest, err = h.ecrManager.GetImageDigest(comp.URL, comp.Version)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to get digest for %s:%s", comp.URL, comp.Version)
		}

		// Validate the digest
		if err := h.validateDockerDigest(digest); err != nil {
			return errors.Wrapf(errors.ErrParam, err, "ECR returned invalid digest")
		}

		// Store in cache
		if h.cache != nil {
			h.cache.Set(cacheKey, digest, ComponentTypeDocker)
		}
	}

	// Update component to locked digest
	comp.OldVersion = comp.Version
	comp.Version = digest
	comp.IsLocked = true
	comp.Metadata["is_digest"] = true

	logging.Debug("Locked Docker image %s:%s -> %s", comp.URL, comp.OldVersion, digest)
	return nil
}

// UnlockVersion unlocks a Docker component to track latest.
func (h *DockerHandler) UnlockVersion(comp *VersionedComponent) error {
	comp.OldVersion = comp.Version
	comp.Version = "latest"
	comp.IsLocked = false
	comp.Metadata["is_digest"] = false

	logging.Debug("Unlocked Docker image %s: %s -> latest", comp.URL, comp.OldVersion)
	return nil
}

// ValidateComponent performs Docker-specific validation.
// Checks:
// - Image URL is present
// - Digest format is valid (if present)
// - Tag/version is not empty
func (h *DockerHandler) ValidateComponent(comp *VersionedComponent) error {
	// Validate URL
	if comp.URL == "" {
		return errors.Newf(errors.ErrParam, "Docker component '%s' must have an image URL", comp.PartID)
	}

	// Validate version
	if comp.Version == "" {
		return errors.Newf(errors.ErrParam, "Docker component '%s' must have a tag/version", comp.PartID)
	}

	// If it's a digest, validate format
	if strings.HasPrefix(comp.Version, "sha256:") {
		if err := h.validateDockerDigest(comp.Version); err != nil {
			return errors.Wrapf(errors.ErrParam, err, "Docker component '%s' has invalid digest", comp.PartID)
		}
	}

	return nil
}

// validateDockerDigest validates a Docker image digest format.
// Valid format: sha256:<64-character-hex-string>
func (h *DockerHandler) validateDockerDigest(digest string) error {
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

// SetECRManager allows updating the ECR manager after handler creation.
// This is useful when the manager is initialized lazily or needs to be replaced.
func (h *DockerHandler) SetECRManager(ecrManager *aws.ECRManager) {
	h.ecrManager = ecrManager
	logging.Debug("Set ECR manager on Docker handler")
}

// SetCache injects a VersionCache for caching resolved versions.
func (h *DockerHandler) SetCache(cache *VersionCache) {
	h.cache = cache
	logging.Debug("Set VersionCache on Docker handler")
}

// init registers the Docker handler with the global component registry.
// This is called automatically when the package is imported.
func init() {
	// Register with nil ECR manager - will be set later via SetECRManager
	// or when ComponentManager initializes
	RegisterComponentType(NewDockerHandler(nil))
}
