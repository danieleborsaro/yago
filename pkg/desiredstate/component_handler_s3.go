package desiredstate

import (
	"fmt"

	"github.com/danieleborsaro/yago/internal/utils/errors"
	"github.com/danieleborsaro/yago/internal/utils/logging"
	"github.com/danieleborsaro/yago/pkg/aws"
)

// S3Handler implements ComponentTypeHandler for S3 objects.
// It handles S3 object versioning, bucket operations, and version resolution.
//
// S3 components have the following structure in YAML:
//
//	artifacts:
//	  my-data:
//	    s3:
//	      us-east-1:
//	        bucket: my-bucket
//	        key: path/to/object.zip
//	        version: v1.2.3  # or version_id for actual S3 version
//
// The handler supports:
//   - Version ID validation
//   - Object existence verification
//   - Latest version resolution
//   - Multi-region S3 objects
type S3Handler struct {
	BaseComponentHandler
	s3Manager *aws.S3Manager
	cache     *VersionCache // Optional cache for resolved versions
}

// NewS3Handler creates a new S3 handler instance.
func NewS3Handler(s3Manager *aws.S3Manager) *S3Handler {
	return &S3Handler{
		s3Manager: s3Manager,
	}
}

// Type returns the component type this handler manages.
func (h *S3Handler) Type() ComponentType {
	return ComponentTypeS3
}

// ParseComponent parses an S3 component from YAML data.
// Extracts bucket, key, version/version_id fields and constructs S3 URL.
func (h *S3Handler) ParseComponent(data map[string]interface{}, partID, partFile string) (*VersionedComponent, error) {
	comp := &VersionedComponent{
		PartFile:         partFile,
		PartID:           partID,
		Type:             ComponentTypeS3,
		VersionSeparator: "/",
		LatestIdentifier: "latest",
		IsVersioned:      true,
		Metadata:         make(map[string]interface{}),
	}

	// Extract bucket
	bucket, err := ParseYAMLString(data, "bucket", true)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "S3 component must have 'bucket' field")
	}

	// Extract key
	key, err := ParseYAMLString(data, "key", true)
	if err != nil {
		return nil, errors.Wrapf(errors.ErrParse, err, "S3 component must have 'key' field")
	}

	// Construct S3 URL
	comp.URL = fmt.Sprintf("s3://%s/%s", bucket, key)

	// Store bucket and key in metadata for easy access
	comp.Metadata["bucket"] = bucket
	comp.Metadata["key"] = key

	// Extract version or version_id (version_id takes precedence)
	versionID, _ := ParseYAMLString(data, "version_id", false)
	if versionID == "" {
		versionID, _ = ParseYAMLString(data, "version", false)
	}

	if versionID != "" {
		comp.Version = versionID
		// Check if this is a locked version (not "latest")
		comp.IsLocked = (versionID != "latest" && versionID != "")
		if comp.IsLocked {
			comp.Metadata["is_version_id"] = true
		}
	} else {
		// Default to latest
		comp.Version = "latest"
		comp.IsLocked = false
	}

	logging.Debug("Parsed S3 component: %s (bucket: %s, key: %s, version: %s, locked: %t)",
		partID, bucket, key, comp.Version, comp.IsLocked)

	return comp, nil
}

// ResolveVersion resolves "latest" or unlocked version to a specific version.
// For S3, this queries S3 to get the latest object version or validates existing version IDs.
func (h *S3Handler) ResolveVersion(comp *VersionedComponent, ctx *ResolveContext) (string, error) {
	// Parse S3 URL to get bucket and key
	bucket, key, err := h.parseS3URL(comp.URL)
	if err != nil {
		return "", errors.Wrapf(errors.ErrParam, err, "invalid S3 URL format")
	}

	// If already a version ID (not "latest"), validate and return it
	if comp.Version != "" && comp.Version != "latest" && comp.Version != comp.LatestIdentifier {
		if err := h.validateS3VersionID(comp.Version); err != nil {
			return "", errors.Wrapf(errors.ErrParam, err, "invalid S3 version ID format")
		}
		logging.Debug("S3 object already has version ID: %s", comp.Version)
		return comp.Version, nil
	}

	// Check cache first
	cacheKey := MakeCacheKey(ComponentTypeS3, comp.URL, comp.Version)
	if h.cache != nil {
		if cached, found := h.cache.Get(cacheKey); found {
			logging.Debug("Cache hit for S3 object: s3://%s/%s -> %s", bucket, key, cached)
			return cached, nil
		}
	}

	// For resolving "latest" or unlocked versions, we need S3 manager
	if h.s3Manager == nil {
		return "", errors.New(errors.ErrFail, "S3 manager not initialized - cannot resolve S3 version")
	}

	// Query S3 for latest version
	version, err := h.s3Manager.GetLatestObjectVersion(bucket, key)
	if err != nil {
		return "", errors.Wrapf(errors.ErrFail, err, "failed to get latest S3 object version for s3://%s/%s", bucket, key)
	}

	// Validate returned version if not empty
	if version != "" {
		if err := h.validateS3VersionID(version); err != nil {
			return "", errors.Wrapf(errors.ErrFail, err, "S3 returned invalid version ID format")
		}

		// Store in cache
		if h.cache != nil {
			h.cache.Set(cacheKey, version, ComponentTypeS3)
		}
	} else {
		logging.Warn("S3 returned empty version ID for s3://%s/%s", bucket, key)
	}

	logging.Debug("Resolved S3 object s3://%s/%s to version: %s", bucket, key, version)
	return version, nil
}

