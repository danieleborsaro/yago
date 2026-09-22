package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// RecursivePatchConfig defines configuration for recursive patching
type RecursivePatchConfig struct {
	// FilePattern: glob or regex pattern for matching files (e.g., "*.yaml", "config-*.yaml")
	FilePattern string
	// BasePath: directory to search from (defaults to current directory)
	BasePath string
	// UseRegex: if true, FilePattern is treated as regex; if false, as glob pattern
	UseRegex bool
	// Recursive: if true, search recursively in subdirectories
	Recursive bool
	// SkipErrors: if true, continue patching even if individual files fail
	SkipErrors bool
}

// RecursivePatchResult represents the result of a recursive patching operation
type RecursivePatchResult struct {
	// FilesMatched: number of files matching the pattern
	FilesMatched int
	// FilesPatched: number of files successfully patched
	FilesPatched int
	// FilesFailed: number of files that failed to patch
	FilesFailed int
	// Results: map of filepath -> patching results
	Results map[string]*FilePatchResult
	// Errors: map of filepath -> error if any
	Errors map[string]error
}

// FilePatchResult represents the result of patching a single file
type FilePatchResult struct {
	// FilePath: path to the file
	FilePath string
	// Success: whether patching succeeded
	Success bool
	// PatchesApplied: number of patches successfully applied
	PatchesApplied int
	// PatchesFailed: number of patches that failed
	PatchesFailed int
	// Content: resulting content (only if successful)
	Content map[string]interface{}
}

// RecursivePatchEngine provides multi-file patching capabilities
// This enables applying patches across multiple files matching patterns
type RecursivePatchEngine struct {
	patchEngine *JSONPatchEngine
	mergeEngine *StrategicMergeEngine
	yamlHandler *YAMLHandler
	stats       map[string]int
}

// NewRecursivePatchEngine creates a new Recursive Patch Engine
func NewRecursivePatchEngine(basePath string) *RecursivePatchEngine {
	if basePath == "" {
		basePath = "."
	}
	return &RecursivePatchEngine{
		patchEngine: NewJSONPatchEngine(),
		mergeEngine: NewStrategicMergeEngine(nil),
		yamlHandler: NewYAMLHandler(basePath),
		stats: map[string]int{
			"recursive_operations": 0,
			"files_matched":        0,
			"files_patched":        0,
			"files_failed":         0,
		},
	}
}

// PatchFiles applies patches to all files matching the configuration
// Returns a RecursivePatchResult with details about the operation
func (e *RecursivePatchEngine) PatchFiles(config *RecursivePatchConfig, patches []JSONPatchOperation) (*RecursivePatchResult, error) {
	e.stats["recursive_operations"]++

	if config == nil {
		return nil, fmt.Errorf("recursive patch config cannot be nil")
	}

	if config.BasePath == "" {
		config.BasePath = "."
	}

	result := &RecursivePatchResult{
		Results: make(map[string]*FilePatchResult),
		Errors:  make(map[string]error),
	}

	// Find matching files
	matchedFiles, err := e.findMatchingFiles(config)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching files: %v", err)
	}

	result.FilesMatched = len(matchedFiles)
	e.stats["files_matched"] += len(matchedFiles)

	// Patch each matched file
	for _, filePath := range matchedFiles {
		patchResult := e.patchSingleFile(filePath, patches)
		result.Results[filePath] = patchResult

		if patchResult.Success {
			result.FilesPatched++
			e.stats["files_patched"]++
		} else {
			result.FilesFailed++
			e.stats["files_failed"]++
			if len(patchResult.Content) == 0 {
				result.Errors[filePath] = fmt.Errorf("failed to patch file")
			}
		}

		// If SkipErrors is false and a file failed, stop processing
		if !patchResult.Success && !config.SkipErrors {
			return result, fmt.Errorf("failed to patch file %s", filePath)
		}
	}

	return result, nil
}

// MergeFiles applies strategic merges to all files matching the configuration
// Files are merged with the provided source content using the merge strategy
func (e *RecursivePatchEngine) MergeFiles(config *RecursivePatchConfig, source map[string]interface{}, mergeConfig StrategicMergeConfig) (*RecursivePatchResult, error) {
	e.stats["recursive_operations"]++

	if config == nil {
		return nil, fmt.Errorf("recursive patch config cannot be nil")
	}

	if config.BasePath == "" {
		config.BasePath = "."
	}

	result := &RecursivePatchResult{
		Results: make(map[string]*FilePatchResult),
		Errors:  make(map[string]error),
	}

	// Find matching files
	matchedFiles, err := e.findMatchingFiles(config)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching files: %v", err)
	}

	result.FilesMatched = len(matchedFiles)
	e.stats["files_matched"] += len(matchedFiles)

	// Create merge engine
	mergeEngine := NewStrategicMergeEngine(mergeConfig)

	// Merge each matched file
	for _, filePath := range matchedFiles {
		mergeResult := e.mergeSingleFile(filePath, source, mergeEngine)
		result.Results[filePath] = mergeResult

		if mergeResult.Success {
			result.FilesPatched++
			e.stats["files_patched"]++
		} else {
			result.FilesFailed++
			e.stats["files_failed"]++
			if len(mergeResult.Content) == 0 {
				result.Errors[filePath] = fmt.Errorf("failed to merge file")
			}
		}

		if !mergeResult.Success && !config.SkipErrors {
			return result, fmt.Errorf("failed to merge file %s", filePath)
		}
	}

	return result, nil
}

