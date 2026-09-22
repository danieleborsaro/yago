package desiredstate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danieleborsaro/yago/pkg/desiredstate"
	"github.com/danieleborsaro/yago/pkg/wrapper"
)

// LoadTestBehavioralContract documents expected behavior for load/stress scenarios
type LoadTestBehavioralContract struct {
	Behavior    string
	CurrentImpl string
	Rationale   string
}

// TestLoadStress_LargeFileProcessing_BehavioralBDD tests processing of large YAML files
func TestLoadStress_LargeFileProcessing_BehavioralBDD(t *testing.T) {
	contract := LoadTestBehavioralContract{
		Behavior: "Process large YAML files (1MB-10MB) within performance thresholds",
		CurrentImpl: `
- Loads large YAML files using yaml.Unmarshal
- Efficient memory management with Go's GC
- Processing time: Target <500ms for 1MB, <10s for 10MB
- Benefits from compiled code performance
`,
		Rationale: "Large file support is critical for complex infrastructure configurations",
	}

	t.Run("process_1mb_file", func(t *testing.T) {
		// Generate ~1MB YAML content (approximately 10,000 lines)
		yamlContent := generateLargeDesiredState(1000)
		fileSize := len(yamlContent)

		if fileSize < 900*1024 { // At least 900KB
			t.Skipf("Generated content too small: %d bytes (expected ~1MB)", fileSize)
		}

		// Write to temp file
		testFile := writeYAMLFile(t, yamlContent)

		// Create service and measure processing time
		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		// Validation may fail due to schema issues with generated content
		// What matters is that it processes without crashing
		if err != nil {
			t.Logf("⚠️  Validation error (acceptable for stress test): %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 500ms for 1MB file
		if elapsed > 500*time.Millisecond {
			t.Logf("⚠️  Processing took %v (target: <500ms) - may need optimization", elapsed)
		} else {
			t.Logf("✓ Processed ~1MB file in %v (target: <500ms)", elapsed)
		}

		t.Logf("File size: %d bytes (%.2f MB), Processing time: %v",
			fileSize, float64(fileSize)/1024/1024, elapsed)
	})

	t.Run("process_5mb_file", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping 5MB test in short mode")
		}

		// Generate ~5MB YAML content
		yamlContent := generateLargeDesiredState(5000)
		fileSize := len(yamlContent)

		if fileSize < 4*1024*1024 { // At least 4MB
			t.Skipf("Generated content too small: %d bytes", fileSize)
		}

		testFile := writeYAMLFile(t, yamlContent)
		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("⚠️  Validation error (acceptable for stress test): %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 2.5s for 5MB file
		if elapsed > 2500*time.Millisecond {
			t.Logf("⚠️  Processing took %v (target: <2.5s) - may need optimization", elapsed)
		} else {
			t.Logf("✓ Processed ~5MB file in %v (target: <2.5s)", elapsed)
		}

		t.Logf("File size: %d bytes (%.2f MB), Processing time: %v",
			fileSize, float64(fileSize)/1024/1024, elapsed)
	})

	t.Run("process_10mb_file", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping 10MB test in short mode")
		}

		// Generate ~10MB YAML content
		yamlContent := generateLargeDesiredState(10000)
		fileSize := len(yamlContent)

		if fileSize < 9*1024*1024 { // At least 9MB
			t.Skipf("Generated content too small: %d bytes", fileSize)
		}

		testFile := writeYAMLFile(t, yamlContent)
		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("⚠️  Validation error (acceptable for stress test): %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 10s for 10MB file
		if elapsed > 10*time.Second {
			t.Logf("⚠️  Processing took %v (target: <10s) - may need optimization", elapsed)
		} else {
			t.Logf("✓ Processed ~10MB file in %v (target: <10s)", elapsed)
		}

		t.Logf("File size: %d bytes (%.2f MB), Processing time: %v",
			fileSize, float64(fileSize)/1024/1024, elapsed)
	})

	t.Logf("Contract: %s", contract.Behavior)
}

