package desiredstate

import (
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// CompareVersions compares two version strings intelligently.
// Returns:
//   - negative if v1 < v2 (upgrade)
//   - zero if v1 == v2 (unchanged)
//   - positive if v1 > v2 (downgrade)
//
// Attempts semantic versioning first, falls back to string comparison.
func CompareVersions(v1, v2 string) int {
	// If versions are identical, return early
	if v1 == v2 {
		return 0
	}

	// Try semantic versioning comparison
	if result, ok := compareSemver(v1, v2); ok {
		return result
	}

	// Fall back to string comparison
	return strings.Compare(v1, v2)
}

// compareSemver attempts to parse and compare versions as semantic versions.
// Returns (comparison_result, success).
func compareSemver(v1, v2 string) (int, bool) {
	// Clean version strings - remove common prefixes
	v1Clean := cleanVersionString(v1)
	v2Clean := cleanVersionString(v2)

	// Try to parse as semantic versions
	sem1, err1 := semver.NewVersion(v1Clean)
	sem2, err2 := semver.NewVersion(v2Clean)

	// Only use semver comparison if both versions parse successfully
	if err1 != nil || err2 != nil {
		logging.Debug("Version strings '%s' and '%s' are not semantic versions, using string comparison", v1, v2)
		return 0, false
	}

	// Compare semantic versions
	result := sem1.Compare(sem2)
	logging.Debug("Semantic version comparison: '%s' vs '%s' = %d", v1, v2, result)
	return result, true
}

// cleanVersionString removes common prefixes and normalizes version strings
// for semantic version parsing.
func cleanVersionString(version string) string {
	// Remove common prefixes
	cleaned := version
	cleaned = strings.TrimPrefix(cleaned, "v")
	cleaned = strings.TrimPrefix(cleaned, "V")
	cleaned = strings.TrimPrefix(cleaned, "version-")
	cleaned = strings.TrimPrefix(cleaned, "release-")

	return cleaned
}

// IsVersionUpgrade checks if v2 is an upgrade from v1.
func IsVersionUpgrade(v1, v2 string) bool {
	return CompareVersions(v1, v2) < 0
}

// IsVersionDowngrade checks if v2 is a downgrade from v1.
func IsVersionDowngrade(v1, v2 string) bool {
	return CompareVersions(v1, v2) > 0
}

// IsVersionUnchanged checks if v1 and v2 are the same version.
func IsVersionUnchanged(v1, v2 string) bool {
	return CompareVersions(v1, v2) == 0
}
