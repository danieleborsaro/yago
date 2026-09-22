package desiredstate

import (
	"sync"
	"testing"
	"time"
)

// TestVersionCache_BasicOperations tests basic cache get/set/invalidate operations.
func TestVersionCache_BasicOperations(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 10,
		CleanupInt: 0, // Disable cleanup goroutine for tests
	}
	cache := NewVersionCache(config)

	t.Run("set and get", func(t *testing.T) {
		key := "git:https://github.com/user/repo.git:main"
		value := "abc123def456"

		cache.Set(key, value, ComponentTypeSourcecode)

		retrieved, found := cache.Get(key)
		if !found {
			t.Error("Expected to find cached value")
		}
		if retrieved != value {
			t.Errorf("Expected %s, got %s", value, retrieved)
		}
	})

	t.Run("get non-existent key", func(t *testing.T) {
		_, found := cache.Get("nonexistent:key")
		if found {
			t.Error("Expected not to find non-existent key")
		}
	})

	t.Run("invalidate specific key", func(t *testing.T) {
		key := "git:https://github.com/user/repo.git:develop"
		cache.Set(key, "def456abc123", ComponentTypeSourcecode) //gitleaks:allow

		// Verify it exists
		if _, found := cache.Get(key); !found {
			t.Error("Expected key to exist before invalidation")
		}

		// Invalidate
		cache.Invalidate(key)

		// Verify it's gone
		if _, found := cache.Get(key); found {
			t.Error("Expected key to be invalidated")
		}
	})

	t.Run("clear all entries", func(t *testing.T) {
		cache.Set("key1", "value1", ComponentTypeDocker)
		cache.Set("key2", "value2", ComponentTypeS3)
		cache.Set("key3", "value3", ComponentTypeSourcecode)

		cache.Clear()

		stats := cache.Stats()
		if stats.Entries != 0 {
			t.Errorf("Expected 0 entries after clear, got %d", stats.Entries)
		}
	})
}

// TestVersionCache_TTLExpiration tests that cache entries expire after TTL.
func TestVersionCache_TTLExpiration(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        100 * time.Millisecond,
		MaxEntries: 10,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	key := "git:https://github.com/user/repo.git:feature"
	value := "xyz789abc123"

	cache.Set(key, value, ComponentTypeSourcecode)

	// Should be available immediately
	if _, found := cache.Get(key); !found {
		t.Error("Expected key to be available immediately")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	if _, found := cache.Get(key); found {
		t.Error("Expected key to be expired after TTL")
	}
}

// TestVersionCache_MaxEntries tests that cache respects max entries limit.
func TestVersionCache_MaxEntries(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 3,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	// Add more entries than max
	cache.Set("key1", "value1", ComponentTypeDocker)
	time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	cache.Set("key2", "value2", ComponentTypeS3)
	time.Sleep(10 * time.Millisecond)
	cache.Set("key3", "value3", ComponentTypeSourcecode)
	time.Sleep(10 * time.Millisecond)
	cache.Set("key4", "value4", ComponentTypeDocker)

	stats := cache.Stats()
	if stats.Entries > config.MaxEntries {
		t.Errorf("Expected max %d entries, got %d", config.MaxEntries, stats.Entries)
	}

	// Key1 (oldest) should have been evicted
	if _, found := cache.Get("key1"); found {
		t.Error("Expected oldest entry to be evicted")
	}

	// Key4 (newest) should exist
	if _, found := cache.Get("key4"); !found {
		t.Error("Expected newest entry to exist")
	}
}

// TestVersionCache_ConcurrentAccess tests thread-safe concurrent operations.
func TestVersionCache_ConcurrentAccess(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 100,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	const numGoroutines = 50
	const numOperations = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes and reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := MakeCacheKey(ComponentTypeSourcecode, "repo", string(rune(id)))
				value := "commit" + string(rune(j))

				// Set
				cache.Set(key, value, ComponentTypeSourcecode)

				// Get
				cache.Get(key)

				// Random operation
				if j%2 == 0 {
					cache.Invalidate(key)
				}
			}
		}(i)
	}

	wg.Wait()

	// Cache should not have crashed
	stats := cache.Stats()
	t.Logf("After concurrent access: %s", stats.String())
}