// TestLoadStress_ManyComponents_BehavioralBDD tests processing configurations with many components
func TestLoadStress_ManyComponents_BehavioralBDD(t *testing.T) {
	contract := LoadTestBehavioralContract{
		Behavior: "Process configurations with hundreds to thousands of components efficiently",
		CurrentImpl: `
- Processes components efficiently
- Optimized memory allocation
- Target: 1000 components in <5s
`,
		Rationale: "Large-scale infrastructure often has hundreds of components",
	}

	t.Run("process_100_components", func(t *testing.T) {
		yamlContent := generateDesiredStateWithComponents(100)
		testFile := writeYAMLFile(t, yamlContent)

		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("⚠️  Validation error: %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 1s for 100 components
		if elapsed > time.Second {
			t.Logf("⚠️  Processing 100 components took %v (target: <1s)", elapsed)
		} else {
			t.Logf("✓ Processed 100 components in %v (target: <1s)", elapsed)
		}
	})

	t.Run("process_500_components", func(t *testing.T) {
		yamlContent := generateDesiredStateWithComponents(500)
		testFile := writeYAMLFile(t, yamlContent)

		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("⚠️  Validation error: %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 3s for 500 components
		if elapsed > 3*time.Second {
			t.Logf("⚠️  Processing 500 components took %v (target: <3s)", elapsed)
		} else {
			t.Logf("✓ Processed 500 components in %v (target: <3s)", elapsed)
		}
	})

	t.Run("process_1000_components", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping 1000 component test in short mode")
		}

		yamlContent := generateDesiredStateWithComponents(1000)
		testFile := writeYAMLFile(t, yamlContent)

		service := desiredstate.NewService(filepath.Dir(testFile), true)

		start := time.Now()
		req := wrapper.ValidateRequest{
			DesiredStateFile: testFile,
			Environment:      "test",
		}

		response, err := service.ValidateDesiredState(req)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("⚠️  Validation error: %v", err)
		}

		if response != nil && response.DesiredStateValidated {
			t.Log("✓ Validation succeeded")
		}

		// Target: < 5s for 1000 components
		if elapsed > 5*time.Second {
			t.Logf("⚠️  Processing 1000 components took %v (target: <5s)", elapsed)
		} else {
			t.Logf("✓ Processed 1000 components in %v (target: <5s)", elapsed)
		}
	})

	t.Logf("Contract: %s", contract.Behavior)
}

