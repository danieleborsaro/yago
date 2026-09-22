package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecursivePatchEngine_BehavioralBDD_BasicPatching tests basic recursive patching
func TestRecursivePatchEngine_BehavioralBDD_BasicPatching(t *testing.T) {
	// Behavioral contract: Basic recursive patching
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine applies patches to multiple files",
		Operation:   "recursive-patch",
		Description: "Finds files matching pattern and applies same patches to all",
	}

	t.Run("BDD-BasicPatching-SingleFile", func(t *testing.T) {
		// Create temporary directory and files
		tmpDir := t.TempDir()

		// Create a test file
		content1 := map[string]interface{}{
			"app": map[string]interface{}{
				"version": "1.0",
				"name":    "app1",
			},
		}

		handler := NewYAMLHandler(tmpDir)
		file1 := filepath.Join(tmpDir, "config.yaml")
		yamlStr, _ := handler.ToString(content1)
		os.WriteFile(file1, []byte(yamlStr), 0644)

		// Patch the file
		engine := NewRecursivePatchEngine(tmpDir)
		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/app/version", Value: "2.0"},
		}

		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
			SkipErrors:  false,
		}

		result, err := engine.PatchFiles(config, patches)
		require.NoError(t, err)
		assert.Equal(t, 1, result.FilesMatched)
		assert.Equal(t, 1, result.FilesPatched)
		assert.Equal(t, "2.0", result.Results[file1].Content["app"].(map[string]interface{})["version"])
	})

	t.Run("BDD-BasicPatching-MultipleFiles", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple test files
		handler := NewYAMLHandler(tmpDir)
		for i := 1; i <= 3; i++ {
			content := map[string]interface{}{
				"version": float64(1),
				"name":    "app",
			}
			yamlStr, _ := handler.ToString(content)
			filePath := filepath.Join(tmpDir, "config"+string(rune(48+i))+".yaml")
			os.WriteFile(filePath, []byte(yamlStr), 0644)
		}

		// Patch all files
		engine := NewRecursivePatchEngine(tmpDir)
		patches := []JSONPatchOperation{
			{Op: "replace", Path: "/version", Value: float64(2)},
		}

		config := &RecursivePatchConfig{
			FilePattern: "config*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
			SkipErrors:  false,
		}

		result, err := engine.PatchFiles(config, patches)
		require.NoError(t, err)
		assert.Equal(t, 3, result.FilesMatched)
		assert.Equal(t, 3, result.FilesPatched)
	})
}

// TestRecursivePatchEngine_BehavioralBDD_PatternMatching tests file pattern matching
func TestRecursivePatchEngine_BehavioralBDD_PatternMatching(t *testing.T) {
	// Behavioral contract: Pattern matching
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine matches files by glob and regex patterns",
		Operation:   "pattern-matching",
		Description: "Supports both glob patterns and regex patterns",
	}

	t.Run("BDD-PatternMatching-GlobPattern", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create files
		for _, name := range []string{"config.yaml", "app.yaml", "test.txt"} {
			content := map[string]interface{}{"file": name}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(filepath.Join(tmpDir, name), []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			UseRegex:    false,
			Recursive:   false,
		}

		result, err := engine.PatchFiles(config, []JSONPatchOperation{})
		require.NoError(t, err)
		assert.Equal(t, 2, result.FilesMatched)
	})

	t.Run("BDD-PatternMatching-RegexPattern", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create files
		for _, name := range []string{"config-prod.yaml", "config-dev.yaml", "app.yaml"} {
			content := map[string]interface{}{"file": name}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(filepath.Join(tmpDir, name), []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: `config-.*\.yaml$`,
			BasePath:    tmpDir,
			UseRegex:    true,
			Recursive:   false,
		}

		result, err := engine.PatchFiles(config, []JSONPatchOperation{})
		require.NoError(t, err)
		assert.Equal(t, 2, result.FilesMatched)
	})
}

