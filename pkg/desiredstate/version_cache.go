package desiredstate

import (
	"fmt"
	"sync"
	"time"

	"github.com/danieleborsaro/yago/internal/utils/logging"
)

// VersionCache provides thread-safe caching for resolved component versions.
// It uses TTL-based expiration to ensure cached versions don't become stale.
//
// Cache Keys:
//   - Docker: "docker:<registry>/<repository>:<tag>"
//   - S3: "s3://<bucket>/<key>:<version>"
//   - Git: "git:<url>@<ref>"
//
// The cache is optional and handlers gracefully degrade without it.
type VersionCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	config  CacheConfig

	// Metrics
	hits   uint64
	misses uint64
}

// CacheEntry represents a single cached version with expiration.
type CacheEntry struct {
	ResolvedVersion string
	CachedAt        time.Time
	ExpiresAt       time.Time
	ComponentType   ComponentType
}

// CacheConfig controls cache behavior.
type CacheConfig struct {
	Enabled    bool          // Enable/disable caching
	TTL        time.Duration // Time-to-live for cache entries
	MaxEntries int           // Maximum number of entries (0 = unlimited)
	CleanupInt time.Duration // Cleanup interval for expired entries
}

// DefaultCacheConfig returns sensible defaults for version caching.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:    true,
		TTL:        5 * time.Minute, // 5 minutes default TTL
		MaxEntries: 1000,            // Limit to 1000 entries
		CleanupInt: 1 * time.Minute, // Cleanup every minute
	}
}

// NewVersionCache creates a new version cache with the given configuration.
func NewVersionCache(config CacheConfig) *VersionCache {
	if !config.Enabled {
		logging.Debug("Version cache disabled")
		return nil
	}

	cache := &VersionCache{
		entries: make(map[string]*CacheEntry),
		config:  config,
	}

	// Start background cleanup goroutine if cleanup interval is set
	if config.CleanupInt > 0 {
		go cache.cleanupLoop()
	}

	logging.Info("Version cache initialized (TTL: %s, MaxEntries: %d)", config.TTL, config.MaxEntries)
	return cache
}

// Get retrieves a cached version if available and not expired.
// Returns the resolved version and true if found and valid, empty string and false otherwise.
func (c *VersionCache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return "", false
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.misses++
		logging.Debug("Cache entry expired: %s", key)
		return "", false
	}

	c.hits++
	logging.Debug("Cache hit: %s -> %s (age: %s)", key, entry.ResolvedVersion, time.Since(entry.CachedAt))
	return entry.ResolvedVersion, true
}

// Set stores a resolved version in the cache with TTL expiration.
func (c *VersionCache) Set(key string, resolvedVersion string, componentType ComponentType) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries (simple FIFO if over limit)
	if c.config.MaxEntries > 0 && len(c.entries) >= c.config.MaxEntries {
		// Remove oldest entry (simple strategy - could be improved with LRU)
		var oldestKey string
		var oldestTime time.Time
		first := true

		for k, v := range c.entries {
			if first || v.CachedAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.CachedAt
				first = false
			}
		}

		if oldestKey != "" {
			delete(c.entries, oldestKey)
			logging.Debug("Evicted oldest cache entry: %s", oldestKey)
		}
	}

	now := time.Now()
	entry := &CacheEntry{
		ResolvedVersion: resolvedVersion,
		CachedAt:        now,
		ExpiresAt:       now.Add(c.config.TTL),
		ComponentType:   componentType,
	}

	c.entries[key] = entry
	logging.Debug("Cache set: %s -> %s (expires: %s)", key, resolvedVersion, entry.ExpiresAt.Format(time.RFC3339))
}

// Invalidate removes a specific cache entry.
func (c *VersionCache) Invalidate(key string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		delete(c.entries, key)
		logging.Debug("Cache invalidated: %s", key)
	}
}

// InvalidatePattern removes all cache entries matching a pattern.
// Useful for invalidating all entries for a specific repository or component.
func (c *VersionCache) InvalidatePattern(pattern string) int {
	if c == nil {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.entries {
		if matchesPattern(key, pattern) {
			delete(c.entries, key)
			count++
		}
	}

	if count > 0 {
		logging.Debug("Cache invalidated %d entries matching pattern: %s", count, pattern)
	}

	return count
}

// Clear removes all cache entries.
func (c *VersionCache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.entries)
	c.entries = make(map[string]*CacheEntry)
	logging.Debug("Cache cleared: %d entries removed", count)
}

// Stats returns current cache statistics.
func (c *VersionCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Entries: len(c.entries),
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: 0.0,
		Enabled: c.config.Enabled,
		TTL:     c.config.TTL,
		MaxSize: c.config.MaxEntries,
	}

	total := c.hits + c.misses
	if total > 0 {
		stats.HitRate = float64(c.hits) / float64(total) * 100.0
	}

	return stats
}

// CacheStats provides cache performance metrics.
type CacheStats struct {
	Entries int           // Current number of entries
	Hits    uint64        // Total cache hits
	Misses  uint64        // Total cache misses
	HitRate float64       // Hit rate percentage
	Enabled bool          // Whether cache is enabled
	TTL     time.Duration // Configured TTL
	MaxSize int           // Maximum entries
}

// String returns a human-readable representation of cache stats.
func (s CacheStats) String() string {
	if !s.Enabled {
		return "Cache: disabled"
	}
	return fmt.Sprintf("Cache: %d entries, %d hits, %d misses, %.1f%% hit rate",
		s.Entries, s.Hits, s.Misses, s.HitRate)
}

// cleanupLoop periodically removes expired entries.
func (c *VersionCache) cleanupLoop() {
	ticker := time.NewTicker(c.config.CleanupInt)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries from the cache.
func (c *VersionCache) cleanup() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
			removed++
		}
	}

	if removed > 0 {
		logging.Debug("Cache cleanup: removed %d expired entries", removed)
	}
}

// matchesPattern checks if a key matches a pattern.
// Simple implementation - could be enhanced with regex or glob patterns.
func matchesPattern(key, pattern string) bool {
	// For now, simple prefix matching
	// Could be enhanced with more sophisticated matching
	if len(pattern) == 0 {
		return false
	}

	// Match prefix
	if len(key) >= len(pattern) {
		return key[:len(pattern)] == pattern
	}

	return false
}

// MakeCacheKey creates a standardized cache key for a component.
// This ensures consistent key formatting across handlers.
func MakeCacheKey(componentType ComponentType, identifier string, version string) string {
	return fmt.Sprintf("%s:%s:%s", componentType, identifier, version)
}