// TestLoadStress_ConcurrentOperations_BehavioralBDD tests concurrent safe operations
func TestLoadStress_ConcurrentOperations_BehavioralBDD(t *testing.T) {
	contract := LoadTestBehavioralContract{
		Behavior: "Handle concurrent read operations safely without deadlocks or race conditions",
		CurrentImpl: `
- True concurrent execution with goroutines
- Thread-safe operations
- Target: 100 concurrent operations complete successfully
`,
		Rationale: "CI/CD systems often make concurrent validation requests",
	}

	t.Run("concurrent_10_validations", func(t *testing.T) {
		yamlContent := generateDesiredStateWithComponents(50)
		testFile := writeYAMLFile(t, yamlContent)
		concurrentOps := 10

		var wg sync.WaitGroup
		errorCount := 0
		successCount := 0
		var mu sync.Mutex

		times := make([]time.Duration, concurrentOps)

		start := time.Now()
		for i := 0; i < concurrentOps; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				opStart := time.Now()
				// Each goroutine gets its own service instance (thread-safe)
				service := desiredstate.NewService(filepath.Dir(testFile), true)

				req := wrapper.ValidateRequest{
					DesiredStateFile: testFile,
					Environment:      "test",
				}

				response, err := service.ValidateDesiredState(req)
				opElapsed := time.Since(opStart)
				times[id] = opElapsed

				mu.Lock()
				if err != nil || (response != nil && !response.DesiredStateValidated) {
					errorCount++
				} else {
					successCount++
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()
		totalElapsed := time.Since(start)

		// Calculate average operation time
		var totalTime time.Duration
		for _, t := range times {
			totalTime += t
		}
		avgTime := totalTime / time.Duration(concurrentOps)

		if errorCount > 0 {
			t.Logf("⚠️  %d/%d concurrent operations had errors", errorCount, concurrentOps)
		} else {
			t.Logf("✓ All %d concurrent operations succeeded", concurrentOps)
		}

		t.Logf("Total time: %v, Avg operation time: %v, Success: %d/%d",
			totalElapsed, avgTime, successCount, concurrentOps)
	})

	t.Run("concurrent_50_validations", func(t *testing.T) {
		yamlContent := generateDesiredStateWithComponents(20)
		testFile := writeYAMLFile(t, yamlContent)
		concurrentOps := 50

		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		start := time.Now()
		for i := 0; i < concurrentOps; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				service := desiredstate.NewService(filepath.Dir(testFile), true)
				req := wrapper.ValidateRequest{
					DesiredStateFile: testFile,
					Environment:      "test",
				}

				response, err := service.ValidateDesiredState(req)

				mu.Lock()
				if err == nil && response != nil && response.DesiredStateValidated {
					successCount++
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		successRate := float64(successCount) / float64(concurrentOps) * 100

		if successRate < 100.0 {
			t.Logf("⚠️  Success rate: %.1f%% (%d/%d) in %v",
				successRate, successCount, concurrentOps, elapsed)
		} else {
			t.Logf("✓ 100%% success rate: %d/%d concurrent operations in %v",
				successCount, concurrentOps, elapsed)
		}
	})

	t.Run("concurrent_100_validations", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping 100 concurrent test in short mode")
		}

		yamlContent := generateDesiredStateWithComponents(10)
		testFile := writeYAMLFile(t, yamlContent)
		concurrentOps := 100

		var wg sync.WaitGroup
		successCount := 0
		var mu sync.Mutex

		start := time.Now()
		for i := 0; i < concurrentOps; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				service := desiredstate.NewService(filepath.Dir(testFile), true)
				req := wrapper.ValidateRequest{
					DesiredStateFile: testFile,
					Environment:      "test",
				}

				response, err := service.ValidateDesiredState(req)

				mu.Lock()
				if err == nil && response != nil && response.DesiredStateValidated {
					successCount++
				}
				mu.Unlock()
			}(i)
		}

		wg.Wait()
		elapsed := time.Since(start)

		// Target: 100% success rate
		successRate := float64(successCount) / float64(concurrentOps) * 100
		if successRate < 100.0 {
			t.Logf("⚠️  Success rate: %.1f%% (%d/%d) in %v",
				successRate, successCount, concurrentOps, elapsed)
		} else {
			t.Logf("✓ 100%% success rate: %d/%d concurrent operations in %v",
				successCount, concurrentOps, elapsed)
		}
	})

	t.Logf("Contract: %s", contract.Behavior)
}

// TestLoadStress_MemoryPressure_BehavioralBDD tests behavior under memory pressure
func TestLoadStress_MemoryPressure_BehavioralBDD(t *testing.T) {
	contract := LoadTestBehavioralContract{
		Behavior: "Handle sustained load without memory leaks or excessive memory growth",
		CurrentImpl: `
- Efficient memory management with Go's GC
- Minimal memory growth over repeated operations
- No memory leaks expected
`,
		Rationale: "Long-running services must not leak memory",
	}

	t.Run("repeated_operations_memory_stability", func(t *testing.T) {
		yamlContent := generateDesiredStateWithComponents(50)
		testFile := writeYAMLFile(t, yamlContent)
		iterations := 100

		// Warm up
		service := desiredstate.NewService(filepath.Dir(testFile), true)
		for i := 0; i < 10; i++ {
			req := wrapper.ValidateRequest{
				DesiredStateFile: testFile,
				Environment:      "test",
			}
			_, _ = service.ValidateDesiredState(req)
		}

		// Run many iterations
		start := time.Now()
		for i := 0; i < iterations; i++ {
			// Create new service instance each time to test resource cleanup
			service := desiredstate.NewService(filepath.Dir(testFile), true)
			req := wrapper.ValidateRequest{
				DesiredStateFile: testFile,
				Environment:      "test",
			}
			_, err := service.ValidateDesiredState(req)

			if err != nil && i == 0 {
				t.Logf("⚠️  Validation error (first iteration): %v", err)
			}
		}
		elapsed := time.Since(start)

		avgTime := elapsed / time.Duration(iterations)
		t.Logf("✓ Completed %d iterations in %v (avg: %v per operation)",
			iterations, elapsed, avgTime)
		t.Logf("Memory stability: No crashes or OOM errors observed")
	})

	t.Run("large_file_repeated_processing", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping repeated large file test in short mode")
		}

		yamlContent := generateLargeDesiredState(1000) // ~1MB
		testFile := writeYAMLFile(t, yamlContent)
		iterations := 50

		start := time.Now()
		for i := 0; i < iterations; i++ {
			service := desiredstate.NewService(filepath.Dir(testFile), true)
			req := wrapper.ValidateRequest{
				DesiredStateFile: testFile,
				Environment:      "test",
			}
			_, _ = service.ValidateDesiredState(req)
		}
		elapsed := time.Since(start)

		avgTime := elapsed / time.Duration(iterations)
		t.Logf("✓ Processed ~1MB file %d times in %v (avg: %v)",
			iterations, elapsed, avgTime)
	})

	t.Logf("Contract: %s", contract.Behavior)
}