// CompareVersions compares two S3 version strings.
// S3 version IDs are opaque strings, so we compare them lexicographically.
// If versions look like semantic versions, use semantic comparison.
func (h *S3Handler) CompareVersions(v1, v2 string) (int, error) {
	// Try semantic version comparison first
	result, err := CompareSemanticVersions(v1, v2)
	if err == nil {
		return result, nil
	}

	// Fall back to timestamp comparison
	result, err = CompareTimestampVersions(v1, v2)
	if err == nil {
		return result, nil
	}

	// Fall back to lexicographic comparison for opaque version IDs
	if v1 < v2 {
		return -1, nil
	} else if v1 > v2 {
		return 1, nil
	}
	return 0, nil
}

// LockVersion locks an S3 component to a specific version ID.
// This converts "latest" to an actual S3 version ID by querying S3.
func (h *S3Handler) LockVersion(comp *VersionedComponent) error {
	// Parse S3 URL
	bucket, key, err := h.parseS3URL(comp.URL)
	if err != nil {
		return errors.Wrapf(errors.ErrParam, err, "invalid S3 URL format")
	}

	// If already a version ID (not "latest"), just validate and mark as locked
	if comp.Version != "" && comp.Version != "latest" && comp.Version != comp.LatestIdentifier {
		if err := h.validateS3VersionID(comp.Version); err != nil {
			return errors.Wrapf(errors.ErrParam, err, "invalid S3 version ID format")
		}
		comp.IsLocked = true
		comp.Metadata["is_version_id"] = true
		logging.Debug("S3 object already locked with version ID: %s", comp.Version)
		return nil
	}

	// For resolving "latest", we need S3 manager
	if h.s3Manager == nil {
		return errors.New(errors.ErrFail, "S3 manager not initialized - cannot lock S3 object")
	}

	// Check cache first
	cacheKey := MakeCacheKey(ComponentTypeS3, comp.URL, comp.Version)
	var versionID string

	if h.cache != nil {
		if cached, found := h.cache.Get(cacheKey); found {
			logging.Debug("Cache hit for S3 version: s3://%s/%s -> %s", bucket, key, cached)
			versionID = cached
		}
	}

	// If not in cache, query S3
	if versionID == "" {
		versionID, err = h.s3Manager.GetObjectVersion(bucket, key)
		if err != nil {
			return errors.Wrapf(errors.ErrFail, err, "failed to get version ID for s3://%s/%s", bucket, key)
		}

		// Validate the version ID
		if versionID != "" {
			if err := h.validateS3VersionID(versionID); err != nil {
				return errors.Wrapf(errors.ErrParam, err, "S3 returned invalid version ID")
			}

			// Store in cache
			if h.cache != nil {
				h.cache.Set(cacheKey, versionID, ComponentTypeS3)
			}
		}
	}

	// Update component to locked version
	comp.OldVersion = comp.Version
	comp.Version = versionID
	comp.IsLocked = true
	comp.Metadata["is_version_id"] = true

	logging.Debug("Locked S3 object s3://%s/%s: %s -> %s", bucket, key, comp.OldVersion, versionID)
	return nil
}

// UnlockVersion unlocks an S3 component to track latest.
func (h *S3Handler) UnlockVersion(comp *VersionedComponent) error {
	comp.OldVersion = comp.Version
	comp.Version = "latest"
	comp.IsLocked = false
	comp.Metadata["is_version_id"] = false

	bucket, key, _ := h.parseS3URL(comp.URL)
	logging.Debug("Unlocked S3 object s3://%s/%s: %s -> latest", bucket, key, comp.OldVersion)
	return nil
}

// ValidateComponent performs S3-specific validation.
func (h *S3Handler) ValidateComponent(comp *VersionedComponent) error {
	if comp.URL == "" {
		return errors.New(errors.ErrParam, "S3 component must have a URL")
	}

	// Parse and validate S3 URL
	bucket, key, err := h.parseS3URL(comp.URL)
	if err != nil {
		return errors.Wrapf(errors.ErrParam, err, "invalid S3 URL")
	}

	if bucket == "" {
		return errors.New(errors.ErrParam, "S3 bucket cannot be empty")
	}

	if key == "" {
		return errors.New(errors.ErrParam, "S3 key cannot be empty")
	}

	// Validate version if present
	if comp.Version != "" && comp.Version != "latest" {
		if err := h.validateS3VersionID(comp.Version); err != nil {
			return errors.Wrapf(errors.ErrParam, err, "S3 component '%s' has invalid version ID", comp.PartID)
		}
	}

	return nil
}

// parseS3URL parses an S3 URL into bucket and key components.
func (h *S3Handler) parseS3URL(url string) (bucket, key string, err error) {
	return aws.ParseS3URL(url)
}

// validateS3VersionID validates an S3 version ID format.
func (h *S3Handler) validateS3VersionID(versionID string) error {
	if versionID == "" {
		return fmt.Errorf("version ID cannot be empty")
	}

	// S3 version IDs are typically alphanumeric with some special characters
	// Length varies but usually between 10-200 characters
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

// SetS3Manager sets the S3 manager for this handler.
// This is useful when the manager is initialized lazily or needs to be replaced.
func (h *S3Handler) SetS3Manager(s3Manager *aws.S3Manager) {
	h.s3Manager = s3Manager
	logging.Debug("Set S3 manager on S3 handler")
}

// SetCache injects a VersionCache for caching resolved versions.
func (h *S3Handler) SetCache(cache *VersionCache) {
	h.cache = cache
	logging.Debug("Set VersionCache on S3 handler")
}

// init registers the S3 handler with the global component registry.
// This is called automatically when the package is imported.
func init() {
	// Register with nil S3 manager - will be set later via SetS3Manager
	// or when ComponentManager initializes
	RegisterComponentType(NewS3Handler(nil))
	logging.Debug("Registered S3 component handler")
}