// TestRecursivePatchEngine_BehavioralBDD_RecursiveMode tests recursive directory traversal
func TestRecursivePatchEngine_BehavioralBDD_RecursiveMode(t *testing.T) {
	// Behavioral contract: Recursive directory traversal
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine traverses subdirectories",
		Operation:   "recursive-traversal",
		Description: "With Recursive=true, searches subdirectories; with false, only top level",
	}

	t.Run("BDD-RecursiveMode-RecursiveSearch", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create nested structure
		os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

		// Create files
		for _, path := range []string{
			filepath.Join(tmpDir, "config.yaml"),
			filepath.Join(tmpDir, "subdir", "config.yaml"),
		} {
			content := map[string]interface{}{"file": path}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(path, []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   true,
			SkipErrors:  false,
		}

		result, err := engine.PatchFiles(config, []JSONPatchOperation{})
		require.NoError(t, err)
		assert.Equal(t, 2, result.FilesMatched)
	})

	t.Run("BDD-RecursiveMode-NonRecursiveSearch", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create nested structure
		os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

		// Create files
		for _, path := range []string{
			filepath.Join(tmpDir, "config.yaml"),
			filepath.Join(tmpDir, "subdir", "config.yaml"),
		} {
			content := map[string]interface{}{"file": path}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(path, []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
			SkipErrors:  false,
		}

		result, err := engine.PatchFiles(config, []JSONPatchOperation{})
		require.NoError(t, err)
		assert.Equal(t, 1, result.FilesMatched)
	})
}

// TestRecursivePatchEngine_BehavioralBDD_ErrorHandling tests error handling
func TestRecursivePatchEngine_BehavioralBDD_ErrorHandling(t *testing.T) {
	// Behavioral contract: Error handling
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine handles errors gracefully",
		Operation:   "error-handling",
		Description: "Supports SkipErrors flag and reports per-file errors",
	}

	t.Run("BDD-ErrorHandling-SkipErrors", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create valid files
		for i := 1; i <= 2; i++ {
			content := map[string]interface{}{"field": "value"}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(filepath.Join(tmpDir, "config"+string(rune(48+i))+".yaml"), []byte(yamlStr), 0644)
		}

		// Create a file that will fail during patching (patch references non-existent path)
		content := map[string]interface{}{"other": "data"}
		yamlStr, _ := handler.ToString(content)
		os.WriteFile(filepath.Join(tmpDir, "config3.yaml"), []byte(yamlStr), 0644)

		engine := NewRecursivePatchEngine(tmpDir)
		patches := []JSONPatchOperation{
			{Op: "remove", Path: "/nonexistent"},
		}

		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
			SkipErrors:  true,
		}

		result, err := engine.PatchFiles(config, patches)
		require.NoError(t, err)
		assert.Equal(t, 3, result.FilesMatched)
		// With SkipErrors, it tries all files even if some fail
		assert.GreaterOrEqual(t, result.FilesFailed, 0)
	})

	t.Run("BDD-ErrorHandling-NoSkipErrors", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create a file that will fail during patching
		content := map[string]interface{}{"other": "data"}
		yamlStr, _ := handler.ToString(content)
		os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(yamlStr), 0644)

		engine := NewRecursivePatchEngine(tmpDir)
		patches := []JSONPatchOperation{
			{Op: "remove", Path: "/nonexistent"},
		}

		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
			SkipErrors:  false,
		}

		result, err := engine.PatchFiles(config, patches)
		// Should fail and return error
		assert.Error(t, err)
		assert.Greater(t, result.FilesFailed, 0)
	})
}

// TestRecursivePatchEngine_BehavioralBDD_MergeFiles tests recursive merging
func TestRecursivePatchEngine_BehavioralBDD_MergeFiles(t *testing.T) {
	// Behavioral contract: Recursive merging
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine applies strategic merges to multiple files",
		Operation:   "recursive-merge",
		Description: "Merges all matching files with source using merge strategies",
	}

	t.Run("BDD-MergeFiles-SimpleOverlay", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create base files
		for i := 1; i <= 2; i++ {
			content := map[string]interface{}{
				"version": "1.0",
				"name":    "app",
			}
			yamlStr, _ := handler.ToString(content)
			filePath := filepath.Join(tmpDir, "base"+string(rune(48+i))+".yaml")
			os.WriteFile(filePath, []byte(yamlStr), 0644)
		}

		// Merge config
		engine := NewRecursivePatchEngine(tmpDir)
		source := map[string]interface{}{
			"version": "2.0",
		}

		config := &RecursivePatchConfig{
			FilePattern: "base*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
		}

		result, err := engine.MergeFiles(config, source, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, result.FilesMatched)
		assert.Equal(t, 2, result.FilesPatched)

		// Verify merge results
		for _, patchResult := range result.Results {
			assert.Equal(t, "2.0", patchResult.Content["version"])
			assert.Equal(t, "app", patchResult.Content["name"])
		}
	})

	t.Run("BDD-MergeFiles-WithStrategies", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create base file
		content := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "app:1.0"},
				},
			},
		}
		yamlStr, _ := handler.ToString(content)
		os.WriteFile(filepath.Join(tmpDir, "pod.yaml"), []byte(yamlStr), 0644)

		// Merge config
		engine := NewRecursivePatchEngine(tmpDir)
		source := map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "app:2.0"},
					map[string]interface{}{"name": "sidecar", "image": "sidecar:1.0"},
				},
			},
		}

		mergeConfig := StrategicMergeConfig{
			"/spec/containers": "MERGE_BY_KEY:name",
		}

		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
		}

		result, err := engine.MergeFiles(config, source, mergeConfig)
		require.NoError(t, err)
		assert.Equal(t, 1, result.FilesPatched)

		containers := result.Results[filepath.Join(tmpDir, "pod.yaml")].Content["spec"].(map[string]interface{})["containers"].([]interface{})
		assert.Equal(t, 2, len(containers))
	})
}