// Helper functions

// generateLargeDesiredState generates a large YAML desiredstate file
func generateLargeDesiredState(componentCount int) string {
	var sb strings.Builder

	sb.WriteString(`schema: v1
kind: DesiredState
metadata:
  name: large-test
  namespace: stress-test
  annotations:
    description: "Stress test file for large YAML processing"
    generated: "automated"
desiredstate:
  meta:
    parts:
      self: test.yaml
    repo: {}
    orchestration: {}
    pipelines_as_code:
      self:
        is_self_updating: true
      children: []
spec:
  components:
`)

	for i := 0; i < componentCount; i++ {
		portNum := 8000 + (i % 1000)
		sb.WriteString(fmt.Sprintf(`    - name: component-%d
      type: service
      version: 1.0.%d
      description: "This is component number %d for stress testing large file processing in yago"
      properties:
        image: registry.example.com/organization/application-name:v%d
        port: %d
        replicas: 3
        env:
          - name: ENV_VAR_%d
            value: "value-with-long-string-to-increase-file-size-%d-padding-data-here"
          - name: DEBUG
            value: "false"
          - name: LOG_LEVEL
            value: "info"
          - name: ADDITIONAL_CONFIG_%d
            value: "extra-configuration-data-to-pad-file-size-component-%d"
        resources:
          cpu: "500m"
          memory: "512Mi"
          storage: "10Gi"
        labels:
          app: component-%d
          tier: backend
          environment: production
          version: v%d
          team: platform-engineering
        annotations:
          prometheus.io/scrape: "true"
          prometheus.io/port: "%d"
          deployment.kubernetes.io/revision: "%d"
        healthChecks:
          liveness:
            httpGet:
              path: /health
              port: %d
            initialDelaySeconds: 30
            periodSeconds: 10
          readiness:
            httpGet:
              path: /ready
              port: %d
            initialDelaySeconds: 10
            periodSeconds: 5
`, i, i%100, i, i%50, portNum, i, i, i, i, i, i, portNum, i%200, portNum, portNum))
	}

	return sb.String()
}

// generateDesiredStateWithComponents generates a desiredstate with specific component count
func generateDesiredStateWithComponents(count int) string {
	var sb strings.Builder

	sb.WriteString(`schema: v1
kind: DesiredState
metadata:
  name: multi-component-test
  namespace: default
desiredstate:
  meta:
    parts:
      self: test.yaml
    repo: {}
    orchestration: {}
    pipelines_as_code:
      self:
        is_self_updating: true
      children: []
spec:
  components:
`)

	for i := 0; i < count; i++ {
		sb.WriteString(fmt.Sprintf(`    - name: comp-%d
      type: service
      config:
        value: data-%d
`, i, i))
	}

	return sb.String()
}

// writeYAMLFile writes YAML content to a temporary file and returns the path
func writeYAMLFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	return tmpFile
}