// TestVersionCache_Stats tests cache statistics tracking.
func TestVersionCache_Stats(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 10,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	// Initial stats
	stats := cache.Stats()
	if stats.Entries != 0 || stats.Hits != 0 || stats.Misses != 0 {
		t.Error("Expected initial stats to be zero")
	}

	// Add entry
	key := "test:key:value"
	cache.Set(key, "resolved", ComponentTypeDocker)

	// Cache hit
	cache.Get(key)

	// Cache miss
	cache.Get("nonexistent")

	stats = cache.Stats()
	if stats.Entries != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Entries)
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	expectedHitRate := 50.0 // 1 hit out of 2 total
	if stats.HitRate != expectedHitRate {
		t.Errorf("Expected hit rate %.1f%%, got %.1f%%", expectedHitRate, stats.HitRate)
	}
}

// TestVersionCache_InvalidatePattern tests pattern-based invalidation.
func TestVersionCache_InvalidatePattern(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 20,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	// Add entries for different repos
	cache.Set("git:https://github.com/user/repo1.git:main", "commit1", ComponentTypeSourcecode)
	cache.Set("git:https://github.com/user/repo1.git:develop", "commit2", ComponentTypeSourcecode)
	cache.Set("git:https://github.com/user/repo2.git:main", "commit3", ComponentTypeSourcecode)
	cache.Set("docker:registry/image:tag", "digest", ComponentTypeDocker)

	// Invalidate all repo1 entries
	count := cache.InvalidatePattern("git:https://github.com/user/repo1.git")

	if count != 2 {
		t.Errorf("Expected to invalidate 2 entries, got %d", count)
	}

	// Verify repo1 entries are gone
	if _, found := cache.Get("git:https://github.com/user/repo1.git:main"); found {
		t.Error("Expected repo1:main to be invalidated")
	}
	if _, found := cache.Get("git:https://github.com/user/repo1.git:develop"); found {
		t.Error("Expected repo1:develop to be invalidated")
	}

	// Verify repo2 and docker entries still exist
	if _, found := cache.Get("git:https://github.com/user/repo2.git:main"); !found {
		t.Error("Expected repo2:main to still exist")
	}
	if _, found := cache.Get("docker:registry/image:tag"); !found {
		t.Error("Expected docker entry to still exist")
	}
}

// TestVersionCache_Cleanup tests automatic cleanup of expired entries.
func TestVersionCache_Cleanup(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        50 * time.Millisecond,
		MaxEntries: 10,
		CleanupInt: 0, // Manual cleanup for testing
	}
	cache := NewVersionCache(config)

	// Add entries
	cache.Set("key1", "value1", ComponentTypeDocker)
	cache.Set("key2", "value2", ComponentTypeS3)
	cache.Set("key3", "value3", ComponentTypeSourcecode)

	stats := cache.Stats()
	if stats.Entries != 3 {
		t.Errorf("Expected 3 entries before cleanup, got %d", stats.Entries)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Manual cleanup
	cache.cleanup()

	stats = cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", stats.Entries)
	}
}

// TestVersionCache_Disabled tests that disabled cache returns no results.
func TestVersionCache_Disabled(t *testing.T) {
	config := CacheConfig{
		Enabled: false,
		TTL:     1 * time.Minute,
	}
	cache := NewVersionCache(config)

	if cache != nil {
		t.Error("Expected nil cache when disabled")
	}

	// All operations should be no-ops
	cache.Set("key", "value", ComponentTypeDocker)
	_, found := cache.Get("key")
	if found {
		t.Error("Expected no result from disabled cache")
	}

	stats := cache.Stats()
	if stats.Enabled {
		t.Error("Expected stats to show cache as disabled")
	}
}