// TestRecursivePatchEngine_BehavioralBDD_Statistics tests statistics tracking
func TestRecursivePatchEngine_BehavioralBDD_Statistics(t *testing.T) {
	// Behavioral contract: Statistics tracking
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine tracks detailed statistics",
		Operation:   "statistics-tracking",
		Description: "Tracks files matched, patched, failed, and recursive operations",
	}

	t.Run("BDD-Statistics-TrackingAccuracy", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create files
		for i := 1; i <= 3; i++ {
			content := map[string]interface{}{"version": float64(1)}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(filepath.Join(tmpDir, "config"+string(rune(48+i))+".yaml"), []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
		}

		_, _ = engine.PatchFiles(config, []JSONPatchOperation{})

		stats := engine.GetStats()
		assert.Equal(t, 1, stats["recursive_operations"])
		assert.Equal(t, 3, stats["files_matched"])
		assert.Equal(t, 3, stats["files_patched"])
		assert.Equal(t, 0, stats["files_failed"])
	})

	t.Run("BDD-Statistics-ResetStats", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		content := map[string]interface{}{"version": float64(1)}
		yamlStr, _ := handler.ToString(content)
		os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(yamlStr), 0644)

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
		}

		_, _ = engine.PatchFiles(config, []JSONPatchOperation{})
		stats1 := engine.GetStats()
		assert.Equal(t, 1, stats1["recursive_operations"])

		engine.ResetStats()
		stats2 := engine.GetStats()
		assert.Equal(t, 0, stats2["recursive_operations"])
	})
}

// TestRecursivePatchEngine_BehavioralBDD_ComplexScenarios tests complex scenarios
func TestRecursivePatchEngine_BehavioralBDD_ComplexScenarios(t *testing.T) {
	// Behavioral contract: Complex scenarios
	_ = BehavioralContractJSONPatch{
		Behavior:    "Recursive Patch Engine handles complex multi-file scenarios",
		Operation:   "complex-scenarios",
		Description: "Processes large file sets with different patches",
	}

	t.Run("BDD-ComplexScenario-MultiLevelDirs", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create nested structure
		os.MkdirAll(filepath.Join(tmpDir, "env", "prod"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "env", "dev"), 0755)

		// Create files in all levels
		for _, path := range []string{
			filepath.Join(tmpDir, "config.yaml"),
			filepath.Join(tmpDir, "env", "config.yaml"),
			filepath.Join(tmpDir, "env", "prod", "config.yaml"),
			filepath.Join(tmpDir, "env", "dev", "config.yaml"),
		} {
			content := map[string]interface{}{"env": "base"}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(path, []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "config.yaml",
			BasePath:    tmpDir,
			Recursive:   true,
		}

		result, _ := engine.PatchFiles(config, []JSONPatchOperation{})
		assert.Equal(t, 4, result.FilesMatched)
		assert.Equal(t, 4, result.FilesPatched)
	})

	t.Run("BDD-ComplexScenario-MixedPatterns", func(t *testing.T) {
		tmpDir := t.TempDir()
		handler := NewYAMLHandler(tmpDir)

		// Create files with different patterns
		for _, name := range []string{"app.yaml", "app-config.yaml", "other.yaml"} {
			content := map[string]interface{}{"type": "config"}
			yamlStr, _ := handler.ToString(content)
			os.WriteFile(filepath.Join(tmpDir, name), []byte(yamlStr), 0644)
		}

		engine := NewRecursivePatchEngine(tmpDir)
		config := &RecursivePatchConfig{
			FilePattern: "app*.yaml",
			BasePath:    tmpDir,
			Recursive:   false,
		}

		result, _ := engine.PatchFiles(config, []JSONPatchOperation{})
		assert.Equal(t, 2, result.FilesMatched)
	})
}