// findMatchingFiles finds all files matching the configuration pattern
func (e *RecursivePatchEngine) findMatchingFiles(config *RecursivePatchConfig) ([]string, error) {
	var matchedFiles []string

	pattern := config.FilePattern
	if pattern == "" {
		pattern = "*.yaml"
	}

	var walkFunc filepath.WalkFunc
	walkFunc = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip if not recursive
			if !config.Recursive && path != config.BasePath {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file matches pattern
		matches, err := e.matchesPattern(path, info.Name(), pattern, config.UseRegex)
		if err != nil {
			return nil // Skip files with pattern errors
		}

		if matches {
			matchedFiles = append(matchedFiles, path)
		}

		return nil
	}

	if config.Recursive {
		err := filepath.Walk(config.BasePath, walkFunc)
		if err != nil {
			return nil, err
		}
	} else {
		// Non-recursive: only list files in the directory
		entries, err := os.ReadDir(config.BasePath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			matches, err := e.matchesPattern(filepath.Join(config.BasePath, entry.Name()), entry.Name(), pattern, config.UseRegex)
			if err != nil {
				continue
			}

			if matches {
				matchedFiles = append(matchedFiles, filepath.Join(config.BasePath, entry.Name()))
			}
		}
	}

	return matchedFiles, nil
}

// matchesPattern checks if a filename matches the given pattern
func (e *RecursivePatchEngine) matchesPattern(fullPath string, filename string, pattern string, useRegex bool) (bool, error) {
	if useRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(filename), nil
	}

	// Glob pattern matching
	matched, err := filepath.Match(pattern, filename)
	if err != nil {
		return false, err
	}
	return matched, nil
}

// patchSingleFile loads a file, applies patches, and returns the result
func (e *RecursivePatchEngine) patchSingleFile(filePath string, patches []JSONPatchOperation) *FilePatchResult {
	result := &FilePatchResult{
		FilePath: filePath,
		Content:  make(map[string]interface{}),
	}

	// Load the file
	content, err := e.yamlHandler.LoadFile(filePath, nil)
	if err != nil {
		return result // Failed: Success=false
	}

	// Apply patches
	patchedContent, err := e.patchEngine.Apply(content, patches)
	if err != nil {
		return result // Failed: Success=false
	}

	result.Success = true
	result.Content = patchedContent
	result.PatchesApplied = len(patches)
	return result
}

// mergeSingleFile loads a file, applies strategic merge, and returns the result
func (e *RecursivePatchEngine) mergeSingleFile(filePath string, source map[string]interface{}, mergeEngine *StrategicMergeEngine) *FilePatchResult {
	result := &FilePatchResult{
		FilePath: filePath,
		Content:  make(map[string]interface{}),
	}

	// Load the file
	content, err := e.yamlHandler.LoadFile(filePath, nil)
	if err != nil {
		return result // Failed: Success=false
	}

	// Apply merge
	mergedContent, err := mergeEngine.Merge(content, source)
	if err != nil {
		return result // Failed: Success=false
	}

	result.Success = true
	result.Content = mergedContent
	return result
}

// WriteResults writes patch results back to files
// Returns number of files written successfully
func (e *RecursivePatchEngine) WriteResults(result *RecursivePatchResult, overwrite bool) (int, error) {
	written := 0

	for filePath, patchResult := range result.Results {
		if !patchResult.Success {
			continue
		}

		// Check if file exists and overwrite is false
		if !overwrite {
			if _, err := os.Stat(filePath); err == nil {
				return written, fmt.Errorf("file %s already exists (overwrite=false)", filePath)
			}
		}

		// Write the file
		file, err := os.Create(filePath)
		if err != nil {
			return written, fmt.Errorf("failed to create file %s: %v", filePath, err)
		}

		err = e.yamlHandler.ToFile(patchResult.Content, file)
		file.Close()

		if err != nil {
			return written, fmt.Errorf("failed to write file %s: %v", filePath, err)
		}

		written++
	}

	return written, nil
}

// GetStats returns engine statistics
func (e *RecursivePatchEngine) GetStats() map[string]int {
	return e.stats
}

// ResetStats resets engine statistics
func (e *RecursivePatchEngine) ResetStats() {
	e.stats = map[string]int{
		"recursive_operations": 0,
		"files_matched":        0,
		"files_patched":        0,
		"files_failed":         0,
	}
}