// TestMakeCacheKey tests cache key generation.
func TestMakeCacheKey(t *testing.T) {
	tests := []struct {
		name       string
		compType   ComponentType
		identifier string
		version    string
		expected   string
	}{
		{
			name:       "docker image",
			compType:   ComponentTypeDocker,
			identifier: "123456789.dkr.ecr.us-east-1.amazonaws.com/my-app",
			version:    "v1.2.3",
			expected:   "docker:123456789.dkr.ecr.us-east-1.amazonaws.com/my-app:v1.2.3",
		},
		{
			name:       "s3 object",
			compType:   ComponentTypeS3,
			identifier: "s3://my-bucket/path/to/object",
			version:    "abc123",
			expected:   "s3:s3://my-bucket/path/to/object:abc123",
		},
		{
			name:       "git repository",
			compType:   ComponentTypeSourcecode,
			identifier: "https://github.com/user/repo.git",
			version:    "main",
			expected:   "sourcecode:https://github.com/user/repo.git:main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := MakeCacheKey(tt.compType, tt.identifier, tt.version)
			if key != tt.expected {
				t.Errorf("Expected key %s, got %s", tt.expected, key)
			}
		})
	}
}

// TestVersionCache_NilSafety tests that nil cache doesn't cause panics.
func TestVersionCache_NilSafety(t *testing.T) {
	var cache *VersionCache // nil cache

	// All these should be no-ops and not panic
	cache.Set("key", "value", ComponentTypeDocker)
	_, _ = cache.Get("key")
	cache.Invalidate("key")
	_ = cache.InvalidatePattern("pattern")
	cache.Clear()
	_ = cache.Stats()

	t.Log("✓ Nil cache operations didn't panic")
}

// TestComponentManager_SetVersionCache tests cache propagation through ComponentManager.
func TestComponentManager_SetVersionCache(t *testing.T) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 100,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	cm := NewComponentManager()
	cm.SetVersionCache(cache)

	// Verify Git handler received the cache
	gitHandler, err := GetComponentHandler(ComponentTypeSourcecode)
	if err != nil {
		t.Fatalf("Failed to get Git handler: %v", err)
	}

	if gh, ok := gitHandler.(*GitHandler); ok {
		if gh.cache == nil {
			t.Error("Cache was not set on Git handler")
		}
	} else {
		t.Fatalf("Handler is not a GitHandler")
	}

	// Verify Docker handler received the cache
	dockerHandler, err := GetComponentHandler(ComponentTypeDocker)
	if err != nil {
		t.Fatalf("Failed to get Docker handler: %v", err)
	}

	if dh, ok := dockerHandler.(*DockerHandler); ok {
		if dh.cache == nil {
			t.Error("Cache was not set on Docker handler")
		}
	} else {
		t.Fatalf("Handler is not a DockerHandler")
	}

	// Verify S3 handler received the cache
	s3Handler, err := GetComponentHandler(ComponentTypeS3)
	if err != nil {
		t.Fatalf("Failed to get S3 handler: %v", err)
	}

	if sh, ok := s3Handler.(*S3Handler); ok {
		if sh.cache == nil {
			t.Error("Cache was not set on S3 handler")
		}
	} else {
		t.Fatalf("Handler is not a S3Handler")
	}

	t.Log("✓ Cache successfully propagated to Git, Docker, and S3 handlers")
}

// BenchmarkVersionCache_Get benchmarks cache read performance.
func BenchmarkVersionCache_Get(b *testing.B) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 1000,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		key := MakeCacheKey(ComponentTypeSourcecode, "repo", string(rune(i)))
		cache.Set(key, "commit"+string(rune(i)), ComponentTypeSourcecode)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := MakeCacheKey(ComponentTypeSourcecode, "repo", string(rune(i%100)))
		cache.Get(key)
	}
}

// BenchmarkVersionCache_Set benchmarks cache write performance.
func BenchmarkVersionCache_Set(b *testing.B) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 1000,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := MakeCacheKey(ComponentTypeSourcecode, "repo", string(rune(i%100)))
		cache.Set(key, "commit"+string(rune(i)), ComponentTypeSourcecode)
	}
}

// BenchmarkVersionCache_Concurrent benchmarks concurrent access performance.
func BenchmarkVersionCache_Concurrent(b *testing.B) {
	config := CacheConfig{
		Enabled:    true,
		TTL:        1 * time.Minute,
		MaxEntries: 1000,
		CleanupInt: 0,
	}
	cache := NewVersionCache(config)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := MakeCacheKey(ComponentTypeSourcecode, "repo", string(rune(i%100)))
			if i%2 == 0 {
				cache.Set(key, "commit"+string(rune(i)), ComponentTypeSourcecode)
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}
